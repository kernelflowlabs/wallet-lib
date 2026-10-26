package kaspa

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kernelflowlabs/wallet-lib/common/httpc"
	chainrpc "github.com/kernelflowlabs/wallet-lib/rpc"
	"github.com/kernelflowlabs/wallet-lib/signing"

	"github.com/kaspanet/kaspad/app/appmessage"
	"github.com/kaspanet/kaspad/domain/consensus/utils/consensushashing"
	"github.com/kaspanet/kaspad/domain/dagconfig"
	"github.com/kaspanet/kaspad/infrastructure/network/rpcclient"
)

var _ chainrpc.BasicChainHandler = (*Handler)(nil)

type Handler struct {
	client     *rpcclient.RPCClient
	scanApi    *httpc.Request
	kasplexApi *httpc.Request
}

func NewHandler(url string) (*Handler, error) {
	h := &Handler{
		scanApi:    httpc.NewRequest("https://api.kaspa.org", nil),
		kasplexApi: httpc.NewRequest("https://api.kasplex.org/v1", nil),
	}
	client, err := rpcclient.NewRPCClient(url)
	if err != nil {
		return nil, fmt.Errorf("failed to NewRPCClient, err=%v", err)
	}
	client.SetTimeout(10 * time.Second)
	h.client = client
	return h, nil
}

func (h *Handler) GetHeight(ctx context.Context) (string, error) {
	res, err := h.client.GetBlockCount()
	if err != nil {
		return "", fmt.Errorf("failed to GetBlockCount, err=%v", err)
	} else if res.Error != nil {
		return "", fmt.Errorf("failed to GetBlockCount, errMSg=%v", res.Error.Error())
	}
	return strconv.FormatUint(res.BlockCount, 10), nil
}

func (h *Handler) GetBalance(ctx context.Context, address, contractAddress, blockNumber string) (string, error) {
	if contractAddress == signing.MagicContactAddressForNative ||
		contractAddress == "KAS" {
		getUTXOsByAddressesResponse, err := h.client.GetUTXOsByAddresses([]string{address})
		if err != nil {
			return "", err
		}
		dagInfo, err := h.client.GetBlockDAGInfo()
		if err != nil {
			return "", err
		}
		var balance uint64
		for _, entry := range getUTXOsByAddressesResponse.Entries {
			if !isUTXOSpendable(entry, dagInfo.VirtualDAAScore) {
				continue
			}
			balance += entry.UTXOEntry.Amount
		}
		return strconv.FormatUint(balance, 10), nil
	}

	path := "krc20/address/" + address + "/token/" + contractAddress
	res := &KasplexGetBalanceRes{}
	err := h.kasplexApi.Get(ctx, res, path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to GetBalance via krc20Api2, err=%v", err)
	} else if res.Message != "successful" {
		return "", fmt.Errorf("failed to GetBalance via krc20Api2, errMSg=%v", res.Message)
	}

	return res.Result[0].Balance, nil
}

func (h *Handler) CheckTx(ctx context.Context, hash string) (*chainrpc.TxResult, error) {
	tmp := strings.Split(hash, "_")
	var ticker string
	if len(tmp) == 1 {
		hash = tmp[0]
	} else if len(tmp) == 2 {
		hash = tmp[0]
		ticker = tmp[1]
	} else {
		return nil, fmt.Errorf("invalid hash")
	}

	r := &chainrpc.TxResult{}
	re := regexp.MustCompile(`^[0-9a-f]{64}$`)
	matched := re.MatchString(hash)
	if !matched {
		r.Status = signing.TxStatusFailed
		r.ErrMsg = "invalid hash"
		return r, nil
	}

	if ticker == "" {
		parentBlueScore, err := h.client.GetVirtualSelectedParentBlueScore()
		if err != nil {
			return nil, fmt.Errorf("failed to GetVirtualSelectedParentBlueScore, err=%v", err)
		} else if parentBlueScore.Error != nil {
			return nil, fmt.Errorf("failed to GetVirtualSelectedParentBlueScore, errMsg=%v",
				parentBlueScore.Error.Error())
		}
		rawTx, isPending, err := getNativeTx(ctx, h.scanApi, hash)
		if err != nil {
			return nil, fmt.Errorf("failed to getNativeTx, err=%v", err)
		} else if isPending || (rawTx != nil && rawTx.BlockTime > 0 && !rawTx.IsAccepted) {
			r.Status = signing.TxStatusPending
		} else if rawTx != nil && rawTx.BlockTime > 0 && rawTx.IsAccepted {
			r.Status = signing.TxStatusSucceeded
		}
		return r, nil
	}

	path := "krc20/op/" + hash
	res := &KasplexGetTransactionRes{}
	err := h.kasplexApi.Get(ctx, res, path, nil)
	if err != nil {
		r.Status = signing.TxStatusPending
	} else if res.Message != "successful" {
		r.Status = signing.TxStatusFailed
		r.ErrMsg = res.Message
		if strings.Contains(res.Message, "op not found") {
			r.Status = signing.TxStatusPending
		} else {
			return r, fmt.Errorf("not successful, msg=%s", res.Message)
		}
	} else if res.Result[0].TxAccept != "1" || res.Result[0].OpAccept != "1" {
		r.Status = signing.TxStatusFailed
		r.ErrMsg = fmt.Sprintf("TxAccept or OpAccept !=1, opError=%s", res.Result[0].OpError)
		return r, nil
	} else if res.Result[0].From != "" && res.Result[0].To != "" && res.Result[0].Tick != "" {
		if res.Result[0].Amt == "" && (res.Result[0].From == res.Result[0].To) {
			r.Status = signing.TxStatusSucceeded
		} else if res.Result[0].Amt != "" {
			r.Status = signing.TxStatusSucceeded
		}
	} else {
		r.Status = signing.TxStatusPending
	}

	return r, nil
}

