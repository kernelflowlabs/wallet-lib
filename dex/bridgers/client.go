package bridgers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/kernelflowlabs/wallet-sdk/common/dexmodel"
	"github.com/kernelflowlabs/wallet-sdk/common/httpc"
	"github.com/shopspring/decimal"
)

const Channel = "bridgers"

const nativeToken = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

const maxResponseBytes = 8 << 20

type tokenCacheEntry struct {
	tokens    []Token
	expiresAt time.Time
}

type tokenLoad struct {
	done   chan struct{}
	tokens []Token
	err    error
}

type Client struct {
	client     *httpc.Request
	config     Config
	tokenMu    sync.Mutex
	tokenCache map[string]tokenCacheEntry
	tokenLoads map[string]*tokenLoad
}

func NewClient(config Config) (*Client, error) {
	config = normalizeConfig(config)
	if config.SourceFlag == "" {
		return nil, fmt.Errorf("sourceFlag is required")
	}
	if config.FixedRateFlag != 0 && config.FixedRateFlag != 1 {
		return nil, fmt.Errorf("fixedRateFlag must be 0 or 1")
	}
	options := append([]httpc.Option{}, config.HTTPOptions...)
	options = append(options, httpc.WithMaxResponseBytes(maxResponseBytes))
	if config.RateLimit.QPS > 0 {
		burst := config.RateLimit.Burst
		if burst <= 0 {
			burst = 1
		}
		options = append(options, httpc.WithRateLimit(config.RateLimit.QPS, burst))
	} else {
		options = append(options, httpc.WithoutRateLimit())
	}
	request, err := httpc.NewRequestWithOptions(config.BaseURL, map[string]string{"Content-Type": "application/json"}, options...)
	if err != nil {
		return nil, fmt.Errorf("create Bridgers HTTP client: %w", err)
	}
	return &Client{
		client:     request,
		config:     config,
		tokenCache: make(map[string]tokenCacheEntry),
		tokenLoads: make(map[string]*tokenLoad),
	}, nil
}

func (c *Client) SupportedChains(ctx context.Context) ([]string, error) {
	response, err := c.Tokens(ctx, "")
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	for _, token := range response.Data.Tokens {
		seen[canonicalChain(token.Chain)] = struct{}{}
	}
	chains := make([]string, 0, len(seen))
	for chain := range seen {
		chains = append(chains, chain)
	}
	sort.Strings(chains)
	return chains, nil
}

func (c *Client) Tokens(ctx context.Context, chain string) (*TokenListResponse, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("Bridgers client is nil")
	}
	providerName := ""
	var err error
	if strings.TrimSpace(chain) != "" {
		providerName, err = providerChain(chain)
		if err != nil {
			return nil, err
		}
	}
	response := &TokenListResponse{}
	if err := c.client.Post(ctx, response, "api/exchangeRecord/getToken", &TokenListRequest{Chain: providerName}); err != nil {
		return response, fmt.Errorf("get Bridgers tokens: %w", err)
	}
	if err := response.responseMeta.providerError(); err != nil {
		return response, err
	}
	if providerName != "" {
		response.Data.Tokens = tokensForChain(response.Data.Tokens, providerName)
		if len(response.Data.Tokens) == 0 {
			all, err := c.Tokens(ctx, "")
			if err != nil {
				return response, err
			}
			response.Data.Tokens = tokensForChain(all.Data.Tokens, providerName)
		}
	}
	return response, nil
}

