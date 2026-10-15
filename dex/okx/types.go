package okx

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	dexmodel "github.com/kernelflowlabs/wallet-sdk/common/dexmodel"
	"log"
	"math/big"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/kernelflowlabs/wallet-sdk/signing/tron"
)

type (
	BaseResponse struct {
		Code json.RawMessage `json:"code"`
		Msg  string          `json:"msg"`
	}
	SupportedChainsRes struct {
		BaseResponse
		Data []struct {
			ChainIndex             json.Number `json:"chainIndex"`
			ChainName              string      `json:"chainName"`
			DexTokenApproveAddress string      `json:"dexTokenApproveAddress"`
		} `json:"data"`
	}
	SingleChainQuoteRes struct {
		BaseResponse
		Data []struct {
			ChainIndex      json.Number `json:"chainIndex"`
			FromTokenAmount string      `json:"fromTokenAmount"`
			ToTokenAmount   string      `json:"toTokenAmount"`
			EstimateGasFee  string      `json:"estimateGasFee"`
			TradeFee        string      `json:"tradeFee"`
			PriceImpact     string      `json:"priceImpactPercent"`
			FromToken       struct {
				TokenSymbol          string `json:"tokenSymbol"`
				TokenContractAddress string `json:"tokenContractAddress"`
				Decimal              string `json:"decimal"`
				TokenUnitPrice       string `json:"tokenUnitPrice"`
			} `json:"fromToken"`
			ToToken struct {
				TokenSymbol          string `json:"tokenSymbol"`
				TokenContractAddress string `json:"tokenContractAddress"`
				Decimal              string `json:"decimal"`
				TokenUnitPrice       string `json:"tokenUnitPrice"`
			} `json:"toToken"`
			SwapMode string `json:"swapMode"`
			Router   string `json:"router"`
		} `json:"data"`
	}
	SingleChainSwapRes struct {
		BaseResponse
		Data json.RawMessage `json:"data"`
	}
	SwapResData struct {
		RouterResult struct {
			ChainIndex      json.Number `json:"chainIndex"`
			FromTokenAmount string      `json:"fromTokenAmount"`
			ToTokenAmount   string      `json:"toTokenAmount"`
			EstimateGasFee  string      `json:"estimateGasFee"`
			TradeFee        string      `json:"tradeFee"`
		} `json:"routerResult"`
		Tx struct {
			From             string   `json:"from"`
			To               string   `json:"to"`
			Data             string   `json:"data"`
			Value            string   `json:"value"`
			Gas              string   `json:"gas"`
			GasPrice         string   `json:"gasPrice"`
			MinReceiveAmount string   `json:"minReceiveAmount"`
			SignatureData    []string `json:"signatureData"`
		} `json:"tx"`
	}
	CrossChainQuoteRes struct {
		BaseResponse
		Data json.RawMessage `json:"data"`
	}
	CrossResData struct {
		FromTokenAmount string `json:"fromTokenAmount"`
		Router          struct {
			BridgeId                  int    `json:"bridgeId"`
			BridgeName                string `json:"bridgeName"`
			CrossChainFee             string `json:"crossChainFee"`
			OtherNativeFee            string `json:"otherNativeFee"`
			CrossChainFeeTokenAddress string `json:"crossChainFeeTokenAddress"`
		} `json:"router"`
		ToTokenAmount   string `json:"toTokenAmount"`
		MinmumReceive   string `json:"minmumReceive"`
		MinimumReceived string `json:"minimumReceived"`
		Tx              struct {
			Data                 string    `json:"data"`
			From                 string    `json:"from"`
			To                   string    `json:"to"`
			Value                RawAmount `json:"value"`
			SignatureData        []string  `json:"signatureData"`
			GasLimit             string    `json:"gasLimit"`
			GasPrice             string    `json:"gasPrice"`
			MaxPriorityFeePerGas string    `json:"maxPriorityFeePerGas"`
		} `json:"tx"`
	}
	CrossChainBuildTxRes struct {
		BaseResponse
		Data []struct {
			FromChainIndex   string `json:"fromChainIndex"`
			ToChainIndex     string `json:"toChainIndex"`
			FromTokenAmount  string `json:"fromTokenAmount"`
			ToTokenAmount    string `json:"toTokenAmount"`
			MinimumReceived  string `json:"minimumReceived"`
			EstimateTime     string `json:"estimateTime"`
			EstimateGasFee   string `json:"estimateGasFee"`
			RouterBridgeName string `json:"routerBridgeName"`
			Tx               struct {
				From     string `json:"from"`
				To       string `json:"to"`
				Data     string `json:"data"`
				Value    string `json:"value"`
				GasPrice string `json:"gasPrice"`
				GasLimit string `json:"gasLimit"`
			} `json:"tx"`
		} `json:"data"`
	}
	HistoryRes struct {
		BaseResponse
		Data struct {
			ChainIndex  json.Number `json:"chainIndex"`
			DexRouter   string      `json:"dexRouter"`
			ErrorMsg    string      `json:"errorMsg"`
			FromAddress string      `json:"fromAddress"`
			GasLimit    string      `json:"gasLimit"`
			GasPrice    string      `json:"gasPrice"`
			GasUsed     string      `json:"gasUsed"`
			Height      string      `json:"height"`
			Status      string      `json:"status"` //success,fail,pending
			ToAddress   string      `json:"toAddress"`
			TxFee       string      `json:"txFee"`
			TxHash      string      `json:"txHash"`
		} `json:"data"`
	}
)

