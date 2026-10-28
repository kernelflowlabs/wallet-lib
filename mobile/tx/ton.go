//go:build ton || !custom

package tx

import (
	"strings"

	"github.com/kernelflowlabs/wallet-lib/session"
	"github.com/kernelflowlabs/wallet-lib/signing/ton"
)

type TonTxBuilder struct {
	in  *ton.TxBuilder
	err error
}

func NewTonTxBuilder() *TonTxBuilder {
	return &TonTxBuilder{
		in: ton.NewTxBuilder(&ton.Ingredient{}),
	}
}

func (b *TonTxBuilder) SetTxType(txType string) *TonTxBuilder {
	b.in.Ingredient.TxType = txType
	return b
}

func (b *TonTxBuilder) SetContractAddress(addr string) *TonTxBuilder {
	b.in.Ingredient.ContractAddress = addr
	return b
}

func (b *TonTxBuilder) SetSender(addr string) *TonTxBuilder {
	b.in.Ingredient.Sender = addr
	return b
}

func (b *TonTxBuilder) SetSenderPublicKey(pubKeyHex string) *TonTxBuilder {
	b.in.Ingredient.SenderPublicKey = pubKeyHex
	return b
}

func (b *TonTxBuilder) SetRecipient(addr string) *TonTxBuilder {
	b.in.Ingredient.Recipient = addr
	return b
}

func (b *TonTxBuilder) SetJettonWallet(addr string) *TonTxBuilder {
	b.in.Ingredient.JettonWallet = addr
	return b
}

func (b *TonTxBuilder) SetAmount(amount string) *TonTxBuilder {
	b.in.Ingredient.Amount = amount
	return b
}

func (b *TonTxBuilder) SetNonce(nonce string) *TonTxBuilder {
	b.in.Ingredient.Nonce = nonce
	return b
}

func (b *TonTxBuilder) SetMemo(memo string) *TonTxBuilder {
	b.in.Ingredient.Memo = memo
	return b
}

func (b *TonTxBuilder) SetFee(fee string) *TonTxBuilder {
	b.in.Ingredient.Fee = fee
	return b
}

func (b *TonTxBuilder) Build() error {
	if b.err != nil {
		return b.err
	}
	return b.in.Build()
}

func (b *TonTxBuilder) SigHash() string {
	sh := b.in.GetSigHash()
	if len(sh) == 0 {
		return ""
	}
	return sh[0]
}

func (b *TonTxBuilder) UnsignedHex() string {
	return b.in.GetUnsignedHex()
}

func (b *TonTxBuilder) SignWithSession(handle string) (string, error) {
	var signature string
	err := session.WithKey(handle, ton.SessionChain, func(privateKey []byte, _ string) error {
		var signErr error
		signature, signErr = b.in.Sign(privateKey)
		return signErr
	})
	if err != nil {
		return "", err
	}
	return signature, nil
}

func (b *TonTxBuilder) Sign(privateKey []byte) (string, error) {
	sig, err := b.in.Sign(privateKey)
	if err != nil {
		return "", err
	}
	WipeByte(privateKey)
	return sig, nil
}

func (b *TonTxBuilder) ConcatSignature(signatureHex string) (string, error) {
	return b.in.ConcatSignature(strings.TrimPrefix(signatureHex, "0x"), false)
}

func (b *TonTxBuilder) TxHash() string {
	return b.in.GetTxHash()
}
