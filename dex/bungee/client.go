package bungee

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	dexmodel "github.com/kernelflowlabs/wallet-lib/common/dexmodel"
	"github.com/kernelflowlabs/wallet-lib/common/httpc"
	"github.com/shopspring/decimal"
)

type Client struct {
	client *httpc.Request
}

func NewClient(opts ...ClientOption) *Client {
	cfg := clientConfig{rateLimit: RateLimitPublic}
	for _, opt := range opts {
		opt(&cfg)
	}
	base := "https://public-backend.socket.tech"
	if cfg.apiKey != "" {
		base = "https://dedicated-backend.socket.tech"
	}
	c := httpc.NewRequest(base,
		map[string]string{"Content-Type": "application/json"})
	if cfg.apiKey != "" {
		c.SetHeader("x-api-key", cfg.apiKey)
	}
	if cfg.affiliateId != "" {
		c.SetHeader("affiliate", cfg.affiliateId)
	}
	cfg.rateLimit.Apply(c)
	return &Client{client: c}
}

func (c *Client) GetSupportedChainIds(ctx context.Context) ([]string, error) {
	ids := make([]string, 0, len(idChainMapper))
	for id := range idChainMapper {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

func (c *Client) Quote(ctx context.Context, in *dexmodel.DexQuoteIn) (*dexmodel.DexQuoteOut, error) {
	req, err := c.toSocketQuoteReq(in)
	if err != nil {
		return nil, fmt.Errorf("fail to convert request to QuoteRequest: %w", err)
	}
	out, err := c.QuoteV3(ctx, req)
	if err != nil {
		return nil, err
	}
	res := c.toStandardQuoteRes(out.Result)
	if res == nil {
		return nil, fmt.Errorf("fail to convert response")
	}
	return res, nil
}

// QuoteV3 returns the complete Socket V3 quote response.
func (c *Client) QuoteV3(ctx context.Context, req *QuoteRequest) (*QuoteResponse, error) {
	if err := validateQuoteRequest(req); err != nil {
		return nil, err
	}

	path := "v3/swap/quote"
	query := url.Values{}
	userOps := make([]string, 0, len(req.UserOps))
	for _, op := range req.UserOps {
		userOps = append(userOps, string(op))
	}
	query.Set("userOps", strings.Join(userOps, ","))
	setQuery(query, "originChainId", req.OriginChainId)
	query.Set("destinationChainId", req.DestinationChainId)
	query.Set("inputToken", req.InputToken)
	query.Set("outputToken", req.OutputToken)
	setQuery(query, "userAddress", req.UserAddress)
	query.Set("receiverAddress", req.ReceiverAddress)
	query.Set("inputAmount", req.InputAmount)
	if req.Slippage != "" {
		query.Set("slippage", strings.TrimSpace(req.Slippage))
	}
	setQuery(query, "refundAddress", req.RefundAddress)
	setQuery(query, "contractCaller", req.ContractCaller)
	if req.FeeBps != "" {
		query.Set("feeBps", strings.TrimSpace(req.FeeBps))
	}
	setQuery(query, "feeTakerAddress", req.FeeTakerAddress)
	setBoolQuery(query, "refuel", req.Refuel)
	setQuery(query, "destinationPayload", req.DestinationPayload)
	setQuery(query, "destinationGasLimit", req.DestinationGasLimit)
	if providers := normalizeCSV(req.IncludeProvider); providers != "" {
		query.Set("includeProvider", providers)
	}
	if providers := normalizeCSV(req.ExcludeProvider); providers != "" {
		query.Set("excludeProvider", providers)
	}
	setQuery(query, "exchange", req.Exchange)
	setBoolQuery(query, "includeQuoteRejections", req.IncludeQuoteRejections)
	setBoolQuery(query, "private", req.Private)
	setBoolQuery(query, "simulatedQuotesRequired", req.SimulatedQuotesRequired)
	setQuery(query, "solanaSponsorAddress", req.SolanaSponsorAddress)

	out := &QuoteResponse{}
	err := c.client.Get(ctx, out, path, query)
	if err != nil {
		return out, fmt.Errorf("fail to quote: %w", err)
	} else if !out.Success {
		return out, fmt.Errorf("fail to quote, statusCode=%d, msg=%s", out.StatusCode, out.Message)
	}
	if out.Result == nil {
		return out, fmt.Errorf("fail to quote: result is null")
	}
	return out, nil
}

func (c *Client) Status(ctx context.Context, in *dexmodel.DexCheckTxIn) (*dexmodel.DexCheckTxOut, error) {
	if in == nil {
		return nil, fmt.Errorf("in is nil")
	} else if in.Hash == "" {
		return nil, fmt.Errorf("hash is empty")
	}

	out := &dexmodel.DexCheckTxOut{
		Channel: "bungee",
		ToChain: in.ToChain,
	}
	req := &StatusRequest{IncludeQuoteDetails: in.IncludeQuoteDetails}
	switch in.HashType {
	case dexmodel.DexHashTypeTxHash:
		req.SrcTxHash = in.Hash
	case "", dexmodel.DexHashTypeRequestHash:
		req.QuoteId = in.Hash
	default:
		return nil, fmt.Errorf("invalid hashType: %s", in.HashType)
	}
	res, err := c.StatusV3(ctx, req)
	if err != nil {
		var statusErr *httpc.HTTPStatusError
		if (res != nil && res.StatusCode == 404) ||
			(errors.As(err, &statusErr) && statusErr.StatusCode == 404) {
			out.Status = dexmodel.DexStatusNotFound
			return out, nil
		}
		return nil, err
	}

	out.FromHash = res.Result.Origin.TxHash
	out.ProviderStatus = res.Result.Status
	out.ProviderStatusCode = res.Result.StatusCode
	out.OriginStatus = res.Result.Origin.Status
	out.DestinationStatus = res.Result.Destination.Status
	out.UserOp = res.Result.UserOp
	out.RouteName = res.Result.RouteDetails.Name
	out.RouteLogoURI = res.Result.RouteDetails.LogoURI
	out.IsDestPayloadExecuted = res.Result.IsDestPayloadExecuted
	out.QuoteDetails = res.Result.QuoteDetails
	if out.ToChain == "" {
		out.ToChain = idChainMapper[strconv.Itoa(res.Result.Destination.ChainId)]
	}
	switch res.Result.Status {
	case "COMPLETED":
		out.Status = dexmodel.DexStatusSucceeded
		out.ToHash = res.Result.Destination.TxHash
	case "REFUNDED":
		out.Status = dexmodel.DexStatusRefunded
		if res.Result.Refund != nil && res.Result.Refund.TxHash != "" {
			out.ToHash = res.Result.Refund.TxHash
		} else {
			out.ToHash = res.Result.Destination.TxHash
		}
	case "FAILED", "EXPIRED":
		out.Status = dexmodel.DexStatusFailed
	case "PENDING", "IN_PROGRESS", "REFUNDING":
		out.Status = dexmodel.DexStatusPending
	default:
		out.Status = dexmodel.DexStatusPending
		out.Msg = fmt.Sprintf("unmapped bungee status: %s", res.Result.Status)
	}
	return out, nil
}

// StatusV3 returns the complete Socket V3 status response.
func (c *Client) StatusV3(ctx context.Context, req *StatusRequest) (*StatusResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("status request is nil")
	}
	if req.QuoteId == "" && req.SrcTxHash == "" {
		return nil, fmt.Errorf("quoteId or srcTxHash is required")
	}

	query := url.Values{}
	setQuery(query, "quoteId", req.QuoteId)
	setQuery(query, "srcTxHash", req.SrcTxHash)
	setBoolQuery(query, "includeQuoteDetails", req.IncludeQuoteDetails)

	out := &StatusResponse{}
	if err := c.client.Get(ctx, out, "v3/swap/status", query); err != nil {
		return out, fmt.Errorf("fail to get status: %w", err)
	}
	if !out.Success {
		return out, fmt.Errorf("fail to get status, statusCode=%d, msg=%s", out.StatusCode, out.Message)
	}
	if out.Result == nil {
		return out, fmt.Errorf("fail to get status: result is null")
	}
	return out, nil
}

func validateQuoteRequest(req *QuoteRequest) error {
	if req == nil {
		return fmt.Errorf("quote request is nil")
	}
	if len(req.UserOps) == 0 {
		return fmt.Errorf("userOps is required")
	}

	var needsOrigin, needsUser, needsRefund, needsExchange bool
	for _, op := range req.UserOps {
		switch op {
		case UserOpTx:
			needsOrigin = true
			needsUser = true
		case UserOpDeposit:
			needsOrigin = true
			needsRefund = true
		case UserOpCEXWithdraw:
			needsRefund = true
			needsExchange = true
		default:
			return fmt.Errorf("invalid userOp: %s", op)
		}
	}
	if needsOrigin && req.OriginChainId == "" {
		return fmt.Errorf("originChainId is required for tx and deposit userOps")
	}
	if req.DestinationChainId == "" || req.InputToken == "" || req.InputAmount == "" ||
		req.OutputToken == "" || req.ReceiverAddress == "" {
		return fmt.Errorf("destinationChainId, inputToken, inputAmount, outputToken, and receiverAddress are required")
	}
	if needsUser && req.UserAddress == "" {
		return fmt.Errorf("userAddress is required for tx userOp")
	}
	if needsRefund && req.RefundAddress == "" {
		return fmt.Errorf("refundAddress is required for deposit and cex-withdraw userOps")
	}
	if needsExchange && req.Exchange == "" {
		return fmt.Errorf("exchange is required for cex-withdraw userOp")
	}
	if err := validateDecimal("slippage", req.Slippage, false, decimal.Zero); err != nil {
		return err
	}
	if (req.FeeBps == "") != (req.FeeTakerAddress == "") {
		return fmt.Errorf("feeBps and feeTakerAddress must be provided together")
	}
	if req.FeeBps != "" {
		if err := validateDecimal("feeBps", req.FeeBps, true, decimal.NewFromInt(10000)); err != nil {
			return err
		}
	}
	if (req.DestinationPayload == "") != (req.DestinationGasLimit == "") {
		return fmt.Errorf("destinationPayload and destinationGasLimit must be provided together")
	}
	if providerListsOverlap(req.IncludeProvider, req.ExcludeProvider) {
		return fmt.Errorf("includeProvider and excludeProvider must not overlap")
	}
	if req.SolanaSponsorAddress != "" && req.OriginChainId != "89999" {
		return fmt.Errorf("solanaSponsorAddress requires Solana originChainId 89999")
	}
	return nil
}

func validateDecimal(name, value string, positive bool, max decimal.Decimal) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	d, err := decimal.NewFromString(strings.TrimSpace(value))
	if err != nil {
		return fmt.Errorf("invalid %s: %w", name, err)
	}
	if (positive && !d.IsPositive()) || (!positive && d.IsNegative()) {
		requirement := "non-negative"
		if positive {
			requirement = "greater than zero"
		}
		return fmt.Errorf("%s must be %s", name, requirement)
	}
	if !max.IsZero() && d.GreaterThan(max) {
		return fmt.Errorf("%s must not exceed %s", name, max.String())
	}
	return nil
}

func setQuery(query url.Values, key, value string) {
	if value != "" {
		query.Set(key, value)
	}
}

func setBoolQuery(query url.Values, key string, value *bool) {
	if value != nil {
		query.Set(key, strconv.FormatBool(*value))
	}
}

func normalizeCSV(value string) string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, ",")
}

func providerListsOverlap(include, exclude string) bool {
	included := make(map[string]struct{})
	for _, provider := range strings.Split(normalizeCSV(include), ",") {
		if provider != "" {
			included[provider] = struct{}{}
		}
	}
	for _, provider := range strings.Split(normalizeCSV(exclude), ",") {
		if _, ok := included[provider]; ok && provider != "" {
			return true
		}
	}
	return false
}
