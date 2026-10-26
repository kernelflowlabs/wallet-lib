package bungee

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	dexmodel "github.com/kernelflowlabs/wallet-lib/common/dexmodel"
	"github.com/kernelflowlabs/wallet-lib/signing/tron"
	"github.com/shopspring/decimal"
)

type UserOp string

type FlexibleString string

const (
	UserOpTx          UserOp = "tx"
	UserOpDeposit     UserOp = "deposit"
	UserOpCEXWithdraw UserOp = "cex-withdraw"
)

type (
	QuoteRequest struct {
		UserOps                 []UserOp `json:"userOps"`
		OriginChainId           string   `json:"originChainId,omitempty"`
		DestinationChainId      string   `json:"destinationChainId"`
		InputToken              string   `json:"inputToken"`
		OutputToken             string   `json:"outputToken"`
		UserAddress             string   `json:"userAddress,omitempty"`
		ReceiverAddress         string   `json:"receiverAddress"`
		InputAmount             string   `json:"inputAmount"`
		Slippage                string   `json:"slippage,omitempty"`
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
	}
	Token struct {
		ChainId  int    `json:"chainId"`
		Address  string `json:"address"`
		Name     string `json:"name"`
		Symbol   string `json:"symbol"`
		Decimals int    `json:"decimals"`
		LogoURI  string `json:"logoURI"`
		Icon     string `json:"icon"`
	}
	TokenAmount struct {
		Token        Token   `json:"token"`
		Amount       string  `json:"amount"`
		MinAmountOut string  `json:"minAmountOut,omitempty"`
		PriceInUsd   float64 `json:"priceInUsd"`
		ValueInUsd   float64 `json:"valueInUsd"`
	}
	Protocol struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		Icon        string `json:"icon"`
	}
	RouteLegDetails struct {
		Protocol           Protocol `json:"protocol"`
		InputTokenAddress  string   `json:"inputTokenAddress"`
		OutputTokenAddress string   `json:"outputTokenAddress"`
		AmountIn           string   `json:"amountIn"`
		AmountOut          string   `json:"amountOut"`
		MinAmountOut       string   `json:"minAmountOut"`
		Slippage           float64  `json:"slippage"`
	}
	RouteDetails struct {
		DexDetails    *RouteLegDetails `json:"dexDetails"`
		BridgeDetails *RouteLegDetails `json:"bridgeDetails"`
		FeeDetails    json.RawMessage  `json:"feeDetails"`
	}
	Approval struct {
		SpenderAddress string `json:"spenderAddress"`
		Amount         string `json:"amount"`
		TokenAddress   string `json:"tokenAddress"`
		UserAddress    string `json:"userAddress"`
	}
	TxData struct {
		Kind   string          `json:"kind"`
		Object json.RawMessage `json:"object"`
	}
	GasFee struct {
		GasToken     Token          `json:"gasToken"`
		GasLimit     FlexibleString `json:"gasLimit"`
		GasPrice     FlexibleString `json:"gasPrice"`
		EstimatedFee FlexibleString `json:"estimatedFee"`
		FeeInUsd     float64        `json:"feeInUsd"`
	}
	StatusCheck struct {
		Endpoint       string `json:"endpoint"`
		Method         string `json:"method"`
		IntervalSec    int    `json:"intervalSec"`
		MaxDurationSec int    `json:"maxDurationSec"`
	}
	DepositDetails struct {
		ChainId        int             `json:"chainId"`
		Token          Token           `json:"token"`
		Amount         string          `json:"amount"`
		TransferType   string          `json:"transferType"`
		DepositAddress string          `json:"depositAddress"`
		Memo           json.RawMessage `json:"memo"`
	}
	QuoteRejection struct {
		ProviderId string `json:"providerId"`
		Reason     string `json:"reason"`
	}
	QuoteResponse struct {
		Success    bool      `json:"success"`
		StatusCode int       `json:"statusCode"`
		Message    string    `json:"message,omitempty"`
		Result     *QuoteOut `json:"result"`
	}
	QuoteOut struct {
		OriginChainId      int              `json:"originChainId"`
		DestinationChainId int              `json:"destinationChainId"`
		QuoteType          string           `json:"quoteType"`
		UserAddress        string           `json:"userAddress"`
		ReceiverAddress    string           `json:"receiverAddress"`
		Input              TokenAmount      `json:"input"`
		Routes             []QuoteRoute     `json:"routes"`
		QuoteRejections    []QuoteRejection `json:"quoteRejections"`
	}
	QuoteRoute struct {
		UserOp             string          `json:"userOp"`
		QuoteId            string          `json:"quoteId"`
		ExpiresAt          int64           `json:"expiresAt"`
		Output             TokenAmount     `json:"output"`
		EstimatedTime      float64         `json:"estimatedTime"`
		Slippage           float64         `json:"slippage"`
		SuggestedSlippage  float64         `json:"suggestedSlippage"`
		RouteTags          []string        `json:"routeTags"`
		RouteDetails       RouteDetails    `json:"routeDetails"`
		Approval           *Approval       `json:"approval"`
		TxData             TxData          `json:"txData"`
		GasFee             GasFee          `json:"gasFee"`
		StatusCheck        StatusCheck     `json:"statusCheck"`
		ActivationRequired *bool           `json:"activationRequired"`
		Deposit            *DepositDetails `json:"deposit"`
		RefundAddress      string          `json:"refundAddress"`
		SignTypedData      json.RawMessage `json:"signTypedData"`
	}
	evmTxObject struct {
		ChainId int            `json:"chainId"`
		To      string         `json:"to"`
		Data    string         `json:"data"`
		Value   FlexibleString `json:"value"`
	}
	StatusRequest struct {
		QuoteId             string `json:"quoteId,omitempty"`
		SrcTxHash           string `json:"srcTxHash,omitempty"`
		IncludeQuoteDetails *bool  `json:"includeQuoteDetails,omitempty"`
	}
	StatusResponse struct {
		Success    bool       `json:"success"`
		StatusCode int        `json:"statusCode"`
		Message    string     `json:"message,omitempty"`
		Result     *StatusOut `json:"result"`
	}
	StatusOut struct {
		QuoteId               string             `json:"quoteId"`
		UserOp                string             `json:"userOp"`
		Status                string             `json:"status"`
		StatusCode            string             `json:"statusCode"`
		Origin                StatusOrigin       `json:"origin"`
		Destination           StatusDestination  `json:"destination"`
		RouteDetails          StatusRouteDetails `json:"routeDetails"`
		Refund                *StatusRefund      `json:"refund"`
		IsDestPayloadExecuted *bool              `json:"isDestPayloadExecuted"`
		QuoteDetails          json.RawMessage    `json:"quoteDetails"`
	}
	StatusOrigin struct {
		ChainId     int           `json:"chainId"`
		Status      string        `json:"status"`
		TxHash      string        `json:"txHash"`
		Timestamp   *int64        `json:"timestamp"`
		UserAddress string        `json:"userAddress"`
		Input       []TokenAmount `json:"input"`
	}
	StatusDestination struct {
		ChainId         int           `json:"chainId"`
		Status          string        `json:"status"`
		TxHash          string        `json:"txHash"`
		Timestamp       *int64        `json:"timestamp"`
		ReceiverAddress string        `json:"receiverAddress"`
		Output          []TokenAmount `json:"output"`
	}
	StatusRouteDetails struct {
		Name    string `json:"name"`
		LogoURI string `json:"logoURI"`
	}
	StatusRefund struct {
		ChainId   int    `json:"chainId"`
		Status    string `json:"status"`
		TxHash    string `json:"txHash"`
		Timestamp *int64 `json:"timestamp"`
	}
)

