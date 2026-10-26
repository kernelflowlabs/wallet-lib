package univ2

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	gethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/kernelflowlabs/wallet-lib/common/dexmodel"
	"github.com/kernelflowlabs/wallet-lib/signing"
)

var (
	factoryABI  abi.ABI
	router02ABI abi.ABI
)

func init() {
	factoryABI = mustParseABI("Factory", factoryABIJSON)
	router02ABI = mustParseABI("Router02", router02ABIJSON)
	_ = mustParseABI("Pair", pairABIJSON)
}

func mustParseABI(name, raw string) abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(raw))
	if err != nil {
		panic(fmt.Sprintf("univ2: parse %s ABI: %v", name, err))
	}
	return parsed
}

type Client struct {
	config          Config
	factory         common.Address
	router02        common.Address
	wrappedNative   common.Address
	quoteBaseTokens []common.Address
	rpc             *gethrpc.Client
	eth             *ethclient.Client
	now             func() time.Time

	chainMu       sync.RWMutex
	chainChecked  bool
	deployMu      sync.RWMutex
	deployChecked bool
}

func NewClient(config Config) (*Client, error) {
	normalized, factory, router, wrapped, bases, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}
	rpcClient, err := gethrpc.DialContext(context.Background(), normalized.RPC)
	if err != nil {
		return nil, fmt.Errorf("univ2: dial RPC: %w", err)
	}
	return &Client{
		config:          normalized,
		factory:         factory,
		router02:        router,
		wrappedNative:   wrapped,
		quoteBaseTokens: bases,
		rpc:             rpcClient,
		eth:             ethclient.NewClient(rpcClient),
		now:             time.Now,
	}, nil
}

func normalizeConfig(config Config) (Config, common.Address, common.Address, common.Address, []common.Address, error) {
	zero := common.Address{}
	config.ChainName = strings.TrimSpace(config.ChainName)
	config.RPC = strings.TrimSpace(config.RPC)
	if config.ChainName == "" {
		return Config{}, zero, zero, zero, nil, fmt.Errorf("univ2: chainName is required")
	}
	if config.ChainID == 0 {
		return Config{}, zero, zero, zero, nil, fmt.Errorf("univ2: chainId must be greater than zero")
	}
	if err := validateRPCURL(config.RPC); err != nil {
		return Config{}, zero, zero, zero, nil, err
	}
	factory, err := requiredAddress("factory", config.Factory)
	if err != nil {
		return Config{}, zero, zero, zero, nil, err
	}
	router, err := requiredAddress("router02", config.Router02)
	if err != nil {
		return Config{}, zero, zero, zero, nil, err
	}
	wrapped, err := requiredAddress("wrappedNative", config.WrappedNative)
	if err != nil {
		return Config{}, zero, zero, zero, nil, err
	}
	if factory == router {
		return Config{}, zero, zero, zero, nil, fmt.Errorf("univ2: factory and router02 must differ")
	}

	seen := make(map[common.Address]struct{}, len(config.QuoteBaseTokens)+1)
	bases := make([]common.Address, 0, len(config.QuoteBaseTokens)+1)
	normalizedBases := make([]string, 0, len(config.QuoteBaseTokens))
	for i, raw := range config.QuoteBaseTokens {
		base, err := requiredAddress(fmt.Sprintf("quoteBaseTokens[%d]", i), raw)
		if err != nil {
			return Config{}, zero, zero, zero, nil, err
		}
		if _, ok := seen[base]; ok {
			continue
		}
		seen[base] = struct{}{}
		bases = append(bases, base)
		normalizedBases = append(normalizedBases, base.Hex())
	}
	if _, ok := seen[wrapped]; !ok {
		bases = append(bases, wrapped)
	}
	config.Factory = factory.Hex()
	config.Router02 = router.Hex()
	config.WrappedNative = wrapped.Hex()
	config.QuoteBaseTokens = normalizedBases
	if config.DeadlineTTL == 0 {
		config.DeadlineTTL = DefaultDeadlineTTL
	}
	if config.DeadlineTTL < time.Second {
		return Config{}, zero, zero, zero, nil, fmt.Errorf("univ2: deadlineTTL must be at least one second")
	}
	return config, factory, router, wrapped, bases, nil
}

func validateRPCURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("univ2: RPC is required")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("univ2: invalid RPC URL %q", raw)
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "ws", "wss":
		return nil
	default:
		return fmt.Errorf("univ2: unsupported RPC URL scheme %q", u.Scheme)
	}
}

