package relay

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	utils "github.com/kernelflowlabs/wallet-sdk/common/bigdec"
	dexmodel "github.com/kernelflowlabs/wallet-sdk/common/dexmodel"
	"github.com/kernelflowlabs/wallet-sdk/signing/tron"

	"github.com/shopspring/decimal"
)

type (
	Error struct {
		Message   string `json:"message"`
		ErrorCode string `json:"errorCode"`
	}
	Currency struct {
		ChainID  int    `json:"chainId"`
		Address  string `json:"address"`
		Symbol   string `json:"symbol"`
		Name     string `json:"name"`
		Decimals int    `json:"decimals"`
	}
	CurrencyAmount struct {
		Currency      Currency `json:"currency"`
		Amount        string   `json:"amount"`
		AmountUsd     string   `json:"amountUsd"`
		MinimumAmount string   `json:"minimumAmount"`
	}
	AppFee struct {
		Recipient string `json:"recipient"`
		Fee       string `json:"fee"`
	}
	ChainsRes struct {
		Chains []struct {
			ID       int    `json:"id"`
			Name     string `json:"name"`
			VMType   string `json:"vmType"`
			Disabled bool   `json:"disabled"`
		} `json:"chains"`
	}
	QuoteReq struct {
		User                string   `json:"user"`
		Recipient           string   `json:"recipient"`
		OriginChainId       int      `json:"originChainId"`
		DestinationChainId  int      `json:"destinationChainId"`
		OriginCurrency      string   `json:"originCurrency"`
		DestinationCurrency string   `json:"destinationCurrency"`
		Amount              string   `json:"amount"`
		TradeType           string   `json:"tradeType"`
		SlippageTolerance   string   `json:"slippageTolerance,omitempty"`
		Referrer            string   `json:"referrer,omitempty"`
		AppFees             []AppFee `json:"appFees,omitempty"`
		RefundTo            string   `json:"refundTo,omitempty"`
	}
	QuoteRes struct {
		Error
		RequestId string `json:"requestId"`
		Steps     []struct {
			ID    string `json:"id"`
			Kind  string `json:"kind"`
			Items []struct {
				Status string          `json:"status"`
				Data   json.RawMessage `json:"data"`
			} `json:"items"`
		} `json:"steps"`
		Fees    map[string]CurrencyAmount `json:"fees"`
		Details struct {
			Operation   string         `json:"operation"`
			CurrencyIn  CurrencyAmount `json:"currencyIn"`
			CurrencyOut CurrencyAmount `json:"currencyOut"`
			TotalImpact struct {
				Usd     string `json:"usd"`
				Percent string `json:"percent"`
			} `json:"totalImpact"`
			SlippageTolerance struct {
				Total string `json:"total"`
			} `json:"slippageTolerance"`
			TimeEstimate int64 `json:"timeEstimate"`
		} `json:"details"`
	}
	EvmStepData struct {
		From     string `json:"from"`
		To       string `json:"to"`
		Data     string `json:"data"`
		Value    string `json:"value"`
		ChainID  int    `json:"chainId"`
		Gas      string `json:"gas"`
		GasPrice string `json:"gasPrice"`
	}
	TronStepData struct {
		Type      string `json:"type"`
		Parameter struct {
			OwnerAddress    string `json:"owner_address"`
			ContractAddress string `json:"contract_address"`
			CallValue       int64  `json:"call_value"`
			Data            string `json:"data"`
		} `json:"parameter"`
	}
	StatusRes struct {
		Status             string   `json:"status"`
		Details            string   `json:"details"`
		InTxHashes         []string `json:"inTxHashes"`
		TxHashes           []string `json:"txHashes"`
		OriginChainId      int      `json:"originChainId"`
		DestinationChainId int      `json:"destinationChainId"`
		FailReason         string   `json:"failReason"`
		RefundFailReason   string   `json:"refundFailReason"`
	}
)

