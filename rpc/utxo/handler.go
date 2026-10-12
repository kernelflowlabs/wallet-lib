package utxo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kernelflowlabs/wallet-sdk/common/httpc"
	chainrpc "github.com/kernelflowlabs/wallet-sdk/rpc"
	"github.com/kernelflowlabs/wallet-sdk/signing"
	walletutxo "github.com/kernelflowlabs/wallet-sdk/signing/utxo"

	"github.com/shopspring/decimal"
)

var _ chainrpc.BasicChainHandler = (*Handler)(nil)

type Handler struct {
	rpc              *httpc.Request
	scanApi          *httpc.Request
	dogeApi          *httpc.Request
	network          string
	blockcypherToken string
	bcMu             sync.Mutex
	bcLastReq        time.Time
}

func NewHandler(url, network, blockcypherToken string) (*Handler, error) {
	if network == walletutxo.NetworkEnumForSYS {
		h := &Handler{blockcypherToken: blockcypherToken, dogeApi: newBlockcypherRequest()}
		rpc := httpc.NewRequest(url, map[string]string{
			"content-type": "text/plain",
		})
		h.rpc = rpc
		h.network = network
		return h, nil
	}

	parts := strings.SplitN(url, ";", 2)
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return nil, fmt.Errorf("invalid url: empty rpc part")
	}

	rpcPart := strings.TrimSpace(parts[0])
	scanApiUrl := ""
	if len(parts) == 2 {
		scanApiUrl = strings.TrimSpace(parts[1])
	}

	var (
		rpcBase string
		rpcUser string
		rpcPass string
	)

	if strings.Contains(rpcPart, "&") {
		tmp := strings.SplitN(rpcPart, "&", 2)
		rpcBase = strings.TrimSpace(tmp[0])
		authStr := strings.TrimSpace(tmp[1])

		up := strings.SplitN(authStr, "@", 2)
		if len(up) != 2 {
			return nil, fmt.Errorf("rpc authorization string error")
		}
		rpcUser = up[0]
		rpcPass = up[1]
	} else {
		rpcBase = rpcPart
	}

	h := &Handler{blockcypherToken: blockcypherToken, dogeApi: newBlockcypherRequest()}

	rpcHeaders := map[string]string{
		"content-type": "text/plain",
	}

	if rpcUser != "" && rpcPass != "" {
		authorization := base64.StdEncoding.EncodeToString([]byte(rpcUser + ":" + rpcPass))
		rpcHeaders["Authorization"] = "Basic " + authorization
	}

	rpc := httpc.NewRequest(rpcBase, rpcHeaders)
	h.rpc = rpc

	if scanApiUrl != "" {
		scanHeaders := map[string]string{}
		h.scanApi = httpc.NewRequest(scanApiUrl, scanHeaders)
	}

	h.network = network
	return h, nil
}

func newBlockcypherRequest() *httpc.Request {
	return httpc.NewRequest("https://api.blockcypher.com", map[string]string{
		"User-Agent": "curl/7.81.0",
	})
}

func (h *Handler) GetHeight(ctx context.Context) (string, error) {
	req := &BaseRequest{
		JsonRPC: "1.0",
		ID:      "1",
		Method:  "getblockcount",
	}
	out := &GetBlockCountRes{}
	err := h.rpc.Post(ctx, out, "", req)
	if err != nil {
		return "", fmt.Errorf("failed to GetHeight, err=%v", err)
	} else if out.Error.Code != 0 {
		return "", fmt.Errorf("failed to GetHeight, errMsg=%v", out.Error.Message)
	} else if out.Result == 0 {
		return "", fmt.Errorf("got zero")
	}

	return strconv.FormatUint(out.Result, 10), nil
}