func requiredAddress(name, raw string) (common.Address, error) {
	if !common.IsHexAddress(strings.TrimSpace(raw)) {
		return common.Address{}, fmt.Errorf("univ2: invalid %s address %q", name, raw)
	}
	addr := common.HexToAddress(raw)
	if addr == (common.Address{}) {
		return common.Address{}, fmt.Errorf("univ2: %s address must not be zero", name)
	}
	return addr, nil
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
	out.QuoteBaseTokens = append([]string(nil), c.config.QuoteBaseTokens...)
	return out
}

func (c *Client) Quote(ctx context.Context, in *dexmodel.DexQuoteIn) (*dexmodel.DexQuoteOut, error) {
	if c == nil || c.eth == nil {
		return nil, fmt.Errorf("univ2: client is not initialized")
	}
	amountIn, tokenIn, tokenOut, inputNative, outputNative, recipient, slippage, err := c.validateQuote(in)
	if err != nil {
		return nil, err
	}
	if err := c.ensureChainID(ctx); err != nil {
		return nil, err
	}
	if err := c.ensureDeployment(ctx); err != nil {
		return nil, err
	}

	paths := candidatePaths(tokenIn, tokenOut, c.quoteBaseTokens)
	pairCache := make(map[string]bool)
	viable := make([][]common.Address, 0, len(paths))
	for _, path := range paths {
		hasPairs, err := c.pathHasPairs(ctx, path, pairCache)
		if err != nil {
			return nil, err
		}
		if hasPairs {
			viable = append(viable, path)
		}
	}

	out := &dexmodel.DexQuoteOut{Channel: Channel, Routes: []*dexmodel.DexRoute{}}
	if len(viable) == 0 {
		return out, nil
	}

	var bestPath []common.Address
	var bestAmount *big.Int
	var quoteErrors []string
	for _, path := range viable {
		amount, err := c.getAmountsOut(ctx, amountIn, path)
		if err != nil {
			quoteErrors = append(quoteErrors, err.Error())
			continue
		}
		if bestAmount == nil || amount.Cmp(bestAmount) > 0 {
			bestAmount = amount
			bestPath = path
		}
	}
	if bestAmount == nil {
		return nil, fmt.Errorf("univ2: Router02 getAmountsOut failed for every viable path: %s", strings.Join(quoteErrors, "; "))
	}

	amountOutMin := applySlippage(bestAmount, slippage)
	deadline := big.NewInt(c.now().Add(c.config.DeadlineTTL).Unix())
	calldata, method, err := encodeSwap(amountIn, amountOutMin, bestPath, recipient, deadline, inputNative, outputNative)
	if err != nil {
		return nil, err
	}
	tx := &dexmodel.DexTx{
		To:   c.router02.Hex(),
		Data: "0x" + hex.EncodeToString(calldata),
	}
	if inputNative {
		tx.Value = amountIn.String()
	}

	slippageFloat, _ := slippage.Float64()
	route := &dexmodel.DexRoute{
		RouteId:           c.routeID(method, amountIn, amountOutMin, bestPath, recipient, deadline),
		Name:              fmt.Sprintf("%s Uniswap V2", c.config.ChainName),
		AmountOut:         bestAmount.String(),
		AmountOutMin:      amountOutMin.String(),
		Slippage:          slippageFloat,
		SuggestedSlippage: slippageFloat,
		EstimatedTime:     30,
		NeedBuild:         false,
		TxData:            tx,
		RouteTags:         []string{"MAX_OUTPUT"},
		ExpiresAt:         deadline.Int64(),
	}
	if !inputNative {
		route.ApprovalData = &dexmodel.DexApproval{
			Token:   tokenIn.Hex(),
			Spender: c.router02.Hex(),
			Amount:  amountIn.String(),
		}
		route.TrustedSpenders = []string{c.router02.Hex()}
	}
	out.Routes = append(out.Routes, route)
	return out, nil
}