func (c *Client) QuoteRaw(ctx context.Context, request *QuoteRequest) (*QuoteResponse, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("Bridgers client is nil")
	}
	if request != nil {
		requestCopy := *request
		request = &requestCopy
		request.SourceFlag = c.config.SourceFlag
		if request.SourceType == "" {
			request.SourceType = c.config.SourceType
		}
		if request.EquipmentNo == "" {
			request.EquipmentNo, _ = equipmentNumber("", request.UserAddress)
		}
	}
	if err := validateQuoteRequest(request); err != nil {
		return nil, err
	}
	response := &QuoteResponse{}
	if err := c.client.Post(ctx, response, "api/sswap/quote", request); err != nil {
		return response, fmt.Errorf("get Bridgers quote: %w", err)
	}
	if err := response.responseMeta.providerError(); err != nil {
		return response, err
	}
	if response.Data.TxData == nil {
		return response, fmt.Errorf("Bridgers quote returned no txData")
	}
	return response, nil
}

func (c *Client) Swap(ctx context.Context, request *SwapRequest) (*SwapResponse, error) {
	return c.swap(ctx, request, "api/sswap/swap")
}

func (c *Client) SolanaGaslessSwap(ctx context.Context, request *SwapRequest) (*SwapResponse, error) {
	if request == nil || !strings.EqualFold(strings.TrimSpace(request.FromTokenChain), "SOLANA") {
		return nil, fmt.Errorf("Solana gasless swap requires fromTokenChain=SOLANA")
	}
	coinCode := strings.ToUpper(strings.TrimSpace(request.FromCoinCode))
	if !strings.HasPrefix(coinCode, "USDC") && !strings.HasPrefix(coinCode, "USDT") {
		return nil, fmt.Errorf("Solana gasless swap supports USDC and USDT input only")
	}
	return c.swap(ctx, request, "api/sswap/solana/swap")
}

func (c *Client) swap(ctx context.Context, request *SwapRequest, path string) (*SwapResponse, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("Bridgers client is nil")
	}
	if request != nil {
		requestCopy := *request
		request = &requestCopy
		request.SourceFlag = c.config.SourceFlag
		if request.SourceType == "" {
			request.SourceType = c.config.SourceType
		}
	}
	if err := validateSwapRequest(request); err != nil {
		return nil, err
	}
	response := &SwapResponse{}
	if err := c.client.Post(ctx, response, path, request); err != nil {
		return response, fmt.Errorf("build Bridgers swap: %w", err)
	}
	if err := response.responseMeta.providerError(); err != nil {
		return response, err
	}
	if len(response.Data.TxData) == 0 || string(response.Data.TxData) == "null" {
		return response, fmt.Errorf("Bridgers swap returned no txData")
	}
	return response, nil
}

func (c *Client) GenerateOrder(ctx context.Context, request *GenerateOrderRequest) (*GenerateOrderResponse, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("Bridgers client is nil")
	}
	if request == nil {
		return nil, fmt.Errorf("generate order request is nil")
	}
	requestCopy := *request
	request = &requestCopy
	request.SourceFlag = c.config.SourceFlag
	if request.SourceType == "" {
		request.SourceType = c.config.SourceType
	}
	if err := validateGenerateOrderRequest(request); err != nil {
		return nil, err
	}
	response := &GenerateOrderResponse{}
	if err := c.client.Post(ctx, response, "api/exchangeRecord/updateDataAndStatus", request); err != nil {
		return response, fmt.Errorf("generate Bridgers order: %w", err)
	}
	if err := response.responseMeta.providerError(); err != nil {
		return response, err
	}
	if response.Data.OrderID == "" {
		return response, fmt.Errorf("Bridgers generated no orderId")
	}
	return response, nil
}

func (c *Client) Orders(ctx context.Context, request *OrderListRequest) (*OrderListResponse, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("Bridgers client is nil")
	}
	if request == nil {
		return nil, fmt.Errorf("order list request is nil")
	}
	requestCopy := *request
	request = &requestCopy
	request.SourceFlag = c.config.SourceFlag
	if request.SourceType == "" {
		request.SourceType = c.config.SourceType
	}
	if request.EquipmentNo == "" || request.FromAddress == "" || request.PageNo <= 0 || request.PageSize <= 0 {
		return nil, fmt.Errorf("equipmentNo, fromAddress, positive pageNo, and positive pageSize are required")
	}
	response := &OrderListResponse{}
	if err := c.client.Post(ctx, response, "api/exchangeRecord/getTransData", request); err != nil {
		return response, fmt.Errorf("get Bridgers orders: %w", err)
	}
	if err := response.responseMeta.providerError(); err != nil {
		return response, err
	}
	return response, nil
}

