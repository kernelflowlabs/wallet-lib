package util

import (
	"crypto/rand"
	"errors"

	"github.com/kernelflowlabs/wallet-lib/crypto"
)

const maxCipherInput = 1 << 20

func AesCbcEncrypt(data, key, iv []byte) ([]byte, error) {
	if len(data) > maxCipherInput {
		return nil, errors.New("encryption input exceeds size limit")
	}
	return crypto.AesEncryptCBC(data, key, iv)
}

func AesCbcDecrypt(data, key, iv []byte) ([]byte, error) {
	if len(data) > maxCipherInput {
		return nil, errors.New("decryption input exceeds size limit")
	}
	return crypto.AesDecryptCBC(data, key, iv)
}

func AesGcmSeal(data, key, aad []byte) ([]byte, error) {
	if len(data) > maxCipherInput || len(aad) > 65536 {
		return nil, errors.New("encryption input exceeds size limit")
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	sealed, err := crypto.AesEncryptGCM(data, key, nonce, aad)
	if err != nil {
		return nil, err
	}
	return append(nonce, sealed...), nil
}

func AesGcmOpen(data, key, aad []byte) ([]byte, error) {
	if len(data) < 28 || len(data) > maxCipherInput+28 || len(aad) > 65536 {
		return nil, errors.New("invalid encrypted input length")
	}
	return crypto.AesDecryptGCM(data[12:], key, data[:12], aad)
}
