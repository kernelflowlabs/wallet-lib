package lifi

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	utils "github.com/kernelflowlabs/wallet-lib/common/bigdec"
	dexmodel "github.com/kernelflowlabs/wallet-lib/common/dexmodel"
	"github.com/kernelflowlabs/wallet-lib/signing/tron"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

type (
	Error struct {
		Message string `json:"message"`
		Code    int    `json:"code"`
	}
	ChainsRes struct {
		Chains []struct {
			ID        int    `json:"id"`
			Key       string `json:"key"`
			Name      string `json:"name"`
			ChainType string `json:"chainType"`
			Coin      string `json:"coin"`
		} `json:"chains"`
	}
	TokensRes struct {
		Tokens map[string][]Token `json:"tokens"`
	}
	Token struct {
		Address  string `json:"address"`
		Symbol   string `json:"symbol"`
		Decimals int    `json:"decimals"`
		ChainID  int    `json:"chainId"`
		Name     string `json:"name"`
		PriceUSD string `json:"priceUSD"`
		LogoURI  string `json:"logoURI"`
	}
	QuoteReq struct {
		FromChain   string
		ToChain     string
		FromToken   string
		ToToken     string
		FromAmount  string
		FromAddress string
		ToAddress   string
		Slippage    string
		Fee         string
		// FromAmountForGas — fraction of FromAmount (smallest unit) routed
		// through LiFi's GasZip integration so the destination chain
		// receives some native gas. Empty disables refuel.
		FromAmountForGas string
	}
	QuoteRes struct {
		Error
		ID            string `json:"id"`
		Type          string `json:"type"`
		Tool          string `json:"tool"`
		TransactionId string `json:"transactionId"`
		ToolDetails   struct {
			Key     string `json:"key"`
			Name    string `json:"name"`
			LogoURI string `json:"logoURI"`
		} `json:"toolDetails"`
		Action struct {
			FromChainID int     `json:"fromChainId"`
			ToChainID   int     `json:"toChainId"`
			FromAmount  string  `json:"fromAmount"`
			Slippage    float64 `json:"slippage"`
			FromToken   struct {
				Address  string `json:"address"`
				Symbol   string `json:"symbol"`
				Decimals int    `json:"decimals"`
			} `json:"fromToken"`
			ToToken struct {
				Address  string `json:"address"`
				Symbol   string `json:"symbol"`
				Decimals int    `json:"decimals"`
			} `json:"toToken"`
		} `json:"action"`
		Estimate struct {
			FromAmount        string `json:"fromAmount"`
			ToAmount          string `json:"toAmount"`
			ToAmountMin       string `json:"toAmountMin"`
			FromAmountUSD     string `json:"fromAmountUSD"`
			ToAmountUSD       string `json:"toAmountUSD"`
			ExecutionDuration int64  `json:"executionDuration"`
			GasCosts          []struct {
				Type      string `json:"type"`
				Estimate  string `json:"estimate"`
				Amount    string `json:"amount"`
				AmountUSD string `json:"amountUSD"`
			} `json:"gasCosts"`
			FeeCosts []struct {
				Name      string `json:"name"`
				Amount    string `json:"amount"`
				AmountUSD string `json:"amountUSD"`
			} `json:"feeCosts"`
			ApprovalAddress string `json:"approvalAddress"`
		} `json:"estimate"`
		TransactionRequest struct {
			To         string `json:"to"`
			Data       string `json:"data"`
			Value      string `json:"value"`
			From       string `json:"from,omitempty"`
			ChainID    int    `json:"chainId,omitempty"`
			GasPrice   string `json:"gasPrice,omitempty"`
			GasLimit   string `json:"gasLimit,omitempty"`
			CustomData struct {
				ContractType    string          `json:"contractType"`
				TronTransaction json.RawMessage `json:"tronTransaction"`
			} `json:"customData,omitempty"`
		} `json:"transactionRequest"`
	}
	StatusRes struct {
		Status       string `json:"status"`
		Substatus    string `json:"substatus"`
		SubstatusMsg string `json:"substatusMessage"`

		Sending struct {
			TxHash  string `json:"txHash"`
			ChainID int    `json:"chainId"`
			Amount  string `json:"amount"`
		} `json:"sending"`

		Receiving struct {
			TxHash  string `json:"txHash"`
			ChainID int    `json:"chainId"`
			Amount  string `json:"amount"`
		} `json:"receiving"`
	}
)

func (c *Client) toLifiQuoteReq(in *dexmodel.DexQuoteIn) *QuoteReq {
	adjustNativeToken(in)
	fromChain, ok := chainID(in.FromChain)
	if !ok {
		return nil
	}
	toChain, ok := chainID(in.ToChain)
	if !ok {
		return nil
	}
	var slippage string
	if in.Slippage != "" {
		slippageDecimal, err := decimal.NewFromString(in.Slippage)
		if err != nil {
			return nil
		}
		slippage = slippageDecimal.Shift(-2).String()
	}
	fee, err := utils.BigDiv(in.FeeRate, "100")
	if err != nil {
		return nil
	}
	return &QuoteReq{
		FromChain:        fromChain,
		ToChain:          toChain,
		FromToken:        in.FromToken,
		ToToken:          in.ToToken,
		FromAddress:      in.FromAddress,
		ToAddress:        in.ToAddress,
		FromAmount:       in.FromAmount,
		Slippage:         slippage,
		Fee:              fee,
		FromAmountForGas: in.GasOnDestination,
	}
}

