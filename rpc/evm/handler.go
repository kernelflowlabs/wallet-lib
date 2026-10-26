package evm

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/kernelflowlabs/wallet-lib/contracts/erc20"
	chainrpc "github.com/kernelflowlabs/wallet-lib/rpc"
	"github.com/kernelflowlabs/wallet-lib/signing"
	walletevm "github.com/kernelflowlabs/wallet-lib/signing/evm"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

var _ chainrpc.BasicChainHandler = (*Handler)(nil)

type Handler struct {
	rpc     *rpc.Client
	client  *ethclient.Client
	chainId int64
	network string
}

func NewHandler(rpcUrl string, network string) (*Handler, error) {
	h := &Handler{}
	rc, err := rpc.Dial(rpcUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to Dial, err=%v", err)
	}
	ec := ethclient.NewClient(rc)
	h.rpc = rc
	h.client = ec

	chainIdInt, err := strconv.ParseInt(network, 10, 64)
	if err != nil {
		return nil, err
	}
	h.chainId = chainIdInt
	h.network = network
	return h, nil
}

func (h *Handler) GetHeight(ctx context.Context) (string, error) {
	var blockNumber string
	err := h.rpc.CallContext(ctx, &blockNumber, "eth_blockNumber")
	if err != nil {
		return "", fmt.Errorf("failed to eth_blockNumber, err=%v", err)
	} else if blockNumber == "" || blockNumber == "0" {
		return "", fmt.Errorf("got zero")
	}
	height, err := strconv.ParseUint(blockNumber, 0, 64)
	if err != nil {
		return "", fmt.Errorf("failed to ParseUint for blockNumber, err=%v", err)
	}
	return strconv.FormatUint(height, 10), nil
}

func (h *Handler) GetBalance(ctx context.Context, address, contractAddress, blockNumber string) (string, error) {
	if !walletevm.ValidAddress(address) {
		return "", fmt.Errorf("invalid address")
	}

	var err error
	if blockNumber == "" {
		blockNumber = "0"
	}
	blockNumberBig, ok := new(big.Int).SetString(blockNumber, 10)
	if !ok {
		return "", fmt.Errorf("failed to SetString for blockNumber")
	}
	if contractAddress == signing.MagicContactAddressForNative {
		balance := big.NewInt(0)
		if blockNumberBig.Cmp(big.NewInt(0)) == 0 {
			balance, err = h.client.BalanceAt(ctx, common.HexToAddress(address), nil)
			if err != nil {
				return "", fmt.Errorf("failed to BalanceAt, err=%v", err)
			}
		} else {
			balance, err = h.client.BalanceAt(ctx, common.HexToAddress(address), blockNumberBig)
			if err != nil {
				return "", fmt.Errorf("failed to BalanceAt, err=%v", err)
			}
		}
		return balance.String(), nil
	} else if walletevm.ValidAddress(contractAddress) {
		p := erc20.CallErc20In{
			Address: address,
		}
		params, _ := json.Marshal(p)
		var out interface{}
		out, err = h.callContractErc20(ctx, contractAddress, "balanceOf", params, blockNumberBig.Uint64())
		if err != nil {
			return "", fmt.Errorf("failed to balanceOf for contractAddress=%s,address=%s,err=%v",
				contractAddress, address, err)
		}

		balance, ok := out.(string)
		if !ok {
			return "", fmt.Errorf("out is not a string")
		}
		return balance, nil
	}
	return "", fmt.Errorf("invalid contractAddress")
}