func (f *FlexibleString) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*f = ""
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = FlexibleString(s)
		return nil
	}
	*f = FlexibleString(b)
	return nil
}

func (f FlexibleString) String() string {
	return string(f)
}

func (c *Client) toSocketQuoteReq(in *dexmodel.DexQuoteIn) (*QuoteRequest, error) {
	if in == nil {
		return nil, fmt.Errorf("in is nil")
	}

	userOps := make([]UserOp, 0, len(in.UserOps))
	for _, op := range in.UserOps {
		userOps = append(userOps, UserOp(op))
	}
	if len(userOps) == 0 {
		userOps = []UserOp{UserOpTx}
	}

	var fromChainId string
	if in.FromChain != "" {
		var ok bool
		fromChainId, ok = chainID(in.FromChain)
		if !ok {
			return nil, fmt.Errorf("invalid fromChain: %s", in.FromChain)
		}
	}
	toChainId, ok := chainID(in.ToChain)
	if !ok {
		return nil, fmt.Errorf("invalid toChain: %s", in.ToChain)
	}

	feeBps, feeTakerAddress, err := socketFeeParams(in)
	if err != nil {
		return nil, err
	}

	return &QuoteRequest{
		UserOps:                 userOps,
		OriginChainId:           fromChainId,
		DestinationChainId:      toChainId,
		InputToken:              in.FromToken,
		OutputToken:             in.ToToken,
		UserAddress:             in.FromAddress,
		ReceiverAddress:         in.ToAddress,
		InputAmount:             in.FromAmount,
		Slippage:                in.Slippage,
		RefundAddress:           in.RefundAddress,
		ContractCaller:          in.ContractCaller,
		FeeBps:                  feeBps,
		FeeTakerAddress:         feeTakerAddress,
		Refuel:                  in.Refuel,
		DestinationPayload:      in.DestinationPayload,
		DestinationGasLimit:     in.DestinationGasLimit,
		IncludeProvider:         in.IncludeProvider,
		ExcludeProvider:         in.ExcludeProvider,
		Exchange:                in.Exchange,
		IncludeQuoteRejections:  in.IncludeQuoteRejections,
		Private:                 in.Private,
		SimulatedQuotesRequired: in.SimulatedQuotesRequired,
		SolanaSponsorAddress:    in.SolanaSponsorAddress,
	}, nil
}