func (c *Client) toStandardStatusRes(res *StatusRes) *dexmodel.DexCheckTxOut {
	var finalStatus dexmodel.DexStatus
	switch res.Status {
	case "DONE":
		if res.Substatus == "REFUNDED" {
			finalStatus = dexmodel.DexStatusRefunded
		} else {
			finalStatus = dexmodel.DexStatusSucceeded
		}
	case "FAILED":
		if res.Substatus == "REFUNDED" {
			finalStatus = dexmodel.DexStatusRefunded
		} else {
			finalStatus = dexmodel.DexStatusFailed
		}
	case "INVALID":
		finalStatus = dexmodel.DexStatusFailed
	case "PENDING", "NOT_FOUND":
		// LI.FI defines NOT_FOUND as either unknown or not yet mined, so keep
		// polling rather than treating it as a terminal failure.
		finalStatus = dexmodel.DexStatusPending
	default:
		finalStatus = dexmodel.DexStatusPending
	}

	return &dexmodel.DexCheckTxOut{
		Channel: "lifi",
		Status:  finalStatus,
		ToHash:  res.Receiving.TxHash,
		Msg:     res.SubstatusMsg,
	}
}

func (c *Client) toStandardQuoteRes(res *QuoteRes) *dexmodel.DexQuoteOut {
	var totalFeeUsd float64
	for _, gc := range res.Estimate.GasCosts {
		totalFeeUsd += parseFloat(gc.AmountUSD)
	}
	for _, fc := range res.Estimate.FeeCosts {
		totalFeeUsd += parseFloat(fc.AmountUSD)
	}

	var txData *dexmodel.DexTx
	var userOp string
	if res.TransactionRequest.To != "" || res.TransactionRequest.Data != "" {
		chain, ok := idChainMapper[strconv.Itoa(res.Action.FromChainID)]
		if !ok {
			return nil
		}

		var data, value, gasPrice, gasLimit, feeLimit string
		if chain == "TRX" {
			if res.TransactionRequest.CustomData.ContractType != "TriggerSmartContract" ||
				len(res.TransactionRequest.CustomData.TronTransaction) == 0 {
				return nil
			}
			data = hex.EncodeToString(res.TransactionRequest.CustomData.TronTransaction)
			feeLimit = utils.HexToDecimal(res.TransactionRequest.GasLimit)
			stated, err := strconv.ParseInt(feeLimit, 10, 64)
			if err != nil || stated <= 0 || stated > tron.MaxFeeLimitSun {
				return nil
			}
			userOp = tronUserOp
		} else if chain == "BTC" {
			data = res.TransactionRequest.Data
			if res.TransactionRequest.Value != "" && res.TransactionRequest.Value != "0" {
				value = res.TransactionRequest.Value
			}
		} else if chain == "SOL" || chain == "SUI" {
			raw, err := base64.StdEncoding.DecodeString(res.TransactionRequest.Data)
			if err == nil {
				data = hex.EncodeToString(raw)
			} else {
				data = res.TransactionRequest.Data
			}
		} else {
			data = res.TransactionRequest.Data
			value = utils.HexToDecimal(res.TransactionRequest.Value)
			if value == "0" {
				value = ""
			}
			gasPrice = utils.HexToDecimal(res.TransactionRequest.GasPrice)
			if gasPrice == "0" {
				gasPrice = ""
			}
			gasLimit = utils.HexToDecimal(res.TransactionRequest.GasLimit)
			if gasLimit == "0" {
				gasLimit = ""
			}
		}
		txData = &dexmodel.DexTx{
			To:       res.TransactionRequest.To,
			Data:     data,
			Value:    value,
			GasPrice: gasPrice,
			GasLimit: gasLimit,
			FeeLimit: feeLimit,
		}
	}

	var approvalData *dexmodel.DexApproval
	if res.Action.FromToken.Address != "0x0000000000000000000000000000000000000000" &&
		res.Action.FromToken.Address != "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee" &&
		res.Estimate.ApprovalAddress != "" &&
		res.Action.FromChainID < 20000000000000 {
		approvalData = &dexmodel.DexApproval{
			Token:   res.Action.FromToken.Address,
			Spender: res.Estimate.ApprovalAddress,
			Amount:  res.Estimate.FromAmount,
		}
	}

	slippage, _ := decimal.NewFromFloat(res.Action.Slippage).Shift(2).Float64()
	// TrustedSpenders: LiFi shows approvalAddress on the estimate
	// (usually LiFi Diamond, occasionally a bridge like Mayan / DeBridge /
	// Wormhole depending on the tool it routed to). Include the tx
	// target too so FE will sign the swap tx itself.
	var trusted []string
	if res.Estimate.ApprovalAddress != "" {
		trusted = append(trusted, res.Estimate.ApprovalAddress)
	}
	if txData != nil && txData.To != "" && txData.To != res.Estimate.ApprovalAddress {
		trusted = append(trusted, txData.To)
	}
	route := &dexmodel.DexRoute{
		RouteId:           res.ID,
		Name:              res.ToolDetails.Name,
		AmountOut:         res.Estimate.ToAmount,
		AmountOutMin:      res.Estimate.ToAmountMin,
		Slippage:          slippage,
		SuggestedSlippage: slippage,
		FeeInUsd:          fmt.Sprintf("%.2f", totalFeeUsd),
		EstimatedTime:     res.Estimate.ExecutionDuration,
		NeedBuild:         false,
		UserOp:            userOp,
		TxData:            txData,
		ApprovalData:      approvalData,
		TrustedSpenders:   trusted,
	}

	return &dexmodel.DexQuoteOut{
		Channel: "lifi",
		Routes:  []*dexmodel.DexRoute{route},
	}
}

