package kaspa

import (
	"github.com/kernelflowlabs/wallet-lib/signing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"golang.org/x/crypto/blake2b"
)

func PublicKey2Address(publicKey []byte) string {
	if len(publicKey) == 33 {
		publicKey = publicKey[1:]
	}
	if len(publicKey) != 32 {
		return ""
	}
	prefix := bech32PrefixKaspaMainnet
	address := encodeAddress(prefix, publicKey, pubKeyAddrID)
	if address == "" {
		return ""
	}
	return correctAddress(address)
}

func ValidAddress(address string) bool {
	if address == signing.MagicContactAddressForNative {
		return true
	}
	_, _, err := decodeAddress(address, bech32PrefixKaspaMainnet)
	if err != nil {
		return false
	}
	return true
}

func VerifySignatureWithBlake2b(publicKey, data, sig []byte) bool {
	pub, err := schnorr.ParsePubKey(publicKey)
	if err != nil {
		return false
	}
	hash := blake2b.Sum256(data)
	signature, err := schnorr.ParseSignature(sig)
	if err != nil {
		return false
	}
	return signature.Verify(hash[:], pub)
}

func SignWithPrivateKeyWithBlake2b(privateKeyBytes, data []byte) ([]byte, error) {
	hash := blake2b.Sum256(data)
	prvKey, _ := btcec.PrivKeyFromBytes(privateKeyBytes)
	signature, err := schnorr.Sign(prvKey, hash[:])
	if err != nil {
		return nil, err
	}
	return signature.Serialize(), err
}

const (
	addressPublicKeyScriptPublicKeyVersion      = 0
	addressPublicKeyECDSAScriptPublicKeyVersion = 0
	addressScriptHashScriptPublicKeyVersion     = 0
)

func init() {
	if err := signing.RegisterAddressValidator("kas_addr", ValidAddress); err != nil {
		panic(err)
	}
}
