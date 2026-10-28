//go:build evm || !custom

package tx

import (
	"github.com/kernelflowlabs/wallet-lib/session"
	"github.com/kernelflowlabs/wallet-lib/signing/evm"
	"strings"
)

type EvmTxBuilder struct {
	in  *evm.TxBuilder
	err error
}

func NewEvmTxBuilder(network string) *EvmTxBuilder {
	return &EvmTxBuilder{
		in: evm.NewTxBuilder(&evm.Ingredient{}, network),
	}
}

func (b *EvmTxBuilder) SetTxType(txType string) *EvmTxBuilder {
	b.in.Ingredient.TxType = txType
	return b
}

func (b *EvmTxBuilder) SetContractAddress(addr string) *EvmTxBuilder {
	b.in.Ingredient.ContractAddress = addr
	return b
}

func (b *EvmTxBuilder) SetSender(addr string) *EvmTxBuilder {
	b.in.Ingredient.Sender = addr
	return b
}

func (b *EvmTxBuilder) SetRecipient(addr string) *EvmTxBuilder {
	b.in.Ingredient.Recipient = addr
	return b
}

func (b *EvmTxBuilder) SetAmount(amount string) *EvmTxBuilder {
	b.in.Ingredient.Amount = amount
	return b
}

func (b *EvmTxBuilder) SetPayloadHex(hexPayload string) *EvmTxBuilder {
	b.in.Ingredient.Payload = strings.TrimPrefix(hexPayload, "0x")
	return b
}

func (b *EvmTxBuilder) SetMemoHex(hexMemo string) *EvmTxBuilder {
	b.in.Ingredient.Memo = strings.TrimPrefix(hexMemo, "0x")
	return b
}

func (b *EvmTxBuilder) SetNonce(nonceDec string) *EvmTxBuilder {
	b.in.Ingredient.Nonce = nonceDec
	return b
}

func (b *EvmTxBuilder) SetGasPriceWei(gasPriceDec string) *EvmTxBuilder {
	b.in.Ingredient.GasPrice = gasPriceDec
	return b
}

func (b *EvmTxBuilder) SetGasLimit(gasLimitDec string) *EvmTxBuilder {
	b.in.Ingredient.GasLimit = gasLimitDec
	return b
}

func (b *EvmTxBuilder) AsLegacyTx() *EvmTxBuilder {
	b.in.Ingredient.IsLegacyTx = "true"
	return b
}

func (b *EvmTxBuilder) AsEIP1559(gasFeeCapWei, gasTipCapWei string) *EvmTxBuilder {
	b.in.Ingredient.IsLegacyTx = "false"
	b.in.Ingredient.GasFeeCap = gasFeeCapWei
	b.in.Ingredient.GasTipCap = gasTipCapWei
	return b
}

func (b *EvmTxBuilder) Build() error {
	if b.err != nil {
		return b.err
	}
	return b.in.Build()
}

func (b *EvmTxBuilder) SigHash() string {
	sh := b.in.GetSigHash()
	if len(sh) == 0 {
		return ""
	}
	return sh[0]
}

func (b *EvmTxBuilder) UnsignedHex() string {
	return b.in.GetUnsignedHex()
}

func (b *EvmTxBuilder) Sign(privateKey []byte) (string, error) {
	sig, err := b.in.Sign(privateKey)
	if err != nil {
		return "", err
	}
	WipeByte(privateKey)
	return sig, nil
}

func (b *EvmTxBuilder) SignWithSession(handle string) (string, error) {
	var signature string
	err := session.WithKey(handle, session.ChainEVM, func(privateKey []byte, _ string) error {
		var signErr error
		signature, signErr = b.in.Sign(privateKey)
		return signErr
	})
	if err != nil {
		return "", err
	}
	return signature, nil
}

func (b *EvmTxBuilder) ConcatSignature(signatureHex string) (string, error) {
	return b.in.ConcatSignature(strings.TrimPrefix(signatureHex, "0x"), false)
}

func (b *EvmTxBuilder) TxHash() string {
	return b.in.GetTxHash()
}