func (h *Handler) GetTransfersByHash(ctx context.Context, hash string, confirmation uint64,
	withInternal bool) (*chainrpc.TxTransfers, error) {
	result := &chainrpc.TxTransfers{
		Hash:      hash,
		Transfers: make([]*chainrpc.Transfer, 0),
	}

	txResult, err := h.CheckTx(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to CheckTx, err=%v", err)
	} else if txResult.Status == signing.TxStatusFailed {
		if txResult.ErrMsg != "" {
			result.ErrMsg = txResult.ErrMsg
		} else {
			result.ErrMsg = "not a succeeded tx"
		}
		result.Rejected = true
		return result, nil
	} else if txResult.Status == signing.TxStatusPending {
		result.ErrMsg = "pending tx"
		return result, nil
	} else if txResult.Status != signing.TxStatusSucceeded {
		result.ErrMsg = "not a succeeded tx"
		return result, nil
	}

	height, err := strconv.ParseUint(txResult.Height, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse tx height %q: %v", txResult.Height, err)
	}
	latestHeightStr, err := h.GetHeight(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to GetHeight, err=%v", err)
	}
	latestHeight, err := strconv.ParseUint(latestHeightStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse latest height %q: %v", latestHeightStr, err)
	}
	var confirmed uint64
	if latestHeight >= height {
		confirmed = latestHeight - height
	}
	if confirmed < confirmation {
		result.ErrMsg = fmt.Sprintf("tx succeeded. But current confirmation number %d hasn't meet "+
			"expected number %d", confirmed, confirmation)
		return result, nil
	}

	tx := &RpcTransaction{}
	err = h.rpc.CallContext(ctx, tx, "eth_getTransactionByHash", hash)
	if err != nil {
		return nil, fmt.Errorf("failed to eth_getTransactionByHash, err=%v", err)
	} else if tx.Hash == "" {
		return nil, fmt.Errorf("failed to eth_getTransactionByHash, tx==nil or empty hash")
	}

	isContract, err := h.isContractAddress(ctx, tx.To)
	if err != nil {
		return nil, fmt.Errorf("failed to isContractAddress, err=%v", err)
	}
	if !isContract {
		transfers, balanceChange, err := h.getTxTransferForEVMFromNative(tx)
		if err != nil {
			result.Rejected = true
			result.ErrMsg = err.Error()
			return result, nil
		}
		result.Transfers = transfers
		result.BalanceChange = balanceChange
	} else {
		receipt := &RpcReceipt{}
		err = h.rpc.CallContext(ctx, receipt, "eth_getTransactionReceipt", hash)
		if err != nil {
			result.ErrMsg = err.Error()
			return result, nil
		}
		internalTxs := make([]*RpcInternalTx, 0)
		if withInternal {
			_ = h.rpc.CallContext(ctx, &internalTxs, "trace_transaction", hash)
		}
		transfers, balanceChange, err := h.getTxTransferForEVMFromContract(tx, receipt, internalTxs)
		if err != nil {
			result.Rejected = true
			result.ErrMsg = err.Error()
			return result, nil
		}
		result.Transfers = transfers
		result.BalanceChange = balanceChange
	}

	return result, nil
}

func (h *Handler) SendTx(ctx context.Context, signedHex string) (string, error) {
	if !strings.HasPrefix(signedHex, "0x") {
		signedHex = "0x" + signedHex
	}
	var raw json.RawMessage
	err := h.rpc.CallContext(ctx, &raw, "eth_sendRawTransaction", signedHex)
	if err != nil {
		return "", err
	} else if len(raw) == 0 {
		return "", fmt.Errorf("len of raw is 0")
	}
	var result string
	if err = json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("failed to unmarshal raw to string")
	}
	if result == "" {
		return "", fmt.Errorf("empty result")
	}
	return result, nil
}

func (h *Handler) CheckTx(ctx context.Context, hash string) (*chainrpc.TxResult, error) {
	r := &chainrpc.TxResult{}

	tx := &RpcTransaction{}
	err := h.rpc.CallContext(ctx, tx, "eth_getTransactionByHash", hash)
	if err != nil {
		return nil, fmt.Errorf("failed to eth_getTransactionByHash, err=%v", err)
	} else if tx.Hash == "" {
		r.Status = signing.TxStatusPending
		return r, nil
	} else if tx.BlockNumber == "" {
		r.Status = signing.TxStatusPending
		return r, nil
	}

	receipt := &RpcReceipt{}
	err = h.rpc.CallContext(ctx, &receipt, "eth_getTransactionReceipt", hash)
	if err != nil {
		return nil, fmt.Errorf("failed to eth_getTransactionReceipt, err=%v", err)
	} else if receipt == nil || receipt.BlockNumber == "" {
		r.Status = signing.TxStatusPending
		return r, nil
	}

	if receipt.Status == "0x0" {
		r.Status = signing.TxStatusFailed
		params := map[string]string{
			"from":  tx.From,
			"to":    tx.To,
			"data":  tx.Payload,
			"nonce": tx.Nonce,
			"gas":   tx.GasLimit,
		}
		reason := ""
		err = h.rpc.CallContext(ctx, &reason, "eth_call", params, tx.BlockNumber) //"latest"
		if err != nil {
			r.ErrMsg = err.Error()
		} else {
			r.ErrMsg = reason
		}
	} else if receipt.Status == "0x1" {
		blockNumber, _ := strconv.ParseUint(strings.TrimPrefix(receipt.BlockNumber, "0x"), 16, 64)
		r.Height = strconv.FormatUint(blockNumber, 10)
		r.Status = signing.TxStatusSucceeded
	} else {
		return nil, fmt.Errorf("weird")
	}
	var logs []chainrpc.EvmLog
	for _, v := range receipt.Logs {
		log := chainrpc.EvmLog{
			Address: v.Address,
			Topics:  strings.Join(v.Topics, ","),
			Data:    v.Data,
		}
		logs = append(logs, log)
	}
	r.Logs = logs

	return r, nil
}

