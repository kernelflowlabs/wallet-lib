package univ3

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	gethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/kernelflowlabs/wallet-lib/common/dexmodel"
	"github.com/kernelflowlabs/wallet-lib/signing"
)

const (
	poolCacheTTL            = 30 * time.Second
	maxFeeTiers             = 6
	maxQuoteBaseTokens      = 6
	maxQuoteCandidates      = 128
	maxPlatformFeeBips      = uint64(100)
	defaultEstimatedTime    = int64(30)
	maxPoolRPCConcurrency   = 8
	maxQuoterRPCConcurrency = 8
)

var defaultFeeTiers = []uint32{100, 500, 3000, 10000}

var (
	parsedFactoryV3 abi.ABI
	parsedQuoterV1  abi.ABI
)

func init() {
	parsedFactoryV3 = mustParseContractABI("Factory", abiFactoryV3)
	parsedQuoterV1 = mustParseContractABI("Quoter V1", abiQuoterV1)
}

func mustParseContractABI(name, raw string) abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(raw))
	if err != nil {
		panic(fmt.Sprintf("univ3: parse %s ABI: %v", name, err))
	}
	return parsed
}

type poolCacheEntry struct {
	exists    bool
	expiresAt time.Time
}

type poolFlight struct {
	done   chan struct{}
	exists bool
	err    error
}

type Client struct {
	config          Config
	factory         common.Address
	swapRouter      common.Address
	quoter          common.Address
	wrappedNative   common.Address
	feeTiers        []uint32
	quoteBaseTokens []common.Address
	rpc             *gethrpc.Client
	eth             *ethclient.Client
	now             func() time.Time

	deployMu       sync.RWMutex
	deployChecked  bool
	poolMu         sync.Mutex
	poolCache      map[string]poolCacheEntry
	poolFlights    map[string]*poolFlight
	poolRPCSlots   chan struct{}
	quoterRPCSlots chan struct{}
}

func NewClient(config Config) (*Client, error) {
	normalized, factory, router, quoter, wrapped, tiers, bases, err := normalizeClientConfig(config)
	if err != nil {
		return nil, err
	}
	rpcClient, err := gethrpc.DialContext(context.Background(), normalized.RPC)
	if err != nil {
		return nil, fmt.Errorf("univ3: dial RPC: %w", err)
	}
	return &Client{
		config:          normalized,
		factory:         factory,
		swapRouter:      router,
		quoter:          quoter,
		wrappedNative:   wrapped,
		feeTiers:        tiers,
		quoteBaseTokens: bases,
		rpc:             rpcClient,
		eth:             ethclient.NewClient(rpcClient),
		now:             time.Now,
		poolCache:       make(map[string]poolCacheEntry),
		poolFlights:     make(map[string]*poolFlight),
		poolRPCSlots:    make(chan struct{}, maxPoolRPCConcurrency),
		quoterRPCSlots:  make(chan struct{}, maxQuoterRPCConcurrency),
	}, nil
}

