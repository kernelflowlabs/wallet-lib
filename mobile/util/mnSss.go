package util

import (
	"encoding/hex"
	"errors"
	"strings"

	"github.com/kernelflowlabs/wallet-lib/crypto"
)

const (
	ThresholdShares = 2
	TotalShares     = 3
)

func SssSplit(mnemonic string) (string, error) {
	shares, err := crypto.PerformSplit(mnemonic, TotalShares, ThresholdShares)
	if err != nil {
		return "", err
	}
	return strings.Join(shares, ","), nil
}

func SssRecover(shares string) (string, error) {
	return recoverShares(shares, TotalShares)
}

func SssRecoverLegacy(shares string) (string, error) {
	return recoverShares(shares, 2)
}

func recoverShares(shares string, maxShares int) (string, error) {
	if len(shares) > 16384 {
		return "", errors.New("shares exceed size limit")
	}
	var clean []string
	for _, line := range strings.Split(shares, ",") {
		s := strings.TrimSpace(line)
		if s == "" {
			return "", errors.New("empty share")
		}
		clean = append(clean, s)
	}
	if len(clean) < 2 || len(clean) > maxShares {
		return "", errors.New("invalid share count")
	}
	return crypto.PerformRecover(clean)
}

func SssEncryptJWE(publicKeyPEM, shares string) (string, error) {
	if len(publicKeyPEM) > crypto.MaxJWEPublicKeyBytes || len(shares) > crypto.MaxJWEPlaintextBytes {
		return "", errors.New("JWE input exceeds size limit")
	}
	data := []byte(shares)
	defer clear(data)
	encrypted, err := crypto.EncryptJWE([]byte(publicKeyPEM), data)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(encrypted), nil
}
