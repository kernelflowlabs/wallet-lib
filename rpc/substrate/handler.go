package substrate

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	gsrpc "github.com/centrifuge/go-substrate-rpc-client/v4"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
	"github.com/kernelflowlabs/wallet-lib/common/httpc"
	chainrpc "github.com/kernelflowlabs/wallet-lib/rpc"
	"github.com/kernelflowlabs/wallet-lib/signing"
	walletsubstrate "github.com/kernelflowlabs/wallet-lib/signing/substrate"
)

var _ chainrpc.BasicChainHandler = (*Handler)(nil)

type Handler struct {
	rpc           *gsrpc.SubstrateAPI
	network       string
	subscanAPIKey string
}

func NewHandler(url, network, subscanAPIKey string) (*Handler, error) {
	h := &Handler{}

	gsRpc, err := gsrpc.NewSubstrateAPI(url)
	if err != nil {
		return nil, fmt.Errorf("failed to NewSubstrateAPI, err=%v", err)
	}
	h.rpc = gsRpc
	h.network = network
	h.subscanAPIKey = subscanAPIKey
	return h, nil
}

func (h *Handler) GetHeight(ctx context.Context) (string, error) {
	r, err := h.rpc.RPC.Chain.GetHeaderLatest()
	if err != nil {
		return "", fmt.Errorf("failed to get latest block, err=%s", err.Error())
	}
	return strconv.FormatUint(uint64(r.Number), 10), nil
}

func (h *Handler) GetBalance(ctx context.Context, address, contractAddress, blockNumber string) (string, error) {
	if contractAddress != signing.MagicContactAddressForNative {
		return "", fmt.Errorf("invalid contractAddress")
	}
	meta, err := h.rpc.RPC.State.GetMetadataLatest()
	if err != nil {
		return "", fmt.Errorf("failed to GetMetadataLatest,err=%v", err)
	}

	key, err := types.CreateStorageKey(meta, "System", "Account", convert2PublicKey(address))
	if err != nil {
		return "", fmt.Errorf("failed to CreateStorageKey,err=%v", err)
	}
	var accountInfo types.AccountInfo
	ok, err := h.rpc.RPC.State.GetStorageLatest(key, &accountInfo)
	if err != nil {
		return "", fmt.Errorf("failed to GetStorageLatest,err=%v", err)
	} else if !ok {
		return "0", nil
	}
	return accountInfo.Data.Free.String(), nil
}

func (h *Handler) SendTx(ctx context.Context, signedHex string) (string, error) {
	var res string
	err := h.rpc.Client.Call(&res, "author_submitExtrinsic", signedHex)
	if err != nil {
		return "", fmt.Errorf("failed to SubmitExtrinsic ext,err=%v", err)
	}
	hash, err := types.NewHashFromHexString(res)
	if err != nil {
		return "", fmt.Errorf("failed to parse tx hash, err=%v", err)
	}
	return hash.Hex(), nil
}

func (h *Handler) CheckTx(ctx context.Context, hash string) (*chainrpc.TxResult, error) {
	r := &chainrpc.TxResult{}
	subRes, err := h.checkTxBySubscan(h.network, hash)
	if err != nil {
		return nil, fmt.Errorf("failed to checkTxBySubscan, err=%v", err)
	} else if subRes.Code != 0 {
		return nil, fmt.Errorf("failed to checkTxBySubscan, errMsg=%v", subRes.Message)
	}
	if subRes.Data.BlockNum == 0 {
		r.Status = signing.TxStatusPending
	} else if subRes.Data.Success == false {
		r.Status = signing.TxStatusFailed
	} else if subRes.Data.Success == true {
		r.Status = signing.TxStatusSucceeded
	} else {
		r.Status = signing.TxStatusUnknown
	}
	return r, nil
}

func (h *Handler) CallContract(ctx context.Context, contractAddress, params, blockNumber string) ([]byte, error) {
	return nil, fmt.Errorf("CallContract not supported")
}

