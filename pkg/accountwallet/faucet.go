package accountwallet

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/mr-tron/base58"

	"github.com/iotaledger/evil-tools/pkg/models"
	"github.com/iotaledger/evil-tools/pkg/utils"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/api"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/wallet"
)

const (
	GenesisAccountAlias              = "genesis-account"
	MaxFaucetRequestsForOneOperation = 10
)

func (a *AccountWallet) RequestBlockBuiltData(ctx context.Context, clt models.Client, account wallet.Account) (*api.CongestionResponse, *api.IssuanceBlockHeaderResponse, iotago.Version, error) {
	// TODO this should be later remove after congestion for implicit account will work
	issuerResp, err := clt.GetBlockIssuance(ctx)
	if err != nil {
		return nil, nil, 0, ierrors.Wrapf(err, "failed to get block issuance data for accID %s, addr %s", account.ID().ToHex(), account.Address().String())
	}

	congestionResp, err := clt.GetCongestion(ctx, account.Address(), lo.PanicOnErr(issuerResp.LatestCommitment.ID()))
	if err != nil {
		return nil, nil, 0, ierrors.Wrapf(err, "failed to get congestion data for issuer accID %s, addr %s", account.ID(), account.Address())
	}

	version := clt.APIForSlot(issuerResp.LatestCommitment.Slot).Version()

	return congestionResp, issuerResp, version, nil
}

func (a *AccountWallet) requestEnoughManaForAccountCreation(ctx context.Context, currentMana iotago.Mana) ([]*models.Output, error) {
	requiredMana, err := a.estimateMinimumRequiredMana(ctx, 1, 0, false, true)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to estimate number of faucet requests to cover minimum required mana")
	}
	log.Debugf("Mana required for account creation: %d, requesting additional mana from the faucet", requiredMana)
	if currentMana >= requiredMana {
		return []*models.Output{}, nil
	}
	additionalBasicInputs, err := a.RequestManaAndFundsFromTheFaucet(ctx, requiredMana, 0)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to request mana from the faucet")
	}
	log.Debugf("successfully requested %d new outputs from the faucet", len(additionalBasicInputs))

	return additionalBasicInputs, nil
}

// RequestManaAndFundsFromTheFaucet sends as many faucet requests to cover for the provided minimum mana and token amount.
// TODO does not work, need to debug
func (a *AccountWallet) RequestManaAndFundsFromTheFaucet(ctx context.Context, minManaAmount iotago.Mana, minTokensAmount iotago.BaseToken) ([]*models.Output, error) {
	if minManaAmount/a.faucet.RequestManaAmount > MaxFaucetRequestsForOneOperation {
		return nil, ierrors.Errorf("required mana is too large, needs more than %d faucet requests", MaxFaucetRequestsForOneOperation)
	}

	if minTokensAmount/a.faucet.RequestTokenAmount > MaxFaucetRequestsForOneOperation {
		return nil, ierrors.Errorf("required token amount is too large, needs more than %d faucet requests", MaxFaucetRequestsForOneOperation)
	}
	numOfRequests := int(math.Max(float64(minManaAmount/a.faucet.RequestManaAmount), float64(minTokensAmount/a.faucet.RequestTokenAmount)))

	outputsChan := make(chan *models.Output)
	doneChan := make(chan struct{})
	// if there is not enough mana to pay for account creation, request mana from the faucet
	for i := 0; i < numOfRequests; i++ {
		go func(ctx context.Context, outputsChan chan *models.Output, doneChan chan struct{}) {
			defer func() {
				doneChan <- struct{}{}
			}()

			faucetOutput, err := a.getModelFunds(ctx, iotago.AddressEd25519)
			if err != nil {
				log.Errorf("failed to request funds from the faucet: %v", err)

				return
			}

			outputsChan <- faucetOutput
		}(ctx, outputsChan, doneChan)
	}

	outputs := make([]*models.Output, 0)
	for requestsProcessed := 0; requestsProcessed < numOfRequests; {
		select {
		case faucetOutput := <-outputsChan:
			log.Debugf("Faucet requests, received faucet output: %s", faucetOutput.OutputID.ToHex())
			outputs = append(outputs, faucetOutput)
		case <-doneChan:
			requestsProcessed++
		}
	}

	return outputs, nil
}

