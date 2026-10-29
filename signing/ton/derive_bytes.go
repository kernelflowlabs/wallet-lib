package ton

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha512"
	"github.com/kernelflowlabs/wallet-lib/crypto"

	"golang.org/x/crypto/pbkdf2"
)

func normalizeMnemonicBytes(mnemonic []byte) []byte {
	words := bytes.Fields(mnemonic)
	for i, w := range words {
		words[i] = bytes.ToLower(w)
	}
	return bytes.Join(words, []byte(" "))
}

func DerivePrivateKeyBytes(mnemonic []byte, _ string) ([]byte, error) {
	normalized := normalizeMnemonicBytes(mnemonic)
	mac := hmac.New(sha512.New, normalized)
	mac.Write(nil)
	hash := mac.Sum(nil)
	key := pbkdf2.Key(hash, []byte(tonSeedSalt), tonSeedIterations, 32, sha512.New)
	crypto.Wipe(normalized)
	crypto.Wipe(hash)
	return key, nil
}
