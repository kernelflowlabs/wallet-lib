package solana

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/kernelflowlabs/wallet-lib/common/httpc"
	chainrpc "github.com/kernelflowlabs/wallet-lib/rpc"
	"github.com/kernelflowlabs/wallet-lib/signing"
	walletsolana "github.com/kernelflowlabs/wallet-lib/signing/solana"

	"github.com/blocto/solana-go-sdk/common"
)

var _ chainrpc.BasicChainHandler = (*Handler)(nil)

type Handler struct {
	rpc           *httpc.Request
	heliusApi     *httpc.Request
	heliusApiMain *httpc.Request
}

func NewHandler(rpcUrl, heliusAPIKey string) (*Handler, error) {
	h := &Handler{}

	url := rpcUrl
	h.rpc = httpc.NewRequest(url,
		map[string]string{"Content-Type": "application/json"})
	h.heliusApi = httpc.NewRequest("https://api.helius.xyz/v0/token-metadata?api-key="+heliusAPIKey, nil)
	h.heliusApiMain = httpc.NewRequest("https://mainnet.helius-rpc.com/?api-key="+heliusAPIKey, nil)
	return h, nil
}

func (h *Handler) GetHeight(ctx context.Context) (string, error) {
	req := &BaseRequest{
		JsonRPC: "2.0",
		ID:      0,
		Method:  "getSlot",
		Params:  nil,
	}
	res := &GetSlotRes{}
	err := h.rpc.Post(ctx, res, "", req)
	if err != nil {
		return "", fmt.Errorf("failed to getSlot, err=%v", err)
	} else if res.Error.Code != 0 {
		return "", fmt.Errorf("failed to getSlot, errMsg=%s", res.Error.Message)
	}

	return strconv.FormatUint(res.Result, 10), nil
}

func (h *Handler) GetBlockHeight(ctx context.Context) (string, error) {
	req := &BaseRequest{
		JsonRPC: "2.0",
		ID:      0,
		Method:  "getBlockHeight",
		Params:  nil,
	}
	res := &GetBlockHeightRes{}
	err := h.rpc.Post(ctx, res, "", req)
	if err != nil {
		return "", fmt.Errorf("failed to getBlockHeight, err=%v", err)
	} else if res.Error.Code != 0 {
		return "", fmt.Errorf("failed to getBlockHeight, errMsg=%s", res.Error.Message)
	}

	return strconv.FormatUint(res.Result, 10), nil
}

func (h *Handler) GetBalance(ctx context.Context, address, contractAddress, blockNumber string) (string, error) {
	if contractAddress == signing.MagicContactAddressForNative ||
		contractAddress == signing.MagicContactAddressForNativeSOL {
		value, err := h.getBaseCoinBalance(ctx, address)
		if err != nil {
			return "", err
		}
		return value.String(), nil
	} else {
		if !walletsolana.ValidAddress(address) {
			return "", fmt.Errorf("invalid address")
		} else if !walletsolana.ValidAddress(contractAddress) {
			return "", fmt.Errorf("invalid contractAddress")
		}
		value, err := h.getTokenBalance(ctx, address, contractAddress)
		if err != nil {
			return "", err
		}
		return value.String(), nil
	}
}

func (h *Handler) SendTx(ctx context.Context, signedHex string) (string, error) {
	tx, err := hex.DecodeString(strings.TrimPrefix(signedHex, "0x"))
	if err != nil {
		return "", fmt.Errorf("failed to DecodeString, err=%v", err)
	}
	req := &BaseRequest{
		JsonRPC: "2.0",
		ID:      0,
		Method:  "sendTransaction",
		Params: []interface{}{
			base64.StdEncoding.EncodeToString(tx),
			SendTransactionConfig{
				SkipPreflight:       false,
				MaxRetries:          5,
				PreflightCommitment: "processed",
				Encoding:            "base64",
			},
		},
	}
	res := &SendTransactionRes{}
	err = h.rpc.Post(ctx, res, "", req)
	if err != nil {
		return "", fmt.Errorf("failed to sendTransaction, err=%v", err)
	} else if res.Error.Code != 0 {
		return "", fmt.Errorf("failed to sendTransaction, errMsg=%s", res.Error.Message)
	} else if res.Result == "" {
		return "", fmt.Errorf("failed to sendTransaction, result is empty")
	}

	return res.Result, nil
}

