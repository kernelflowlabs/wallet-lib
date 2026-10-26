package algorand

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"github.com/kernelflowlabs/wallet-lib/common/httpc"
	chainrpc "github.com/kernelflowlabs/wallet-lib/rpc"
	"github.com/kernelflowlabs/wallet-lib/signing"
)

var _ chainrpc.BasicChainHandler = (*Handler)(nil)

type Handler struct {
	rpc     *httpc.Request
	indexer *httpc.Request
}

func newAlgorandRequest(url, tokenHeader string) (*httpc.Request, error) {
	tmp := strings.Split(url, "@")
	headers := map[string]string{"Content-Type": "application/json"}
	switch len(tmp) {
	case 1:
	case 2:
		headers[tokenHeader] = tmp[1]
	default:
		return nil, fmt.Errorf("invalid params")
	}
	return httpc.NewRequest(tmp[0], headers), nil
}

func newAlgodRequest(url string) (*httpc.Request, error) {
	return newAlgorandRequest(url, "X-Algo-API-Token")
}

func newIndexerRequest(url string) (*httpc.Request, error) {
	return newAlgorandRequest(url, "X-Indexer-API-Token")
}

func NewHandler(rpcUrl string) (*Handler, error) {
	parts := strings.SplitN(rpcUrl, ";", 2)
	rpc, err := newAlgodRequest(parts[0])
	if err != nil {
		return nil, err
	}
	h := &Handler{rpc: rpc}
	if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
		indexer, err := newIndexerRequest(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, err
		}
		h.indexer = indexer
	}
	return h, nil
}

func (h *Handler) GetHeight(ctx context.Context) (string, error) {
	out := &NodeStatusResponse{}
	err := h.rpc.Get(ctx, out, "v2/status", nil)
	if err != nil {
		return "", err
	} else if out.Message != "" {
		return "", fmt.Errorf("fail to get latest block, err=%v", out.Message)
	} else if out.LastRound == 0 {
		return "", fmt.Errorf("fail to get latest block, return value is 0")
	}
	return strconv.FormatUint(out.LastRound, 10), nil
}

func (h *Handler) GetBalance(ctx context.Context, address, contractAddress, blockNumber string) (string, error) {
	if contractAddress == signing.MagicContactAddressForNative {
		value, err := h.getBaseCoinBalance(ctx, address)
		if err != nil {
			return "", err
		}
		return value.String(), nil
	}
	value, err := h.getTokenBalance(ctx, address, contractAddress)
	if err != nil {
		return "", err
	}
	return value.String(), nil
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
			r.ErrMsg = "a failed tx"
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

	tx, err := h.checkTransactionById(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("fail to checkTransactionById, err=%v", err)
	}
	transfer := &chainrpc.Transfer{}
	if tx.Txn.Txn.Type == "pay" {
		transfer.Sender = tx.Txn.Txn.Snd
		transfer.Recipient = tx.Txn.Txn.Rcv
		transfer.Amount = strconv.FormatUint(tx.Txn.Txn.Amt, 10)
		transfer.ContractAddress = signing.MagicContactAddressForNative
	} else if tx.Txn.Txn.Type == "axfer" {
		transfer.Sender = tx.Txn.Txn.Snd
		transfer.Recipient = tx.Txn.Txn.Arcv
		transfer.Amount = strconv.FormatUint(tx.Txn.Txn.Aamt, 10)
		transfer.ContractAddress = strconv.FormatUint(tx.Txn.Txn.Xaid, 10)
	} else {
		r.Rejected = true
		r.ErrMsg = "invalid tx Type"
		return r, nil
	}

	if transfer.Sender == "" || transfer.Recipient == "" || transfer.Amount == "" || transfer.ContractAddress == "" {
		r.ErrMsg = "incomplete return data"
		r.Rejected = true
		return r, nil
	}
	r.Transfers = append(r.Transfers, transfer)
	return r, nil
}

func (h *Handler) SendTx(ctx context.Context, signedHex string) (string, error) {
	signedBytes, err := hex.DecodeString(strings.TrimPrefix(signedHex, "0x"))
	if err != nil {
		return "", fmt.Errorf("fail to DecodeString signedHex, err=%v", err)
	}
	out := &SendTxOut{}
	h.rpc.SetHeader("Content-Type", "application/x-binary")
	err = h.rpc.PostWithOutEncoded(ctx, out, "v2/transactions", signedBytes)
	if err != nil {
		return "", fmt.Errorf("fail to send transaction, err=%v", err)
	} else if out.Message != "" {
		return "", fmt.Errorf("fail to send transaction, err=%v", out.Message)
	} else if out.TxId == "" {
		return "", fmt.Errorf("fail to send transaction, return TxId is empty")
	}
	return out.TxId, nil
}

func (h *Handler) CheckTx(ctx context.Context, hash string) (*chainrpc.TxResult, error) {
	r := &chainrpc.TxResult{}
	tx, err := h.checkTransactionById(ctx, hash)
	if err != nil {
		if httpc.StatusCode(err) == http.StatusNotFound ||
			strings.Contains(err.Error(), "could not find the transaction") {
			r.Status = signing.TxStatusPending
			return r, nil
		}
		return nil, err
	}
	if tx.ConfirmedRound > 0 {
		r.Status = signing.TxStatusSucceeded
		r.Height = strconv.FormatUint(tx.ConfirmedRound, 10)
	} else if tx.PoolError != "" {
		r.Status = signing.TxStatusFailed
		r.ErrMsg = tx.PoolError
	} else {
		r.Status = signing.TxStatusPending
	}
	return r, nil
}

