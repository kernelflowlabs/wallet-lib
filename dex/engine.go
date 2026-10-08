package dex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kernelflowlabs/wallet-sdk/common/dexmodel"
	"github.com/kernelflowlabs/wallet-sdk/common/httpc"
)

const DefaultTimeout = 5 * time.Second
const ThinLiquidityRatio = 0.05

const (
	WarnThinLiquidity   = "THIN_LIQUIDITY"
	WarnPriceImpactHigh = "PRICE_IMPACT_HIGH"
)

type Candidate struct {
	Channel      string
	Via          string
	FromChain    string
	FromToken    string
	FromAddress  string
	ToChain      string
	ToToken      string
	ToAddress    string
	ToDecimals   int
	LiquidityUsd float64
	SourceIssuer string
	FromCoinCode string
	ToCoinCode   string
}

const (
	ViaDevice = "device"
	ViaServer = "server"
)

func (c *Candidate) CrossChain() bool { return c.FromChain != c.ToChain }

type Request struct {
	FromAmount              string
	SlippageBps             int
	Timeout                 time.Duration
	FromAmountUsd           float64
	ToPriceUsd              float64
	FeeRate                 string
	FeeReceiver             string
	UserOps                 []string
	RefundAddress           string
	ContractCaller          string
	FeeBps                  string
	FeeTakerAddress         string
	Refuel                  *bool
	DestinationPayload      string
	DestinationGasLimit     string
	IncludeProvider         string
	ExcludeProvider         string
	Exchange                string
	IncludeQuoteRejections  *bool
	Private                 *bool
	SimulatedQuotesRequired *bool
	SolanaSponsorAddress    string
	EquipmentNo             string
	SourceType              string
	FixedRateFlag           *int
	ScamCheck               func(chain, token string) bool
}

type Route struct {
	Rank            int                   `json:"rank"`
	Recommended     bool                  `json:"recommended"`
	CrossChain      bool                  `json:"crossChain"`
	FromChain       string                `json:"fromChain"`
	FromToken       string                `json:"fromToken"`
	ToChain         string                `json:"toChain"`
	ToToken         string                `json:"toToken"`
	ToDecimals      int                   `json:"toDecimals"`
	SourceIssuer    string                `json:"sourceIssuer,omitempty"`
	ToAmount        string                `json:"toAmount"`
	ToAmountUsd     float64               `json:"toAmountUsd"`
	ExpectedFillUsd float64               `json:"expectedFillUsd,omitempty"`
	Channel         string                `json:"channel"`
	RouteId         string                `json:"routeId,omitempty"`
	AmountOutMin    string                `json:"amountOutMin,omitempty"`
	NeedBuild       bool                  `json:"needBuild"`
	TxData          *dexmodel.DexTx       `json:"txData,omitempty"`
	ApprovalData    *dexmodel.DexApproval `json:"approvalData,omitempty"`
	TrustedSpenders []string              `json:"trustedSpenders,omitempty"`
	FeeUsd          float64               `json:"feeUsd,omitempty"`
	PriceImpactPct  float64               `json:"priceImpactPct,omitempty"`
	LiquidityUsd    float64               `json:"liquidityUsd,omitempty"`
	EstSeconds      int64                 `json:"estSeconds,omitempty"`
	RouteTags       []string              `json:"routeTags,omitempty"`
	ExpiresAt       int64                 `json:"expiresAt,omitempty"`
	SubmitRequest   json.RawMessage       `json:"submitRequest,omitempty"`
	Warnings        []Warning             `json:"warnings,omitempty"`
	Reason          string                `json:"reason,omitempty"`
}

type Warning = dexmodel.DexWarning

const (
	FailureRateLimited = "RATE_LIMITED"
	FailureTimeout     = "TIMEOUT"
	FailureProvider    = "PROVIDER_ERROR"
	FailureNoRoute     = "NO_ROUTE"
	FailureConfig      = "CONFIGURATION_ERROR"
)

