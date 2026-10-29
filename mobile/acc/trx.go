//go:build trx || !custom

package acc

import (
	"github.com/kernelflowlabs/wallet-lib/crypto"
	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/kernelflowlabs/wallet-lib/signing"
	"github.com/kernelflowlabs/wallet-lib/signing/tron"
)

type TrxAccount struct {
	in *tron.Account
}

func NewTrxFromMnemonic(mnemonic, path string) (*TrxAccount, error) {
	acc, err := tron.NewAccountFromMnemonic(mnemonic, path)
	if err != nil {
		return nil, err
	}
	return &TrxAccount{in: acc.(*tron.Account)}, nil
}

func NewTrxFromMnemonicBytes(mnemonic []byte, path string) (*TrxAccount, error) {
	priv, err := key.DerivePrivateKeyECDSABytes(mnemonic, path)
	if err != nil {
		return nil, err
	}
	defer crypto.Wipe(priv)
	acc, err := tron.NewAccountFromPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	return &TrxAccount{in: acc.(*tron.Account)}, nil
}

func (a *TrxAccount) PrivateKey() []byte {
	return a.in.PrivateKey()
}

func (a *TrxAccount) PublicKey() []byte {
	return a.in.PublicKey()
}

func (a *TrxAccount) Address() string {
	return a.in.Address()
}
func init() {
	registerAddressValidator(signing.FamilyOfTRX, func(address, network string) bool {
		return tron.ValidAddress(address)
	})
}

func (a *TrxAccount) Wipe() {
	a.in.Wipe()
}
