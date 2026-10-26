package substrate

import (
	"encoding/hex"
	"fmt"
	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/kernelflowlabs/wallet-lib/signing"
	"strings"
)

type Account struct {
	privateKey []byte
	publicKey  []byte
	address    string
	network    string
}

func NewAccount() signing.AccountHandler {
	return &Account{}
}

func NewAccountFromMnemonic(mnemonic, path, network string) (signing.AccountHandler, error) {
	privateKey, err := key.DerivePrivateKeySR25519(mnemonic, normalizePath(path))
	if err != nil {
		return nil, err
	}
	return NewAccountFromPrivateKey(privateKey, network)
}

func NewAccountFromMnemonicStandard(mnemonic, path, network string) (signing.AccountHandler, error) {
	privateKey, err := key.DerivePrivateKeySR25519Standard(mnemonic, path)
	if err != nil {
		return nil, err
	}
	return NewAccountFromPrivateKey(privateKey, network)
}

func NewAccountFromPrivateKey(privateKey []byte, network string) (signing.AccountHandler, error) {
	a := &Account{}
	publicKey, err := key.PrivateKey2PublicKeySR25519(privateKey)
	if err != nil {
		return nil, err
	}
	address := PublicKey2Address(publicKey, network)
	if !ValidAddress(address, network) {
		return nil, fmt.Errorf("invalid address generated")
	}
	a.privateKey = privateKey
	a.publicKey = publicKey
	a.address = address
	a.network = network
	return a, nil
}

func NewAccountFromPrivateKeyHex(privateKey string, network string) (signing.AccountHandler, error) {
	privateKeyBytes, err := hex.DecodeString(strings.TrimPrefix(privateKey, "0x"))
	if err != nil {
		return nil, fmt.Errorf("failed to DecodeString privateKey: %v", err)
	}
	return NewAccountFromPrivateKey(privateKeyBytes, network)
}

func (a *Account) PrivateKey() []byte {
	return a.privateKey
}

func (a *Account) PublicKey() []byte {
	return a.publicKey
}

func (a *Account) PrivateKeyHex() string {
	return hex.EncodeToString(a.privateKey)
}

func (a *Account) PublicKeyHex() string {
	return hex.EncodeToString(a.publicKey)
}

func (a *Account) Address() string {
	return a.address
}

func (a *Account) SignData(data []byte) ([]byte, error) {
	return key.SignWithPrivateKeySR25519(a.privateKey, data)
}

func (a *Account) VerifySignData(data, sig []byte) bool {
	return key.VerifySignatureSR25519(a.publicKey, data, sig)
}

// normalizePath converts the caller-supplied path to the internal derivation path.
//
// Convention: path is "//cointype//index", e.g. "//354//0" for DOT account 0.
// The coin type segment is stripped — it is for client-side identification only.
// Account index 0 maps to the root key (empty path), which is the standard
// Polkadot/Substrate address. Every other index is kept as it was given.
//
//	"//354//0"  →  ""     (root key, account 0)
//	"//354//1"  →  "//1"  (account 1)
//	"//434//0"  →  ""     (root key, KSM account 0)
//	"//0"       →  ""     (legacy shorthand, kept for compatibility)
func normalizePath(path string) string {
	// Strip leading //segment (coin type) if a second // exists
	if strings.HasPrefix(path, "//") {
		rest := path[2:]
		if idx := strings.Index(rest, "//"); idx >= 0 {
			path = rest[idx:]
		}
	}
	// Account 0 uses root key
	if path == "//0" {
		return ""
	}
	return path
}

func (a *Account) Wipe() {
	for i := range a.privateKey {
		a.privateKey[i] = 0
	}
}
