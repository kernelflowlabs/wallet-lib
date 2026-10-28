//go:build sui || !custom

package tx

import (
	"github.com/kernelflowlabs/wallet-lib/session"
	"github.com/kernelflowlabs/wallet-lib/signing/sui"
	"strings"
)

type SuiTxBuilder struct {
	in  *sui.TxBuilder
	err error
}

func NewSuiTxBuilder() *SuiTxBuilder {
	return &SuiTxBuilder{
		in: sui.NewTxBuilder(&sui.Ingredient{}),
	}
}

func NewSuiTxBuilderFromUnsignedHex(unsignedHex string) (*SuiTxBuilder, error) {
	txBuilder, err := sui.NewTxBuilderFromUnsignedHex(unsignedHex)
	if err != nil {
		return nil, err
	}
	return &SuiTxBuilder{
		in: txBuilder,
	}, nil
}

func (b *SuiTxBuilder) SetTxType(txType string) *SuiTxBuilder {
	b.in.Ingredient.TxType = txType
	return b
}

func (b *SuiTxBuilder) SetContractAddress(addr string) *SuiTxBuilder {
	b.in.Ingredient.ContractAddress = addr
	return b
}

func (b *SuiTxBuilder) SetSender(addr string) *SuiTxBuilder {
	b.in.Ingredient.Sender = addr
	return b
}

func (b *SuiTxBuilder) SetRecipient(addr string) *SuiTxBuilder {
	b.in.Ingredient.Recipient = addr
	return b
}

func (b *SuiTxBuilder) SetAmount(amount string) *SuiTxBuilder {
	b.in.Ingredient.Amount = amount
	return b
}

func (b *SuiTxBuilder) SetGasPrice(gasPrice string) *SuiTxBuilder {
	b.in.Ingredient.GasPrice = gasPrice
	return b
}

func (b *SuiTxBuilder) SetGasBudget(gasBudget string) *SuiTxBuilder {
	b.in.Ingredient.GasBudget = gasBudget
	return b
}

func (b *SuiTxBuilder) SetCoins(coinsStr string) *SuiTxBuilder {
	b.in.Ingredient.Coins = coinsStr
	return b
}

func (b *SuiTxBuilder) SetGasCoins(gasCoinsStr string) *SuiTxBuilder {
	b.in.Ingredient.GasCoins = gasCoinsStr
	return b
}

func (b *SuiTxBuilder) Build() error {
	if b.err != nil {
		return b.err
	}
	return b.in.Build()
}

func (b *SuiTxBuilder) SigHash() string {
	sh := b.in.GetSigHash()
	if len(sh) == 0 {
		return ""
	}
	return sh[0]
}

func (b *SuiTxBuilder) UnsignedHex() string {
	return b.in.GetUnsignedHex()
}

func (b *SuiTxBuilder) SignWithSession(handle string) (string, error) {
	var signature string
	err := session.WithKey(handle, sui.SessionChain, func(privateKey []byte, _ string) error {
		var signErr error
		signature, signErr = b.in.Sign(privateKey)
		return signErr
	})
	if err != nil {
		return "", err
	}
	return signature, nil
}

func (b *SuiTxBuilder) Sign(privateKey []byte) (string, error) {
	sig, err := b.in.Sign(privateKey)
	if err != nil {
		return "", err
	}
	WipeByte(privateKey)
	return sig, nil
}

func (b *SuiTxBuilder) ConcatSignature(signatureHex string) (string, error) {
	return b.in.ConcatSignature(strings.TrimPrefix(signatureHex, "0x"), false)
}

func (b *SuiTxBuilder) TxHash() string {
	return b.in.GetTxHash()
}