func (h *Handler) CallContract(ctx context.Context, contractAddress, params, blockNumber string) ([]byte, error) {
	var msg ethereum.CallMsg
	var to common.Address
	var data []byte
	var err error
	var resBytes []byte

	to = common.HexToAddress(contractAddress)
	msg.To = &to
	data, err = hex.DecodeString(params)
	if err != nil {
		return nil, fmt.Errorf("failed to DecodeString for payload, err=%v", err)
	}
	msg.Data = data
	if blockNumber == "0" || blockNumber == "" {
		resBytes, err = h.client.CallContract(ctx, msg, nil)
		if err != nil {
			return nil, err
		}
	} else {
		blockNumberBig, ok := new(big.Int).SetString(blockNumber, 10)
		if !ok {
			return nil, fmt.Errorf("invalid blockNumber")
		}
		resBytes, err = h.client.CallContract(ctx, msg, blockNumberBig)
		if err != nil {
			return nil, err
		}
	}
	return resBytes, nil
}

func (h *Handler) InquireChain(ctx context.Context, instruction, params string) (string, error) {
	switch instruction {
	case "getTransactionByHash":
		tx := chainrpc.BasicEvmTx{}
		err := h.rpc.CallContext(ctx, &tx, "eth_getTransactionByHash", params)
		if err != nil {
			return "", err
		}
		txBytes, err := json.Marshal(tx)
		if err != nil {
			return "", err
		}
		return string(txBytes), nil
	case "getNonce":
		nonce, err := h.client.NonceAt(ctx, common.HexToAddress(params), nil)
		if err != nil {
			return "", fmt.Errorf("failed to NonceAt, err=%v", err)
		}
		return strconv.FormatUint(nonce, 10), nil
	case "getPendingNonce":
		nonce, err := h.client.PendingNonceAt(ctx, common.HexToAddress(params))
		if err != nil {
			return "", fmt.Errorf("failed to PendingNonceAt, err=%v", err)
		}
		return strconv.FormatUint(nonce, 10), nil
	case "getGasPrice":
		gasPrice, err := h.client.SuggestGasPrice(ctx)
		if err != nil {
			return "", err
		}
		return gasPrice.String(), nil
	case "getGasBaseFee":
		header, err := h.client.HeaderByNumber(ctx, nil)
		if err != nil {
			return "", fmt.Errorf("failed to HeaderByNumber, err=%v", err)
		}
		if header.BaseFee == nil {
			return "0", nil
		}
		return header.BaseFee.String(), nil
	case "getGasTipCap":
		gasTipCap, err := h.client.SuggestGasTipCap(ctx)
		if err != nil {
			return "", err
		}
		return gasTipCap.String(), nil
	case "estimateGas":
		tmp := strings.Split(params, ":")
		if len(tmp) != 5 {
			return "", fmt.Errorf("invalid params")
		}
		from := tmp[0]
		to := tmp[1]
		value := tmp[2]
		payload := tmp[3]
		gasPrice := tmp[4]

		fromAddr := common.HexToAddress(from)
		toAddr := common.HexToAddress(to)
		gasPriceBig, ok := new(big.Int).SetString(gasPrice, 10)
		if !ok {
			return "", fmt.Errorf("failed to get gasPrice")
		}
		valueBig, ok := new(big.Int).SetString(value, 10)
		if !ok {
			return "", fmt.Errorf("failed to get value")
		}
		data, err := hex.DecodeString(strings.TrimPrefix(payload, "0x"))
		if err != nil {
			return "", err
		}
		gas, err := h.client.EstimateGas(ctx, ethereum.CallMsg{
			From:     fromAddr,
			To:       &toAddr,
			GasPrice: gasPriceBig,
			Value:    valueBig,
			Data:     data,
		})
		if err != nil {
			return "", err
		} else if gas != 21000 {
			gas = gas * 3 / 2
		}
		return strconv.FormatUint(gas, 10), nil
	case "getTransactionLog":
		receipt := &RpcReceipt{}
		err := h.rpc.CallContext(ctx, &receipt, "eth_getTransactionReceipt", params)
		if err != nil {
			return "", fmt.Errorf("failed to eth_getTransactionReceipt, err=%v", err)
		} else if receipt == nil || receipt.BlockNumber == "" {
			return "", fmt.Errorf("receipt == nil or empty blocknumber")
		}
		var logs []chainrpc.EvmLog
		for _, v := range receipt.Logs {
			log := chainrpc.EvmLog{
				Address: v.Address,
				Topics:  strings.Join(v.Topics, ","),
				Data:    v.Data,
			}
			logs = append(logs, log)
		}
		if len(logs) == 0 {
			return "", nil
		}
		var res string
		for _, v := range logs {
			res += "_" + v.String()
		}
		return res[1:], nil
	case "eth_getLogs":
		tmp := strings.Split(params, ":")
		if len(tmp) < 4 {
			return "", fmt.Errorf("invalid params")
		}
		from, ok := big.NewInt(0).SetString(tmp[0], 10)
		if !ok {
			return "", fmt.Errorf("from block number is invalid")
		}
		to, ok := big.NewInt(0).SetString(tmp[1], 10)
		if !ok {
			return "", fmt.Errorf("to block number is invalid")
		}
		q := ethereum.FilterQuery{
			FromBlock: from,
			ToBlock:   to,
			Addresses: []common.Address{common.HexToAddress(tmp[2])},
			Topics:    [][]common.Hash{{common.HexToHash(tmp[3])}},
		}

		if len(tmp) > 4 {
			for i := 0; i < len(tmp)-4; i++ {
				if tmp[4+i] == "" {
					q.Topics = append(q.Topics, nil)
				} else {
					q.Topics = append(q.Topics, []common.Hash{common.HexToHash(tmp[4+i])})
				}
			}
		}

		getLogs, err := h.client.FilterLogs(ctx, q)
		if err != nil {
			return "", fmt.Errorf("failed to eth_getTransactionReceipt, err=%v", err)
		}
		if len(getLogs) == 0 {
			return "", nil
		}
		var logs []chainrpc.EvmLog
		for _, v := range getLogs {
			topics := ""
			for _, v := range v.Topics {
				topics += "," + v.String()
			}
			log := chainrpc.EvmLog{
				Address:     v.Address.Hex(),
				Topics:      topics[1:],
				Data:        common.Bytes2Hex(v.Data),
				BlockNumber: v.BlockNumber,
				TxHash:      v.TxHash.Hex(),
			}
			logs = append(logs, log)
		}
		if len(logs) == 0 {
			return "", nil
		}
		var res string
		for _, v := range logs {
			res += "_" + v.String()
		}
		return res[1:], nil
	case "eth_getCode":
		code, err := h.client.CodeAt(ctx, common.HexToAddress(params), nil)
		if err != nil {
			return "", fmt.Errorf("failed to eth_getCode, err=%v", err)
		}
		return common.Bytes2Hex(code), nil
	case "getBlockByNumber":
		height, err := strconv.ParseUint(params, 10, 64)
		if err != nil {
			return "", fmt.Errorf("failed to ParseUint for height, err=%v", err)
		}
		var raw json.RawMessage
		err = h.rpc.CallContext(ctx, &raw, "eth_getBlockByNumber",
			fmt.Sprintf("%#x", height), true)
		if err != nil {
			return "", fmt.Errorf("failed to eth_getBlockByNumber, height=%d, err=%v", height, err)
		} else if len(raw) == 0 {
			return "", fmt.Errorf("failed to eth_getBlockByNumber, len(raw) == 0")
		}
		var head RpcBlockHeader
		if err = json.Unmarshal(raw, &head); err != nil {
			return "", fmt.Errorf("failed to Unmarshal, err=%v", err)
		}
		var body RpcBlockBody
		if err = json.Unmarshal(raw, &body); err != nil {
			return "", fmt.Errorf("failed to Unmarshal, err=%v", err)
		}
		block := RpcBlock{
			RpcBlockHeader: head,
			RpcBlockBody:   body,
		}
		blockBytes, err := json.Marshal(block)
		if err != nil {
			return "", fmt.Errorf("failed to Marshal, err=%v", err)
		}
		return string(blockBytes), nil
	case "IsTxExisted":
		tx := &RpcTransaction{}
		err := h.rpc.CallContext(ctx, tx, "eth_getTransactionByHash", params)
		if err != nil {
			return "", fmt.Errorf("failed to eth_getTransactionByHash, err=%v", err)
		}
		if tx.Hash != "" {
			return "true", nil
		}
		return "false", nil
	case "getOpL1Fee":
		contract, err := abi.JSON(strings.NewReader(opGasPriceOracleABI))
		if err != nil {
			return "", fmt.Errorf("failed to parse ABI: %v", err)
		}
		if params == "" {
			return "", fmt.Errorf("getOpL1Fee requires the serialized transaction as params")
		}
		txBytes, err := hex.DecodeString(strings.TrimPrefix(params, "0x"))
		if err != nil {
			return "", fmt.Errorf("invalid transaction hex: %v", err)
		}
		input, err := contract.Pack("getL1Fee", txBytes)
		if err != nil {
			return "", fmt.Errorf("failed to pack input: %v", err)
		}

		to := common.HexToAddress(opGasPriceOracleAddress)
		msg := ethereum.CallMsg{
			To:   &to,
			Data: input,
		}
		output, err := h.client.CallContract(ctx, msg, nil)
		if err != nil {
			return "", fmt.Errorf("failed to call contract: %v", err)
		}
		result, err := contract.Unpack("getL1Fee", output)
		if err != nil {
			return "", fmt.Errorf("failed to unpack output: %v", err)
		}
		if len(result) == 0 {
			return "", fmt.Errorf("empty result")
		}
		l1Fee, ok := result[0].(*big.Int)
		if !ok {
			return "", fmt.Errorf("failed to convert result to big.Int")
		}
		return l1Fee.String(), nil
	case "getTokenDecimals":
		if params == signing.MagicContactAddressForNative {
			return "18", nil
		}
		var out interface{}
		var err error
		out, err = h.callContractErc20(ctx, params, "decimals", nil, 0)
		if err != nil {
			return "", fmt.Errorf("failed to get decimals, contractAddr=%s, err=%v", params, err)
		}
		decimals, ok := out.(string)
		if !ok {
			return "", fmt.Errorf("out is not string")
		}
		return decimals, nil

	}
	return "", fmt.Errorf("unsupported function")
}

