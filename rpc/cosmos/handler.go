package cosmos

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/kernelflowlabs/wallet-lib/common/httpc"
	chainrpc "github.com/kernelflowlabs/wallet-lib/rpc"
	"github.com/kernelflowlabs/wallet-lib/signing"
	walletcosmos "github.com/kernelflowlabs/wallet-lib/signing/cosmos"
)

var _ chainrpc.BasicChainHandler = (*Handler)(nil)

type Handler struct {
	rpc     *httpc.Request
	network string
}

func NewHandler(rpcUrl, network string) (*Handler, error) {
	tmp := strings.Split(rpcUrl, "&")
	apiKey := ""
	if len(tmp) == 2 {
		rpcUrl = tmp[0]
		apiKey = tmp[1]
	}

	h := &Handler{}
	h.rpc = httpc.NewRequest(rpcUrl, map[string]string{
		"Content-Type":          "application/json",
		"x-allthatnode-api-key": apiKey,
	})
	h.network = network
	return h, nil
}

func (h *Handler) GetHeight(ctx context.Context) (string, error) {
	if h.network == walletcosmos.NetworkEnumForSei {
		return h.getHeightSei(ctx)
	}

	out := &GetBlockRes{}
	path := "cosmos/base/tendermint/v1beta1/blocks/latest"
	err := h.rpc.Get(ctx, out, path, nil)
	if err != nil {
		return "", fmt.Errorf("fail to get latest block,err=%s", err)
	} else if out.Code != 0 {
		return "", fmt.Errorf("fail to get latest block,errMsg=%s", out.Message)
	}
	return out.Block.Header.Height, nil
}

func (h *Handler) GetBalance(ctx context.Context, address, contractAddress, blockNumber string) (string, error) {
	var demon string
	if contractAddress == signing.MagicContactAddressForNative {
		demon = walletcosmos.Denom(h.network)
	} else {
		tmp := strings.Split(contractAddress, ":")
		if len(tmp) < 1 {
			return "", fmt.Errorf("unsupported contractAddress")
		}
		demon = tmp[0]
	}

	out := &GetBalanceRes{}
	path := "cosmos/bank/v1beta1/balances/" + address
	err := h.rpc.Get(ctx, out, path, nil)
	if err != nil {
		return "", fmt.Errorf("fail to get balance, err=%v", err)
	} else if out.Code != 0 {
		return "", fmt.Errorf("fail to get balance, errMsg=%v", out.Message)
	}
	for _, v := range out.Balances {
		if v.Denom == demon {
			return v.Amount, nil
		}
	}
	return "0", nil
}

func (h *Handler) GetTransfersByHash(ctx context.Context, hash string, confirmation uint64) (*chainrpc.TxTransfers, error) {
	r := &chainrpc.TxTransfers{Hash: hash}
	txResult, err := h.CheckTx(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("fail to CheckTx, err=%v", err)
	} else if txResult.Status == signing.TxStatusFailed {
		r.Rejected = true
		if txResult.ErrMsg != "" {
			r.ErrMsg = txResult.ErrMsg
		} else {
			r.ErrMsg = "not a succeeded tx"
		}
		return r, nil
	} else if txResult.Status == signing.TxStatusPending {
		r.ErrMsg = "pending tx"
		return r, nil
	} else if txResult.Status != signing.TxStatusSucceeded {
		r.ErrMsg = "not a succeeded tx"
		return r, nil
	}

	confirmed, err := chainrpc.Confirmations(ctx, txResult.Height, confirmation, h.GetHeight)
	if err != nil {
		return nil, fmt.Errorf("failed to check confirmations, err=%w", err)
	}
	if confirmed < confirmation {
		r.ErrMsg = fmt.Sprintf("tx succeeded.But current confirmation number %d hasn't meet "+
			"expected number %d", confirmed, confirmation)
		return r, nil
	}

	demon := walletcosmos.Denom(h.network)
	out := &GetTxRes{}
	path := "cosmos/tx/v1beta1/txs/" + hash
	err = h.rpc.Get(ctx, out, path, nil)
	if err != nil {
		return nil, fmt.Errorf("fail to get tx, err=%v", err)
	} else if out.Code != 0 {
		r.Rejected = true
		r.ErrMsg = "tx key not found"
		return r, nil
	} else if len(out.TxResponse.Tx.Body.Messages) != 1 {
		r.Rejected = true
		r.ErrMsg = "invalid length of Messages"
		return r, nil
	}
	msg := out.TxResponse.Tx.Body.Messages[0]
	if strings.Contains(msg.Type, "/cosmos.bank.v1beta1.MsgSend") {
		if len(msg.Amount) == 1 {
			contractAddress := signing.MagicContactAddressForNative
			if msg.Amount[0].Denom == demon {

			} else if msg.Amount[0].Denom == "uusd" {
				contractAddress = "uusd:50000000:200000"
			} else {
				r.Rejected = true
				r.ErrMsg = fmt.Sprintf("unsupported denom %s", msg.Amount[0].Denom)
				return r, nil
			}
			r.Transfers = append(r.Transfers, &chainrpc.Transfer{
				Sender:          msg.FromAddress,
				Recipient:       msg.ToAddress,
				Amount:          msg.Amount[0].Amount,
				ContractAddress: contractAddress,
			})
		}
	} else {
		r.Rejected = true
		r.ErrMsg = "invalid msg type"
		return r, nil
	}

	if len(r.Transfers) == 0 || r.Transfers[0].Sender == "" || r.Transfers[0].Recipient == "" ||
		r.Transfers[0].Amount == "" || r.Transfers[0].ContractAddress == "" {
		r.Rejected = true
		r.ErrMsg = "incomplete return data"
		return r, nil
	}
	return r, nil
}

