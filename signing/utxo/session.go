package utxo

import (
	"fmt"

	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/kernelflowlabs/wallet-lib/session"
)

// Each UTXO coin is its own session chain: the key derivation differs by coin
// type and by script type, and the address encoding differs by network.
const (
	SessionChainBitcoin        = "utxo-bitcoin"
	SessionChainBitcoinTaproot = "utxo-bitcointaproot"
	SessionChainLitecoin       = "utxo-litecoin"
	SessionChainDogecoin       = "utxo-dogecoin"
	SessionChainSyscoin        = "utxo-syscoin"
)

type sessionCoin struct {
	name    string
	network string
	// purpose is the BIP43 purpose field: 44 for legacy, 84 for native
	// segwit, 86 for taproot. It is part of the path, so it decides the
	// addresses as much as the coin type does.
	purpose  int
	coinType int
}

var sessionCoins = []sessionCoin{
	{SessionChainBitcoin, NetworkEnumForBTC, 84, 0},
	{SessionChainBitcoinTaproot, NetworkEnumForBTCP2TR, 86, 0},
	{SessionChainLitecoin, NetworkEnumForLTC, 84, 2},
	{SessionChainDogecoin, NetworkEnumForDOGE, 44, 3},
	{SessionChainSyscoin, NetworkEnumForSYS, 84, 57},
}

func DerivationPath(purpose, coinType, accountIndex int) string {
	return fmt.Sprintf("m/%d'/%d'/0'/0/%d", purpose, coinType, accountIndex)
}

func init() {
	for _, entry := range sessionCoins {
		purpose := entry.purpose
		coinType := entry.coinType
		network := entry.network
		err := session.RegisterChain(session.Chain{
			Name: entry.name,
			Path: func(accountIndex int) string {
				return DerivationPath(purpose, coinType, accountIndex)
			},
			Derive: key.DerivePrivateKeyECDSA,
			Address: func(privateKey []byte) (string, error) {
				account, err := NewAccountFromPrivateKey(privateKey, network)
				if err != nil {
					return "", err
				}
				return account.Address(), nil
			},
		})
		if err != nil {
			panic(err)
		}
	}
}

// SessionChainForNetwork maps a network enum onto the session chain name.
func SessionChainForNetwork(network string) (string, bool) {
	for _, entry := range sessionCoins {
		if entry.network == network {
			return entry.name, true
		}
	}
	return "", false
}
