package kaspa

import (
	"encoding/hex"
	"fmt"
	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/kernelflowlabs/wallet-lib/signing"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"golang.org/x/crypto/blake2b"
)

type Account struct {
	privateKey []byte
	publicKey  []byte
	address    string
}

type AddressData struct {
	version    []byte
	pubKeyHash []byte
	checksum   []byte
}

func NewAccount() signing.AccountHandler {
	return &Account{}
}

func NewAccountFromMnemonic(mnemonic, path string) (signing.AccountHandler, error) {
	privateKey, err := key.DerivePrivateKeyECDSA(mnemonic, path)
	if err != nil {
		return nil, fmt.Errorf("NewPrivateKeyFromMnemonic: %v", err)
	}
	return NewAccountFromPrivateKey(privateKey)
}

func NewAccountFromPrivateKey(privateKey []byte) (signing.AccountHandler, error) {
	a := &Account{}
	publicKey, err := key.PrivateKey2PublicKeyECDSA(privateKey)
	if err != nil {
		return nil, fmt.Errorf("PrivateKey2PublicKeyECDSA: %v", err)
	} else if len(publicKey) < 1 {
		return nil, fmt.Errorf("invaid publicKey length %v", err)
	}
	publicKey = publicKey[1:]
	address := PublicKey2Address(publicKey)
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
	return key.SignWithPrivateKeySchnorr(a.privateKey, data)
}

func (a *Account) VerifySignData(data, sig []byte) bool {
	return key.VerifySignatureSchnorr(a.publicKey, data, sig)
}

func (a *Account) SignDataLikeOkx(data []byte) []byte {
	blake2b256, err := blake2b.New256([]byte("PersonalMessageSigningHash"))
	if err != nil {
		return nil
	}
	blake2b256.Write(data)
	hash := blake2b256.Sum(nil)
	prvKey, _ := btcec.PrivKeyFromBytes(a.privateKey)
	signature, err := schnorr.Sign(prvKey, hash)
	if err != nil {
		return nil
	}
	return signature.Serialize()
}

func (a *Account) Wipe() {
	for i := range a.privateKey {
		a.privateKey[i] = 0
	}
}