const (
	channelName    = "relay"
	tradeTypeIn    = "EXACT_INPUT"
	stepApprove    = "approve"
	stepDeposit    = "deposit"
	tronStepType   = "TriggerSmartContract"
	tronChain      = "TRX"
	tronUserOp     = "tron_tx"
	feeBucketGas   = "gas"
	feeBucketRelay = "relayer"
	feeBucketApp   = "app"
)

func (c *Client) toRelayQuoteReq(in *dexmodel.DexQuoteIn) (*QuoteReq, error) {
	adjustNativeToken(in)
	originChain, ok := chainID(in.FromChain)
	if !ok {
		return nil, fmt.Errorf("unsupported Relay chain %q", in.FromChain)
	}
	destinationChain, ok := chainID(in.ToChain)
	if !ok {
		return nil, fmt.Errorf("unsupported Relay chain %q", in.ToChain)
	}
	var slippage string
	if in.Slippage != "" {
		slippageDecimal, err := decimal.NewFromString(in.Slippage)
		if err != nil {
			return nil, fmt.Errorf("invalid slippage %q", in.Slippage)
		}
		slippage = slippageDecimal.Shift(2).Truncate(0).String()
	}
	req := &QuoteReq{
		User:                in.FromAddress,
		Recipient:           in.ToAddress,
		OriginChainId:       originChain,
		DestinationChainId:  destinationChain,
		OriginCurrency:      in.FromToken,
		DestinationCurrency: in.ToToken,
		Amount:              in.FromAmount,
		TradeType:           tradeTypeIn,
		SlippageTolerance:   slippage,
		Referrer:            c.referrer,
		RefundTo:            in.RefundAddress,
	}
	if fee := appFee(in.FeeRate, in.FeeReceiver); fee != nil {
		req.AppFees = []AppFee{*fee}
	}
	return req, nil
}

func appFee(feeRate, receiver string) *AppFee {
	if receiver == "" || feeRate == "" {
		return nil
	}
	rate, err := decimal.NewFromString(feeRate)
	if err != nil || !rate.IsPositive() {
		return nil
	}
	return &AppFee{Recipient: receiver, Fee: rate.Shift(2).Truncate(0).String()}
}

func (c *Client) toStandardQuoteRes(res *QuoteRes) *dexmodel.DexQuoteOut {
	chain, ok := idChainMapper[strconv.Itoa(res.Details.CurrencyIn.Currency.ChainID)]
	if !ok {
		return nil
	}
	approveStep, depositStep := res.step(stepApprove), res.step(stepDeposit)
	if depositStep == nil {
		return nil
	}

	var txData *dexmodel.DexTx
	var userOp string
	if chain == tronChain {
		txData = tronTxData(depositStep)
		userOp = tronUserOp
	} else {
		txData = evmTxData(depositStep)
	}
	if txData == nil {
		return nil
	}

	var approvalData *dexmodel.DexApproval
	if approveStep != nil {
		if approve := evmTxData(approveStep); approve != nil && approve.To != "" {
			approvalData = &dexmodel.DexApproval{
				Token:   approve.To,
				Spender: txData.To,
				Amount:  res.Details.CurrencyIn.Amount,
			}
		}
	}

	var trusted []string
	if approvalData != nil {
		trusted = append(trusted, approvalData.Spender)
	}
	if txData.To != "" && (approvalData == nil || txData.To != approvalData.Spender) {
		trusted = append(trusted, txData.To)
	}

	slippage, _ := decimal.NewFromString(res.Details.SlippageTolerance.Total)
	route := &dexmodel.DexRoute{
		RouteId:           res.RequestId,
		Name:              channelName,
		AmountOut:         res.Details.CurrencyOut.Amount,
		AmountOutMin:      res.Details.CurrencyOut.MinimumAmount,
		Slippage:          slippage.Shift(-2).InexactFloat64(),
		SuggestedSlippage: slippage.Shift(-2).InexactFloat64(),
		FeeInUsd:          fmt.Sprintf("%.2f", res.feeUsd()),
		EstimatedTime:     res.Details.TimeEstimate,
		NeedBuild:         false,
		UserOp:            userOp,
		TxData:            txData,
		ApprovalData:      approvalData,
		TrustedSpenders:   trusted,
		PriceImpactPct:    priceImpactPct(res.Details.TotalImpact.Percent),
	}

	return &dexmodel.DexQuoteOut{
		Channel: channelName,
		Routes:  []*dexmodel.DexRoute{route},
	}
}

