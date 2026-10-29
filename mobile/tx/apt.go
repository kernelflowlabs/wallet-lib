//go:build apt || !custom

package tx

import (
	"github.com/kernelflowlabs/wallet-lib/session"
	"github.com/kernelflowlabs/wallet-lib/signing/aptos"
	"strings"
)

type AptTxBuilder struct {
	in  *aptos.TxBuilder
	err error
}

func NewAptTxBuilder() *AptTxBuilder {
	return &AptTxBuilder{
		in: aptos.NewTxBuilder(&aptos.Ingredient{}),
	}
}

func (b *AptTxBuilder) SetTxType(txType string) *AptTxBuilder {
	b.in.Ingredient.TxType = txType
	return b
}

func (b *AptTxBuilder) SetContractAddress(addr string) *AptTxBuilder {
	b.in.Ingredient.ContractAddress = addr
	return b
}

func (b *AptTxBuilder) SetSender(addr string) *AptTxBuilder {
	b.in.Ingredient.Sender = addr
	return b
}

func (b *AptTxBuilder) SetSenderPubKey(pubKey string) *AptTxBuilder {
	b.in.Ingredient.SenderPublicKey = pubKey
	return b
}

func (b *AptTxBuilder) SetRecipient(addr string) *AptTxBuilder {
	b.in.Ingredient.Recipient = addr
	return b
}

func (b *AptTxBuilder) SetAmount(amount string) *AptTxBuilder {
	b.in.Ingredient.Amount = amount
	return b
}

func (b *AptTxBuilder) SetNonce(nonceDec string) *AptTxBuilder {
	b.in.Ingredient.Nonce = nonceDec
	return b
}

func (b *AptTxBuilder) SetGasPrice(gasPrice string) *AptTxBuilder {
	b.in.Ingredient.GasPrice = gasPrice
	return b
}

func (b *AptTxBuilder) SetGasLimit(gasLimitDec string) *AptTxBuilder {
	b.in.Ingredient.GasLimit = gasLimitDec
	return b
}

func (b *AptTxBuilder) SetChainId(chainId string) *AptTxBuilder {
	if b.in.Ingredient.LedgerInfoParams == nil {
		b.in.Ingredient.LedgerInfoParams = &aptos.LedgerInfoParams{}
	}
	b.in.Ingredient.LedgerInfoParams.ChainId = chainId
	return b
}

func (b *AptTxBuilder) SetExpirationTimestamp(expirationTimestamp string) *AptTxBuilder {
	if b.in.Ingredient.LedgerInfoParams == nil {
		b.in.Ingredient.LedgerInfoParams = &aptos.LedgerInfoParams{}
	}
	b.in.Ingredient.LedgerInfoParams.ExpirationTimestamp = expirationTimestamp
	return b
}

func (b *AptTxBuilder) Build() error {
	if b.err != nil {
		return b.err
	}
	return b.in.Build()
}

func (b *AptTxBuilder) SigHash() string {
	sh := b.in.GetSigHash()
	if len(sh) == 0 {
		return ""
	}
	return sh[0]
}

func (b *AptTxBuilder) UnsignedHex() string {
	return b.in.GetUnsignedHex()
}

func (b *AptTxBuilder) SignWithSession(handle string) (string, error) {
	var signature string
	err := session.WithKeyFor(handle, aptos.SessionChain, b.in.Ingredient.Sender, func(privateKey []byte, _ string) error {
		var signErr error
		signature, signErr = b.in.Sign(privateKey)
		return signErr
	})
	if err != nil {
		return "", err
	}
	return signature, nil
}

func (b *AptTxBuilder) Sign(privateKey []byte) (string, error) {
	sig, err := b.in.Sign(privateKey)
	if err != nil {
		return "", err
	}
	WipeByte(privateKey)
	return sig, nil
}

func (b *AptTxBuilder) ConcatSignature(signatureHex string) (string, error) {
	return b.in.ConcatSignature(strings.TrimPrefix(signatureHex, "0x"), false)
}

func (b *AptTxBuilder) TxHash() string {
	return b.in.GetTxHash()
}
