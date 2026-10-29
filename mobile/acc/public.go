package acc

import (
	"github.com/kernelflowlabs/wallet-lib/crypto"
)

var addressValidators = map[int]func(address, network string) bool{}

func registerAddressValidator(family int, fn func(address, network string) bool) {
	addressValidators[family] = fn
}

func GenerateMnemonic() (string, error) {
	return crypto.GenerateMnemonic(12)
}

func WriteToKeyStore(name, text, password string, isMn bool) (string, error) {
	return crypto.WriteToKeyStore(name, text, password, isMn)
}

func WriteToKeyStoreBytes(name string, text, password []byte, isMn bool) (string, error) {
	return crypto.WriteToKeyStoreBytes(name, text, password, isMn)
}

func ReadFromKeyStoreBytes(ksString string, password []byte, isMn bool) ([]byte, error) {
	return crypto.ReadFromKeyStoreBytes(ksString, password, isMn)
}

func ReadFromKeyStore(ksString, password string, isMn bool) (string, error) {
	return crypto.ReadFromKeyStore(ksString, password, isMn)
}

func ValidAddress(address string, family int, network string) bool {
	if fn, ok := addressValidators[family]; ok {
		return fn(address, network)
	}
	return false
}