func normalizeClientConfig(config Config) (Config, common.Address, common.Address, common.Address, common.Address, []uint32, []common.Address, error) {
	zero := common.Address{}
	config.ChainName = strings.TrimSpace(config.ChainName)
	config.RPC = strings.TrimSpace(config.RPC)
	if config.ChainName == "" {
		return Config{}, zero, zero, zero, zero, nil, nil, fmt.Errorf("univ3: chainName is required")
	}
	if config.ChainID == 0 {
		return Config{}, zero, zero, zero, zero, nil, nil, fmt.Errorf("univ3: chainId must be greater than zero")
	}
	if err := validateClientRPCURL(config.RPC); err != nil {
		return Config{}, zero, zero, zero, zero, nil, nil, err
	}
	factory, err := requiredClientAddress("factory", config.Factory)
	if err != nil {
		return Config{}, zero, zero, zero, zero, nil, nil, err
	}
	router, err := requiredClientAddress("swapRouter", config.SwapRouter)
	if err != nil {
		return Config{}, zero, zero, zero, zero, nil, nil, err
	}
	quoter, err := requiredClientAddress("quoter", config.Quoter)
	if err != nil {
		return Config{}, zero, zero, zero, zero, nil, nil, err
	}
	wrapped, err := requiredClientAddress("wrappedNative", config.WrappedNative)
	if err != nil {
		return Config{}, zero, zero, zero, zero, nil, nil, err
	}
	if factory == router || factory == quoter || router == quoter {
		return Config{}, zero, zero, zero, zero, nil, nil, fmt.Errorf("univ3: factory, swapRouter, and quoter must be distinct")
	}

	tiers := append([]uint32(nil), config.FeeTiers...)
	if len(tiers) == 0 {
		tiers = append([]uint32(nil), defaultFeeTiers...)
	}
	if len(tiers) > maxFeeTiers {
		return Config{}, zero, zero, zero, zero, nil, nil, fmt.Errorf("univ3: at most %d fee tiers are allowed", maxFeeTiers)
	}
	seenFees := make(map[uint32]struct{}, len(tiers))
	normalizedTiers := make([]uint32, 0, len(tiers))
	for i, fee := range tiers {
		if fee > MaxUint24 {
			return Config{}, zero, zero, zero, zero, nil, nil, fmt.Errorf("univ3: feeTiers[%d]=%d exceeds uint24", i, fee)
		}
		if _, ok := seenFees[fee]; ok {
			continue
		}
		seenFees[fee] = struct{}{}
		normalizedTiers = append(normalizedTiers, fee)
	}
	if len(normalizedTiers) == 0 {
		return Config{}, zero, zero, zero, zero, nil, nil, fmt.Errorf("univ3: at least one fee tier is required")
	}
	sort.Slice(normalizedTiers, func(i, j int) bool { return normalizedTiers[i] < normalizedTiers[j] })

	if len(config.QuoteBaseTokens) > maxQuoteBaseTokens {
		return Config{}, zero, zero, zero, zero, nil, nil, fmt.Errorf("univ3: at most %d quote base tokens are allowed", maxQuoteBaseTokens)
	}
	seenBases := make(map[common.Address]struct{}, len(config.QuoteBaseTokens)+1)
	bases := make([]common.Address, 0, len(config.QuoteBaseTokens)+1)
	normalizedBases := make([]string, 0, len(config.QuoteBaseTokens))
	for i, raw := range config.QuoteBaseTokens {
		base, err := requiredClientAddress(fmt.Sprintf("quoteBaseTokens[%d]", i), raw)
		if err != nil {
			return Config{}, zero, zero, zero, zero, nil, nil, err
		}
		if _, ok := seenBases[base]; ok {
			continue
		}
		seenBases[base] = struct{}{}
		bases = append(bases, base)
		normalizedBases = append(normalizedBases, base.Hex())
	}
	if _, ok := seenBases[wrapped]; !ok {
		bases = append(bases, wrapped)
	}
	candidateLimit := len(normalizedTiers) + len(bases)*len(normalizedTiers)*len(normalizedTiers)
	if candidateLimit > maxQuoteCandidates {
		return Config{}, zero, zero, zero, zero, nil, nil, fmt.Errorf("univ3: configuration can create %d candidates; maximum is %d", candidateLimit, maxQuoteCandidates)
	}

	if config.DeadlineTTL == 0 {
		config.DeadlineTTL = DefaultDeadlineTTL
	}
	if config.DeadlineTTL < time.Second {
		return Config{}, zero, zero, zero, zero, nil, nil, fmt.Errorf("univ3: deadlineTTL must be at least one second")
	}
	config.Factory = factory.Hex()
	config.SwapRouter = router.Hex()
	config.Quoter = quoter.Hex()
	config.WrappedNative = wrapped.Hex()
	config.FeeTiers = append([]uint32(nil), normalizedTiers...)
	config.QuoteBaseTokens = normalizedBases
	return config, factory, router, quoter, wrapped, normalizedTiers, bases, nil
}

func validateClientRPCURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("univ3: RPC is required")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("univ3: invalid RPC URL %q", raw)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "ws", "wss":
		return nil
	default:
		return fmt.Errorf("univ3: unsupported RPC URL scheme %q", u.Scheme)
	}
}

func requiredClientAddress(name, raw string) (common.Address, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) != 42 || !common.IsHexAddress(raw) {
		return common.Address{}, fmt.Errorf("univ3: invalid %s address %q", name, raw)
	}
	address := common.HexToAddress(raw)
	if address == (common.Address{}) {
		return common.Address{}, fmt.Errorf("univ3: %s address must not be zero", name)
	}
	return address, nil
}

func (c *Client) Close() {
	if c != nil && c.rpc != nil {
		c.rpc.Close()
	}
}

func (c *Client) Config() Config {
	if c == nil {
		return Config{}
	}
	out := c.config
	out.FeeTiers = append([]uint32(nil), c.config.FeeTiers...)
	out.QuoteBaseTokens = append([]string(nil), c.config.QuoteBaseTokens...)
	return out
}

func (c *Client) IsConfiguredRouter(address string) bool {
	if c == nil || len(strings.TrimSpace(address)) != 42 || !common.IsHexAddress(address) {
		return false
	}
	return common.HexToAddress(address) == c.swapRouter
}

type quoteRequest struct {
	amountIn     *big.Int
	tokenIn      common.Address
	tokenOut     common.Address
	inputNative  bool
	outputNative bool
	recipient    common.Address
	slippage     *big.Rat
	feeBips      uint64
	feeReceiver  common.Address
}

type routeCandidate struct {
	tokens    []common.Address
	fees      []uint32
	path      []byte
	amountOut *big.Int
}

