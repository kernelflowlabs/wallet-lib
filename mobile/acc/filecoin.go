//go:build filecoin || !custom

package acc

import (
	"github.com/kernelflowlabs/wallet-lib/signing"
	"github.com/kernelflowlabs/wallet-lib/signing/filecoin"
)

type FilecoinAccount struct {
	in *filecoin.Account
}

func NewFilecoinFromMnemonic(mnemonic, path string) (*FilecoinAccount, error) {
	acc, err := filecoin.NewAccountFromMnemonic(mnemonic, path)
	if err != nil {
		return nil, err
	}
	return &FilecoinAccount{in: acc.(*filecoin.Account)}, nil
}

func (a *FilecoinAccount) PrivateKey() []byte { return a.in.PrivateKey() }
func (a *FilecoinAccount) PublicKey() []byte  { return a.in.PublicKey() }
func (a *FilecoinAccount) Address() string    { return a.in.Address() }
func init() {
	registerAddressValidator(signing.FamilyOfFilecoin, func(address, network string) bool {
		return filecoin.ValidAddress(address)
	})
}
func (a *FilecoinAccount) Wipe() { a.in.Wipe() }
