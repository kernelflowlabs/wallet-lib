//go:build sol || !custom

package acc

import (
	"github.com/kernelflowlabs/wallet-lib/signing"
	"github.com/kernelflowlabs/wallet-lib/signing/solana"
)

type SolAccount struct {
	in *solana.Account
}

func NewSolFromMnemonic(mnemonic, path string) (*SolAccount, error) {
	acc, err := solana.NewAccountFromMnemonic(mnemonic, path)
	if err != nil {
		return nil, err
	}
	return &SolAccount{in: acc.(*solana.Account)}, nil
}

func (a *SolAccount) PrivateKey() []byte {
	return a.in.PrivateKey()
}

func (a *SolAccount) PublicKey() []byte {
	return a.in.PublicKey()
}

func (a *SolAccount) Address() string {
	return a.in.Address()
}
func init() {
	registerAddressValidator(signing.FamilyOfSOL, func(address, network string) bool {
		return solana.ValidAddress(address)
	})
}

func (a *SolAccount) Wipe() {
	a.in.Wipe()
}