func (h *Handler) SendTx(ctx context.Context, signedHex string) (string, error) {
	signedTxBytes, err := hex.DecodeString(signedHex)
	if err != nil {
		return "", fmt.Errorf("failed to DecodeString signedHex, err=%v", err)
	}
	stx := &signedTx{}
	err = json.Unmarshal(signedTxBytes, stx)
	if err != nil {
		return "", fmt.Errorf("failed to Unmarshal signedHex, err=%v", err)
	}
	rpcTransactionBytes, err := hex.DecodeString(stx.SignedHex)
	if err != nil {
		return "", fmt.Errorf("failed to DecodeString, err=%v", err)
	}
	transaction := &appmessage.RPCTransaction{}
	err = json.Unmarshal(rpcTransactionBytes, transaction)
	if err != nil {
		return "", fmt.Errorf("failed to Unmarshal, err=%v", err)
	}
	submitTransactionResponse, err := h.client.SubmitTransaction(transaction, stx.TxHash, false)
	if err != nil {
		if strings.Contains(err.Error(), "Rejected transaction") &&
			strings.Contains(err.Error(), "already spent by transaction") &&
			strings.Contains(err.Error(), "in the mempool") ||
			strings.Contains(err.Error(), "Rejected transaction") &&
				strings.Contains(err.Error(), "is an orphan where orphan is disallowed") {
			return "", fmt.Errorf("SEND_RETRY_ONCE")
		}
		return "", fmt.Errorf("error submitting transaction err=%v", err)
	}
	return submitTransactionResponse.TransactionID, nil
}

func (h *Handler) CallContract(ctx context.Context, contractAddress, payload, blockNumber string) ([]byte, error) {
	return nil, fmt.Errorf("CallContract not supported")
}

