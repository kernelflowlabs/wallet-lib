//go:build apt || !custom

package acc

import (
	"github.com/kernelflowlabs/wallet-lib/signing"
	"github.com/kernelflowlabs/wallet-lib/signing/aptos"
)

type AptAccount struct {
	in *aptos.Account
}

func NewAptFromMnemonic(mnemonic, path string) (*AptAccount, error) {
	acc, err := aptos.NewAccountFromMnemonic(mnemonic, path)
	if err != nil {
		return nil, err
	}
	return &AptAccount{in: acc.(*aptos.Account)}, nil
}

func (a *AptAccount) PrivateKey() []byte {
	return a.in.PrivateKey()
}

func (a *AptAccount) PublicKey() []byte {
	return a.in.PublicKey()
}

func (a *AptAccount) PublicKeyHex() string {
	return a.in.PublicKeyHex()
}

func (a *AptAccount) Address() string {
	return a.in.Address()
}
func init() {
	registerAddressValidator(signing.FamilyOfAPT, func(address, network string) bool {
		return aptos.ValidAddress(address)
	})
}

func (a *AptAccount) Wipe() {
	a.in.Wipe()
}
