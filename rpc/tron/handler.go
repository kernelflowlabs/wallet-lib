package tron

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/kernelflowlabs/wallet-sdk/common/httpc"
	chainrpc "github.com/kernelflowlabs/wallet-sdk/rpc"
	"github.com/kernelflowlabs/wallet-sdk/signing"
	wallettron "github.com/kernelflowlabs/wallet-sdk/signing/tron"
)

var _ chainrpc.BasicChainHandler = (*Handler)(nil)

type Handler struct {
	rpc *httpc.Request
}

func NewHandler(rpcUrl string) (*Handler, error) {
	h := &Handler{}

	h.rpc = httpc.NewRequest(rpcUrl, nil)
	return h, nil
}

func (h *Handler) GetHeight(ctx context.Context) (string, error) {
	block := &Block{}
	err := h.rpc.Post(ctx, block, "wallet/getnowblock", nil)
	if err != nil {
		return "", fmt.Errorf("failed to get latest block, err=%v", err)
	}
	height := block.BlockHeader.Data.Number
	if height == 0 {
		return "", fmt.Errorf("got zero")
	}
	return strconv.FormatUint(height, 10), nil
}

func (h *Handler) GetBalance(ctx context.Context, address, contractAddress, blockNumber string) (string, error) {
	if contractAddress == signing.MagicContactAddressForNative ||
		contractAddress == signing.MagicContactAddressForNativeTRX {
		bal, err := h.getBaseCoinBalance(ctx, address)
		if err != nil {
			return "", fmt.Errorf("failed to getBaseCoinBalance, err=%v", err)
		}
		return strconv.FormatUint(bal, 10), nil
	} else {
		balance, err := h.getTokenBalance(ctx, address, contractAddress)
		if err != nil {
			return "", fmt.Errorf("failed to getTokenBalance, err=%v", err)
		}
		return balance.String(), nil
	}
}

func (h *Handler) GetTransfersByHash(ctx context.Context, hash string,
	confirmation uint64, withInternal bool) (*chainrpc.TxTransfers, error) {
	result := &chainrpc.TxTransfers{
		Hash: hash,
	}
	txResult, err := h.CheckTx(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to CheckTx, err=%v", err)
	} else if txResult.Status == signing.TxStatusFailed {
		result.Rejected = true
		if txResult.ErrMsg != "" {
			result.ErrMsg = txResult.ErrMsg
		} else {
			result.ErrMsg = "not a succeeded tx"
		}
		return result, nil
	} else if txResult.Status == signing.TxStatusPending {
		result.ErrMsg = "pending tx"
		return result, nil
	} else if txResult.Status != signing.TxStatusSucceeded {
		result.ErrMsg = "not a succeeded tx"
		return result, nil
	}

	confirmed, err := chainrpc.Confirmations(ctx, txResult.Height, confirmation, h.GetHeight)
	if err != nil {
		return nil, fmt.Errorf("failed to check confirmations, err=%w", err)
	}
	if confirmed < confirmation {
		result.ErrMsg = fmt.Sprintf("tx succeeded.But current confirmation number %d hasn't meet "+
			"expected number %d", confirmed, confirmation)
		return result, nil
	}

	in := &GetTransactionInfoByIdReq{
		Value: hash,
	}
	tx := &Tx{}
	err = h.rpc.Post(ctx, tx, "wallet/gettransactionbyid", in)
	if err != nil {
		return nil, fmt.Errorf("failed to gettransactionbyid, err=%v", err)
	}

	transfer := &chainrpc.Transfer{}
	if tx.Data.Contracts[0].Type == "TransferContract" {
		transfer.Sender = wallettron.ConvertFromHex(tx.Data.Contracts[0].Parameter.Value.OwnerAddress)
		transfer.Recipient = wallettron.ConvertFromHex(tx.Data.Contracts[0].Parameter.Value.ToAddress)
		amt := tx.Data.Contracts[0].Parameter.Value.Amount
		transfer.Amount = amt.String()
		transfer.ContractAddress = signing.MagicContactAddressForNative

		if transfer.Sender != "" &&
			transfer.Recipient != "" &&
			transfer.Amount != "" &&
			transfer.ContractAddress != "" {
			result.Transfers = append(result.Transfers, transfer)
			result.BalanceChange = append(result.BalanceChange, &chainrpc.BalanceChange{
				Address:         transfer.Sender,
				ContractAddress: transfer.ContractAddress,
				Change:          new(big.Int).Neg(amt).String(),
			})
			result.BalanceChange = append(result.BalanceChange, &chainrpc.BalanceChange{
				Address:         transfer.Recipient,
				ContractAddress: transfer.ContractAddress,
				Change:          amt.String(),
			})
		}
	} else if tx.Data.Contracts[0].Type == "TriggerSmartContract" {
		transfers, changes, err := trc20TransfersFromLogs(txResult.Logs)
		if err != nil {
			result.Rejected = true
			result.ErrMsg = err.Error()
			return result, nil
		}
		result.Transfers = append(result.Transfers, transfers...)
		result.BalanceChange = append(result.BalanceChange, changes...)
	} else {
		result.Rejected = true
		result.ErrMsg = "invalid tx type"
		return result, nil
	}

	return result, nil
}

