package ton

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/tonkeeper/tongo"
	"github.com/tonkeeper/tongo/liteapi"
	"github.com/tonkeeper/tongo/tlb"

	"github.com/kernelflowlabs/wallet-lib/common/httpc"
	chainrpc "github.com/kernelflowlabs/wallet-lib/rpc"
	"github.com/kernelflowlabs/wallet-lib/signing"
	walletton "github.com/kernelflowlabs/wallet-lib/signing/ton"
)

var _ chainrpc.BasicChainHandler = (*Handler)(nil)

type Handler struct {
	url          string
	liteapi      *liteapi.Client
	api          *httpc.Request
	tokenMetaApi *httpc.Request
}

func NewHandler(url string) (*Handler, error) {
	tmp := strings.Split(url, ";")
	if len(tmp) != 2 {
		return nil, fmt.Errorf("invalid url")
	}

	h := &Handler{}
	client, err := liteapi.NewClientWithDefaultMainnet()
	if err != nil {
		return nil, fmt.Errorf("fail to create liteapi client, err=%w", err)
	}
	h.liteapi = client

	h.api = httpc.NewRequest(tmp[1], map[string]string{
		"Content-Type": "application/json",
	})
	h.tokenMetaApi = httpc.NewRequest("", map[string]string{
		"Content-Type": "application/json",
	})
	return h, nil
}

func (h *Handler) GetHeight(ctx context.Context) (string, error) {
	block, err := h.liteapi.GetMasterchainInfo(ctx)
	if err != nil {
		return "", fmt.Errorf("fail to GetMasterchainInfo, err=%v", err)
	}
	return strconv.FormatUint(uint64(block.Last.Seqno), 10), nil
}

func (h *Handler) GetBalance(ctx context.Context, address, contractAddress, blockNumber string) (string, error) {
	var balance string
	addr, err := tongo.ParseAddress(address)
	if err != nil {
		return "", fmt.Errorf("fail to ParseAddress for address, err=%v", err)
	}
	if contractAddress == signing.MagicContactAddressForNative {
		acc, err := h.liteapi.GetAccountState(ctx, addr.ID)
		if err != nil {
			return "", fmt.Errorf("fail to GetAccountState, err=%v", err)
		}
		balance = strconv.FormatUint(uint64(acc.Account.Account.Storage.Balance.Grams), 10)
	} else {
		master, err := tongo.ParseAddress(contractAddress)
		if err != nil {
			return "", fmt.Errorf("fail to ParseAddress for contractAddress, err=%v", err)
		}
		jettaAddr, err := h.liteapi.GetJettonWallet(ctx, master.ID, addr.ID)
		if err != nil {
			return "", fmt.Errorf("fail to GetJettonWallet, err=%v", err)
		}
		bal, err := h.liteapi.GetJettonBalance(ctx, jettaAddr)
		if err != nil {
			return "", fmt.Errorf("fail to GetJettonBalance, err=%v", err)
		}
		balance = bal.String()
	}
	return balance, nil
}

func (h *Handler) CheckTx(ctx context.Context, hash string) (*chainrpc.TxResult, error) {
	path := "v2/blockchain/transactions/" + hash
	result := &chainrpc.TxResult{
		Status: signing.TxStatusSucceeded,
	}
	res := &ApiTransaction{}
	err := h.api.Get(ctx, res, path, nil)
	if err != nil {
		if httpc.StatusCode(err) != http.StatusNotFound {
			return nil, fmt.Errorf("fail to get transaction, err=%w", err)
		}
		result.Status = signing.TxStatusPending
		return result, nil
	} else if res.Error != "" {
		if strings.Contains(res.Error, "entity not found") {
			result.Status = signing.TxStatusPending
		} else {
			result.Status = signing.TxStatusFailed
		}
		return result, nil
	}

	var inMsg InMsgDetails
	var outMsg OutMsgDetails
	tmp, ok := res.InMsg.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("inMsg type error")
	}
	tmpString, err := json.Marshal(tmp)
	if err != nil {
		return nil, fmt.Errorf("fail to Marshal inMsg, err=%v", err)
	}
	if err := json.Unmarshal(tmpString, &inMsg); err != nil {
		return nil, fmt.Errorf("fail to Unmarshal inMsg, err=%v", err)
	}

	if len(res.OutMsgs) > 0 {
		tmp, ok = res.OutMsgs[0].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("outMsg type error")
		}
		tmpString, err := json.Marshal(tmp)
		if err != nil {
			return nil, fmt.Errorf("fail to Marshal outMsg, err=%v", err)
		}
		if err := json.Unmarshal(tmpString, &outMsg); err != nil {
			return nil, fmt.Errorf("fail to Unmarshal outMsg, err=%v", err)
		}
	}

	msgStatus := func(msgHash, typeSel string) (bool, bool, error) {
		if msgHash == "" {
			return false, false, nil
		}
		ok, err := h.getHashStatus(ctx, msgHash, typeSel)
		if err == nil {
			return ok, false, nil
		}
		if httpc.StatusCode(err) == http.StatusNotFound || strings.Contains(err.Error(), "entity not found") {
			return false, true, nil
		}
		return false, false, err
	}
	inMsgHashStatus, pending, err := msgStatus(inMsg.Hash, "in")
	if err != nil {
		return nil, fmt.Errorf("fail to check inbound message, err=%w", err)
	} else if pending {
		result.Status = signing.TxStatusPending
		return result, nil
	}
	outMsgHashStatus, pending, err := msgStatus(outMsg.Hash, "out")
	if err != nil {
		return nil, fmt.Errorf("fail to check outbound message, err=%w", err)
	} else if pending {
		result.Status = signing.TxStatusPending
		return result, nil
	}

	if inMsgHashStatus && outMsgHashStatus {
		result.Status = signing.TxStatusSucceeded
	} else {
		result.Status = signing.TxStatusFailed
	}
	return result, nil
}

