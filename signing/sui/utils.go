package sui

import (
	"encoding/hex"
	"github.com/kernelflowlabs/wallet-lib/signing"

	"golang.org/x/crypto/blake2b"
)

func PublicKey2Address(publicKey []byte) (string, error) {
	tmp := []byte{0}
	tmp = append(tmp, publicKey...)
	addrBytes := blake2b.Sum256(tmp)
	address := "0x" + hex.EncodeToString(addrBytes[:])[:64]
	return address, nil
}

func ValidAddress(address string) bool {
	if address == signing.MagicContactAddressForNative {
		return true
	}
	if len(address) > 66 {
		return false
	}
	if len(address) >= 2 && (address[:2] == "0x" || address[:2] == "0X") {
		address = address[2:]
	}
	for _, c := range address {
		if !((c >= '0' && c <= '9') ||
			(c >= 'a' && c <= 'f') ||
			(c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func init() {
	if err := signing.RegisterAddressValidator("sui_addr", ValidAddress); err != nil {
		panic(err)
	}
}
