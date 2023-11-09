package models

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/google/martian/log"

	"github.com/iotaledger/hive.go/ierrors"
	"github.com/iotaledger/hive.go/runtime/options"
	"github.com/iotaledger/hive.go/runtime/syncutils"
	"github.com/iotaledger/inx-faucet/pkg/faucet"
	"github.com/iotaledger/iota-core/pkg/model"
	iotago "github.com/iotaledger/iota.go/v4"
	"github.com/iotaledger/iota.go/v4/builder"
	"github.com/iotaledger/iota.go/v4/nodeclient"
	"github.com/iotaledger/iota.go/v4/nodeclient/apimodels"
)

type ServerInfo struct {
	Healthy bool
	Version string
}

type ServerInfos []*ServerInfo

type Connector interface {
	// ServersStatuses retrieves the connected server status for each client.
	ServersStatuses() ServerInfos
	// ServerStatus retrieves the connected server status.
	ServerStatus(cltIdx int) (status *ServerInfo, err error)
	// Clients returns list of all clients.
	Clients(...bool) []Client
	// GetClients returns the numOfClt client instances that were used the longest time ago.
	GetClients(numOfClt int) []Client
	// AddClient adds a client to WebClients based on provided GoShimmerAPI url.
	AddClient(url string, setters ...options.Option[WebClient])
	// RemoveClient removes a client with the provided url from the WebClients.
	RemoveClient(url string)
	// GetClient returns the client instance that was used the longest time ago.
	GetClient() Client
	// GetIndexerClient returns the indexer client instance.
	GetIndexerClient() Client
}

// WebClients is responsible for handling connections via GoShimmerAPI.
type WebClients struct {
	clients        []*WebClient
	indexerClients []*WebClient
	urls           []string
	faucetURL      string

	// helper variable indicating which clt was recently used, useful for double, triple,... spends
	lastUsed int

	mu syncutils.Mutex
}

// NewWebClients creates Connector from provided GoShimmerAPI urls.
func NewWebClients(urls []string, faucetURL string, setters ...options.Option[WebClient]) *WebClients {
	clients := make([]*WebClient, len(urls))
	indexers := make([]*WebClient, 0)
	var err error
	for i, url := range urls {
		clients[i], err = NewWebClient(url, faucetURL, setters...)
		if err != nil {
			log.Errorf("failed to create client for url %s: %s", url, err)

			return nil
		}

		if _, err := clients[i].client.Indexer(context.Background()); err == nil {
			indexers = append(indexers, clients[i])
		}
	}

	return &WebClients{
		clients:        clients,
		indexerClients: indexers,
		urls:           urls,
		faucetURL:      faucetURL,
		lastUsed:       -1,
	}
}

// ServersStatuses retrieves the connected server status for each client.
func (c *WebClients) ServersStatuses() ServerInfos {
	status := make(ServerInfos, len(c.clients))

	for i := range c.clients {
		status[i], _ = c.ServerStatus(i)
	}

	return status
}

// ServerStatus retrieves the connected server status.
func (c *WebClients) ServerStatus(cltIdx int) (status *ServerInfo, err error) {
	response, err := c.clients[cltIdx].client.Info(context.Background())
	if err != nil {
		return nil, err
	}

	return &ServerInfo{
		Healthy: response.Status.IsHealthy,
		Version: response.Version,
	}, nil
}

// Clients returns list of all clients.
func (c *WebClients) Clients(...bool) []Client {
	clients := make([]Client, len(c.clients))
	for i, c := range c.clients {
		clients[i] = c
	}

	return clients
}

// GetClients returns the numOfClt client instances that were used the longest time ago.
func (c *WebClients) GetClients(numOfClt int) []Client {
	c.mu.Lock()
	defer c.mu.Unlock()

	clts := make([]Client, numOfClt)

	for i := range clts {
		clts[i] = c.getClient()
	}

	return clts
}

func (c *WebClients) GetIndexerClient() Client {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.indexerClients[0]
}

// getClient returns the client instance that was used the longest time ago, not protected by mutex.
func (c *WebClients) getClient() Client {
	if c.lastUsed >= len(c.clients)-1 {
		c.lastUsed = 0
	} else {
		c.lastUsed++
	}

	return c.clients[c.lastUsed]
}

