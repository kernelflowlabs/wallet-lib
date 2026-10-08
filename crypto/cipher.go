package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/go-jose/go-jose/v4"
	"golang.org/x/crypto/scrypt"
)

const (
	MaxJWEPublicKeyBytes = 16 * 1024
	MaxJWEPlaintextBytes = 1024 * 1024
	cipherSaltSize       = 32
	cipherKeySize        = 32
	cipherScryptN        = 32768
	cipherScryptR        = 8
	cipherScryptP        = 1
)

func EncryptText(clearText, password []byte) ([]byte, error) {
	if len(clearText) == 0 || len(password) == 0 {
		return nil, errors.New("invalid input")
	}

	salt := make([]byte, cipherSaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	key, err := scrypt.Key(password, salt, cipherScryptN, cipherScryptR, cipherScryptP, cipherKeySize)
	if err != nil {
		return nil, fmt.Errorf("key derivation failed: %w", err)
	}
	defer func() {
		for i := range key {
			key[i] = 0
		}
	}()

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("nonce generation failed: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, clearText, nil)

	result := make([]byte, len(salt)+len(nonce)+len(ciphertext))
	copy(result, salt)
	copy(result[len(salt):], nonce)
	copy(result[len(salt)+len(nonce):], ciphertext)

	return result, nil
}

func DecryptText(cipherText, password []byte) ([]byte, error) {
	if cipherText == nil || len(password) == 0 {
		return nil, errors.New("invalid input")
	}
	if len(cipherText) < cipherSaltSize+12 {
		return nil, errors.New("invalid encrypted data")
	}

	salt := cipherText[:cipherSaltSize]
	nonce := cipherText[cipherSaltSize : cipherSaltSize+12]
	ciphertext := cipherText[cipherSaltSize+12:]

	key, err := scrypt.Key(password, salt, cipherScryptN, cipherScryptR, cipherScryptP, cipherKeySize)
	if err != nil {
		return nil, fmt.Errorf("key derivation failed: %w", err)
	}
	defer func() {
		for i := range key {
			key[i] = 0
		}
	}()

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.New("decryption failed")
	}

	return plaintext, nil
}

func EncryptJWE(publicKeyPEM, plaintext []byte) ([]byte, error) {
	if len(publicKeyPEM) > MaxJWEPublicKeyBytes || len(plaintext) > MaxJWEPlaintextBytes {
		return nil, errors.New("JWE input exceeds size limit")
	}
	if len(plaintext) == 0 {
		return nil, errors.New("plaintext is empty")
	}
	block, rest := pem.Decode(publicKeyPEM)
	if block == nil || block.Type != "PUBLIC KEY" || strings.TrimSpace(string(rest)) != "" {
		return nil, errors.New("expected one PKIX public key")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, errors.New("invalid public key")
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok || key.N == nil || key.N.BitLen() < 3072 || key.N.BitLen() > 8192 || key.E != 65537 {
		return nil, errors.New("expected RSA public key of 3072 to 8192 bits with exponent 65537")
	}
	id := sha256.Sum256(block.Bytes)
	enc, err := jose.NewEncrypter(jose.A256GCM, jose.Recipient{Algorithm: jose.RSA_OAEP_256, Key: key, KeyID: hex.EncodeToString(id[:])}, nil)
	if err != nil {
		return nil, err
	}
	object, err := enc.Encrypt(plaintext)
	if err != nil {
		return nil, err
	}
	compact, err := object.CompactSerialize()
	if err != nil {
		return nil, err
	}
	return []byte(compact), nil
}