func (c *Client) Quote(ctx context.Context, in *dexmodel.DexQuoteIn) (*dexmodel.DexQuoteOut, error) {
	if c == nil || c.eth == nil {
		return nil, fmt.Errorf("univ3: client is not initialized")
	}
	if ctx == nil {
		return nil, fmt.Errorf("univ3: context is nil")
	}
	req, err := c.validateQuote(in)
	if err != nil {
		return nil, err
	}
	if err := c.ensureDeployment(ctx); err != nil {
		return nil, err
	}

	out := &dexmodel.DexQuoteOut{Channel: Channel, Routes: []*dexmodel.DexRoute{}}
	candidates := c.candidateRoutes(req.tokenIn, req.tokenOut)
	poolStates, err := c.resolveCandidatePools(ctx, candidates)
	if err != nil {
		return nil, err
	}
	viable := make([]routeCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidatePoolsExist(candidate, poolStates) {
			viable = append(viable, candidate)
		}
	}
	if len(viable) == 0 {
		return out, nil
	}

	best, err := c.bestQuotedCandidate(ctx, viable, req.amountIn)
	if err != nil {
		return nil, err
	}
	if best == nil {
		return out, nil
	}

	grossMinimum := applyQuoteSlippage(best.amountOut, req.slippage)
	if grossMinimum.Sign() <= 0 {
		return nil, fmt.Errorf("univ3: slippage makes amountOutMinimum zero")
	}
	netAmountOut := deductPlatformFee(best.amountOut, req.feeBips)
	netMinimum := deductPlatformFee(grossMinimum, req.feeBips)
	if netAmountOut.Sign() <= 0 || netMinimum.Sign() <= 0 {
		return nil, fmt.Errorf("univ3: platform fee makes the user output zero")
	}
	quotedAt := c.now()
	deadline := big.NewInt(quotedAt.Add(c.config.DeadlineTTL).Unix())
	if deadline.Cmp(big.NewInt(quotedAt.Unix())) <= 0 {
		return nil, fmt.Errorf("univ3: invalid transaction deadline")
	}
	calldata, method, err := c.buildSwapCalldata(req, best, grossMinimum, deadline)
	if err != nil {
		return nil, err
	}
	tx := &dexmodel.DexTx{To: c.swapRouter.Hex(), Data: "0x" + hex.EncodeToString(calldata)}
	if req.inputNative {
		tx.Value = req.amountIn.String()
	}

	slippageFloat, _ := req.slippage.Float64()
	route := &dexmodel.DexRoute{
		RouteId:           c.routeID(method, req, best, grossMinimum, deadline),
		Name:              fmt.Sprintf("%s Uniswap V3", c.config.ChainName),
		AmountOut:         netAmountOut.String(),
		AmountOutMin:      netMinimum.String(),
		Slippage:          slippageFloat,
		SuggestedSlippage: slippageFloat,
		EstimatedTime:     defaultEstimatedTime,
		NeedBuild:         false,
		TxData:            tx,
		RouteTags:         []string{"MAX_OUTPUT"},
		ExpiresAt:         deadline.Int64(),
	}
	if !req.inputNative {
		route.ApprovalData = &dexmodel.DexApproval{
			Token:   req.tokenIn.Hex(),
			Spender: c.swapRouter.Hex(),
			Amount:  req.amountIn.String(),
		}
		route.TrustedSpenders = []string{c.swapRouter.Hex()}
	}
	out.Routes = append(out.Routes, route)
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("univ3: quote context ended before completion: %w", err)
	}
	return out, nil
}

type candidateQuoteResult struct {
	index     int
	amountOut *big.Int
	err       error
}

func (c *Client) bestQuotedCandidate(ctx context.Context, viable []routeCandidate, amountIn *big.Int) (*routeCandidate, error) {
	results := make(chan candidateQuoteResult, len(viable))
	for index := range viable {
		go func(index int) {
			select {
			case c.quoterRPCSlots <- struct{}{}:
				defer func() { <-c.quoterRPCSlots }()
			case <-ctx.Done():
				results <- candidateQuoteResult{index: index, err: ctx.Err()}
				return
			}
			amountOut, err := c.quoteCandidate(ctx, &viable[index], amountIn)
			results <- candidateQuoteResult{index: index, amountOut: amountOut, err: err}
		}(index)
	}

	amounts := make([]*big.Int, len(viable))
	errorsByIndex := make([]error, len(viable))
	for range viable {
		result := <-results
		amounts[result.index] = result.amountOut
		errorsByIndex[result.index] = result.err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("univ3: Quoter calls canceled: %w", err)
	}

	bestIndex := -1
	var firstInfrastructureError error
	for index := range viable {
		if err := errorsByIndex[index]; err != nil {
			if !isExpectedQuoterFailure(err) && firstInfrastructureError == nil {
				firstInfrastructureError = err
			}
			continue
		}
		if amounts[index] == nil || amounts[index].Sign() <= 0 {
			continue
		}
		viable[index].amountOut = amounts[index]
		if bestIndex < 0 || amounts[index].Cmp(viable[bestIndex].amountOut) > 0 {
			bestIndex = index
		}
	}
	if bestIndex < 0 {
		if firstInfrastructureError != nil {
			return nil, fmt.Errorf("univ3: all usable candidates failed and at least one Quoter RPC response was invalid: %w", firstInfrastructureError)
		}
		return nil, nil
	}
	return &viable[bestIndex], nil
}

