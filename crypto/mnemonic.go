package crypto

import (
	"fmt"
	"github.com/kernelflowlabs/wallet-lib/crypto/bip"
)

const (
	MnWordNumber12 = 12
	MnWordNumber24 = 24
)

func GenerateMnemonic(wordNumber int) (string, error) {
	if wordNumber != MnWordNumber12 && wordNumber != MnWordNumber24 {
		return "", fmt.Errorf("invalid length")
	}

	entropyBytes, err := bip.NewEntropy(wordNumber / 3 * 32)
	if err != nil {
		return "", fmt.Errorf("failed to NewEntropy, err=%v", err)
	}
	mnemonic, err := bip.NewMnemonic(entropyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to NewMnemonic, err=%v", err)
	}
	return mnemonic, nil
}

func IsValidMnemonic(mn string) bool {
	return bip.IsMnemonicValid(mn)
}
