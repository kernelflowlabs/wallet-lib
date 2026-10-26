package aptos

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/kernelflowlabs/wallet-lib/common/httpc"
	chainrpc "github.com/kernelflowlabs/wallet-lib/rpc"
	"github.com/kernelflowlabs/wallet-lib/signing"
	walletaptos "github.com/kernelflowlabs/wallet-lib/signing/aptos"
)

var _ chainrpc.BasicChainHandler = (*Handler)(nil)

const TxValiditySeconds = 600

type Handler struct {
	rpc       *httpc.Request
	submitRpc *httpc.Request
}

func NewHandler(rpcUrl string) (*Handler, error) {
	h := &Handler{}
	h.rpc = httpc.NewRequest(rpcUrl, map[string]string{
		"Content-Type": "application/json",
	})
	h.submitRpc = httpc.NewRequest(rpcUrl, map[string]string{
		"Content-Type": "application/x.aptos.signed_transaction+bcs",
	})
	return h, nil
}

func (h *Handler) GetHeight(ctx context.Context) (string, error) {
	res := &LedgerInfoRes{}
	path := "v1"
	err := h.rpc.Get(ctx, res, path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get ledger info, err=%v", err)
	} else if res.Message != "" {
		return "", fmt.Errorf("failed to get ledger info, errMsg=%v", res.Message)
	}
	return res.BlockHeight, nil
}

func (h *Handler) GetBalance(ctx context.Context, address, contractAddress, blockNumber string) (string, error) {
	coinType := contractAddress
	if contractAddress == signing.MagicContactAddressForNative {
		coinType = "0x1::aptos_coin::AptosCoin"
	}

	if strings.Contains(coinType, "::") {
		return h.getCoinBalance(ctx, address, coinType)
	}
	return h.getFungibleAssetBalance(ctx, address, coinType)
}

func (h *Handler) SendTx(ctx context.Context, signedHex string) (string, error) {
	signedBytes, err := hex.DecodeString(signedHex)
	if err != nil {
		return "", fmt.Errorf("failed to DecodeString, err=%v", err)
	}
	res := &RpcTx{}
	path := "v1/transactions"
	err = h.submitRpc.PostWithOutEncoded(ctx, res, path, signedBytes)
	if err != nil {
		return "", fmt.Errorf("failed to post transactions, err=%v", err)
	} else if res.Message != "" {
		return "", fmt.Errorf("failed to post transactions, errMsg=%v", res.Message)
	}
	return res.Hash, nil
}

func (h *Handler) CheckTx(ctx context.Context, hash string) (*chainrpc.TxResult, error) {
	r := &chainrpc.TxResult{}

	tx, err := h.getTransactionByHash(ctx, hash)
	if err != nil {
		if httpc.StatusCode(err) == http.StatusNotFound ||
			strings.Contains(err.Error(), "Transaction not found by Transaction hash") {
			r.Status = signing.TxStatusPending
			return r, nil
		}
		return r, err
	}

	if tx.Version != "" && tx.VmStatus != "" {
		r.GasUsed = tx.GasUsed
		//r.Height = tx.Version
		time, _ := strconv.ParseUint(tx.Timestamp, 10, 64)
		r.Time = strconv.FormatUint(time/1000, 10)

		if tx.Success && tx.VmStatus == "Executed successfully" {
			r.Status = signing.TxStatusSucceeded
			return r, nil
		}

		r.Status = signing.TxStatusFailed
		r.ErrMsg = tx.VmStatus
		return r, nil
	}

	r.Status = signing.TxStatusPending
	return r, nil
}

func (h *Handler) CallContract(ctx context.Context, contractAddress, params, blockNumber string) ([]byte, error) {
	return nil, fmt.Errorf("CallContract not supported")
}

