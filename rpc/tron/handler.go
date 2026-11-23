package tron

import (
	"bytes"
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
	wallettron "github.com/kernelflowlabs/wallet-lib/signing/tron"
)

var _ chainrpc.BasicChainHandler = (*Handler)(nil)

type Handler struct {
	rpc *httpc.Request
}

func NewHandler(rpcUrl, apiKey string, options ...httpc.Option) (*Handler, error) {
	headers := map[string]string{}
	if apiKey != "" {
		headers["TRON-PRO-API-KEY"] = apiKey
	}
	client, err := httpc.NewRequestWithOptions(rpcUrl, headers, options...)
	if err != nil {
		return nil, err
	}
	return &Handler{rpc: client}, nil
}

func (h *Handler) GetHeight(ctx context.Context) (string, error) {
	block := &Block{}
	err := h.rpc.Post(ctx, block, "wallet/getnowblock", nil)
	if err != nil {
		return "", fmt.Errorf("failed to get latest block, err=%w", err)
	}
	height := block.BlockHeader.Data.Number
	if height == 0 {
		return "", fmt.Errorf("got zero")
	}
	return strconv.FormatUint(height, 10), nil
}

func (h *Handler) GetSolidifiedHeight(ctx context.Context) (string, error) {
	result, err := h.queryJSON(ctx, "walletsolidity/getnowblock", nil, '{')
	if err != nil {
		return "", err
	}
	var block struct {
		BlockHeader *struct {
			Data *struct {
				Number *uint64 `json:"number"`
			} `json:"raw_data"`
		} `json:"block_header"`
	}
	if err := json.Unmarshal(result, &block); err != nil {
		return "", fmt.Errorf("invalid solidified block: %w", err)
	}
	if block.BlockHeader == nil || block.BlockHeader.Data == nil || block.BlockHeader.Data.Number == nil {
		return "", fmt.Errorf("solidified block height missing")
	}
	return strconv.FormatUint(*block.BlockHeader.Data.Number, 10), nil
}

