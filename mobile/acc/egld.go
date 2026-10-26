//go:build egld || !custom

package acc

import (
	"github.com/kernelflowlabs/wallet-lib/signing"
	"github.com/kernelflowlabs/wallet-lib/signing/multiversx"
)

type EgldAccount struct {
	in *multiversx.Account
}

func NewEgldFromMnemonic(mnemonic, path string) (*EgldAccount, error) {
	acc, err := multiversx.NewAccountFromMnemonic(mnemonic, path)
	if err != nil {
		return nil, err
	}
	return &EgldAccount{in: acc.(*multiversx.Account)}, nil
}

func (a *EgldAccount) PrivateKey() []byte {
	return a.in.PrivateKey()
}

func (a *EgldAccount) PublicKey() []byte {
	return a.in.PublicKey()
}

func (a *EgldAccount) PublicKeyHex() string {
	return a.in.PublicKeyHex()
}

func (a *EgldAccount) Address() string {
	return a.in.Address()
}
func init() {
	registerAddressValidator(signing.FamilyOfEGLD, func(address, network string) bool {
		return multiversx.ValidAddress(address)
	})
}

func (a *EgldAccount) Wipe() {
	a.in.Wipe()
}
