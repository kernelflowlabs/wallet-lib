package okx

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"
	"time"

	dexmodel "github.com/kernelflowlabs/wallet-sdk/common/dexmodel"
	"github.com/kernelflowlabs/wallet-sdk/common/httpc"
	"github.com/shopspring/decimal"
)

type Client struct {
	client     *httpc.Request
	apiKey     string
	apiSecret  string
	passphrase string
}

func NewClient(apiKey, apiSecret, passphrase string) *Client {
	client, _ := NewClientWithOptions(apiKey, apiSecret, passphrase)
	return client
}

func NewClientWithOptions(apiKey, apiSecret, passphrase string, options ...httpc.Option) (*Client, error) {
	options = append(options, httpc.WithMaxResponseBytes(8<<20))
	request, err := httpc.NewRequestWithOptions("https://web3.okx.com", map[string]string{"Content-Type": "application/json"}, options...)
	if err != nil {
		return nil, err
	}
	return &Client{client: request, apiKey: apiKey, apiSecret: apiSecret, passphrase: passphrase}, nil
}

func (c *Client) SupportedChains(ctx context.Context) ([]string, error) {
	path := "api/v6/dex/aggregator/supported/chain"
	query := url.Values{}

	out := &SupportedChainsRes{}
	err := c.signedGet(ctx, out, path, query)
	if err != nil {
		return nil, fmt.Errorf("fail to get supported chains, err=%v", err)
	} else if !out.IsSuccess() {
		return nil, fmt.Errorf("fail to get supported chains, code=%s, msg=%s", out.Code, out.Msg)
	}

	var res []string
	for _, v := range out.Data {
		if isEVMChainID(v.ChainIndex) {
			res = append(res, v.ChainName)
		}
	}
	return res, nil
}

func (c *Client) SupportedCrossChains(ctx context.Context) ([]string, error) {
	out := &CrossBridgesResponse{}
	if err := c.signedGet(ctx, out, "api/v6/dex/cross-chain/supported/bridges", url.Values{}); err != nil {
		return nil, err
	}
	if !out.IsSuccess() {
		return nil, fmt.Errorf("get cross-chain bridges: code=%s", out.Code)
	}
	seen := make(map[string]bool)
	for _, bridge := range out.Data {
		for _, id := range bridge.SupportedChains {
			if chain, ok := idChainMapper[id]; ok && isEVMChainID(id) {
				seen[chain] = true
			}
		}
	}
	result := make([]string, 0, len(seen))
	for chain := range seen {
		result = append(result, chain)
	}
	sort.Strings(result)
	return result, nil
}

func (c *Client) PreSingleChainQuote(ctx context.Context, in *dexmodel.DexQuoteIn) (*dexmodel.DexQuoteOut, error) {
	if in == nil {
		return nil, fmt.Errorf("quote request is nil")
	}
	copy := *in
	copy.FromChain = strings.ToUpper(strings.TrimSpace(in.FromChain))
	if !isEVMChain(copy.FromChain) {
		return nil, fmt.Errorf("unsupported EVM chain: %s", copy.FromChain)
	}
	in = &copy
	req := c.toOkxSingleChainQuoteReq(in)
	if req == nil {
		return nil, fmt.Errorf("fail to convert request to QuoteReq")
	}

	path := "api/v6/dex/aggregator/quote"
	query := url.Values{}
	query.Set("chainIndex", req.ChainIndex)
	query.Set("fromTokenAddress", req.FromToken)
	query.Set("toTokenAddress", req.ToToken)
	query.Set("amount", req.Amount)
	query.Set("slippagePercent", req.Slippage)

	out := &SingleChainQuoteRes{}
	err := c.signedGet(ctx, out, path, query)
	if err != nil {
		return nil, fmt.Errorf("fail to quote, err=%v", err)
	} else if !out.IsSuccess() {
		return nil, fmt.Errorf("fail to quote, code=%v, msg=%s", out.Code, out.Msg)
	}
	res := c.toStandardQuoteResPre(out, in)
	if res == nil {
		return nil, fmt.Errorf("fail to convert response")
	}
	return res, nil
}