func (h *Handler) queryJSON(ctx context.Context, path string, body interface{}, shape byte) (json.RawMessage, error) {
	var result json.RawMessage
	if err := h.rpc.Post(ctx, &result, path, body); err != nil {
		return nil, fmt.Errorf("failed to %s: %w", path, err)
	}
	if len(result) > 0 && result[0] == '{' {
		var response struct {
			Error string `json:"Error"`
		}
		if err := json.Unmarshal(result, &response); err != nil {
			return nil, fmt.Errorf("invalid %s response: %w", path, err)
		}
		if response.Error != "" {
			return nil, fmt.Errorf("failed to %s: %s", path, response.Error)
		}
	}
	if len(result) == 0 || result[0] != shape {
		return nil, fmt.Errorf("invalid %s response", path)
	}
	return result, nil
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

	if tx.Error != "" {
		return nil, fmt.Errorf("failed to gettransactionbyid: %s", tx.Error)
	}

	if len(tx.Data.Contracts) == 0 {
		return nil, fmt.Errorf("transaction has no contracts")
	}
	contract := tx.Data.Contracts[0]
	value := contract.Parameter.Value
	if contract.Type == "TransferContract" {
		sender := wallettron.ConvertFromHex(value.OwnerAddress)
		recipient := wallettron.ConvertFromHex(value.ToAddress)
		if value.Amount != nil && sender != "" && recipient != "" {
			result.Transfers = append(result.Transfers, &chainrpc.Transfer{
				Sender: sender, Recipient: recipient, Amount: value.Amount.String(),
				ContractAddress: signing.MagicContactAddressForNative,
			})
		}
	} else if contract.Type == "TriggerSmartContract" {
		transfers, _, err := trc20TransfersFromLogs(txResult.Logs)
		if err == nil {
			result.Transfers = append(result.Transfers, transfers...)
		}
		if value.CallValue != nil && value.CallValue.Sign() > 0 {
			result.Transfers = append(result.Transfers, &chainrpc.Transfer{
				Sender:          wallettron.ConvertFromHex(value.OwnerAddress),
				Recipient:       wallettron.ConvertFromHex(value.ContractAddress),
				Amount:          value.CallValue.String(),
				ContractAddress: signing.MagicContactAddressForNative,
			})
		}
	} else {
		result.Rejected = true
		result.ErrMsg = "invalid tx type"
		return result, nil
	}
	if withInternal {
		info := &GetTransactionInfoByIdRes{}
		if err := h.rpc.Post(ctx, info, "wallet/gettransactioninfobyid", in); err != nil {
			return nil, fmt.Errorf("failed to gettransactioninfobyid, err=%v", err)
		}
		if info.Error != "" {
			return nil, fmt.Errorf("failed to gettransactioninfobyid: %s", info.Error)
		}
		for _, internal := range info.InternalTransactions {
			if internal.Rejected {
				continue
			}
			amount := new(big.Int)
			for _, value := range internal.CallValueInfo {
				if (value.TokenID == "" || value.TokenID == "_") && value.CallValue != nil && value.CallValue.Sign() > 0 {
					amount.Add(amount, value.CallValue)
				}
			}
			if amount.Sign() > 0 {
				result.Transfers = append(result.Transfers, &chainrpc.Transfer{
					Sender:          wallettron.ConvertFromHex(internal.CallerAddress),
					Recipient:       wallettron.ConvertFromHex(internal.TransferToAddress),
					Amount:          amount.String(),
					ContractAddress: signing.MagicContactAddressForNative,
				})
			}
		}
	}
	balanceMap := make(map[string]map[string]*big.Int)
	for _, transfer := range result.Transfers {
		amount, ok := new(big.Int).SetString(transfer.Amount, 10)
		if !ok {
			return nil, fmt.Errorf("invalid transfer amount %q", transfer.Amount)
		}
		for i, address := range []string{transfer.Sender, transfer.Recipient} {
			if balanceMap[address] == nil {
				balanceMap[address] = make(map[string]*big.Int)
			}
			if balanceMap[address][transfer.ContractAddress] == nil {
				balanceMap[address][transfer.ContractAddress] = new(big.Int)
			}
			if i == 0 {
				balanceMap[address][transfer.ContractAddress].Sub(balanceMap[address][transfer.ContractAddress], amount)
			} else {
				balanceMap[address][transfer.ContractAddress].Add(balanceMap[address][transfer.ContractAddress], amount)
			}
		}
	}
	for address, contracts := range balanceMap {
		for contract, change := range contracts {
			if change.Sign() != 0 {
				result.BalanceChange = append(result.BalanceChange, &chainrpc.BalanceChange{
					Address: address, ContractAddress: contract, Change: change.String(),
				})
			}
		}
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
	var req interface{}
	endpoint := "wallet/broadcasthex"
	if trimmed := bytes.TrimSpace(signedBytes); len(trimmed) > 0 && trimmed[0] == '{' {
		ntx := &wallettron.NativeTx{}
		err = json.Unmarshal(signedBytes, ntx)
		if err != nil {
			return "", fmt.Errorf("failed to Unmarshal for ntx, err=%v", err)
		}
		req = &BroadcastJsonRawTransactionReq{
			Signature: ntx.Signature,
			ID:        ntx.ID,
			RawData:   ntx.RawData,
		}
		endpoint = "wallet/broadcasttransaction"
	} else {
		req = &BroadcastProtoRawTransactionReq{Transaction: strings.TrimPrefix(signedHex, "0x")}
	}
	res := &BroadcastJsonRawTransactionRes{}
	err = h.rpc.Post(ctx, res, endpoint, req)
	if err != nil {
		return "", fmt.Errorf("failed to broadcasttransaction, err=%v", err)
	} else if res.Result == false {
		return "", fmt.Errorf("failed to broadcasttransaction, errCode=%v, errMsg=%v", res.Code, res.Message)
	}

	if res.TxID == "" {
		return "", fmt.Errorf("failed to broadcasttransaction, txid is empty")
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

	if res.Error != "" {
		return nil, fmt.Errorf("failed to gettransactioninfobyid: %s", res.Error)
	}

	r.Height = strconv.FormatUint(res.BlockNumber, 10)
	r.Time = strconv.FormatInt(res.BlockTimeStamp/1000, 10)
	r.GasUsed = strconv.FormatUint(res.Receipt.NetUsage, 10)
	if res.Result == "FAILED" {
		r.Status = signing.TxStatusFailed
		r.ErrMsg = res.Result
	} else {
		ntx := &Tx{}
		err = h.rpc.Post(ctx, ntx, "wallet/gettransactionbyid", req)
		if err != nil {
			return nil, fmt.Errorf("failed to gettransactionbyid, err=%v", err)
		} else if ntx.Error != "" {
			return nil, fmt.Errorf("failed to gettransactionbyid: %s", ntx.Error)
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
		} else if ntx.Ret[0].ContractRet != "" && ntx.Ret[0].ContractRet != "SUCCESS" && res.BlockNumber != 0 {
			r.Status = signing.TxStatusFailed
			r.ErrMsg = ntx.Ret[0].ContractRet
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
	case "getblockbynum", "gettransactioninfobyblocknum":
		blockNumber, err := strconv.ParseUint(params, 10, 63)
		if err != nil {
			return "", fmt.Errorf("invalid params: block number must be a nonnegative decimal int64")
		}
		shape := byte('{')
		if instruction == "gettransactioninfobyblocknum" {
			shape = '['
		}
		result, err := h.queryJSON(ctx, "wallet/"+instruction, map[string]uint64{"num": blockNumber}, shape)
		if err != nil {
			return "", err
		}
		if instruction == "getblockbynum" {
			var block struct {
				BlockID     string    `json:"blockID"`
				BlockHeader *struct{} `json:"block_header"`
			}
			if err := json.Unmarshal(result, &block); err != nil {
				return "", fmt.Errorf("invalid block: %w", err)
			}
			if block.BlockID == "" || block.BlockHeader == nil {
				return "", fmt.Errorf("block not found")
			}
		}
		return string(result), nil
	case "getSolidifiedHeight":
		return h.GetSolidifiedHeight(ctx)
	case "getSolidifiedTransactionInfoById":
		hash, err := hex.DecodeString(params)
		if err != nil || len(hash) != 32 {
			return "", fmt.Errorf("invalid params: transaction ID must be 32 bytes of hex without a prefix")
		}
		result, err := h.queryJSON(ctx, "walletsolidity/gettransactioninfobyid", GetTransactionInfoByIdReq{Value: params}, '{')
		if err != nil {
			return "", err
		}
		var receipt struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(result, &receipt); err != nil {
			return "", fmt.Errorf("invalid solidified receipt: %w", err)
		}
		if receipt.ID == "" {
			return "", fmt.Errorf("solidified transaction not found")
		}
		if !strings.EqualFold(receipt.ID, params) {
			return "", fmt.Errorf("solidified transaction ID mismatch")
		}
		return string(result), nil
	case "getblockbylimitnext":
		var request struct {
			StartNum *uint64 `json:"startNum"`
			EndNum   *uint64 `json:"endNum"`
		}
		if err := json.Unmarshal([]byte(params), &request); err != nil || request.StartNum == nil || request.EndNum == nil ||
			*request.StartNum >= *request.EndNum || *request.EndNum > uint64(1<<63-1) {
			return "", fmt.Errorf("invalid params: require 0 <= startNum < endNum <= int64 max")
		}
		result, err := h.queryJSON(ctx, "wallet/getblockbylimitnext", request, '{')
		if err != nil {
			return "", err
		}
		var response struct {
			Blocks []struct {
				ID     string `json:"blockID"`
				Header *struct {
					Data *struct {
						Number *uint64 `json:"number"`
					} `json:"raw_data"`
				} `json:"block_header"`
			} `json:"block"`
		}
		if err := json.Unmarshal(result, &response); err != nil {
			return "", fmt.Errorf("invalid block range: %w", err)
		}
		if uint64(len(response.Blocks)) != *request.EndNum-*request.StartNum {
			return "", fmt.Errorf("incomplete block range")
		}
		for i, block := range response.Blocks {
			if block.ID == "" || block.Header == nil || block.Header.Data == nil || block.Header.Data.Number == nil ||
				*block.Header.Data.Number != *request.StartNum+uint64(i) {
				return "", fmt.Errorf("invalid block at height %d", *request.StartNum+uint64(i))
			}
		}
		return string(result), nil
	case "isTxExisted":
		req := &GetTransactionInfoByIdReq{Value: params}
		res := &Tx{}
		if err := h.rpc.Post(ctx, res, "wallet/gettransactionbyid", req); err != nil {
			return "", fmt.Errorf("failed to gettransactionbyid, err=%v", err)
		}
		if res.Error != "" {
			return "", fmt.Errorf("failed to gettransactionbyid: %s", res.Error)
		}
		return strconv.FormatBool(res.ID != ""), nil
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