type ChannelConfigurationError struct {
	Channel string
	Chain   string
}

func (e *ChannelConfigurationError) Error() string {
	if e == nil {
		return "quote channel is not configured"
	}
	return fmt.Sprintf("%s is not configured for chain %q", e.Channel, e.Chain)
}

// CandidateFailure records why one requested target has no route. It
// intentionally excludes the provider/channel name so SDK consumers do not
// accidentally expose infrastructure brands in user-facing messages.
type CandidateFailure struct {
	FromChain         string `json:"fromChain"`
	ToChain           string `json:"toChain"`
	ToToken           string `json:"toToken"`
	SourceIssuer      string `json:"sourceIssuer,omitempty"`
	Code              string `json:"code"`
	RetryAfterSeconds int    `json:"retryAfterSeconds,omitempty"`
}

type Response struct {
	Routes        []*Route            `json:"routes"`
	Failures      []*CandidateFailure `json:"failures,omitempty"`
	Reason        string              `json:"reason,omitempty"`
	ReasonMessage string              `json:"reasonMessage,omitempty"`
}

type QuoteFn func(ctx context.Context, cand *Candidate, in *dexmodel.DexQuoteIn) (*dexmodel.DexQuoteOut, error)

type Engine struct {
	Logf func(format string, args ...interface{})
}