func (c *Client) OrderDetails(ctx context.Context, orderID string) (*OrderDetailsResponse, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("Bridgers client is nil")
	}
	if strings.TrimSpace(orderID) == "" {
		return nil, fmt.Errorf("orderId is required")
	}
	response := &OrderDetailsResponse{}
	request := &OrderDetailsRequest{OrderID: strings.TrimSpace(orderID)}
	if err := c.client.Post(ctx, response, "api/exchangeRecord/getTransDataById", request); err != nil {
		return response, fmt.Errorf("get Bridgers order details: %w", err)
	}
	if err := response.responseMeta.providerError(); err != nil {
		return response, err
	}
	return response, nil
}

func (c *Client) Quote(ctx context.Context, in *dexmodel.DexQuoteIn) (*dexmodel.DexQuoteOut, error) {
	request, slippage, slippageFraction, fromChain, toChain, err := c.standardQuoteRequest(in)
	if err != nil {
		return nil, err
	}
	quoteResponse, err := c.QuoteRaw(ctx, request)
	if err != nil {
		return nil, err
	}
	quote := quoteResponse.Data.TxData
	if err := validateAmountRange(in.FromAmount, quote); err != nil {
		return nil, err
	}
	quotedAmount, ok := new(big.Int).SetString(quote.AmountOutMin, 10)
	if !ok || quotedAmount.Sign() <= 0 {
		return nil, fmt.Errorf("Bridgers amountOutMin must be a positive raw integer")
	}
	if !common.IsHexAddress(quote.ContractAddress) {
		return nil, fmt.Errorf("Bridgers quote returned an invalid contract address")
	}
	fromCoinCode, toCoinCode, err := c.resolveCoinCodes(ctx, in, fromChain, toChain)
	if err != nil {
		return nil, err
	}
	swapRequest := &SwapRequest{
		FromTokenAddress: in.FromToken,
		AmountOutMin:     quote.AmountOutMin,
		EquipmentNo:      request.EquipmentNo,
		ToAddress:        in.ToAddress,
		ToTokenChain:     toChain,
		FromTokenAmount:  in.FromAmount,
		FromTokenChain:   fromChain,
		ToTokenAddress:   in.ToToken,
		FromAddress:      in.FromAddress,
		SourceFlag:       c.config.SourceFlag,
		FromCoinCode:     fromCoinCode,
		ToCoinCode:       toCoinCode,
		SourceType:       request.SourceType,
		Slippage:         slippageFraction,
		FixedRateFlag:    request.FixedRateFlag,
	}
	swapResponse, err := c.Swap(ctx, swapRequest)
	if err != nil {
		return nil, err
	}
	tx, err := standardTransaction(swapResponse.Data.TxData, quote.ContractAddress)
	if err != nil {
		return nil, err
	}
	if isNativeToken(in.FromToken) {
		if tx.Value != in.FromAmount {
			return nil, fmt.Errorf("Bridgers native transaction value differs from fromAmount")
		}
	} else if tx.Value != "" {
		return nil, fmt.Errorf("Bridgers ERC-20 transaction returned an unexpected native value")
	}
	minimum, err := minimumOutput(quote.AmountOutMin, slippage)
	if err != nil {
		return nil, err
	}
	suggestedSlippage := percentFromFraction(quote.RecommendedSlippage.String())
	report := &GenerateOrderRequest{
		EquipmentNo:      request.EquipmentNo,
		SourceFlag:       c.config.SourceFlag,
		SourceType:       request.SourceType,
		FromTokenAddress: in.FromToken,
		ToTokenAddress:   in.ToToken,
		FromAddress:      in.FromAddress,
		ToAddress:        in.ToAddress,
		FromTokenChain:   fromChain,
		ToTokenChain:     toChain,
		FromTokenAmount:  in.FromAmount,
		AmountOutMin:     quote.AmountOutMin,
		FromCoinCode:     fromCoinCode,
		ToCoinCode:       toCoinCode,
		Slippage:         slippageFraction,
	}
	submitRequest, err := json.Marshal(report)
	if err != nil {
		return nil, fmt.Errorf("encode Bridgers submit request: %w", err)
	}
	approval, trusted := standardApproval(in, quote.ContractAddress, tx.To)
	route := &dexmodel.DexRoute{
		RouteId:           routeID(in, quote, tx),
		Name:              providerName(quote.Dex),
		AmountOut:         quote.AmountOutMin,
		AmountOutMin:      minimum,
		Slippage:          decimalFloat(slippage),
		SuggestedSlippage: suggestedSlippage,
		EstimatedTime:     quote.EstimatedTime,
		NeedBuild:         false,
		TxData:            tx,
		ApprovalData:      approval,
		TrustedSpenders:   trusted,
		RouteTags:         []string{"MAX_OUTPUT"},
		SubmitRequest:     submitRequest,
	}
	return &dexmodel.DexQuoteOut{Channel: Channel, Routes: []*dexmodel.DexRoute{route}}, nil
}