func (h *Handler) CallContract(ctx context.Context, contractAddress, params, blockNumber string) ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

func (h *Handler) InquireChain(ctx context.Context, instruction, params string) (string, error) {
	switch instruction {
	case "transactionsParams":
		out := &TransactionParams{}
		err := h.rpc.Get(ctx, out, "v2/transactions/params", nil)
		if err != nil {
			return "", fmt.Errorf("fail to get latest transactions params, err=%v", err)
		} else if out.Message != "" {
			return "", fmt.Errorf("fail to get latest transactions params, err=%v", out.Message)
		}
		res := map[string]string{
			"genesisID":   out.GenesisId,
			"genesisHash": out.GenesisHash,
			"firstValid":  strconv.FormatUint(out.LastRound, 10),
		}
		resBytes, err := json.Marshal(res)
		if err != nil {
			return "", fmt.Errorf("fail to Marshal for res, err=%v", err)
		}
		return string(resBytes), nil
	case "isAddressActivatedForToken":
		tmp := strings.Split(params, ":")
		if len(tmp) != 2 {
			return "", fmt.Errorf("invalid params")
		}
		assetsId, err := strconv.ParseUint(tmp[1], 10, 64)
		if err != nil {
			return "", fmt.Errorf("fail to ParseUint for contractAddress, err=%v", err)
		}
		out := &AccountAddress{}
		err = h.rpc.Get(ctx, out, "v2/accounts/"+tmp[0], nil)
		if err != nil {
			return "", fmt.Errorf("fail to get balance, err=%v", err)
		} else if out.Message != "" {
			return "", fmt.Errorf("fail to get balance, errMSg=%v", out.Message)
		}
		for _, v := range out.Assets {
			if v.AssetId == assetsId {
				return "true", nil
			}
		}
		return "false", nil
	case "getMinFee":
		out := &TransactionParams{}
		err := h.rpc.Get(ctx, out, "v2/transactions/params", nil)
		if err != nil {
			return "", fmt.Errorf("fail to get latest transactions params, err=%v", err)
		} else if out.Message != "" {
			return "", fmt.Errorf("fail to get latest transactions params, err=%v", out.Message)
		}
		return strconv.FormatUint(out.MinFee, 10), nil
	}
	return "", fmt.Errorf("unsupported function")
}

func (h *Handler) getBaseCoinBalance(ctx context.Context, address string) (*big.Int, error) {
	out := &AccountAddress{}
	err := h.rpc.Get(ctx, out, "v2/accounts/"+address, nil)
	if err != nil {
		return nil, fmt.Errorf("fail to get balance, err=%v", err)
	} else if out.Message != "" {
		return nil, fmt.Errorf("fail to get balance, err=%v", out.Message)
	}
	return big.NewInt(0).SetUint64(out.AmountWithoutPendingRewards), nil
}

func (h *Handler) getTokenBalance(ctx context.Context, address, contract string) (*big.Int, error) {
	assetsId, err := strconv.ParseUint(contract, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("fail to ParseUint for contract, err=%v", err)
	}
	out := &AccountAddress{}
	err = h.rpc.Get(ctx, out, "v2/accounts/"+address, nil)
	if err != nil {
		return nil, fmt.Errorf("fail to get balance, err=%v", err)
	} else if out.Message != "" {
		return nil, fmt.Errorf("fail to get balance, err=%v", out.Message)
	}
	for _, v := range out.Assets {
		if v.AssetId == assetsId {
			return big.NewInt(0).SetUint64(v.Amount), nil
		}
	}
	return big.NewInt(0), nil
}

func (h *Handler) checkTransactionById(ctx context.Context, txID string) (*PendingTransactionResponse, error) {
	out := &PendingTransactionResponse{}
	err := h.rpc.Get(ctx, out, "v2/transactions/pending/"+txID, nil)
	if err != nil {
		if httpc.StatusCode(err) == http.StatusNotFound && h.indexer != nil {
			return h.lookupInIndexer(ctx, txID)
		}
		return nil, fmt.Errorf("fail to get pending tx, err=%w", err)
	} else if out.Message != "" {
		return nil, fmt.Errorf("fail to get pending tx, errMsg=%v", out.Message)
	}
	return out, nil
}

func (h *Handler) lookupInIndexer(ctx context.Context, txID string) (*PendingTransactionResponse, error) {
	res := &IndexerTransactionRes{}
	if err := h.indexer.Get(ctx, res, "v2/transactions/"+txID, nil); err != nil {
		return nil, fmt.Errorf("fail to get tx from indexer, err=%w", err)
	}
	if res.Message != "" {
		return nil, fmt.Errorf("fail to get tx from indexer, errMsg=%v", res.Message)
	}
	tx := res.Transaction
	out := &PendingTransactionResponse{ConfirmedRound: tx.ConfirmedRound}
	out.Txn.Txn.Type = tx.TxType
	out.Txn.Txn.Snd = tx.Sender
	out.Txn.Txn.Fee = tx.Fee
	out.Txn.Txn.Rcv = tx.PaymentTransaction.Receiver
	out.Txn.Txn.Amt = tx.PaymentTransaction.Amount
	out.Txn.Txn.Arcv = tx.AssetTransferTransaction.Receiver
	out.Txn.Txn.Aamt = tx.AssetTransferTransaction.Amount
	out.Txn.Txn.Xaid = tx.AssetTransferTransaction.AssetID
	return out, nil
}