func trc20TransfersFromLogs(logs []chainrpc.EvmLog) ([]*chainrpc.Transfer, []*chainrpc.BalanceChange, error) {
	var transfers []*chainrpc.Transfer
	var changes []*chainrpc.BalanceChange
	for _, log := range logs {
		topics := strings.Split(log.Topics, ",")
		if len(topics) != 3 || !strings.EqualFold(strings.TrimPrefix(topics[0], "0x"), TransferEventTopic) {
			continue
		}
		sender := topicToAddress(topics[1])
		recipient := topicToAddress(topics[2])
		contract := wallettron.ConvertFromHex("41" + strings.TrimPrefix(log.Address, "0x"))
		amount, ok := new(big.Int).SetString(strings.TrimPrefix(log.Data, "0x"), 16)
		if !ok || sender == "" || recipient == "" || contract == "" {
			continue
		}
		transfers = append(transfers, &chainrpc.Transfer{
			Sender:          sender,
			Recipient:       recipient,
			Amount:          amount.String(),
			ContractAddress: contract,
		})
		changes = append(changes,
			&chainrpc.BalanceChange{Address: sender, ContractAddress: contract, Change: new(big.Int).Neg(amount).String()},
			&chainrpc.BalanceChange{Address: recipient, ContractAddress: contract, Change: amount.String()})
	}
	if len(transfers) == 0 {
		return nil, nil, fmt.Errorf("no transfer events")
	}
	return transfers, changes, nil
}

func topicToAddress(topic string) string {
	topic = strings.TrimPrefix(topic, "0x")
	if len(topic) != 64 {
		return ""
	}
	return wallettron.ConvertFromHex("41" + topic[24:])
}

func (h *Handler) GetAddressFee(ctx context.Context, address string) (wallettron.FreeGas, error) {
	res := wallettron.FreeGas{}
	ar, err := h.getAccountResource(ctx, address)
	if err != nil {
		return res, fmt.Errorf("failed to getAccountResource, err=%v", err)
	}
	res.FreeNetUsed = strconv.FormatInt(ar.FreeNetUsed, 10)
	res.FreeNetLimit = strconv.FormatInt(ar.FreeNetLimit, 10)
	return res, nil
}

func (h *Handler) SendTx(ctx context.Context, signedHex string) (string, error) {
	signedBytes, err := hex.DecodeString(strings.TrimPrefix(signedHex, "0x"))
	if err != nil {
		return "", fmt.Errorf("failed to DecodeString, err=%v", err)
	}
	ntx := &wallettron.NativeTx{}
	err = json.Unmarshal(signedBytes, ntx)
	if err != nil {
		return "", fmt.Errorf("failed to Unmarshal for ntx, err=%v", err)
	}
	req := &BroadcastJsonRawTransactionReq{
		Signature: ntx.Signature,
		ID:        ntx.ID,
		RawData:   ntx.RawData,
	}
	res := &BroadcastJsonRawTransactionRes{}
	err = h.rpc.Post(ctx, res, "wallet/broadcasttransaction", req)
	if err != nil {
		return "", fmt.Errorf("failed to broadcasttransaction, err=%v", err)
	} else if res.Result == false {
		return "", fmt.Errorf("failed to broadcasttransaction, errCode=%v, errMsg=%v", res.Code, res.Message)
	}

	return res.TxID, nil
}