func (e *Engine) Quote(ctx context.Context, req *Request, cands []*Candidate, quote QuoteFn) *Response {
	if req == nil || quote == nil {
		return &Response{Reason: "INVALID_REQUEST"}
	}
	resp := &Response{
		Routes:   []*Route{},
		Failures: []*CandidateFailure{},
	}
	if len(cands) == 0 {
		resp.Reason = "NO_CANDIDATE"
		return resp
	}
	if _, ok := new(big.Int).SetString(req.FromAmount, 10); !ok {
		resp.Reason = "INVALID_AMOUNT"
		return resp
	}

	slippagePct := "1"
	if req.SlippageBps > 0 {
		slippagePct = strconv.FormatFloat(float64(req.SlippageBps)/100.0, 'f', -1, 64)
	}
	feeRate := req.FeeRate
	if feeRate == "" {
		feeRate = "0"
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	qctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type result struct {
		cand *Candidate
		out  *dexmodel.DexQuoteOut
		err  error
	}
	results := make([]result, len(cands))
	var wg sync.WaitGroup
	for i, c := range cands {
		wg.Add(1)
		go func(i int, c *Candidate) {
			defer wg.Done()
			if req.ScamCheck != nil && req.ScamCheck(c.ToChain, c.ToToken) {
				results[i] = result{c, nil, fmt.Errorf("scam-filtered: %s/%s", c.ToChain, c.ToToken)}
				return
			}
			in := &dexmodel.DexQuoteIn{
				FromChain:               c.FromChain,
				ToChain:                 c.ToChain,
				FromToken:               c.FromToken,
				ToToken:                 c.ToToken,
				FromAddress:             c.FromAddress,
				ToAddress:               c.ToAddress,
				FromAmount:              req.FromAmount,
				Slippage:                slippagePct,
				FeeRate:                 feeRate,
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
				FromCoinCode:            c.FromCoinCode,
				ToCoinCode:              c.ToCoinCode,
				FixedRateFlag:           req.FixedRateFlag,
			}
			out, err := quote(qctx, c, in)
			results[i] = result{c, out, err}
			if err != nil && e.Logf != nil {
				e.Logf("[trade-engine] %s→%s (%s/%s): %v", c.FromChain, c.ToChain, c.Channel, c.ToToken, err)
			}
		}(i, c)
	}
	wg.Wait()

	out := make([]*Route, 0, len(results))
	providerErrors := 0
	configurationErrors := 0
	emptyResponses := 0
	timedOut := false
	rateLimited := false
	for _, r := range results {
		if r.err != nil {
			providerErrors++
			code, retryAfter := classifyQuoteFailure(r.err)
			if code == FailureConfig {
				configurationErrors++
			}
			timedOut = timedOut || code == FailureTimeout
			rateLimited = rateLimited || code == FailureRateLimited
			resp.Failures = append(resp.Failures, candidateFailure(r.cand, code, retryAfter))
			continue
		}
		if r.out == nil {
			providerErrors++
			resp.Failures = append(resp.Failures, candidateFailure(r.cand, FailureProvider, 0))
			continue
		}
		if len(r.out.Routes) == 0 {
			emptyResponses++
			resp.Failures = append(resp.Failures, candidateFailure(r.cand, FailureNoRoute, 0))
			continue
		}
		best := r.out.Routes[0]
		toAmtUsdGross := RawToHuman(best.AmountOut, r.cand.ToDecimals) * req.ToPriceUsd

		expectedFillUsd := 0.0
		if best.PriceImpactPct > 0 {
			expectedFillUsd = toAmtUsdGross * (1 - best.PriceImpactPct/100.0)
		} else if req.FromAmountUsd > 0 && r.cand.LiquidityUsd > 0 {
			ratio := r.cand.LiquidityUsd / (r.cand.LiquidityUsd + req.FromAmountUsd)
			expectedFillUsd = toAmtUsdGross * ratio
		}

		feeUsd, _ := strconv.ParseFloat(best.FeeInUsd, 64)
		channel := r.out.Channel
		if channel == "" {
			channel = r.cand.Channel
		}
		route := &Route{
			CrossChain:      r.cand.CrossChain(),
			FromChain:       r.cand.FromChain,
			FromToken:       r.cand.FromToken,
			ToChain:         r.cand.ToChain,
			ToToken:         r.cand.ToToken,
			ToDecimals:      r.cand.ToDecimals,
			SourceIssuer:    r.cand.SourceIssuer,
			ToAmount:        best.AmountOut,
			ToAmountUsd:     toAmtUsdGross,
			ExpectedFillUsd: expectedFillUsd,
			Channel:         channel,
			RouteId:         best.RouteId,
			AmountOutMin:    best.AmountOutMin,
			NeedBuild:       best.NeedBuild,
			TxData:          best.TxData,
			ApprovalData:    best.ApprovalData,
			TrustedSpenders: append([]string(nil), best.TrustedSpenders...),
			FeeUsd:          feeUsd,
			PriceImpactPct:  best.PriceImpactPct,
			LiquidityUsd:    r.cand.LiquidityUsd,
			EstSeconds:      best.EstimatedTime,
			RouteTags:       append([]string(nil), best.RouteTags...),
			ExpiresAt:       best.ExpiresAt,
			SubmitRequest:   append(json.RawMessage(nil), best.SubmitRequest...),
			Warnings:        append([]Warning(nil), best.Warnings...),
			Reason:          ReasonFor(r.cand, channel),
		}
		if req.FromAmountUsd > 0 && route.LiquidityUsd > 0 && req.FromAmountUsd > route.LiquidityUsd*ThinLiquidityRatio {
			route.Warnings = append(route.Warnings, Warning{
				Code: WarnThinLiquidity,
				Message: fmt.Sprintf("this trade is %.0f%% of the pool ($%.0f of $%.0f) — real fill will be much worse than quoted",
					req.FromAmountUsd/route.LiquidityUsd*100, req.FromAmountUsd, route.LiquidityUsd),
			})
		}
		out = append(out, route)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if IsRouteUnsafe(out[i]) != IsRouteUnsafe(out[j]) {
			return !IsRouteUnsafe(out[i])
		}
		iKey := out[i].ExpectedFillUsd
		if iKey == 0 {
			iKey = out[i].ToAmountUsd
		}
		jKey := out[j].ExpectedFillUsd
		if jKey == 0 {
			jKey = out[j].ToAmountUsd
		}
		if iKey != jKey {
			return iKey > jKey
		}
		return !out[i].CrossChain && out[j].CrossChain
	})
	for i, r := range out {
		r.Rank = i + 1
		r.Recommended = i == 0 && !IsRouteUnsafe(r)
	}

	resp.Routes = out
	if len(out) == 0 {
		switch {
		case configurationErrors > 0:
			resp.Reason = "QUOTE_CONFIGURATION_ERROR"
			resp.ReasonMessage = "one or more requested quote channels are not configured for the selected chain"
		case errors.Is(qctx.Err(), context.DeadlineExceeded) || timedOut:
			resp.Reason = "QUOTE_TIMEOUT"
			resp.ReasonMessage = "quote request timed out — try again"
		case rateLimited:
			resp.Reason = "QUOTE_RATE_LIMITED"
			resp.ReasonMessage = "quote provider is busy — wait a moment and try again"
		case providerErrors > 0:
			resp.Reason = "QUOTE_PROVIDER_ERROR"
			resp.ReasonMessage = "quote provider could not complete the request — try again"
		case emptyResponses > 0:
			resp.Reason = "NO_ROUTE_AVAILABLE"
			resp.ReasonMessage = "no route is available for this pair at the moment"
		default:
			resp.Reason = "NO_ROUTE_AVAILABLE"
			resp.ReasonMessage = "no route is available at the moment"
		}
	}
	return resp
}