func (c *Client) Quote(ctx context.Context, in *dexmodel.DexQuoteIn) (*dexmodel.DexQuoteOut, error) {
	if in == nil {
		return nil, fmt.Errorf("quote request is nil")
	}
	copy := *in
	copy.FromChain, copy.ToChain = strings.ToUpper(strings.TrimSpace(in.FromChain)), strings.ToUpper(strings.TrimSpace(in.ToChain))
	if !isEVMChain(copy.FromChain) || !isEVMChain(copy.ToChain) {
		return nil, fmt.Errorf("OKX quotes support EVM chains only")
	}
	if copy.ContractCaller != "" {
		copy.FromAddress = copy.ContractCaller
	}
	in = &copy
	if in.FromChain == in.ToChain {
		return c.singleChainQuote(ctx, in)
	}
	return c.crossChainQuote(ctx, in)
}

func (c *Client) singleChainQuote(ctx context.Context, in *dexmodel.DexQuoteIn) (*dexmodel.DexQuoteOut, error) {
	req := c.toOkxSingleChainQuoteReq(in)
	if req == nil {
		return nil, fmt.Errorf("fail to convert request to QuoteReq")
	}

	path := "api/v6/dex/aggregator/swap"
	query := url.Values{}
	query.Set("chainIndex", req.ChainIndex)
	query.Set("fromTokenAddress", req.FromToken)
	query.Set("toTokenAddress", req.ToToken)
	query.Set("amount", req.Amount)
	query.Set("slippagePercent", req.Slippage)
	query.Set("userWalletAddress", req.UserAddress)
	if in.ToAddress != "" {
		query.Set("swapReceiverAddress", in.ToAddress)
	}
	query.Set("approveTransaction", "true")
	query.Set("approveAmount", req.Amount)
	query.Set("singleRouteOnly", "true")
	//query.Set("fromTokenReferrerWalletAddress", req.ReferrerAddress)
	query.Set("toTokenReferrerWalletAddress", req.ReferrerAddress)
	query.Set("feePercent", req.FeePercent)

	out := &SingleChainSwapRes{}
	err := c.signedGet(ctx, out, path, query)
	if err != nil {
		return nil, fmt.Errorf("fail to swap, err=%v", err)
	} else if !out.IsSuccess() {
		return nil, fmt.Errorf("fail to swap, code=%s, msg=%s", out.Code, out.Msg)
	}

	data, err := c.parseSwapResData(out.Data)
	if err != nil || len(data) == 0 {
		return nil, fmt.Errorf("fail to swap, no data")
	}
	res := c.toStandardQuoteResFromSwapResData(data[0], in)
	if res == nil {
		return nil, fmt.Errorf("fail to convert response")
	}
	return res, nil
}