func (c *Client) Status(ctx context.Context, in *dexmodel.DexCheckTxIn) (*dexmodel.DexCheckTxOut, error) {
	if in == nil {
		return nil, fmt.Errorf("status request is nil")
	}
	if strings.TrimSpace(in.Hash) == "" {
		return nil, fmt.Errorf("orderId is required")
	}
	if in.HashType != dexmodel.DexHashTypeRequestHash {
		return nil, fmt.Errorf("Bridgers status requires orderId with hashType=requestHash; call GenerateOrder after broadcasting the source transaction")
	}
	response, err := c.OrderDetails(ctx, in.Hash)
	if err != nil {
		var providerErr *ProviderError
		if errors.As(err, &providerErr) && providerErr.Code == 415 {
			return &dexmodel.DexCheckTxOut{Channel: Channel, Status: dexmodel.DexStatusNotFound, ToChain: in.ToChain}, nil
		}
		return nil, err
	}
	record := response.Data
	status, message := standardStatus(record.Status)
	toHash := record.ToHash
	if status == dexmodel.DexStatusRefunded && record.RefundHash != "" {
		toHash = record.RefundHash
	}
	toChain := in.ToChain
	if toChain == "" {
		toChain = canonicalChain(record.ToChain)
	}
	return &dexmodel.DexCheckTxOut{
		Channel:        Channel,
		Status:         status,
		ToChain:        toChain,
		ToHash:         toHash,
		FromHash:       record.Hash,
		ProviderStatus: record.Status,
		RouteName:      providerName(record.DexName),
		Msg:            message,
	}, nil
}