func (c *Client) validateQuote(in *dexmodel.DexQuoteIn) (*big.Int, common.Address, common.Address, bool, bool, common.Address, *big.Rat, error) {
	if in == nil {
		return nil, common.Address{}, common.Address{}, false, false, common.Address{}, nil, fmt.Errorf("univ2: quote input is nil")
	}
	if !c.matchesChain(in.FromChain) || !c.matchesChain(in.ToChain) {
		return nil, common.Address{}, common.Address{}, false, false, common.Address{}, nil, fmt.Errorf("univ2: only same-chain %s/%d quotes are supported", c.config.ChainName, c.config.ChainID)
	}
	amountIn, ok := new(big.Int).SetString(strings.TrimSpace(in.FromAmount), 10)
	if !ok || amountIn.Sign() <= 0 {
		return nil, common.Address{}, common.Address{}, false, false, common.Address{}, nil, fmt.Errorf("univ2: fromAmount must be a positive base-10 integer")
	}
	inputNative := isNative(in.FromToken)
	outputNative := isNative(in.ToToken)
	if inputNative && outputNative {
		return nil, common.Address{}, common.Address{}, false, false, common.Address{}, nil, fmt.Errorf("univ2: native-to-native swap is not supported")
	}
	tokenIn, err := c.swapToken("fromToken", in.FromToken, inputNative)
	if err != nil {
		return nil, common.Address{}, common.Address{}, false, false, common.Address{}, nil, err
	}
	tokenOut, err := c.swapToken("toToken", in.ToToken, outputNative)
	if err != nil {
		return nil, common.Address{}, common.Address{}, false, false, common.Address{}, nil, err
	}
	if tokenIn == tokenOut {
		return nil, common.Address{}, common.Address{}, false, false, common.Address{}, nil, fmt.Errorf("univ2: fromToken and toToken resolve to the same address")
	}
	if _, err := requiredAddress("fromAddress", in.FromAddress); err != nil {
		return nil, common.Address{}, common.Address{}, false, false, common.Address{}, nil, err
	}
	recipient, err := requiredAddress("toAddress", in.ToAddress)
	if err != nil {
		return nil, common.Address{}, common.Address{}, false, false, common.Address{}, nil, err
	}
	slippage, err := parseSlippage(in.Slippage)
	if err != nil {
		return nil, common.Address{}, common.Address{}, false, false, common.Address{}, nil, err
	}
	return amountIn, tokenIn, tokenOut, inputNative, outputNative, recipient, slippage, nil
}

func (c *Client) swapToken(name, raw string, native bool) (common.Address, error) {
	if native {
		return c.wrappedNative, nil
	}
	return requiredAddress(name, raw)
}

func (c *Client) matchesChain(raw string) bool {
	raw = strings.TrimSpace(raw)
	return strings.EqualFold(raw, c.config.ChainName) || raw == strconv.FormatUint(c.config.ChainID, 10)
}

func isNative(raw string) bool {
	return strings.EqualFold(strings.TrimSpace(raw), signing.MagicContactAddressForNative) ||
		strings.EqualFold(strings.TrimSpace(raw), signing.MagicAddressForZeroEVM)
}

func parseSlippage(raw string) (*big.Rat, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return new(big.Rat), nil
	}
	slippage, ok := new(big.Rat).SetString(raw)
	if !ok || slippage.Sign() < 0 || slippage.Cmp(big.NewRat(100, 1)) >= 0 {
		return nil, fmt.Errorf("univ2: slippage must be a percentage in [0,100)")
	}
	return slippage, nil
}

func applySlippage(amountOut *big.Int, slippage *big.Rat) *big.Int {
	factor := new(big.Rat).Sub(big.NewRat(100, 1), slippage)
	value := new(big.Rat).Mul(new(big.Rat).SetInt(amountOut), factor)
	value.Quo(value, big.NewRat(100, 1))
	return new(big.Int).Quo(value.Num(), value.Denom())
}

func candidatePaths(tokenIn, tokenOut common.Address, bases []common.Address) [][]common.Address {
	paths := [][]common.Address{{tokenIn, tokenOut}}
	seen := map[string]struct{}{pathKey(paths[0]): {}}
	for _, base := range bases {
		if base == tokenIn || base == tokenOut {
			continue
		}
		path := []common.Address{tokenIn, base, tokenOut}
		key := pathKey(path)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		paths = append(paths, path)
	}
	return paths
}

func pathKey(path []common.Address) string {
	parts := make([]string, len(path))
	for i, addr := range path {
		parts[i] = strings.ToLower(addr.Hex())
	}
	return strings.Join(parts, ">")
}

func pairKey(a, b common.Address) string {
	left, right := strings.ToLower(a.Hex()), strings.ToLower(b.Hex())
	if left > right {
		left, right = right, left
	}
	return left + ":" + right
}

func (c *Client) pathHasPairs(ctx context.Context, path []common.Address, cache map[string]bool) (bool, error) {
	for i := 0; i+1 < len(path); i++ {
		key := pairKey(path[i], path[i+1])
		hasPair, ok := cache[key]
		if !ok {
			pair, err := c.getPair(ctx, path[i], path[i+1])
			if err != nil {
				return false, err
			}
			hasPair = pair != (common.Address{})
			cache[key] = hasPair
		}
		if !hasPair {
			return false, nil
		}
	}
	return true, nil
}

