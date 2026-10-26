package ton

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"github.com/tonkeeper/tongo"
	"github.com/tonkeeper/tongo/abi"
	"github.com/tonkeeper/tongo/boc"
	"github.com/tonkeeper/tongo/contract/jetton"
	"github.com/tonkeeper/tongo/tlb"
	"github.com/tonkeeper/tongo/ton"
	"github.com/tonkeeper/tongo/wallet"

	"github.com/kernelflowlabs/wallet-lib/signing"
)

var nowFunc = time.Now

func NewTxBuilder(ti *Ingredient) *TxBuilder {
	return &TxBuilder{
		Ingredient: ti,
	}
}

func (tx *TxBuilder) Build() error {
	if tx == nil {
		return fmt.Errorf("tx == nil")
	}
	tx.sigHash = nil
	tx.unsignedHex = ""
	tx.txHash = ""
	amountBig, ok := big.NewInt(0).SetString(tx.Ingredient.Amount, 10)
	if !ok {
		return fmt.Errorf("fail to SetString Amount")
	}
	if amountBig.Sign() < 0 {
		return fmt.Errorf("amount must not be negative")
	}
	senderAaddr, err := tongo.ParseAddress(tx.Ingredient.Sender)
	if err != nil {
		return fmt.Errorf("fail to ParseAddress for sender, err=%v", err)
	}
	recipientAddr, err := tongo.ParseAddress(tx.Ingredient.Recipient)
	if err != nil {
		return fmt.Errorf("fail to ParseAddress for recipientAaddr, err=%v", err)
	}
	seqno, err := strconv.ParseUint(tx.Ingredient.Nonce, 10, 64)
	if err != nil {
		return fmt.Errorf("fail to ParseUint for Nonce, err=%v", err)
	}
	publicKeyBytes, err := hex.DecodeString(tx.Ingredient.SenderPublicKey)
	if err != nil {
		return fmt.Errorf("fail to DecodeString for PublicKey, err=%v", err)
	}
	var publicKey tlb.Bits256
	copy(publicKey[:], publicKeyBytes[:])

	var intMsg tlb.Message
	var mode uint8
	if tx.Ingredient.ContractAddress == signing.MagicContactAddressForNative {
		if amountBig.BitLen() > 64 {
			return fmt.Errorf("amount out of range %q", tx.Ingredient.Amount)
		}
		messages := wallet.SimpleTransfer{
			Amount:  tlb.Grams(amountBig.Uint64()),
			Address: recipientAddr.ID,
			Comment: tx.Ingredient.Memo,
		}
		intMsg, mode, err = simpleTransferToInternal(messages)
		if err != nil {
			return fmt.Errorf("fail to simpleTransferToInternal for msg, err=%v", err)
		}
	} else {
		memoCell := boc.NewCell()
		if tx.Ingredient.Memo != "" {
			if err := tlb.Marshal(memoCell, wallet.TextComment(tx.Ingredient.Memo)); err != nil {
				return fmt.Errorf("fail to Marshal for Memo, err=%v", err)
			}
		}
		masterAddr, err := tongo.ParseAddress(tx.Ingredient.ContractAddress)
		if err != nil {
			return fmt.Errorf("fail to ParseAddress for ContractAddress, err=%v", err)
		}
		feeBig, ok := big.NewInt(0).SetString(tx.Ingredient.Fee, 10)
		if !ok {
			return fmt.Errorf("fail to SetString Fee")
		}
		if feeBig.Sign() < 0 || feeBig.BitLen() > 64 {
			return fmt.Errorf("fee out of range %q", tx.Ingredient.Fee)
		}
		messages := jetton.TransferMessage{
			Jetton: &jetton.Jetton{
				Master: masterAddr.ID,
			},
			Sender:              senderAaddr.ID,
			JettonAmount:        amountBig,
			Destination:         recipientAddr.ID,
			ResponseDestination: &senderAaddr.ID,
			AttachedTon:         tlb.Grams(feeBig.Uint64()),
			ForwardTonAmount:    tlb.Grams(1),
			ForwardPayload:      memoCell,
		}
		intMsg, mode, err = transferMessageToInternal(messages, tx.Ingredient.JettonWallet)
		if err != nil {
			return fmt.Errorf("fail to transferMessageToInternal for msg, err=%v", err)
		}
	}

	cell := boc.NewCell()
	if err := tlb.Marshal(cell, intMsg); err != nil {
		return fmt.Errorf("fail to Marshal for intMsg, err=%v", err)
	}
	rawMsg := wallet.RawMessage{
		Message: cell,
		Mode:    mode,
	}
	msgConfig := wallet.MessageConfig{
		Seqno:      uint32(seqno),
		ValidUntil: nowFunc().Add(defaultMessageLifetime),
		V5MsgType:  wallet.V5MsgTypeSignedExternal,
	}
	actions := make([]wallet.W5SendMessageAction, 0, 1)
	actions = append(actions, wallet.W5SendMessageAction{
		Msg:  rawMsg.Message,
		Mode: rawMsg.Mode,
	})
	w5Actions := wallet.W5Actions(actions)
	ntx := nativeTx{
		WalletId:        walletId,
		ValidUntil:      uint32(msgConfig.ValidUntil.Unix()),
		Seqno:           msgConfig.Seqno,
		Actions:         &w5Actions,
		ExtendedActions: nil,
	}
	bodyCell := boc.NewCell()
	if err := bodyCell.WriteUint(uint64(msgConfig.V5MsgType), 32); err != nil {
		return fmt.Errorf("fail to WriteUint, err=%v", err)
	}
	if err := tlb.Marshal(bodyCell, ntx); err != nil {
		return fmt.Errorf("fail to Marshal for ntx, err=%v", err)
	}
	sigHash, err := bodyCell.Hash()
	if err != nil {
		return fmt.Errorf("fail to Hash for bodyCell, err=%v", err)
	}
	bodyCellBytes, err := bodyCell.MarshalJSON()
	if err != nil {
		return fmt.Errorf("fail to ToBocString bodyCell, err=%v", err)
	}

	tx.sigHash = append(tx.sigHash, hex.EncodeToString(sigHash))
	tx.unsignedHex = hex.EncodeToString(bodyCellBytes)
	return nil
}