// unexported
func (h *Handler) callContractErc20(ctx context.Context, contractAddress, function string, params []byte, blockNumber uint64) (string, error) {
	var msg ethereum.CallMsg
	var to common.Address
	var data []byte
	var err error
	var resBytes []byte

	to = common.HexToAddress(contractAddress)
	msg.To = &to

	jsonABI, err := abi.JSON(strings.NewReader(erc20.ABIERC20))
	if err != nil {
		return "", err
	}
	hexData, err := erc20.PackPayloadForErc20(function, params)
	if err != nil {
		return "", err
	}
	data, _ = hex.DecodeString(hexData)
	msg.Data = data

	if blockNumber == 0 {
		resBytes, err = h.client.CallContract(ctx, msg, nil)
		if err != nil {
			return "", fmt.Errorf("failed to callcontract, err=%v", err)
		}
	} else {
		blockNumberBig := new(big.Int).SetUint64(blockNumber)
		resBytes, err = h.client.CallContract(ctx, msg, blockNumberBig)
		if err != nil {
			return "", fmt.Errorf("failed to callcontract, err=%v", err)
		}
	}
	out, err := jsonABI.Unpack(function, resBytes)
	if err != nil {
		return "", err
	}

	switch function {
	case "name", "symbol":
		if len(out) != 1 {
			return "", fmt.Errorf("invalid length of result")
		}
		r, ok := out[0].(string)
		if !ok {
			return "", fmt.Errorf("invalid result type")
		}
		return r, nil
	case "decimals", "totalSupply", "balanceOf", "allowance":
		if len(out) != 1 {
			return "", fmt.Errorf("invalid length of result")
		}
		r, ok := out[0].(*big.Int)
		if ok {
			return r.String(), nil
		}
		r1, ok1 := out[0].(uint8)
		if ok1 {
			return strconv.FormatUint(uint64(r1), 10), nil
		}
		return "", fmt.Errorf("invalid type")
	default:
		return "", fmt.Errorf("unsupported function")
	}
}