func (c *Client) getPair(ctx context.Context, tokenA, tokenB common.Address) (common.Address, error) {
	data, err := factoryABI.Pack("getPair", tokenA, tokenB)
	if err != nil {
		return common.Address{}, fmt.Errorf("univ2: pack Factory.getPair: %w", err)
	}
	raw, err := c.eth.CallContract(ctx, ethereum.CallMsg{To: &c.factory, Data: data}, nil)
	if err != nil {
		return common.Address{}, fmt.Errorf("univ2: Factory.getPair(%s,%s): %w", tokenA.Hex(), tokenB.Hex(), err)
	}
	values, err := factoryABI.Unpack("getPair", raw)
	if err != nil {
		return common.Address{}, fmt.Errorf("univ2: decode Factory.getPair response: %w", err)
	}
	if len(values) != 1 {
		return common.Address{}, fmt.Errorf("univ2: Factory.getPair returned %d values", len(values))
	}
	pair, ok := values[0].(common.Address)
	if !ok {
		return common.Address{}, fmt.Errorf("univ2: Factory.getPair returned %T", values[0])
	}
	return pair, nil
}

func (c *Client) getAmountsOut(ctx context.Context, amountIn *big.Int, path []common.Address) (*big.Int, error) {
	data, err := router02ABI.Pack("getAmountsOut", amountIn, path)
	if err != nil {
		return nil, fmt.Errorf("univ2: pack Router02.getAmountsOut: %w", err)
	}
	raw, err := c.eth.CallContract(ctx, ethereum.CallMsg{To: &c.router02, Data: data}, nil)
	if err != nil {
		return nil, fmt.Errorf("univ2: Router02.getAmountsOut(%s): %w", pathKey(path), err)
	}
	values, err := router02ABI.Unpack("getAmountsOut", raw)
	if err != nil {
		return nil, fmt.Errorf("univ2: decode Router02.getAmountsOut response: %w", err)
	}
	if len(values) != 1 {
		return nil, fmt.Errorf("univ2: Router02.getAmountsOut returned %d values", len(values))
	}
	amounts, ok := values[0].([]*big.Int)
	if !ok || len(amounts) != len(path) || amounts[len(amounts)-1] == nil || amounts[len(amounts)-1].Sign() <= 0 {
		return nil, fmt.Errorf("univ2: invalid Router02.getAmountsOut response")
	}
	return new(big.Int).Set(amounts[len(amounts)-1]), nil
}

func encodeSwap(amountIn, amountOutMin *big.Int, path []common.Address, recipient common.Address, deadline *big.Int, inputNative, outputNative bool) ([]byte, string, error) {
	var (
		data   []byte
		method string
		err    error
	)
	switch {
	case inputNative:
		method = "swapExactETHForTokens"
		data, err = router02ABI.Pack(method, amountOutMin, path, recipient, deadline)
	case outputNative:
		method = "swapExactTokensForETH"
		data, err = router02ABI.Pack(method, amountIn, amountOutMin, path, recipient, deadline)
	default:
		method = "swapExactTokensForTokens"
		data, err = router02ABI.Pack(method, amountIn, amountOutMin, path, recipient, deadline)
	}
	if err != nil {
		return nil, "", fmt.Errorf("univ2: pack Router02.%s: %w", method, err)
	}
	return data, method, nil
}

func (c *Client) routeID(method string, amountIn, amountOutMin *big.Int, path []common.Address, recipient common.Address, deadline *big.Int) string {
	payload := strings.Join([]string{
		strconv.FormatUint(c.config.ChainID, 10),
		strings.ToLower(c.factory.Hex()),
		strings.ToLower(c.router02.Hex()),
		method,
		amountIn.String(),
		amountOutMin.String(),
		pathKey(path),
		strings.ToLower(recipient.Hex()),
		deadline.String(),
	}, "|")
	hash := crypto.Keccak256Hash([]byte(payload))
	return fmt.Sprintf("univ2-%d-%s", c.config.ChainID, hex.EncodeToString(hash[:8]))
}