func (h *Handler) CheckTx(ctx context.Context, hash string) (*chainrpc.TxResult, error) {
	r := &chainrpc.TxResult{}

	req := &BaseRequest{
		JsonRPC: "2.0",
		ID:      0,
		Method:  "getSignatureStatuses",
		Params: []interface{}{[]string{
			hash,
		},
			SearchTransactionHistory{
				SearchTransactionHistory: true,
			},
		},
	}
	var err error
	res := &GetSignatureStatusRes{}
	err = h.rpc.Post(ctx, res, "", req)
	if err != nil {
		return nil, fmt.Errorf("failed to getSignatureStatuses, err=%v", err)
	} else if res.Error.Code != 0 {
		return nil, fmt.Errorf("failed to getSignatureStatuses, err=%s", res.Error.Message)
	} else if len(res.Result.Value) == 0 || res.Result.Value[0] == nil {
		r.Status = signing.TxStatusPending
		return r, nil
	}

	rv := res.Result.Value[0]
	r.Height = strconv.FormatUint(rv.Slot, 10)
	r.Time = strconv.FormatInt(time.Now().Unix(), 10)
	if rv.Err != nil {
		r.Status = signing.TxStatusFailed
		if e, ok := rv.Err.(map[string]interface{}); ok {
			for k, _ := range e {
				r.ErrMsg = k
			}
		}
	} else if rv.ConfirmationStatus == "processed" ||
		rv.ConfirmationStatus == "confirmed" {
		r.Status = signing.TxStatusPending
	} else if rv.ConfirmationStatus == "finalized" {
		r.Status = signing.TxStatusSucceeded
	} else {
		r.Status = signing.TxStatusUnknown
	}
	return r, nil
}

func (h *Handler) GetTxByHashWithLogs(ctx context.Context, hash string) (*GetTransactionWithLog, error) {
	req := &BaseRequest{
		JsonRPC: "2.0",
		ID:      1,
		Method:  "getTransaction",
		Params: []interface{}{
			hash,
			SearchTransaction{
				Commitment:                     "finalized",
				MaxSupportedTransactionVersion: 0,
			},
		},
	}
	res := &GetTransactionWithLog{}
	err := h.rpc.Post(ctx, res, "", req)
	if err != nil {
		return nil, fmt.Errorf("failed to _GetTransaction, err=%v", err)
	} else if res.Error.Code != 0 {
		return nil, fmt.Errorf("failed to _GetTransaction, err=%s", res.Error.Message)
	}
	return res, nil
}

func (h *Handler) CallContract(ctx context.Context, contractAddress, params, blockNumber string) ([]byte, error) {
	return nil, fmt.Errorf("CallContract not supported")
}

