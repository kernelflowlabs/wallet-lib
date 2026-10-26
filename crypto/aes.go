package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
)

func AesEncryptCTR(data, key, iv []byte) ([]byte, error) {
	aesBlock, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(iv) != aes.BlockSize {
		return nil, fmt.Errorf("invalid iv length %d", len(iv))
	}
	stream := cipher.NewCTR(aesBlock, iv)
	outText := make([]byte, len(data))
	stream.XORKeyStream(outText, data)
	return outText, nil
}

func AesDecryptCTR(data, key, iv []byte) ([]byte, error) {
	return AesEncryptCTR(data, key, iv)
}

func AesEncryptGCM(data, key, nonce, aad []byte) ([]byte, error) {
	gcm, err := newGCM(key, nonce)
	if err != nil {
		return nil, err
	}
	return gcm.Seal(nil, nonce, data, aad), nil
}

func AesDecryptGCM(data, key, nonce, aad []byte) ([]byte, error) {
	gcm, err := newGCM(key, nonce)
	if err != nil {
		return nil, err
	}
	out, err := gcm.Open(nil, nonce, data, aad)
	if err != nil {
		return nil, fmt.Errorf("invalid authenticated ciphertext")
	}
	return out, nil
}

func newGCM(key, nonce []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("invalid AES key")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, fmt.Errorf("invalid GCM nonce length")
	}
	return gcm, nil
}
