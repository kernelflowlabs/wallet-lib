package near

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/btcsuite/btcd/address/v2/base58"

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
	req := &BaseRequest{JsonRPC: "2.0", ID: "dontcare", Method: "block",
		Params: map[string]interface{}{"finality": "final"}}
	res := &GetBlockRes{}
	err := h.rpc.Post(ctx, res, "", req)
	if err != nil {
		return "", fmt.Errorf("fail to get latest block, err=%v", err)
	} else if res.Error != nil {
		return "", fmt.Errorf("fail to get latest block, errMsg=%v", res.Error)
	}
	return strconv.FormatInt(res.Result.Header.Height, 10), nil
}

func (h *Handler) GetBalance(ctx context.Context, address, contractAddress, blockNumber string) (string, error) {
	if contractAddress == signing.MagicContactAddressForNative {
		value, err := h.getBaseCoinBalance(ctx, address)
		if err != nil {
			if strings.Contains(err.Error(), "UNKNOWN_ACCOUNT") ||
				strings.Contains(err.Error(), "does not exist while viewing") {
				return "0", nil
			}
			return "", err
		}
		return value.String(), nil
	}
	return h.getTokenBalance(ctx, address, contractAddress)
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

	tmp := strings.Split(hash, ":")
	if len(tmp) != 2 {
		r.Rejected = true
		r.ErrMsg = "invalid params, pattern should be hash:from"
		return r, nil
	}
	tx, err := h.getTransactionReceipt(ctx, tmp[0], tmp[1])
	if err != nil {
		return r, fmt.Errorf("fail to getTransactionReceipt, err=%v", err)
	}

	transfer := &chainrpc.Transfer{}
	if len(tx.Transaction.Actions) != 1 {
		r.Rejected = true
		r.ErrMsg = "invalid length of Actions"
		return r, nil
	} else if tx.Transaction.Actions[0].Transfer.Deposit != "" &&
		tx.Transaction.Actions[0].FunctionCall.Args == "" {
		transfer.Sender = tx.Transaction.SignerID
		transfer.Recipient = tx.Transaction.ReceiverID
		transfer.ContractAddress = signing.MagicContactAddressForNative
		transfer.Amount = tx.Transaction.Actions[0].Transfer.Deposit
	} else if tx.Transaction.Actions[0].Transfer.Deposit == "" &&
		tx.Transaction.Actions[0].FunctionCall.Args != "" &&
		tx.Transaction.Actions[0].FunctionCall.MethodName == "ft_transfer" {
		transfer.Sender = tx.Transaction.SignerID
		transfer.ContractAddress = tx.Transaction.ReceiverID
		argsBytes, err := base64.StdEncoding.DecodeString(tx.Transaction.Actions[0].FunctionCall.Args)
		if err != nil {
			r.Rejected = true
			r.ErrMsg = "fail to decode for Args"
			return r, nil
		}
		nt := &nearTokenTransfer{}
		if err := json.Unmarshal(argsBytes, nt); err != nil {
			r.Rejected = true
			r.ErrMsg = "fail to unmarshal for argsBytes"
			return r, nil
		}
		transfer.Recipient = nt.ReceiverId
		transfer.Amount = nt.Amount
	} else {
		r.Rejected = true
		r.ErrMsg = "invalid tx"
		return r, nil
	}

	if transfer.Sender == "" || transfer.Recipient == "" || transfer.Amount == "" || transfer.ContractAddress == "" {
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
		return "", fmt.Errorf("fail to DecodeString for signedHex, err=%v", err)
	}
	rawBase64 := base64.StdEncoding.EncodeToString(signedBytes)
	req := &BaseRequest{JsonRPC: "2.0", ID: "dontcare", Method: "broadcast_tx_commit",
		Params: []string{rawBase64}}
	res := &ReceiptRes{}
	err = h.rpc.Post(ctx, res, "", req)
	if err != nil {
		return "", fmt.Errorf("fail to send tx, err=%v", err)
	} else if res.Error != nil {
		return "", fmt.Errorf("fail to send tx, errMsg=%v", res.Error)
	}
	return res.Result.Transaction.Hash, nil
}

