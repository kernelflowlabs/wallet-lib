package bridgers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

type FlexibleString string

func (f *FlexibleString) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		*f = ""
		return nil
	}
	if len(data) > 0 && data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*f = FlexibleString(value)
		return nil
	}
	if !json.Valid(data) {
		return fmt.Errorf("invalid value")
	}
	*f = FlexibleString(data)
	return nil
}

func (f FlexibleString) String() string {
	return string(f)
}

type FlexibleInt int

func (f *FlexibleInt) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		*f = 0
		return nil
	}
	var value int
	if len(data) > 0 && data[0] == '"' {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		parsed, err := strconv.Atoi(text)
		if err != nil {
			return err
		}
		value = parsed
	} else if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*f = FlexibleInt(value)
	return nil
}

type ProviderError struct {
	Code    int
	Message string
}

func (e *ProviderError) Error() string {
	if e == nil {
		return "Bridgers request failed"
	}
	if e.Code == 777 {
		return fmt.Sprintf("Bridgers rate limit, code=%d, message=%s", e.Code, e.Message)
	}
	return fmt.Sprintf("Bridgers request failed, code=%d, message=%s", e.Code, e.Message)
}

type AmountRangeError struct {
	Amount   string
	Minimum  string
	Maximum  string
	Decimals int
}

func (e *AmountRangeError) Error() string {
	if e == nil {
		return "Bridgers amount is outside the supported range"
	}
	return fmt.Sprintf("Bridgers amount %s is outside range [%s, %s] at %d decimals", e.Amount, e.Minimum, e.Maximum, e.Decimals)
}

type responseMeta struct {
	ResCode FlexibleInt `json:"resCode"`
	ResMsg  string      `json:"resMsg"`
}

func (m responseMeta) providerError() error {
	if int(m.ResCode) == 100 {
		return nil
	}
	return &ProviderError{Code: int(m.ResCode), Message: m.ResMsg}
}

type Token struct {
	Symbol        string         `json:"symbol"`
	Name          string         `json:"name"`
	NickName      string         `json:"nickName"`
	Address       string         `json:"address"`
	AddressAlias  string         `json:"addressAlias"`
	Decimals      int            `json:"decimals"`
	LogoURI       string         `json:"logoURI"`
	Chain         string         `json:"chain"`
	ChainID       FlexibleString `json:"chainId"`
	CrossEnabled  FlexibleInt    `json:"isCrossEnable"`
	WithdrawalGas FlexibleString `json:"withdrawGas"`
}

type TokenListRequest struct {
	Chain string `json:"chain,omitempty"`
}

type TokenListResponse struct {
	responseMeta
	Data struct {
		Tokens []Token `json:"tokens"`
	} `json:"data"`
}

type QuoteRequest struct {
	EquipmentNo      string `json:"equipmentNo"`
	SourceFlag       string `json:"sourceFlag"`
	FromTokenAddress string `json:"fromTokenAddress"`
	ToTokenAddress   string `json:"toTokenAddress"`
	FromTokenAmount  string `json:"fromTokenAmount"`
	FromTokenChain   string `json:"fromTokenChain"`
	ToTokenChain     string `json:"toTokenChain"`
	UserAddress      string `json:"userAddr,omitempty"`
	SourceType       string `json:"sourceType,omitempty"`
	FixedRateFlag    int    `json:"fixedRateFlag"`
}

type QuoteData struct {
	AmountOutMin        string         `json:"amountOutMin"`
	ChainFee            FlexibleString `json:"chainFee"`
	ContractAddress     string         `json:"contractAddress"`
	DepositMin          FlexibleString `json:"depositMin"`
	DepositMax          FlexibleString `json:"depositMax"`
	Dex                 string         `json:"dex"`
	Fee                 FlexibleString `json:"fee"`
	FeeToken            string         `json:"feeToken"`
	FromTokenAmount     string         `json:"fromTokenAmount"`
	FromTokenDecimal    int            `json:"fromTokenDecimal"`
	ToTokenAmount       FlexibleString `json:"toTokenAmount"`
	ToTokenDecimal      int            `json:"toTokenDecimal"`
	Path                []string       `json:"path"`
	LogoURL             string         `json:"logoUrl"`
	RecommendedSlippage FlexibleString `json:"recommendedSlippage"`
	EstimatedTime       int64          `json:"estimatedTime"`
	InstantRate         FlexibleString `json:"instantRate"`
	QuoteFromBefore     FlexibleString `json:"quoteFromBefore"`
	QuoteFromAfter      FlexibleString `json:"quoteFromAfter"`
	Difference          FlexibleString `json:"difference"`
}

