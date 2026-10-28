//go:build algorand || !custom

package tx

import (
	"strings"

	"github.com/kernelflowlabs/wallet-lib/session"
	"github.com/kernelflowlabs/wallet-lib/signing/algorand"
)

type AlgorandTxBuilder struct {
	in  *algorand.TxBuilder
	err error
}

func NewAlgorandTxBuilder() *AlgorandTxBuilder {
	return &AlgorandTxBuilder{in: algorand.NewTxBuilder(&algorand.Ingredient{})}
}

func (b *AlgorandTxBuilder) SetTxType(v string) *AlgorandTxBuilder {
	b.in.Ingredient.TxType = v
	return b
}
func (b *AlgorandTxBuilder) SetContractAddress(v string) *AlgorandTxBuilder {
	b.in.Ingredient.ContractAddress = v
	return b
}
func (b *AlgorandTxBuilder) SetSender(v string) *AlgorandTxBuilder {
	b.in.Ingredient.Sender = v
	return b
}
func (b *AlgorandTxBuilder) SetRecipient(v string) *AlgorandTxBuilder {
	b.in.Ingredient.Recipient = v
	return b
}
func (b *AlgorandTxBuilder) SetAmount(v string) *AlgorandTxBuilder {
	b.in.Ingredient.Amount = v
	return b
}
func (b *AlgorandTxBuilder) SetFee(v string) *AlgorandTxBuilder { b.in.Ingredient.Fee = v; return b }
func (b *AlgorandTxBuilder) SetGenesisID(v string) *AlgorandTxBuilder {
	b.in.Ingredient.GenesisID = v
	return b
}
func (b *AlgorandTxBuilder) SetGenesisHash(v string) *AlgorandTxBuilder {
	b.in.Ingredient.GenesisHash = v
	return b
}
func (b *AlgorandTxBuilder) SetFirstValid(v string) *AlgorandTxBuilder {
	b.in.Ingredient.FirstValid = v
	return b
}

func (b *AlgorandTxBuilder) Build() error {
	if b.err != nil {
		return b.err
	}
	return b.in.Build()
}

func (b *AlgorandTxBuilder) SigHash() string {
	sh := b.in.GetSigHash()
	if len(sh) == 0 {
		return ""
	}
	return sh[0]
}

func (b *AlgorandTxBuilder) UnsignedHex() string { return b.in.GetUnsignedHex() }

func (b *AlgorandTxBuilder) SignWithSession(handle string) (string, error) {
	var signature string
	err := session.WithKey(handle, algorand.SessionChain, func(privateKey []byte, _ string) error {
		var signErr error
		signature, signErr = b.in.Sign(privateKey)
		return signErr
	})
	if err != nil {
		return "", err
	}
	return signature, nil
}

func (b *AlgorandTxBuilder) Sign(privateKey []byte) (string, error) {
	sig, err := b.in.Sign(privateKey)
	if err != nil {
		return "", err
	}
	WipeByte(privateKey)
	return sig, nil
}

func (b *AlgorandTxBuilder) ConcatSignature(signature string) (string, error) {
	return b.in.ConcatSignature(strings.TrimPrefix(signature, "0x"), false)
}

func (b *AlgorandTxBuilder) TxHash() string { return b.in.GetTxHash() }