func isExpectedQuoterFailure(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "execution reverted") ||
		strings.Contains(message, "revert data") ||
		strings.Contains(message, "no liquidity") ||
		strings.Contains(message, "insufficient liquidity")
}

func (c *Client) validateQuote(in *dexmodel.DexQuoteIn) (*quoteRequest, error) {
	if in == nil {
		return nil, fmt.Errorf("univ3: quote input is nil")
	}
	if !c.matchesChain(in.FromChain) || !c.matchesChain(in.ToChain) {
		return nil, fmt.Errorf("univ3: only same-chain %s/%d quotes are supported", c.config.ChainName, c.config.ChainID)
	}
	rawAmount := strings.TrimSpace(in.FromAmount)
	if len(rawAmount) == 0 || len(rawAmount) > 78 {
		return nil, fmt.Errorf("univ3: fromAmount must be a positive uint256 base-10 integer")
	}
	amountIn, ok := new(big.Int).SetString(rawAmount, 10)
	if !ok || amountIn.Sign() <= 0 || amountIn.BitLen() > 256 {
		return nil, fmt.Errorf("univ3: fromAmount must be a positive uint256 base-10 integer")
	}
	inputNative := isClientNative(in.FromToken)
	outputNative := isClientNative(in.ToToken)
	if inputNative && outputNative {
		return nil, fmt.Errorf("univ3: native-to-native swap is not supported")
	}
	tokenIn, err := c.swapToken("fromToken", in.FromToken, inputNative)
	if err != nil {
		return nil, err
	}
	tokenOut, err := c.swapToken("toToken", in.ToToken, outputNative)
	if err != nil {
		return nil, err
	}
	if tokenIn == tokenOut {
		return nil, fmt.Errorf("univ3: fromToken and toToken resolve to the same address")
	}
	if _, err := requiredClientAddress("fromAddress", in.FromAddress); err != nil {
		return nil, err
	}
	recipient, err := requiredClientAddress("toAddress", in.ToAddress)
	if err != nil {
		return nil, err
	}
	slippage, err := parseProtectedSlippage(in.Slippage)
	if err != nil {
		return nil, err
	}
	feeBips, feeReceiver, err := parsePlatformFee(in.FeeRate, in.FeeReceiver)
	if err != nil {
		return nil, err
	}
	return &quoteRequest{
		amountIn: amountIn, tokenIn: tokenIn, tokenOut: tokenOut,
		inputNative: inputNative, outputNative: outputNative,
		recipient: recipient, slippage: slippage,
		feeBips: feeBips, feeReceiver: feeReceiver,
	}, nil
}

func (c *Client) matchesChain(raw string) bool {
	raw = strings.TrimSpace(raw)
	return strings.EqualFold(raw, c.config.ChainName) || raw == strconv.FormatUint(c.config.ChainID, 10)
}

func (c *Client) swapToken(name, raw string, native bool) (common.Address, error) {
	if native {
		return c.wrappedNative, nil
	}
	return requiredClientAddress(name, raw)
}

func isClientNative(raw string) bool {
	raw = strings.TrimSpace(raw)
	return strings.EqualFold(raw, signing.MagicContactAddressForNative) ||
		strings.EqualFold(raw, signing.MagicAddressForZeroEVM)
}

func parseProtectedSlippage(raw string) (*big.Rat, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 64 || strings.Contains(raw, "/") {
		return nil, fmt.Errorf("univ3: slippage must be an explicit percentage in (0,100)")
	}
	slippage, ok := new(big.Rat).SetString(raw)
	if !ok || slippage.Sign() <= 0 || slippage.Cmp(big.NewRat(100, 1)) >= 0 {
		return nil, fmt.Errorf("univ3: slippage must be an explicit percentage in (0,100)")
	}
	return slippage, nil
}

func parsePlatformFee(rawRate, rawReceiver string) (uint64, common.Address, error) {
	rawRate = strings.TrimSpace(rawRate)
	rawReceiver = strings.TrimSpace(rawReceiver)
	if rawRate == "" {
		return 0, common.Address{}, nil
	}
	if len(rawRate) > 64 || strings.Contains(rawRate, "/") {
		return 0, common.Address{}, fmt.Errorf("univ3: feeRate must be a decimal percentage")
	}
	rate, ok := new(big.Rat).SetString(rawRate)
	if !ok || rate.Sign() < 0 {
		return 0, common.Address{}, fmt.Errorf("univ3: feeRate must be a non-negative decimal percentage")
	}
	if rate.Sign() == 0 {
		return 0, common.Address{}, nil
	}
	bipsRat := new(big.Rat).Mul(rate, big.NewRat(100, 1))
	if !bipsRat.IsInt() {
		return 0, common.Address{}, fmt.Errorf("univ3: feeRate must be exactly representable in whole basis points")
	}
	bips := bipsRat.Num()
	if !bips.IsUint64() || bips.Uint64() > maxPlatformFeeBips {
		return 0, common.Address{}, fmt.Errorf("univ3: feeRate exceeds the SwapRouter maximum of 1%%")
	}
	receiver, err := requiredClientAddress("feeReceiver", rawReceiver)
	if err != nil {
		return 0, common.Address{}, err
	}
	return bips.Uint64(), receiver, nil
}