func (h *Handler) InquireChain(ctx context.Context, instruction, params string) (string, error) {
	switch instruction {
	case "estimateFee":
	case "getUtxo":
		res := &signing.UtxoList{}
		getUTXOsByAddressesResponse, err := h.client.GetUTXOsByAddresses([]string{params})
		if err != nil {
			return "", fmt.Errorf("failed to GetUTXOsByAddresses, err=%v", err)
		}
		dagInfo, err := h.client.GetBlockDAGInfo()
		if err != nil {
			return "", fmt.Errorf("failed to GetBlockDAGInfo, err=%v", err)
		}
		spendableUTXOs := make(map[appmessage.RPCOutpoint]*appmessage.RPCUTXOEntry, 0)
		for _, entry := range getUTXOsByAddressesResponse.Entries {
			if !isUTXOSpendable(entry, dagInfo.VirtualDAAScore) {
				continue
			}
			spendableUTXOs[*entry.Outpoint] = entry.UTXOEntry
		}
		for outpoint, utxo := range spendableUTXOs {
			outpointCopy := outpoint
			utxoInfo := &signing.UtxoInfo{
				Hash:          outpointCopy.TransactionID,
				Index:         strconv.FormatUint(uint64(outpointCopy.Index), 10),
				Script:        utxo.ScriptPublicKey.Script,
				Value:         strconv.FormatUint(utxo.Amount, 10),
				Version:       strconv.FormatUint(uint64(utxo.ScriptPublicKey.Version), 10),
				IsCoinbase:    strconv.FormatBool(utxo.IsCoinbase),
				BlockDAAScore: strconv.FormatUint(utxo.BlockDAAScore, 10),
			}
			res.List = append(res.List, utxoInfo)
		}
		resBytes, err := json.Marshal(res)
		if err != nil {
			return "", fmt.Errorf("failed to Marshal for res, err=%v", err)
		}
		return string(resBytes), nil
	case "isTxInMemPool":
		mempoolEntries, err := h.client.GetMempoolEntries(true, false)
		if err != nil {
			return "", fmt.Errorf("failed to GetMempoolEntries, err=%v", err)
		}
		for _, entry := range mempoolEntries.Entries {
			dtx, err := appmessage.RPCTransactionToDomainTransaction(entry.Transaction)
			if err != nil {
				return "", fmt.Errorf("failed to RPCTransactionToDomainTransaction, err=%v", err)
			}
			txid := consensushashing.TransactionID(dtx).String()
			if txid == params {
				return "true", nil
			}
		}
		return "false", nil
	case "getMempoolEntriesByAddresses":
		_, err := h.client.GetMempoolEntriesByAddresses([]string{params},
			false, false)
		if err != nil {
			return "", fmt.Errorf("failed to GetMempoolEntries, err=%v", err)
		}
		return "false", nil
	case "submitTransaction":
		signedTxBytes, err := hex.DecodeString(params)
		if err != nil {
			return "", fmt.Errorf("failed to DecodeString signedHex, err=%v", err)
		}
		stx := &signedTx{}
		err = json.Unmarshal(signedTxBytes, stx)
		if err != nil {
			return "", fmt.Errorf("failed to Unmarshal signedHex, err=%v", err)
		}
		rpcTransactionBytes, err := hex.DecodeString(stx.SignedHex)
		if err != nil {
			return "", fmt.Errorf("failed to DecodeString, err=%v", err)
		}
		transaction := &appmessage.RPCTransaction{}
		err = json.Unmarshal(rpcTransactionBytes, transaction)
		if err != nil {
			return "", fmt.Errorf("failed to Unmarshal, err=%v", err)
		}
		submitTransactionResponse, err := h.client.SubmitTransaction(transaction, stx.TxHash, false)
		if err != nil {
			return "", fmt.Errorf("error submitting transaction err=%v", err)
		}
		return submitTransactionResponse.TransactionID, nil
	case "getVirtualSelectedParentBlueScore":
		parentBlueScore, err := h.client.GetVirtualSelectedParentBlueScore()
		if err != nil {
			return "", fmt.Errorf("failed to GetVirtualSelectedParentBlueScore, err=%v", err)
		}
		return strconv.FormatUint(parentBlueScore.BlueScore, 10), nil
	case "getPointHash":
		dagInfo, err := h.client.GetBlockDAGInfo()
		if err != nil {
			return "", fmt.Errorf("failed to GetBlockDAGInfo, err=%v", err)
		}
		return dagInfo.PruningPointHash, nil
	case "getAddedBlockHashes":
		block, err := h.client.GetVirtualSelectedParentChainFromBlock(params, false)
		if err != nil {
			return "", fmt.Errorf("failed to GetVirtualSelectedParentChainFromBlock, err=%v", err)
		}
		var added []string
		for _, hash := range block.AddedChainBlockHashes {
			added = append(added, hash)
		}
		addedBytes, err := json.Marshal(added)
		if err != nil {
			return "", fmt.Errorf("failed to Marshal added, err=%v", err)
		}
		return string(addedBytes), nil
	case "getBlock":
		block, err := h.client.GetBlock(params, true)
		if err != nil {
			return "", fmt.Errorf("failed to GetBlock, err=%v", err)
		}
		blockBytes, err := json.Marshal(block)
		if err != nil {
			return "", fmt.Errorf("failed to Marshal block, err=%v", err)
		}
		return string(blockBytes), nil
	case "getTokenDecimals":
		if params == signing.MagicContactAddressForNative {
			return "8", nil
		}
	}
	return "", fmt.Errorf("unsupported function %s", instruction)
}

// Verify UTXO is spendable (check if a minimum of 10 confirmations have been processed since UTXO creation)
func isUTXOSpendable(entry *appmessage.UTXOsByAddressesEntry, virtualSelectedParentBlueScore uint64) bool {
	blockDAAScore := entry.UTXOEntry.BlockDAAScore
	if !entry.UTXOEntry.IsCoinbase {
		const minConfirmations = 10
		return blockDAAScore+minConfirmations < virtualSelectedParentBlueScore
	}
	coinbaseMaturity := dagconfig.MainnetParams.BlockCoinbaseMaturity
	return blockDAAScore+coinbaseMaturity < virtualSelectedParentBlueScore
}

func getNativeTx(ctx context.Context, api *httpc.Request, hash string) (*KasScanGetTransactionRes, bool, error) {
	out := &KasScanGetTransactionRes{}
	err := api.Get(ctx, out, "transactions/"+hash, nil)
	if httpc.StatusCode(err) == http.StatusNotFound {
		return nil, true, nil
	} else if err != nil {
		return nil, false, fmt.Errorf("failed to get transactions, hash=%s, err=%w", hash, err)
	} else if out.Detail != "" {
		if strings.Contains(out.Detail, "Transaction not found") {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("failed to get transactions, hash=%s, detail=%s",
			hash, out.Detail)
	} else if out.Message != "" {
		return nil, false, fmt.Errorf("failed to get transactions, hash=%s, message=%s",
			hash, out.Message)
	}
	return out, false, nil
}

type signedTx struct {
	SignedHex string
	TxHash    string
}
