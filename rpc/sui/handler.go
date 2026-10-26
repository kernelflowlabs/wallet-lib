package sui

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/kernelflowlabs/wallet-lib/common/httpc"
	chainrpc "github.com/kernelflowlabs/wallet-lib/rpc"
	"github.com/kernelflowlabs/wallet-lib/signing"
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
	out := &GetBlockCountRes{}
	body := &BaseRequest{
		JsonRPC: "2.0",
		ID:      1,
		Method:  "sui_getLatestCheckpointSequenceNumber",
		Params:  nil,
	}
	err := h.rpc.Post(ctx, out, "", body)
	if err != nil {
		return "", fmt.Errorf("failed to get latest block,err=%s", err)
	} else if out.Error.Code != 0 {
		return "", fmt.Errorf("failed to get latest block,errMsg=%s", out.Error.Message)
	}
	return out.Result, nil
}

func (h *Handler) GetBalance(ctx context.Context, address, contractAddress, blockNumber string) (string, error) {
	coinType := contractAddress
	if contractAddress == signing.MagicContactAddressForNative {
		coinType = "0x2::sui::SUI"
	}

	out := &GetBalanceRes{}
	body := &BaseRequest{
		JsonRPC: "2.0",
		ID:      1,
		Method:  "suix_getBalance",
		Params:  []string{address, coinType},
	}
	err := h.rpc.Post(ctx, out, "", body)
	if err != nil {
		return "", fmt.Errorf("failed to get latest balance, err=%s", err)
	} else if out.Error.Code != 0 {
		return "", fmt.Errorf("failed to get latest balance, errMsg=%s", out.Error.Message)
	}
	return out.Result.TotalBalance, nil
}

func (h *Handler) SendTx(ctx context.Context, signedHex string) (string, error) {
	signedTxBytes, err := hex.DecodeString(signedHex)
	if err != nil {
		return "", fmt.Errorf("failed to decode signedHex, err=%s", err)
	}
	var st signedTx
	err = json.Unmarshal(signedTxBytes, &st)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal signedTx, err=%s", err)
	}
	out := &SendRawTransactionRes{}
	body := &BaseRequest{
		JsonRPC: "2.0",
		ID:      1,
		Method:  "sui_executeTransactionBlock",
		Params: []interface{}{
			st.Tx,
			[]string{st.Signature},
			map[string]bool{
				"showInput":          true,
				"showEffects":        true,
				"showEvents":         true,
				"showObjectChanges":  true,
				"showBalanceChanges": true,
				"showRawInput":       true,
				"showRawEffects":     true,
			},
			"WaitForLocalExecution",
		},
	}
	err = h.rpc.Post(ctx, out, "", body)
	if err != nil {
		return "", fmt.Errorf("failed to executeTransactionBlock, err=%s", err)
	} else if out.Error.Code != 0 {
		return "", fmt.Errorf("failed to executeTransactionBlock, errMsg=%s", out.Error.Message)
	}

	return out.Result.Digest, nil
}

func (h *Handler) CheckTx(ctx context.Context, hash string) (*chainrpc.TxResult, error) {
	out := &GetTransactionsRes{}
	body := &BaseRequest{
		JsonRPC: "2.0",
		ID:      1,
		Method:  "sui_getTransactionBlock",
		Params:  []interface{}{hash, map[string]bool{"showEffects": true}},
	}
	err := h.rpc.Post(ctx, out, "", body)
	if err != nil {
		return nil, fmt.Errorf("failed to getTransactionBlock, err=%s", err)
	} else if out.Error.Code != 0 {
		return nil, fmt.Errorf("failed to getTransactionBlock, errMsg=%s", out.Error.Message)
	}

	r := &chainrpc.TxResult{}
	if out.Result.Checkpoint == nil {
		r.Status = signing.TxStatusPending
		return r, nil
	} else if out.Result.Effects.Status.Status == "success" {
		r.Status = signing.TxStatusSucceeded
		r.Height = *out.Result.Checkpoint
		return r, nil
	}
	r.Status = signing.TxStatusFailed
	r.Height = *out.Result.Checkpoint
	return r, nil
}

