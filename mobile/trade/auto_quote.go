package trade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kernelflowlabs/wallet-lib/common/dexmodel"
	"github.com/kernelflowlabs/wallet-lib/common/httpc"
	"github.com/kernelflowlabs/wallet-lib/dex"
	"github.com/kernelflowlabs/wallet-lib/dex/bridgers"
	"github.com/kernelflowlabs/wallet-lib/dex/bungee"
	"github.com/kernelflowlabs/wallet-lib/dex/lifi"
)

var (
	bng     *bungee.Client
	brg     *bridgers.Client
	lfiOnce sync.Once
	lfi     *lifi.Client
	proxy   *httpc.Request
)

func autoQuoteBungee() *bungee.Client {
	tradeMu.RLock()
	if bng != nil {
		client := bng
		tradeMu.RUnlock()
		return client
	}
	tradeMu.RUnlock()

	tradeMu.Lock()
	defer tradeMu.Unlock()
	if bng == nil {
		opts := make([]bungee.ClientOption, 0, 2)
		if bungeeAPIKey != "" {
			opts = append(opts, bungee.WithApiKey(bungeeAPIKey))
		}
		if bungeeAffiliateID != "" {
			opts = append(opts, bungee.WithAffiliateId(bungeeAffiliateID))
		}
		bng = bungee.NewClient(opts...)
	}
	return bng
}

func autoQuoteLiFi() *lifi.Client {
	// Device-side keeps the previous free-tier defaults explicitly.
	lfiOnce.Do(func() {
		lfi = lifi.NewClient(
			lifi.WithRateLimit(lifi.RateLimitFree),
			lifi.WithIntegrator("onerootapp"),
		)
	})
	return lfi
}

func autoQuoteBridgers() *bridgers.Client {
	tradeMu.RLock()
	defer tradeMu.RUnlock()
	return brg
}

func serverProxy() *httpc.Request {
	tradeMu.RLock()
	if proxy != nil || server == "" {
		p := proxy
		tradeMu.RUnlock()
		return p
	}
	tradeMu.RUnlock()

	tradeMu.Lock()
	defer tradeMu.Unlock()
	if proxy == nil && server != "" {
		proxy = httpc.NewRequest(server, nil)
	}
	return proxy
}

type autoQuoteReq struct {
	FromAmount              string           `json:"from_amount"`
	SlippageBps             int              `json:"slippage_bps"`
	FromAmountUsd           float64          `json:"from_amount_usd"`
	ToPriceUsd              float64          `json:"to_price_usd"`
	TimeoutMs               int              `json:"timeout_ms"`
	FeeRate                 string           `json:"fee_rate,omitempty"`
	FeeReceiver             string           `json:"fee_receiver,omitempty"`
	UserOps                 []string         `json:"user_ops,omitempty"`
	RefundAddress           string           `json:"refund_address,omitempty"`
	ContractCaller          string           `json:"contract_caller,omitempty"`
	FeeBps                  string           `json:"fee_bps,omitempty"`
	FeeTakerAddress         string           `json:"fee_taker_address,omitempty"`
	Refuel                  *bool            `json:"refuel,omitempty"`
	DestinationPayload      string           `json:"destination_payload,omitempty"`
	DestinationGasLimit     string           `json:"destination_gas_limit,omitempty"`
	IncludeProvider         string           `json:"include_provider,omitempty"`
	ExcludeProvider         string           `json:"exclude_provider,omitempty"`
	Exchange                string           `json:"exchange,omitempty"`
	IncludeQuoteRejections  *bool            `json:"include_quote_rejections,omitempty"`
	Private                 *bool            `json:"private,omitempty"`
	SimulatedQuotesRequired *bool            `json:"simulated_quotes_required,omitempty"`
	SolanaSponsorAddress    string           `json:"solana_sponsor_address,omitempty"`
	EquipmentNo             string           `json:"equipment_no,omitempty"`
	SourceType              string           `json:"source_type,omitempty"`
	FixedRateFlag           *int             `json:"fixed_rate_flag,omitempty"`
	Candidates              []*wireCandidate `json:"candidates"`
}

