package accountwallet

import (
	"context"
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
	GenesisAccountAlias = "genesis-account"
)

func (a *AccountWallet) RequestBlockIssuanceData(ctx context.Context, clt models.Client, account wallet.Account) (*api.CongestionResponse, *api.IssuanceBlockHeaderResponse, iotago.Version, error) {
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

func (a *AccountWallet) getFaucetFundsOutput(ctx context.Context, addressType iotago.AddressType) (*models.Output, error) {
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
	err := a.Client.RequestFaucetFunds(ctx, receiveAddr)
	if err != nil {
		return iotago.EmptyOutputID, nil, ierrors.Wrap(err, "failed to request funds from faucet")
	}

	outputID, outputStruct, err := utils.AwaitAddressUnspentOutputToBeAccepted(ctx, a.Logger, a.Client, receiveAddr)
	if err != nil {
		return iotago.EmptyOutputID, nil, ierrors.Wrap(err, "failed to await faucet funds")
	}

	a.LogDebugf("RequestFaucetFunds received faucet funds for addr type: %s, %s", receiveAddr.Type(), receiveAddr.String())

	return outputID, outputStruct, nil
}

func (a *AccountWallet) PostWithBlock(ctx context.Context, clt models.Client, payload iotago.Payload, issuer wallet.Account, congestionResp *api.CongestionResponse, issuerResp *api.IssuanceBlockHeaderResponse, version iotago.Version, strongParents ...iotago.BlockID) (iotago.BlockID, error) {
	signedBlock, err := a.CreateBlock(payload, issuer, congestionResp, issuerResp, version, strongParents...)
	if err != nil {
		a.LogErrorf("failed to create block: %s", err)

		return iotago.EmptyBlockID, err
	}

	blockID, err := clt.PostBlock(ctx, signedBlock)
	if err != nil {
		a.LogErrorf("failed to post block: %s", err)

		return iotago.EmptyBlockID, err
	}

	return blockID, nil
}

func (a *AccountWallet) CreateBlock(payload iotago.Payload, issuer wallet.Account, congestionResp *api.CongestionResponse, issuerResp *api.IssuanceBlockHeaderResponse, version iotago.Version, strongParents ...iotago.BlockID) (*iotago.Block, error) {
	issuingTime := time.Now()
	issuingSlot := a.Client.LatestAPI().TimeProvider().SlotFromTime(issuingTime)
	apiForSlot := a.Client.APIForSlot(issuingSlot)
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
