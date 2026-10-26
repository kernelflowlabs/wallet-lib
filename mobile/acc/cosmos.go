//go:build cosmos || !custom

package acc

import (
	"github.com/kernelflowlabs/wallet-lib/signing"
	"github.com/kernelflowlabs/wallet-lib/signing/cosmos"
)

type CosmosAccount struct {
	in *cosmos.Account
}

func NewCosmosFromMnemonic(mnemonic, path, network string) (*CosmosAccount, error) {
	acc, err := cosmos.NewAccountFromMnemonic(mnemonic, path, network)
	if err != nil {
		return nil, err
	}
	return &CosmosAccount{in: acc.(*cosmos.Account)}, nil
}

func (a *CosmosAccount) PrivateKey() []byte {
	return a.in.PrivateKey()
}

func (a *CosmosAccount) PublicKey() []byte {
	return a.in.PublicKey()
}

func (a *CosmosAccount) Address() string {
	return a.in.Address()
}
func init() {
	registerAddressValidator(signing.FamilyOfCOSMOS, func(address, network string) bool {
		return cosmos.ValidAddress(address, network)
	})
}

func (a *CosmosAccount) Wipe() {
	a.in.Wipe()
}
