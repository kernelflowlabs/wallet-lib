//go:build near || !custom

package tx

import (
	"strings"

	"github.com/kernelflowlabs/wallet-lib/session"
	"github.com/kernelflowlabs/wallet-lib/signing/near"
)

type NearTxBuilder struct {
	in  *near.TxBuilder
	err error
}

func NewNearTxBuilder() *NearTxBuilder {
	return &NearTxBuilder{
		in: near.NewTxBuilder(&near.Ingredient{}),
	}
}

func (b *NearTxBuilder) SetTxType(txType string) *NearTxBuilder {
	b.in.Ingredient.TxType = txType
	return b
}

func (b *NearTxBuilder) SetContractAddress(addr string) *NearTxBuilder {
	b.in.Ingredient.ContractAddress = addr
	return b
}

func (b *NearTxBuilder) SetSender(addr string) *NearTxBuilder {
	b.in.Ingredient.Sender = addr
	return b
}

func (b *NearTxBuilder) SetSenderPublicKey(pubKeyHex string) *NearTxBuilder {
	b.in.Ingredient.SenderPublicKey = pubKeyHex
	return b
}

func (b *NearTxBuilder) SetRecipient(addr string) *NearTxBuilder {
	b.in.Ingredient.Recipient = addr
	return b
}

func (b *NearTxBuilder) SetAmount(amount string) *NearTxBuilder {
	b.in.Ingredient.Amount = amount
	return b
}

func (b *NearTxBuilder) SetNonce(nonce string) *NearTxBuilder {
	b.in.Ingredient.Nonce = nonce
	return b
}

func (b *NearTxBuilder) SetRequiredAmount(amount string) *NearTxBuilder {
	b.in.Ingredient.RequiredAmount = amount
	return b
}

func (b *NearTxBuilder) SetBlockHash(blockHash string) *NearTxBuilder {
	b.in.Ingredient.BlockHash = blockHash
	return b
}

func (b *NearTxBuilder) Build() error {
	if b.err != nil {
		return b.err
	}
	return b.in.Build()
}

func (b *NearTxBuilder) SigHash() string {
	sh := b.in.GetSigHash()
	if len(sh) == 0 {
		return ""
	}
	return sh[0]
}

func (b *NearTxBuilder) UnsignedHex() string {
	return b.in.GetUnsignedHex()
}

func (b *NearTxBuilder) SignWithSession(handle string) (string, error) {
	var signature string
	err := session.WithKeyFor(handle, near.SessionChain, b.in.Ingredient.Sender, func(privateKey []byte, _ string) error {
		var signErr error
		signature, signErr = b.in.Sign(privateKey)
		return signErr
	})
	if err != nil {
		return "", err
	}
	return signature, nil
}

func (b *NearTxBuilder) Sign(privateKey []byte) (string, error) {
	sig, err := b.in.Sign(privateKey)
	if err != nil {
		return "", err
	}
	WipeByte(privateKey)
	return sig, nil
}

func (b *NearTxBuilder) ConcatSignature(signature string) (string, error) {
	return b.in.ConcatSignature(strings.TrimPrefix(signature, "0x"), false)
}

func (b *NearTxBuilder) TxHash() string {
	return b.in.GetTxHash()
}