// GetClient returns the client instance that was used the longest time ago.
func (c *WebClients) GetClient() Client {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.getClient()
}

// AddClient adds client to WebClients based on provided GoShimmerAPI url.
func (c *WebClients) AddClient(url string, setters ...options.Option[WebClient]) {
	c.mu.Lock()
	defer c.mu.Unlock()

	clt, err := NewWebClient(url, c.faucetURL, setters...)
	if err != nil {
		log.Errorf("failed to create client for url %s: %s", url, err)

		return
	}
	c.clients = append(c.clients, clt)
	c.urls = append(c.urls, url)
}

// RemoveClient removes client with the provided url from the WebClients.
func (c *WebClients) RemoveClient(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	indexToRemove := -1
	for i, u := range c.urls {
		if u == url {
			indexToRemove = i
			break
		}
	}
	if indexToRemove == -1 {
		return
	}
	c.clients = append(c.clients[:indexToRemove], c.clients[indexToRemove+1:]...)
	c.urls = append(c.urls[:indexToRemove], c.urls[indexToRemove+1:]...)
}

type Client interface {
	Client() *nodeclient.Client
	Indexer() (nodeclient.IndexerClient, error)
	// URL returns a client API url.
	URL() (cltID string)
	// PostBlock sends a block to the Tangle via a given client.
	PostBlock(block *iotago.Block) (iotago.BlockID, error)
	// PostData sends the given data (payload) by creating a block in the backend.
	PostData(data []byte) (blkID string, err error)
	// GetBlockConfirmationState returns the AcceptanceState of a given block ID.
	GetBlockConfirmationState(blkID iotago.BlockID) (resp *apimodels.BlockMetadataResponse, err error)
	// GetBlockStateFromTransaction returns the AcceptanceState of a given transaction ID.
	GetBlockStateFromTransaction(txID iotago.TransactionID) (resp *apimodels.BlockMetadataResponse, err error)
	// GetOutput gets the output of a given outputID.
	GetOutput(outputID iotago.OutputID) iotago.Output
	// GetOutputConfirmationState gets the first unspent outputs of a given address.
	GetOutputConfirmationState(outputID iotago.OutputID) string
	// GetTransaction gets the transaction.
	GetTransaction(txID iotago.TransactionID) (resp *iotago.SignedTransaction, err error)
	// GetBlockIssuance returns the latest commitment and data needed to create a new block.
	GetBlockIssuance(...iotago.SlotIndex) (resp *apimodels.IssuanceBlockHeaderResponse, err error)
	// GetCongestion returns congestion data such as rmc or issuing readiness.
	GetCongestion(id iotago.AccountID) (resp *apimodels.CongestionResponse, err error)
	// RequestFaucetFunds requests funds from the faucet.
	RequestFaucetFunds(address iotago.Address) (err error)
	// GetAccountFromIndexer returns the outputID, accountOutput and slotIndex of a given accountID.
	GetAccountFromIndexer(accountID iotago.AccountID) (*iotago.OutputID, *iotago.AccountOutput, iotago.SlotIndex, error)

	iotago.APIProvider
}

// WebClient contains a GoShimmer web API to interact with a node.
type WebClient struct {
	client    *nodeclient.Client
	url       string
	faucetURL string
}

func (c *WebClient) Client() *nodeclient.Client {
	return c.client
}

func (c *WebClient) Indexer() (nodeclient.IndexerClient, error) {
	return c.client.Indexer(context.Background())
}

func (c *WebClient) APIForVersion(version iotago.Version) (iotago.API, error) {
	return c.client.APIForVersion(version)
}

func (c *WebClient) APIForTime(t time.Time) iotago.API {
	return c.client.APIForTime(t)
}

func (c *WebClient) APIForSlot(index iotago.SlotIndex) iotago.API {
	return c.client.APIForSlot(index)
}

func (c *WebClient) APIForEpoch(index iotago.EpochIndex) iotago.API {
	return c.client.APIForEpoch(index)
}

func (c *WebClient) CommittedAPI() iotago.API {
	return c.client.CommittedAPI()
}

func (c *WebClient) LatestAPI() iotago.API {
	return c.client.LatestAPI()
}

// URL returns a client API Url.
func (c *WebClient) URL() string {
	return c.url
}

