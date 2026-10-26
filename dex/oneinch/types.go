package oneinch

// FusionQuoteReq mirrors POST /fusion/quoter/v2.0/{chainId}/quote/receive.
// Amount is in fromToken's smallest unit; WalletAddress is the user EOA
// that will sign the EIP-712 order.
type FusionQuoteReq struct {
	FromTokenAddress string `json:"fromTokenAddress"`
	ToTokenAddress   string `json:"toTokenAddress"`
	Amount           string `json:"amount"`
	WalletAddress    string `json:"walletAddress"`
	EnableEstimate   bool   `json:"enableEstimate"` // true returns presets + gas estimates
	Source           string `json:"source,omitempty"`
}

// FusionPreset is one of the auction profiles 1inch returns (fast / medium
// / slow). Pick fast for retail UX — landed-in-30s typical, slightly worse
// price than slow.
type FusionPreset struct {
	AuctionDuration    int     `json:"auctionDuration"`
	StartAuctionIn     int     `json:"startAuctionIn"`
	BankFee            string  `json:"bankFee"`
	InitialRateBump    int     `json:"initialRateBump"`
	AuctionStartAmount string  `json:"auctionStartAmount"`
	StartAmount        string  `json:"startAmount"`
	AuctionEndAmount   string  `json:"auctionEndAmount"`
	ExclusiveResolver  any     `json:"exclusiveResolver"`
	CostInDstToken     string  `json:"costInDstToken"`
	Points             []any   `json:"points"`
	AllowPartialFills  bool    `json:"allowPartialFills"`
	AllowMultipleFills bool    `json:"allowMultipleFills"`
	GasCost            GasCost `json:"gasCost"`
}

type GasCost struct {
	GasBumpEstimate  int    `json:"gasBumpEstimate"`
	GasPriceEstimate string `json:"gasPriceEstimate"`
}

// FusionQuoteRes is what the quoter returns. Presets carries the
// fast/medium/slow auction options; pick one based on Settings.
type FusionQuoteRes struct {
	QuoteID           string                  `json:"quoteId"`
	FromTokenAmount   string                  `json:"fromTokenAmount"`
	ToTokenAmount     string                  `json:"toTokenAmount"`
	Presets           map[string]FusionPreset `json:"presets"`
	RecommendedPreset string                  `json:"recommended_preset"`
	Prices            FusionPrices            `json:"prices"`
	Volume            FusionVolume            `json:"volume"`
	SettlementAddress string                  `json:"settlementAddress"`
	Whitelist         []string                `json:"whitelist"`

	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
	Success *bool  `json:"success,omitempty"`
}

type FusionPrices struct {
	UsdFrom string `json:"usd.from"`
	UsdTo   string `json:"usd.to"`
}

type FusionVolume struct {
	UsdFrom string `json:"usd.from"`
	UsdTo   string `json:"usd.to"`
}

// FusionOrderSubmitReq carries the signed order ready for the relayer.
// Order is the limit-order struct, Signature is the EIP-712 hex sig,
// Extension is the optional Permit2 / interactions field.
type FusionOrderSubmitReq struct {
	Order     FusionOrder `json:"order"`
	Signature string      `json:"signature"`
	Extension string      `json:"extension,omitempty"`
	QuoteID   string      `json:"quoteId"`
}

// FusionOrder is the EIP-712 limit-order shape 1inch resolvers fill. Caller
// builds this from the QuoteRes + chosen preset before signing.
type FusionOrder struct {
	Salt         string `json:"salt"`
	Maker        string `json:"maker"`      // user EOA
	Receiver     string `json:"receiver"`   // usually maker
	MakerAsset   string `json:"makerAsset"` // fromToken
	TakerAsset   string `json:"takerAsset"` // toToken
	MakingAmount string `json:"makingAmount"`
	TakingAmount string `json:"takingAmount"`
	MakerTraits  string `json:"makerTraits"`
}

type FusionOrderSubmitRes struct {
	OrderHash string `json:"orderHash"`
	Error     string `json:"error,omitempty"`
}

// FusionOrderStatusRes mirrors GET /fusion/orders/v2.0/{chainId}/order/status/{orderHash}.
// Status values per 1inch docs: pending | filled | falsePartiallyFilled |
// partiallyFilled | expired | cancelled | refunding | refunded.
type FusionOrderStatusRes struct {
	OrderHash string `json:"orderHash"`
	Status    string `json:"status"`
	Fills     []any  `json:"fills"`
	Error     string `json:"error,omitempty"`
}

// SupportedChains maps the chainserver chain name (ETH/BASE/BNB/etc.) to
// 1inch's chainId. Mirrors what configs/chain table has but kept here so
// the package is self-contained for tests.
var SupportedChains = map[string]int{
	"ETH":  1,
	"BASE": 8453,
	"BNB":  56,
	"ARB":  42161,
	"OP":   10,
}
