package multiversx

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"strconv"
	"strings"

	"github.com/kernelflowlabs/wallet-lib/common/httpc"
	chainrpc "github.com/kernelflowlabs/wallet-lib/rpc"
	"github.com/kernelflowlabs/wallet-lib/signing"
	walletmvx "github.com/kernelflowlabs/wallet-lib/signing/multiversx"
)

var _ chainrpc.BasicChainHandler = (*Handler)(nil)

type Handler struct {
	rpc *httpc.Request
}

func NewHandler(rpcUrl string) (*Handler, error) {
	h := &Handler{}
	h.rpc = httpc.NewRequest(rpcUrl, map[string]string{
		"Content-Type": "application/json",
	})
	return h, nil
}

func (h *Handler) GetHeight(ctx context.Context) (string, error) {
	res := &StatusRes{}
	err := h.rpc.Get(ctx, res, "network/status/4294967295", nil)
	if err != nil {
		return "", fmt.Errorf("failed to get latest block, err=%v", err)
	} else if res.Code != "successful" {
		return "", fmt.Errorf("failed to get latest block, errMsg=%s", res.Error)
	}

	return strconv.FormatUint(res.Data.Status.ErdHighestFinalNonce, 10), nil
}

func (h *Handler) GetBalance(ctx context.Context, address, contractAddress, blockNumber string) (string, error) {
	if contractAddress == signing.MagicContactAddressForNative {
		res := &AddressRes{}
		path := "address/" + address
		err := h.rpc.Get(ctx, res, path, nil)
		if err != nil {
			return "", fmt.Errorf("failed to get balance, err=%v", err)
		} else if res.Code != "successful" {
			return "", fmt.Errorf("failed to get balance, errMsg=%s", res.Error)
		}
		if bal, ok := big.NewInt(0).SetString(res.Data.Account.Balance, 10); ok {
			return bal.String(), nil
		}
		return "", fmt.Errorf("wrong return type")
	}

	res := &EsdtBalanceRes{}
	path := "address/" + address + "/esdt/" + contractAddress
	err := h.rpc.Get(context.Background(), res, path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get getTokenBalance, err=%v", err)
	} else if res.Code != "successful" {
		if strings.Contains(res.Error, "account was not found") {
			return "0", nil
		}
		return "", fmt.Errorf("failed to get getTokenBalance, err=%s", res.Error)
	}
	if bal, ok := big.NewInt(0).SetString(res.Data.TokenData.Balance, 10); ok {
		return bal.String(), nil
	}
	return "", fmt.Errorf("wrong return type")
}

func (h *Handler) SendTx(ctx context.Context, signedHex string) (string, error) {
	signedBytes, err := hex.DecodeString(strings.TrimPrefix(signedHex, "0x"))
	if err != nil {
		return "", fmt.Errorf("failed to DecodeString for signedHex, err=%v", err)
	}
	req := &nativeTx{}
	err = json.Unmarshal(signedBytes, req)
	if err != nil {
		return "", fmt.Errorf("failed to Unmarshal for signedBytes, err=%v", err)
	}

	res := &TransactionSendRes{}
	path := "transaction/send"
	err = h.rpc.Post(ctx, res, path, req)
	if err != nil {
		return "", fmt.Errorf("failed to sendTransaction, err=%v", err)
	} else if res.Code != "successful" {
		return "", fmt.Errorf("failed to sendTransaction, errMsg=%s", res.Error)
	}

	return res.Data.TxHash, nil
}

