// Package pancakeswap loads token and pool metadata from PancakeSwap's BNB
// Chain V3 subgraph. The upstream schema retains the Uniswap-derived field
// name derivedETH even though its value represents BNB on this deployment.
//
// The official Graph Network endpoint requires an API key. Call
// NewClientWithConfig to supply it, or pass a complete custom endpoint to
// NewClient.
package pancakeswap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/kernelflowlabs/wallet-lib/common/httpc"
)

const (
	DefaultSubgraphID  = "Hv1GncLY5docZoGtXjo4kwbTvxm3MAhVZqBZE4sUT9eZ"
	DefaultSubgraphURL = "https://gateway.thegraph.com/api/subgraphs/id/" + DefaultSubgraphID
)

var ErrAPIKeyRequired = errors.New("pancakeswap: The Graph API key is required for the default subgraph")

type Client struct {
	http    *httpc.Request
	initErr error
}

// NewClient preserves the original constructor. A non-empty URL is treated as
// a complete custom endpoint. Passing "" selects the official endpoint, whose
// calls return ErrAPIKeyRequired; use NewClientWithConfig with an API key for
// that endpoint.
func NewClient(subgraphURL string) *Client {
	client, err := NewClientWithConfig(Config{SubgraphURL: subgraphURL})
	if err != nil {
		return &Client{initErr: err}
	}
	return client
}

// NewClientWithAPIKey connects to the official BNB Chain V3 subgraph using a
// bearer API key.
func NewClientWithAPIKey(apiKey string) (*Client, error) {
	return NewClientWithConfig(Config{APIKey: apiKey})
}

// NewClientWithConfig validates the endpoint and configures optional bearer
// authentication without embedding the API key in a URL or error message.
func NewClientWithConfig(config Config) (*Client, error) {
	endpoint := strings.TrimSpace(config.SubgraphURL)
	apiKey := strings.TrimSpace(config.APIKey)
	if endpoint == "" {
		endpoint = DefaultSubgraphURL
	}

	parsed, err := url.ParseRequestURI(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("pancakeswap: invalid subgraph URL")
	}
	endpoint = strings.TrimRight(endpoint, "/")
	if endpoint == DefaultSubgraphURL && apiKey == "" {
		return nil, ErrAPIKeyRequired
	}
	headers := map[string]string{"Content-Type": "application/json"}
	if apiKey != "" {
		headers["Authorization"] = "Bearer " + apiKey
	}
	return &Client{
		http: httpc.NewRequest(endpoint, headers),
	}, nil
}

// GetTokenInfo loads one BNB Chain token and its deepest whitelisted pools.
// The address is validated and normalized to the lower-case subgraph ID form.
// It returns (nil, nil) when the token is not indexed.
func (c *Client) GetTokenInfo(ctx context.Context, contractAddr string) (*TokenInfo, error) {
	if c == nil {
		return nil, fmt.Errorf("pancakeswap: nil client")
	}
	if c.initErr != nil {
		return nil, c.initErr
	}
	if ctx == nil {
		return nil, fmt.Errorf("pancakeswap: nil context")
	}
	contractAddr = strings.TrimSpace(contractAddr)
	if len(contractAddr) != 42 || !common.IsHexAddress(contractAddr) {
		return nil, fmt.Errorf("pancakeswap: invalid token address")
	}
	contractAddr = strings.ToLower(common.HexToAddress(contractAddr).Hex())

	query := `query($id:ID!){
		token(id:$id){
			id symbol decimals derivedETH derivedUSD totalValueLockedUSD volumeUSD
			whitelistPools{
				id feeTier liquidity totalValueLockedUSD
				token0{id symbol}
				token1{id symbol}
			}
		}
	}`
	type respData struct {
		Token *TokenInfo `json:"token"`
	}
	payload, err := json.Marshal(map[string]any{
		"query":     query,
		"variables": map[string]any{"id": contractAddr},
	})
	if err != nil {
		return nil, err
	}
	var out graphResp[respData]
	if err := c.http.PostWithOutEncoded(ctx, &out, "", payload); err != nil {
		return nil, fmt.Errorf("pancakeswap subgraph post: %w", err)
	}
	if out.Message != "" {
		return nil, fmt.Errorf("pancakeswap graphql: %s", out.Message)
	}
	if len(out.Errors) > 0 {
		messages := make([]string, 0, len(out.Errors))
		for _, graphErr := range out.Errors {
			if message := strings.TrimSpace(graphErr.Message); message != "" {
				messages = append(messages, message)
			}
		}
		if len(messages) == 0 {
			return nil, fmt.Errorf("pancakeswap graphql: unknown error")
		}
		return nil, fmt.Errorf("pancakeswap graphql: %s", strings.Join(messages, "; "))
	}
	if out.Data.Token != nil {
		out.Data.Token.WhitelistPools = deepestPools(out.Data.Token.WhitelistPools, 5)
	}
	return out.Data.Token, nil
}

func deepestPools(pools []*Pool, limit int) []*Pool {
	if len(pools) == 0 || limit <= 0 {
		return nil
	}
	result := append([]*Pool(nil), pools...)
	sort.SliceStable(result, func(i, j int) bool {
		left, leftOK := poolTVL(result[i])
		right, rightOK := poolTVL(result[j])
		if leftOK != rightOK {
			return leftOK
		}
		return leftOK && left.Cmp(right) > 0
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func poolTVL(pool *Pool) (*big.Rat, bool) {
	if pool == nil {
		return nil, false
	}
	value, ok := new(big.Rat).SetString(strings.TrimSpace(pool.TotalValueLockedUSD))
	return value, ok
}