func (h *Handler) CheckTx(ctx context.Context, hash string) (*chainrpc.TxResult, error) {
	if hash == "" {
		return nil, fmt.Errorf("hash is empty")
	}
	r := &chainrpc.TxResult{}

	req := &GetTransactionInfoByIdReq{
		Value: hash,
	}
	res := &GetTransactionInfoByIdRes{}
	err := h.rpc.Post(ctx, res, "wallet/gettransactioninfobyid", req)
	if err != nil {
		return nil, fmt.Errorf("failed to gettransactioninfobyid, err=%v", err)
	}

	r.Height = strconv.FormatUint(res.BlockNumber, 10)
	r.Time = strconv.FormatInt(res.BlockTimeStamp/1000, 10)
	r.GasUsed = strconv.FormatUint(res.Receipt.NetUsage, 10)
	if res.Result == "FAILED" {
		r.Status = signing.TxStatusFailed
		r.ErrMsg = res.Result
	} else {
		ntx := &wallettron.NativeTx{}
		err = h.rpc.Post(ctx, ntx, "wallet/gettransactionbyid", req)
		if err != nil {
			return nil, fmt.Errorf("failed to gettransactionbyid, err=%v", err)
		} else if len(ntx.Ret) < 1 {
			r.Status = signing.TxStatusPending
		} else if ntx.Ret[0].ContractRet == "SUCCESS" && r.Height != "0" {
			r.Status = signing.TxStatusSucceeded

			var logs []chainrpc.EvmLog
			for _, v := range res.Log {
				log := chainrpc.EvmLog{
					Address: "0x" + v.Address,
					Topics:  strings.Join(v.Topics, ","),
					Data:    v.Data,
				}
				logs = append(logs, log)
			}
			r.Logs = logs
		} else {
			r.Status = signing.TxStatusPending
		}
	}
	return r, nil
}

func (h *Handler) CallContract(ctx context.Context, contractAddress, params, blockNumber string) ([]byte, error) {
	return nil, fmt.Errorf("CallContract not supported")
}