func socketFeeParams(in *dexmodel.DexQuoteIn) (string, string, error) {
	feeTakerAddress := in.FeeTakerAddress
	if feeTakerAddress == "" {
		feeTakerAddress = in.FeeReceiver
	} else if in.FeeReceiver != "" && !strings.EqualFold(feeTakerAddress, in.FeeReceiver) {
		return "", "", fmt.Errorf("feeTakerAddress and feeReceiver differ")
	}

	feeBps := in.FeeBps
	if in.FeeRate == "" {
		return feeBps, feeTakerAddress, nil
	}
	feeRate, err := decimal.NewFromString(in.FeeRate)
	if err != nil {
		return "", "", fmt.Errorf("invalid feeRate: %w", err)
	}
	if feeRate.IsNegative() {
		return "", "", fmt.Errorf("feeRate must not be negative")
	}
	if feeBps != "" && !feeRate.IsZero() {
		return "", "", fmt.Errorf("feeBps and feeRate cannot both be set")
	}
	if feeBps == "" && !feeRate.IsZero() {
		feeBps = feeRate.Mul(decimal.NewFromInt(100)).String()
	}
	if feeBps == "" && in.FeeTakerAddress == "" {
		feeTakerAddress = ""
	}
	return feeBps, feeTakerAddress, nil
}

func routeName(rt *QuoteRoute) string {
	if b := rt.RouteDetails.BridgeDetails; b != nil && b.Protocol.DisplayName != "" {
		return b.Protocol.DisplayName
	}
	if d := rt.RouteDetails.DexDetails; d != nil && d.Protocol.DisplayName != "" {
		return d.Protocol.DisplayName
	}
	return "Socket"
}

func (c *Client) toStandardQuoteRes(res *QuoteOut) *dexmodel.DexQuoteOut {
	if res == nil {
		return nil
	}

	out := &dexmodel.DexQuoteOut{
		Channel: "bungee",
		Routes:  make([]*dexmodel.DexRoute, 0, len(res.Routes)),
	}
	for i := range res.Routes {
		rt := &res.Routes[i]
		route := &dexmodel.DexRoute{
			RouteId:           rt.QuoteId,
			Name:              routeName(rt),
			AmountOut:         rt.Output.Amount,
			AmountOutMin:      rt.Output.MinAmountOut,
			Slippage:          rt.Slippage,
			SuggestedSlippage: rt.SuggestedSlippage,
			FeeInUsd:          fmt.Sprintf("%.2f", rt.GasFee.FeeInUsd),
			EstimatedTime:     int64(math.Round(rt.EstimatedTime)),
			NeedBuild:         false,
			UserOp:            rt.UserOp,
			SignTypedData:     rt.SignTypedData,
			GasLimit:          string(rt.GasFee.GasLimit),
			RouteTags:         append([]string(nil), rt.RouteTags...),
			ExpiresAt:         rt.ExpiresAt,
		}
		switch rt.TxData.Kind {
		case tronTxKind:
			var obj evmTxObject
			if err := json.Unmarshal(rt.TxData.Object, &obj); err != nil {
				continue
			}
			to := tronAddress(obj.To)
			feeLimit := tronFeeLimitSun(rt.GasFee)
			if to == "" || !strings.HasPrefix(obj.Data, "0x") || len(obj.Data) <= 2 || feeLimit == "" {
				continue
			}
			value := string(obj.Value)
			if value == "0" {
				value = ""
			}
			route.GasLimit = ""
			route.TxData = &dexmodel.DexTx{To: to, Data: obj.Data, Value: value, FeeLimit: feeLimit}
			route.UserOp = tronTxKind
		case "", "evm_tx":
			var obj evmTxObject
			if err := json.Unmarshal(rt.TxData.Object, &obj); err == nil && obj.To != "" {
				route.TxData = &dexmodel.DexTx{
					To:       obj.To,
					Data:     obj.Data,
					Value:    string(obj.Value),
					GasPrice: string(rt.GasFee.GasPrice),
					GasLimit: string(rt.GasFee.GasLimit),
				}
			}
		default:
			if len(rt.TxData.Object) > 0 {
				route.TxData = &dexmodel.DexTx{Data: string(rt.TxData.Object)}
				route.UserOp = rt.TxData.Kind
			}
		}
		if rt.Approval != nil && rt.Approval.SpenderAddress != "" {
			token, spender := rt.Approval.TokenAddress, rt.Approval.SpenderAddress
			if route.UserOp == tronTxKind {
				token, spender = tronAddress(token), tronAddress(spender)
			}
			if token != "" && spender != "" {
				route.ApprovalData = &dexmodel.DexApproval{
					Token:   token,
					Spender: spender,
					Amount:  rt.Approval.Amount,
				}
				route.TrustedSpenders = append(route.TrustedSpenders, spender)
			}
		}
		out.Routes = append(out.Routes, route)
	}
	return out
}