func (h *Handler) isContractAddress(ctx context.Context, address string) (bool, error) {
	addr := common.HexToAddress(address)
	bytecode, err := h.client.CodeAt(ctx, addr, nil)
	if err != nil {
		return false, err
	}
	return len(bytecode) > 0, nil
}

func (h *Handler) getTxTransferForEVMFromNative(tx *RpcTransaction) ([]*chainrpc.Transfer, []*chainrpc.BalanceChange, error) {
	valueBig, ok := new(big.Int).SetString(strings.TrimPrefix(tx.Value, "0x"), 16)
	if !ok {
		return nil, nil, fmt.Errorf("failed to parse tx value %q", tx.Value)
	}
	memoBytes, err := hex.DecodeString(strings.TrimPrefix(tx.Payload, "0x"))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to TrimSuffix, err=%v", err)
	}

	var transfers []*chainrpc.Transfer
	transfers = append(transfers, &chainrpc.Transfer{
		Sender:          tx.From,
		Recipient:       tx.To,
		Amount:          valueBig.String(),
		ContractAddress: signing.MagicContactAddressForNative,
		Memo:            string(memoBytes),
	})

	var balanceChange []*chainrpc.BalanceChange
	balanceChangeFrom := &chainrpc.BalanceChange{
		Address:         tx.From,
		ContractAddress: signing.MagicContactAddressForNative,
		Change:          new(big.Int).Neg(valueBig).String(),
	}
	balanceChangeTo := &chainrpc.BalanceChange{
		Address:         tx.To,
		ContractAddress: signing.MagicContactAddressForNative,
		Change:          valueBig.String(),
	}
	balanceChange = append(balanceChange, balanceChangeFrom)
	balanceChange = append(balanceChange, balanceChangeTo)
	return transfers, balanceChange, nil
}

