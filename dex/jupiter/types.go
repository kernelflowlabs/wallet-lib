package jupiter

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	dexmodel "github.com/kernelflowlabs/wallet-lib/common/dexmodel"
	"strings"

	"github.com/shopspring/decimal"
)

// UltraOrderReq builds the GET /ultra/v1/order query. Amount is in input
// mint's smallest unit; Taker is the user's wallet pubkey (base58).
type UltraOrderReq struct {
	InputMint   string
	OutputMint  string
	Amount      string
	Taker       string
	SlippageBps int // 0 == let Ultra pick dynamic slippage
}

// UltraOrderRes mirrors /ultra/v1/order. Router distinguishes the RFQ
// gasless path ("jupiterz") from regular aggregator ("metis"); Gasless is
// the explicit boolean. Transaction is base64 versioned tx and is only
// populated on the gasless path — metis still requires us to fetch swap
// instructions and assemble the tx with our own fee payer.
type UltraOrderRes struct {
	Router  string `json:"router"`
	Gasless bool   `json:"gasless"`
	Mode    string `json:"mode"`

	InputMint            string `json:"inputMint"`
	OutputMint           string `json:"outputMint"`
	InAmount             string `json:"inAmount"`
	OutAmount            string `json:"outAmount"`
	OtherAmountThreshold string `json:"otherAmountThreshold"`
	SlippageBps          int    `json:"slippageBps"`
	PriceImpactPct       string `json:"priceImpactPct"`

	Taker     string `json:"taker"`
	RequestID string `json:"requestId"`

	SignatureFeePayer         string `json:"signatureFeePayer"`
	PrioritizationFeePayer    string `json:"prioritizationFeePayer"`
	RentFeePayer              string `json:"rentFeePayer"`
	SignatureFeeLamports      int64  `json:"signatureFeeLamports"`
	PrioritizationFeeLamports int64  `json:"prioritizationFeeLamports"`
	RentFeeLamports           int64  `json:"rentFeeLamports"`

	Transaction          string `json:"transaction"` // gasless path only
	LastValidBlockHeight uint64 `json:"lastValidBlockHeight"`

	ErrorCode    string `json:"errorCode,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

type UltraExecuteReq struct {
	SignedTransaction string `json:"signedTransaction"`
	RequestID         string `json:"requestId"`
}

// SwapInstructionsReq feeds POST /swap/v1/swap-instructions. UserPublicKey
// is the account whose USDC moves, not the fee payer — we put our own
// wallet in the fee-payer slot when assembling the v0 tx.
type SwapInstructionsReq struct {
	UserPublicKey            string    `json:"userPublicKey"`
	QuoteResponse            *QuoteRes `json:"quoteResponse"`
	WrapAndUnwrapSol         bool      `json:"wrapAndUnwrapSol"`
	UseSharedAccounts        bool      `json:"useSharedAccounts"`
	FeeAccount               string    `json:"feeAccount,omitempty"`
	DynamicComputeUnitLimit  bool      `json:"dynamicComputeUnitLimit,omitempty"`
	SkipUserAccountsRpcCalls bool      `json:"skipUserAccountsRpcCalls,omitempty"`
}

type InstructionDTO struct {
	ProgramID string            `json:"programId"`
	Accounts  []InstructionAcct `json:"accounts"`
	Data      string            `json:"data"` // base64
}

type InstructionAcct struct {
	Pubkey     string `json:"pubkey"`
	IsSigner   bool   `json:"isSigner"`
	IsWritable bool   `json:"isWritable"`
}

// BlockhashWithMetadata mirrors Jupiter's response field; blockhash arrives
// as a JSON array of 32 integers, not a base58 string, so we decode through
// []int and expose Bytes() for the tx builder.
type BlockhashWithMetadata struct {
	Blockhash            []int  `json:"blockhash"`
	LastValidBlockHeight uint64 `json:"lastValidBlockHeight"`
}

func (b *BlockhashWithMetadata) Bytes() [32]byte {
	var out [32]byte
	for i, v := range b.Blockhash {
		if i >= 32 {
			break
		}
		out[i] = byte(v)
	}
	return out
}

type SwapInstructionsRes struct {
	TokenLedgerInstruction        *InstructionDTO       `json:"tokenLedgerInstruction"`
	ComputeBudgetInstructions     []InstructionDTO      `json:"computeBudgetInstructions"`
	SetupInstructions             []InstructionDTO      `json:"setupInstructions"`
	SwapInstruction               *InstructionDTO       `json:"swapInstruction"`
	CleanupInstruction            *InstructionDTO       `json:"cleanupInstruction"`
	AddressLookupTableAddresses   []string              `json:"addressLookupTableAddresses"`
	AddressesByLookupTableAddress map[string][]string   `json:"addressesByLookupTableAddress"`
	BlockhashWithMetadata         BlockhashWithMetadata `json:"blockhashWithMetadata"`
	PrioritizationFeeLamports     int64                 `json:"prioritizationFeeLamports"`
	ComputeUnitLimit              int64                 `json:"computeUnitLimit"`
	OtherInstructions             []InstructionDTO      `json:"otherInstructions"`
	SimulationError               json.RawMessage       `json:"simulationError"`
	ErrorCode                     string                `json:"errorCode,omitempty"`
	ErrorMessage                  string                `json:"error,omitempty"`
}

type UltraExecuteRes struct {
	Status    string `json:"status"`
	Signature string `json:"signature"`
	Slot      uint64 `json:"slot"`
	Code      int    `json:"code"`
	Error     string `json:"error"`
}

type (
	QuoteReq struct {
		InputMint      string
		OutputMint     string
		Amount         string
		SlippageBps    string
		PlatformFeeBps string
		UserPublicKey  string
	}
	QuoteRes struct {
		ErrorCode            string          `json:"errorCode,omitempty"`
		ErrorMsg             string          `json:"error,omitempty"`
		InputMint            string          `json:"inputMint"`
		OutputMint           string          `json:"outputMint"`
		InAmount             string          `json:"inAmount"`
		OutAmount            string          `json:"outAmount"`
		OtherAmountThreshold string          `json:"otherAmountThreshold"`
		SwapMode             string          `json:"swapMode"`
		SlippageBps          int             `json:"slippageBps"`
		PlatformFee          *PlatformFee    `json:"platformFee,omitempty"`
		PriceImpactPct       string          `json:"priceImpactPct"`
		RoutePlan            []RoutePlanStep `json:"routePlan"`
		ContextSlot          int64           `json:"contextSlot"`
		TimeTaken            float64         `json:"timeTaken"`
	}
	PlatformFee struct {
		Amount string `json:"amount"`
		FeeBps int    `json:"feeBps"`
	}
	RoutePlanStep struct {
		SwapInfo struct {
			AmmKey     string `json:"ammKey"`
			Label      string `json:"label"`
			InputMint  string `json:"inputMint"`
			OutputMint string `json:"outputMint"`
			InAmount   string `json:"inAmount"`
			OutAmount  string `json:"outAmount"`
			FeeAmount  string `json:"feeAmount"`
			FeeMint    string `json:"feeMint"`
		} `json:"swapInfo"`
		Percent int `json:"percent"`
	}
	SwapReq struct {
		QuoteResponse           *QuoteRes `json:"quoteResponse"`
		UserPublicKey           string    `json:"userPublicKey"`
		WrapAndUnwrapSol        bool      `json:"wrapAndUnwrapSol"`
		DynamicComputeUnitLimit bool      `json:"dynamicComputeUnitLimit"`
		FeeAccount              string    `json:"feeAccount,omitempty"`
	}
	SwapRes struct {
		ErrorCode                 string `json:"errorCode,omitempty"`
		ErrorMsg                  string `json:"error,omitempty"`
		SwapTransaction           string `json:"swapTransaction"`
		LastValidBlockHeight      int64  `json:"lastValidBlockHeight"`
		PrioritizationFeeLamports int64  `json:"prioritizationFeeLamports"`
	}
	SignatureStatusRes struct {
		Result struct {
			Value []*SignatureStatusValue `json:"value"`
		} `json:"result"`
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	SignatureStatusValue struct {
		Slot               int64       `json:"slot"`
		Confirmations      *int64      `json:"confirmations"`
		ConfirmationStatus string      `json:"confirmationStatus"`
		Err                interface{} `json:"err"`
	}

	StocksQuery struct {
		SortBy            string // e.g. "volume", "mcap", "liquidity"
		SortDir           string // "asc" or "desc"
		Limit             int
		Offset            int
		IncludeOndoStatus bool
	}
	StocksRes struct {
		Assets     []StockAsset `json:"assets"`
		Total      int          `json:"total"`
		Next       int          `json:"next"`
		OndoStatus *OndoStatus  `json:"ondoStatus,omitempty"`
	}
	OndoStatus struct {
		PauseInfo  json.RawMessage `json:"pauseInfo,omitempty"`
		MarketInfo OndoMarketInfo  `json:"marketInfo"`
	}
	OndoMarketInfo struct {
		IsOpen    bool   `json:"isOpen"`
		NextOpen  string `json:"nextOpen"`
		NextClose string `json:"nextClose"`
	}
	StockAsset struct {
		ID              string     `json:"id"`
		Name            string     `json:"name"`
		Symbol          string     `json:"symbol"`
		Icon            string     `json:"icon"`
		Decimals        int        `json:"decimals"`
		Website         string     `json:"website"`
		Dev             string     `json:"dev"`
		CircSupply      float64    `json:"circSupply"`
		TotalSupply     float64    `json:"totalSupply"`
		TokenProgram    string     `json:"tokenProgram"`
		MintAuthority   string     `json:"mintAuthority"`
		FreezeAuthority string     `json:"freezeAuthority"`
		FirstPool       StockPool  `json:"firstPool"`
		HolderCount     int        `json:"holderCount"`
		Audit           StockAudit `json:"audit"`
		StockData       StockData  `json:"stockData"`
		OrganicScore    float64    `json:"organicScore"`
		OrganicLabel    string     `json:"organicScoreLabel"`
		CtLikes         int        `json:"ctLikes"`
		IsVerified      bool       `json:"isVerified"`
		Tags            []string   `json:"tags"`
		CreatedAt       string     `json:"createdAt"`
		FDV             float64    `json:"fdv"`
		MCap            float64    `json:"mcap"`
		UsdPrice        float64    `json:"usdPrice"`
		PriceBlockID    int64      `json:"priceBlockId"`
		Liquidity       float64    `json:"liquidity"`
		Stats5m         StockStats `json:"stats5m"`
		Stats1h         StockStats `json:"stats1h"`
		Stats6h         StockStats `json:"stats6h"`
		Stats24h        StockStats `json:"stats24h"`
		Stats7d         StockStats `json:"stats7d"`
		Stats30d        StockStats `json:"stats30d"`
		Fees            float64    `json:"fees"`
		UpdatedAt       string     `json:"updatedAt"`
	}
	StockPool struct {
		ID        string `json:"id"`
		CreatedAt string `json:"createdAt"`
	}
	StockAudit struct {
		TopHoldersPercentage    float64           `json:"topHoldersPercentage"`
		DevMints                int               `json:"devMints"`
		PermanentControlEnabled bool              `json:"permanentControlEnabled"`
		MutableFees             bool              `json:"mutableFees"`
		BotHoldersCount         int               `json:"botHoldersCount"`
		BotHoldersPercentage    float64           `json:"botHoldersPercentage"`
		DevFundedAt             string            `json:"devFundedAt"`
		BundlerStats            StockBundlerStats `json:"bundlerStats"`
	}
	StockBundlerStats struct {
		TotalBundles   int     `json:"totalBundles"`
		TotalNativeVol float64 `json:"totalNativeVol"`
		HoldingPctATH  float64 `json:"holdingPctATH"`
	}
	StockData struct {
		ID        string  `json:"id"`
		Price     float64 `json:"price"`
		MCap      float64 `json:"mcap"`
		UpdatedAt string  `json:"updatedAt"`
	}
	ChartReq struct {
		Mint     string
		Interval string // 1_MINUTE / 5_MINUTE / 15_MINUTE / 30_MINUTE / 1_HOUR / 4_HOUR / 1_DAY / 1_WEEK
		ToMs     int64  // unix ms; required upstream
		Candles  int
		Type     string // price | mcap (default price)
	}
	ChartsRes struct {
		Candles []Candle `json:"candles"`
	}
	Candle struct {
		Time   int64   `json:"time"` // unix seconds
		Open   float64 `json:"open"`
		High   float64 `json:"high"`
		Low    float64 `json:"low"`
		Close  float64 `json:"close"`
		Volume float64 `json:"volume"`
	}

	HoldersRes struct {
		Holders   []Holder              `json:"holders"`
		Count     int                   `json:"count"`
		HolderPnl map[string]*HolderPnl `json:"holderPnl"`
	}
	Holder struct {
		Address           string             `json:"address"`
		Amount            float64            `json:"amount"`
		SolBalance        string             `json:"solBalance"`
		SolBalanceDisplay float64            `json:"solBalanceDisplay"`
		AddressInfo       *HolderAddressInfo `json:"addressInfo,omitempty"`
		Tags              []HolderTag        `json:"tags"`
	}
	HolderTag struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	HolderAddressInfo struct {
		FundingAddress   string  `json:"fundingAddress"`
		FundingAmount    float64 `json:"fundingAmount"`
		FundingTx        string  `json:"fundingTx"`
		FundingBlockTime string  `json:"fundingBlockTime"`
		FundingSlot      int64   `json:"fundingSlot"`
	}
	HolderPnl struct {
		RealizedPnl                   float64 `json:"realizedPnl"`
		RealizedPnlNative             float64 `json:"realizedPnlNative"`
		UnrealizedPnl                 float64 `json:"unrealizedPnl"`
		UnrealizedPnlNative           float64 `json:"unrealizedPnlNative"`
		TotalPnl                      float64 `json:"totalPnl"`
		TotalPnlNative                float64 `json:"totalPnlNative"`
		TotalBuys                     int     `json:"totalBuys"`
		TotalSells                    int     `json:"totalSells"`
		BoughtAmount                  float64 `json:"boughtAmount"`
		BoughtValue                   float64 `json:"boughtValue"`
		BoughtValueNative             float64 `json:"boughtValueNative"`
		SoldAmount                    float64 `json:"soldAmount"`
		SoldValue                     float64 `json:"soldValue"`
		SoldValueNative               float64 `json:"soldValueNative"`
		RealizedPnlPercentage         float64 `json:"realizedPnlPercentage"`
		RealizedPnlPercentageNative   float64 `json:"realizedPnlPercentageNative"`
		UnrealizedPnlPercentage       float64 `json:"unrealizedPnlPercentage"`
		UnrealizedPnlPercentageNative float64 `json:"unrealizedPnlPercentageNative"`
		TotalPnlPercentage            float64 `json:"totalPnlPercentage"`
		TotalPnlPercentageNative      float64 `json:"totalPnlPercentageNative"`
	}

	StockStats struct {
		PriceChange       float64 `json:"priceChange"`
		HolderChange      float64 `json:"holderChange"`
		LiquidityChange   float64 `json:"liquidityChange"`
		VolumeChange      float64 `json:"volumeChange"`
		BuyVolume         float64 `json:"buyVolume"`
		SellVolume        float64 `json:"sellVolume"`
		BuyOrganicVolume  float64 `json:"buyOrganicVolume"`
		SellOrganicVolume float64 `json:"sellOrganicVolume"`
		NumBuys           int     `json:"numBuys"`
		NumSells          int     `json:"numSells"`
		NumTraders        int     `json:"numTraders"`
		NumOrganicBuyers  int     `json:"numOrganicBuyers"`
		NumNetBuyers      int     `json:"numNetBuyers"`
	}
)

const (
	nativeSolPlaceholder = "11111111111111111111111111111111"
	wrappedSolMint       = "So11111111111111111111111111111111111111112"
)

func (c *Client) toJupiterQuoteReq(in *dexmodel.DexQuoteIn) *QuoteReq {
	if in.FromAddress == "" {
		return nil
	}
	inputMint := normalizeMint(in.FromToken)
	outputMint := normalizeMint(in.ToToken)
	if inputMint == "" || outputMint == "" {
		return nil
	}

	slippageBps := "50"
	if in.Slippage != "" {
		d, err := decimal.NewFromString(in.Slippage)
		if err == nil {
			slippageBps = d.Shift(2).Truncate(0).String()
		}
	}

	var platformFeeBps string
	if c.feeAccount != "" && in.FeeRate != "" {
		d, err := decimal.NewFromString(in.FeeRate)
		if err == nil {
			platformFeeBps = d.Shift(2).Truncate(0).String()
		}
	}

	return &QuoteReq{
		InputMint:      inputMint,
		OutputMint:     outputMint,
		Amount:         in.FromAmount,
		SlippageBps:    slippageBps,
		PlatformFeeBps: platformFeeBps,
		UserPublicKey:  in.FromAddress,
	}
}

func (c *Client) toStandardQuoteRes(quote *QuoteRes, swap *SwapRes) *dexmodel.DexQuoteOut {
	if quote == nil || swap == nil || swap.SwapTransaction == "" {
		return nil
	}

	raw, err := base64.StdEncoding.DecodeString(swap.SwapTransaction)
	if err != nil {
		return nil
	}
	data := hex.EncodeToString(raw)

	slippage, _ := decimal.NewFromInt(int64(quote.SlippageBps)).Shift(-2).Float64()

	feeUsd := ""

	// Jupiter emits priceImpactPct as a string ("0.0015" = 0.15%).
	// Empty / unparsable → 0 (treated as "unknown" downstream).
	// Multiply by 100 so the wire value is a straight percent
	// ("99.99") matching the DexRoute.PriceImpactPct doc.
	priceImpact := 0.0
	if quote.PriceImpactPct != "" {
		if pip, err := decimal.NewFromString(quote.PriceImpactPct); err == nil {
			priceImpact, _ = pip.Shift(2).Float64()
		}
	}
	route := &dexmodel.DexRoute{
		RouteId:           fmt.Sprintf("jupiter-%d", quote.ContextSlot),
		Name:              "Jupiter",
		AmountOut:         quote.OutAmount,
		AmountOutMin:      quote.OtherAmountThreshold,
		Slippage:          slippage,
		SuggestedSlippage: slippage,
		FeeInUsd:          feeUsd,
		EstimatedTime:     30,
		NeedBuild:         false,
		PriceImpactPct:    priceImpact,
		TxData: &dexmodel.DexTx{
			Data: data,
		},
	}

	return &dexmodel.DexQuoteOut{
		Channel: "jupiter",
		Routes:  []*dexmodel.DexRoute{route},
	}
}

// ultraToStandardQuoteRes converts a gasless Ultra Order into the unified
// DexQuoteOut shape. Returns nil when the order isn't gasless so the caller
// falls through to the regular Jupiter path. Gasless == true is the canonical
// signal; the underlying Router can be jupiterz / metis / okx / dflow — any
// of those may opportunistically have an MM cover the fee-payer slot.
func (c *Client) ultraToStandardQuoteRes(o *UltraOrderRes) *dexmodel.DexQuoteOut {
	if o == nil || !o.Gasless || o.Transaction == "" {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(o.Transaction)
	if err != nil {
		return nil
	}
	data := hex.EncodeToString(raw)
	slippage, _ := decimal.NewFromInt(int64(o.SlippageBps)).Shift(-2).Float64()
	return &dexmodel.DexQuoteOut{
		Channel: "jupiter_ultra",
		Routes: []*dexmodel.DexRoute{{
			RouteId:           "ultra-" + o.RequestID,
			Name:              "Jupiter Ultra",
			AmountOut:         o.OutAmount,
			AmountOutMin:      o.OtherAmountThreshold,
			Slippage:          slippage,
			SuggestedSlippage: slippage,
			EstimatedTime:     15,
			NeedBuild:         false,
			Gasless:           true,
			RequestId:         o.RequestID,
			TxData:            &dexmodel.DexTx{Data: data},
		}},
	}
}

func (c *Client) toStandardStatusRes(res *SignatureStatusRes, in *dexmodel.DexCheckTxIn) *dexmodel.DexCheckTxOut {
	out := &dexmodel.DexCheckTxOut{
		Channel: "jupiter",
		ToChain: in.ToChain,
		ToHash:  in.Hash,
		Status:  dexmodel.DexStatusPending,
	}
	if res == nil || len(res.Result.Value) == 0 || res.Result.Value[0] == nil {
		return out
	}

	v := res.Result.Value[0]
	if v.Err != nil {
		out.Status = dexmodel.DexStatusFailed
		out.Msg = fmt.Sprintf("%v", v.Err)
		return out
	}
	switch strings.ToLower(v.ConfirmationStatus) {
	case "finalized", "confirmed":
		out.Status = dexmodel.DexStatusSucceeded
	default:
		out.Status = dexmodel.DexStatusPending
	}
	return out
}

func normalizeMint(token string) string {
	t := strings.TrimSpace(token)
	if t == "" {
		return ""
	}
	lower := strings.ToLower(t)
	if lower == "0x0000000000000000000000000000000000000000" ||
		lower == "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" ||
		t == nativeSolPlaceholder {
		return wrappedSolMint
	}
	return t
}