func (h *Handler) InquireChain(ctx context.Context, instruction, params string) (string, error) {
	switch instruction {
	case "getRefInfo":
		blockHeight := params
		var err error
		if blockHeight == "" {
			blockHeight, err = h.GetHeight(ctx)
			if err != nil {
				return "", err
			}
		}
		blockNumberUint64, err := strconv.ParseUint(blockHeight, 10, 64)
		if err != nil {
			return "", fmt.Errorf("failed to ParseUint, err=%v", err)
		}
		block, err := h.getBlockByNumber(ctx, blockNumberUint64)
		if err != nil {
			return "", fmt.Errorf("failed to getBlockByNumber, err=%v", err)
		}
		return block.BlockId + "_" +
			blockHeight + "_" +
			strconv.FormatInt(block.BlockHeader.Data.Timestamp, 10), nil
	case "getAddressFee":
		res := wallettron.FreeGas{}
		ar, err := h.getAccountResource(ctx, params)
		if err != nil {
			return "", fmt.Errorf("failed to getAccountResource, err=%v", err)
		}
		res.FreeNetUsed = strconv.FormatInt(ar.FreeNetUsed, 10)
		res.FreeNetLimit = strconv.FormatInt(ar.FreeNetLimit, 10)
		resBytes, err := json.Marshal(res)
		if err != nil {
			return "", fmt.Errorf("failed to Marshal for res, err=%v", err)
		}
		return string(resBytes), nil
	case "getChainParameters":
		res, err := h.GetChainParameters(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to GetChainParameters, err=%v", err)
		}
		resBytes, err := json.Marshal(res)
		if err != nil {
			return "", fmt.Errorf("failed to Marshal for res, err=%v", err)
		}
		return string(resBytes), nil
	case "getAccountActivation":
		res, err := h.GetAccountActivation(ctx, params)
		if err != nil {
			return "", fmt.Errorf("failed to GetAccountActivation, err=%v", err)
		}
		resBytes, err := json.Marshal(res)
		if err != nil {
			return "", fmt.Errorf("failed to Marshal for res, err=%v", err)
		}
		return string(resBytes), nil
	case "getBlockByNumber":
		blockNumberUint64, err := strconv.ParseUint(params, 10, 64)
		if err != nil {
			return "", fmt.Errorf("failed to ParseUint, err=%v", err)
		}
		block, err := h.getBlockByNumber(ctx, blockNumberUint64)
		if err != nil {
			return "", fmt.Errorf("failed to getBlockByNumber, err=%v", err)
		}
		resBytes, err := json.Marshal(block)
		if err != nil {
			return "", fmt.Errorf("failed to Marshal, err=%v", err)
		}
		return string(resBytes), nil
	case "getAllowance":
		tmp := strings.Split(params, ":")
		if len(tmp) != 3 {
			return "", fmt.Errorf("len(tmp) != 3")
		}
		contractAddressHex := wallettron.ConvertToHex(tmp[0])
		if len(contractAddressHex) == 0 {
			return "", fmt.Errorf("contractAddressHex is empty")
		}
		ownerHex := wallettron.ConvertToHex(tmp[1])
		if len(ownerHex) == 0 {
			return "", fmt.Errorf("ownerHex is empty")
		}
		spenderHex := wallettron.ConvertToHex(tmp[2])
		if len(spenderHex) == 0 {
			return "", fmt.Errorf("spenderHex is empty")
		}
		in := &SmartContractReq{
			OwnerAddress:     ownerHex,
			ContractAddress:  contractAddressHex,
			FunctionSelector: "allowance(address,address)",
			Parameter: "000000000000000000000000" + ownerHex[2:] +
				"000000000000000000000000" + spenderHex[2:],
		}
		out := &ConstantSmartContractRes{}
		err := h.rpc.Post(ctx, out, "wallet/triggerconstantcontract", in)
		if err != nil {
			return "", err
		} else if out.Result.Result == false {
			return "", fmt.Errorf("get Result==false")
		}
		r, err := parseContractNumber(out.ConstantResult)
		if err != nil {
			return "", fmt.Errorf("failed to getAllowance, err=%w", err)
		}
		return r.String(), nil
	case "getTokenDecimals":
		if params == signing.MagicContactAddressForNative {
			return "6", nil
		}
		contractAddressHex := wallettron.ConvertToHex(params)
		if len(contractAddressHex) == 0 {
			return "", fmt.Errorf("contractAddressHex is empty")
		}
		in := &SmartContractReq{
			OwnerAddress:     contractAddressHex,
			ContractAddress:  contractAddressHex,
			FunctionSelector: "decimals()",
			Parameter:        "",
		}
		out := &ConstantSmartContractRes{}
		err := h.rpc.Post(ctx, out, "wallet/triggerconstantcontract", in)
		if err != nil {
			return "", err
		} else if out.Result.Result == false {
			return "", fmt.Errorf("get Result==false")
		}
		r, err := parseContractNumber(out.ConstantResult)
		if err != nil {
			return "", fmt.Errorf("failed to getTokenDecimals, err=%w", err)
		}
		return r.String(), nil
	}
	return "", fmt.Errorf("unsupported function")
}

