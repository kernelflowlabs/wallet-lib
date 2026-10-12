package near

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/address/v2/base58"

	"github.com/kernelflowlabs/wallet-sdk/crypto/key"
	"github.com/kernelflowlabs/wallet-sdk/signing"
)

type Account struct {
	privateKey []byte
	publicKey  []byte
	address    string
}

func NewAccountFromMnemonic(mnemonic, path string) (signing.AccountHandler, error) {
	privateKey, err := key.DerivePrivateKeyED25519(mnemonic, path)
	if err != nil {
		return nil, fmt.Errorf("DerivePrivateKeyED25519: %v", err)
	}
	return NewAccountFromPrivateKey(privateKey)
}

func NewAccountFromPrivateKey(privateKey []byte) (signing.AccountHandler, error) {
	if len(privateKey) != ed25519.SeedSize {
		return nil, fmt.Errorf("invalid private key length %d", len(privateKey))
	}
	publicKey := ed25519.NewKeyFromSeed(privateKey).Public().(ed25519.PublicKey)

	address := hex.EncodeToString(publicKey)
	return &Account{privateKey: privateKey, publicKey: publicKey, address: address}, nil
}

func NewAccountFromPrivateKeyHex(privateKey string) (signing.AccountHandler, error) {
	privateKeyBytes, err := hex.DecodeString(strings.TrimPrefix(privateKey, "0x"))
	if err != nil {
		return nil, fmt.Errorf("failed to DecodeString privateKey: %v", err)
	}
	return NewAccountFromPrivateKey(privateKeyBytes)
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

func (a *Account) PrivateKeyBase58() string {
	return base58.Encode(ed25519.NewKeyFromSeed(a.privateKey))
}

func (a *Account) PublicKeyBase58() string {
	return base58.Encode(a.publicKey)
}

func (a *Account) Address() string {
	return a.address
}

func (a *Account) SignData(data []byte) ([]byte, error) {
	return key.SignWithPrivateKeyED25519(a.privateKey, data)
}

func (a *Account) VerifySignData(data, sig []byte) bool {
	return key.VerifySignatureED25519(a.publicKey, data, sig)
}

func (a *Account) Wipe() {
	for i := range a.privateKey {
		a.privateKey[i] = 0
	}
}