func (h *Handler) CallContract(ctx context.Context, contractAddress, params, blockNumber string) ([]byte, error) {
	return nil, fmt.Errorf("CallContract not supported")
}

func (h *Handler) InquireChain(ctx context.Context, instruction, params string) (string, error) {
	switch instruction {
	case "getGasPrice":
		out := &GetGasPriceRes{}
		body := &BaseRequest{
			JsonRPC: "2.0",
			ID:      1,
			Method:  "suix_getReferenceGasPrice",
			Params:  []string{params},
		}
		err := h.rpc.Post(ctx, out, "", body)
		if err != nil {
			return "", fmt.Errorf("failed to get gas price, err=%s", err)
		} else if out.Error.Code != 0 {
			return "", fmt.Errorf("failed to get gas price, errMsg=%s", out.Error.Message)
		}
		return out.Result, nil
	case "getCoinMetadata":
		out := &GetCoinMetadataRes{}
		body := &BaseRequest{
			JsonRPC: "2.0",
			ID:      1,
			Method:  "suix_getCoinMetadata",
			Params:  []string{params},
		}
		err := h.rpc.Post(ctx, out, "", body)
		if err != nil {
			return "", fmt.Errorf("failed to get coin metadata, err=%s", err)
		} else if out.Error.Code != 0 {
			return "", fmt.Errorf("failed to get coin metadata, errMsg=%s", out.Error.Message)
		}
		rBytes, err := json.Marshal(out.Result)
		if err != nil {
			return "", fmt.Errorf("failed to Marshal, err=%v", err)
		}
		return string(rBytes), nil
	case "getCoins":
		tmp := strings.Split(params, "_")
		if len(tmp) != 2 {
			return "", fmt.Errorf("len(tmp) != 2")
		}
		address := tmp[0]
		contractAddress := tmp[1]
		if contractAddress == signing.MagicContactAddressForNative {
			contractAddress = "0x2::sui::SUI"
		}
		out := &GetCoinsRes{}
		cursor := ""
		var coins Coins
		for {
			request := struct {
				JsonRPC string   `json:"jsonrpc"`
				ID      int      `json:"id"`
				Method  string   `json:"method"`
				Params  []string `json:"params"`
			}{
				JsonRPC: "2.0",
				ID:      1,
				Method:  "suix_getCoins",
				Params:  []string{address, contractAddress},
			}

			if cursor != "" {
				request.Params = append(request.Params, cursor)
			}

			if err := h.rpc.Post(ctx, &out, "", request); err != nil {
				return "", fmt.Errorf("failed to get coins, err=%s", err)
			} else if out.Error.Code != 0 {
				return "", fmt.Errorf("failed to get coins, errMsg=%s", out.Error.Message)
			}
			coins = append(coins, out.Result.Data...)
			if !out.Result.HasNextPage {
				break
			}
			cursor = out.Result.NextCursor
		}
		rBytes, err := json.Marshal(coins)
		if err != nil {
			return "", fmt.Errorf("failed to Marshal, err=%v", err)
		}
		return string(rBytes), nil
	case "getTokenDecimals":
		if params == signing.MagicContactAddressForNative {
			return "9", nil
		}
		out := &GetCoinMetadataRes{}
		body := &BaseRequest{
			JsonRPC: "2.0",
			ID:      1,
			Method:  "suix_getCoinMetadata",
			Params:  []string{params},
		}
		err := h.rpc.Post(ctx, out, "", body)
		if err != nil {
			return "", fmt.Errorf("failed to get coin metadata, err=%s", err)
		} else if out.Error.Code != 0 {
			return "", fmt.Errorf("failed to get coin metadata, errMsg=%s", out.Error.Message)
		}
		decimals := strconv.FormatInt(out.Result.Decimals, 10)
		if decimals == "" || decimals == "0" {
			return "", fmt.Errorf("got zero")
		}
		return decimals, nil
	}

	return "", fmt.Errorf("unsupported function")
}