func candidateFailure(c *Candidate, code string, retryAfterSeconds int) *CandidateFailure {
	failure := &CandidateFailure{
		Code:              code,
		RetryAfterSeconds: retryAfterSeconds,
	}
	if c != nil {
		failure.FromChain = c.FromChain
		failure.ToChain = c.ToChain
		failure.ToToken = c.ToToken
		failure.SourceIssuer = c.SourceIssuer
	}
	return failure
}

func classifyQuoteFailure(err error) (code string, retryAfterSeconds int) {
	if err == nil {
		return FailureProvider, 0
	}
	var configErr *ChannelConfigurationError
	if errors.As(err, &configErr) {
		return FailureConfig, 0
	}
	var statusErr *httpc.HTTPStatusError
	if errors.As(err, &statusErr) && statusErr.StatusCode == 429 {
		return FailureRateLimited, statusErr.RetryAfterSeconds
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return FailureTimeout, 0
	}
	errText := strings.ToLower(err.Error())
	if strings.Contains(errText, "rate limit") ||
		strings.Contains(errText, "status=429") ||
		strings.Contains(errText, "statuscode=429") ||
		strings.Contains(errText, "error code: 1015") {
		return FailureRateLimited, 0
	}
	return FailureProvider, 0
}

func IsRouteUnsafe(r *Route) bool {
	if r == nil {
		return false
	}
	for _, w := range r.Warnings {
		if w.Code == WarnPriceImpactHigh || w.Code == WarnThinLiquidity {
			return true
		}
	}
	return false
}

func ReasonFor(c *Candidate, channel string) string {
	if c == nil {
		return ""
	}
	if c.CrossChain() {
		return fmt.Sprintf("cross-chain via %s (%s pool)", channel, c.ToChain)
	}
	if c.LiquidityUsd > 0 {
		return fmt.Sprintf("same-chain, liq %s", FormatUSD(c.LiquidityUsd))
	}
	return "same-chain"
}

func RawToHuman(raw string, decimals int) float64 {
	if raw == "" || decimals <= 0 {
		return 0
	}
	r, ok := new(big.Rat).SetString(raw)
	if !ok {
		return 0
	}
	div := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	r.Quo(r, new(big.Rat).SetInt(div))
	f, _ := r.Float64()
	return f
}

func FormatUSD(v float64) string {
	switch {
	case v >= 1_000_000:
		return fmt.Sprintf("$%.1fM", v/1_000_000)
	case v >= 1_000:
		return fmt.Sprintf("$%.1fk", v/1_000)
	default:
		return fmt.Sprintf("$%.0f", v)
	}
}