func (c *Client) ensureChainID(ctx context.Context) error {
	c.chainMu.RLock()
	checked := c.chainChecked
	c.chainMu.RUnlock()
	if checked {
		return nil
	}
	chainID, err := c.eth.ChainID(ctx)
	if err != nil {
		return fmt.Errorf("univ2: read RPC chainId: %w", err)
	}
	if !chainID.IsUint64() || chainID.Uint64() != c.config.ChainID {
		return fmt.Errorf("univ2: RPC chainId=%s, configured chainId=%d", chainID.String(), c.config.ChainID)
	}
	c.chainMu.Lock()
	c.chainChecked = true
	c.chainMu.Unlock()
	return nil
}

func (c *Client) ensureDeployment(ctx context.Context) error {
	c.deployMu.RLock()
	checked := c.deployChecked
	c.deployMu.RUnlock()
	if checked {
		return nil
	}
	factory, err := c.callRouterAddress(ctx, "factory")
	if err != nil {
		return err
	}
	if factory != c.factory {
		return fmt.Errorf("univ2: Router02.factory()=%s, configured factory=%s", factory.Hex(), c.factory.Hex())
	}
	wrapped, err := c.callRouterAddress(ctx, "WETH")
	if err != nil {
		return err
	}
	if wrapped != c.wrappedNative {
		return fmt.Errorf("univ2: Router02.WETH()=%s, configured wrappedNative=%s", wrapped.Hex(), c.wrappedNative.Hex())
	}
	c.deployMu.Lock()
	c.deployChecked = true
	c.deployMu.Unlock()
	return nil
}

func (c *Client) callRouterAddress(ctx context.Context, methodName string) (common.Address, error) {
	data, err := router02ABI.Pack(methodName)
	if err != nil {
		return common.Address{}, fmt.Errorf("univ2: pack Router02.%s: %w", methodName, err)
	}
	raw, err := c.eth.CallContract(ctx, ethereum.CallMsg{To: &c.router02, Data: data}, nil)
	if err != nil {
		return common.Address{}, fmt.Errorf("univ2: Router02.%s(): %w", methodName, err)
	}
	values, err := router02ABI.Unpack(methodName, raw)
	if err != nil {
		return common.Address{}, fmt.Errorf("univ2: decode Router02.%s response: %w", methodName, err)
	}
	if len(values) != 1 {
		return common.Address{}, fmt.Errorf("univ2: Router02.%s returned %d values", methodName, len(values))
	}
	address, ok := values[0].(common.Address)
	if !ok {
		return common.Address{}, fmt.Errorf("univ2: Router02.%s returned %T", methodName, values[0])
	}
	return address, nil
}

func (c *Client) Status(ctx context.Context, in *dexmodel.DexCheckTxIn) (*dexmodel.DexCheckTxOut, error) {
	if c == nil || c.eth == nil {
		return nil, fmt.Errorf("univ2: client is not initialized")
	}
	if in == nil {
		return nil, fmt.Errorf("univ2: status input is nil")
	}
	if in.HashType != "" && in.HashType != dexmodel.DexHashTypeTxHash {
		return nil, fmt.Errorf("univ2: only txHash status checks are supported")
	}
	if in.FromChain != "" && !c.matchesChain(in.FromChain) {
		return nil, fmt.Errorf("univ2: fromChain does not match %s/%d", c.config.ChainName, c.config.ChainID)
	}
	if in.ToChain != "" && !c.matchesChain(in.ToChain) {
		return nil, fmt.Errorf("univ2: toChain does not match %s/%d", c.config.ChainName, c.config.ChainID)
	}
	hashBytes, err := hexutil.Decode(in.Hash)
	if err != nil || len(hashBytes) != common.HashLength {
		return nil, fmt.Errorf("univ2: invalid transaction hash %q", in.Hash)
	}
	if err := c.ensureChainID(ctx); err != nil {
		return nil, err
	}
	hash := common.BytesToHash(hashBytes)
	receipt, err := c.eth.TransactionReceipt(ctx, hash)
	if errors.Is(err, ethereum.NotFound) {
		return &dexmodel.DexCheckTxOut{
			Channel:  Channel,
			Status:   dexmodel.DexStatusPending,
			ToChain:  c.config.ChainName,
			ToHash:   hash.Hex(),
			FromHash: hash.Hex(),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("univ2: transaction receipt: %w", err)
	}
	status := dexmodel.DexStatusFailed
	if receipt.Status == 1 {
		status = dexmodel.DexStatusSucceeded
	}
	return &dexmodel.DexCheckTxOut{
		Channel:        Channel,
		Status:         status,
		ToChain:        c.config.ChainName,
		ToHash:         hash.Hex(),
		FromHash:       hash.Hex(),
		ProviderStatus: strconv.FormatUint(receipt.Status, 10),
	}, nil
}
