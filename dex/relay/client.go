package relay

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"

	dexmodel "github.com/kernelflowlabs/wallet-sdk/common/dexmodel"
	"github.com/kernelflowlabs/wallet-sdk/common/httpc"
)

type Client struct {
	client   *httpc.Request
	referrer string
}

func NewClient(opts ...ClientOption) *Client {
	cfg := clientConfig{rateLimit: RateLimitPublic}
	for _, opt := range opts {
		opt(&cfg)
	}
	c := httpc.NewRequest("https://api.relay.link",
		map[string]string{"Content-Type": "application/json"})
	referrer := ""
	if cfg.apiKey != "" {
		c.SetHeader("x-api-key", cfg.apiKey)
		referrer = cfg.referrer
	}
	cfg.rateLimit.Apply(c)
	return &Client{client: c, referrer: referrer}
}

func (c *Client) Chains(ctx context.Context) ([]string, error) {
	path := "chains"

	out := &ChainsRes{}
	err := c.client.Get(ctx, out, path, url.Values{})
	if err != nil {
		return nil, fmt.Errorf("fail to get chains, err=%v", err)
	}

	var res []string
	for _, v := range out.Chains {
		if v.Disabled {
			continue
		}
		if chain, ok := idChainMapper[strconv.Itoa(v.ID)]; ok {
			res = append(res, chain)
		}
	}
	sort.Strings(res)
	return res, nil
}

func (c *Client) Quote(ctx context.Context, in *dexmodel.DexQuoteIn) (*dexmodel.DexQuoteOut, error) {
	req, err := c.toRelayQuoteReq(in)
	if err != nil {
		return nil, fmt.Errorf("fail to convert request to QuoteReq: %w", err)
	}

	path := "quote"

	out := &QuoteRes{}
	err = c.client.Post(ctx, out, path, req)
	if err != nil {
		return nil, fmt.Errorf("fail to get quote, from=%s, to=%s, err=%w",
			in.FromChain, in.ToChain, err)
	} else if out.RequestId == "" {
		return nil, fmt.Errorf("fail to get quote, from=%s, to=%s, code=%s, msg=%s",
			in.FromChain, in.ToChain, out.ErrorCode, out.Message)
	}
	res := c.toStandardQuoteRes(out)
	if res == nil {
		return nil, fmt.Errorf("fail to convert response")
	}
	return res, nil
}

func (c *Client) Status(ctx context.Context, in *dexmodel.DexCheckTxIn) (*dexmodel.DexCheckTxOut, error) {
	if in == nil {
		return nil, fmt.Errorf("in is nil")
	} else if in.Hash == "" {
		return nil, fmt.Errorf("hash (requestId) is empty")
	}

	path := "intents/status/v3"

	query := url.Values{}
	query.Set("requestId", in.Hash)

	out := &StatusRes{}
	err := c.client.Get(ctx, out, path, query)
	if err != nil {
		return nil, fmt.Errorf("fail to get status, requestId=%s, err=%w", in.Hash, err)
	}
	res := c.toStandardStatusRes(out)
	if res == nil {
		return nil, fmt.Errorf("fail to get status, requestId=%s", in.Hash)
	}
	if res.ToChain == "" {
		res.ToChain = in.ToChain
	}
	return res, nil
}