func (h *Handler) GetBalance(ctx context.Context, address, contractAddress, blockNumber string) (string, error) {
	if contractAddress != signing.MagicContactAddressForNative {
		return "", fmt.Errorf("invalid contractAddress")
	}
	switch h.network {
	case walletutxo.NetworkEnumForBTC, walletutxo.NetworkEnumForBTCP2TR:
		return h.getBalanceForBTC(ctx, address)
	case walletutxo.NetworkEnumForLTC:
		return h.getBalanceForLTC(ctx, address)
	case walletutxo.NetworkEnumForDOGE:
		return h.getBalanceForDOGE(ctx, address)
	}
	return "", fmt.Errorf("failed to get balance, invalid network")
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

	//confirmation := getConfirmation(h.network)
	confirmed, err := chainrpc.Confirmations(ctx, txResult.Height, confirmation, h.GetHeight)
	if err != nil {
		return nil, fmt.Errorf("failed to check confirmations, err=%w", err)
	}
	if confirmed < confirmation {
		result.ErrMsg = fmt.Sprintf("tx succeeded.But current confirmation number %d hasn't meet "+
			"expected number %d", confirmed, confirmation)
		return result, nil
	}

	switch h.network {
	case walletutxo.NetworkEnumForBTC, walletutxo.NetworkEnumForBTCP2TR:
		transfers, balanceChange, err := h.getTxTransferForBTC(ctx, hash)
		if err != nil {
			result.ErrMsg = err.Error()
			result.Rejected = true
			return result, nil
		}
		result.Transfers = transfers
		result.BalanceChange = balanceChange
		return result, nil
	case walletutxo.NetworkEnumForLTC:
	case walletutxo.NetworkEnumForDOGE:
	case walletutxo.NetworkEnumForSYS:
	}
	return nil, fmt.Errorf("invalid network")
}

func (h *Handler) SendTx(ctx context.Context, signedHex string) (string, error) {
	req := &BaseRequest{
		JsonRPC: "1.0",
		ID:      "1",
		Method:  "sendrawtransaction",
		Params: []string{
			signedHex,
		},
	}
	out := &SendRawTransactionRes{}
	err := h.rpc.Post(ctx, out, "", req)
	if err != nil {
		return "", fmt.Errorf("failed to sendrawtransaction, err=%v", err)
	} else if out.Error.Code != 0 {
		return "", fmt.Errorf("failed to sendrawtransaction, errMsg=%v", out.Error.Message)
	}
	return out.Result, nil
}

func (h *Handler) CheckTx(ctx context.Context, hash string) (*chainrpc.TxResult, error) {
	switch h.network {
	case walletutxo.NetworkEnumForBTC, walletutxo.NetworkEnumForBTCP2TR:
		return h.getTxStatusForBTC(ctx, hash)
	case walletutxo.NetworkEnumForLTC:
		return h.getTxStatusForLTC(ctx, hash)
	case walletutxo.NetworkEnumForDOGE:
		return h.getTxStatusForDOGE(ctx, hash)
	}
	return nil, fmt.Errorf("failed to get hash status, invalid network")
}

func (h *Handler) CallContract(ctx context.Context, contractAddress, params, blockNumber string) ([]byte, error) {
	return nil, fmt.Errorf("CallContract not supported")
}

func (h *Handler) InquireChain(ctx context.Context, instruction, params string) (string, error) {
	switch instruction {
	case "getUtxo":
		res := &signing.UtxoList{}
		var err error
		switch h.network {
		case walletutxo.NetworkEnumForBTC, walletutxo.NetworkEnumForBTCP2TR:
			res, err = h.getUtxoForBTC(ctx, params)
			if err != nil {
				return "", err
			}
		case walletutxo.NetworkEnumForLTC:
			res, err = h.getUtxoForLTC(ctx, params)
			if err != nil {
				return "", err
			}
		case walletutxo.NetworkEnumForDOGE:
			res, err = h.getUtxoForDOGE(ctx, params)
			if err != nil {
				return "", err
			}
		default:
			return "", fmt.Errorf("failed to get utxo, invalid network")
		}
		resBytes, err := json.Marshal(res)
		if err != nil {
			return "", fmt.Errorf("failed to Marshal for res, err=%v", err)
		}
		return string(resBytes), nil
	case "getByteFee":
		switch h.network {
		case walletutxo.NetworkEnumForBTC, walletutxo.NetworkEnumForBTCP2TR:
			return h.getByteFeeForBTC(ctx)
		case walletutxo.NetworkEnumForLTC:
			return h.getByteFeeForLTC(ctx)
		case walletutxo.NetworkEnumForDOGE:
			return h.getByteFeeForDOGE(ctx)
		default:
			return "", fmt.Errorf("failed to get utxo, invalid network")
		}
	case "getBlockByNumber":
		blockNumber, err := strconv.ParseInt(params, 10, 64)
		if err != nil {
			return "", fmt.Errorf("failed to parse block number, err=%v", err)
		}
		blockHash, err := h.getBlockHashByNumber(ctx, blockNumber)
		if err != nil {
			return "", fmt.Errorf("failed to get block hash by number, err=%v", err)
		}

		block, err := h.getBlockByHash(ctx, blockHash)
		if err != nil {
			return "", fmt.Errorf("failed to get block by hash, err=%v", err)
		}
		return block, nil
	case "getTokenDecimals":
		if params == signing.MagicContactAddressForNative {
			return "8", nil
		}
		return "", fmt.Errorf("unsupported token address")
	}
	return "", fmt.Errorf("unsupported function")
}