func (res *QuoteRes) step(id string) json.RawMessage {
	for _, s := range res.Steps {
		if s.ID != id || len(s.Items) == 0 {
			continue
		}
		return s.Items[0].Data
	}
	return nil
}

func (res *QuoteRes) feeUsd() float64 {
	var total float64
	for _, bucket := range []string{feeBucketGas, feeBucketRelay, feeBucketApp} {
		fee, ok := res.Fees[bucket]
		if !ok {
			continue
		}
		amount, err := strconv.ParseFloat(fee.AmountUsd, 64)
		if err != nil {
			continue
		}
		total += amount
	}
	return total
}

func priceImpactPct(percent string) float64 {
	impact, err := decimal.NewFromString(percent)
	if err != nil || !impact.IsNegative() {
		return 0
	}
	return impact.Neg().InexactFloat64()
}

func evmTxData(raw json.RawMessage) *dexmodel.DexTx {
	data := &EvmStepData{}
	if err := json.Unmarshal(raw, data); err != nil {
		return nil
	}
	if data.To == "" || !strings.HasPrefix(data.Data, "0x") {
		return nil
	}
	value := utils.HexToDecimal(data.Value)
	if value == "0" {
		value = ""
	}
	gasPrice := utils.HexToDecimal(data.GasPrice)
	if gasPrice == "0" {
		gasPrice = ""
	}
	gasLimit := utils.HexToDecimal(data.Gas)
	if gasLimit == "0" {
		gasLimit = ""
	}
	return &dexmodel.DexTx{
		To:       data.To,
		Data:     data.Data,
		Value:    value,
		GasPrice: gasPrice,
		GasLimit: gasLimit,
	}
}

func tronTxData(raw json.RawMessage) *dexmodel.DexTx {
	data := &TronStepData{}
	if err := json.Unmarshal(raw, data); err != nil {
		return nil
	}
	if data.Type != tronStepType || data.Parameter.CallValue < 0 {
		return nil
	}
	contract := tronBase58(data.Parameter.ContractAddress)
	if contract == "" {
		return nil
	}
	if _, err := hex.DecodeString(data.Parameter.Data); err != nil || data.Parameter.Data == "" {
		return nil
	}
	value := ""
	if data.Parameter.CallValue > 0 {
		value = strconv.FormatInt(data.Parameter.CallValue, 10)
	}
	return &dexmodel.DexTx{
		To:       contract,
		Data:     data.Parameter.Data,
		Value:    value,
		FeeLimit: strconv.FormatInt(tron.MaxFeeLimitSun, 10),
	}
}

func tronBase58(address string) string {
	if len(address) == 34 && tron.ValidAddress(address) {
		return address
	}
	converted := tron.ConvertFromHex(address)
	if !tron.ValidAddress(converted) {
		return ""
	}
	return converted
}

func (c *Client) toStandardStatusRes(res *StatusRes) *dexmodel.DexCheckTxOut {
	var finalStatus dexmodel.DexStatus
	switch res.Status {
	case "success":
		finalStatus = dexmodel.DexStatusSucceeded
	case "refund":
		finalStatus = dexmodel.DexStatusRefunded
	case "failure":
		finalStatus = dexmodel.DexStatusFailed
	case "unknown":
		finalStatus = dexmodel.DexStatusNotFound
	default:
		finalStatus = dexmodel.DexStatusPending
	}

	out := &dexmodel.DexCheckTxOut{
		Channel:        channelName,
		Status:         finalStatus,
		ProviderStatus: res.Status,
		Msg:            res.Details,
	}
	if len(res.TxHashes) > 0 {
		out.ToHash = res.TxHashes[len(res.TxHashes)-1]
	}
	if len(res.InTxHashes) > 0 {
		out.FromHash = res.InTxHashes[0]
	}
	if reason := failReason(res); reason != "" {
		out.Msg = reason
	}
	if res.DestinationChainId != 0 {
		out.ToChain = idChainMapper[strconv.Itoa(res.DestinationChainId)]
	}
	return out
}