func (a *AccountWallet) getModelFunds(ctx context.Context, addressType iotago.AddressType) (*models.Output, error) {
	receiverAddr, privateKey, usedIndex := a.getAddress(addressType)

	outputID, output, err := a.RequestFaucetFunds(ctx, receiverAddr)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to request funds from Faucet")
	}
	createdOutput, err := models.NewOutputWithID(a.API, outputID, receiverAddr, usedIndex, privateKey, output)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to create output")
	}

	return createdOutput, nil
}

func (a *AccountWallet) RequestFaucetFunds(ctx context.Context, receiveAddr iotago.Address) (iotago.OutputID, iotago.Output, error) {
	err := a.client.RequestFaucetFunds(ctx, receiveAddr)
	if err != nil {
		return iotago.EmptyOutputID, nil, ierrors.Wrap(err, "failed to request funds from faucet")
	}

	outputID, outputStruct, err := utils.AwaitAddressUnspentOutputToBeAccepted(ctx, a.client, receiveAddr)
	if err != nil {
		return iotago.EmptyOutputID, nil, ierrors.Wrap(err, "failed to await faucet funds")
	}

	log.Debugf("RequestFaucetFunds received faucet funds for addr type: %s, %s", receiveAddr.Type(), receiveAddr.String())

	return outputID, outputStruct, nil
}

func (a *AccountWallet) PostWithBlock(ctx context.Context, clt models.Client, payload iotago.Payload, issuer wallet.Account, congestionResp *api.CongestionResponse, issuerResp *api.IssuanceBlockHeaderResponse, version iotago.Version, strongParents ...iotago.BlockID) (iotago.BlockID, error) {
	signedBlock, err := a.CreateBlock(payload, issuer, congestionResp, issuerResp, version, strongParents...)
	if err != nil {
		log.Errorf("failed to create block: %s", err)

		return iotago.EmptyBlockID, err
	}

	blockID, err := clt.PostBlock(ctx, signedBlock)
	if err != nil {
		log.Errorf("failed to post block: %s", err)

		return iotago.EmptyBlockID, err
	}

	return blockID, nil
}

func (a *AccountWallet) CreateBlock(payload iotago.Payload, issuer wallet.Account, congestionResp *api.CongestionResponse, issuerResp *api.IssuanceBlockHeaderResponse, version iotago.Version, strongParents ...iotago.BlockID) (*iotago.Block, error) {
	issuingTime := time.Now()
	issuingSlot := a.client.LatestAPI().TimeProvider().SlotFromTime(issuingTime)
	apiForSlot := a.client.APIForSlot(issuingSlot)
	blockBuilder := builder.NewBasicBlockBuilder(apiForSlot)

	commitmentID, err := issuerResp.LatestCommitment.ID()
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get commitment id")
	}

	blockBuilder.ProtocolVersion(version)
	blockBuilder.SlotCommitmentID(commitmentID)
	blockBuilder.LatestFinalizedSlot(issuerResp.LatestFinalizedSlot)
	blockBuilder.IssuingTime(issuingTime)
	blockBuilder.StrongParents(append(issuerResp.StrongParents, strongParents...))
	blockBuilder.WeakParents(issuerResp.WeakParents)
	blockBuilder.ShallowLikeParents(issuerResp.ShallowLikeParents)

	blockBuilder.Payload(payload)
	blockBuilder.CalculateAndSetMaxBurnedMana(congestionResp.ReferenceManaCost)
	blockBuilder.Sign(issuer.Address().AccountID(), issuer.PrivateKey())

	blk, err := blockBuilder.Build()
	if err != nil {
		return nil, ierrors.Errorf("failed to build block: %w", err)
	}

	return blk, nil
}

type faucetParams struct {
	faucetPrivateKey string
	faucetAccountID  string
	genesisSeed      string
}

type faucet struct {
	account           wallet.Account
	genesisKeyManager *wallet.KeyManager

	RequestTokenAmount iotago.BaseToken
	RequestManaAmount  iotago.Mana

	clt models.Client

	sync.Mutex
}

func newFaucet(clt models.Client, faucetParams *faucetParams) *faucet {
	genesisSeed, err := base58.Decode(faucetParams.genesisSeed)
	if err != nil {
		log.Warnf("failed to decode base58 seed, using the default one: %v", err)
	}

	f := &faucet{
		clt:               clt,
		account:           lo.PanicOnErr(wallet.AccountFromParams(faucetParams.faucetAccountID, faucetParams.faucetPrivateKey)),
		genesisKeyManager: lo.PanicOnErr(wallet.NewKeyManager(genesisSeed, 0)),
	}

	return f
}
