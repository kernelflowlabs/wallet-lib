//go:build stellar || !custom

package tx

import (
	"strings"

	"github.com/kernelflowlabs/wallet-lib/signing/stellar"
)

type StellarTxBuilder struct {
	in  *stellar.TxBuilder
	err error
}

func NewStellarTxBuilder() *StellarTxBuilder {
	return &StellarTxBuilder{
		in: stellar.NewTxBuilder(&stellar.Ingredient{}),
	}
}

func (b *StellarTxBuilder) SetTxType(txType string) *StellarTxBuilder {
	b.in.Ingredient.TxType = txType
	return b
}

func (b *StellarTxBuilder) SetContractAddress(addr string) *StellarTxBuilder {
	b.in.Ingredient.ContractAddress = addr
	return b
}

func (b *StellarTxBuilder) SetSender(addr string) *StellarTxBuilder {
	b.in.Ingredient.Sender = addr
	return b
}

func (b *StellarTxBuilder) SetRecipient(addr string) *StellarTxBuilder {
	b.in.Ingredient.Recipient = addr
	return b
}

func (b *StellarTxBuilder) SetAmount(amount string) *StellarTxBuilder {
	b.in.Ingredient.Amount = amount
	return b
}

func (b *StellarTxBuilder) SetMemo(memo string) *StellarTxBuilder {
	b.in.Ingredient.Memo = memo
	return b
}

func (b *StellarTxBuilder) SetSequence(sequence string) *StellarTxBuilder {
	b.in.Ingredient.Sequence = sequence
	return b
}

func (b *StellarTxBuilder) SetIsRecipientActivated(activated string) *StellarTxBuilder {
	b.in.Ingredient.IsRecipientActivated = activated
	return b
}

func (b *StellarTxBuilder) Build() error {
	if b.err != nil {
		return b.err
	}
	return b.in.Build()
}

func (b *StellarTxBuilder) SigHash() string {
	sh := b.in.GetSigHash()
	if len(sh) == 0 {
		return ""
	}
	return sh[0]
}

func (b *StellarTxBuilder) UnsignedHex() string {
	return b.in.GetUnsignedHex()
}

func (b *StellarTxBuilder) Sign(privateKey []byte) (string, error) {
	sig, err := b.in.Sign(privateKey)
	if err != nil {
		return "", err
	}
	WipeByte(privateKey)
	return sig, nil
}

func (b *StellarTxBuilder) ConcatSignature(signature string) (string, error) {
	return b.in.ConcatSignature(strings.TrimPrefix(signature, "0x"), false)
}

func (b *StellarTxBuilder) TxHash() string {
	return b.in.GetTxHash()
}
