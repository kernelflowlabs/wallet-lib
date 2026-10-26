package utxo

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/kernelflowlabs/wallet-lib/signing"

	"github.com/btcsuite/btcd/chaincfg/v2"
)

var btcParams = chaincfg.MainNetParams
var ltcParams = chaincfg.MainNetParams
var dogeParams = chaincfg.MainNetParams
var sysParams = chaincfg.MainNetParams

func init() {
	{
		ltcParams.Net = 2
		ltcParams.DefaultPort = "9333"
		ltcParams.Bech32HRPSegwit = "ltc"
		ltcParams.PubKeyHashAddrID = 0x30
		ltcParams.ScriptHashAddrID = 0x32
		ltcParams.PrivateKeyID = 0xB0
		ltcParams.HDCoinType = 2
		err := chaincfg.Register(&ltcParams)
		if err != nil {
			panic(fmt.Sprintf("failed to register params for LTC, err=%v", err))
		}
	}
	{
		dogeParams.Net = 3
		dogeParams.DefaultPort = "22556"
		dogeParams.PubKeyHashAddrID = 0x1E
		dogeParams.ScriptHashAddrID = 0x16
		dogeParams.PrivateKeyID = 0x9E
		err := chaincfg.Register(&dogeParams)
		if err != nil {
			panic(fmt.Sprintf("failed to register params for DOGE, err=%v", err))
		}
	}
	{
		sysParams.Name = "sysmainnet"
		sysParams.Net = 1
		sysParams.Bech32HRPSegwit = "sys"
		sysParams.PubKeyHashAddrID = 0x3F
		sysParams.ScriptHashAddrID = 0x05
		sysParams.PrivateKeyID = 0x80
		sysParams.WitnessPubKeyHashAddrID = 0x06
		sysParams.WitnessScriptHashAddrID = 0x0A
		sysParams.HDCoinType = 57
		sysParams.HDPrivateKeyID = [4]byte{0x04, 0xB2, 0x43, 0x0C}
		sysParams.HDPublicKeyID = [4]byte{0x04, 0xb2, 0x47, 0x46}
		err := chaincfg.Register(&sysParams)
		if err != nil {
			panic(fmt.Sprintf("failed to register params for SYS, err=%v", err))
		}
	}
}

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
	privateKey, err := key.DerivePrivateKeyECDSA(mnemonic, path)
	if err != nil {
		return nil, fmt.Errorf("NewPrivateKeyFromMnemonic: %v", err)
	}
	return NewAccountFromPrivateKey(privateKey, network)
}

func NewAccountFromPrivateKey(privateKey []byte, network string) (signing.AccountHandler, error) {
	a := &Account{}
	publicKey, err := key.PrivateKey2PublicKeyECDSA(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to PrivateKey2PublicKey, err=%v", err)
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
