package cosmos

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/kernelflowlabs/wallet-lib/signing"
)

type Account struct {
	privateKey []byte
	publicKey  []byte
	address    string
	network    string
}

func NewAccountFromMnemonic(mnemonic, path, network string) (signing.AccountHandler, error) {
	privateKey, err := key.DerivePrivateKeyECDSA(mnemonic, path)
	if err != nil {
		return nil, fmt.Errorf("DerivePrivateKeyECDSA: %v", err)
	}
	return NewAccountFromPrivateKey(privateKey, network)
}

func NewAccountFromPrivateKey(privateKey []byte, network string) (signing.AccountHandler, error) {
	publicKey, err := key.PrivateKey2PublicKeyECDSA(privateKey)
	if err != nil {
		return nil, fmt.Errorf("PrivateKey2PublicKeyECDSA: %v", err)
	}
	address, err := PublicKey2Address(publicKey, network)
	if err != nil {
		return nil, fmt.Errorf("PublicKey2Address: %v", err)
	}
	if !ValidAddress(address, network) {
		return nil, fmt.Errorf("invalid address generated")
	}
	return &Account{privateKey: privateKey, publicKey: publicKey, address: address, network: network}, nil
}

func NewAccountFromPrivateKeyHex(privateKey, network string) (signing.AccountHandler, error) {
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
	return key.SignWithPrivateKeyECDSAForUTXO(a.privateKey, data)
}

func (a *Account) VerifySignData(data, sig []byte) bool {
	return key.VerifySignatureECDSAForUTXO(a.publicKey, data, sig)
}

func (a *Account) Wipe() {
	for i := range a.privateKey {
		a.privateKey[i] = 0
	}
}
