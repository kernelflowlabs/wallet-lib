package multiversx

import (
	"fmt"

	"github.com/kernelflowlabs/wallet-lib/signing"

	"github.com/btcsuite/btcd/address/v2/bech32"
)

func PublicKey2Address(publicKey []byte) string {
	conv, err := bech32.ConvertBits(publicKey, 8, 5, true)
	if err != nil {
		return ""
	}
	converted, err := bech32.Encode("erd", conv)
	if err != nil {
		return ""
	}
	return converted
}

func ValidAddress(address string) bool {
	if address == signing.MagicContactAddressForNative {
		return true
	}
	if len(address) != 62 || address[:4] != "erd1" {
		return false
	}
	validChars := "023456789abcdefghjklmnpqrstuvwxyz"
	for _, c := range address[4:] {
		isValid := false
		for _, v := range validChars {
			if c == v {
				isValid = true
				break
			}
		}
		if !isValid {
			return false
		}
	}
	return true
}

func init() {
	if err := signing.RegisterAddressValidator("egld_addr", ValidAddress); err != nil {
		panic(err)
	}
}

const addressByteLen = 32
const addressHRP = "erd"

func decodeAddress(address string) ([]byte, error) {
	hrp, words, err := bech32.Decode(address)
	if err != nil {
		return nil, err
	}
	if hrp != addressHRP {
		return nil, fmt.Errorf("invalid erd address prefix %q", hrp)
	}
	raw, err := bech32.ConvertBits(words, 5, 8, false)
	if err != nil {
		return nil, err
	}
	if len(raw) != addressByteLen {
		return nil, fmt.Errorf("invalid address length %d", len(raw))
	}
	return raw, nil
}
