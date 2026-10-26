package stellar

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/kernelflowlabs/wallet-lib/common/httpc"
	chainrpc "github.com/kernelflowlabs/wallet-lib/rpc"
	"github.com/kernelflowlabs/wallet-lib/signing"
)

var _ chainrpc.BasicChainHandler = (*Handler)(nil)

type Handler struct {
	rpc      *httpc.Request
	decimals int64
}

func NewHandler(rpcUrl string) (*Handler, error) {
	h := &Handler{}
	h.rpc = httpc.NewRequest(rpcUrl, map[string]string{
		"accept": "application/json",
	})
	h.decimals = 7
	return h, nil
}

func (h *Handler) GetHeight(ctx context.Context) (string, error) {
	out := &LedgersPageRes{}
	params := url.Values{}
	params.Set("limit", "1")
	params.Set("order", "desc")
	err := h.rpc.Get(ctx, out, "ledgers", params)
	if err != nil {
		return "", fmt.Errorf("fail to get latest block,err=%s", err)
	} else if out.Status != 0 {
		return "", fmt.Errorf("fail to get latest block,err=%s", out.Error())
	} else if len(out.Embedded.Records) != 1 {
		return "", fmt.Errorf("len(res.Embedded.Records) != 1")
	}
	return strconv.FormatInt(int64(out.Embedded.Records[0].Sequence), 10), nil
}

func (h *Handler) GetBalance(ctx context.Context, address, contractAddress, blockNumber string) (string, error) {
	if contractAddress != signing.MagicContactAddressForNative {
		return "", fmt.Errorf("only basecoin supported")
	}
	out, err := h.getAccountInfo(ctx, address)
	if err != nil {
		if strings.Contains(err.Error(), "account not activated") {
			return "0", nil
		}
		return "", fmt.Errorf("fail to getAccountInfo, err=%v", err)
	}
	for _, v := range out.Balances {
		if v.AssetType == "native" {
			bal, err := decimal.NewFromString(v.Balance)
			if err != nil {
				return "", err
			}
			return bal.Shift(int32(h.decimals)).String(), nil
		}
	}
	return "", fmt.Errorf("fail to get balance")
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

	op, err := h.getTransactionOptionsById(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("fail to getTransactionOptionsById, err=%v", err)
	} else if len(op.Embedded.Records) != 1 {
		r.Rejected = true
		r.ErrMsg = "invalid Records length"
		return r, nil
	} else if !op.Embedded.Records[0].TransactionSuccessful {
		r.Rejected = true
		r.ErrMsg = "not a succeeded tx"
		return r, nil
	}
	record := op.Embedded.Records[0]
	var sender, recipient, amount string
	switch record.Type {
	case "payment":
		if record.AssetType != "native" {
			r.Rejected = true
			r.ErrMsg = fmt.Sprintf("unsupported asset type %s", record.AssetType)
			return r, nil
		}
		sender, recipient, amount = record.From, record.To, record.Amount
	case "create_account":
		sender, recipient, amount = record.Funder, record.Account, record.StartingBalance
	default:
		r.Rejected = true
		r.ErrMsg = fmt.Sprintf("unsupported operation type %s", record.Type)
		return r, nil
	}
	amountDecimal, err := decimal.NewFromString(amount)
	if err != nil {
		r.Rejected = true
		r.ErrMsg = "fail to NewFromString for Amount"
		return r, nil
	}

	transfer := &chainrpc.Transfer{
		Sender:          sender,
		Recipient:       recipient,
		Amount:          amountDecimal.Shift(int32(h.decimals)).String(),
		ContractAddress: signing.MagicContactAddressForNative,
	}
	if transfer.Sender == "" || transfer.Recipient == "" || transfer.Amount == "" {
		r.Rejected = true
		r.ErrMsg = "incomplete return data"
		return r, nil
	}
	r.Transfers = append(r.Transfers, transfer)
	return r, nil
}

func (h *Handler) SendTx(ctx context.Context, signedHex string) (string, error) {
	signedBytes, err := hex.DecodeString(strings.TrimPrefix(signedHex, "0x"))
	if err != nil {
		return "", fmt.Errorf("fail to DecodeString,err=%v", err)
	}
	signedBase64 := base64.StdEncoding.EncodeToString(signedBytes)

	res := &TransactionRes{}
	h.rpc.SetHeader("content-type", "application/x-www-form-urlencoded")
	body := url.Values{}
	body.Set("tx", signedBase64)
	err = h.rpc.PostWithXWWWFormUrlencoded(ctx, res, "transactions", body)
	if err != nil {
		return "", err
	} else if res.Status != 0 {
		return "", fmt.Errorf("fail to PostWithXWWWFormUrlencoded, err=%s", res.Error())
	}
	return res.Hash, nil
}

func (h *Handler) CheckTx(ctx context.Context, hash string) (*chainrpc.TxResult, error) {
	r := &chainrpc.TxResult{}
	tx, err := h.getTransactionById(ctx, hash)
	if err != nil {
		if httpc.StatusCode(err) != http.StatusNotFound {
			return nil, fmt.Errorf("fail to get transaction, err=%w", err)
		}
		r.Status = signing.TxStatusPending
	} else if tx.Successful {
		r.Height = strconv.FormatInt(int64(tx.Ledger), 10)
		r.Status = signing.TxStatusSucceeded
	} else {
		r.Height = strconv.FormatInt(int64(tx.Ledger), 10)
		r.Status = signing.TxStatusFailed
	}
	return r, nil
}

func (h *Handler) CallContract(ctx context.Context, contractAddress, params, blockNumber string) ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

func (h *Handler) InquireChain(ctx context.Context, instruction, params string) (string, error) {
	switch instruction {
	case "getAccountSequence":
		out, err := h.getAccountInfo(ctx, params)
		if err != nil {
			return "", fmt.Errorf("fail to getAccountInfo, err=%v", err)
		}
		return out.Sequence, nil
	case "IsAccountActivated":
		_, err := h.getAccountInfo(ctx, params)
		if err != nil {
			if strings.Contains(err.Error(), "account not activated") {
				return "false", nil
			}
			return "", fmt.Errorf("fail to getAccountInfo, err=%v", err)
		}
		return "true", nil
	}
	return "", fmt.Errorf("unsupported function")
}

func (h *Handler) getAccountInfo(ctx context.Context, address string) (*AccountRes, error) {
	res := &AccountRes{}
	err := h.rpc.Get(ctx, res, "accounts/"+address, nil)
	if httpc.StatusCode(err) == http.StatusNotFound {
		return nil, fmt.Errorf("account not activated")
	} else if err != nil {
		return nil, err
	} else if res.Status != 0 {
		if res.Title == "Resource Missing" && res.Status == 404 {
			return nil, fmt.Errorf("account not activated")
		}
		return nil, fmt.Errorf("%s", res.Error())
	}
	return res, nil
}

func (h *Handler) getTransactionById(ctx context.Context, txID string) (*TransactionRes, error) {
	res := &TransactionRes{}
	err := h.rpc.Get(ctx, res, "transactions/"+txID, nil)
	if err != nil {
		return nil, err
	} else if res.Status != 0 {
		return nil, fmt.Errorf("%s", res.Error())
	}
	return res, nil
}

func (h *Handler) getTransactionOptionsById(ctx context.Context, txID string) (*RpcOptionsRes, error) {
	res := &RpcOptionsRes{}
	err := h.rpc.Get(ctx, res, "transactions/"+txID+"/operations", nil)
	if err != nil {
		return nil, err
	} else if res.Status != 0 {
		return nil, fmt.Errorf("%s", res.Error())
	}
	return res, nil
}