const tronTxKind = "tron_tx"

func tronAddress(address string) string {
	address = strings.TrimSpace(address)
	if len(address) == 34 && tron.ValidAddress(address) {
		return address
	}
	payload := strings.TrimPrefix(strings.TrimPrefix(address, "0x"), "0X")
	if len(payload) != 40 {
		return ""
	}
	converted := tron.ConvertFromHex("41" + payload)
	if len(converted) != 34 || !tron.ValidAddress(converted) {
		return ""
	}
	return converted
}

func tronFeeLimitSun(fee GasFee) string {
	sun, err := strconv.ParseInt(strings.TrimSpace(string(fee.EstimatedFee)), 10, 64)
	if err != nil || sun <= 0 {
		limit, limitErr := strconv.ParseInt(strings.TrimSpace(string(fee.GasLimit)), 10, 64)
		price, priceErr := strconv.ParseInt(strings.TrimSpace(string(fee.GasPrice)), 10, 64)
		if limitErr != nil || priceErr != nil || limit <= 0 || price <= 0 || limit > tron.ProtocolMaxFeeLimitSun/price {
			return ""
		}
		sun = limit * price
	}
	feeLimit, ok := tron.FeeLimitFromSun(sun, tron.DefaultFeeLimitHeadroom)
	if !ok {
		return ""
	}
	return strconv.FormatInt(feeLimit, 10)
}

var idChainMapper = map[string]string{
	"728126428":  "TRX",
	"1":          "ETH",
	"56":         "BNB",
	"89999":      "SOL",
	"137":        "POLYGON",
	"43114":      "AVAXC",
	"42161":      "ARB",
	"10":         "OP",
	"8453":       "BASE",
	"59144":      "LINEA",
	"324":        "ZKSYNC",
	"1101":       "ZKEVM",
	"534352":     "SCROLL",
	"5000":       "MANTLE",
	"169":        "MANTA",
	"81457":      "BLAST",
	"34443":      "MODE",
	"252":        "FRAXTAL",
	"7777777":    "ZORA",
	"666666666":  "DEGEN",
	"1088":       "METIS",
	"100":        "GNOSIS",
	"250":        "FTM",
	"1284":       "MOONBEAM",
	"1285":       "MOONRIVER",
	"42220":      "CELO",
	"1313161554": "AURORA",
	"25":         "CRONOS",
	"288":        "BOBA",
	"122":        "FUSE",
	"1666600000": "HARMONY",
	"2222":       "KAVA",
	"321":        "KCC",
	"106":        "VELAS",
	"128":        "HECO",
	"66":         "OKEX",
	"40":         "TELOS",
	"167000":     "TAIKO",
	"196":        "XLAYER",
	"480":        "WORLDCHAIN",
	"1135":       "LISK",
	"1868":       "SONEIUM",
	"1923":       "SWELL",
	"60808":      "BOB",
	"33139":      "APECHAIN",
	"2741":       "ABSTRACT",
	"4663":       "ROBINHOOD",
	"130":        "UNICHAIN",
	"57073":      "INK",
}

func chainID(chain string) (string, bool) {
	id, ok := idChainMapperReverse[strings.ToUpper(strings.TrimSpace(chain))]
	return id, ok
}

var idChainMapperReverse = func() map[string]string {
	m := make(map[string]string, len(idChainMapper))
	for k, v := range idChainMapper {
		m[v] = k
	}
	return m
}()