func (h *Handler) InquireChain(ctx context.Context, instruction, params string) (string, error) {
	switch instruction {
	case "getNonce":
		res := &AccountCoreRes{}
		path := "v1/accounts/" + params
		err := h.rpc.Get(ctx, res, path, nil)
		if httpc.StatusCode(err) == http.StatusNotFound {
			return "0", nil
		} else if err != nil {
			return "", fmt.Errorf("failed to get accounts, err=%w", err)
		} else if res.Message != "" {
			return "", fmt.Errorf("failed to get accounts, err=Msg%v", res.Message)
		}
		return res.SequenceNumber, nil
	case "getGasPrice":
		res := &GasPriceRes{}
		path := "v1/estimate_gas_price"
		err := h.rpc.Get(ctx, res, path, nil)
		if err != nil {
			return "", fmt.Errorf("failed to get gasprice, err=%v", err)
		} else if res.Message != "" {
			return "", fmt.Errorf("failed to get gasprice, errMsg=%v", res.Message)
		}
		return strconv.FormatUint(res.PrioritizedGasEstimate, 10), nil
	case "getLedgerInfo":
		result := walletaptos.LedgerInfoParams{}
		res := &LedgerInfoRes{}
		path := "v1"
		err := h.rpc.Get(ctx, res, path, nil)
		if err != nil {
			return "", fmt.Errorf("failed to get ledger info, err=%v", err)
		} else if res.Message != "" {
			return "", fmt.Errorf("failed to get ledger info, errMsg=%v", res.Message)
		}
		ledgerMicros, err := strconv.ParseUint(res.LedgerTimestamp, 10, 64)
		if err != nil {
			return "", fmt.Errorf("invalid ledger timestamp %q, err=%w", res.LedgerTimestamp, err)
		}
		result.ChainId = strconv.FormatUint(res.ChainId, 10)
		result.ExpirationTimestamp = strconv.FormatUint(ledgerMicros/1e6+TxValiditySeconds, 10)
		resultBytes, err := json.Marshal(result)
		if err != nil {
			return "", fmt.Errorf("failed to Marshal for result, err=%v", err)
		}
		return string(resultBytes), nil
	case "getBlockByNumber":
		query := url.Values{}
		query.Set("with_transactions", "true")
		path := "v1/blocks/by_height/" + params
		res := &BlockTx{}
		err := h.rpc.Get(ctx, res, path, query)
		if err != nil {
			return "", fmt.Errorf("failed to get masterchain transactions, err=%v", err)
		} else if res.Message != "" {
			return "", fmt.Errorf("failed to get masterchain transactions, errMsg=%v", res.Message)
		}
		resBytes, err := json.Marshal(res)
		if err != nil {
			return "", fmt.Errorf("failed to Marshal for res, err=%v", err)
		}
		return string(resBytes), nil
	case "getTokenDecimals":
		parts := strings.SplitN(params, "::", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid coin type: %s", params)
		}
		accountAddr := parts[0]
		path := "v1/accounts/" + accountAddr + "/resource/0x1::coin::CoinInfo%3C" + url.QueryEscape(params) + "%3E"
		res := &CoinInfoRes{}
		err := h.rpc.Get(ctx, res, path, nil)
		if err != nil {
			return "", fmt.Errorf("failed to get token decimals, err=%v", err)
		} else if res.Message != "" {
			return "", fmt.Errorf("failed to get token decimals, errMsg=%s", res.Message)
		}
		return strconv.FormatInt(int64(res.Data.Decimals), 10), nil
	}
	return "", fmt.Errorf("unsupported function")
}

// unexported
func (h *Handler) getTransactionByHash(ctx context.Context, hash string) (*RpcTx, error) {
	out := &RpcTx{}
	path := "v1/transactions/by_hash/" + hash
	err := h.rpc.Get(ctx, out, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction by hash, err=%w", err)
	} else if out.Message != "" {
		return nil, fmt.Errorf("failed to get transaction by hash, errMsg=%v", out.Message)
	}
	return out, nil
}

func (h *Handler) getCoinBalance(ctx context.Context, address, coinType string) (string, error) {
	var buf bytes.Buffer
	path := "v1/accounts/" + address + "/balance/" + coinType
	err := h.rpc.GetRaw(ctx, &buf, path, nil)
	if err != nil {
		if httpc.StatusCode(err) == http.StatusNotFound ||
			strings.Contains(err.Error(), "Resource not found") ||
			strings.Contains(err.Error(), "resource_not_found") {
			return "0", nil
		}
		return "", fmt.Errorf("failed to get coin balance, err=%v", err)
	}
	balance, ok := big.NewInt(0).SetString(strings.TrimSpace(buf.String()), 10)
	if !ok {
		return "0", nil
	}
	return balance.String(), nil
}

func (h *Handler) getFungibleAssetBalance(ctx context.Context, address, metadataAddress string) (string, error) {
	type viewReq struct {
		Function      string   `json:"function"`
		TypeArguments []string `json:"type_arguments"`
		Arguments     []string `json:"arguments"`
	}
	body := viewReq{
		Function:      "0x1::primary_fungible_store::balance",
		TypeArguments: []string{"0x1::fungible_asset::Metadata"},
		Arguments:     []string{address, metadataAddress},
	}

	var result []string
	err := h.rpc.Post(ctx, &result, "v1/view", body)
	if err != nil {
		if strings.Contains(err.Error(), "Resource not found") ||
			strings.Contains(err.Error(), "resource_not_found") {
			return "0", nil
		}
		return "", fmt.Errorf("failed to get fungible asset balance, err=%v", err)
	}
	if len(result) == 0 {
		return "0", nil
	}
	balance, ok := big.NewInt(0).SetString(strings.TrimSpace(result[0]), 10)
	if !ok {
		return "0", nil
	}
	return balance.String(), nil
}