// unexported
func (h *Handler) getBlockHashByNumber(ctx context.Context, blockNumber int64) (string, error) {
	req := &BaseRequest{
		JsonRPC: "1.0",
		ID:      "1",
		Method:  "getblockhash",
		Params:  []interface{}{blockNumber},
	}

	out := &GetBlockHashRes{}
	err := h.rpc.Post(ctx, out, "", req)
	if err != nil {
		return "", fmt.Errorf("failed to get block hash, err=%v", err)
	} else if out.Error.Code != 0 {
		return "", fmt.Errorf("failed to get block hash, errMsg=%v", out.Error.Message)
	}

	return out.Result, nil
}
func (h *Handler) getBlockByHash(ctx context.Context, blockHash string) (string, error) {
	req := &BaseRequest{
		JsonRPC: "1.0",
		ID:      "1",
		Method:  "getblock",
		Params:  []interface{}{blockHash, 2},
	}

	out := &GetBlockVerboseRes{}
	err := h.rpc.Post(ctx, out, "", req)
	if err != nil {
		return "", fmt.Errorf("failed to get block, err=%v", err)
	} else if out.Error.Code != 0 {
		return "", fmt.Errorf("failed to get block, errMsg=%v", out.Error.Message)
	}
	blockJSON, err := json.Marshal(out.Result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal block, err=%v", err)
	}

	return string(blockJSON), nil
}
func (h *Handler) getTxTransferForBTC(ctx context.Context, hash string) ([]*chainrpc.Transfer, []*chainrpc.BalanceChange, error) {
	path := "tx/" + hash
	res := &ElectrsTxRes{}
	scanApi, err := h.scan()
	if err != nil {
		return nil, nil, err
	}
	err = scanApi.Get(ctx, res, path, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get tx from Electrs, err=%v", err)
	}

	inputAmounts := make(map[string]*big.Int)
	for _, vin := range res.Vin {
		addr := vin.Prevout.ScriptpubkeyAddress
		if addr == "" {
			continue
		}
		if inputAmounts[addr] == nil {
			inputAmounts[addr] = big.NewInt(0)
		}
		inputAmounts[addr].Add(inputAmounts[addr], big.NewInt(vin.Prevout.Value))
	}

	outputAmounts := make(map[string]*big.Int)
	for _, vout := range res.Vout {
		addr := vout.ScriptpubkeyAddress
		if addr == "" {
			continue
		}
		if outputAmounts[addr] == nil {
			outputAmounts[addr] = big.NewInt(0)
		}
		outputAmounts[addr].Add(outputAmounts[addr], big.NewInt(vout.Value))
	}

	allAddresses := make(map[string]bool)
	for addr := range inputAmounts {
		allAddresses[addr] = true
	}
	for addr := range outputAmounts {
		allAddresses[addr] = true
	}

	balanceChanges := make([]*chainrpc.BalanceChange, 0)
	realSenders := make([]string, 0)
	realReceivers := make(map[string]*big.Int)

	for addr := range allAddresses {
		inputAmt := inputAmounts[addr]
		if inputAmt == nil {
			inputAmt = big.NewInt(0)
		}
		outputAmt := outputAmounts[addr]
		if outputAmt == nil {
			outputAmt = big.NewInt(0)
		}

		change := new(big.Int).Sub(outputAmt, inputAmt)

		if change.Cmp(big.NewInt(0)) != 0 {
			balanceChanges = append(balanceChanges, &chainrpc.BalanceChange{
				Address:         addr,
				ContractAddress: signing.MagicContactAddressForNative,
				Change:          change.String(),
			})

			if change.Cmp(big.NewInt(0)) > 0 {
				realReceivers[addr] = change
			} else {
				realSenders = append(realSenders, addr)
			}
		}
	}

	if len(realSenders) == 0 || len(realReceivers) == 0 {
		return nil, balanceChanges, fmt.Errorf("no valid transfers found")
	}

	primarySender := realSenders[0]

	transfers := make([]*chainrpc.Transfer, 0)
	for recipient, amount := range realReceivers {
		transfers = append(transfers, &chainrpc.Transfer{
			Sender:          primarySender,
			Recipient:       recipient,
			Amount:          amount.String(),
			ContractAddress: signing.MagicContactAddressForNative,
		})
	}

	return transfers, balanceChanges, nil
}

