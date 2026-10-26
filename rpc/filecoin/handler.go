package filecoin

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
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
	rpcBase := ""
	tmp := strings.Split(rpcUrl, "@")
	headers := map[string]string{"Content-Type": "application/json"}
	if len(tmp) == 1 {
		rpcBase = tmp[0]
	} else if len(tmp) == 2 {
		rpcBase = tmp[0]
		headers["Authorization"] = "Bearer " + tmp[1]
	} else {
		return nil, fmt.Errorf("invalid params")
	}
	return &Handler{rpc: httpc.NewRequest(rpcBase, headers)}, nil
}

func (h *Handler) GetHeight(ctx context.Context) (string, error) {
	req := &BaseRequest{JsonRPC: "2.0", ID: 1, Method: "Filecoin.ChainHead"}
	res := &GetBlockHeightRes{Result: &GetBlockHeight{}}
	err := h.rpc.Post(ctx, res, "", req)
	if err != nil {
		return "", fmt.Errorf("fail to GetHeight, err=%v", err)
	} else if res.Error.Code != 0 {
		return "", fmt.Errorf("fail to GetHeight, err=%v", res.Error.Message)
	} else if res.Result == nil {
		return "", fmt.Errorf("fail to GetHeight, result is empty")
	}
	return strconv.FormatUint(res.Result.Height, 10), nil
}

func (h *Handler) GetBalance(ctx context.Context, address, contractAddress, blockNumber string) (string, error) {
	if contractAddress != signing.MagicContactAddressForNative {
		return "", fmt.Errorf("only basecoin supported")
	}
	req := &BaseRequest{JsonRPC: "2.0", ID: 1, Method: "Filecoin.WalletBalance", Params: []string{address}}
	res := &GetBalanceResponse{}
	err := h.rpc.Post(ctx, res, "", req)
	if err != nil {
		return "", fmt.Errorf("fail to WalletBalance, err=%v", err)
	} else if res.Error.Code != 0 {
		return "", fmt.Errorf("getBalance:%s", res.Error.Message)
	}
	bal, ok := new(big.Int).SetString(res.Result, 10)
	if !ok {
		return "", fmt.Errorf("fail to SetString for balance result")
	}
	return bal.String(), nil
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

	message, err := h.getMessageByHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("fail to getMessageByHash, hash=%s, err=%v", hash, err)
	}
	transfer := &chainrpc.Transfer{
		Sender:          message.From,
		Recipient:       message.To,
		Amount:          message.Value,
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
		return "", fmt.Errorf("fail to DecodeString, err=%v", err)
	}
	signedTx := &SignedMessage{}
	if err := json.Unmarshal(signedBytes, signedTx); err != nil {
		return "", fmt.Errorf("fail to Unmarshal for signedTx, err=%v", err)
	}
	req := &BaseRequest{JsonRPC: "2.0", ID: 1, Method: "Filecoin.MpoolPush",
		Params: []interface{}{signedTx}}
	res := &MpoolPushResponse{}
	err = h.rpc.Post(ctx, res, "", req)
	if err != nil {
		return "", fmt.Errorf("fail to send tx, err=%v", err)
	} else if res.Error.Code != 0 {
		return "", fmt.Errorf("fail to send tx, errMsg=%v", res.Error.Message)
	}
	return res.Result.Cid, nil
}

func (h *Handler) CheckTx(ctx context.Context, hash string) (*chainrpc.TxResult, error) {
	r := &chainrpc.TxResult{}
	ts, err := h.checkTransactionStatus(ctx, hash)
	if err != nil {
		if strings.Contains(err.Error(), "could not find") ||
			strings.Contains(err.Error(), "invalid character '<' looking for beginning of value") {
			r.Status = signing.TxStatusPending
			return r, nil
		}
		return r, fmt.Errorf("fail to get status of hash=%s, err=%v", hash, err)
	} else if ts == nil || ts.Height == 0 {
		r.Status = signing.TxStatusPending
	} else {
		r.Height = strconv.FormatUint(ts.Height-1, 10)
		r.GasUsed = strconv.FormatUint(ts.Receipt.GasUsed, 10)
		g, err := h.getTipSetByHeight(ctx, ts.Height-1)
		if err == nil && len(g.Blocks) != 0 {
			r.Time = strconv.FormatInt(g.Blocks[0].Timestamp, 10)
		}
		if ts.Receipt.ExitCode == 0 {
			r.Status = signing.TxStatusSucceeded
		} else {
			r.Status = signing.TxStatusFailed
			r.ErrMsg = ts.Receipt.Return
		}
	}
	return r, nil
}

