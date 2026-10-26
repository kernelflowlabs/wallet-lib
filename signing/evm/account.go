package evm

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
}

func NewAccountFromMnemonic(mnemonic, path string) (signing.AccountHandler, error) {
	privateKey, err := key.DerivePrivateKeyECDSA(mnemonic, path)
	if err != nil {
		return nil, fmt.Errorf("failed to DerivePrivateKey: %v", err)
	}
	return NewAccountFromPrivateKey(privateKey)
}

func NewAccountFromPrivateKey(privateKey []byte) (signing.AccountHandler, error) {
	a := &Account{}
	publicKey, err := key.PrivateKey2PublicKeyECDSA(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to PrivateKey2PublicKeyECDSA: %v", err)
	}
	address, err := PublicKey2Address(publicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to PublicKeyToAddress: %v", err)
	}
	if !ValidAddress(address) {
		return nil, fmt.Errorf("invalid address generated")
	}
	a.privateKey = privateKey
	a.publicKey = publicKey
	a.address = address
	return a, nil
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
	return key.SignWithPrivateKeyECDSAForEVM(a.privateKey, data)
}

func (a *Account) VerifySignData(data, sig []byte) bool {
	return key.VerifySignatureECDSAForEVM(a.publicKey, data, sig)
}

func (a *Account) Wipe() {
	for i := range a.privateKey {
		a.privateKey[i] = 0
	}
}