func (tx *TxBuilder) Sign(privateKey []byte) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("tx == nil")
	} else if len(tx.sigHash) != 1 {
		return "", fmt.Errorf("tx.SigHash == nil")
	}

	sigHash, err := hex.DecodeString(tx.sigHash[0])
	if err != nil {
		return "", fmt.Errorf("fail to DecodeString for sigHash, err=%v", err)
	}
	if len(privateKey) != ed25519.SeedSize {
		return "", fmt.Errorf("invalid private key length %d", len(privateKey))
	}
	p := ed25519.NewKeyFromSeed(privateKey)
	signatureBytes := ed25519.Sign(p, sigHash[:])
	if len(signatureBytes) != 64 {
		return "", fmt.Errorf("sign error,length is not equal 64, length=%d", len(signatureBytes))
	}
	return hex.EncodeToString(signatureBytes), nil
}

func (tx *TxBuilder) ConcatSignature(signature string, isDerFormat bool) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("tx == nil")
	} else if tx.unsignedHex == "" {
		return "", fmt.Errorf("tx.UnsignedHex == nil")
	}

	bodyCellBytes, err := hex.DecodeString(tx.unsignedHex)
	if err != nil {
		return "", fmt.Errorf("fail to DecodeString for UnsignedHex, err=%v", err)
	}
	parsed := boc.NewCell()
	if err := parsed.UnmarshalJSON(bodyCellBytes); err != nil {
		return "", fmt.Errorf("fail to UnmarshalJSON unsignedBytes, err=%v", err)
	}

	if isDerFormat {
		return "", fmt.Errorf("der format not supported")
	}
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return "", fmt.Errorf("fail to DecodeString for Signature, err=%v", err)
	}

	bodyCell := boc.NewCell()
	if err := bodyCell.WriteBitString(parsed.RawBitString()); err != nil {
		return "", fmt.Errorf("fail to WriteBitString, err=%v", err)
	}
	for _, ref := range parsed.Refs() {
		if err := bodyCell.AddRef(ref); err != nil {
			return "", fmt.Errorf("fail to AddRef, err=%v", err)
		}
	}
	if err := bodyCell.WriteBytes(sigBytes); err != nil {
		return "", fmt.Errorf("fail to WriteBytes, err=%v", err)
	}
	senderAaddr, err := tongo.ParseAddress(tx.Ingredient.Sender)
	if err != nil {
		return "", fmt.Errorf("fail to ParseAddress for sender, err=%v", err)
	}
	var init *tlb.StateInit
	if tx.Ingredient.Nonce == "0" {
		init, err = generateStateInitV5R1(tx.Ingredient.SenderPublicKey)
		if err != nil {
			return "", fmt.Errorf("fail to generateStateInitV5R1, err=%v", err)
		}
	}
	extMsg, err := ton.CreateExternalMessage(senderAaddr.ID, bodyCell, init, tlb.VarUInteger16{})
	if err != nil {
		return "", fmt.Errorf("fail to CreateExternalMessage, err=%v", err)
	}
	extMsgCell := boc.NewCell()
	if err := tlb.Marshal(extMsgCell, extMsg); err != nil {
		return "", fmt.Errorf("fail to Marshal for extMsgCell, err=%v", err)
	}
	msgHash, err := extMsgCell.Hash256()
	if err != nil {
		return "", fmt.Errorf("fail to Hash256 for extMsgCell, err=%v", err)
	}
	toBeSend, err := extMsgCell.ToBocCustom(false, false, false, 0)
	if err != nil {
		return "", fmt.Errorf("fail to ToBocCustom for extMsgCell, err=%v", err)
	}
	tx.txHash = hex.EncodeToString(msgHash[:])
	return hex.EncodeToString(toBeSend), nil
}