func (c *Client) standardQuoteRequest(in *dexmodel.DexQuoteIn) (*QuoteRequest, decimal.Decimal, string, string, string, error) {
	if in == nil {
		return nil, decimal.Zero, "", "", "", fmt.Errorf("quote request is nil")
	}
	fromChain, err := providerChain(in.FromChain)
	if err != nil {
		return nil, decimal.Zero, "", "", "", err
	}
	if !isEVMProviderChain(fromChain) {
		return nil, decimal.Zero, "", "", "", fmt.Errorf("Bridgers standard Quote supports EVM source transactions only; use QuoteRaw and Swap for %s", fromChain)
	}
	toChain, err := providerChain(in.ToChain)
	if err != nil {
		return nil, decimal.Zero, "", "", "", err
	}
	if in.FromToken == "" || in.ToToken == "" || in.FromAddress == "" || in.ToAddress == "" {
		return nil, decimal.Zero, "", "", "", fmt.Errorf("fromToken, toToken, fromAddress, and toAddress are required")
	}
	if !common.IsHexAddress(in.FromAddress) {
		return nil, decimal.Zero, "", "", "", fmt.Errorf("invalid EVM fromAddress")
	}
	if !isNativeToken(in.FromToken) && !common.IsHexAddress(in.FromToken) {
		return nil, decimal.Zero, "", "", "", fmt.Errorf("invalid EVM fromToken")
	}
	if isEVMProviderChain(toChain) {
		if !isNativeToken(in.ToToken) && !common.IsHexAddress(in.ToToken) {
			return nil, decimal.Zero, "", "", "", fmt.Errorf("invalid EVM toToken")
		}
		if !common.IsHexAddress(in.ToAddress) {
			return nil, decimal.Zero, "", "", "", fmt.Errorf("invalid EVM toAddress")
		}
	}
	amount, ok := new(big.Int).SetString(in.FromAmount, 10)
	if !ok || amount.Sign() <= 0 {
		return nil, decimal.Zero, "", "", "", fmt.Errorf("fromAmount must be a positive raw integer")
	}
	slippage, err := decimal.NewFromString(strings.TrimSpace(in.Slippage))
	if err != nil || !slippage.GreaterThan(decimal.Zero) || !slippage.LessThan(decimal.NewFromInt(100)) {
		return nil, decimal.Zero, "", "", "", fmt.Errorf("slippage must be greater than 0 and less than 100 percent")
	}
	if err := validateFee(in); err != nil {
		return nil, decimal.Zero, "", "", "", err
	}
	fixedRateFlag := c.config.FixedRateFlag
	if in.FixedRateFlag != nil {
		fixedRateFlag = *in.FixedRateFlag
	}
	if fixedRateFlag != 0 && fixedRateFlag != 1 {
		return nil, decimal.Zero, "", "", "", fmt.Errorf("fixedRateFlag must be 0 or 1")
	}
	sourceType := strings.TrimSpace(in.SourceType)
	if sourceType == "" {
		sourceType = c.config.SourceType
	}
	equipmentNo, err := equipmentNumber(in.EquipmentNo, in.FromAddress)
	if err != nil {
		return nil, decimal.Zero, "", "", "", err
	}
	return &QuoteRequest{
		EquipmentNo:      equipmentNo,
		SourceFlag:       c.config.SourceFlag,
		FromTokenAddress: in.FromToken,
		ToTokenAddress:   in.ToToken,
		FromTokenAmount:  in.FromAmount,
		FromTokenChain:   fromChain,
		ToTokenChain:     toChain,
		UserAddress:      in.FromAddress,
		SourceType:       sourceType,
		FixedRateFlag:    fixedRateFlag,
	}, slippage, slippage.Div(decimal.NewFromInt(100)).String(), fromChain, toChain, nil
}

func validateFee(in *dexmodel.DexQuoteIn) error {
	rate := strings.TrimSpace(in.FeeRate)
	if rate == "" {
		return nil
	}
	value, err := decimal.NewFromString(rate)
	if err != nil || value.IsNegative() {
		return fmt.Errorf("invalid feeRate")
	}
	if value.IsZero() {
		return nil
	}
	return fmt.Errorf("Bridgers does not accept a per-quote feeRate; configure partner fees with Bridgers")
}

func validateQuoteRequest(request *QuoteRequest) error {
	if request == nil {
		return fmt.Errorf("quote request is nil")
	}
	if request.EquipmentNo == "" || request.SourceFlag == "" || request.FromTokenAddress == "" ||
		request.ToTokenAddress == "" || request.FromTokenAmount == "" || request.FromTokenChain == "" || request.ToTokenChain == "" {
		return fmt.Errorf("equipmentNo, sourceFlag, token addresses, amount, and chains are required")
	}
	if request.FixedRateFlag != 0 && request.FixedRateFlag != 1 {
		return fmt.Errorf("fixedRateFlag must be 0 or 1")
	}
	return nil
}