func failReason(res *StatusRes) string {
	for _, reason := range []string{res.RefundFailReason, res.FailReason} {
		if reason != "" && reason != "N/A" {
			return reason
		}
	}
	return ""
}

var idChainMapper = map[string]string{
	"1":         "ETH",
	"10":        "OP",
	"25":        "CRONOS",
	"56":        "BNB",
	"100":       "GNOSIS",
	"130":       "UNICHAIN",
	"137":       "POLYGON",
	"143":       "MON",
	"146":       "SONIC",
	"169":       "MANTA",
	"196":       "XLAYER",
	"288":       "BOBA",
	"324":       "ZKSYNC",
	"360":       "SHAPE",
	"480":       "WORLDCHAIN",
	"747":       "FLOWEVM",
	"988":       "STABLE",
	"999":       "HYPEEVM",
	"1088":      "METIS",
	"1135":      "LISK",
	"1868":      "SONEIUM",
	"2020":      "RONIN",
	"2741":      "ABSTRACT",
	"2818":      "MORPH",
	"4217":      "TEMPO",
	"4326":      "MEGAETH",
	"4663":      "ROBINHOOD",
	"5000":      "MANTLE",
	"5031":      "SOMNIA",
	"5042":      "ARC",
	"8453":      "BASE",
	"9745":      "PLASMA",
	"33139":     "APECHAIN",
	"34443":     "MODE",
	"42018":     "MYTHOS",
	"42161":     "ARB",
	"42220":     "CELO",
	"43114":     "AVAXC",
	"48900":     "ZIRCUIT",
	"57073":     "INK",
	"59144":     "LINEA",
	"60808":     "BOB",
	"80094":     "BERA",
	"97477":     "DOMA",
	"98866":     "PLUME",
	"534352":    "SCROLL",
	"685689":    "GENSYN",
	"747474":    "KATANA",
	"5064014":   "ETHEREAL",
	"7777777":   "ZORA",
	"728126428": "TRX",
}

var idChainMapperReverse = func() map[string]string {
	m := make(map[string]string, len(idChainMapper))
	for k, v := range idChainMapper {
		m[v] = k
	}
	return m
}()

func chainID(chain string) (int, bool) {
	id, ok := idChainMapperReverse[strings.ToUpper(strings.TrimSpace(chain))]
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(id)
	if err != nil {
		return 0, false
	}
	return n, true
}

var nativeTokens = map[string]string{
	"LOCAL": "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
	"EVM":   "0x0000000000000000000000000000000000000000",
	"TRX":   "T9yD14Nj9j7xAB4dbGeiX9h8unkKHxuWwb",
}

func nativeTokenByChain(chain string) string {
	if chain == tronChain {
		return nativeTokens["TRX"]
	}
	return nativeTokens["EVM"]
}

func adjustNativeToken(in *dexmodel.DexQuoteIn) {
	fromTokenLower := strings.ToLower(in.FromToken)
	if fromTokenLower == nativeTokens["LOCAL"] || fromTokenLower == nativeTokens["EVM"] {
		in.FromToken = nativeTokenByChain(strings.ToUpper(in.FromChain))
	}

	toTokenLower := strings.ToLower(in.ToToken)
	if toTokenLower == nativeTokens["LOCAL"] || toTokenLower == nativeTokens["EVM"] {
		in.ToToken = nativeTokenByChain(strings.ToUpper(in.ToChain))
	}
}
