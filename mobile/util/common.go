package util

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"strings"
)

func Sha256Hash(data string) string {
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}

func SignEcdsaEvm(privateKey []byte, hash string) (string, error) {
	hashByte, err := hex.DecodeString(strings.TrimPrefix(hash, "0x"))
	if err != nil {
		return "", err
	}
	sig, err := key.SignWithPrivateKeyECDSAForEVM(privateKey, hashByte)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sig), nil
}
