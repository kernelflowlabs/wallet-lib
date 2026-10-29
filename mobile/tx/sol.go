//go:build sol || !custom

package tx

import (
	"github.com/kernelflowlabs/wallet-lib/session"
	"github.com/kernelflowlabs/wallet-lib/signing/solana"
	"strings"
)

type SolTxBuilder struct {
	in     *solana.TxBuilder
	err    error
	fee    string
	feeSet bool
}

func NewSolTxBuilder() *SolTxBuilder {
	return &SolTxBuilder{
		in: solana.NewTxBuilder(&solana.Ingredient{}),
	}
}

func NewSolTxBuilderFromUnsignedHex(unsignedHex string) (*SolTxBuilder, error) {
	txBuilder, err := solana.NewTxBuilderFromUnsignedHex(unsignedHex)
	if err != nil {
		return nil, err
	}
	return &SolTxBuilder{
		in: txBuilder,
	}, nil
}

func (b *SolTxBuilder) SetTxType(txType string) *SolTxBuilder {
	b.in.Ingredient.TxType = txType
	return b
}

func (b *SolTxBuilder) SetContractAddress(addr string) *SolTxBuilder {
	b.in.Ingredient.ContractAddress = addr
	return b
}

func (b *SolTxBuilder) SetSender(addr string) *SolTxBuilder {
	b.in.Ingredient.Sender = addr
	return b
}

func (b *SolTxBuilder) SetRecipient(addr string) *SolTxBuilder {
	b.in.Ingredient.Recipient = addr
	return b
}

func (b *SolTxBuilder) SetAmount(amount string) *SolTxBuilder {
	b.in.Ingredient.Amount = amount
	return b
}

func (b *SolTxBuilder) SetFee(fee string) *SolTxBuilder {
	b.fee = fee
	b.feeSet = true
	return b
}

func (b *SolTxBuilder) SetRefBlockHash(hash string) *SolTxBuilder {
	b.in.Ingredient.RefBlockHash = hash
	return b
}

func (b *SolTxBuilder) HasATA(has string) *SolTxBuilder {
	b.in.Ingredient.HasATA = has
	return b
}

// SetToken2022 marks the mint as Token-2022 ("true"/"false").
func (b *SolTxBuilder) SetToken2022(is string) *SolTxBuilder {
	b.in.Ingredient.Token2022 = is
	return b
}

func (b *SolTxBuilder) SetDecimals(decimals string) *SolTxBuilder {
	b.in.Ingredient.Decimals = decimals
	return b
}

func (b *SolTxBuilder) Build() error {
	if b.err != nil {
		return b.err
	}
	if b.feeSet {
		isToken := !solana.IsNativeSentinel(b.in.Ingredient.ContractAddress)
		unitPrice, unitLimit := solana.RecommendComputeBudget(b.fee, isToken)
		b.in.Ingredient.UnitPrice = unitPrice
		b.in.Ingredient.UnitLimit = unitLimit
	}
	return b.in.Build()
}

func (b *SolTxBuilder) SigHash() string {
	sh := b.in.GetSigHash()
	if len(sh) == 0 {
		return ""
	}
	return sh[0]
}

func (b *SolTxBuilder) UnsignedHex() string {
	return b.in.GetUnsignedHex()
}

func (b *SolTxBuilder) Sign(privateKey []byte) (string, error) {
	sig, err := b.in.Sign(privateKey)
	if err != nil {
		return "", err
	}
	WipeByte(privateKey)
	return sig, nil
}

func (b *SolTxBuilder) SignWithSession(handle string) (string, error) {
	var signature string
	err := session.WithKeyFor(handle, solana.SessionChain, b.in.Ingredient.Sender, func(privateKey []byte, _ string) error {
		var signErr error
		signature, signErr = b.in.Sign(privateKey)
		return signErr
	})
	if err != nil {
		return "", err
	}
	return signature, nil
}

func (b *SolTxBuilder) ConcatSignature(signatureHex string) (string, error) {
	return b.in.ConcatSignature(strings.TrimPrefix(signatureHex, "0x"), false)
}

func (b *SolTxBuilder) TxHash() string {
	return b.in.GetTxHash()
}
