//go:build substrate || !custom

package tx

import (
	"encoding/json"
	"fmt"
	"github.com/kernelflowlabs/wallet-lib/session"
	"github.com/kernelflowlabs/wallet-lib/signing/substrate"
	"strings"
)

type SubstrateTxBuilder struct {
	in  *substrate.TxBuilder
	err error
	// network decides which session chain holds this builder's key: one
	// package serves several chains here, and they do not share an address.
	network string
}

func NewSubstrateTxBuilder(network string) *SubstrateTxBuilder {
	return &SubstrateTxBuilder{
		in:      substrate.NewTxBuilder(&substrate.Ingredient{}, network),
		network: network,
	}
}

func (b *SubstrateTxBuilder) SetTxType(txType string) *SubstrateTxBuilder {
	b.in.Ingredient.TxType = txType
	return b
}

func (b *SubstrateTxBuilder) SetContractAddress(addr string) *SubstrateTxBuilder {
	b.in.Ingredient.ContractAddress = addr
	return b
}

func (b *SubstrateTxBuilder) SetSender(addr string) *SubstrateTxBuilder {
	b.in.Ingredient.Sender = addr
	return b
}

func (b *SubstrateTxBuilder) SetRecipient(addr string) *SubstrateTxBuilder {
	b.in.Ingredient.Recipient = addr
	return b
}

func (b *SubstrateTxBuilder) SetAmount(amount string) *SubstrateTxBuilder {
	b.in.Ingredient.Amount = amount
	return b
}

func (b *SubstrateTxBuilder) SetNonce(nonce string) *SubstrateTxBuilder {
	b.in.Ingredient.Nonce = nonce
	return b
}

func (b *SubstrateTxBuilder) SetFee(fee string) *SubstrateTxBuilder {
	b.in.Ingredient.Fee = fee
	return b
}

func (b *SubstrateTxBuilder) SetChainInfo(chainInfoJSON string) *SubstrateTxBuilder {
	var ci substrate.ChainInfo
	if err := json.Unmarshal([]byte(chainInfoJSON), &ci); err != nil {
		b.err = err
		return b
	}
	b.in.Ingredient.ChainInfo = &ci
	return b
}

func (b *SubstrateTxBuilder) Build() error {
	if b.err != nil {
		return b.err
	}
	return b.in.Build()
}

func (b *SubstrateTxBuilder) SigHash() string {
	sh := b.in.GetSigHash()
	if len(sh) == 0 {
		return ""
	}
	return sh[0]
}

func (b *SubstrateTxBuilder) UnsignedHex() string {
	return b.in.GetUnsignedHex()
}

func (b *SubstrateTxBuilder) SignWithSession(handle string) (string, error) {
	chain, ok := substrate.SessionChainForNetwork(b.network)
	if !ok {
		return "", fmt.Errorf("invalid_input: no session chain for substrate network %q", b.network)
	}
	var signature string
	err := session.WithKey(handle, chain, func(privateKey []byte, _ string) error {
		var signErr error
		signature, signErr = b.in.Sign(privateKey)
		return signErr
	})
	if err != nil {
		return "", err
	}
	return signature, nil
}

func (b *SubstrateTxBuilder) Sign(privateKey []byte) (string, error) {
	sig, err := b.in.Sign(privateKey)
	if err != nil {
		return "", err
	}
	WipeByte(privateKey)
	return sig, nil
}

func (b *SubstrateTxBuilder) ConcatSignature(signatureHex string) (string, error) {
	return b.in.ConcatSignature(strings.TrimPrefix(signatureHex, "0x"), false)
}

func (b *SubstrateTxBuilder) TxHash() string {
	return b.in.GetTxHash()
}
