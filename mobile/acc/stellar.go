//go:build stellar || !custom

package acc

import (
	"github.com/kernelflowlabs/wallet-lib/signing"
	"github.com/kernelflowlabs/wallet-lib/signing/stellar"
)

type StellarAccount struct {
	in *stellar.Account
}

func NewStellarFromMnemonic(mnemonic, path string) (*StellarAccount, error) {
	acc, err := stellar.NewAccountFromMnemonic(mnemonic, path)
	if err != nil {
		return nil, err
	}
	return &StellarAccount{in: acc.(*stellar.Account)}, nil
}

func (a *StellarAccount) PrivateKey() []byte {
	return a.in.PrivateKey()
}

func (a *StellarAccount) PublicKey() []byte {
	return a.in.PublicKey()
}

func (a *StellarAccount) Address() string {
	return a.in.Address()
}
func init() {
	registerAddressValidator(signing.FamilyOfStellar, func(address, network string) bool {
		return stellar.ValidAddress(address)
	})
}

func (a *StellarAccount) Wipe() {
	a.in.Wipe()
}
