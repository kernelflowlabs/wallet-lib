package oneinch

import (
	"context"
	"fmt"
	"github.com/kernelflowlabs/wallet-lib/common/httpc"
	"strconv"
)

const defaultBase = "https://api.1inch.dev"

// Client wraps 1inch Fusion endpoints. Same instance covers all chains; the
// chainId is path-segment so the underlying httpc.Request can be reused.
type Client struct {
	req    *httpc.Request
	apiKey string
}

func NewClient(apiKey string) *Client {
	headers := map[string]string{
		"Accept":        "application/json",
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + apiKey,
	}
	return &Client{
		req:    httpc.NewRequest(defaultBase, headers),
		apiKey: apiKey,
	}
}

func (c *Client) Configured() bool { return c.apiKey != "" }

// ChainID converts our internal chain name (ETH/BASE/BNB) into 1inch's
// numeric chainId. Returns 0 + ok=false for unsupported chains so the
// orchestrator can degrade cleanly.
func ChainID(chainName string) (int, bool) {
	v, ok := SupportedChains[chainName]
	return v, ok
}

// QuoteFusion calls /fusion/quoter/v2.0/{chainId}/quote/receive.
// Returns the multi-preset quote; caller picks which preset (fast / medium
// / slow) to construct the order from.
func (c *Client) QuoteFusion(ctx context.Context, chainID int, req *FusionQuoteReq) (*FusionQuoteRes, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("1inch api key not configured")
	}
	if req == nil {
		return nil, fmt.Errorf("nil quote request")
	}
	out := &FusionQuoteRes{}
	path := "fusion/quoter/v2.0/" + strconv.Itoa(chainID) + "/quote/receive"
	if err := c.req.Post(ctx, out, path, req); err != nil {
		return nil, fmt.Errorf("fusion quote: %w", err)
	}
	if out.Error != "" {
		return nil, fmt.Errorf("fusion quote: %s — %s", out.Error, out.Message)
	}
	return out, nil
}

// SubmitOrder pushes a user-signed order to the resolver network. The
// returned orderHash is what the frontend polls via Status().
func (c *Client) SubmitOrder(ctx context.Context, chainID int, req *FusionOrderSubmitReq) (*FusionOrderSubmitRes, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("1inch api key not configured")
	}
	if req == nil || req.Signature == "" {
		return nil, fmt.Errorf("invalid submit request")
	}
	out := &FusionOrderSubmitRes{}
	path := "fusion/relayer/v2.0/" + strconv.Itoa(chainID) + "/order/submit"
	if err := c.req.Post(ctx, out, path, req); err != nil {
		return nil, fmt.Errorf("fusion submit: %w", err)
	}
	if out.Error != "" {
		return nil, fmt.Errorf("fusion submit: %s", out.Error)
	}
	return out, nil
}

// Status polls /fusion/orders/v2.0/{chainId}/order/status/{orderHash}.
// Frontend hits this through /trade/checkTx every few seconds until the
// status moves out of pending.
func (c *Client) Status(ctx context.Context, chainID int, orderHash string) (*FusionOrderStatusRes, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("1inch api key not configured")
	}
	if orderHash == "" {
		return nil, fmt.Errorf("empty order hash")
	}
	out := &FusionOrderStatusRes{}
	path := "fusion/orders/v2.0/" + strconv.Itoa(chainID) + "/order/status/" + orderHash
	if err := c.req.Get(ctx, out, path, nil); err != nil {
		return nil, fmt.Errorf("fusion status: %w", err)
	}
	if out.Error != "" {
		return nil, fmt.Errorf("fusion status: %s", out.Error)
	}
	return out, nil
}