func (h *Handler) CallContract(ctx context.Context, contractAddress, params, blockNumber string) ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

func (h *Handler) InquireChain(ctx context.Context, instruction, params string) (string, error) {
	switch instruction {
	case "getNonce":
		req := &BaseRequest{JsonRPC: "2.0", ID: 1, Method: "Filecoin.MpoolGetNonce",
			Params: []interface{}{params}}
		res := &GetMpoolGetNonceRes{}
		err := h.rpc.Post(ctx, res, "", req)
		if err != nil {
			return "", fmt.Errorf("fail to GetNonce, err=%v", err)
		} else if res.Error.Code != 0 {
			return "", fmt.Errorf("fail to GetNonce, errMsg=%v", res.Error.Message)
		}
		return strconv.FormatUint(res.Result, 10), nil
	case "estimateGas":
		var p struct {
			From  string `json:"from"`
			To    string `json:"to"`
			Value string `json:"value"`
			Nonce string `json:"nonce"`
		}
		if err := json.Unmarshal([]byte(params), &p); err != nil {
			return "", fmt.Errorf("estimateGas: invalid params, err=%v", err)
		}
		nonce, err := strconv.ParseUint(p.Nonce, 10, 64)
		if err != nil {
			return "", fmt.Errorf("estimateGas: invalid nonce, err=%v", err)
		}
		msg := map[string]interface{}{
			"Version": 0, "To": p.To, "From": p.From, "Nonce": nonce,
			"Value": p.Value, "GasLimit": 0, "GasFeeCap": "0", "GasPremium": "0",
			"Method": 0, "Params": "",
		}
		req := &BaseRequest{JsonRPC: "2.0", ID: 1, Method: "Filecoin.GasEstimateMessageGas",
			Params: []interface{}{msg, map[string]interface{}{"MaxFee": "0"}, nil}}
		res := &GasEstimateRes{}
		if err := h.rpc.Post(ctx, res, "", req); err != nil {
			return "", fmt.Errorf("fail to GasEstimateMessageGas, err=%v", err)
		} else if res.Error.Code != 0 {
			return "", fmt.Errorf("fail to GasEstimateMessageGas, errMsg=%v", res.Error.Message)
		}
		gasModel := map[string]string{
			"gasLimit":   strconv.FormatInt(res.Result.GasLimit, 10),
			"gasFeeCap":  res.Result.GasFeeCap,
			"gasPremium": res.Result.GasPremium,
		}
		gasModelBytes, err := json.Marshal(gasModel)
		if err != nil {
			return "", err
		}
		return string(gasModelBytes), nil
	}
	return "", fmt.Errorf("unsupported function")
}

func (h *Handler) checkTransactionStatus(ctx context.Context, cid string) (*StateSearchMsgLimited, error) {
	req := &BaseRequest{JsonRPC: "2.0", ID: 1, Method: "Filecoin.StateSearchMsgLimited",
		Params: []interface{}{map[string]interface{}{"/": cid}, 10000}}
	res := &StateSearchMsgLimitedRes{}
	err := h.rpc.Post(ctx, res, "", req)
	if err != nil {
		return nil, err
	} else if res.Error.Code != 0 {
		return nil, fmt.Errorf("checkTransactionStatus:%s", res.Error.Message)
	}
	return res.Result, nil
}

func (h *Handler) getTipSetByHeight(ctx context.Context, height uint64) (*GetTipSetByHeight, error) {
	req := &BaseRequest{JsonRPC: "2.0", ID: 1, Method: "Filecoin.ChainGetTipSetByHeight",
		Params: []interface{}{height, nil}}
	res := &GetTipSetByHeightRes{}
	err := h.rpc.Post(ctx, res, "", req)
	if err != nil {
		return nil, err
	} else if res.Error.Code != 0 {
		return nil, fmt.Errorf("getTipSetByHeight:%s", res.Error.Message)
	}
	return res.Result, nil
}

func (h *Handler) getMessageByHash(ctx context.Context, cid string) (*Message, error) {
	req := &BaseRequest{JsonRPC: "2.0", ID: 1, Method: "Filecoin.ChainGetMessage",
		Params: []interface{}{map[string]interface{}{"/": cid}}}
	res := &GetMessageResponse{}
	err := h.rpc.Post(ctx, res, "", req)
	if err != nil {
		return nil, err
	} else if res.Error.Code != 0 {
		return nil, fmt.Errorf("getMessageByHash:%s", res.Error.Message)
	}
	return res.Result, nil
}
