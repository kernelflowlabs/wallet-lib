package okx

import (
	"encoding/json"
	"fmt"
	dexmodel "github.com/kernelflowlabs/wallet-sdk/common/dexmodel"
	"log"
	"strconv"
	"strings"
)

type (
	BaseResponse struct {
		Code json.RawMessage `json:"code"`
		Msg  string          `json:"msg"`
	}
	SupportedChainsRes struct {
		BaseResponse
		Data []struct {
			ChainIndex             string `json:"chainIndex"`
			ChainName              string `json:"chainName"`
			DexTokenApproveAddress string `json:"dexTokenApproveAddress"`
		} `json:"data"`
	}
	SingleChainQuoteRes struct {
		BaseResponse
		Data []struct {
			ChainIndex      string `json:"chainIndex"`
			FromTokenAmount string `json:"fromTokenAmount"`
			ToTokenAmount   string `json:"toTokenAmount"`
			EstimateGasFee  string `json:"estimateGasFee"`
			TradeFee        string `json:"tradeFee"`
			PriceImpact     string `json:"priceImpactPercent"`
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
			ChainIndex      string `json:"chainIndex"`
			FromTokenAmount string `json:"fromTokenAmount"`
			ToTokenAmount   string `json:"toTokenAmount"`
			EstimateGasFee  string `json:"estimateGasFee"`
			TradeFee        string `json:"tradeFee"`
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
			ChainIndex  string `json:"chainIndex"`
			DexRouter   string `json:"dexRouter"`
			ErrorMsg    string `json:"errorMsg"`
			FromAddress string `json:"fromAddress"`
			GasLimit    string `json:"gasLimit"`
			GasPrice    string `json:"gasPrice"`
			GasUsed     string `json:"gasUsed"`
			Height      string `json:"height"`
			Status      string `json:"status"` //success,fail,pending
			ToAddress   string `json:"toAddress"`
			TxFee       string `json:"txFee"`
			TxHash      string `json:"txHash"`
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
		FromToken:       in.FromToken,
		ToToken:         in.ToToken,
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
		FromToken:       in.FromToken,
		ToToken:         in.ToToken,
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
	"25":     "CRO",
	"56":     "BNB",
	"66":     "OKTC",
	"130":    "UNICHAIN",
	"137":    "POLYGON",
	"143":    "MONAD",
	"146":    "SONIC",
	"169":    "MANTA",
	"195":    "TRON",
	"196":    "XLAYER",
	"250":    "FTM",
	"324":    "ZKSYNC",
	"501":    "SOL",
	"607":    "TON",
	"784":    "SUI",
	"1030":   "CFX",
	"1088":   "METIS",
	"1101":   "POLYGONZKEVM",
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

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
