package jupiter

import (
	dexmodel "github.com/kernelflowlabs/wallet-lib/common/dexmodel"
	"github.com/kernelflowlabs/wallet-lib/common/httpc"
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultJupiterBase     = "https://api.jup.ag" //"https://lite-api.jup.ag"
	defaultJupiterDataBase = "https://datapi.jup.ag"
	defaultJupiterUltra    = "https://lite-api.jup.ag" // Ultra API host
	defaultSolanaRPC       = "https://api.mainnet-beta.solana.com"
)

type Client struct {
	client     *httpc.Request
	data       *httpc.Request
	ultra      *httpc.Request
	rpc        *httpc.Request
	feeAccount string
}

// NewClient constructs a Jupiter client with the free-tier rate limit
// as the default. Pass WithRateLimit(jupiter.RateLimitPro) if you're
// using a paid api-key that raises the ceiling. Additional options
// (WithRPC, WithFeeAccount) can be layered on for the fee-payer /
// gasless path.
func NewClient(token string, opts ...ClientOption) *Client {
	return NewClientWithOptions("", "", token, opts...)
}

// NewClientWithOptions is the legacy 3-positional signature; kept
// backward-compatible because existing callers pass rpcURL /
// feeAccount as fixed strings. Prefer NewClient + WithRPC /
// WithFeeAccount for new code.
func NewClientWithOptions(rpcURL, feeAccount, token string, opts ...ClientOption) *Client {
	cfg := clientConfig{
		rpcURL:     rpcURL,
		feeAccount: feeAccount,
		rateLimit:  RateLimitFree,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.rpcURL == "" {
		cfg.rpcURL = defaultSolanaRPC
	}
	if cfg.swapBase == "" {
		cfg.swapBase = defaultJupiterBase
	}
	if cfg.dataBase == "" {
		cfg.dataBase = defaultJupiterDataBase
	}
	if cfg.ultraBase == "" {
		cfg.ultraBase = defaultJupiterUltra
	}
	headers := map[string]string{"Content-Type": "application/json"}
	if token != "" {
		headers["x-api-key"] = token
	}
	dataHeaders := map[string]string{
		"Content-Type": "application/json",
		"User-Agent":   "Mozilla/5.0 (compatible; wallet-sdk/1.0)",
	}
	for k, v := range headers {
		dataHeaders[k] = v
	}
	swapReq := httpc.NewRequest(cfg.swapBase, headers)
	dataReq := httpc.NewRequest(cfg.dataBase, dataHeaders)
	ultraReq := httpc.NewRequest(cfg.ultraBase, headers)
	// Same policy on all three subdomains — Jupiter's swap /
	// datapi / lite-api gateways each maintain their own token
	// bucket, so applying the same cap to each doesn't triple-
	// count against a single documented tier.
	cfg.rateLimit.Apply(swapReq)
	cfg.rateLimit.Apply(dataReq)
	cfg.rateLimit.Apply(ultraReq)
	return &Client{
		client:     swapReq,
		data:       dataReq,
		ultra:      ultraReq,
		rpc:        httpc.NewRequest(cfg.rpcURL, map[string]string{"Content-Type": "application/json"}),
		feeAccount: cfg.feeAccount,
	}
}

// UltraOrder fetches a quote from Ultra. Caller inspects res.Router to
// decide the execution path: "jupiterz" means res.Transaction is ready to
// be signed and submitted via UltraExecute; "metis" means we still need
// to assemble the tx ourselves (via swap-instructions) with our own
// fee payer.
func (c *Client) UltraOrder(ctx context.Context, req *UltraOrderReq) (*UltraOrderRes, error) {
	if req == nil || req.InputMint == "" || req.OutputMint == "" || req.Amount == "" || req.Taker == "" {
		return nil, fmt.Errorf("invalid ultra order request")
	}
	q := url.Values{}
	q.Set("inputMint", req.InputMint)
	q.Set("outputMint", req.OutputMint)
	q.Set("amount", req.Amount)
	q.Set("taker", req.Taker)
	if req.SlippageBps > 0 {
		q.Set("slippageBps", strconv.Itoa(req.SlippageBps))
	}
	out := &UltraOrderRes{}
	if err := c.ultra.Get(ctx, out, "ultra/v1/order", q); err != nil {
		return nil, fmt.Errorf("ultra order: %w", err)
	}
	if out.ErrorCode != "" || out.ErrorMessage != "" {
		return nil, fmt.Errorf("ultra order: %s %s", out.ErrorCode, out.ErrorMessage)
	}
	return out, nil
}

// QuoteUltra wraps UltraOrder + the gasless conversion so TradeService
// only sees DexQuoteOut. Returns (nil, nil) when Ultra succeeds but the
// route isn't jupiterz — caller treats that as "fall back to regular".
func (c *Client) QuoteUltra(ctx context.Context, in *dexmodel.DexQuoteIn) (*dexmodel.DexQuoteOut, error) {
	if in == nil {
		return nil, fmt.Errorf("nil quote input")
	}
	// Codebase convention: in.Slippage is percentage ("0.5" == 0.5%),
	// not decimal — matches what the other dex clients (bungee/lifi/okx)
	// expect. 0.5 percent → 50 bps.
	slippageBps := 0
	if in.Slippage != "" {
		if f, err := strconv.ParseFloat(in.Slippage, 64); err == nil {
			slippageBps = int(f * 100)
		}
	}
	ord, err := c.UltraOrder(ctx, &UltraOrderReq{
		InputMint:   in.FromToken,
		OutputMint:  in.ToToken,
		Amount:      in.FromAmount,
		Taker:       in.FromAddress,
		SlippageBps: slippageBps,
	})
	if err != nil {
		return nil, err
	}
	return c.ultraToStandardQuoteRes(ord), nil
}

// QuoteRaw exposes the unaggregated /swap/v1/quote response and the input
// QuoteReq so TradeService's server-fee-payer path can feed it into
// SwapInstructions without going through the full Quote()+buildSwap()
// pipeline (which would build a tx with the user as fee payer).
func (c *Client) QuoteRaw(ctx context.Context, in *dexmodel.DexQuoteIn) (*QuoteRes, *QuoteReq, error) {
	req := c.toJupiterQuoteReq(in)
	if req == nil {
		return nil, nil, fmt.Errorf("invalid quote input")
	}
	quote, err := c.fetchQuote(ctx, req)
	if err != nil {
		return nil, nil, err
	}
	return quote, req, nil
}

// SwapInstructions fetches the raw instruction list for a previously-built
// quote. Caller assembles its own v0 versioned tx — typically with a
// custom fee payer — instead of accepting Jupiter's default tx where the
// user is fee payer.
func (c *Client) SwapInstructions(ctx context.Context, req *SwapInstructionsReq) (*SwapInstructionsRes, error) {
	if req == nil || req.UserPublicKey == "" || req.QuoteResponse == nil {
		return nil, fmt.Errorf("invalid swap-instructions request")
	}
	out := &SwapInstructionsRes{}
	if err := c.client.Post(ctx, out, "swap/v1/swap-instructions", req); err != nil {
		return nil, fmt.Errorf("swap-instructions: %w", err)
	}
	if out.ErrorCode != "" || out.ErrorMessage != "" {
		return nil, fmt.Errorf("swap-instructions: %s %s", out.ErrorCode, out.ErrorMessage)
	}
	if out.SwapInstruction == nil {
		return nil, fmt.Errorf("swap-instructions: no swap instruction returned")
	}
	return out, nil
}

// UltraExecute submits a signed tx through Ultra's relayer. Only valid
// after UltraOrder returned router="jupiterz"; for metis routes we go
// through Solana RPC directly.
func (c *Client) UltraExecute(ctx context.Context, req *UltraExecuteReq) (*UltraExecuteRes, error) {
	if req == nil || req.SignedTransaction == "" || req.RequestID == "" {
		return nil, fmt.Errorf("invalid ultra execute request")
	}
	out := &UltraExecuteRes{}
	if err := c.ultra.Post(ctx, out, "ultra/v1/execute", req); err != nil {
		return nil, fmt.Errorf("ultra execute: %w", err)
	}
	if out.Error != "" {
		return nil, fmt.Errorf("ultra execute: %s (code=%d)", out.Error, out.Code)
	}
	return out, nil
}

// https://datapi.jup.ag/v2/charts/{mint}?interval=&to=&candles=&type=&quote=usd
// `to` is required upstream; if not set we use now. quote is fixed to usd
// (the only value the upstream accepts).
func (c *Client) Charts(ctx context.Context, req *ChartReq) (*ChartsRes, error) {
	if req == nil || req.Mint == "" {
		return nil, fmt.Errorf("mint is empty")
	}
	interval := req.Interval
	if interval == "" {
		interval = "1_HOUR"
	}
	candles := req.Candles
	if candles <= 0 {
		candles = 100
	}
	chartType := req.Type
	if chartType == "" {
		chartType = "price"
	}
	toMs := req.ToMs
	if toMs <= 0 {
		toMs = time.Now().UnixMilli()
	}

	path := "v2/charts/" + req.Mint
	query := url.Values{}
	query.Set("interval", interval)
	query.Set("to", strconv.FormatInt(toMs, 10))
	query.Set("candles", strconv.Itoa(candles))
	query.Set("type", chartType)
	query.Set("quote", "usd")

	out := &ChartsRes{}
	if err := c.data.Get(ctx, out, path, query); err != nil {
		return nil, fmt.Errorf("fail to get charts, mint=%s, err=%v", req.Mint, err)
	}
	return out, nil
}

// https://datapi.jup.ag/v1/holders/{mint}
// Returns top 100 holders sorted by amount desc, plus PnL data for holders
// that have on-chain trade history. limit/offset query params are ignored
// upstream so we don't expose them.
func (c *Client) Holders(ctx context.Context, mint string) (*HoldersRes, error) {
	if mint == "" {
		return nil, fmt.Errorf("mint is empty")
	}
	path := "v1/holders/" + mint
	out := &HoldersRes{}
	if err := c.data.Get(ctx, out, path, nil); err != nil {
		return nil, fmt.Errorf("fail to get holders, mint=%s, err=%v", mint, err)
	}
	return out, nil
}

// https://datapi.jup.ag/v2/assets/stocks/24h?sortBy=volume&sortDir=desc&offset=0&includeOndoStatus=true
func (c *Client) Stocks24h(ctx context.Context, limit, offset int) (*StocksRes, error) {
	path := "v2/assets/stocks/24h"
	query := url.Values{}
	query.Set("sortBy", "volume")
	query.Set("sortDir", "desc")
	query.Set("includeOndoStatus", "true")
	query.Set("limit", strconv.Itoa(limit))
	query.Set("offset", strconv.Itoa(offset))

	out := &StocksRes{}
	if err := c.data.Get(ctx, out, path, query); err != nil {
		return nil, fmt.Errorf("fail to get stocks 24h, err=%v", err)
	}
	return out, nil
}

func (c *Client) Quote(ctx context.Context, in *dexmodel.DexQuoteIn) (*dexmodel.DexQuoteOut, error) {
	req := c.toJupiterQuoteReq(in)
	if req == nil {
		return nil, fmt.Errorf("fail to convert request to QuoteReq")
	}

	quote, err := c.fetchQuoteRetry(ctx, req)
	if err != nil {
		return nil, err
	}

	swap, err := c.buildSwapRetry(ctx, quote, req)
	if err != nil {
		return nil, err
	}

	res := c.toStandardQuoteRes(quote, swap)
	if res == nil {
		return nil, fmt.Errorf("fail to convert response")
	}
	return res, nil
}

func isTransientJupiterErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "TOKEN_NOT_TRADABLE") ||
		strings.Contains(s, "MARKET_NOT_FOUND")
}

