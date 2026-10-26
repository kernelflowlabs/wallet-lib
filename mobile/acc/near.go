//go:build near || !custom

package acc

import (
	"github.com/kernelflowlabs/wallet-lib/signing"
	"github.com/kernelflowlabs/wallet-lib/signing/near"
)

type NearAccount struct {
	in *near.Account
}

func NewNearFromMnemonic(mnemonic, path string) (*NearAccount, error) {
	acc, err := near.NewAccountFromMnemonic(mnemonic, path)
	if err != nil {
		return nil, err
	}
	return &NearAccount{in: acc.(*near.Account)}, nil
}

func (a *NearAccount) PrivateKey() []byte {
	return a.in.PrivateKey()
}

func (a *NearAccount) PublicKey() []byte {
	return a.in.PublicKey()
}

func (a *NearAccount) PublicKeyBase58() string {
	return a.in.PublicKeyBase58()
}

func (a *NearAccount) Address() string {
	return a.in.Address()
}
func init() {
	registerAddressValidator(signing.FamilyOfNear, func(address, network string) bool {
		return near.ValidAddress(address)
	})
}

func (a *NearAccount) Wipe() {
	a.in.Wipe()
}