func (h *Handler) GetTransfersByHash(ctx context.Context, hash string, confirmation uint64) (*chainrpc.TxTransfers, error) {
	r := &chainrpc.TxTransfers{Hash: hash}
	path := "v2/blockchain/transactions/" + hash
	res := &ApiTransaction{}
	err := h.api.Get(ctx, res, path, nil)
	if err != nil {
		return nil, fmt.Errorf("fail to get transactions, err=%v", err)
	} else if res.Error != "" {
		return nil, fmt.Errorf("fail to get transactions, errMsg=%v", res.Error)
	} else if len(res.OutMsgs) != 1 {
		return nil, fmt.Errorf("len(res.OutMsgs) != 1")
	}

	var inMsg InMsgDetails
	var outMsg OutMsgDetails
	inMsgTmp, ok := res.InMsg.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("inMsg type error")
	}
	tmpString, err := json.Marshal(inMsgTmp)
	if err != nil {
		return nil, fmt.Errorf("fail to Marshal inMsg, err=%v", err)
	}
	if err := json.Unmarshal(tmpString, &inMsg); err != nil {
		return nil, fmt.Errorf("fail to Unmarshal inMsg, err=%v", err)
	}
	outMsgTmp, ok := res.OutMsgs[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("outMsg type error")
	}
	tmpString, err = json.Marshal(outMsgTmp)
	if err != nil {
		return nil, fmt.Errorf("fail to Marshal outMsg, err=%v", err)
	}
	if err := json.Unmarshal(tmpString, &outMsg); err != nil {
		return nil, fmt.Errorf("fail to Unmarshal outMsg, err=%v", err)
	}

	var tokenRecipient, tokenAmount, memo string
	if outMsg.DecodedBody.Text != "" {
		memo = outMsg.DecodedBody.Text
	} else if outMsg.DecodedBody.ForwardPayload.Value.Value.Text != "" {
		memo = outMsg.DecodedBody.ForwardPayload.Value.Value.Text
		tokenRecipient, err = walletton.RawToAddress(outMsg.DecodedBody.Destination)
		if err != nil {
			return nil, fmt.Errorf("fail to RawToAddress for token amount, err=%v", err)
		}
		tokenAmount = outMsg.DecodedBody.Amount
		if tokenAmount == "" || tokenAmount == "0" {
			return nil, fmt.Errorf("tokenAmount is empty")
		}
	} else {
		return nil, fmt.Errorf("memo cannot be empty")
	}
	tmp := strings.Split(memo, ":")
	if len(tmp) != 7 {
		return nil, fmt.Errorf("memo length error")
	}
	contractAddress := tmp[6]

	sender, err := walletton.RawToAddress(outMsg.Source.Address)
	if err != nil {
		return nil, fmt.Errorf("fail to FromAddress, err=%v", err)
	}
	dst, err := walletton.RawToAddress(outMsg.Destination.Address)
	if err != nil {
		return nil, fmt.Errorf("fail to RawToAddress for Destination, err=%v", err)
	}
	var recipient, amount string
	if contractAddress == signing.MagicContactAddressForNative && outMsg.OpCode == "0x00000000" {
		recipient = dst
		amount = strconv.FormatUint(outMsg.Value, 10)
	} else {
		if outMsg.DecodedOpName != "jetton_transfer" {
			return nil, fmt.Errorf("not a jetton_transfer transaction")
		}
		realUserContractAddress := dst
		masterAddr, err := tongo.ParseAddress(tmp[6])
		if err != nil {
			return nil, fmt.Errorf("fail to ParseAddress for master, err=%v", err)
		}
		ownerAddr, err := tongo.ParseAddress(sender)
		if err != nil {
			return nil, fmt.Errorf("fail to ParseAddress for owner, err=%v", err)
		}
		jettaAddr, err := h.liteapi.GetJettonWallet(ctx, masterAddr.ID, ownerAddr.ID)
		if err != nil {
			return nil, fmt.Errorf("fail to GetJettonWallet, err=%v", err)
		}
		expectedUserContractAddress, err := walletton.AddressToNoBounce(jettaAddr.ToHuman(false, false))
		if err != nil {
			return nil, fmt.Errorf("fail to AddressToNoBounce, err=%v", err)
		}
		if realUserContractAddress != expectedUserContractAddress {
			return nil, fmt.Errorf("realUserContractAddress != expectedUserContractAddress")
		}
		recipient = tokenRecipient
		amount = tokenAmount
	}

	if sender == "" || recipient == "" || amount == "" || contractAddress == "" {
		r.Rejected = true
		r.ErrMsg = "incomplete return data"
		return r, nil
	}

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

	r.Transfers = append(r.Transfers, &chainrpc.Transfer{
		Sender:          sender,
		Recipient:       recipient,
		Amount:          amount,
		ContractAddress: contractAddress,
		Memo:            memo,
	})
	return r, nil
}