func (h *Handler) getByteFeeForBTC(ctx context.Context) (string, error) {
	return h.getByteFeeFromBlockcypher(ctx, "BTC")
}
func (h *Handler) getBalanceForBTC(ctx context.Context, address string) (string, error) {
	return h.getBalanceFromBlockcypher(ctx, "BTC", address)
}
func (h *Handler) getUtxoForBTC(ctx context.Context, address string) (*signing.UtxoList, error) {
	return h.getUtxoFromBlockcypher(ctx, "BTC", address)
}
func (h *Handler) getTxStatusForBTC(ctx context.Context, hash string) (*chainrpc.TxResult, error) {
	return h.getTxStatusFromBlockcypher(ctx, "BTC", hash)
}

func (h *Handler) getByteFeeForLTC(ctx context.Context) (string, error) {
	return h.getByteFeeFromBlockcypher(ctx, "LTC")
}
func (h *Handler) getBalanceForLTC(ctx context.Context, address string) (string, error) {
	return h.getBalanceFromBlockcypher(ctx, "LTC", address)
}
func (h *Handler) getUtxoForLTC(ctx context.Context, address string) (*signing.UtxoList, error) {
	return h.getUtxoFromBlockcypher(ctx, "LTC", address)
}
func (h *Handler) getTxStatusForLTC(ctx context.Context, hash string) (*chainrpc.TxResult, error) {
	return h.getTxStatusFromBlockcypher(ctx, "LTC", hash)
}

func (h *Handler) getByteFeeForDOGE(ctx context.Context) (string, error) {
	return h.getByteFeeFromBlockcypher(ctx, "DOGE")
}
func (h *Handler) getBalanceForDOGE(ctx context.Context, address string) (string, error) {
	return h.getBalanceFromBlockcypher(ctx, "DOGE", address)
}
func (h *Handler) getUtxoForDOGE(ctx context.Context, address string) (*signing.UtxoList, error) {
	return h.getUtxoFromBlockcypher(ctx, "DOGE", address)
}
func (h *Handler) getTxStatusForDOGE(ctx context.Context, hash string) (*chainrpc.TxResult, error) {
	return h.getTxStatusFromBlockcypher(ctx, "DOGE", hash)
}

