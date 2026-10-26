package aptos

import (
	"crypto/sha3"
	"encoding/hex"
	"strings"

	"github.com/kernelflowlabs/wallet-lib/signing"
)

func PublicKey2Address(publicKey []byte) (string, error) {
	data := append(publicKey, 0x00)
	authKey := sha3.Sum256(data)
	return "0x" + hex.EncodeToString(authKey[:]), nil
}

func ValidAddress(address string) bool {
	if address == signing.MagicContactAddressForNative {
		return true
	}
	parts := strings.Split(address, "::")
	switch len(parts) {
	case 1:
		return validHexAddress(parts[0])
	case 3:
		return validHexAddress(parts[0]) && validMoveIdentifier(parts[1]) && validMoveIdentifier(parts[2])
	}
	return false
}

func validHexAddress(address string) bool {
	if !strings.HasPrefix(address, "0x") && !strings.HasPrefix(address, "0X") {
		return false
	}
	digits := address[2:]
	if len(digits) == 0 || len(digits) > 64 {
		return false
	}
	for _, c := range digits {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

func validMoveIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, c := range name {
		switch {
		case c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
		case i > 0 && c >= '0' && c <= '9':
		default:
			return false
		}
	}
	return true
}

func init() {
	if err := signing.RegisterAddressValidator("apt_addr", ValidAddress); err != nil {
		panic(err)
	}
}
