package ton

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/tonkeeper/tongo/wallet"

	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/kernelflowlabs/wallet-lib/signing"
)

type Account struct {
	privateKey []byte
	publicKey  []byte
	address    string
}

func NewAccountFromMnemonic(mnemonic, path string) (signing.AccountHandler, error) {
	privateKey, err := derivePrivateKey(mnemonic)
	if err != nil {
		return nil, fmt.Errorf("derivePrivateKey: %v", err)
	}
	return NewAccountFromPrivateKey(privateKey)
}

func NewAccountFromPrivateKey(privateKey []byte) (signing.AccountHandler, error) {
	if len(privateKey) != ed25519.SeedSize {
		return nil, fmt.Errorf("invalid private key length %d", len(privateKey))
	}
	publicKey := ed25519.NewKeyFromSeed(privateKey).Public().(ed25519.PublicKey)
	address, err := PublicKey2Address(publicKey)
	if err != nil {
		return nil, fmt.Errorf("PublicKey2Address: %v", err)
	}
	if !ValidAddress(address) {
		return nil, fmt.Errorf("invalid address generated")
	}
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

func PublicKey2Address(publicKey []byte) (string, error) {
	address, err := wallet.GenerateWalletAddress(ed25519.PublicKey(publicKey), wallet.V5R1, nil, 0, nil)
	if err != nil {
		return "", err
	}
	return address.ToHuman(false, false), nil
}