func (h *Handler) getByteFeeFromBlockcypher(ctx context.Context, chainName string) (string, error) {
	if err := h.bcLimit(ctx); err != nil {
		return "", err
	}
	out := &BlockcypherFeeRes{}
	path := "v1/" + strings.ToLower(chainName) + "/main"
	scanApi, err := h.blockcypher(chainName)
	if err != nil {
		return "", err
	}
	err = scanApi.Get(ctx, out, path, h.blockcypherQuery())
	if err != nil {
		return "", fmt.Errorf("failed to get byteFee, err=%v", err)
	}
	feePerByte := float64(1)
	if out.MediumFeePerKb != 0 {
		feePerByte = out.MediumFeePerKb / 1000
	} else if out.HighFeePerKb != 0 {
		feePerByte = out.HighFeePerKb / 1000
	} else if out.LowFeePerKb != 0 {
		feePerByte = out.LowFeePerKb / 1000
	}
	return decimal.NewFromFloat(feePerByte).Ceil().String(), nil
}
func (h *Handler) getBalanceFromBlockcypher(ctx context.Context, chainName, address string) (string, error) {
	if err := h.bcLimit(ctx); err != nil {
		return "", err
	}
	path := "v1/" + strings.ToLower(chainName) + "/main/addrs/" + address
	res := &BlockcypherUtxoRes{}

	req := h.blockcypherQuery()
	req.Set("unspentOnly", "true")
	scanApi, err := h.blockcypher(chainName)
	if err != nil {
		return "", err
	}
	err = scanApi.Get(ctx, res, path, req)
	if err != nil {
		return "", fmt.Errorf("failed to get balance from blockcypher, err=%v", err)
	}
	return strconv.FormatInt(res.FinalBalance, 10), nil
}
func (h *Handler) getTxStatusFromBlockcypher(ctx context.Context, chainName, hash string) (*chainrpc.TxResult, error) {
	if err := h.bcLimit(ctx); err != nil {
		return nil, err
	}
	path := "v1/" + strings.ToLower(chainName) + "/main/txs/" + hash
	res := &BlockcypherTxRes{}
	scanApi, err := h.blockcypher(chainName)
	if err != nil {
		return nil, err
	}
	err = scanApi.Get(ctx, res, path, h.blockcypherQuery())
	result := &chainrpc.TxResult{}
	if err != nil {
		if httpc.StatusCode(err) == http.StatusNotFound {
			result.Status = signing.TxStatusPending
			return result, nil
		}
		return nil, fmt.Errorf("failed to get hash status, err=%w", err)
	} else if res.BlockHeight > 0 &&
		res.DoubleSpend == false &&
		res.Confirmations > 0 &&
		res.Confidence == 1 {
		result.Status = signing.TxStatusSucceeded
		result.Height = strconv.FormatInt(res.BlockHeight, 10)
		return result, nil
	} else {
		result.Status = signing.TxStatusPending
		return result, nil
	}
}
func (h *Handler) getUtxoFromBlockcypher(ctx context.Context, chainName, address string) (*signing.UtxoList, error) {
	if err := h.bcLimit(ctx); err != nil {
		return nil, err
	}
	path := "v1/" + strings.ToLower(chainName) + "/main/addrs/" + address
	res := &BlockcypherUtxoRes{}

	req := h.blockcypherQuery()
	req.Set("unspentOnly", "true")
	scanApi, err := h.blockcypher(chainName)
	if err != nil {
		return nil, err
	}
	err = scanApi.Get(ctx, res, path, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get utxo from blockcypher, err=%v", err)
	}

	utxoList := &signing.UtxoList{}
	network := getNetwork(chainName)
	if network == "" {
		return nil, fmt.Errorf("failed to get network for %s", chainName)
	}
	for _, v := range res.Txrefs {
		if v.Confirmed == "" {
			continue
		} else if v.Confirmations == 0 {
			continue
		} else if network == walletutxo.NetworkEnumForDOGE && v.Value <= walletutxo.DogeDust {
			continue
		}
		pubKeyScript, err := walletutxo.AddressToScriptPubKey(address, network)
		if err != nil {
			return nil, fmt.Errorf("failed to AddressToScriptPubKey, addr=%s, err=%v", address, err)
		}
		utxo := &signing.UtxoInfo{
			Hash:   v.TxHash,
			Script: pubKeyScript,
			Index:  strconv.FormatInt(v.TxOutputN, 10),
			Value:  strconv.FormatInt(v.Value, 10),
		}
		utxoList.List = append(utxoList.List, utxo)
	}
	return utxoList, nil
}
func (h *Handler) scan() (*httpc.Request, error) {
	if h.scanApi == nil {
		return nil, fmt.Errorf("scan API URL is not configured")
	}
	return h.scanApi, nil
}

func (h *Handler) blockcypher(chainName string) (*httpc.Request, error) {
	if chainName == "DOGE" {
		return h.dogeApi, nil
	}
	return h.scan()
}

func (h *Handler) blockcypherQuery() url.Values {
	query := url.Values{}
	if h.blockcypherToken != "" {
		query.Set("token", h.blockcypherToken)
	}
	return query
}

func (h *Handler) bcLimit(ctx context.Context) error {
	minInterval := 5 * time.Second

	for {
		h.bcMu.Lock()
		wait := minInterval - time.Since(h.bcLastReq)
		if wait <= 0 {
			h.bcLastReq = time.Now()
			h.bcMu.Unlock()
			return nil
		}
		h.bcMu.Unlock()

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func getConfirmation(network string) uint64 {
	switch network {
	case walletutxo.NetworkEnumForBTC:
		return 0
	case walletutxo.NetworkEnumForDOGE:
		return 1
	case walletutxo.NetworkEnumForSYS:
		return 2
	}
	return 1
}

func getNetwork(chainName string) string {
	switch chainName {
	case "BTC":
		return walletutxo.NetworkEnumForBTC
	case "LTC":
		return walletutxo.NetworkEnumForLTC
	case "DOGE":
		return walletutxo.NetworkEnumForDOGE
	}
	return ""
}
