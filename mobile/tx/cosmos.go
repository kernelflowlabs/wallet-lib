//go:build cosmos || !custom

package tx

import (
	"strings"

	"github.com/kernelflowlabs/wallet-lib/signing/cosmos"
)

type CosmosTxBuilder struct {
	in  *cosmos.TxBuilder
	err error
}

func NewCosmosTxBuilder(network string) *CosmosTxBuilder {
	return &CosmosTxBuilder{
		in: cosmos.NewTxBuilder(&cosmos.Ingredient{AccountInfo: &cosmos.AccountInfo{}}, network),
	}
}

func (b *CosmosTxBuilder) SetTxType(txType string) *CosmosTxBuilder {
	b.in.Ingredient.TxType = txType
	return b
}

func (b *CosmosTxBuilder) SetContractAddress(addr string) *CosmosTxBuilder {
	b.in.Ingredient.ContractAddress = addr
	return b
}

func (b *CosmosTxBuilder) SetSender(addr string) *CosmosTxBuilder {
	b.in.Ingredient.Sender = addr
	return b
}

func (b *CosmosTxBuilder) SetSenderPublicKey(pubKeyHex string) *CosmosTxBuilder {
	b.in.Ingredient.SenderPublicKey = pubKeyHex
	return b
}

func (b *CosmosTxBuilder) SetRecipient(addr string) *CosmosTxBuilder {
	b.in.Ingredient.Recipient = addr
	return b
}

func (b *CosmosTxBuilder) SetAmount(amount string) *CosmosTxBuilder {
	b.in.Ingredient.Amount = amount
	return b
}

func (b *CosmosTxBuilder) SetFeeAmount(fee string) *CosmosTxBuilder {
	b.in.Ingredient.FeeAmount = fee
	return b
}

func (b *CosmosTxBuilder) SetGasLimit(gasLimit string) *CosmosTxBuilder {
	b.in.Ingredient.GasLimit = gasLimit
	return b
}

func (b *CosmosTxBuilder) SetMemo(memo string) *CosmosTxBuilder {
	b.in.Ingredient.Memo = memo
	return b
}

func (b *CosmosTxBuilder) SetAccountNumber(accountNumber string) *CosmosTxBuilder {
	b.in.Ingredient.AccountInfo.AccountNumber = accountNumber
	return b
}

func (b *CosmosTxBuilder) SetSequence(sequence string) *CosmosTxBuilder {
	b.in.Ingredient.AccountInfo.Sequence = sequence
	return b
}

func (b *CosmosTxBuilder) Build() error {
	if b.err != nil {
		return b.err
	}
	return b.in.Build()
}

func (b *CosmosTxBuilder) SigHash() string {
	sh := b.in.GetSigHash()
	if len(sh) == 0 {
		return ""
	}
	return sh[0]
}

func (b *CosmosTxBuilder) UnsignedHex() string {
	return b.in.GetUnsignedHex()
}

func (b *CosmosTxBuilder) Sign(privateKey []byte) (string, error) {
	sig, err := b.in.Sign(privateKey)
	if err != nil {
		return "", err
	}
	WipeByte(privateKey)
	return sig, nil
}

func (b *CosmosTxBuilder) ConcatSignature(signatureHex string) (string, error) {
	return b.in.ConcatSignature(strings.TrimPrefix(signatureHex, "0x"), false)
}

func (b *CosmosTxBuilder) TxHash() string {
	return b.in.GetTxHash()
}
