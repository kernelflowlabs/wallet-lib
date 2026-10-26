//go:build kas || !custom

package acc

import (
	"github.com/kernelflowlabs/wallet-lib/signing"
	"github.com/kernelflowlabs/wallet-lib/signing/kaspa"
)

type KasAccount struct {
	in *kaspa.Account
}

func NewKasFromMnemonic(mnemonic, path string) (*KasAccount, error) {
	acc, err := kaspa.NewAccountFromMnemonic(mnemonic, path)
	if err != nil {
		return nil, err
	}
	return &KasAccount{in: acc.(*kaspa.Account)}, nil
}

func (a *KasAccount) PrivateKey() []byte {
	return a.in.PrivateKey()
}

func (a *KasAccount) PublicKey() []byte {
	return a.in.PublicKey()
}

func (a *KasAccount) Address() string {
	return a.in.Address()
}
func init() {
	registerAddressValidator(signing.FamilyOfKAS, func(address, network string) bool {
		return kaspa.ValidAddress(address)
	})
}

func (a *KasAccount) Wipe() {
	a.in.Wipe()
}