func (c *Client) crossChainQuote(ctx context.Context, in *dexmodel.DexQuoteIn) (*dexmodel.DexQuoteOut, error) {
	req := c.toOkxCrossChainQuoteReq(in)
	if req == nil {
		return nil, fmt.Errorf("fail to convert request to QuoteReq")
	}

	path := "api/v6/dex/cross-chain/swap"
	query := url.Values{}
	query.Set("fromChainIndex", req.FromChainIndex)
	query.Set("toChainIndex", req.ToChainIndex)
	query.Set("fromTokenAddress", req.FromToken)
	query.Set("toTokenAddress", req.ToToken)
	query.Set("userWalletAddress", req.UserAddress)
	query.Set("receiveAddress", req.ReceiverAddress)
	query.Set("amount", req.Amount)
	slippage, err := decimal.NewFromString(req.Slippage)
	if err != nil || !slippage.GreaterThan(decimal.Zero) || !slippage.LessThan(decimal.NewFromInt(100)) {
		return nil, fmt.Errorf("invalid slippage percent")
	}
	query.Set("slippage", slippage.Div(decimal.NewFromInt(100)).String())
	query.Set("referrerAddress", req.ReferrerAddress)
	query.Set("feePercent", req.FeePercent)

	out := &CrossChainQuoteRes{}
	err = c.signedGet(ctx, out, path, query)
	if err != nil {
		return nil, fmt.Errorf("fail to swap, err=%v", err)
	} else if !out.IsSuccess() {
		return nil, fmt.Errorf("fail to swap, code=%s, msg=%s", out.Code, out.Msg)
	}

	data, err := c.parseCrossResData(out.Data)
	if err != nil || len(data) == 0 {
		return nil, fmt.Errorf("fail to swap, no data")
	}
	if !strings.EqualFold(in.FromToken, "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee") {
		approval, err := c.crossChainApproval(ctx, in, data[0].Router.BridgeId)
		if err != nil {
			return nil, err
		}
		data[0].Tx.SignatureData = []string{approval}
	}
	res := c.toStandardQuoteResFromCrossResData(data[0], in)
	if res == nil {
		return nil, fmt.Errorf("fail to convert response")
	}
	return res, nil
}

func (c *Client) Status(ctx context.Context, in *dexmodel.DexCheckTxIn) (*dexmodel.DexCheckTxOut, error) {
	if in == nil {
		return nil, fmt.Errorf("status request is nil")
	}
	copy := *in
	copy.FromChain, copy.ToChain = strings.ToUpper(strings.TrimSpace(in.FromChain)), strings.ToUpper(strings.TrimSpace(in.ToChain))
	if !isEVMChain(copy.FromChain) || copy.ToChain != "" && !isEVMChain(copy.ToChain) {
		return nil, fmt.Errorf("OKX status supports EVM chains only")
	}
	in = &copy
	if in.FromChain != in.ToChain && in.ToChain != "" {
		return c.crossChainStatus(ctx, in)
	}
	chainId, ok := idChainMapperReverse[in.FromChain]
	if !ok {
		log.Printf("not found chain id %s", in.FromChain)
		return nil, fmt.Errorf("unsupported chain")
	}

	path := "api/v6/dex/aggregator/history"
	query := url.Values{}
	query.Set("chainIndex", chainId)
	query.Set("txHash", in.Hash)
	//query.Set("isFromMyProject", "true")

	out := &HistoryRes{}
	err := c.signedGet(ctx, out, path, query)
	if err != nil {
		return nil, fmt.Errorf("fail to get status, txHash=%s, err=%v", in.Hash, err)
	} else if !out.IsSuccess() {
		return nil, fmt.Errorf("fail to get status, code=%s, msg=%s", out.Code, out.Msg)
	}

	res := c.toStandardStatusRes(out)
	if res == nil {
		return nil, fmt.Errorf("fail to convert response")
	}
	res.Channel = in.Channel
	res.ToChain = in.ToChain
	return res, nil
}

func (c *Client) signedGet(ctx context.Context, res interface{}, path string, query url.Values) error {
	query = cloneQuery(query)
	for _, key := range []string{"feePercent", "referrerAddress", "toTokenReferrerWalletAddress"} {
		if query.Get(key) == "" || key == "feePercent" && query.Get(key) == "0" {
			query.Del(key)
		}
	}
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	requestPath := path
	if len(query) > 0 {
		requestPath = path + "?" + query.Encode()
	}
	sign := c.hmacSign(timestamp + "GET" + "/" + requestPath)

	headers := map[string]string{
		"OK-ACCESS-KEY":        c.apiKey,
		"OK-ACCESS-SIGN":       sign,
		"OK-ACCESS-PASSPHRASE": c.passphrase,
		"OK-ACCESS-TIMESTAMP":  timestamp,
	}
	return c.client.GetWithHeaders(ctx, res, path, query, headers)
}

func (c *Client) hmacSign(message string) string {
	mac := hmac.New(sha256.New, []byte(c.apiSecret))
	mac.Write([]byte(message))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