func (h *Handler) SendTx(ctx context.Context, signedHex string) (string, error) {
	signedBytes, err := hex.DecodeString(strings.TrimPrefix(signedHex, "0x"))
	if err != nil {
		return "", fmt.Errorf("fail to DecodeString signedHex, err=%v", err)
	}
	status, err := h.liteapi.SendMessage(ctx, signedBytes)
	if status != 1 {
		return "", fmt.Errorf("fail to SendMessage, status=%d, err=%v", status, err)
	}
	return "", nil
}

func (h *Handler) CallContract(ctx context.Context, contractAddress, params, blockNumber string) ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

func (h *Handler) InquireChain(ctx context.Context, instruction, params string) (string, error) {
	switch instruction {
	case "getNonce":
		addr, err := tongo.ParseAddress(params)
		if err != nil {
			return "", fmt.Errorf("fail to ParseAddress for address, err=%v", err)
		}
		nonce, err := h.liteapi.GetSeqno(ctx, addr.ID)
		if err != nil {
			return "", fmt.Errorf("fail to GetSeqno, err=%v", err)
		}
		return strconv.FormatUint(uint64(nonce), 10), nil
	case "getAddressActivated":
		addr, err := tongo.ParseAddress(params)
		if err != nil {
			return "", fmt.Errorf("fail to ParseAddress for address, err=%v", err)
		}
		acc, err := h.liteapi.GetAccountState(ctx, addr.ID)
		if err != nil {
			return "", fmt.Errorf("fail to GetAccountState, err=%v", err)
		}
		return strconv.FormatBool(acc.Account.Status() == tlb.AccountActive), nil
	case "getTokenInfo":
		addr, err := tongo.ParseAddress(params)
		if err != nil {
			return "", fmt.Errorf("fail to ParseAddress for address, err=%v", err)
		}
		tokenInfo := &chainrpc.TokenInfo{}
		meta, err := h.liteapi.GetJettonData(ctx, addr.ID)
		if err != nil {
			return "", fmt.Errorf("fail to GetJettonData for address, err=%v", err)
		}
		if !(meta.Name != "" && meta.Symbol != "") && meta.Uri != "" && meta.Decimals != "" {
			h.tokenMetaApi.SetBaseUrl(meta.Uri)
			var remotesTokenMeta RemotesTokenMeta
			if err := h.tokenMetaApi.Get(ctx, &remotesTokenMeta, "", nil); err != nil {
				return "", fmt.Errorf("fail to Get remotesTokenMeta, err=%v", err)
			}
			tokenInfo.Name = remotesTokenMeta.Name
			tokenInfo.Symbol = remotesTokenMeta.Symbol
			tokenInfo.Decimals = meta.Decimals
		} else if meta.Name != "" && meta.Symbol != "" && meta.Decimals != "" {
			tokenInfo.Name = meta.Name
			tokenInfo.Symbol = meta.Symbol
			tokenInfo.Decimals = meta.Decimals
		} else {
			return "", fmt.Errorf("incomplete token meta")
		}
		tokenInfoBytes, _ := json.Marshal(tokenInfo)
		return string(tokenInfoBytes), nil
	case "getJettonWallet":
		tmp := strings.Split(params, ":")
		if len(tmp) != 2 {
			return "", fmt.Errorf("len(tmp) != 2")
		}
		masterAddr, err := tongo.ParseAddress(tmp[0])
		if err != nil {
			return "", fmt.Errorf("fail to ParseAddress for master, err=%v", err)
		}
		ownerAddr, err := tongo.ParseAddress(tmp[1])
		if err != nil {
			return "", fmt.Errorf("fail to ParseAddress for owner, err=%v", err)
		}
		jettaAddr, err := h.liteapi.GetJettonWallet(ctx, masterAddr.ID, ownerAddr.ID)
		if err != nil {
			return "", fmt.Errorf("fail to GetJettonWallet, err=%v", err)
		}
		return jettaAddr.ToHuman(false, false), nil
	case "getBlockByNumber":
		path := "v2/blockchain/masterchain/" + params + "/transactions"
		res := &ApiBlock{}
		err := h.api.Get(ctx, res, path, nil)
		if err != nil {
			return "", fmt.Errorf("fail to get masterchain transactions, err=%v", err)
		} else if res.Error != "" {
			return "", fmt.Errorf("fail to get masterchain transactions, errMsg=%v", res.Error)
		}
		resBytes, err := json.Marshal(res)
		if err != nil {
			return "", fmt.Errorf("fail to Marshal for res, err=%v", err)
		}
		return string(resBytes), nil
	}
	return "", fmt.Errorf("unsupported function")
}

