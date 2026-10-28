package cosmos

import (
	"fmt"

	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/kernelflowlabs/wallet-lib/session"
)

// Cosmos is a family, not one chain: the key is the same shape everywhere but
// the bech32 prefix and the coin type differ per network. Each network is its
// own session chain, named "<package>:<network>" so that every chain name in
// the registry starts with the package that registered it.
const (
	SessionChainCosmosHub    = "cosmos-cosmoshub"
	SessionChainTerra        = "cosmos-terra"
	SessionChainTerraClassic = "cosmos-terraclassic"
	SessionChainSei          = "cosmos-sei"
	SessionChainCelestia     = "cosmos-celestia"
	SessionChainInjective    = "cosmos-injective"
)

type sessionNetwork struct {
	name     string
	network  string
	coinType int
}

// Terra moved to coin type 330, and Injective is an Ethermint chain using 60.
// The rest follow the Cosmos Hub's 118.
var sessionNetworks = []sessionNetwork{
	{SessionChainCosmosHub, NetworkEnumForCosmosHub, 118},
	{SessionChainTerra, NetworkEnumForTerra, 330},
	{SessionChainTerraClassic, NetworkEnumForTerrac, 330},
	{SessionChainSei, NetworkEnumForSei, 118},
	{SessionChainCelestia, NetworkEnumForCelestia, 118},
	{SessionChainInjective, NetworkEnumForInjective, 60},
}

func DerivationPath(coinType, accountIndex int) string {
	return fmt.Sprintf("m/44'/%d'/0'/0/%d", coinType, accountIndex)
}

func init() {
	for _, entry := range sessionNetworks {
		coinType := entry.coinType
		network := entry.network
		err := session.RegisterChain(session.Chain{
			Name: entry.name,
			Path: func(accountIndex int) string {
				return DerivationPath(coinType, accountIndex)
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

// SessionChainForNetwork maps a chain id onto the session chain name, so a
// builder that was created for a network can find the key for it.
func SessionChainForNetwork(network string) (string, bool) {
	for _, entry := range sessionNetworks {
		if entry.network == network {
			return entry.name, true
		}
	}
	return "", false
}
