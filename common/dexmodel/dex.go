package dexmodel

import "encoding/json"

type (
	DexQuoteIn struct {
		FromChain               string   `json:"fromChain"`
		ToChain                 string   `json:"toChain"`
		FromToken               string   `json:"fromToken"`
		ToToken                 string   `json:"toToken"`
		FromAddress             string   `json:"fromAddress"`
		ToAddress               string   `json:"toAddress"`
		FromAmount              string   `json:"fromAmount"`
		Slippage                string   `json:"slippage"`
		FeeRate                 string   `json:"feeRate,omitempty"`
		FeeReceiver             string   `json:"feeReceiver,omitempty"`
		FromValueUsd            string   `json:"fromValueUsd,omitempty"`
		GasOnDestination        string   `json:"gasOnDestination,omitempty"`
		UserOps                 []string `json:"userOps,omitempty"`
		RefundAddress           string   `json:"refundAddress,omitempty"`
		ContractCaller          string   `json:"contractCaller,omitempty"`
		FeeBps                  string   `json:"feeBps,omitempty"`
		FeeTakerAddress         string   `json:"feeTakerAddress,omitempty"`
		Refuel                  *bool    `json:"refuel,omitempty"`
		DestinationPayload      string   `json:"destinationPayload,omitempty"`
		DestinationGasLimit     string   `json:"destinationGasLimit,omitempty"`
		IncludeProvider         string   `json:"includeProvider,omitempty"`
		ExcludeProvider         string   `json:"excludeProvider,omitempty"`
		Exchange                string   `json:"exchange,omitempty"`
		IncludeQuoteRejections  *bool    `json:"includeQuoteRejections,omitempty"`
		Private                 *bool    `json:"private,omitempty"`
		SimulatedQuotesRequired *bool    `json:"simulatedQuotesRequired,omitempty"`
		SolanaSponsorAddress    string   `json:"solanaSponsorAddress,omitempty"`
		EquipmentNo             string   `json:"equipmentNo,omitempty"`
		SourceType              string   `json:"sourceType,omitempty"`
		FromCoinCode            string   `json:"fromCoinCode,omitempty"`
		ToCoinCode              string   `json:"toCoinCode,omitempty"`
		FixedRateFlag           *int     `json:"fixedRateFlag,omitempty"`
	}
	DexQuoteOut struct {
		Channel string      `json:"channel"`
		Routes  []*DexRoute `json:"routes"`
	}
	DexRoute struct {
		RouteId           string          `json:"routeId"`
		RequestHash       string          `json:"requestHash,omitempty"`
		Name              string          `json:"name"`
		AmountOut         string          `json:"amountOut"`
		AmountOutMin      string          `json:"amountOutMin"`
		Slippage          float64         `json:"slippage,omitempty"`
		SuggestedSlippage float64         `json:"suggestedSlippage"`
		FeeInUsd          string          `json:"feeInUsd"`
		EstimatedTime     int64           `json:"estimatedTime"`
		NeedBuild         bool            `json:"needBuild"`
		Gasless           bool            `json:"gasless,omitempty"`
		RequestId         string          `json:"requestId,omitempty"`
		TxData            *DexTx          `json:"txData,omitempty"`
		ApprovalData      *DexApproval    `json:"approvalData,omitempty"`
		UserOp            string          `json:"userOp,omitempty"`
		SignTypedData     json.RawMessage `json:"signTypedData,omitempty"`
		SubmitRequest     json.RawMessage `json:"submitRequest,omitempty"`
		TrustedSpenders   []string        `json:"trustedSpenders,omitempty"`
		PriceImpactPct    float64         `json:"priceImpactPct,omitempty"`
		Warnings          []DexWarning    `json:"warnings,omitempty"`
		GasLimit          string          `json:"gasLimit,omitempty"`
		RouteTags         []string        `json:"routeTags,omitempty"`
		// ExpiresAt is the route TTL as a unix timestamp; 0 when unknown.
		ExpiresAt int64 `json:"expiresAt,omitempty"`
	}

	DexWarning struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}

	DexBuildTxIn struct {
		Channel string `json:"channel"`
		RouteId string `json:"routeId"`
	}
	DexBuildTxOut struct {
		UserOp       string       `json:"userOp,omitempty"`
		TxData       *DexTx       `json:"txData"`
		ApprovalData *DexApproval `json:"approvalData,omitempty"`
	}

	DexCheckTxIn struct {
		Channel             string      `json:"channel"`
		HashType            DexHashType `json:"hashType"`
		Hash                string      `json:"hash"`
		FromChain           string      `json:"fromChain"`
		ToChain             string      `json:"toChain"`
		Bridge              string      `json:"bridge,omitempty"`
		IncludeQuoteDetails *bool       `json:"includeQuoteDetails,omitempty"`
	}
	DexCheckTxOut struct {
		Channel               string          `json:"channel"`
		Status                DexStatus       `json:"status"`
		ToChain               string          `json:"toChain"`
		ToHash                string          `json:"toHash"`
		FromHash              string          `json:"fromHash,omitempty"`
		ProviderStatus        string          `json:"providerStatus,omitempty"`
		ProviderStatusCode    string          `json:"providerStatusCode,omitempty"`
		OriginStatus          string          `json:"originStatus,omitempty"`
		DestinationStatus     string          `json:"destinationStatus,omitempty"`
		UserOp                string          `json:"userOp,omitempty"`
		RouteName             string          `json:"routeName,omitempty"`
		RouteLogoURI          string          `json:"routeLogoURI,omitempty"`
		IsDestPayloadExecuted *bool           `json:"isDestPayloadExecuted,omitempty"`
		QuoteDetails          json.RawMessage `json:"quoteDetails,omitempty"`
		Msg                   string          `json:"msg,omitempty"`
	}
)

type (
	DexTx struct {
		To       string `json:"to,omitempty"`
		Data     string `json:"data,omitempty"`
		Value    string `json:"value,omitempty"`
		GasPrice string `json:"gasPrice,omitempty"`
		GasLimit string `json:"gasLimit,omitempty"`
		FeeLimit string `json:"feeLimit,omitempty"`
	}
	DexApproval struct {
		Token   string `json:"token"`
		Spender string `json:"spender"`
		Amount  string `json:"amount"`
	}
)

type DexHashType string

const (
	DexHashTypeTxHash      DexHashType = "txHash"
	DexHashTypeRequestHash DexHashType = "requestHash"
)

func (h DexHashType) String() string { return string(h) }
func (h DexHashType) IsValid() bool {
	switch h {
	case DexHashTypeTxHash, DexHashTypeRequestHash:
		return true
	}
	return false
}

type DexStatus string

const (
	DexStatusPending   DexStatus = "Pending"
	DexStatusSucceeded DexStatus = "Succeeded"
	DexStatusRefunded  DexStatus = "Refunded"
	DexStatusFailed    DexStatus = "Failed"
	DexStatusNotFound  DexStatus = "NotFound"
)

func (h DexStatus) String() string { return string(h) }
