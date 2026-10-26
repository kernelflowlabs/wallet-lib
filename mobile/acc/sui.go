//go:build sui || !custom

package acc

import (
	"github.com/kernelflowlabs/wallet-lib/signing"
	"github.com/kernelflowlabs/wallet-lib/signing/sui"
)

type SuiAccount struct {
	in *sui.Account
}

func NewSuiFromMnemonic(mnemonic, path string) (*SuiAccount, error) {
	acc, err := sui.NewAccountFromMnemonic(mnemonic, path)
	if err != nil {
		return nil, err
	}
	return &SuiAccount{in: acc.(*sui.Account)}, nil
}

func (a *SuiAccount) PrivateKey() []byte {
	return a.in.PrivateKey()
}

func (a *SuiAccount) PublicKey() []byte {
	return a.in.PublicKey()
}

func (a *SuiAccount) Address() string {
	return a.in.Address()
}
func init() {
	registerAddressValidator(signing.FamilyOfSUI, func(address, network string) bool {
		return sui.ValidAddress(address)
	})
}

func (a *SuiAccount) Wipe() {
	a.in.Wipe()
}