func validateSwapRequest(request *SwapRequest) error {
	if request == nil {
		return fmt.Errorf("swap request is nil")
	}
	if request.FromTokenAddress == "" || request.AmountOutMin == "" || request.EquipmentNo == "" ||
		request.ToAddress == "" || request.ToTokenChain == "" || request.FromTokenAmount == "" ||
		request.FromTokenChain == "" || request.ToTokenAddress == "" || request.FromAddress == "" ||
		request.SourceFlag == "" || request.FromCoinCode == "" || request.ToCoinCode == "" {
		return fmt.Errorf("Bridgers swap required fields are missing")
	}
	if request.FixedRateFlag != 0 && request.FixedRateFlag != 1 {
		return fmt.Errorf("fixedRateFlag must be 0 or 1")
	}
	return nil
}

func validateGenerateOrderRequest(request *GenerateOrderRequest) error {
	if request.EquipmentNo == "" || request.SourceFlag == "" || request.Hash == "" ||
		request.FromTokenAddress == "" || request.ToTokenAddress == "" || request.FromAddress == "" ||
		request.ToAddress == "" || request.FromTokenChain == "" || request.ToTokenChain == "" ||
		request.FromTokenAmount == "" || request.AmountOutMin == "" || request.FromCoinCode == "" || request.ToCoinCode == "" {
		return fmt.Errorf("Bridgers generate order required fields are missing")
	}
	return nil
}

func equipmentNumber(configured, address string) (string, error) {
	value := strings.TrimSpace(configured)
	if value == "" {
		value = strings.TrimSpace(address)
		if len(value) > 32 {
			value = value[:32]
		}
	}
	if value == "" {
		return "", fmt.Errorf("equipmentNo is required when fromAddress is empty")
	}
	return value, nil
}

func validateAmountRange(amount string, quote *QuoteData) error {
	input, err := decimal.NewFromString(amount)
	if err != nil {
		return fmt.Errorf("invalid fromAmount: %w", err)
	}
	minimum := decimal.Zero
	if quote.DepositMin.String() != "" {
		minimum, err = decimal.NewFromString(quote.DepositMin.String())
		if err != nil {
			return fmt.Errorf("invalid Bridgers depositMin: %w", err)
		}
		minimum = minimum.Shift(int32(quote.FromTokenDecimal))
	}
	maximum := decimal.Zero
	if quote.DepositMax.String() != "" {
		maximum, err = decimal.NewFromString(quote.DepositMax.String())
		if err != nil {
			return fmt.Errorf("invalid Bridgers depositMax: %w", err)
		}
		maximum = maximum.Shift(int32(quote.FromTokenDecimal))
	}
	if input.LessThan(minimum) || maximum.GreaterThan(decimal.Zero) && input.GreaterThan(maximum) {
		return &AmountRangeError{
			Amount:   amount,
			Minimum:  minimum.StringFixed(0),
			Maximum:  maximum.StringFixed(0),
			Decimals: quote.FromTokenDecimal,
		}
	}
	return nil
}

func minimumOutput(amount string, slippage decimal.Decimal) (string, error) {
	value, ok := new(big.Int).SetString(amount, 10)
	if !ok || value.Sign() <= 0 {
		return "", fmt.Errorf("Bridgers amountOutMin must be a positive raw integer")
	}
	minimum := decimal.NewFromBigInt(value, 0).
		Mul(decimal.NewFromInt(100).Sub(slippage)).
		Div(decimal.NewFromInt(100)).
		Truncate(0)
	if !minimum.GreaterThan(decimal.Zero) {
		return "", fmt.Errorf("slippage makes amountOutMin zero")
	}
	return minimum.StringFixed(0), nil
}

