package okx

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/kernelflowlabs/wallet-sdk/common/dexmodel"
)

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
		BridgeID     int    `json:"bridgeId"`
		ChainIndex   string `json:"chainIndex"`
		Status       string `json:"status"`
		TxHash       string `json:"txHash"`
		ToTxHash     string `json:"toTxHash"`
		RefundTxHash string `json:"refundTxHash"`
	} `json:"data"`
}

func cloneQuery(query url.Values) url.Values {
	result := make(url.Values, len(query))
	for key, values := range query {
		result[key] = append([]string(nil), values...)
	}
	return result
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
		if approved != nil || !common.IsHexAddress(entry.Contract) || common.HexToAddress(entry.Contract) == (common.Address{}) {
			return fmt.Errorf("invalid approval spender")
		}
		data, err := hex.DecodeString(strings.TrimPrefix(entry.Calldata, "0x"))
		if err != nil || len(data) != 68 || hex.EncodeToString(data[:4]) != "095ea7b3" || new(big.Int).SetBytes(data[4:16]).Sign() != 0 || common.BytesToAddress(data[16:36]) != common.HexToAddress(entry.Contract) {
			return fmt.Errorf("approval calldata mismatch")
		}
		amount, ok := new(big.Int).SetString(in.FromAmount, 10)
		if !ok || amount.Sign() <= 0 || new(big.Int).SetBytes(data[36:]).Cmp(amount) < 0 {
			return fmt.Errorf("approval amount mismatch")
		}
		approved = &dexmodel.DexApproval{Token: in.FromToken, Spender: strings.ToLower(entry.Contract), Amount: amount.String()}
	}
	if approved == nil {
		return fmt.Errorf("missing approval spender")
	}
	route.ApprovalData = approved
	route.TrustedSpenders = []string{approved.Spender}
	return nil
}

func (c *Client) crossChainStatus(ctx context.Context, in *dexmodel.DexCheckTxIn) (*dexmodel.DexCheckTxOut, error) {
	chain, ok := idChainMapperReverse[strings.ToUpper(in.FromChain)]
	bridge, err := strconv.Atoi(in.Bridge)
	if !ok || err != nil || bridge <= 0 || in.Hash == "" || in.HashType != dexmodel.DexHashTypeTxHash {
		return nil, fmt.Errorf("cross-chain status requires source chain, transaction hash and bridge ID")
	}
	query := url.Values{"chainIndex": {chain}, "hash": {in.Hash}, "bridgeId": {in.Bridge}}
	response := &CrossStatusResponse{}
	if err := c.signedGet(ctx, response, "api/v6/dex/cross-chain/status", query); err != nil {
		return nil, err
	}
	if !response.IsSuccess() {
		return nil, fmt.Errorf("cross-chain status: code=%s", response.Code)
	}
	result := &dexmodel.DexCheckTxOut{Channel: "okx", ToChain: in.ToChain, FromHash: in.Hash, Status: dexmodel.DexStatusPending}
	if len(response.Data) == 0 {
		result.Status = dexmodel.DexStatusNotFound
		return result, nil
	}
	if len(response.Data) != 1 {
		return nil, fmt.Errorf("ambiguous cross-chain status")
	}
	entry := response.Data[0]
	if !strings.EqualFold(entry.TxHash, in.Hash) || entry.ChainIndex != chain {
		return nil, fmt.Errorf("cross-chain status identity mismatch")
	}
	result.ProviderStatus = entry.Status
	switch strings.ToUpper(entry.Status) {
	case "SUCCESS":
		if !validTransactionHash(entry.ToTxHash) {
			return nil, fmt.Errorf("successful cross-chain status has no destination hash")
		}
		result.Status, result.ToHash = dexmodel.DexStatusSucceeded, entry.ToTxHash
	case "NOT_FOUND":
		result.Status = dexmodel.DexStatusNotFound
	case "REFUNDED", "REFUND_SUCCESS":
		if !validTransactionHash(entry.RefundTxHash) {
			return result, nil
		}
		result.Status, result.ToHash = dexmodel.DexStatusRefunded, entry.RefundTxHash
	}
	return result, nil
}

func validTransactionHash(value string) bool {
	if len(value) != 66 || !strings.HasPrefix(value, "0x") {
		return false
	}
	bytes, err := hex.DecodeString(value[2:])
	return err == nil && new(big.Int).SetBytes(bytes).Sign() > 0
}

func (c *Client) crossChainApproval(ctx context.Context, in *dexmodel.DexQuoteIn, bridge int) (string, error) {
	chain, ok := idChainMapperReverse[in.FromChain]
	if !ok || bridge <= 0 {
		return "", fmt.Errorf("invalid approval route")
	}
	query := url.Values{"chainIndex": {chain}, "tokenContractAddress": {in.FromToken}, "userWalletAddress": {in.FromAddress}, "bridgeId": {strconv.Itoa(bridge)}, "approveAmount": {in.FromAmount}}
	var response struct {
		BaseResponse
		Data []struct {
			ApproveAddress       string `json:"approveAddress"`
			ChainIndex           string `json:"chainIndex"`
			TokenContractAddress string `json:"tokenContractAddress"`
			Tx                   struct {
				From string `json:"from"`
				To   string `json:"to"`
				Data string `json:"data"`
			} `json:"tx"`
		} `json:"data"`
	}
	if err := c.signedGet(ctx, &response, "api/v6/dex/cross-chain/approve-tx", query); err != nil {
		return "", err
	}
	if !response.IsSuccess() || len(response.Data) != 1 {
		return "", fmt.Errorf("missing cross-chain approval")
	}
	entry := response.Data[0]
	if entry.ChainIndex != chain || !strings.EqualFold(entry.TokenContractAddress, in.FromToken) || !strings.EqualFold(entry.Tx.To, in.FromToken) || !strings.EqualFold(entry.Tx.From, in.FromAddress) {
		return "", fmt.Errorf("cross-chain approval identity mismatch")
	}
	encoded, err := json.Marshal(map[string]string{"approveContract": entry.ApproveAddress, "approveTxCalldata": entry.Tx.Data})
	return string(encoded), err
}