func (h *Handler) CheckTx(ctx context.Context, hash string) (*chainrpc.TxResult, error) {
	tmp := strings.Split(hash, ":")
	if len(tmp) != 2 {
		return nil, fmt.Errorf("invalid params, pattern should be hash:from")
	}
	r := &chainrpc.TxResult{}
	receipt, err := h.getTransactionReceipt(ctx, tmp[0], tmp[1])
	if err != nil {
		return r, fmt.Errorf("fail to getTransactionReceipt, err=%v", err)
	}
	switch {
	case receipt.Status.Failure != nil:
		r.Status = signing.TxStatusFailed
	case receipt.Status.SuccessValue != nil:
		r.Status = signing.TxStatusSucceeded
		block, err := h.getBlockByHash(ctx, receipt.TransactionOutcome.BlockHash)
		if err != nil {
			return r, fmt.Errorf("fail to getBlockByHash, err=%w", err)
		}
		r.Height = strconv.FormatInt(block.Header.Height, 10)
		if block.Header.Timestamp > 1000000000 {
			r.Time = strconv.FormatInt(block.Header.Timestamp/1000000000, 10)
		}
	default:
		r.Status = signing.TxStatusPending
	}
	return r, nil
}

func (h *Handler) CallContract(ctx context.Context, contractAddress, params, blockNumber string) ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

func (h *Handler) InquireChain(ctx context.Context, instruction, params string) (string, error) {
	switch instruction {
	case "getNonce":
		var publicKey, accountId string
		tmp := strings.Split(params, ":")
		if len(tmp) == 2 {
			publicKey, accountId = tmp[0], tmp[1]
		} else {
			publicKey, accountId = params, params
		}
		publicKeyBytes, err := hex.DecodeString(publicKey)
		if err != nil {
			return "", fmt.Errorf("fail DecodeString, err=%v", err)
		}
		b58PubKey := "ed25519:" + base58.Encode(publicKeyBytes)
		req := &BaseRequest{JsonRPC: "2.0", ID: "dontcare", Method: "query",
			Params: map[string]interface{}{
				"request_type": "view_access_key",
				"account_id":   accountId,
				"public_key":   b58PubKey,
				"finality":     "final",
			}}
		res := &GetNonceRes{}
		err = h.rpc.Post(ctx, res, "", req)
		if err != nil {
			return "", fmt.Errorf("fail to get nonce, err=%v", err)
		} else if res.Error != nil {
			return "", fmt.Errorf("fail to get nonce, errMsg=%v", res.Error)
		} else if res.Result.Error != "" {
			return "", fmt.Errorf("fail to get nonce, result errMsg=%v", res.Result.Error)
		}
		return strconv.FormatUint(res.Result.Nonce+1, 10), nil
	case "getRefBlockHash":
		req := &BaseRequest{JsonRPC: "2.0", ID: "dontcare", Method: "block",
			Params: map[string]interface{}{"finality": "final"}}
		res := &GetBlockRes{}
		err := h.rpc.Post(ctx, res, "", req)
		if err != nil {
			return "", fmt.Errorf("fail to getRefBlockHash, err=%v", err)
		} else if res.Error != nil {
			return "", fmt.Errorf("fail to getRefBlockHash, errMsg=%v", res.Error)
		}
		return res.Result.Header.Hash, nil
	case "getRequiredDepositAmount":
		tmp := strings.Split(params, ":")
		if len(tmp) != 2 {
			return "", fmt.Errorf("invalid params")
		}
		storageBalance, err := h.getStorageBalance(ctx, tmp[0], tmp[1])
		if err != nil {
			return "", err
		}
		if storageBalance.Total != "" {
			return "0", nil
		}
		storageBounds, err := h.getStorageBounds(ctx, tmp[1])
		if err != nil {
			return "", fmt.Errorf("fail to getStorageBounds, err=%v", err)
		} else if storageBounds == nil {
			return "", fmt.Errorf("fail to getStorageBounds, storageBounds == nil")
		}
		return storageBounds.Min, nil
	}
	return "", fmt.Errorf("unsupported function")
}

func (h *Handler) getBaseCoinBalance(ctx context.Context, address string) (*big.Int, error) {
	req := &BaseRequest{JsonRPC: "2.0", ID: "dontcare", Method: "query",
		Params: map[string]interface{}{
			"finality":     "final",
			"request_type": "view_account",
			"account_id":   address,
		}}
	res := &GetBalanceRes{}
	err := h.rpc.Post(ctx, res, "", req)
	if err != nil {
		return nil, err
	} else if res.Error != nil {
		return nil, res.Error
	}
	bal, ok := big.NewInt(0).SetString(res.Result.Amount, 10)
	if !ok {
		return nil, fmt.Errorf("fail to SetString")
	}
	return bal, nil
}