func (h *Handler) getHashStatus(ctx context.Context, hash, typeSel string) (bool, error) {
	if hash == "" {
		return false, fmt.Errorf("fail to getHashStatus hash empty")
	}
	inrcm := 0
	var txHash []string
	for {
		path := "v2/blockchain/transactions/" + hash
		res := &ApiTransaction{}
		err := h.api.Get(ctx, res, path, nil)
		if err != nil {
			return false, fmt.Errorf("fail to getHashStatus, err=%w", err)
		} else if res.Error != "" {
			return false, fmt.Errorf("fail to getHashStatus res.Error, err=%v", res.Error)
		} else if len(res.OutMsgs) == 0 && inrcm > 0 {
			return true, nil
		} else if !res.Success {
			return false, nil
		}
		inrcm++
		var inMsg InMsgDetails
		var outMsg OutMsgDetails
		tmp, ok := res.InMsg.(map[string]interface{})
		if !ok {
			return false, fmt.Errorf("inMsg type error")
		}
		tmpString, err := json.Marshal(tmp)
		if err != nil {
			return false, fmt.Errorf("fail to Marshal inMsg, err=%v", err)
		}
		if err := json.Unmarshal(tmpString, &inMsg); err != nil {
			return false, fmt.Errorf("fail to Unmarshal inMsg, err=%v", err)
		}
		if len(res.OutMsgs) > 0 {
			tmp, ok = res.OutMsgs[0].(map[string]interface{})
			if !ok {
				return false, fmt.Errorf("outMsg type error")
			}
			tmpString, err := json.Marshal(tmp)
			if err != nil {
				return false, fmt.Errorf("fail to Marshal outMsg, err=%v", err)
			}
			if err := json.Unmarshal(tmpString, &outMsg); err != nil {
				return false, fmt.Errorf("fail to Unmarshal outMsg, err=%v", err)
			}
		}

		txHash = append(txHash, strings.ToLower(hash))
		shash := inMsg.Hash
		if typeSel == "out" {
			if outMsg.Hash != "" {
				shash = outMsg.Hash
			} else {
				shash = ""
			}
		}
		if shash != "" {
			isInArr := false
			for _, thash := range txHash {
				if strings.EqualFold(thash, shash) {
					isInArr = true
					break
				}
			}
			if isInArr {
				return true, nil
			}
			hash = shash
		} else {
			return true, nil
		}
	}
}
