package walletmanager

import (
	"context"
	"math"
	"time"

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
	MaxFaucetRequestsForOneOperation = 10
)

func (m *Manager) RequestBlockIssuanceData(ctx context.Context, clt models.Client, account wallet.Account) (*api.CongestionResponse, *api.IssuanceBlockHeaderResponse, iotago.Version, error) {
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

func (m *Manager) getFaucetFundsOutput(ctx context.Context, clt models.Client, wallet *Wallet, addressType iotago.AddressType) (*models.OutputData, error) {
	receiverAddr, privateKey, usedIndex := wallet.getAddress(addressType)

	outputID, output, err := m.RequestFaucetFunds(ctx, clt, receiverAddr)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to request funds from Faucet")
	}
	createdOutput, err := models.NewOutputDataWithID(clt.LatestAPI(), outputID, receiverAddr, usedIndex, privateKey, output)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to create output")
	}

	return createdOutput, nil
}

func (m *Manager) RequestManaAndFundsFromTheFaucet(ctx context.Context, w *Wallet, minManaAmount iotago.Mana, minTokensAmount iotago.BaseToken) ([]*models.OutputData, error) {
	if minManaAmount/m.RequestManaAmount > MaxFaucetRequestsForOneOperation {
		return nil, ierrors.Errorf("required mana is too large, needs more than %d faucet requests", MaxFaucetRequestsForOneOperation)
	}

	if minTokensAmount/m.RequestTokenAmount > MaxFaucetRequestsForOneOperation {
		return nil, ierrors.Errorf("required token amount is too large, needs more than %d faucet requests", MaxFaucetRequestsForOneOperation)
	}
	numOfRequests := int(math.Max(float64(minManaAmount/m.RequestManaAmount), float64(minTokensAmount/m.RequestTokenAmount)))

	inputs := make([]*models.OutputData, 0, numOfRequests)

	// if there is not enough mana to pay for account creation, request mana from the faucet
	for range numOfRequests {
		faucetOutput, err := m.getFaucetFundsOutput(ctx, m.Client, w, iotago.AddressEd25519)
		if err != nil {
			return nil, ierrors.Wrap(err, "failed to get faucet funds for delegation output")
		}
		inputs = append(inputs, faucetOutput)
	}

	return inputs, nil
}

func (m *Manager) RequestFaucetFunds(ctx context.Context, clt models.Client, receiveAddr iotago.Address) (iotago.OutputID, iotago.Output, error) {
	err := clt.RequestFaucetFunds(ctx, receiveAddr)
	if err != nil {
		return iotago.EmptyOutputID, nil, ierrors.Wrap(err, "failed to request funds from faucet")
	}

	outputID, outputStruct, err := utils.AwaitAddressUnspentOutputToBeAccepted(ctx, m.Logger, clt, receiveAddr)
	if err != nil {
		return iotago.EmptyOutputID, nil, ierrors.Wrap(err, "failed to await faucet funds")
	}

	m.LogDebugf("RequestFaucetFunds received faucet funds for addr type: %s, %s", receiveAddr.Type(), receiveAddr.String())

	return outputID, outputStruct, nil
}

func (m *Manager) PostWithBlock(ctx context.Context, clt models.Client, payload iotago.Payload, issuer wallet.Account, congestionResp *api.CongestionResponse, issuerResp *api.IssuanceBlockHeaderResponse, version iotago.Version, strongParents ...iotago.BlockID) (iotago.BlockID, error) {
	signedBlock, err := m.CreateBlock(clt, payload, issuer, congestionResp, issuerResp, version, strongParents...)
	if err != nil {
		m.LogErrorf("failed to create block: %s", err)

		return iotago.EmptyBlockID, err
	}

	blockID, err := clt.PostBlock(ctx, signedBlock)
	if err != nil {
		m.LogErrorf("failed to post block: %s", err)

		return iotago.EmptyBlockID, err
	}

	return blockID, nil
}

func (m *Manager) CreateBlock(clt models.Client, payload iotago.Payload, issuer wallet.Account, congestionResp *api.CongestionResponse, issuerResp *api.IssuanceBlockHeaderResponse, version iotago.Version, strongParents ...iotago.BlockID) (*iotago.Block, error) {
	issuingTime := time.Now()
	issuingSlot := clt.LatestAPI().TimeProvider().SlotFromTime(issuingTime)
	apiForSlot := clt.APIForSlot(issuingSlot)
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
		return nil, ierrors.Errorf("failed to build block: %v", err)
	}

	return blk, nil
}
