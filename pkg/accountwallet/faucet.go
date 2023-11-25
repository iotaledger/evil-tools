package accountwallet

import (
	"context"
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
	GenesisAccountAlias   = "genesis-account"
	MaxFaucetManaRequests = 10
)

func (a *AccountWallet) RequestBlockBuiltData(ctx context.Context, clt models.Client, account wallet.Account) (*api.CongestionResponse, *api.IssuanceBlockHeaderResponse, iotago.Version, error) {
	issuerResp, err := clt.GetBlockIssuance(ctx)
	if err != nil {
		return nil, nil, 0, ierrors.Wrap(err, "failed to get block issuance data")
	}

	// TODO: pass commitmentID from issuerResp
	congestionResp, err := clt.GetCongestion(ctx, account.Address())
	if err != nil {
		return nil, nil, 0, ierrors.Wrapf(err, "failed to get congestion data for issuer %s", account.Address())
	}

	version := clt.APIForSlot(issuerResp.Commitment.Slot).Version()

	return congestionResp, issuerResp, version, nil
}

func (a *AccountWallet) RequestFaucetFunds(ctx context.Context, receiveAddr iotago.Address) (*models.Output, error) {
	err := a.client.RequestFaucetFunds(ctx, receiveAddr)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to request funds from faucet")
	}

	outputID, outputStruct, err := utils.AwaitAddressUnspentOutputToBeAccepted(ctx, a.client, receiveAddr)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to await faucet funds")
	}

	return &models.Output{
		OutputID:     outputID,
		Address:      receiveAddr,
		AddressIndex: 0,
		OutputStruct: outputStruct,
	}, nil
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

	commitmentID, err := issuerResp.Commitment.ID()
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

func newFaucet(clt models.Client, faucetParams *faucetParams) (*faucet, error) {
	genesisSeed, err := base58.Decode(faucetParams.genesisSeed)
	if err != nil {
		log.Warnf("failed to decode base58 seed, using the default one: %v", err)
	}

	f := &faucet{
		clt:               clt,
		account:           lo.PanicOnErr(wallet.AccountFromParams(faucetParams.faucetAccountID, faucetParams.faucetPrivateKey)),
		genesisKeyManager: lo.PanicOnErr(wallet.NewKeyManager(genesisSeed, 0)),
	}

	return f, nil
}