func (h *Handler) InquireChain(ctx context.Context, instruction, params string) (string, error) {
	switch instruction {
	case "getNonce":
		var nextIndex types.U32
		if err := h.rpc.Client.Call(&nextIndex, "system_accountNextIndex", params); err == nil {
			return strconv.FormatUint(uint64(nextIndex), 10), nil
		}
		meta, err := h.rpc.RPC.State.GetMetadataLatest()
		if err != nil {
			return "", fmt.Errorf("failed to GetMetadataLatest,err=%v", err)
		}
		key, err := types.CreateStorageKey(meta, "System", "Account", convert2PublicKey(params))
		if err != nil {
			return "", fmt.Errorf("failed to CreateStorageKey,err=%v", err)
		}

		var accountInfo types.AccountInfo
		_, err = h.rpc.RPC.State.GetStorageLatest(key, &accountInfo)
		if err != nil {
			return "", fmt.Errorf("failed to GetStorageLatest for from accountInfo, err=%v", err)
		}
		return strconv.FormatUint(uint64(accountInfo.Nonce), 10), nil
	case "getChainInfo":
		res := walletsubstrate.ChainInfo{}
		meta, err := h.rpc.RPC.State.GetMetadataLatest()
		if err != nil {
			return "", fmt.Errorf("failed to GetMetadataLatest, err=%v", err)
		}
		callIndex, err := meta.FindCallIndex("Balances.transfer_keep_alive")
		if err != nil {
			return "", fmt.Errorf("failed to FindCallIndex, err=%v", err)
		}
		blockHash, err := h.getBlockHash()
		if err != nil {
			return "", fmt.Errorf("failed to getBlockHash, err=%v", err)
		}
		rv, err := h.getRuntimeVersion()
		if err != nil {
			return "", fmt.Errorf("failed to getRuntimeVersion, err=%v", err)
		}

		existentialDeposit := ""
		switch h.network {
		case walletsubstrate.NetworkEnumForDOT:
			existentialDeposit = "10000000000"
		case walletsubstrate.NetworkEnumForKSM:
			existentialDeposit = "34000000"
		case walletsubstrate.NetworkEnumForAZERO:
			existentialDeposit = "500"
		case walletsubstrate.NetworkEnumForDOTASSETSHUB:
			existentialDeposit = "1000000000"
		}
		res.CallIndex = fmt.Sprintf("%02x%02x", callIndex.SectionIndex, callIndex.MethodIndex)
		res.BlockHash = blockHash.Hex()
		res.GenesisHash = blockHash.Hex()
		res.HasCheckMetadataHash = hasCheckMetadataHashExtension(meta)
		res.HasAssetTxPayment = hasAssetTxPaymentExtension(meta)
		res.SpecVersion = strconv.FormatUint(uint64(rv.SpecVersion), 10)
		res.TransactionVersion = strconv.FormatUint(uint64(rv.TransactionVersion), 10)
		res.ExistentialDeposit = existentialDeposit
		resBytes, err := json.Marshal(res)
		if err != nil {
			return "", fmt.Errorf("failed to Marshal for res, err=%v", err)
		}
		return string(resBytes), nil
	}
	return "", fmt.Errorf("unsupported function")
}

// unexported
func (h *Handler) getBlockHash() (types.Hash, error) {
	return h.rpc.RPC.Chain.GetBlockHash(0)
}
func (h *Handler) getRuntimeVersion() (*types.RuntimeVersion, error) {
	return h.rpc.RPC.State.GetRuntimeVersionLatest()
}
func (h *Handler) checkTxBySubscan(network, hash string) (*SubscanExtrinsicRes, error) {
	baseUrl, ok := getSubscanURL(network)
	if !ok {
		return nil, fmt.Errorf("unsupported network: %s", network)
	}
	api := httpc.NewRequest(baseUrl, map[string]string{
		"Content-Type": "application/json",
		"X-API-Key":    h.subscanAPIKey,
	})
	res := &SubscanExtrinsicRes{}
	err := api.Post(context.Background(), res, "", SubscanExtrinsicReq{Hash: hash})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func getConfirmation(network string) uint64 {
	switch network {
	case walletsubstrate.NetworkEnumForDOT, walletsubstrate.NetworkEnumForDOTASSETSHUB:
		return 1
	case walletsubstrate.NetworkEnumForKSM:
		return 2
	case walletsubstrate.NetworkEnumForAZERO:
		return 10
	}
	return 1
}

func hasAssetTxPaymentExtension(meta *types.Metadata) bool {
	if meta.Version != 14 {
		return false
	}
	for _, ext := range meta.AsMetadataV14.Extrinsic.SignedExtensions {
		if string(ext.Identifier) == "ChargeAssetTxPayment" {
			return true
		}
	}
	return false
}

func hasCheckMetadataHashExtension(meta *types.Metadata) bool {
	if meta.Version != 14 {
		return false
	}
	for _, ext := range meta.AsMetadataV14.Extrinsic.SignedExtensions {
		if string(ext.Identifier) == "CheckMetadataHash" {
			return true
		}
	}
	return false
}

func getDecimals(network string) int64 {
	switch network {
	case walletsubstrate.NetworkEnumForDOT, walletsubstrate.NetworkEnumForDOTASSETSHUB:
		return 10
	case walletsubstrate.NetworkEnumForKSM:
		return 12
	case walletsubstrate.NetworkEnumForAZERO, walletsubstrate.NetworkEnumForTAO:
		return 12
	}
	return 0
}