func (h *Handler) getTokenBalance(ctx context.Context, address, contract string) (string, error) {
	contractParam, err := json.Marshal(map[string]interface{}{"account_id": address})
	if err != nil {
		return "", fmt.Errorf("fail to Marshal address, err=%v", err)
	}
	req := &BaseRequest{JsonRPC: "2.0", ID: "dontcare", Method: "query",
		Params: map[string]interface{}{
			"request_type": "call_function",
			"finality":     "final",
			"account_id":   contract,
			"method_name":  "ft_balance_of",
			"args_base64":  base64.StdEncoding.EncodeToString(contractParam),
		}}
	res := &ContractTokenRes{}
	err = h.rpc.Post(ctx, res, "", req)
	if err != nil {
		return "", fmt.Errorf("fail to call_function for ft_balance_of, err=%v", err)
	} else if res.Error != nil {
		return "", fmt.Errorf("fail to call_function for ft_balance_of, errMsg=%v", res.Error)
	}
	return strings.TrimPrefix(strings.TrimSuffix(string(res.Result.Result), `"`), `"`), nil
}

func (h *Handler) getTransactionReceipt(ctx context.Context, hash, address string) (*TransactionReceipt, error) {
	req := &BaseRequest{JsonRPC: "2.0", ID: "dontcare", Method: "tx", Params: []string{hash, address}}
	res := &ReceiptRes{}
	err := h.rpc.Post(ctx, res, "", req)
	if err != nil {
		return nil, err
	} else if res.Error != nil {
		return nil, res.Error
	}
	return &res.Result, nil
}

func (h *Handler) getBlockByHash(ctx context.Context, blockHash string) (*Block, error) {
	req := &BaseRequest{JsonRPC: "2.0", ID: "dontcare", Method: "block",
		Params: map[string]interface{}{"block_id": blockHash}}
	res := &GetBlockRes{}
	err := h.rpc.Post(ctx, res, "", req)
	if err != nil {
		return nil, err
	} else if res.Error != nil {
		return nil, res.Error
	}
	return &res.Result, nil
}

func (h *Handler) getStorageBalance(ctx context.Context, account, contract string) (*TokenStorageBalance, error) {
	contractParam, _ := json.Marshal(map[string]interface{}{"account_id": account})
	req := &BaseRequest{JsonRPC: "2.0", ID: "dontcare", Method: "query",
		Params: map[string]interface{}{
			"request_type": "call_function",
			"finality":     "final",
			"account_id":   contract,
			"method_name":  "storage_balance_of",
			"args_base64":  base64.StdEncoding.EncodeToString(contractParam),
		}}
	res := &ContractTokenRes{}
	err := h.rpc.Post(ctx, res, "", req)
	if err != nil {
		return nil, fmt.Errorf("fail to GetStorageBalance, err=%v", err)
	} else if res.Error != nil {
		return nil, fmt.Errorf("fail to GetStorageBalance, errMsg=%v", res.Error)
	}
	tokenStorage := &TokenStorageBalance{}
	if err := json.Unmarshal(res.Result.Result, tokenStorage); err != nil {
		return nil, fmt.Errorf("fail to Unmarshal tokenStorage, err=%v", err)
	}
	return tokenStorage, nil
}

func (h *Handler) getStorageBounds(ctx context.Context, contract string) (*TokenStorageBounds, error) {
	req := &BaseRequest{JsonRPC: "2.0", ID: "dontcare", Method: "query",
		Params: map[string]interface{}{
			"request_type": "call_function",
			"finality":     "final",
			"account_id":   contract,
			"method_name":  "storage_balance_bounds",
			"args_base64":  "",
		}}
	res := &ContractTokenRes{}
	err := h.rpc.Post(ctx, res, "", req)
	if err != nil {
		return nil, fmt.Errorf("fail to call_function for storage_balance_bounds, err=%v", err)
	} else if res.Error != nil {
		return nil, fmt.Errorf("fail to call_function for storage_balance_bounds, errMsg=%v", res.Error)
	}
	tokenStorageBounds := &TokenStorageBounds{}
	if err := json.Unmarshal(res.Result.Result, tokenStorageBounds); err != nil {
		return nil, fmt.Errorf("fail to Unmarshal, err=%v", err)
	}
	return tokenStorageBounds, nil
}