func applyQuoteSlippage(amountOut *big.Int, slippage *big.Rat) *big.Int {
	factor := new(big.Rat).Sub(big.NewRat(100, 1), slippage)
	value := new(big.Rat).Mul(new(big.Rat).SetInt(amountOut), factor)
	value.Quo(value, big.NewRat(100, 1))
	return new(big.Int).Quo(value.Num(), value.Denom())
}

func deductPlatformFee(amount *big.Int, feeBips uint64) *big.Int {
	if feeBips == 0 {
		return new(big.Int).Set(amount)
	}
	fee := new(big.Int).Mul(amount, new(big.Int).SetUint64(feeBips))
	fee.Quo(fee, big.NewInt(10_000))
	return new(big.Int).Sub(new(big.Int).Set(amount), fee)
}

func (c *Client) candidateRoutes(tokenIn, tokenOut common.Address) []routeCandidate {
	candidates := make([]routeCandidate, 0, len(c.feeTiers)+len(c.quoteBaseTokens)*len(c.feeTiers)*len(c.feeTiers))
	for _, fee := range c.feeTiers {
		tokens := []common.Address{tokenIn, tokenOut}
		fees := []uint32{fee}
		candidates = append(candidates, routeCandidate{tokens: tokens, fees: fees, path: encodeV3Path(tokens, fees)})
	}
	for _, base := range c.quoteBaseTokens {
		if base == tokenIn || base == tokenOut || base == (common.Address{}) {
			continue
		}
		for _, firstFee := range c.feeTiers {
			for _, secondFee := range c.feeTiers {
				tokens := []common.Address{tokenIn, base, tokenOut}
				fees := []uint32{firstFee, secondFee}
				candidates = append(candidates, routeCandidate{tokens: tokens, fees: fees, path: encodeV3Path(tokens, fees)})
			}
		}
	}
	return candidates
}

func encodeV3Path(tokens []common.Address, fees []uint32) []byte {
	if len(tokens) != len(fees)+1 || len(fees) == 0 {
		return nil
	}
	path := make([]byte, 0, common.AddressLength*len(tokens)+3*len(fees))
	path = append(path, tokens[0].Bytes()...)
	for i, fee := range fees {
		var encoded [4]byte
		binary.BigEndian.PutUint32(encoded[:], fee)
		path = append(path, encoded[1:]...)
		path = append(path, tokens[i+1].Bytes()...)
	}
	return path
}

func decodeV3Path(path []byte) ([]common.Address, []uint32, error) {
	if len(path) < 43 || (len(path)-20)%23 != 0 {
		return nil, nil, fmt.Errorf("univ3: invalid V3 path length %d", len(path))
	}
	tokens := []common.Address{common.BytesToAddress(path[:20])}
	fees := make([]uint32, 0, (len(path)-20)/23)
	offset := 20
	for offset < len(path) {
		fee := uint32(path[offset])<<16 | uint32(path[offset+1])<<8 | uint32(path[offset+2])
		next := common.BytesToAddress(path[offset+3 : offset+23])
		if fee > MaxUint24 || tokens[len(tokens)-1] == (common.Address{}) || next == (common.Address{}) || next == tokens[len(tokens)-1] {
			return nil, nil, fmt.Errorf("univ3: V3 path contains an invalid hop")
		}
		fees = append(fees, fee)
		tokens = append(tokens, next)
		offset += 23
	}
	return tokens, fees, nil
}

type poolRequest struct {
	tokenA common.Address
	tokenB common.Address
	fee    uint32
}

type poolResult struct {
	key    string
	exists bool
	err    error
}