// unexported
func (h *Handler) getBaseCoinBalance(ctx context.Context, address string) (uint64, error) {
	in := &GetAccountReq{
		wallettron.ConvertToHex(address),
	}
	out := &GetAccountRes{}
	err := h.rpc.Post(ctx, out, "wallet/getaccount", in)
	if err != nil {
		return 0, err
	}
	return out.Balance, nil
}
func (h *Handler) getTokenBalance(ctx context.Context, address string, contract string) (*big.Int, error) {
	addressHex := wallettron.ConvertToHex(address)
	if len(addressHex) == 0 {
		return nil, fmt.Errorf("addressHex is empty")
	}
	contractHex := wallettron.ConvertToHex(contract)
	if len(contract) == 0 {
		return nil, fmt.Errorf("contract is empty")
	}
	in := &SmartContractReq{
		OwnerAddress:     addressHex,
		ContractAddress:  contractHex,
		FunctionSelector: "balanceOf(address)",
		Parameter:        "000000000000000000000000" + addressHex[2:],
	}
	out := &ConstantSmartContractRes{}
	err := h.rpc.Post(ctx, out, "wallet/triggerconstantcontract", in)
	if err != nil {
		return nil, err
	} else if out.Result.Result == false {
		return nil, fmt.Errorf("get Result==false")
	}
	r, err := parseContractNumber(out.ConstantResult)
	if err != nil {
		return nil, fmt.Errorf("failed to parse balance, err=%w", err)
	}
	return r, nil
}
func (h *Handler) getBlockByNumber(ctx context.Context, num uint64) (*Block, error) {
	blocks := &Blocks{}
	err := h.rpc.Post(ctx, blocks, "wallet/getblockbylimitnext",
		BlockRequest{StartNum: num, EndNum: num + 1})
	if err != nil {
		return nil, err
	}
	if blocks.Blocks == nil {
		return nil, fmt.Errorf("get blocks.Blocks == nil")
	}
	if len(blocks.Blocks) == 0 {
		return nil, fmt.Errorf("len(blocks.Blocks) == 0")
	}
	return &blocks.Blocks[0], nil
}
func (h *Handler) GetChainParameters(ctx context.Context) (*ChainParameters, error) {
	out := &GetChainParametersOut{}
	if err := h.rpc.Post(ctx, out, "wallet/getchainparameters", struct{}{}); err != nil {
		return nil, err
	}
	if len(out.ChainParameter) == 0 {
		return nil, fmt.Errorf("empty chainParameter")
	}
	res := &ChainParameters{}
	for _, p := range out.ChainParameter {
		switch p.Key {
		case "getEnergyFee":
			res.EnergyFeeSun = strconv.FormatInt(p.Value, 10)
		case "getTransactionFee":
			res.TransactionFeeSun = strconv.FormatInt(p.Value, 10)
		case "getMaxFeeLimit":
			res.MaxFeeLimitSun = strconv.FormatInt(p.Value, 10)
		case "getCreateAccountFee":
			res.CreateAccountFeeSun = strconv.FormatInt(p.Value, 10)
		case "getCreateNewAccountFeeInSystemContract":
			res.ContractCreateAccountFeeSun = strconv.FormatInt(p.Value, 10)
		}
	}
	if res.EnergyFeeSun == "" {
		return nil, fmt.Errorf("getEnergyFee missing from chainParameter")
	}
	return res, nil
}

func (h *Handler) GetAccountActivation(ctx context.Context, address string) (*AccountActivation, error) {
	if !wallettron.ValidAddress(address) || len(address) != 34 {
		return nil, fmt.Errorf("invalid TRON address")
	}
	out := &GetAccountRes{}
	if err := h.rpc.Post(ctx, out, "wallet/getaccount", &GetAccountReq{Address: wallettron.ConvertToHex(address)}); err != nil {
		return nil, err
	}
	res := &AccountActivation{Address: address, Activated: out.Address != "" || out.CreateTime > 0}
	if res.Activated {
		return res, nil
	}
	params, err := h.GetChainParameters(ctx)
	if err != nil {
		return nil, err
	}
	if params.CreateAccountFeeSun == "" || params.ContractCreateAccountFeeSun == "" {
		return nil, fmt.Errorf("account creation fees missing from chainParameter")
	}
	res.NativeTransferFeeSun = params.CreateAccountFeeSun
	res.ContractTransferFeeSun = params.ContractCreateAccountFeeSun
	return res, nil
}

func (h *Handler) getAccountResource(ctx context.Context, address string) (*GetAccountResourceOut, error) {
	out := &GetAccountResourceOut{}
	err := h.rpc.Post(ctx, out, "wallet/getaccountresource",
		GetAccountResourceIn{Address: address, Visible: true})
	if err != nil {
		return nil, err
	}
	return out, nil
}
func parseContractNumber(results []string) (*big.Int, error) {
	if len(results) == 0 {
		return nil, fmt.Errorf("empty constant_result")
	}
	data := strings.TrimPrefix(results[0], "0x")
	if len(data) != 64 {
		return nil, fmt.Errorf("unexpected constant_result %q", results[0])
	}
	n, ok := new(big.Int).SetString(data, 16)
	if !ok {
		return nil, fmt.Errorf("invalid constant_result %q", results[0])
	}
	return n, nil
}