func (h *Handler) SendTx(ctx context.Context, signedHex string) (string, error) {
	signedBytes, err := hex.DecodeString(strings.TrimPrefix(signedHex, "0x"))
	if err != nil {
		return "", fmt.Errorf("fail to DecodeString for signedHex, err=%v", err)
	}
	out := &SendTxRes{}
	path := "cosmos/tx/v1beta1/txs"
	in := &SendTxReq{
		TxBytes: signedBytes,
		Mode:    "BROADCAST_MODE_SYNC",
	}
	err = h.rpc.Post(ctx, out, path, in)
	if err != nil {
		return "", fmt.Errorf("fail to send tx, err=%v", err)
	} else if out.Code != 0 {
		return "", fmt.Errorf("fail to send tx, code=%d, err=%s", out.Code, out.Message)
	} else if out.TxResponse.Code != 0 {
		return "", fmt.Errorf("fail to send tx, response code=%d, response errMsg=%s",
			out.TxResponse.Code, out.TxResponse.RawLog)
	}
	return out.TxResponse.Txhash, nil
}

func (h *Handler) CheckTx(ctx context.Context, hash string) (*chainrpc.TxResult, error) {
	result := &chainrpc.TxResult{}

	out := &GetTxRes{}
	path := "cosmos/tx/v1beta1/txs/" + hash
	err := h.rpc.Get(ctx, out, path, nil)
	notFound := httpc.StatusCode(err) == http.StatusNotFound
	if err != nil && !notFound {
		return nil, fmt.Errorf("fail to get tx, err=%w", err)
	} else if notFound || out.Code != 0 {
		if notFound || strings.Contains(out.Message, "key not found") ||
			strings.Contains(out.Message, "tx not found") {
			result.Status = signing.TxStatusPending
			return result, nil
		}
		return nil, fmt.Errorf("fail to get tx, code=%d, errMsg=%s", out.Code, out.Message)
	}

	result.Height = out.TxResponse.Height
	if out.TxResponse.Code != 0 {
		result.Status = signing.TxStatusFailed
		result.ErrMsg = out.TxResponse.RawLog
	} else {
		result.Status = signing.TxStatusSucceeded
	}
	return result, nil
}

func (h *Handler) CallContract(ctx context.Context, contractAddress, params, blockNumber string) ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

func (h *Handler) InquireChain(ctx context.Context, instruction, params string) (string, error) {
	switch instruction {
	case "getAccountInfo":
		result := &walletcosmos.AccountInfo{}
		out := &GetAccInfoRes{}
		path := "cosmos/auth/v1beta1/accounts/" + params
		err := h.rpc.Get(ctx, out, path, nil)
		notFound := httpc.StatusCode(err) == http.StatusNotFound
		if err != nil && !notFound {
			return "", fmt.Errorf("fail to get account info, err=%w", err)
		} else if notFound || out.Code != 0 {
			if notFound || strings.Contains(out.Message, "key not found") {
				result.AccountNumber = "0"
				result.Sequence = "0"
				resultBytes, err := json.Marshal(result)
				if err != nil {
					return "", err
				}
				return string(resultBytes), nil
			}
			return "", fmt.Errorf("fail to get account info,err=%s", out.Message)
		} else if out.Account.AccountNumber == "" || out.Account.Sequence == "" {
			return "", fmt.Errorf("got empty result")
		}
		result.AccountNumber = out.Account.AccountNumber
		result.Sequence = out.Account.Sequence
		resultBytes, err := json.Marshal(result)
		if err != nil {
			return "", err
		}
		return string(resultBytes), nil
	}
	return "", fmt.Errorf("unsupported function")
}

func (h *Handler) getHeightSei(ctx context.Context) (string, error) {
	out := &SeiStatus{}
	path := "status"
	err := h.rpc.Get(ctx, out, path, nil)
	if err != nil {
		return "", fmt.Errorf("fail to get latest block,err=%s", err)
	} else if out.SyncInfo.LatestBlockHeight == "0" || out.SyncInfo.LatestBlockHeight == "" {
		return "", fmt.Errorf("fail to get latest block, height==0")
	}
	return out.SyncInfo.LatestBlockHeight, nil
}
