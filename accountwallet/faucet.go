package accountwallet

import (
	"context"
	"sync"
	"time"

	"github.com/mr-tron/base58"

	"github.com/iotaledger/evil-tools/models"
	"github.com/iotaledger/evil-tools/utils"
	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/lo"
	"github.com/iotaledger/iota-core/pkg/blockhandler"
	"github.com/iotaledger/iota-core/pkg/testsuite/mock"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/nodeclient"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

const (
	FaucetAccountAlias = "faucet"
)

func (a *AccountWallet) RequestBlockBuiltData(clt *nodeclient.Client, issuerID iotago.AccountID) (*apimodels.CongestionResponse, *apimodels.IssuanceBlockHeaderResponse, iotago.Version, error) {
	congestionResp, err := clt.Congestion(context.Background(), issuerID)
	if err != nil {
		return nil, nil, 0, ierrors.Wrapf(err, "failed to get congestion data for issuer %s", issuerID.ToHex())
	}

	issuerResp, err := clt.BlockIssuance(context.Background(), congestionResp.Slot)
	if err != nil {
		return nil, nil, 0, ierrors.Wrap(err, "failed to get block issuance data")
	}

	version := clt.APIForSlot(congestionResp.Slot).Version()

	return congestionResp, issuerResp, version, nil
}

func (a *AccountWallet) RequestFaucetFunds(clt models.Client, receiveAddr iotago.Address) (*models.Output, error) {
	err := clt.RequestFaucetFunds(receiveAddr)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to request funds from faucet")
	}

	outputID, outputStruct, err := utils.AwaitAddressUnspentOutputToBeAccepted(clt, receiveAddr)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to await faucet funds")
	}

	return &models.Output{
		OutputID:     outputID,
		Address:      receiveAddr,
		AddressIndex: 0,
		Balance:      outputStruct.BaseTokenAmount(),
		OutputStruct: outputStruct,
	}, nil
}

func (a *AccountWallet) PostWithBlock(clt models.Client, payload iotago.Payload, issuer blockhandler.Account, congestionResp *apimodels.CongestionResponse, issuerResp *apimodels.IssuanceBlockHeaderResponse, version iotago.Version, strongParents ...iotago.BlockID) (iotago.BlockID, error) {
	signedBlock, err := a.CreateBlock(payload, issuer, congestionResp, issuerResp, version, strongParents...)
	if err != nil {
		log.Errorf("failed to create block: %s", err)

		return iotago.EmptyBlockID, err
	}

	blockID, err := clt.PostBlock(signedBlock)
	if err != nil {
		log.Errorf("failed to post block: %s", err)

		return iotago.EmptyBlockID, err
	}

	return blockID, nil
}

func (a *AccountWallet) CreateBlock(payload iotago.Payload, issuer blockhandler.Account, congestionResp *apimodels.CongestionResponse, issuerResp *apimodels.IssuanceBlockHeaderResponse, version iotago.Version, strongParents ...iotago.BlockID) (*iotago.ProtocolBlock, error) {
	issuingTime := time.Now()
	issuingSlot := a.client.LatestAPI().TimeProvider().SlotFromTime(issuingTime)
	apiForSlot := a.client.APIForSlot(issuingSlot)
	if congestionResp == nil {
		var err error
		congestionResp, err = a.client.GetCongestion(issuer.ID())
		if err != nil {
			return nil, ierrors.Wrap(err, "failed to get congestion data")
		}
	}

	blockBuilder := builder.NewBasicBlockBuilder(apiForSlot)

	commitmentID, err := issuerResp.Commitment.ID()
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get commitment id")
	}

	blockBuilder.ProtocolVersion(version)
	blockBuilder.SlotCommitmentID(commitmentID)
	blockBuilder.LatestFinalizedSlot(issuerResp.LatestFinalizedSlot)
	blockBuilder.IssuingTime(time.Now())
	blockBuilder.StrongParents(append(issuerResp.StrongParents, strongParents...))
	blockBuilder.WeakParents(issuerResp.WeakParents)
	blockBuilder.ShallowLikeParents(issuerResp.ShallowLikeParents)

	blockBuilder.Payload(payload)
	blockBuilder.CalculateAndSetMaxBurnedMana(congestionResp.ReferenceManaCost)
	blockBuilder.Sign(issuer.ID(), issuer.PrivateKey())

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
	unspentOutput   *models.Output
	account         blockhandler.Account
	genesisHdWallet *mock.HDWallet

	clt models.Client

	sync.Mutex
}

func newFaucet(clt models.Client, faucetParams *faucetParams) (*faucet, error) {
	genesisSeed, err := base58.Decode(faucetParams.genesisSeed)
	if err != nil {
		log.Warnf("failed to decode base58 seed, using the default one: %v", err)
	}
	faucetAddr := mock.NewHDWallet("", genesisSeed, 0).Address(iotago.AddressEd25519)

	f := &faucet{
		clt:             clt,
		account:         blockhandler.AccountFromParams(faucetParams.faucetAccountID, faucetParams.faucetPrivateKey),
		genesisHdWallet: mock.NewHDWallet("", genesisSeed, 0),
	}

	faucetUnspentOutput, faucetUnspentOutputID, faucetAmount, err := f.getGenesisOutputFromIndexer(clt, faucetAddr)
	if err != nil {
		return nil, ierrors.Wrap(err, "failed to get faucet output from indexer")
	}

	//nolint:all,forcetypassert
	f.unspentOutput = &models.Output{
		Address:      faucetAddr.(*iotago.Ed25519Address),
		AddressIndex: 0,
		OutputID:     faucetUnspentOutputID,
		Balance:      faucetAmount,
		OutputStruct: faucetUnspentOutput,
	}

	return f, nil
}

func (f *faucet) getGenesisOutputFromIndexer(clt models.Client, faucetAddr iotago.DirectUnlockableAddress) (iotago.Output, iotago.OutputID, iotago.BaseToken, error) {
	indexer, err := clt.Indexer()
	if err != nil {
		log.Errorf("account wallet failed due to failure in connecting to indexer")

		return nil, iotago.EmptyOutputID, 0, ierrors.Wrapf(err, "failed to get indexer from client")
	}

	results, err := indexer.Outputs(context.Background(), &apimodels.BasicOutputsQuery{
		AddressBech32: faucetAddr.Bech32(iotago.PrefixTestnet),
	})
	if err != nil {
		return nil, iotago.EmptyOutputID, 0, ierrors.Wrap(err, "failed to prepare faucet unspent outputs indexer request")
	}

	var (
		faucetUnspentOutput   iotago.Output
		faucetUnspentOutputID iotago.OutputID
		faucetAmount          iotago.BaseToken
	)
	for results.Next() {
		unspents, err := results.Outputs(context.TODO())
		if err != nil {
			return nil, iotago.EmptyOutputID, 0, ierrors.Wrap(err, "failed to get faucet unspent outputs")
		}

		faucetUnspentOutput = unspents[0]
		faucetAmount = faucetUnspentOutput.BaseTokenAmount()
		faucetUnspentOutputID = lo.Return1(results.Response.Items.OutputIDs())[0]
	}

	return faucetUnspentOutput, faucetUnspentOutputID, faucetAmount, nil
}
