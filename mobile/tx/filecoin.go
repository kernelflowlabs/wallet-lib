//go:build filecoin || !custom

package tx

import (
	"strings"

	"github.com/kernelflowlabs/wallet-lib/signing/filecoin"
)

type FilecoinTxBuilder struct {
	in  *filecoin.TxBuilder
	err error
}

func NewFilecoinTxBuilder() *FilecoinTxBuilder {
	return &FilecoinTxBuilder{
		in: filecoin.NewTxBuilder(&filecoin.Ingredient{GasModel: &filecoin.GasModel{}}),
	}
}

func (b *FilecoinTxBuilder) SetTxType(v string) *FilecoinTxBuilder {
	b.in.Ingredient.TxType = v
	return b
}
func (b *FilecoinTxBuilder) SetContractAddress(v string) *FilecoinTxBuilder {
	b.in.Ingredient.ContractAddress = v
	return b
}
func (b *FilecoinTxBuilder) SetSender(v string) *FilecoinTxBuilder {
	b.in.Ingredient.Sender = v
	return b
}
func (b *FilecoinTxBuilder) SetRecipient(v string) *FilecoinTxBuilder {
	b.in.Ingredient.Recipient = v
	return b
}
func (b *FilecoinTxBuilder) SetAmount(v string) *FilecoinTxBuilder {
	b.in.Ingredient.Amount = v
	return b
}
func (b *FilecoinTxBuilder) SetNonce(v string) *FilecoinTxBuilder {
	b.in.Ingredient.Nonce = v
	return b
}
func (b *FilecoinTxBuilder) SetGasLimit(v string) *FilecoinTxBuilder {
	b.in.Ingredient.GasModel.GasLimit = v
	return b
}
func (b *FilecoinTxBuilder) SetGasFeeCap(v string) *FilecoinTxBuilder {
	b.in.Ingredient.GasModel.GasFeeCap = v
	return b
}
func (b *FilecoinTxBuilder) SetGasPremium(v string) *FilecoinTxBuilder {
	b.in.Ingredient.GasModel.GasPremium = v
	return b
}

func (b *FilecoinTxBuilder) Build() error {
	if b.err != nil {
		return b.err
	}
	return b.in.Build()
}

func (b *FilecoinTxBuilder) SigHash() string {
	sh := b.in.GetSigHash()
	if len(sh) == 0 {
		return ""
	}
	return sh[0]
}

func (b *FilecoinTxBuilder) UnsignedHex() string { return b.in.GetUnsignedHex() }

func (b *FilecoinTxBuilder) Sign(privateKey []byte) (string, error) {
	sig, err := b.in.Sign(privateKey)
	if err != nil {
		return "", err
	}
	WipeByte(privateKey)
	return sig, nil
}

func (b *FilecoinTxBuilder) ConcatSignature(signature string) (string, error) {
	return b.in.ConcatSignature(strings.TrimPrefix(signature, "0x"), false)
}

func (b *FilecoinTxBuilder) TxHash() string { return b.in.GetTxHash() }