func (h *Handler) CheckTx(ctx context.Context, hash string) (*chainrpc.TxResult, error) {
	tmp := strings.Split(hash, ":")
	if len(tmp) != 2 {
		return nil, fmt.Errorf("invalid params, pattern should be hash:from")
	}
	hash = tmp[0]
	sender := tmp[1]
	r := &chainrpc.TxResult{}

	tx, err := h.getTransaction(ctx, hash, sender)
	if err != nil {
		return nil, err
	}
	if tx.Status == "pending" {
		r.Status = signing.TxStatusPending
	} else if tx.Status == "invalid" {
		r.Status = signing.TxStatusFailed
	} else if tx.Status == "success" {
		if tx.Operation == "transfer" {
			r.Status = signing.TxStatusSucceeded
		} else if tx.Operation == "ESDTTransfer" {
			ifContractFailed := false

			if len(tx.SmartContractResults) == 0 {
				ifContractFailed = true
			}
			for _, v := range tx.SmartContractResults {
				if v.ReturnMessage != "" {
					ifContractFailed = true
				}
			}
			if ifContractFailed {
				r.Status = signing.TxStatusFailed
			} else {
				r.Status = signing.TxStatusSucceeded
			}
		} else {
			return r, fmt.Errorf("unsupported operation")
		}
	} else {
		r.Status = signing.TxStatusUnknown
	}
	return r, nil
}

func (h *Handler) CallContract(ctx context.Context, contractAddress, params, blockNumber string) ([]byte, error) {
	return nil, fmt.Errorf("CallContract not supported")
}

func (h *Handler) InquireChain(ctx context.Context, instruction, params string) (string, error) {
	switch instruction {
	case "getNonce":
		res := &AddressRes{}
		path := "address/" + params
		err := h.rpc.Get(ctx, res, path, nil)
		if err != nil {
			return "", fmt.Errorf("failed to get getNonce, err=%v", err)
		} else if res.Code != "successful" {
			return "", fmt.Errorf("failed to get getNonce, errMsg=%s", res.Error)
		}

		return strconv.FormatUint(res.Data.Account.Nonce, 10), nil
	case "getNetworkConfig":
		res := &NetWorkConfigRes{}
		result := &walletmvx.NetWorkConfig{}
		err := h.rpc.Get(ctx, res, "network/config", nil)
		if err != nil {
			return "", fmt.Errorf("failed to network/config, err=%v", err)
		} else if res.Code != "successful" {
			return "", fmt.Errorf("failed to network/config, errMsg=%s", res.Error)
		}
		result.GasPrice = strconv.FormatUint(res.Data.Config.MinGasPrice, 10)
		result.GasLimit = strconv.FormatUint(res.Data.Config.MinGasLimit, 10)
		result.ChainID = res.Data.Config.ChainID
		result.Version = strconv.FormatUint(uint64(res.Data.Config.MinTransactionVersion), 10)
		resultBytes, err := json.Marshal(result)
		if err != nil {
			return "", fmt.Errorf("failed to Marshal for result, err=%s", err)
		}
		return string(resultBytes), nil
	case "getTokenDecimals":
		if params == signing.MagicContactAddressForNative {
			return "18", nil
		}
		res := &TokenInfoRes{}
		path := "tokens/" + params
		err := h.rpc.Get(ctx, res, path, nil)
		if err != nil {
			return "", fmt.Errorf("failed to get token decimals, err=%v", err)
		} else if res.Code != "successful" {
			return "", fmt.Errorf("failed to get token decimals, errMsg=%s", res.Error)
		}
		return strconv.FormatUint(uint64(res.Data.Decimals), 10), nil
	}
	return "", fmt.Errorf("unsupported function")
}

// unexported
func (h *Handler) getTransaction(ctx context.Context, hash string, sender string) (*EgldTransaction, error) {
	res := &TransactionRes{}
	path := "transaction/" + hash
	query := url.Values{}
	query.Set("withResults", "true")
	query.Set("sender", sender)
	err := h.rpc.Get(ctx, res, path, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get getTransaction %s, err=%v", hash, err)
	} else if res.Code != "successful" {
		return nil, fmt.Errorf("failed to get getTransaction %s, errMsg=%s", hash, res.Error)
	}

	return res.Data.Transaction, nil
}