func (h *Handler) getTxTransferForEVMFromContract(tx *RpcTransaction, receipt *RpcReceipt, internalTxs []*RpcInternalTx) ([]*chainrpc.Transfer, []*chainrpc.BalanceChange, error) {
	transferList := make([]*chainrpc.Transfer, 0)
	balanceMap := make(map[string]map[string]*big.Int)
	transferEventSignature := ERC20TransferEvent

	for _, log := range receipt.Logs {
		if len(log.Topics) < 3 {
			continue
		}

		if log.Topics[0] != transferEventSignature {
			continue
		}

		sender := "0x" + log.Topics[1][26:]
		recipient := "0x" + log.Topics[2][26:]
		contractAddr := log.Address

		if log.Data == "" || log.Data == "0x" {
			continue
		}

		amountBig, ok := new(big.Int).SetString(strings.TrimPrefix(log.Data, "0x"), 16)
		if !ok || amountBig.Cmp(big.NewInt(0)) == 0 {
			continue
		}

		transferList = append(transferList, &chainrpc.Transfer{
			Sender:          sender,
			Recipient:       recipient,
			Amount:          amountBig.String(),
			ContractAddress: contractAddr,
		})

		updateBalanceMap(balanceMap, sender, contractAddr, new(big.Int).Neg(amountBig))
		updateBalanceMap(balanceMap, recipient, contractAddr, amountBig)
	}

	rootTraced := false
	failedFrames := make(map[string]bool)
	for _, itx := range internalTxs {
		if itx.TraceAddress == nil {
			continue
		}
		if len(itx.TraceAddress) == 0 {
			rootTraced = true
		}
		if itx.Error != "" {
			failedFrames[fmt.Sprint(itx.TraceAddress)] = true
		}
	}
	inFailedFrame := func(itx *RpcInternalTx) bool {
		if itx.Error != "" {
			return true
		}
		if itx.TraceAddress == nil {
			return false
		}
		for i := 0; i <= len(itx.TraceAddress); i++ {
			if failedFrames[fmt.Sprint(itx.TraceAddress[:i])] {
				return true
			}
		}
		return false
	}

	if !rootTraced && tx.Value != "" && tx.Value != "0x" && tx.Value != "0x0" {
		valueBig, ok := new(big.Int).SetString(strings.TrimPrefix(tx.Value, "0x"), 16)
		if ok && valueBig.Cmp(big.NewInt(0)) > 0 {
			transferList = append(transferList, &chainrpc.Transfer{
				Sender:          tx.From,
				Recipient:       tx.To,
				Amount:          valueBig.String(),
				ContractAddress: signing.MagicContactAddressForNative,
			})

			updateBalanceMap(balanceMap, tx.From, signing.MagicContactAddressForNative, new(big.Int).Neg(valueBig))
			updateBalanceMap(balanceMap, tx.To, signing.MagicContactAddressForNative, valueBig)
		}
	}

	if len(internalTxs) > 0 {
		internalNativeMap := make(map[string]map[string]*big.Int) // from -> (to -> amount)

		for _, itx := range internalTxs {
			if itx.Action == nil || inFailedFrame(itx) {
				continue
			}

			if itx.Action.CallType != "call" {
				continue
			}

			if itx.Action.Value == "" || itx.Action.Value == "0x" || itx.Action.Value == "0x0" {
				continue
			}

			valueBig, ok := new(big.Int).SetString(strings.TrimPrefix(itx.Action.Value, "0x"), 16)
			if !ok || valueBig.Cmp(big.NewInt(0)) == 0 {
				continue
			}

			from := itx.Action.From
			to := itx.Action.To

			if from == "" || to == "" {
				continue
			}

			if internalNativeMap[from] == nil {
				internalNativeMap[from] = make(map[string]*big.Int)
			}
			if internalNativeMap[from][to] == nil {
				internalNativeMap[from][to] = big.NewInt(0)
			}
			internalNativeMap[from][to].Add(internalNativeMap[from][to], valueBig)
		}

		for from, tos := range internalNativeMap {
			for to, amount := range tos {
				if amount.Cmp(big.NewInt(0)) > 0 {
					transferList = append(transferList, &chainrpc.Transfer{
						Sender:          from,
						Recipient:       to,
						Amount:          amount.String(),
						ContractAddress: signing.MagicContactAddressForNative,
					})

					updateBalanceMap(balanceMap, from, signing.MagicContactAddressForNative, new(big.Int).Neg(amount))
					updateBalanceMap(balanceMap, to, signing.MagicContactAddressForNative, amount)
				}
			}
		}
	}

	if len(transferList) == 0 {
		return nil, nil, fmt.Errorf("no transfers found in contract call")
	}

	balanceChanges := make([]*chainrpc.BalanceChange, 0)
	for addr, contracts := range balanceMap {
		for contractAddr, change := range contracts {
			if change.Cmp(big.NewInt(0)) != 0 {
				balanceChanges = append(balanceChanges, &chainrpc.BalanceChange{
					Address:         addr,
					ContractAddress: contractAddr,
					Change:          change.String(),
				})
			}
		}
	}

	return transferList, balanceChanges, nil
}

func updateBalanceMap(balanceMap map[string]map[string]*big.Int, addr, contractAddr string, change *big.Int) {
	if balanceMap[addr] == nil {
		balanceMap[addr] = make(map[string]*big.Int)
	}
	if balanceMap[addr][contractAddr] == nil {
		balanceMap[addr][contractAddr] = big.NewInt(0)
	}
	balanceMap[addr][contractAddr].Add(balanceMap[addr][contractAddr], change)
}
