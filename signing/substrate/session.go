package substrate

import (
	"fmt"

	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/kernelflowlabs/wallet-lib/session"
)

// Substrate chains share one key and differ only in the SS58 prefix used to
// print it, so each is its own session chain, named "<package>-<chain>".
const (
	SessionChainPolkadot         = "substrate-polkadot"
	SessionChainKusama           = "substrate-kusama"
	SessionChainAstar            = "substrate-astar"
	SessionChainAcala            = "substrate-acala"
	SessionChainAlephZero        = "substrate-alephzero"
	SessionChainBittensor        = "substrate-bittensor"
	SessionChainPolkadotAssetHub = "substrate-polkadotassethub"
)

var sessionNetworks = map[string]string{
	SessionChainPolkadot:         NetworkEnumForDOT,
	SessionChainKusama:           NetworkEnumForKSM,
	SessionChainAstar:            NetworkEnumForASTR,
	SessionChainAcala:            NetworkEnumForACA,
	SessionChainAlephZero:        NetworkEnumForAZERO,
	SessionChainBittensor:        NetworkEnumForTAO,
	SessionChainPolkadotAssetHub: NetworkEnumForDOTASSETSHUB,
}

// DerivationPath is not BIP44.
//
// Substrate derives with its own junction syntax, and normalizePath maps "//0"
// onto the root key — which is what Polkadot-JS calls the primary account of a
// phrase. Index 1 and up are hard junctions off that root. Handing a BIP44
// string to the sr25519 deriver would be taken as one oddly named junction and
// produce an address no other Substrate wallet agrees with.
func DerivationPath(accountIndex int) string {
	return fmt.Sprintf("//%d", accountIndex)
}

func init() {
	for name, network := range sessionNetworks {
		network := network
		err := session.RegisterChain(session.Chain{
			Name:   name,
			Path:   DerivationPath,
			Derive: key.DerivePrivateKeySR25519,
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
	for name, candidate := range sessionNetworks {
		if candidate == network {
			return name, true
		}
	}
	return "", false
}