// isJupiterRateLimited spots the 429 pattern separately so callers can
// back off longer than for a normal transient — 200ms after a 429 just
// slams the same gateway limiter again.
func isJupiterRateLimited(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "status=429") ||
		strings.Contains(s, "\"code\":429") ||
		strings.Contains(s, "Too many requests")
}

// fetchQuoteRetry / buildSwapRetry: at most 3 attempts total. Backoff
// is 200ms for normal transients (TOKEN_NOT_TRADABLE / MARKET_NOT_FOUND
// resolve on the next upstream tick) and grows exponentially for 429
// so we don't keep hammering the same rate-limit bucket. Ctx cancellation
// aborts immediately.
func (c *Client) fetchQuoteRetry(ctx context.Context, req *QuoteReq) (*QuoteRes, error) {
	var lastErr error
	for i := 0; i < 3; i++ {
		if i > 0 {
			backoff := 200 * time.Millisecond
			if isJupiterRateLimited(lastErr) {
				backoff = time.Duration(1<<i) * time.Second // 2s, 4s
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
		out, err := c.fetchQuote(ctx, req)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if !isTransientJupiterErr(err) && !isJupiterRateLimited(err) {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *Client) buildSwapRetry(ctx context.Context, quote *QuoteRes, req *QuoteReq) (*SwapRes, error) {
	var lastErr error
	for i := 0; i < 3; i++ {
		if i > 0 {
			backoff := 200 * time.Millisecond
			if isJupiterRateLimited(lastErr) {
				backoff = time.Duration(1<<i) * time.Second
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
		out, err := c.buildSwap(ctx, quote, req)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if !isTransientJupiterErr(err) && !isJupiterRateLimited(err) {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *Client) Status(ctx context.Context, in *dexmodel.DexCheckTxIn) (*dexmodel.DexCheckTxOut, error) {
	if in == nil || in.Hash == "" {
		return nil, fmt.Errorf("invalid request")
	}

	body := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getSignatureStatuses",
		"params": []interface{}{
			[]string{in.Hash},
			map[string]interface{}{"searchTransactionHistory": true},
		},
	}

	out := &SignatureStatusRes{}
	if err := c.rpc.Post(ctx, out, "", body); err != nil {
		return nil, fmt.Errorf("fail to get status, txHash=%s, err=%w", in.Hash, err)
	} else if out.Error != nil {
		return nil, fmt.Errorf("fail to get status, code=%d, msg=%s", out.Error.Code, out.Error.Message)
	}
	return c.toStandardStatusRes(out, in), nil
}

func (c *Client) fetchQuote(ctx context.Context, req *QuoteReq) (*QuoteRes, error) {
	path := "swap/v1/quote"
	query := url.Values{}
	query.Set("inputMint", req.InputMint)
	query.Set("outputMint", req.OutputMint)
	query.Set("amount", req.Amount)
	query.Set("slippageBps", req.SlippageBps)
	query.Set("swapMode", "ExactIn")
	query.Set("restrictIntermediateTokens", "true")
	if c.feeAccount != "" && req.PlatformFeeBps != "" {
		query.Set("platformFeeBps", req.PlatformFeeBps)
	}

	out := &QuoteRes{}
	err := c.client.Get(ctx, out, path, query)
	if err != nil {
		return nil, fmt.Errorf("fail to get quote, err=%w", err)
	} else if out.ErrorCode != "" || out.ErrorMsg != "" {
		return nil, fmt.Errorf("fail to get quote, code=%s, msg=%s", out.ErrorCode, out.ErrorMsg)
	}
	return out, nil
}

func (c *Client) buildSwap(ctx context.Context, quote *QuoteRes, req *QuoteReq) (*SwapRes, error) {
	path := "swap/v1/swap"
	body := &SwapReq{
		QuoteResponse:           quote,
		UserPublicKey:           req.UserPublicKey,
		WrapAndUnwrapSol:        true,
		DynamicComputeUnitLimit: true,
	}
	if c.feeAccount != "" && req.PlatformFeeBps != "" {
		body.FeeAccount = c.feeAccount
	}

	out := &SwapRes{}
	err := c.client.Post(ctx, out, path, body)
	if err != nil {
		return nil, fmt.Errorf("fail to build swap, err=%v", err)
	} else if out.ErrorCode != "" || out.ErrorMsg != "" {
		return nil, fmt.Errorf("fail to build swap, code=%s, msg=%s", out.ErrorCode, out.ErrorMsg)
	}
	return out, nil
}