func (tx *TxBuilder) GetTxHash() string {
	return tx.txHash
}

func (tx *TxBuilder) GetSigHash() []string {
	return tx.sigHash
}

func (tx *TxBuilder) GetUnsignedHex() string {
	return tx.unsignedHex
}

func (tx *TxBuilder) SetSigHash(sigHash []string) {
	tx.sigHash = sigHash
}

func (tx *TxBuilder) SetUnsignedHex(unsignedHex string) {
	tx.unsignedHex = unsignedHex
}

type (
	Ingredient struct {
		TxType          string `json:"txType"`
		ContractAddress string `json:"contractAddress"`
		Sender          string `json:"sender"`
		SenderPublicKey string `json:"senderPublicKey"`
		Recipient       string `json:"recipient"`
		JettonWallet    string `json:"jettonWallet,omitempty"`
		Amount          string `json:"amount"`
		Nonce           string `json:"nonce,omitempty"`
		Memo            string `json:"memo,omitempty"`
		Fee             string `json:"fee"`
	}

	TxBuilder struct {
		*Ingredient
		unsignedHex string
		sigHash     []string
		txHash      string
	}

	nativeTx struct {
		WalletId        uint32
		ValidUntil      uint32
		Seqno           uint32
		Actions         *wallet.W5Actions         `tlb:"maybe^"`
		ExtendedActions *wallet.W5ExtendedActions `tlb:"maybe"`
	}
)

const codesV5R1 = "te6cckECFAEAAoEAART/APSkE/S88sgLAQIBIAINAgFIAwQC3NAg10nBIJFbj2Mg1wsfIIIQZXh0br0hghBzaW50vbCSXwPgghBleHRuuo60gCDXIQHQdNch+kAw+kT4KPpEMFi9kVvg7UTQgQFB1yH0BYMH9A5voTGRMOGAQNchcH/bPOAxINdJgQKAuZEw4HDiEA8CASAFDAIBIAYJAgFuBwgAGa3OdqJoQCDrkOuF/8AAGa8d9qJoQBDrkOuFj8ACAUgKCwAXsyX7UTQcdch1wsfgABGyYvtRNDXCgCAAGb5fD2omhAgKDrkPoCwBAvIOAR4g1wsfghBzaWduuvLgin8PAeaO8O2i7fshgwjXIgKDCNcjIIAg1yHTH9Mf0x/tRNDSANMfINMf0//XCgAK+QFAzPkQmiiUXwrbMeHywIffArNQB7Dy0IRRJbry4IVQNrry4Ib4I7vy0IgikvgA3gGkf8jKAMsfAc8Wye1UIJL4D95w2zzYEAP27aLt+wL0BCFukmwhjkwCIdc5MHCUIccAs44tAdcoIHYeQ2wg10nACPLgkyDXSsAC8uCTINcdBscSwgBSMLDy0InXTNc5MAGk6GwShAe78uCT10rAAPLgk+1V4tIAAcAAkVvg69csCBQgkXCWAdcsCBwS4lIQseMPINdKERITAJYB+kAB+kT4KPpEMFi68uCR7UTQgQFB1xj0BQSdf8jKAEAEgwf0U/Lgi44UA4MH9Fvy4Iwi1woAIW4Bs7Dy0JDiyFADzxYS9ADJ7VQAcjDXLAgkji0h8uCS0gDtRNDSAFETuvLQj1RQMJExnAGBAUDXIdcKAPLgjuLIygBYzxbJ7VST8sCN4gAQk1vbMeHXTNC01sNe"
const defaultMessageLifetime = time.Minute * 3
const walletId = 2147483409