func (r *BaseResponse) IsSuccess() bool {
	return string(r.Code) == `"0"` || string(r.Code) == `0`
}

func (c *Client) parseSwapResData(raw json.RawMessage) ([]SwapResData, error) {
	var arr []SwapResData
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, nil
	}
	return nil, fmt.Errorf("no swap data available")
}

func (c *Client) parseCrossResData(raw json.RawMessage) ([]CrossResData, error) {
	var arr []CrossResData
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, nil
	}
	return nil, fmt.Errorf("no cross data available")
}

type (
	okxSingleChainQuoteReq struct {
		ChainIndex      string
		FromToken       string
		ToToken         string
		Amount          string
		Slippage        string
		UserAddress     string
		ReferrerAddress string
		FeePercent      string
	}
	okxCrossChainQuoteReq struct {
		FromChainIndex  string
		ToChainIndex    string
		FromToken       string
		ToToken         string
		Amount          string
		UserAddress     string
		ReceiverAddress string
		Slippage        string
		ReferrerAddress string
		FeePercent      string
	}
)

func (c *Client) toOkxSingleChainQuoteReq(in *dexmodel.DexQuoteIn) *okxSingleChainQuoteReq {
	chainId, ok := idChainMapperReverse[in.FromChain]
	if !ok {
		log.Printf("not found chain id %s", in.FromChain)
		return nil
	}
	return &okxSingleChainQuoteReq{
		ChainIndex:      chainId,
		FromToken:       providerToken(in.FromChain, in.FromToken),
		ToToken:         providerToken(in.ToChain, in.ToToken),
		Amount:          in.FromAmount,
		Slippage:        in.Slippage,
		UserAddress:     in.FromAddress,
		ReferrerAddress: in.FeeReceiver,
		FeePercent:      in.FeeRate,
	}
}

func (c *Client) toOkxCrossChainQuoteReq(in *dexmodel.DexQuoteIn) *okxCrossChainQuoteReq {
	fromChainId, ok := idChainMapperReverse[in.FromChain]
	if !ok {
		log.Printf("not found chain %s", in.FromChain)
		return nil
	}
	toChainId, ok := idChainMapperReverse[in.ToChain]
	if !ok {
		log.Printf("not found chain %s", in.ToChain)
		return nil
	}
	return &okxCrossChainQuoteReq{
		FromChainIndex:  fromChainId,
		ToChainIndex:    toChainId,
		FromToken:       providerToken(in.FromChain, in.FromToken),
		ToToken:         providerToken(in.ToChain, in.ToToken),
		Amount:          in.FromAmount,
		UserAddress:     in.FromAddress,
		ReceiverAddress: in.ToAddress,
		Slippage:        in.Slippage,
		ReferrerAddress: in.FeeReceiver,
		FeePercent:      in.FeeRate,
	}
}

func (c *Client) toStandardQuoteResPre(res *SingleChainQuoteRes, in *dexmodel.DexQuoteIn) *dexmodel.DexQuoteOut {
	if len(res.Data) == 0 {
		return &dexmodel.DexQuoteOut{Channel: "okx"}
	}
	d := res.Data[0]
	chainId, ok := idChainMapperReverse[in.FromChain]
	if !ok {
		return nil
	}
	route := &dexmodel.DexRoute{
		RouteId:       fmt.Sprintf("okx-%s-%s", chainId, in.FromToken),
		Name:          "OKX",
		AmountOut:     d.ToTokenAmount,
		FeeInUsd:      d.EstimateGasFee,
		EstimatedTime: 30,
		NeedBuild:     true,
	}
	return &dexmodel.DexQuoteOut{
		Channel: "okx",
		Routes:  []*dexmodel.DexRoute{route},
	}
}

