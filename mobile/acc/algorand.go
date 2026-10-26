//go:build algorand || !custom

package acc

import (
	"github.com/kernelflowlabs/wallet-lib/signing"
	"github.com/kernelflowlabs/wallet-lib/signing/algorand"
)

type AlgorandAccount struct {
	in *algorand.Account
}

func NewAlgorandFromMnemonic(mnemonic, path string) (*AlgorandAccount, error) {
	acc, err := algorand.NewAccountFromMnemonic(mnemonic, path)
	if err != nil {
		return nil, err
	}
	return &AlgorandAccount{in: acc.(*algorand.Account)}, nil
}

func (a *AlgorandAccount) PrivateKey() []byte { return a.in.PrivateKey() }
func (a *AlgorandAccount) PublicKey() []byte  { return a.in.PublicKey() }
func (a *AlgorandAccount) Address() string    { return a.in.Address() }
func init() {
	registerAddressValidator(signing.FamilyOfAlgorand, func(address, network string) bool {
		return algorand.ValidAddress(address)
	})
}
func (a *AlgorandAccount) Wipe() { a.in.Wipe() }
