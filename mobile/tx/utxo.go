//go:build utxo || !custom

package tx

import (
	"fmt"
	"github.com/kernelflowlabs/wallet-lib/session"
	"github.com/kernelflowlabs/wallet-lib/signing"
	"github.com/kernelflowlabs/wallet-lib/signing/utxo"
	"strings"
)

type UtxoTxBuilder struct {
	in  *utxo.TxBuilder
	err error
	// network decides which session chain holds this builder's key: one
	// package serves several chains here, and they do not share an address.
	network string
}

func NewUtxoTxBuilder(network string) *UtxoTxBuilder {
	return &UtxoTxBuilder{
		in:      utxo.NewTxBuilder(&utxo.Ingredient{}, network),
		network: network,
	}
}

func NewUtxoTxBuilderFromUnsignedHex(unsignedHex string, publicKey string, network string) (*UtxoTxBuilder, error) {
	txBuilder, err := utxo.NewTxBuilderFromUnsignedHex(unsignedHex, publicKey, network)
	if err != nil {
		return nil, err
	}
	return &UtxoTxBuilder{
		in:      txBuilder,
		network: network,
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

func (b *UtxoTxBuilder) SignWithSession(handle string) (string, error) {
	chain, ok := utxo.SessionChainForNetwork(b.network)
	if !ok {
		return "", fmt.Errorf("invalid_input: no session chain for utxo network %q", b.network)
	}
	var signature string
	err := session.WithKeyFor(handle, chain, b.in.Ingredient.Sender, func(privateKey []byte, _ string) error {
		var signErr error
		signature, signErr = b.in.Sign(privateKey)
		return signErr
	})
	if err != nil {
		return "", err
	}
	return signature, nil
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