func (c *Client) toStandardQuoteResFromSwapResData(data SwapResData, in *dexmodel.DexQuoteIn) *dexmodel.DexQuoteOut {
	var txData *dexmodel.DexTx
	if data.Tx.To != "" {
		txData = &dexmodel.DexTx{
			To:       data.Tx.To,
			Data:     data.Tx.Data,
			Value:    data.Tx.Value,
			GasPrice: data.Tx.GasPrice,
			GasLimit: data.Tx.Gas,
		}
		if !applyTronTx(txData, in.FromChain, data.Tx.Gas) {
			return nil
		}
	}
	chainId, ok := idChainMapperReverse[in.FromChain]
	if !ok {
		return nil
	}
	route := &dexmodel.DexRoute{
		RouteId:       fmt.Sprintf("okx-%s-%s", chainId, in.FromToken),
		Name:          "okx",
		AmountOut:     data.RouterResult.ToTokenAmount,
		AmountOutMin:  data.Tx.MinReceiveAmount,
		FeeInUsd:      data.RouterResult.TradeFee,
		EstimatedTime: 30,
		NeedBuild:     false,
		TxData:        txData,
	}
	if err := applyApproval(route, in, data.Tx.SignatureData); err != nil {
		return nil
	}
	return &dexmodel.DexQuoteOut{
		Channel: "okx",
		Routes:  []*dexmodel.DexRoute{route},
	}
}

func (c *Client) toStandardQuoteResFromCrossResData(data CrossResData, in *dexmodel.DexQuoteIn) *dexmodel.DexQuoteOut {
	route := &dexmodel.DexRoute{
		RouteId:      fmt.Sprintf("%d", data.Router.BridgeId),
		Name:         data.Router.BridgeName,
		AmountOut:    data.ToTokenAmount,
		AmountOutMin: data.MinimumReceived,
		Slippage:     parseFloat(in.Slippage),
		FeeInUsd:     data.Router.CrossChainFee,
		NeedBuild:    false,
		TxData: &dexmodel.DexTx{
			To:       data.Tx.To,
			Data:     data.Tx.Data,
			Value:    string(data.Tx.Value),
			GasPrice: data.Tx.GasPrice,
			GasLimit: data.Tx.GasLimit,
		},
	}
	if !applyTronTx(route.TxData, in.FromChain, data.Tx.GasLimit) {
		return nil
	}

	if route.AmountOutMin == "" {
		route.AmountOutMin = data.MinmumReceive
	}
	if err := applyApproval(route, in, data.Tx.SignatureData); err != nil {
		return nil
	}
	return &dexmodel.DexQuoteOut{
		Channel: "okx",
		Routes:  []*dexmodel.DexRoute{route},
	}
}

func (c *Client) toStandardStatusRes(res *HistoryRes) *dexmodel.DexCheckTxOut {
	var status dexmodel.DexStatus
	switch strings.ToLower(res.Data.Status) {
	case "success":
		status = dexmodel.DexStatusSucceeded
	case "fail":
		status = dexmodel.DexStatusFailed
	default:
		status = dexmodel.DexStatusPending
	}

	return &dexmodel.DexCheckTxOut{
		Channel: "okx",
		Status:  status,
		ToHash:  res.Data.TxHash,
		Msg:     res.Data.ErrorMsg,
	}
}

var idChainMapper = map[string]string{
	"1":      "ETH",
	"10":     "OP",
	"25":     "CRONOS",
	"56":     "BNB",
	"66":     "OKEX",
	"130":    "UNICHAIN",
	"137":    "POLYGON",
	"143":    "MON",
	"146":    "SONIC",
	"169":    "MANTA",
	"195":    "TRX",
	"196":    "XLAYER",
	"250":    "FTM",
	"324":    "ZKSYNC",
	"501":    "SOL",
	"607":    "TON",
	"784":    "SUI",
	"1030":   "CFXEVM",
	"1088":   "METIS",
	"1101":   "ZKEVM",
	"4200":   "MERLIN",
	"5000":   "MANTLE",
	"7000":   "ZETA",
	"8453":   "BASE",
	"9745":   "PLASMA",
	"59144":  "LINEA",
	"81457":  "BLAST",
	"42161":  "ARB",
	"43114":  "AVAXC",
	"534352": "SCROLL",
}

var idChainMapperReverse = func() map[string]string {
	m := make(map[string]string, len(idChainMapper))
	for k, v := range idChainMapper {
		m[v] = k
	}
	return m
}()

var nonEVMChainIDs = map[string]struct{}{
	"195": {},
	"501": {},
	"607": {},
	"784": {},
}

const (
	nativeTokenSentinel = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	tronChain           = "TRX"
	tronNativeToken     = "T9yD14Nj9j7xAB4dbGeiX9h8unkKHxuWwb"
)

func applyTronTx(tx *dexmodel.DexTx, chain, energy string) bool {
	if tx == nil || chain != tronChain {
		return true
	}
	feeLimit := tronFeeLimitSun(energy)
	if feeLimit == "" || len(tx.To) != 34 || !tron.ValidAddress(tx.To) {
		return false
	}
	if tx.Value == "0" {
		tx.Value = ""
	}
	tx.GasPrice, tx.GasLimit = "", ""
	tx.FeeLimit = feeLimit
	return true
}

