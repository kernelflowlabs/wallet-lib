//go:build trx || !custom

package tx

import (
	"github.com/kernelflowlabs/wallet-lib/session"
	"github.com/kernelflowlabs/wallet-lib/signing/tron"
	"strings"
)

type TrxTxBuilder struct {
	in  *tron.TxBuilder
	err error
}

func NewTrxTxBuilder() *TrxTxBuilder {
	return &TrxTxBuilder{
		in: tron.NewTxBuilder(&tron.Ingredient{}),
	}
}

func NewTrxTxBuilderFromUnsignedHex(unsignedHex string, sender string) (*TrxTxBuilder, error) {
	txBuilder, err := tron.NewTxBuilderFromUnsignedHex(unsignedHex, sender)
	if err != nil {
		return nil, err
	}
	return &TrxTxBuilder{
		in: txBuilder,
	}, nil
}

func (b *TrxTxBuilder) SetTxType(txType string) *TrxTxBuilder {
	b.in.Ingredient.TxType = txType
	return b
}

func (b *TrxTxBuilder) SetContractAddress(addr string) *TrxTxBuilder {
	b.in.Ingredient.ContractAddress = addr
	return b
}

func (b *TrxTxBuilder) SetSender(addr string) *TrxTxBuilder {
	b.in.Ingredient.Sender = addr
	return b
}

func (b *TrxTxBuilder) SetRecipient(addr string) *TrxTxBuilder {
	b.in.Ingredient.Recipient = addr
	return b
}

func (b *TrxTxBuilder) SetAmount(amount string) *TrxTxBuilder {
	b.in.Ingredient.Amount = amount
	return b
}

func (b *TrxTxBuilder) SetFeeLimit(fee string) *TrxTxBuilder {
	b.in.Ingredient.FeeLimit = fee
	return b
}

func (b *TrxTxBuilder) SetPayloadHex(payload string) *TrxTxBuilder {
	b.in.Ingredient.Payload = strings.TrimPrefix(payload, "0x")
	return b
}

func (b *TrxTxBuilder) SetRefBlockHash(blockHash string) *TrxTxBuilder {
	b.in.Ingredient.RefBlockHash = blockHash
	return b
}

func (b *TrxTxBuilder) SetRefBlockNumber(blockNumber string) *TrxTxBuilder {
	b.in.Ingredient.RefBlockNumber = blockNumber
	return b
}

func (b *TrxTxBuilder) SetRefBlockTime(blockTime string) *TrxTxBuilder {
	b.in.Ingredient.RefBlockTimestamp = blockTime
	return b
}

func (b *TrxTxBuilder) Build() error {
	if b.err != nil {
		return b.err
	}
	return b.in.Build()
}

func (b *TrxTxBuilder) SigHash() string {
	sh := b.in.GetSigHash()
	if len(sh) == 0 {
		return ""
	}
	return sh[0]
}

func (b *TrxTxBuilder) UnsignedHex() string {
	return b.in.GetUnsignedHex()
}

func (b *TrxTxBuilder) Sign(privateKey []byte) (string, error) {
	sig, err := b.in.Sign(privateKey)
	if err != nil {
		return "", err
	}
	WipeByte(privateKey)
	return sig, nil
}

func (b *TrxTxBuilder) SignWithSession(handle string) (string, error) {
	var signature string
	err := session.WithKey(handle, session.ChainTRX, func(privateKey []byte, _ string) error {
		var signErr error
		signature, signErr = b.in.Sign(privateKey)
		return signErr
	})
	if err != nil {
		return "", err
	}
	return signature, nil
}

func (b *TrxTxBuilder) ConcatSignature(signatureHex string) (string, error) {
	return b.in.ConcatSignature(strings.TrimPrefix(signatureHex, "0x"), false)
}

func (b *TrxTxBuilder) TxHash() string {
	return b.in.GetTxHash()
}
