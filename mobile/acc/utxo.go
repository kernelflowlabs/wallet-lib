//go:build utxo || !custom

package acc

import (
	"encoding/hex"
	"github.com/kernelflowlabs/wallet-lib/signing"
	"github.com/kernelflowlabs/wallet-lib/signing/utxo"
	"strings"
)

type UtxoAccount struct {
	in *utxo.Account
}

func NewUtxoFromMnemonic(mnemonic, path, network string) (*UtxoAccount, error) {
	acc, err := utxo.NewAccountFromMnemonic(mnemonic, path, network)
	if err != nil {
		return nil, err
	}
	return &UtxoAccount{in: acc.(*utxo.Account)}, nil
}

func NewUtxoFromPrivateKeyHex(privateKey, network string) (*UtxoAccount, error) {
	acc, err := utxo.NewAccountFromPrivateKeyHex(strings.TrimPrefix(privateKey, "0x"), network)
	if err != nil {
		return nil, err
	}
	return &UtxoAccount{in: acc.(*utxo.Account)}, nil
}

func (a *UtxoAccount) PrivateKey() []byte {
	return a.in.PrivateKey()
}

func (a *UtxoAccount) PublicKey() []byte {
	return a.in.PublicKey()
}

func (a *UtxoAccount) PublicKeyHex() string {
	return hex.EncodeToString(a.in.PublicKey())
}

func (a *UtxoAccount) Address() string {
	return a.in.Address()
}

func PrivateKeyHexToWIF(privateKey string) (string, error) {
	return utxo.PrivateKeyHexToWIF(privateKey)
}

func PrivateKeyWIFToHex(privateKey string) (string, error) {
	return utxo.PrivateKeyWIFToHex(privateKey)
}

const (
	UtxoNetworkEnumForBTC     string = utxo.NetworkEnumForBTC
	UtxoNetworkEnumForLTC     string = utxo.NetworkEnumForLTC
	UtxoNetworkEnumForDOGE    string = utxo.NetworkEnumForDOGE
	UtxoNetworkEnumForBTCP2TR string = utxo.NetworkEnumForBTCP2TR
	UtxoNetworkEnumForSYS     string = utxo.NetworkEnumForSYS
)

func init() {
	registerAddressValidator(signing.FamilyOfUTXO, func(address, network string) bool {
		return utxo.ValidAddress(address, network)
	})
}

func (a *UtxoAccount) Wipe() {
	a.in.Wipe()
}
