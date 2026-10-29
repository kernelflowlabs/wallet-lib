package key

import (
	"fmt"
	"github.com/kernelflowlabs/wallet-lib/crypto"

	schnorrkel "github.com/ChainSafe/go-schnorrkel"
	"github.com/kernelflowlabs/wallet-lib/crypto/bip"
)

func DerivePrivateKeyECDSABytes(mnemonic []byte, path string) ([]byte, error) {
	if len(mnemonic) == 0 {
		return nil, fmt.Errorf("mnemonic is required")
	}
	seed, err := bip.NewSeedWithErrorCheckingBytes(mnemonic, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to NewSeedWithErrorChecking, err=%v", err)
	}
	defer crypto.Wipe(seed)
	k, err := bip.SeedToKeyForECDSA(seed, path)
	if err != nil {
		return nil, fmt.Errorf("failed to SeedToKeyForECDSA, err=%v", err)
	}
	return k.Key, nil
}

func DerivePrivateKeyED25519Bytes(mnemonic []byte, path string) ([]byte, error) {
	if len(mnemonic) == 0 {
		return nil, fmt.Errorf("mnemonic is required")
	}
	seed, err := bip.NewSeedWithErrorCheckingBytes(mnemonic, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to NewSeedWithErrorChecking, err=%v", err)
	}
	defer crypto.Wipe(seed)
	k, err := bip.SeedToKeyForED25519(seed, path)
	if err != nil {
		return nil, fmt.Errorf("failed to SeedToKeyForED25519, err=%v", err)
	}
	return k.Key, nil
}

func DerivePrivateKeySR25519Bytes(mnemonic []byte, path string) ([]byte, error) {
	return derivePrivateKeySR25519Bytes(mnemonic, path, substrateJunctionID)
}

func DerivePrivateKeySR25519StandardBytes(mnemonic []byte, path string) ([]byte, error) {
	return derivePrivateKeySR25519Bytes(mnemonic, path, substrateJunctionIDStandard)
}

func derivePrivateKeySR25519Bytes(mnemonic []byte, path string, junctionID func(string) []byte) ([]byte, error) {
	if len(mnemonic) == 0 {
		return nil, fmt.Errorf("mnemonic is required")
	}
	seed, err := bip.NewSeedFromMnemonicSr25519Bytes(mnemonic, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to NewSeedFromMnemonicSr25519: %w", err)
	}
	defer crypto.Wipe(seed[:])

	var secretBytes [32]byte
	copy(secretBytes[:], seed[:32])
	ms, err := schnorrkel.NewMiniSecretKeyFromRaw(secretBytes)
	if err != nil {
		return nil, err
	}

	if path == "" {
		keyBytes := ms.Encode()
		return keyBytes[:], nil
	}

	junctions, err := parseSubstratePath(path, junctionID)
	if err != nil {
		return nil, err
	}
	for _, j := range junctions {
		ms, err = substrateHardDerive(ms, j)
		if err != nil {
			return nil, err
		}
	}

	keyBytes := ms.Encode()
	return keyBytes[:], nil
}
