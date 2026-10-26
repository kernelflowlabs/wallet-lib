package lifi

import (
	"context"
	"fmt"
	dexmodel "github.com/kernelflowlabs/wallet-lib/common/dexmodel"
	"github.com/kernelflowlabs/wallet-lib/common/httpc"
	"net/url"
	"strconv"
	"strings"
)

type Client struct {
	client     *httpc.Request
	integrator string
}

func NewClient(opts ...ClientOption) *Client {
	cfg := clientConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	headers := map[string]string{"Content-Type": "application/json"}
	if cfg.apiKey != "" {
		headers["x-lifi-api-key"] = cfg.apiKey
	}
	c := httpc.NewRequest("https://li.quest", headers)
	cfg.rateLimit.Apply(c)
	return &Client{
		client:     c,
		integrator: cfg.integrator,
	}
}

func (c *Client) Chains(ctx context.Context) ([]string, error) {
	path := "v1/chains"

	query := url.Values{}
	query.Set("chainTypes", "EVM,UTXO,SVM,MVM,TVM")

	out := &ChainsRes{}
	err := c.client.Get(ctx, out, path, query)
	if err != nil {
		return nil, fmt.Errorf("fail to get chains, err=%v", err)
	}

	var res []string
	for _, v := range out.Chains {
		if chain, ok := idChainMapper[strconv.Itoa(v.ID)]; ok {
			res = append(res, chain)
		}
	}
	return res, nil
}

func (c *Client) Tokens(ctx context.Context) error {
	path := "v1/tokens"

	query := url.Values{}
	//query.Set("chainTypes", "EVM,UTXO,SVM,MVM")
	query.Set("chainTypes", "EVM")

	out := &TokensRes{}
	err := c.client.Get(ctx, out, path, query)
	if err != nil {
		return fmt.Errorf("fail to get chains, err=%v", err)
	}
	return nil
}

func (c *Client) Quote(ctx context.Context, in *dexmodel.DexQuoteIn) (*dexmodel.DexQuoteOut, error) {
	req := c.toLifiQuoteReq(in)
	if req == nil {
		return nil, fmt.Errorf("fail to convert request to QuoteReq")
	}

	path := "v1/quote"

	query := url.Values{}
	query.Set("fromChain", req.FromChain)
	query.Set("toChain", req.ToChain)
	query.Set("fromToken", req.FromToken)
	query.Set("toToken", req.ToToken)
	query.Set("fromAddress", req.FromAddress)
	query.Set("toAddress", req.ToAddress)
	query.Set("fromAmount", req.FromAmount)
	query.Set("slippage", req.Slippage)
	setProviderFilters(query, in.IncludeProvider, in.ExcludeProvider)
	if c.integrator != "" {
		query.Set("integrator", c.integrator)
	}
	query.Set("fee", req.Fee)
	if req.FromAmountForGas != "" && req.FromAmountForGas != "0" {
		query.Set("fromAmountForGas", req.FromAmountForGas)
	}

	out := &QuoteRes{}

	err := c.client.Get(ctx, out, path, query)
	if err != nil {
		return nil, fmt.Errorf("fail to get quote, from=%s, to=%s, err=%w",
			in.FromChain, in.ToChain, err)
	}
	res := c.toStandardQuoteRes(out)
	if res == nil {
		return nil, fmt.Errorf("fail to convert response")
	}
	return res, nil
}

func setProviderFilters(query url.Values, include, exclude string) {
	for _, filter := range []struct {
		list        string
		bridgeKey   string
		exchangeKey string
	}{
		{include, "allowBridges", "allowExchanges"},
		{exclude, "denyBridges", "denyExchanges"},
	} {
		bridges, exchanges := splitProviderTools(filter.list)
		if bridges != "" {
			query.Set(filter.bridgeKey, bridges)
		}
		if exchanges != "" {
			query.Set(filter.exchangeKey, exchanges)
		}
	}
}

func splitProviderTools(list string) (string, string) {
	var bridges, exchanges []string
	for _, name := range strings.Split(list, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := exchangeToolIndex[strings.ToLower(name)]; ok {
			exchanges = append(exchanges, name)
			continue
		}
		bridges = append(bridges, name)
	}
	return strings.Join(bridges, ","), strings.Join(exchanges, ",")
}

func (c *Client) Status(ctx context.Context, in *dexmodel.DexCheckTxIn) (*dexmodel.DexCheckTxOut, error) {
	if in == nil {
		return nil, fmt.Errorf("in is nil")
	} else if in.Hash == "" {
		return nil, fmt.Errorf("hash (txHash) is empty")
	}

	path := "v1/status"

	query := url.Values{}
	query.Set("txHash", in.Hash)
	if in.FromChain != "" {
		fromChainID, ok := chainID(in.FromChain)
		if !ok {
			return nil, fmt.Errorf("invalid fromChain")
		}
		query.Set("fromChain", fromChainID)
	}
	if in.ToChain != "" {
		toChainID, ok := chainID(in.ToChain)
		if !ok {
			return nil, fmt.Errorf("invalid toChain")
		}
		query.Set("toChain", toChainID)
	}
	if in.Bridge != "" {
		query.Set("bridge", in.Bridge)
	}
	out := &StatusRes{}
	err := c.client.Get(ctx, out, path, query)
	if err != nil {
		return nil, fmt.Errorf("fail to get status, txHash=%s, err=%w", in.Hash, err)
	}
	res := c.toStandardStatusRes(out)
	if res == nil {
		return nil, fmt.Errorf("fail to get status, txHash=%s", in.Hash)
	}
	res.ToChain = in.ToChain
	if res.ToChain == "" && out.Receiving.ChainID != 0 {
		res.ToChain = idChainMapper[strconv.Itoa(out.Receiving.ChainID)]
	}
	return res, nil
}

//func (c *Client) GetConnections(ctx context.Context, fromChain, toChain string) (*ConnectionsResponse, error) {
//	params := map[string]string{}
//	if fromChain != "" {
//		params["fromChain"] = fromChain
//	}
//	if toChain != "" {
//		params["toChain"] = toChain
//	}
//
//	res := &ConnectionsResponse{}
//	err := c.client.Get(ctx, res, "connections", params)
//	if err != nil {
//		return nil, fmt.Errorf("fail to get connections, from=%s, to=%s, err=%v",
//			fromChain, toChain, err)
//	}
//	return res, nil
//}
