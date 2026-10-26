package substrate

import (
	"github.com/btcsuite/btcd/address/v2/base58"
	walletsubstrate "github.com/kernelflowlabs/wallet-lib/signing/substrate"
)

func convert2PublicKey(address string) []byte {
	addrBytes := base58.Decode(address)
	if len(addrBytes) != 35 {
		return nil
	}
	addrBytes = addrBytes[1:]
	addrBytes = addrBytes[:len(addrBytes)-2]
	return addrBytes
}

var networkSubscanURL = map[string]string{
	walletsubstrate.NetworkEnumForDOT:          "https://polkadot.api.subscan.io/api/scan/extrinsic",
	walletsubstrate.NetworkEnumForKSM:          "https://kusama.api.subscan.io/api/scan/extrinsic",
	walletsubstrate.NetworkEnumForASTR:         "https://astar.api.subscan.io/api/scan/extrinsic",
	walletsubstrate.NetworkEnumForACA:          "https://acala.api.subscan.io/api/scan/extrinsic",
	walletsubstrate.NetworkEnumForAZERO:        "https://alephzero.api.subscan.io/api/scan/extrinsic",
	walletsubstrate.NetworkEnumForTAO:          "https://bittensor.api.subscan.io/api/scan/extrinsic",
	walletsubstrate.NetworkEnumForDOTASSETSHUB: "https://assethub-polkadot.api.subscan.io/api/scan/extrinsic",
}

func getSubscanURL(network string) (string, bool) {
	url, ok := networkSubscanURL[network]
	return url, ok
}
