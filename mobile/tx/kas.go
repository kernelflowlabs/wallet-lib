//go:build kas || !custom

package tx

import (
	"github.com/kernelflowlabs/wallet-lib/session"
	"github.com/kernelflowlabs/wallet-lib/signing"
	"github.com/kernelflowlabs/wallet-lib/signing/kaspa"
	"strings"
)

type KasTxBuilder struct {
	in  *kaspa.TxBuilder
	err error
}

func NewKasTxBuilder() *KasTxBuilder {
	return &KasTxBuilder{
		in: kaspa.NewTxBuilder(&kaspa.Ingredient{}),
	}
}

func (b *KasTxBuilder) SetTxType(txType string) *KasTxBuilder {
	b.in.Ingredient.TxType = txType
	return b
}

func (b *KasTxBuilder) SetContractAddress(addr string) *KasTxBuilder {
	b.in.Ingredient.ContractAddress = addr
	return b
}

func (b *KasTxBuilder) SetSender(addr string) *KasTxBuilder {
	b.in.Ingredient.Sender = addr
	return b
}

func (b *KasTxBuilder) SetRecipient(addr string) *KasTxBuilder {
	b.in.Ingredient.Recipient = addr
	return b
}

func (b *KasTxBuilder) SetAmount(amount string) *KasTxBuilder {
	b.in.Ingredient.Amount = amount
	return b
}

func (b *KasTxBuilder) SetFee(fee string) *KasTxBuilder {
	b.in.Ingredient.Fee = fee
	return b
}

func (b *KasTxBuilder) SetUtxos(utxos *MobileUtxoList) *KasTxBuilder {
	b.in.Ingredient.Utxos = (*signing.UtxoList)(utxos)
	return b
}

func (b *KasTxBuilder) SetKrc20RedeemScript(krc20RedeemScript string) *KasTxBuilder {
	b.in.Ingredient.Krc20RedeemScript = krc20RedeemScript
	return b
}

func (b *KasTxBuilder) Build() error {
	if b.err != nil {
		return b.err
	}
	return b.in.Build()
}

func (b *KasTxBuilder) SigHash() string {
	sh := b.in.GetSigHash()
	if len(sh) == 0 {
		return ""
	}
	return sh[0]
}

func (b *KasTxBuilder) UnsignedHex() string {
	return b.in.GetUnsignedHex()
}

func (b *KasTxBuilder) SignWithSession(handle string) (string, error) {
	var signature string
	err := session.WithKeyFor(handle, kaspa.SessionChain, b.in.Ingredient.Sender, func(privateKey []byte, _ string) error {
		var signErr error
		signature, signErr = b.in.Sign(privateKey)
		return signErr
	})
	if err != nil {
		return "", err
	}
	return signature, nil
}

func (b *KasTxBuilder) Sign(privateKey []byte) (string, error) {
	sig, err := b.in.Sign(privateKey)
	if err != nil {
		return "", err
	}
	WipeByte(privateKey)
	return sig, nil
}

func (b *KasTxBuilder) ConcatSignature(signatureHex string) (string, error) {
	return b.in.ConcatSignature(strings.TrimPrefix(signatureHex, "0x"), false)
}

func (b *KasTxBuilder) TxHash() string {
	return b.in.GetTxHash()
}

func GenerateKrc20Params(in *MobileKrc20Params, pubKey []byte) (string, error) {
	return kaspa.GetKrc20Params((*kaspa.Krc20Params)(in), pubKey)
}

type MobileKrc20Params kaspa.Krc20Params