func providerToken(chain, token string) string {
	if chain == tronChain && strings.EqualFold(token, nativeTokenSentinel) {
		return tronNativeToken
	}
	return token
}

func tronFeeLimitSun(energy string) string {
	units, err := strconv.ParseInt(strings.TrimSpace(energy), 10, 64)
	if err != nil {
		return ""
	}
	feeLimit, ok := tron.FeeLimitForEnergy(units, tron.DefaultFeeLimitHeadroom)
	if !ok {
		return ""
	}
	return strconv.FormatInt(feeLimit, 10)
}

func isQuoteChain(chain string) bool {
	return isEVMChain(chain) || chain == tronChain
}

func isEVMChainID(id string) bool {
	if _, ok := idChainMapper[id]; !ok {
		return false
	}
	_, nonEVM := nonEVMChainIDs[id]
	return !nonEVM
}

func isEVMChain(chain string) bool {
	id, ok := idChainMapperReverse[chain]
	return ok && isEVMChainID(id)
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

type RawAmount string

func (a *RawAmount) UnmarshalJSON(raw []byte) error {
	text := string(raw)
	if strings.HasPrefix(text, `"`) {
		if err := json.Unmarshal(raw, &text); err != nil {
			return err
		}
	}
	value, ok := new(big.Int).SetString(text, 10)
	if !ok || value.Sign() < 0 || value.BitLen() > 256 || value.String() != text {
		return fmt.Errorf("invalid raw amount")
	}
	*a = RawAmount(text)
	return nil
}

type CrossBridgesResponse struct {
	BaseResponse
	Data []struct {
		SupportedChains []string `json:"supportedChains"`
	} `json:"data"`
}

type CrossStatusResponse struct {
	BaseResponse
	Data []struct {
		BridgeID     int         `json:"bridgeId"`
		ChainIndex   json.Number `json:"chainIndex"`
		Status       string      `json:"status"`
		TxHash       string      `json:"txHash"`
		ToTxHash     string      `json:"toTxHash"`
		RefundTxHash string      `json:"refundTxHash"`
	} `json:"data"`
}

func approvalSpender(chain, contract string) (common.Address, string, bool) {
	if chain == tronChain {
		if len(contract) != 34 || !tron.ValidAddress(contract) {
			return common.Address{}, "", false
		}
		encoded := tron.ConvertToHex(contract)
		if len(encoded) != 42 {
			return common.Address{}, "", false
		}
		return common.HexToAddress(encoded[2:]), contract, true
	}
	if !common.IsHexAddress(contract) {
		return common.Address{}, "", false
	}
	return common.HexToAddress(contract), strings.ToLower(contract), true
}

func applyApproval(route *dexmodel.DexRoute, in *dexmodel.DexQuoteIn, signatureData []string) error {
	if strings.EqualFold(in.FromToken, "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee") {
		return nil
	}
	var approved *dexmodel.DexApproval
	for _, raw := range signatureData {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		var entry struct {
			Contract string `json:"approveContract"`
			Calldata string `json:"approveTxCalldata"`
		}
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			return fmt.Errorf("invalid approval metadata")
		}
		if entry.Contract == "" && entry.Calldata == "" {
			continue
		}
		spender, canonical, ok := approvalSpender(strings.ToUpper(strings.TrimSpace(in.FromChain)), entry.Contract)
		if approved != nil || !ok || spender == (common.Address{}) {
			return fmt.Errorf("invalid approval spender")
		}
		data, err := hex.DecodeString(strings.TrimPrefix(entry.Calldata, "0x"))
		if err != nil || len(data) != 68 || hex.EncodeToString(data[:4]) != "095ea7b3" || new(big.Int).SetBytes(data[4:16]).Sign() != 0 || common.BytesToAddress(data[16:36]) != spender {
			return fmt.Errorf("approval calldata mismatch")
		}
		amount, ok := new(big.Int).SetString(in.FromAmount, 10)
		if !ok || amount.Sign() <= 0 || new(big.Int).SetBytes(data[36:]).Cmp(amount) < 0 {
			return fmt.Errorf("approval amount mismatch")
		}
		approved = &dexmodel.DexApproval{Token: in.FromToken, Spender: canonical, Amount: amount.String()}
	}
	if approved == nil {
		return fmt.Errorf("missing approval spender")
	}
	route.ApprovalData = approved
	route.TrustedSpenders = []string{approved.Spender}
	return nil
}

func validTransactionHash(value string) bool {
	if len(value) != 66 || !strings.HasPrefix(value, "0x") {
		return false
	}
	bytes, err := hex.DecodeString(value[2:])
	return err == nil && new(big.Int).SetBytes(bytes).Sign() > 0
}
