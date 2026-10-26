//go:build utxo || !custom

package tx

import (
	"github.com/kernelflowlabs/wallet-lib/signing"
	"github.com/kernelflowlabs/wallet-lib/signing/utxo"
	"strings"
)

type UtxoTxBuilder struct {
	in  *utxo.TxBuilder
	err error
}

func NewUtxoTxBuilder(network string) *UtxoTxBuilder {
	return &UtxoTxBuilder{
		in: utxo.NewTxBuilder(&utxo.Ingredient{}, network),
	}
}

func NewUtxoTxBuilderFromUnsignedHex(unsignedHex string, publicKey string, network string) (*UtxoTxBuilder, error) {
	txBuilder, err := utxo.NewTxBuilderFromUnsignedHex(unsignedHex, publicKey, network)
	if err != nil {
		return nil, err
	}
	return &UtxoTxBuilder{
		in: txBuilder,
	}, nil
}

func (b *UtxoTxBuilder) SetTxType(txType string) *UtxoTxBuilder {
	b.in.Ingredient.TxType = txType
	return b
}

func (b *UtxoTxBuilder) SetContractAddress(addr string) *UtxoTxBuilder {
	b.in.Ingredient.ContractAddress = addr
	return b
}

func (b *UtxoTxBuilder) SetSender(addr string) *UtxoTxBuilder {
	b.in.Ingredient.Sender = addr
	return b
}

func (b *UtxoTxBuilder) SetSenderPublicKey(pubKey string) *UtxoTxBuilder {
	b.in.Ingredient.SenderPublicKey = pubKey
	return b
}

func (b *UtxoTxBuilder) SetRecipient(addr string) *UtxoTxBuilder {
	b.in.Ingredient.Recipient = addr
	return b
}

func (b *UtxoTxBuilder) SetAmount(amount string) *UtxoTxBuilder {
	b.in.Ingredient.Amount = amount
	return b
}

func (b *UtxoTxBuilder) SetByteFee(feeRate string) *UtxoTxBuilder {
	b.in.Ingredient.ByteFee = feeRate
	return b
}

func (b *UtxoTxBuilder) SetUtxos(utxos *MobileUtxoList) *UtxoTxBuilder {
	b.in.Ingredient.Utxos = (*signing.UtxoList)(utxos)
	return b
}

func (b *UtxoTxBuilder) Build() error {
	if b.err != nil {
		return b.err
	}
	return b.in.Build()
}

func (b *UtxoTxBuilder) SigHash() string {
	sh := b.in.GetSigHash()
	if len(sh) == 0 {
		return ""
	}
	return sh[0]
}

func (b *UtxoTxBuilder) UnsignedHex() string {
	return b.in.GetUnsignedHex()
}

func (b *UtxoTxBuilder) Sign(privateKey []byte) (string, error) {
	sig, err := b.in.Sign(privateKey)
	if err != nil {
		return "", err
	}
	WipeByte(privateKey)
	return sig, nil
}

func (b *UtxoTxBuilder) ConcatSignature(signatureHex string) (string, error) {
	return b.in.ConcatSignature(strings.TrimPrefix(signatureHex, "0x"), false)
}

func (b *UtxoTxBuilder) TxHash() string {
	return b.in.GetTxHash()
}