type QuoteResponse struct {
	responseMeta
	Data struct {
		TxData *QuoteData `json:"txData"`
	} `json:"data"`
}

type SwapRequest struct {
	FromTokenAddress string `json:"fromTokenAddress"`
	AmountOutMin     string `json:"amountOutMin"`
	EquipmentNo      string `json:"equipmentNo"`
	ToAddress        string `json:"toAddress"`
	ToTokenChain     string `json:"toTokenChain"`
	FromTokenAmount  string `json:"fromTokenAmount"`
	FromTokenChain   string `json:"fromTokenChain"`
	ToTokenAddress   string `json:"toTokenAddress"`
	FromAddress      string `json:"fromAddress"`
	SourceFlag       string `json:"sourceFlag"`
	FromCoinCode     string `json:"fromCoinCode"`
	ToCoinCode       string `json:"toCoinCode"`
	SourceType       string `json:"sourceType,omitempty"`
	Slippage         string `json:"slippage,omitempty"`
	FixedRateFlag    int    `json:"fixedRateFlag"`
}

type SwapResponse struct {
	responseMeta
	Data struct {
		TxData json.RawMessage `json:"txData"`
	} `json:"data"`
}

type EVMTransaction struct {
	Data  string         `json:"data"`
	To    string         `json:"to"`
	Value FlexibleString `json:"value"`
}

type GenerateOrderRequest struct {
	EquipmentNo      string `json:"equipmentNo"`
	SourceFlag       string `json:"sourceFlag"`
	Hash             string `json:"hash"`
	SourceType       string `json:"sourceType,omitempty"`
	FromTokenAddress string `json:"fromTokenAddress"`
	ToTokenAddress   string `json:"toTokenAddress"`
	FromAddress      string `json:"fromAddress"`
	ToAddress        string `json:"toAddress"`
	FromTokenChain   string `json:"fromTokenChain"`
	ToTokenChain     string `json:"toTokenChain"`
	FromTokenAmount  string `json:"fromTokenAmount"`
	AmountOutMin     string `json:"amountOutMin"`
	FromCoinCode     string `json:"fromCoinCode"`
	ToCoinCode       string `json:"toCoinCode"`
	Slippage         string `json:"slippage,omitempty"`
}

type GenerateOrderResponse struct {
	responseMeta
	Data struct {
		OrderID string `json:"orderId"`
	} `json:"data"`
}

type OrderDetailsRequest struct {
	OrderID string `json:"orderId"`
}

type OrderRecord struct {
	ID                 int            `json:"id"`
	OrderID            string         `json:"orderId"`
	FromTokenAddress   string         `json:"fromTokenAddress"`
	ToTokenAddress     string         `json:"toTokenAddress"`
	FromTokenAmount    FlexibleString `json:"fromTokenAmount"`
	ToTokenAmount      FlexibleString `json:"toTokenAmount"`
	FromAddress        string         `json:"fromAddress"`
	ToAddress          string         `json:"toAddress"`
	Slippage           FlexibleString `json:"slippage"`
	FromChain          string         `json:"fromChain"`
	ToChain            string         `json:"toChain"`
	Hash               string         `json:"hash"`
	DepositHashExplore string         `json:"depositHashExplore"`
	DexName            string         `json:"dexName"`
	Status             string         `json:"status"`
	CreateTime         string         `json:"createTime"`
	Source             string         `json:"source"`
	ToHash             string         `json:"toHash"`
	ReceiveHashExplore string         `json:"receiveHashExplore"`
	EquipmentNo        string         `json:"equipmentNo"`
	RefundCoinAmount   FlexibleString `json:"refundCoinAmt"`
	RefundHash         string         `json:"refundHash"`
	RefundHashExplore  string         `json:"refundHashExplore"`
	RefundReason       FlexibleString `json:"refundReason"`
	FromCoinCode       string         `json:"fromCoinCode"`
	ToCoinCode         string         `json:"toCoinCode"`
}

type OrderDetailsResponse struct {
	responseMeta
	Data OrderRecord `json:"data"`
}

type OrderListRequest struct {
	EquipmentNo string `json:"equipmentNo"`
	SourceType  string `json:"sourceType,omitempty"`
	SourceFlag  string `json:"sourceFlag"`
	PageNo      int    `json:"pageNo"`
	PageSize    int    `json:"pageSize"`
	FromAddress string `json:"fromAddress"`
}

type OrderListResponse struct {
	responseMeta
	Data struct {
		List     []OrderRecord  `json:"list"`
		Total    FlexibleString `json:"total"`
		PageNo   FlexibleString `json:"pageNo"`
		PageSize FlexibleString `json:"pageSize"`
	} `json:"data"`
}