func standardTransaction(raw json.RawMessage, contractAddress string) (*dexmodel.DexTx, error) {
	var transaction EVMTransaction
	if err := json.Unmarshal(raw, &transaction); err != nil {
		return nil, fmt.Errorf("decode Bridgers EVM txData: %w", err)
	}
	if !common.IsHexAddress(transaction.To) || !strings.HasPrefix(transaction.Data, "0x") || len(transaction.Data) <= 2 {
		return nil, fmt.Errorf("Bridgers returned a non-EVM transaction format")
	}
	if _, err := hex.DecodeString(transaction.Data[2:]); err != nil {
		return nil, fmt.Errorf("Bridgers returned invalid EVM calldata: %w", err)
	}
	if !common.IsHexAddress(contractAddress) || !strings.EqualFold(transaction.To, contractAddress) {
		return nil, fmt.Errorf("Bridgers quote contract and swap target differ")
	}
	value, err := decimalValue(transaction.Value.String())
	if err != nil {
		return nil, fmt.Errorf("invalid Bridgers transaction value: %w", err)
	}
	return &dexmodel.DexTx{To: transaction.To, Data: transaction.Data, Value: value}, nil
}

func decimalValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	base := 10
	if strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X") {
		base = 16
		value = value[2:]
	}
	parsed, ok := new(big.Int).SetString(value, base)
	if !ok || parsed.Sign() < 0 {
		return "", fmt.Errorf("invalid unsigned integer")
	}
	if parsed.Sign() == 0 {
		return "", nil
	}
	return parsed.String(), nil
}

func standardApproval(in *dexmodel.DexQuoteIn, contractAddress, txTarget string) (*dexmodel.DexApproval, []string) {
	trusted := []string{txTarget}
	if isNativeToken(in.FromToken) {
		return nil, trusted
	}
	return &dexmodel.DexApproval{
		Token:   in.FromToken,
		Spender: contractAddress,
		Amount:  in.FromAmount,
	}, trusted
}

func routeID(in *dexmodel.DexQuoteIn, quote *QuoteData, tx *dexmodel.DexTx) string {
	payload, _ := json.Marshal(struct {
		Channel     string          `json:"channel"`
		FromChain   string          `json:"fromChain"`
		ToChain     string          `json:"toChain"`
		FromToken   string          `json:"fromToken"`
		ToToken     string          `json:"toToken"`
		FromAddress string          `json:"fromAddress"`
		ToAddress   string          `json:"toAddress"`
		FromAmount  string          `json:"fromAmount"`
		AmountOut   string          `json:"amountOut"`
		Slippage    string          `json:"slippage"`
		Tx          *dexmodel.DexTx `json:"tx"`
	}{Channel, in.FromChain, in.ToChain, in.FromToken, in.ToToken, in.FromAddress, in.ToAddress, in.FromAmount, quote.AmountOutMin, in.Slippage, tx})
	hash := sha256.Sum256(payload)
	return Channel + "_" + hex.EncodeToString(hash[:])
}

func providerName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Bridgers"
	}
	return name
}

func isNativeToken(token string) bool {
	return strings.EqualFold(strings.TrimSpace(token), nativeToken)
}

func percentFromFraction(value string) float64 {
	fraction, err := decimal.NewFromString(value)
	if err != nil {
		return 0
	}
	result, _ := fraction.Mul(decimal.NewFromInt(100)).Float64()
	return result
}

func decimalFloat(value decimal.Decimal) float64 {
	result, _ := value.Float64()
	return result
}

