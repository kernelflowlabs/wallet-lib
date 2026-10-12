package substrate

import (
	"github.com/btcsuite/btcd/address/v2/base58"
)

func PublicKey2Address(publicKey []byte, network string) string {
	prefix, ok := networkSS58Prefix[network]
	if !ok {
		return ""
	}
	address, err := toBase58(append([]byte{prefix}, publicKey[:]...), publicKey[:], prefix)
	if err != nil {
		return ""
	}
	return address
}

func ValidAddress(address, network string) bool {
	prefix, ok := networkSS58Prefix[network]
	if !ok {
		return false
	}
	addrBytes := base58.Decode(address)
	if len(addrBytes) != 35 {
		return false
	}
	if addrBytes[0] != prefix {
		return false
	}
	publicKey := addrBytes[1:33]
	storedChecksum := addrBytes[33:35]
	buf := append([]byte{prefix}, publicKey...)
	expectedChecksum, err := ss58Checksum(buf)
	if err != nil {
		return false
	}
	return storedChecksum[0] == expectedChecksum[0] && storedChecksum[1] == expectedChecksum[1]
}

const (
	NetworkEnumForDOT          string = "0"            //354
	NetworkEnumForKSM          string = "2"            //434
	NetworkEnumForASTR         string = "5"            //810
	NetworkEnumForACA          string = "10"           //787
	NetworkEnumForAZERO        string = "42"           //643
	NetworkEnumForTAO          string = "tao"          //13116
	NetworkEnumForDOTASSETSHUB string = "assethub-dot" // Polkadot AssetHub parachain (parachain ID 1000)
)

var networkSS58Prefix = map[string]uint8{
	NetworkEnumForDOT:          0,
	NetworkEnumForKSM:          2,
	NetworkEnumForASTR:         5,
	NetworkEnumForACA:          10,
	NetworkEnumForAZERO:        42,
	NetworkEnumForTAO:          42,
	NetworkEnumForDOTASSETSHUB: 0, // same SS58 prefix as Polkadot relay chain
}