func (h *Handler) InquireChain(ctx context.Context, instruction, params string) (string, error) {
	switch instruction {
	case "getAccountInfo":
		req := &BaseRequest{
			JsonRPC: "2.0",
			ID:      0,
			Method:  "getAccountInfo",
			Params: []interface{}{
				params,
				struct {
					Encoding string `json:"encoding"`
				}{
					"jsonParsed",
				},
			},
		}
		res := &GetAccountInfoJsonRes{}
		err := h.rpc.Post(ctx, res, "", req)
		if err != nil {
			return "", fmt.Errorf("failed to getAccountInfo, err=%v", err)
		} else if res.Error.Code != 0 {
			return "", fmt.Errorf("failed to getAccountInfo, errMsg=%s", res.Error.Message)
		}
		valueBytes, err := json.Marshal(res.Result.Value)
		if err != nil {
			return "", fmt.Errorf("failed to Marshal for result, err=%v", err)
		}
		return string(valueBytes), nil
	case "getLatestBlockHash":
		req := &BaseRequest{
			JsonRPC: "2.0",
			ID:      0,
			Method:  "getLatestBlockhash",
			Params:  nil,
		}
		res := &GetLatestBlockhash{}
		var lastErr error
		for i := 0; i < 3; i++ {
			err := h.rpc.Post(ctx, res, "", req)
			if err == nil && res.Error.Code == 0 && res.Result.Value.Blockhash != "" {
				return res.Result.Value.Blockhash, nil
			}

			if err != nil {
				lastErr = fmt.Errorf("attempt %d failed to getFees, err=%v", i+1, err)
			} else if res.Error.Code != 0 {
				lastErr = fmt.Errorf("attempt %d failed to getFees, errMsg=%s", i+1, res.Error.Message)
			}
			if i < 2 {
				time.Sleep(time.Second)
			}
		}
		return "", lastErr
	case "hasATA":
		tmp := strings.Split(params, "_")
		if len(tmp) != 2 {
			return "", fmt.Errorf("len(tmp) != 2")
		}
		address := tmp[0]
		tokenAddress := tmp[1]
		ownerPub := common.PublicKeyFromString(address)
		mintPub := common.PublicKeyFromString(tokenAddress)
		legacyATA, _, err := walletsolana.FindAssociatedTokenAddressWithProgram(ownerPub, mintPub, false)
		if err != nil {
			return "", fmt.Errorf("failed to FindAssociatedTokenAddress, err=%v", err)
		}
		ata2022, _, err := walletsolana.FindAssociatedTokenAddressWithProgram(ownerPub, mintPub, true)
		if err != nil {
			return "", fmt.Errorf("failed to FindAssociatedTokenAddress (token-2022), err=%v", err)
		}
		legacyATAB58 := legacyATA.ToBase58()
		ata2022B58 := ata2022.ToBase58()
		req := &BaseRequest{
			JsonRPC: "2.0",
			ID:      0,
			Method:  "getTokenAccountsByOwner",
			Params: []interface{}{
				address,
				struct {
					Mint string `json:"mint"`
				}{
					tokenAddress,
				},
				struct {
					Encoding string `json:"encoding"`
				}{
					"jsonParsed",
				},
			},
		}
		res := &GetTokenAccountsByOwnerResponse{}
		err = h.rpc.Post(ctx, res, "", req)
		if err != nil {
			return "", fmt.Errorf("failed to getTokenAccountsByOwner, err=%v", err)
		} else if res.Error.Code != 0 {
			return "", fmt.Errorf("failed to getTokenAccountsByOwner, err=%s", res.Error.Message)
		}
		for _, v := range res.Result.Value {
			isLegacyATA := v.Pubkey == legacyATAB58 &&
				common.PublicKeyFromString(v.Account.Owner) == common.TokenProgramID &&
				v.Account.Data.Program == "spl-token"
			is2022ATA := v.Pubkey == ata2022B58 &&
				common.PublicKeyFromString(v.Account.Owner) == common.Token2022ProgramID &&
				v.Account.Data.Program == "spl-token-2022"
			if !isLegacyATA && !is2022ATA {
				continue
			} else if v.Account.Data.Parsed.Type != "account" {
				continue
			} else if v.Account.Data.Parsed.Info.Mint != tokenAddress {
				continue
			} else if v.Account.Data.Parsed.Info.State != "initialized" {
				continue
			}
			return "true", nil
		}
		return "false", nil
	case "getBlockByNumber":
		height, err := strconv.ParseUint(params, 10, 64)
		if err != nil {
			return "", fmt.Errorf("failed to ParseUint for height, err=%v", err)
		}
		block, err := h.getBlockByNumber(ctx, height)
		if err != nil {
			return "", fmt.Errorf("failed to getBlockByNumber, height=%d, err=%v", height, err)
		}
		var blockTx RpcBlock
		blockTx.Transactions = block.Result.Transactions
		blockTxBytes, err := json.Marshal(blockTx)
		if err != nil {
			return "", fmt.Errorf("failed to Marshal, err=%v", err)
		}
		return string(blockTxBytes), nil
	case "isBlockHashValid":
		req := &BaseRequest{
			JsonRPC: "2.0",
			ID:      0,
			Method:  "isBlockhashValid",
			Params:  []string{params},
		}
		res := &IsBlockHashValidRes{}
		err := h.rpc.Post(ctx, res, "", req)
		if err != nil {
			return "", fmt.Errorf("failed to getFees, err=%v", err)
		} else if res.Error.Code != 0 {
			return "", fmt.Errorf("failed to getFees, errMsg=%s", res.Error.Message)
		}
		return strconv.FormatBool(res.Result.Value), nil
	case "getTokenInfo":
		//first try
		tokenInfo := &chainrpc.TokenInfo{}
		{
			var out []HeliusTokenMetaResItem
			req := &HeliusTokenMetaReq{
				MintAccounts: []string{
					params,
				},
			}
			err := h.heliusApi.Post(ctx, &out, "", req)
			if err != nil {
				goto SECOND_TRY
			} else if len(out) != 1 {
				goto SECOND_TRY
			}

			if out[0].LegacyMetadata.Address != "" &&
				out[0].LegacyMetadata.Name != "" &&
				out[0].LegacyMetadata.Symbol != "" {
				tokenInfo.Name = out[0].LegacyMetadata.Name
				tokenInfo.Symbol = out[0].LegacyMetadata.Symbol
				tokenInfo.Decimals = strconv.FormatInt(int64(out[0].LegacyMetadata.Decimals), 10)
			} else if out[0].OnChainMetadata.Metadata.Data.Name != "" &&
				out[0].OnChainMetadata.Metadata.Data.Symbol != "" &&
				out[0].OnChainAccountInfo.AccountInfo.Data.Parsed.Info.Decimals != 0 {
				tokenInfo.Name = out[0].OnChainMetadata.Metadata.Data.Name
				tokenInfo.Symbol = out[0].OnChainMetadata.Metadata.Data.Symbol
				tokenInfo.Decimals = strconv.FormatInt(int64(out[0].OnChainAccountInfo.AccountInfo.Data.Parsed.Info.Decimals), 10)
			} else {
				goto SECOND_TRY
			}
			if tokenInfo.Name == "" || tokenInfo.Symbol == "" ||
				tokenInfo.Decimals == "" || tokenInfo.Decimals == "0" {
				goto SECOND_TRY
			}
			tokenInfoBytes, _ := json.Marshal(tokenInfo)
			return string(tokenInfoBytes), nil
		}
	SECOND_TRY:
		{
			var out HeliusTokenMetaMainRes
			req := &HeliusTokenMetaMainReq{}
			req.Jsonrpc = "2.0"
			req.Id = "my-id"
			req.Params.Id = params
			req.Method = "getAsset"
			req.Params.DisplayOptions.ShowFungible = true

			err := h.heliusApiMain.Post(ctx, &out, "", req)
			if err != nil {
				return "", fmt.Errorf("failed to heliusApiMain, err=%v", err)
			}
			tokenInfo.Name = out.Result.Content.Metadata.Name
			tokenInfo.Symbol = out.Result.Content.Metadata.Symbol
			tokenInfo.Decimals = strconv.FormatInt(out.Result.TokenInfo.Decimals, 10)
			if tokenInfo.Name == "" || tokenInfo.Symbol == "" ||
				tokenInfo.Decimals == "" || tokenInfo.Decimals == "0" {
				return "", fmt.Errorf("failed to get meta info 2, err=%v", err)
			}
			tokenInfoBytes, _ := json.Marshal(tokenInfo)
			return string(tokenInfoBytes), nil
		}
	case "getMinimumBalanceForRentExemption":
		space, err := strconv.ParseUint(params, 10, 64)
		if err != nil {
			return "", fmt.Errorf("failed to ParseUint for space, err=%v", err)
		}
		req := &BaseRequest{
			JsonRPC: "2.0",
			ID:      0,
			Method:  "getMinimumBalanceForRentExemption",
			Params:  []uint64{space},
		}
		res := &GetminimumbalanceforrentexemptionRes{}
		err = h.rpc.Post(ctx, res, "", req)
		if err != nil {
			return "", fmt.Errorf("failed to getMinimumBalanceForRentExemption, err=%v", err)
		} else if res.Error.Code != 0 {
			return "", fmt.Errorf("failed to getMinimumBalanceForRentExemption, errMsg=%s", res.Error.Message)
		}
		return strconv.FormatUint(res.Result, 10), nil
	case "getNonceAccountBlockHash":
		req := &BaseRequest{
			JsonRPC: "2.0",
			ID:      0,
			Method:  "getAccountInfo",
			Params: []interface{}{
				params,
				struct {
					Encoding string `json:"encoding"`
				}{
					"jsonParsed",
				},
			},
		}
		res := &GetAccountInfoJsonRes{}
		err := h.rpc.Post(ctx, res, "", req)
		if err != nil {
			return "", fmt.Errorf("failed to getAccountInfo, err=%v", err)
		} else if res.Error.Code != 0 {
			return "", fmt.Errorf("failed to getAccountInfo, errMsg=%s", res.Error.Message)
		} else if res.Result.Value.Data.Parsed.Info.Blockhash == "" {
			return "", fmt.Errorf("failed to getAccountInfo, Blockhash is empty")
		}
		return res.Result.Value.Data.Parsed.Info.Blockhash, nil
	case "getPriorityFee":
		req := &BaseRequest{
			JsonRPC: "2.0",
			ID:      0,
			Method:  "getRecentPrioritizationFees",
			Params:  []interface{}{},
		}
		res := &GetPrioritizationFee{}
		err := h.rpc.Post(ctx, res, "", req)
		if err != nil {
			return "", fmt.Errorf("failed to getRecentPrioritizationFees, err=%v", err)
		} else if res.Error.Code != 0 {
			return "", fmt.Errorf("failed to getRecentPrioritizationFees, errMsg=%s", res.Error.Message)
		}
		var maxFee uint64
		for _, it := range res.Result {
			if it.PrioritizationFee > maxFee {
				maxFee = it.PrioritizationFee
			}
		}
		return strconv.FormatUint(maxFee, 10), nil
	case "getTokenDecimals":
		if params == signing.MagicContactAddressForNative {
			return "9", nil
		}
		{
			var out []HeliusTokenMetaResItem
			req := &HeliusTokenMetaReq{
				MintAccounts: []string{
					params,
				},
			}
			err := h.heliusApi.Post(ctx, &out, "", req)
			if err != nil {
				goto DECIMALS_SECOND_TRY
			} else if len(out) != 1 {
				goto DECIMALS_SECOND_TRY
			}

			if out[0].LegacyMetadata.Address != "" {
				return strconv.FormatInt(int64(out[0].LegacyMetadata.Decimals), 10), nil
			}
			if out[0].OnChainAccountInfo.AccountInfo.Data.Parsed.Info.Decimals != 0 {
				return strconv.FormatInt(int64(out[0].OnChainAccountInfo.AccountInfo.Data.Parsed.Info.Decimals), 10), nil
			}
			goto DECIMALS_SECOND_TRY
		}
	DECIMALS_SECOND_TRY:
		{
			var out HeliusTokenMetaMainRes
			req := &HeliusTokenMetaMainReq{}
			req.Jsonrpc = "2.0"
			req.Id = "my-id"
			req.Params.Id = params
			req.Method = "getAsset"
			req.Params.DisplayOptions.ShowFungible = true

			err := h.heliusApiMain.Post(ctx, &out, "", req)
			if err != nil {
				return "", fmt.Errorf("failed to heliusApiMain, err=%v", err)
			}
			decimals := strconv.FormatInt(out.Result.TokenInfo.Decimals, 10)
			if decimals == "" || decimals == "0" {
				return "", fmt.Errorf("failed to get decimals 2, err=%v", err)
			}
			return decimals, nil
		}
	}
	return "", fmt.Errorf("unsupported funciton")
}