func (c *Client) resolveCandidatePools(ctx context.Context, candidates []routeCandidate) (map[string]bool, error) {
	unique := make(map[string]poolRequest)
	for _, candidate := range candidates {
		if len(candidate.tokens) != len(candidate.fees)+1 || len(candidate.path) == 0 {
			return nil, fmt.Errorf("univ3: malformed route candidate")
		}
		for index, fee := range candidate.fees {
			request := poolRequest{tokenA: candidate.tokens[index], tokenB: candidate.tokens[index+1], fee: fee}
			unique[poolKey(request.tokenA, request.tokenB, fee)] = request
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("univ3: pool discovery canceled: %w", err)
	}
	keys := make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	results := make(chan poolResult, len(keys))
	for _, key := range keys {
		request := unique[key]
		go func(key string, request poolRequest) {
			exists, err := c.poolExists(ctx, request.tokenA, request.tokenB, request.fee)
			results <- poolResult{key: key, exists: exists, err: err}
		}(key, request)
	}

	states := make(map[string]bool, len(keys))
	var firstError error
	for range keys {
		result := <-results
		if result.err != nil {
			if firstError == nil {
				firstError = result.err
			}
			continue
		}
		states[result.key] = result.exists
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("univ3: pool discovery canceled: %w", err)
	}
	if firstError != nil {
		for _, candidate := range candidates {
			if candidatePoolsExist(candidate, states) {
				return states, nil
			}
		}
		return nil, fmt.Errorf("univ3: no usable route after pool discovery failure: %w", firstError)
	}
	return states, nil
}

func candidatePoolsExist(candidate routeCandidate, states map[string]bool) bool {
	if len(candidate.tokens) != len(candidate.fees)+1 || len(candidate.path) == 0 {
		return false
	}
	for i, fee := range candidate.fees {
		if !states[poolKey(candidate.tokens[i], candidate.tokens[i+1], fee)] {
			return false
		}
	}
	return true
}

func poolKey(tokenA, tokenB common.Address, fee uint32) string {
	left, right := strings.ToLower(tokenA.Hex()), strings.ToLower(tokenB.Hex())
	if left > right {
		left, right = right, left
	}
	return fmt.Sprintf("%s:%s:%d", left, right, fee)
}

func (c *Client) poolExists(ctx context.Context, tokenA, tokenB common.Address, fee uint32) (bool, error) {
	key := poolKey(tokenA, tokenB, fee)
	now := c.now()
	c.poolMu.Lock()
	entry, ok := c.poolCache[key]
	if ok && now.Before(entry.expiresAt) {
		c.poolMu.Unlock()
		return entry.exists, nil
	}
	if flight, ok := c.poolFlights[key]; ok {
		c.poolMu.Unlock()
		select {
		case <-flight.done:
			return flight.exists, flight.err
		case <-ctx.Done():
			return false, fmt.Errorf("univ3: wait for Factory.getPool(%s,%s,%d): %w", tokenA.Hex(), tokenB.Hex(), fee, ctx.Err())
		}
	}
	flight := &poolFlight{done: make(chan struct{})}
	c.poolFlights[key] = flight
	c.poolMu.Unlock()

	var exists bool
	var err error
	select {
	case c.poolRPCSlots <- struct{}{}:
		exists, err = c.fetchPoolExists(ctx, tokenA, tokenB, fee)
		<-c.poolRPCSlots
	case <-ctx.Done():
		err = fmt.Errorf("univ3: wait to call Factory.getPool(%s,%s,%d): %w", tokenA.Hex(), tokenB.Hex(), fee, ctx.Err())
	}
	c.poolMu.Lock()
	flight.exists = exists
	flight.err = err
	if err == nil {
		c.poolCache[key] = poolCacheEntry{exists: exists, expiresAt: now.Add(poolCacheTTL)}
	}
	delete(c.poolFlights, key)
	close(flight.done)
	c.poolMu.Unlock()
	return exists, err
}

func (c *Client) fetchPoolExists(ctx context.Context, tokenA, tokenB common.Address, fee uint32) (bool, error) {
	data, err := parsedFactoryV3.Pack("getPool", tokenA, tokenB, new(big.Int).SetUint64(uint64(fee)))
	if err != nil {
		return false, fmt.Errorf("univ3: pack Factory.getPool: %w", err)
	}
	raw, err := c.eth.CallContract(ctx, ethereum.CallMsg{To: &c.factory, Data: data}, nil)
	if err != nil {
		return false, fmt.Errorf("univ3: Factory.getPool(%s,%s,%d): %w", tokenA.Hex(), tokenB.Hex(), fee, err)
	}
	values, err := parsedFactoryV3.Unpack("getPool", raw)
	if err != nil || len(values) != 1 {
		if err == nil {
			err = fmt.Errorf("returned %d values", len(values))
		}
		return false, fmt.Errorf("univ3: decode Factory.getPool response: %w", err)
	}
	pool, ok := values[0].(common.Address)
	if !ok {
		return false, fmt.Errorf("univ3: Factory.getPool returned %T", values[0])
	}
	return pool != (common.Address{}), nil
}

func (c *Client) quoteCandidate(ctx context.Context, candidate *routeCandidate, amountIn *big.Int) (*big.Int, error) {
	var (
		method string
		data   []byte
		err    error
	)
	if len(candidate.fees) == 1 {
		method = "quoteExactInputSingle"
		data, err = parsedQuoterV1.Pack(method, candidate.tokens[0], candidate.tokens[1], new(big.Int).SetUint64(uint64(candidate.fees[0])), amountIn, new(big.Int))
	} else {
		method = "quoteExactInput"
		data, err = parsedQuoterV1.Pack(method, candidate.path, amountIn)
	}
	if err != nil {
		return nil, fmt.Errorf("univ3: pack Quoter.%s: %w", method, err)
	}
	raw, err := c.eth.CallContract(ctx, ethereum.CallMsg{To: &c.quoter, Data: data}, nil)
	if err != nil {
		return nil, fmt.Errorf("univ3: Quoter.%s: %w", method, err)
	}
	values, err := parsedQuoterV1.Unpack(method, raw)
	if err != nil || len(values) != 1 {
		if err == nil {
			err = fmt.Errorf("returned %d values", len(values))
		}
		return nil, fmt.Errorf("univ3: decode Quoter.%s response: %w", method, err)
	}
	amountOut, ok := values[0].(*big.Int)
	if !ok || amountOut == nil || amountOut.Sign() <= 0 {
		return nil, fmt.Errorf("univ3: Quoter.%s returned an invalid amount", method)
	}
	return new(big.Int).Set(amountOut), nil
}

func (c *Client) buildSwapCalldata(req *quoteRequest, candidate *routeCandidate, grossMinimum, deadline *big.Int) ([]byte, string, error) {
	if deadline == nil || deadline.Sign() <= 0 {
		return nil, "", fmt.Errorf("univ3: deadline is required")
	}
	if grossMinimum == nil || grossMinimum.Sign() <= 0 {
		return nil, "", fmt.Errorf("univ3: amountOutMinimum must be positive")
	}
	swapRecipient := req.recipient
	if req.outputNative || req.feeBips > 0 {
		swapRecipient = c.swapRouter
	}

	var (
		swapCall []byte
		method   string
		err      error
	)
	if len(candidate.fees) == 1 {
		method = "exactInputSingle"
		swapCall, err = encodeV1(&SwapInfo{
			TokenIn: candidate.tokens[0], TokenOut: candidate.tokens[1], Fee: candidate.fees[0],
			AmountIn: req.amountIn, AmountOutMinimum: grossMinimum, SqrtPriceLimitX96: new(big.Int),
		}, swapRecipient, deadline)
	} else {
		method = "exactInput"
		swapCall, err = encodeExactInputV1(candidate.path, swapRecipient, deadline, req.amountIn, grossMinimum)
	}
	if err != nil {
		return nil, "", fmt.Errorf("univ3: encode %s: %w", method, err)
	}

	calls := [][]byte{swapCall}
	if req.outputNative {
		paymentMethod := "unwrapWETH9"
		if req.feeBips > 0 {
			paymentMethod = "unwrapWETH9WithFee"
			payment, err := parsedV1.Pack(paymentMethod, grossMinimum, req.recipient, new(big.Int).SetUint64(req.feeBips), req.feeReceiver)
			if err != nil {
				return nil, "", fmt.Errorf("univ3: encode %s: %w", paymentMethod, err)
			}
			calls = append(calls, payment)
		} else {
			payment, err := parsedV1.Pack(paymentMethod, grossMinimum, req.recipient)
			if err != nil {
				return nil, "", fmt.Errorf("univ3: encode %s: %w", paymentMethod, err)
			}
			calls = append(calls, payment)
		}
		method += "+" + paymentMethod
	} else if req.feeBips > 0 {
		payment, err := parsedV1.Pack("sweepTokenWithFee", req.tokenOut, grossMinimum, req.recipient, new(big.Int).SetUint64(req.feeBips), req.feeReceiver)
		if err != nil {
			return nil, "", fmt.Errorf("univ3: encode sweepTokenWithFee: %w", err)
		}
		calls = append(calls, payment)
		method += "+sweepTokenWithFee"
	}
	if len(calls) == 1 {
		return calls[0], method, nil
	}
	multicall, err := parsedV1.Pack("multicall", calls)
	if err != nil {
		return nil, "", fmt.Errorf("univ3: encode multicall: %w", err)
	}
	return multicall, "multicall:" + method, nil
}

func encodeExactInputV1(path []byte, recipient common.Address, deadline, amountIn, amountOutMinimum *big.Int) ([]byte, error) {
	if recipient == (common.Address{}) || deadline == nil || deadline.Sign() <= 0 || amountIn == nil || amountIn.Sign() <= 0 || amountOutMinimum == nil || amountOutMinimum.Sign() <= 0 {
		return nil, fmt.Errorf("invalid exactInput parameters")
	}
	if _, _, err := decodeV3Path(path); err != nil {
		return nil, err
	}
	type exactInputParams struct {
		Path             []byte
		Recipient        common.Address
		Deadline         *big.Int
		AmountIn         *big.Int
		AmountOutMinimum *big.Int
	}
	return parsedV1.Pack("exactInput", exactInputParams{path, recipient, deadline, amountIn, amountOutMinimum})
}

func (c *Client) routeID(method string, req *quoteRequest, candidate *routeCandidate, grossMinimum, deadline *big.Int) string {
	payload := strings.Join([]string{
		strconv.FormatUint(c.config.ChainID, 10),
		strings.ToLower(c.swapRouter.Hex()),
		method,
		hex.EncodeToString(candidate.path),
		req.amountIn.String(),
		candidate.amountOut.String(),
		grossMinimum.String(),
		strings.ToLower(req.recipient.Hex()),
		deadline.String(),
		strconv.FormatUint(req.feeBips, 10),
		strings.ToLower(req.feeReceiver.Hex()),
		strconv.FormatBool(req.inputNative),
		strconv.FormatBool(req.outputNative),
	}, "|")
	hash := crypto.Keccak256Hash([]byte(payload))
	return fmt.Sprintf("univ3-%d-%s", c.config.ChainID, hash.Hex()[2:])
}

func (c *Client) ensureDeployment(ctx context.Context) error {
	c.deployMu.RLock()
	checked := c.deployChecked
	c.deployMu.RUnlock()
	if checked {
		return nil
	}
	c.deployMu.Lock()
	defer c.deployMu.Unlock()
	if c.deployChecked {
		return nil
	}
	chainID, err := c.eth.ChainID(ctx)
	if err != nil {
		return fmt.Errorf("univ3: read RPC chainId: %w", err)
	}
	if !chainID.IsUint64() || chainID.Uint64() != c.config.ChainID {
		return fmt.Errorf("univ3: RPC chainId=%s, configured chainId=%d", chainID.String(), c.config.ChainID)
	}

	type deploymentCheck func() error
	checks := make([]deploymentCheck, 0, 7)
	for _, target := range []struct {
		name    string
		address common.Address
	}{
		{name: "Factory", address: c.factory},
		{name: "SwapRouter", address: c.swapRouter},
		{name: "Quoter", address: c.quoter},
	} {
		target := target
		checks = append(checks, func() error {
			code, err := c.eth.CodeAt(ctx, target.address, nil)
			if err != nil {
				return fmt.Errorf("univ3: read %s code at %s: %w", target.name, target.address.Hex(), err)
			}
			if len(code) == 0 {
				return fmt.Errorf("univ3: %s address %s has no contract code", target.name, target.address.Hex())
			}
			return nil
		})
	}
	for _, target := range []struct {
		name    string
		address common.Address
		abi     *abi.ABI
	}{
		{name: "SwapRouter", address: c.swapRouter, abi: &parsedV1},
		{name: "Quoter", address: c.quoter, abi: &parsedQuoterV1},
	} {
		target := target
		checks = append(checks,
			func() error {
				factory, err := c.callImmutableAddress(ctx, target.name, target.address, target.abi, "factory")
				if err != nil {
					return err
				}
				if factory != c.factory {
					return fmt.Errorf("univ3: %s.factory()=%s, configured factory=%s", target.name, factory.Hex(), c.factory.Hex())
				}
				return nil
			},
			func() error {
				wrapped, err := c.callImmutableAddress(ctx, target.name, target.address, target.abi, "WETH9")
				if err != nil {
					return err
				}
				if wrapped != c.wrappedNative {
					return fmt.Errorf("univ3: %s.WETH9()=%s, configured wrappedNative=%s", target.name, wrapped.Hex(), c.wrappedNative.Hex())
				}
				return nil
			},
		)
	}
	type deploymentResult struct {
		index int
		err   error
	}
	results := make(chan deploymentResult, len(checks))
	for index, check := range checks {
		go func(index int, check deploymentCheck) {
			results <- deploymentResult{index: index, err: check()}
		}(index, check)
	}
	errorsByIndex := make([]error, len(checks))
	for range checks {
		result := <-results
		errorsByIndex[result.index] = result.err
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("univ3: deployment validation canceled: %w", err)
	}
	for _, err := range errorsByIndex {
		if err != nil {
			return err
		}
	}
	c.deployChecked = true
	return nil
}

func (c *Client) callImmutableAddress(ctx context.Context, contractName string, address common.Address, contractABI *abi.ABI, method string) (common.Address, error) {
	data, err := contractABI.Pack(method)
	if err != nil {
		return common.Address{}, fmt.Errorf("univ3: pack %s.%s: %w", contractName, method, err)
	}
	raw, err := c.eth.CallContract(ctx, ethereum.CallMsg{To: &address, Data: data}, nil)
	if err != nil {
		return common.Address{}, fmt.Errorf("univ3: %s.%s(): %w", contractName, method, err)
	}
	values, err := contractABI.Unpack(method, raw)
	if err != nil || len(values) != 1 {
		if err == nil {
			err = fmt.Errorf("returned %d values", len(values))
		}
		return common.Address{}, fmt.Errorf("univ3: decode %s.%s response: %w", contractName, method, err)
	}
	result, ok := values[0].(common.Address)
	if !ok || result == (common.Address{}) {
		return common.Address{}, fmt.Errorf("univ3: %s.%s returned invalid address", contractName, method)
	}
	return result, nil
}