type wireCandidate struct {
	Channel      string  `json:"channel"`
	Via          string  `json:"via"`
	FromChain    string  `json:"from_chain"`
	FromToken    string  `json:"from_token"`
	FromAddress  string  `json:"from_address"`
	ToChain      string  `json:"to_chain"`
	ToToken      string  `json:"to_token"`
	ToAddress    string  `json:"to_address"`
	ToDecimals   int     `json:"to_decimals"`
	LiquidityUsd float64 `json:"liquidity_usd"`
	SourceIssuer string  `json:"source_issuer,omitempty"`
	FromCoinCode string  `json:"from_coin_code,omitempty"`
	ToCoinCode   string  `json:"to_coin_code,omitempty"`
}

func AutoQuote(reqJSON string) string {
	var req autoQuoteReq
	if err := json.Unmarshal([]byte(reqJSON), &req); err != nil {
		return errRespAutoQuote(fmt.Sprintf("parse request: %v", err))
	}
	cands := make([]*dex.Candidate, 0, len(req.Candidates))
	for _, c := range req.Candidates {
		cands = append(cands, &dex.Candidate{
			Channel:      c.Channel,
			Via:          c.Via,
			FromChain:    c.FromChain,
			FromToken:    c.FromToken,
			FromAddress:  c.FromAddress,
			ToChain:      c.ToChain,
			ToToken:      c.ToToken,
			ToAddress:    c.ToAddress,
			ToDecimals:   c.ToDecimals,
			LiquidityUsd: c.LiquidityUsd,
			SourceIssuer: c.SourceIssuer,
			FromCoinCode: c.FromCoinCode,
			ToCoinCode:   c.ToCoinCode,
		})
	}
	sdkReq := &dex.Request{
		FromAmount:              req.FromAmount,
		SlippageBps:             req.SlippageBps,
		FromAmountUsd:           req.FromAmountUsd,
		ToPriceUsd:              req.ToPriceUsd,
		Timeout:                 time.Duration(req.TimeoutMs) * time.Millisecond,
		FeeRate:                 req.FeeRate,
		FeeReceiver:             req.FeeReceiver,
		UserOps:                 append([]string(nil), req.UserOps...),
		RefundAddress:           req.RefundAddress,
		ContractCaller:          req.ContractCaller,
		FeeBps:                  req.FeeBps,
		FeeTakerAddress:         req.FeeTakerAddress,
		Refuel:                  req.Refuel,
		DestinationPayload:      req.DestinationPayload,
		DestinationGasLimit:     req.DestinationGasLimit,
		IncludeProvider:         req.IncludeProvider,
		ExcludeProvider:         req.ExcludeProvider,
		Exchange:                req.Exchange,
		IncludeQuoteRejections:  req.IncludeQuoteRejections,
		Private:                 req.Private,
		SimulatedQuotesRequired: req.SimulatedQuotesRequired,
		SolanaSponsorAddress:    req.SolanaSponsorAddress,
		EquipmentNo:             req.EquipmentNo,
		SourceType:              req.SourceType,
		FixedRateFlag:           req.FixedRateFlag,
	}
	engine := &dex.Engine{}
	resp := engine.Quote(context.Background(), sdkReq, cands, dispatch)
	b, _ := json.Marshal(resp)
	return string(b)
}

func dispatch(ctx context.Context, cand *dex.Candidate, in *dexmodel.DexQuoteIn) (*dexmodel.DexQuoteOut, error) {
	if cand.Via == dex.ViaServer {
		return quoteViaServer(ctx, in)
	}
	if cand.Via == "" && cand.Channel == "jupiter" && cand.FromChain == "SOL" && cand.ToChain == "SOL" {
		return quoteViaServer(ctx, in)
	}
	switch cand.Channel {
	case "jupiter":
		j := jupClient()
		if j == nil {
			return nil, fmt.Errorf("jupiter not initialized — call Init")
		}
		return j.Quote(ctx, in)
	case "bungee":
		return autoQuoteBungee().Quote(ctx, in)
	case "lifi":
		return autoQuoteLiFi().Quote(ctx, in)
	case "bridgers":
		client := autoQuoteBridgers()
		if client == nil {
			return nil, &dex.ChannelConfigurationError{Channel: "bridgers", Chain: cand.FromChain}
		}
		return client.Quote(ctx, in)
	case "univ2":
		client, release := acquireUniv2Client(cand.FromChain)
		if client == nil {
			return nil, &dex.ChannelConfigurationError{Channel: "univ2", Chain: cand.FromChain}
		}
		defer release()
		return client.Quote(ctx, in)
	case "univ3":
		client, release := acquireUniv3Client(cand.FromChain)
		if client == nil {
			return nil, &dex.ChannelConfigurationError{Channel: "univ3", Chain: cand.FromChain}
		}
		defer release()
		return client.Quote(ctx, in)
	}
	return nil, fmt.Errorf("unknown channel: %q", cand.Channel)
}