// NewWebClient creates Connector from provided iota-core API urls.
func NewWebClient(url, faucetURL string, opts ...options.Option[WebClient]) (*WebClient, error) {
	var initErr error
	return options.Apply(&WebClient{
		url:       url,
		faucetURL: faucetURL,
	}, opts, func(w *WebClient) {
		w.client, initErr = nodeclient.New(w.url)
	}), initErr
}

func (c *WebClient) RequestFaucetFunds(address iotago.Address) (err error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, c.faucetURL+"/api/enqueue", func() io.Reader {
		jsonData, _ := json.Marshal(&faucet.EnqueueRequest{
			Address: address.Bech32(c.client.CommittedAPI().ProtocolParameters().Bech32HRP()),
		})

		return bytes.NewReader(jsonData)
	}())
	if err != nil {
		return ierrors.Errorf("unable to build http request: %w", err)
	}

	req.Header.Set("Content-Type", nodeclient.MIMEApplicationJSON)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return ierrors.Errorf("client: error making http request: %s\n", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusAccepted {
		return ierrors.Errorf("faucet request failed: %s", res.Body)
	}

	return nil
}

func (c *WebClient) PostBlock(block *iotago.Block) (blockID iotago.BlockID, err error) {
	return c.client.SubmitBlock(context.Background(), block)
}

// PostData sends the given data (payload) by creating a block in the backend.
func (c *WebClient) PostData(data []byte) (blkID string, err error) {
	blockBuilder := builder.NewBasicBlockBuilder(c.client.CommittedAPI())
	blockBuilder.IssuingTime(time.Time{})

	blockBuilder.Payload(&iotago.TaggedData{
		Tag: data,
	})

	blk, err := blockBuilder.Build()
	if err != nil {
		return iotago.EmptyBlockID.ToHex(), err
	}

	id, err := c.client.SubmitBlock(context.Background(), blk)
	if err != nil {
		return
	}

	return id.ToHex(), nil
}

// GetOutputConfirmationState gets the first unspent outputs of a given address.
func (c *WebClient) GetOutputConfirmationState(outputID iotago.OutputID) string {
	txID := outputID.TransactionID()
	resp, err := c.GetBlockStateFromTransaction(txID)
	if err != nil {
		return ""
	}

	return resp.TransactionState
}

// GetOutput gets the output of a given outputID.
func (c *WebClient) GetOutput(outputID iotago.OutputID) iotago.Output {
	res, err := c.client.OutputByID(context.Background(), outputID)
	if err != nil {
		return nil
	}

	return res
}

func (c *WebClient) GetAccountFromIndexer(accountID iotago.AccountID) (*iotago.OutputID, *iotago.AccountOutput, iotago.SlotIndex, error) {
	indexer, err := c.Indexer()
	if err != nil {
		return nil, nil, 0, ierrors.Errorf("unable to get indexer client: %w", err)
	}

	return indexer.Account(context.Background(), accountID)
}

// GetBlockConfirmationState returns the AcceptanceState of a given block ID.
func (c *WebClient) GetBlockConfirmationState(blkID iotago.BlockID) (*apimodels.BlockMetadataResponse, error) {
	return c.client.BlockMetadataByBlockID(context.Background(), blkID)
}

// GetBlockStateFromTransaction returns the AcceptanceState of a given transaction ID.
func (c *WebClient) GetBlockStateFromTransaction(txID iotago.TransactionID) (*apimodels.BlockMetadataResponse, error) {
	return c.client.TransactionIncludedBlockMetadata(context.Background(), txID)
}

// GetTransaction gets the transaction.
func (c *WebClient) GetTransaction(txID iotago.TransactionID) (tx *iotago.SignedTransaction, err error) {
	resp, err := c.client.TransactionIncludedBlock(context.Background(), txID)
	if err != nil {
		return
	}

	modelBlk, err := model.BlockFromBlock(resp)
	if err != nil {
		return
	}

	tx, _ = modelBlk.SignedTransaction()

	return tx, nil
}

func (c *WebClient) GetBlockIssuance(slotIndex ...iotago.SlotIndex) (resp *apimodels.IssuanceBlockHeaderResponse, err error) {
	return c.client.BlockIssuance(context.Background(), slotIndex...)
}

func (c *WebClient) GetCongestion(accountID iotago.AccountID) (resp *apimodels.CongestionResponse, err error) {
	return c.client.Congestion(context.Background(), accountID)
}
