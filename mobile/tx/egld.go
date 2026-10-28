//go:build egld || !custom

package tx

import (
	"github.com/kernelflowlabs/wallet-lib/session"
	"github.com/kernelflowlabs/wallet-lib/signing/multiversx"
	"strings"
)

type EgldTxBuilder struct {
	in  *multiversx.TxBuilder
	err error
}

func NewEgldTxBuilder() *EgldTxBuilder {
	return &EgldTxBuilder{
		in: multiversx.NewTxBuilder(&multiversx.Ingredient{}),
	}
}

func (b *EgldTxBuilder) SetTxType(txType string) *EgldTxBuilder {
	b.in.Ingredient.TxType = txType
	return b
}

func (b *EgldTxBuilder) SetContractAddress(addr string) *EgldTxBuilder {
	b.in.Ingredient.ContractAddress = addr
	return b
}

func (b *EgldTxBuilder) SetSender(addr string) *EgldTxBuilder {
	b.in.Ingredient.Sender = addr
	return b
}

func (b *EgldTxBuilder) SetRecipient(addr string) *EgldTxBuilder {
	b.in.Ingredient.Recipient = addr
	return b
}

func (b *EgldTxBuilder) SetAmount(amount string) *EgldTxBuilder {
	b.in.Ingredient.Amount = amount
	return b
}

func (b *EgldTxBuilder) SetNonce(nonceDec string) *EgldTxBuilder {
	b.in.Ingredient.Nonce = nonceDec
	return b
}

func (b *EgldTxBuilder) SetGasPrice(gasPrice string) *EgldTxBuilder {
	if b.in.Ingredient.NetWorkConfig == nil {
		b.in.Ingredient.NetWorkConfig = &multiversx.NetWorkConfig{}
	}
	b.in.Ingredient.NetWorkConfig.GasPrice = gasPrice
	return b
}

func (b *EgldTxBuilder) SetGasLimit(gasLimit string) *EgldTxBuilder {
	if b.in.Ingredient.NetWorkConfig == nil {
		b.in.Ingredient.NetWorkConfig = &multiversx.NetWorkConfig{}
	}
	b.in.Ingredient.NetWorkConfig.GasLimit = gasLimit
	return b
}

func (b *EgldTxBuilder) SetChainId(chainId string) *EgldTxBuilder {
	if b.in.Ingredient.NetWorkConfig == nil {
		b.in.Ingredient.NetWorkConfig = &multiversx.NetWorkConfig{}
	}
	b.in.Ingredient.NetWorkConfig.ChainID = chainId
	return b
}

func (b *EgldTxBuilder) SetVersion(version string) *EgldTxBuilder {
	if b.in.Ingredient.NetWorkConfig == nil {
		b.in.Ingredient.NetWorkConfig = &multiversx.NetWorkConfig{}
	}
	b.in.Ingredient.NetWorkConfig.Version = version
	return b
}

func (b *EgldTxBuilder) Build() error {
	if b.err != nil {
		return b.err
	}
	return b.in.Build()
}

func (b *EgldTxBuilder) SigHash() string {
	sh := b.in.GetSigHash()
	if len(sh) == 0 {
		return ""
	}
	return sh[0]
}

func (b *EgldTxBuilder) UnsignedHex() string {
	return b.in.GetUnsignedHex()
}

func (b *EgldTxBuilder) SignWithSession(handle string) (string, error) {
	var signature string
	err := session.WithKey(handle, multiversx.SessionChain, func(privateKey []byte, _ string) error {
		var signErr error
		signature, signErr = b.in.Sign(privateKey)
		return signErr
	})
	if err != nil {
		return "", err
	}
	return signature, nil
}

func (b *EgldTxBuilder) Sign(privateKey []byte) (string, error) {
	sig, err := b.in.Sign(privateKey)
	if err != nil {
		return "", err
	}
	WipeByte(privateKey)
	return sig, nil
}

func (b *EgldTxBuilder) ConcatSignature(signatureHex string) (string, error) {
	return b.in.ConcatSignature(strings.TrimPrefix(signatureHex, "0x"), false)
}

func (b *EgldTxBuilder) TxHash() string {
	return b.in.GetTxHash()
}
