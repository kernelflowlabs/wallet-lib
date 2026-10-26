//go:build evm || !custom

package acc

import (
	"encoding/hex"

	"github.com/kernelflowlabs/wallet-lib/signing"
	"github.com/kernelflowlabs/wallet-lib/signing/evm"
)

type EvmAccount struct {
	in *evm.Account
}

func NewEvmFromMnemonic(mnemonic, path string) (*EvmAccount, error) {
	acc, err := evm.NewAccountFromMnemonic(mnemonic, path)
	if err != nil {
		return nil, err
	}
	return &EvmAccount{in: acc.(*evm.Account)}, nil
}

func (a *EvmAccount) PrivateKey() []byte {
	return a.in.PrivateKey()
}

func (a *EvmAccount) PublicKey() []byte {
	return a.in.PublicKey()
}

func (a *EvmAccount) Address() string {
	return a.in.Address()
}
func init() {
	registerAddressValidator(signing.FamilyOfEVM, func(address, network string) bool {
		return evm.ValidAddress(address)
	})
}

func (a *EvmAccount) SignTypedDataJSON(typedDataJSON string) (string, error) {
	sig, err := evm.SignTypedDataJSON(a.in.PrivateKey(), []byte(typedDataJSON))
	if err != nil {
		return "", err
	}
	return "0x" + hex.EncodeToString(sig), nil
}

func (a *EvmAccount) Wipe() {
	a.in.Wipe()
}