const tronUserOp = "tron_tx"

var ExchangeTools = []string{
	"1inch", "aftermath", "bebop", "bitget", "bluefin7k", "cetus", "dflow", "dodo", "eisen", "enso",
	"fly", "gluex", "hyperbloom", "hyperflow", "hyperliquidSpotDex", "hypertrade", "jupiter",
	"kumbaya", "kuru", "kyberswap", "lifiIntentsDex", "lifidexaggregator", "liquidswap", "merkle",
	"momentum", "monorail", "nexroute", "nordstern", "okx", "openocean", "paraswap", "peach",
	"rialto", "soroswap", "sunswap", "supersetDex", "sushiswap", "titan",
}

var exchangeToolIndex = func() map[string]struct{} {
	m := make(map[string]struct{}, len(ExchangeTools))
	for _, name := range ExchangeTools {
		m[strings.ToLower(name)] = struct{}{}
	}
	return m
}()

func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}

var idChainMapper = map[string]string{
	"143":              "MON",
	"999":              "HYPEEVM",
	"1337":             "HYPE",
	"728126428":        "TRX",
	"1":                "ETH",
	"56":               "BNB",
	"137":              "POLYGON",
	"43114":            "AVAXC",
	"42161":            "ARB",
	"10":               "OP",
	"8453":             "BASE",
	"59144":            "LINEA",
	"324":              "ZKSYNC",
	"1101":             "ZKEVM",
	"534352":           "SCROLL",
	"5000":             "MANTLE",
	"169":              "MANTA",
	"81457":            "BLAST",
	"34443":            "MODE",
	"252":              "FRAXTAL",
	"7777777":          "ZORA",
	"666666666":        "DEGEN",
	"1088":             "METIS",
	"100":              "GNOSIS",
	"250":              "FTM",
	"1284":             "MOONBEAM",
	"1285":             "MOONRIVER",
	"42220":            "CELO",
	"1313161554":       "AURORA",
	"25":               "CRONOS",
	"288":              "BOBA",
	"122":              "FUSE",
	"1666600000":       "HARMONY",
	"2222":             "KAVA",
	"321":              "KCC",
	"106":              "VELAS",
	"128":              "HECO",
	"66":               "OKEX",
	"40":               "TELOS",
	"167000":           "TAIKO",
	"196":              "XLAYER",
	"480":              "WORLDCHAIN",
	"1135":             "LISK",
	"1868":             "SONEIUM",
	"1923":             "SWELL",
	"60808":            "BOB",
	"33139":            "APECHAIN",
	"2741":             "ABSTRACT",
	"130":              "UNICHAIN",
	"57073":            "INK",
	"20000000000001":   "BTC",
	"1151111081099710": "SOL",
	"9270000000000000": "SUI",
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

var nativeTokens = map[string]string{
	"LOCAL": "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
	"EVM":   "0x0000000000000000000000000000000000000000",
	"BTC":   "bitcoin",
	"SOL":   "11111111111111111111111111111111",
	"SUI":   "0x0000000000000000000000000000000000000000000000000000000000000002::sui::SUI",
}

func nativeTokenByChain(chain string) string {
	switch chain {
	case "BTC":
		return nativeTokens["BTC"]
	case "SOL":
		return nativeTokens["SOL"]
	case "SUI":
		return nativeTokens["SUI"]
	default:
		return nativeTokens["EVM"]
	}
}

func adjustNativeToken(in *dexmodel.DexQuoteIn) {
	fromTokenLower := strings.ToLower(in.FromToken)
	if fromTokenLower == nativeTokens["LOCAL"] || fromTokenLower == nativeTokens["EVM"] {
		in.FromToken = nativeTokenByChain(in.FromChain)
	}

	toTokenLower := strings.ToLower(in.ToToken)
	if toTokenLower == nativeTokens["LOCAL"] || toTokenLower == nativeTokens["EVM"] {
		in.ToToken = nativeTokenByChain(in.ToChain)
	}
}