// unexported
func (h *Handler) getBaseCoinBalance(ctx context.Context, address string) (*big.Int, error) {
	req := &BaseRequest{
		JsonRPC: "2.0",
		ID:      0,
		Method:  "getBalance",
		Params:  []string{address},
	}
	res := &GetBalanceRes{}
	if err := h.rpc.Post(ctx, res, "", req); err != nil {
		return nil, fmt.Errorf("failed to getBalance, err=%v", err)
	} else if res.Error.Code != 0 {
		return nil, fmt.Errorf("failed to getBalance, errMsg=%s", res.Error.Message)
	} else if res.Result.Value == nil {
		return nil, fmt.Errorf("failed to getBalance, empty result")
	}

	return res.Result.Value, nil
}

func (h *Handler) getTokenBalance(ctx context.Context, address, contractAddress string) (*big.Int, error) {
	req := &BaseRequest{
		JsonRPC: "2.0",
		ID:      0,
		Method:  "getTokenAccountsByOwner",
		Params: []interface{}{
			address,
			struct {
				Mint string `json:"mint"`
			}{contractAddress},
			struct {
				Encoding string `json:"encoding"`
			}{"jsonParsed"},
		},
	}
	res := &GetTokenAccountsByOwnerResponse{}
	err := h.rpc.Post(ctx, res, "", req)
	if err != nil {
		return nil, fmt.Errorf("failed to getTokenAccountsByOwner, err=%v", err)
	} else if res.Error.Code != 0 {
		return nil, fmt.Errorf("failed to getTokenAccountsByOwner, err=%s", res.Error.Message)
	}

	total := big.NewInt(0)
	for _, v := range res.Result.Value {
		parsed := v.Account.Data.Parsed
		if parsed.Type != "account" || parsed.Info.Mint != contractAddress {
			continue
		}
		amount, ok := new(big.Int).SetString(parsed.Info.TokenAmount.Amount, 10)
		if !ok {
			return nil, fmt.Errorf("invalid token amount %q for account %s", parsed.Info.TokenAmount.Amount, v.Pubkey)
		}
		total.Add(total, amount)
	}
	return total, nil
}
func (h *Handler) getBlockByNumber(ctx context.Context, num uint64) (*GetBlockByNumberRes, error) {
	req := &BaseRequest{
		JsonRPC: "2.0",
		ID:      1,
		Method:  "getBlock",
		Params: []interface{}{
			num,
			map[string]interface{}{
				"encoding":                       "json",
				"maxSupportedTransactionVersion": 0,
				"transactionDetails":             "full",
				"rewards":                        false,
			},
		},
	}
	res := &GetBlockByNumberRes{}
	err := h.rpc.Post(ctx, res, "", req)
	if err != nil {
		return nil, fmt.Errorf("failed to getBlockByNumber, err=%v", err)
	} else if res.Error.Code != 0 {
		return nil, fmt.Errorf("failed to getBlockByNumber, err=%s", res.Error.Message)
	}
	return res, nil
}