func generateStateInitV5R1(pub string) (*tlb.StateInit, error) {
	publicKeyBytes, err := hex.DecodeString(pub)
	if err != nil {
		return nil, fmt.Errorf("fail to DecodeString for PublicKey, err=%v", err)
	}
	var publicKey tlb.Bits256
	copy(publicKey[:], publicKeyBytes[:])

	data := wallet.DataV5R1{
		IsSignatureAllowed: true,
		Seqno:              0,
		WalletID:           walletId,
		PublicKey:          publicKey,
	}
	dataCell := boc.NewCell()
	if err := tlb.Marshal(dataCell, data); err != nil {
		return nil, err
	}
	bocData, err := base64.StdEncoding.DecodeString(codesV5R1)
	if err != nil {
		return nil, err
	}
	codeCells, err := boc.DeserializeBoc(bocData)
	if err != nil {
		return nil, err
	} else if len(codeCells) != 1 {
		return nil, fmt.Errorf("len(codeCells) != 1")
	}
	state := tlb.StateInit{
		Code: tlb.Maybe[tlb.Ref[boc.Cell]]{Exists: true, Value: tlb.Ref[boc.Cell]{Value: *codeCells[0]}},
		Data: tlb.Maybe[tlb.Ref[boc.Cell]]{Exists: true, Value: tlb.Ref[boc.Cell]{Value: *dataCell}},
	}
	return &state, nil
}

func simpleTransferToInternal(st wallet.SimpleTransfer) (message tlb.Message, mode uint8, err error) {
	info := tlb.CommonMsgInfo{
		SumType: "IntMsgInfo",
	}

	info.IntMsgInfo = &struct {
		IhrDisabled bool
		Bounce      bool
		Bounced     bool
		Src         tlb.MsgAddress
		Dest        tlb.MsgAddress
		Value       tlb.CurrencyCollection
		IhrFee      tlb.Grams
		FwdFee      tlb.Grams
		CreatedLt   uint64
		CreatedAt   uint32
	}{
		IhrDisabled: true,
		Bounce:      st.Bounceable,
		Src:         (*ton.AccountID)(nil).ToMsgAddress(),
		Dest:        st.Address.ToMsgAddress(),
	}
	info.IntMsgInfo.Value.Grams = st.Amount

	intMsg := tlb.Message{
		Info: info,
	}

	if st.Comment != "" {
		body := boc.NewCell()
		if err := tlb.Marshal(body, wallet.TextComment(st.Comment)); err != nil {
			return tlb.Message{}, 0, err
		}
		intMsg.Body.IsRight = true
		intMsg.Body.Value = tlb.Any(*body)
	}
	return intMsg, wallet.DefaultMessageMode, nil
}

func transferMessageToInternal(tm jetton.TransferMessage, jettonWallet string) (tlb.Message, uint8, error) {
	c := boc.NewCell()
	forwardTon := big.NewInt(int64(tm.ForwardTonAmount))
	msgBody := abi.JettonTransferMsgBody{
		QueryId:             uint64(nowFunc().UnixNano()),
		Amount:              tlb.VarUInteger16(*tm.JettonAmount),
		Destination:         tm.Destination.ToMsgAddress(),
		ResponseDestination: tm.ResponseDestination.ToMsgAddress(),
		ForwardTonAmount:    tlb.VarUInteger16(*forwardTon),
	}
	if tm.CustomPayload != nil {
		payload := tlb.Any(*tm.CustomPayload)
		msgBody.CustomPayload = &payload
	}
	if tm.ForwardPayload != nil {
		msgBody.ForwardPayload.IsRight = true
		msgBody.ForwardPayload.Value = abi.JettonPayload{SumType: abi.UnknownJettonOp, Value: tm.ForwardPayload}
	}
	if err := c.WriteUint(0xf8a7ea5, 32); err != nil {
		return tlb.Message{}, 0, err
	}
	if err := tlb.Marshal(c, msgBody); err != nil {
		return tlb.Message{}, 0, err
	}
	jettonWalletAddr, err := tongo.ParseAddress(jettonWallet)
	if err != nil {
		return tlb.Message{}, 0, err
	}

	m := wallet.Message{
		Amount:  tm.AttachedTon,
		Address: jettonWalletAddr.ID,
		Bounce:  true,
		Mode:    wallet.DefaultMessageMode,
		Body:    c,
	}
	return m.ToInternal()
}
