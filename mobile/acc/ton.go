//go:build ton || !custom

package acc

import (
	"github.com/kernelflowlabs/wallet-lib/signing"
	"github.com/kernelflowlabs/wallet-lib/signing/ton"
)

type TonAccount struct {
	in *ton.Account
}

func NewTonFromMnemonic(mnemonic, path string) (*TonAccount, error) {
	acc, err := ton.NewAccountFromMnemonic(mnemonic, path)
	if err != nil {
		return nil, err
	}
	return &TonAccount{in: acc.(*ton.Account)}, nil
}

func (a *TonAccount) PrivateKey() []byte {
	return a.in.PrivateKey()
}

func (a *TonAccount) PublicKey() []byte {
	return a.in.PublicKey()
}

func (a *TonAccount) Address() string {
	return a.in.Address()
}
func init() {
	registerAddressValidator(signing.FamilyOfTON, func(address, network string) bool {
		return ton.ValidAddress(address)
	})
}

func (a *TonAccount) Wipe() {
	a.in.Wipe()
}