func standardStatus(status string) (dexmodel.DexStatus, string) {
	switch status {
	case "receive_complete":
		return dexmodel.DexStatusSucceeded, ""
	case "refund_complete":
		return dexmodel.DexStatusRefunded, ""
	case "wait_deposit_send_fail", "timeout":
		return dexmodel.DexStatusFailed, ""
	case "wait_deposit_send", "wait_exchange_push", "wait_exchange_return", "wait_exchange_return_success",
		"wait_receive_send", "wait_enough_send", "wait_receive_confirm", "wait_refund_send", "wait_refund_confirm", "ERROR":
		return dexmodel.DexStatusPending, ""
	case "wait_for_information":
		return dexmodel.DexStatusPending, "Bridgers requires support review"
	default:
		return dexmodel.DexStatusPending, fmt.Sprintf("unmapped Bridgers status: %s", status)
	}
}

func (c *Client) resolveCoinCodes(ctx context.Context, in *dexmodel.DexQuoteIn, fromChain, toChain string) (string, string, error) {
	fromCode := strings.TrimSpace(in.FromCoinCode)
	toCode := strings.TrimSpace(in.ToCoinCode)
	type result struct {
		from   bool
		symbol string
		err    error
	}
	results := make(chan result, 2)
	pending := 0
	if fromCode == "" {
		pending++
		go func() {
			symbol, err := c.tokenSymbol(ctx, fromChain, in.FromToken)
			results <- result{from: true, symbol: symbol, err: err}
		}()
	}
	if toCode == "" {
		pending++
		go func() {
			symbol, err := c.tokenSymbol(ctx, toChain, in.ToToken)
			results <- result{symbol: symbol, err: err}
		}()
	}
	for i := 0; i < pending; i++ {
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		case current := <-results:
			if current.err != nil {
				return "", "", current.err
			}
			if current.from {
				fromCode = current.symbol
			} else {
				toCode = current.symbol
			}
		}
	}
	return fromCode, toCode, nil
}

func (c *Client) tokenSymbol(ctx context.Context, chain, address string) (string, error) {
	tokens, err := c.cachedTokens(ctx, chain)
	if err != nil {
		return "", err
	}
	for _, token := range tokens {
		matches := token.Address == address || token.AddressAlias == address
		if isEVMProviderChain(chain) {
			matches = matches || strings.EqualFold(token.Address, address) || strings.EqualFold(token.AddressAlias, address)
		}
		if matches {
			if token.Symbol == "" {
				break
			}
			return token.Symbol, nil
		}
	}
	return "", fmt.Errorf("Bridgers token %q not found on %s", address, chain)
}

func (c *Client) cachedTokens(ctx context.Context, chain string) ([]Token, error) {
	now := time.Now()
	c.tokenMu.Lock()
	if cached, ok := c.tokenCache[chain]; ok && now.Before(cached.expiresAt) {
		tokens := append([]Token(nil), cached.tokens...)
		c.tokenMu.Unlock()
		return tokens, nil
	}
	if loading, ok := c.tokenLoads[chain]; ok {
		c.tokenMu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-loading.done:
			return append([]Token(nil), loading.tokens...), loading.err
		}
	}
	loading := &tokenLoad{done: make(chan struct{})}
	c.tokenLoads[chain] = loading
	c.tokenMu.Unlock()

	response, err := c.Tokens(ctx, chain)
	var tokens []Token
	if err == nil {
		tokens = append([]Token(nil), response.Data.Tokens...)
	}

	c.tokenMu.Lock()
	loading.tokens = tokens
	loading.err = err
	if err == nil {
		c.tokenCache[chain] = tokenCacheEntry{tokens: tokens, expiresAt: time.Now().Add(c.config.TokenCacheTTL)}
	}
	delete(c.tokenLoads, chain)
	close(loading.done)
	c.tokenMu.Unlock()
	return append([]Token(nil), tokens...), err
}

func tokensForChain(tokens []Token, chain string) []Token {
	if chain == "" {
		return append([]Token(nil), tokens...)
	}
	filtered := make([]Token, 0, len(tokens))
	for _, token := range tokens {
		if strings.EqualFold(strings.TrimSpace(token.Chain), chain) {
			filtered = append(filtered, token)
		}
	}
	return filtered
}