func quoteViaServer(ctx context.Context, in *dexmodel.DexQuoteIn) (*dexmodel.DexQuoteOut, error) {
	p := serverProxy()
	if p == nil {
		return nil, fmt.Errorf("via=server but server_url not configured — pass it to Init")
	}
	q := url.Values{}
	q.Set("fromChain", in.FromChain)
	q.Set("toChain", in.ToChain)
	q.Set("fromToken", in.FromToken)
	q.Set("toToken", in.ToToken)
	q.Set("fromAddress", in.FromAddress)
	q.Set("toAddress", in.ToAddress)
	q.Set("fromAmount", in.FromAmount)
	if in.Slippage != "" {
		q.Set("slippage", in.Slippage)
	}
	if in.FromValueUsd != "" {
		q.Set("fromValueUsd", in.FromValueUsd)
	}
	if in.GasOnDestination != "" {
		q.Set("gasOnDestination", in.GasOnDestination)
	}
	if in.FeeRate != "" {
		q.Set("feeRate", in.FeeRate)
	}
	if in.FeeReceiver != "" {
		q.Set("feeReceiver", in.FeeReceiver)
	}
	if len(in.UserOps) > 0 {
		q.Set("userOps", strings.Join(in.UserOps, ","))
	}
	if in.RefundAddress != "" {
		q.Set("refundAddress", in.RefundAddress)
	}
	if in.ContractCaller != "" {
		q.Set("contractCaller", in.ContractCaller)
	}
	if in.FeeBps != "" {
		q.Set("feeBps", in.FeeBps)
	}
	if in.FeeTakerAddress != "" {
		q.Set("feeTakerAddress", in.FeeTakerAddress)
	}
	if in.Refuel != nil {
		q.Set("refuel", strconv.FormatBool(*in.Refuel))
	}
	if in.DestinationPayload != "" {
		q.Set("destinationPayload", in.DestinationPayload)
	}
	if in.DestinationGasLimit != "" {
		q.Set("destinationGasLimit", in.DestinationGasLimit)
	}
	if in.IncludeProvider != "" {
		q.Set("includeProvider", in.IncludeProvider)
	}
	if in.ExcludeProvider != "" {
		q.Set("excludeProvider", in.ExcludeProvider)
	}
	if in.Exchange != "" {
		q.Set("exchange", in.Exchange)
	}
	if in.IncludeQuoteRejections != nil {
		q.Set("includeQuoteRejections", strconv.FormatBool(*in.IncludeQuoteRejections))
	}
	if in.Private != nil {
		q.Set("private", strconv.FormatBool(*in.Private))
	}
	if in.SimulatedQuotesRequired != nil {
		q.Set("simulatedQuotesRequired", strconv.FormatBool(*in.SimulatedQuotesRequired))
	}
	if in.SolanaSponsorAddress != "" {
		q.Set("solanaSponsorAddress", in.SolanaSponsorAddress)
	}
	if in.EquipmentNo != "" {
		q.Set("equipmentNo", in.EquipmentNo)
	}
	if in.SourceType != "" {
		q.Set("sourceType", in.SourceType)
	}
	if in.FromCoinCode != "" {
		q.Set("fromCoinCode", in.FromCoinCode)
	}
	if in.ToCoinCode != "" {
		q.Set("toCoinCode", in.ToCoinCode)
	}
	if in.FixedRateFlag != nil {
		q.Set("fixedRateFlag", strconv.Itoa(*in.FixedRateFlag))
	}
	var env struct {
		Code int                   `json:"code"`
		Msg  string                `json:"msg"`
		Data *dexmodel.DexQuoteOut `json:"data"`
	}
	if err := p.Get(ctx, &env, "public/trade/quote", q); err != nil {
		return nil, fmt.Errorf("proxy quote: %w", err)
	}
	if env.Code != 0 {
		return nil, fmt.Errorf("proxy quote code=%d msg=%s", env.Code, env.Msg)
	}
	return env.Data, nil
}

func errRespAutoQuote(msg string) string {
	b, _ := json.Marshal(&dex.Response{
		Routes:        []*dex.Route{},
		Reason:        "INVALID_REQUEST",
		ReasonMessage: msg,
	})
	return string(b)
}
