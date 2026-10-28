package crypto

import (
	"crypto/aes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/kernelflowlabs/wallet-lib/crypto/bip"
	"io"
	"unicode/utf8"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
	"golang.org/x/crypto/scrypt"
)

func WriteToKeyStore(name, text, password string, isMn bool) (string, error) {
	return WriteToKeyStoreBytes(name, []byte(text), []byte(password), isMn)
}

func WriteToKeyStoreBytes(name string, text, password []byte, isMn bool) (string, error) {
	address := "address"
	path := "path"
	coin := 0
	type1 := "any"
	if isMn {
		if !IsValidMnemonic(string(text)) {
			return "", fmt.Errorf("invalid mnemonic")
		}
		coin = 60
		type1 = "mnemonic"
		path = "m/44'/60'/0'/0/0"
		{
			seed, err := bip.NewSeedWithErrorChecking(string(text), "")
			if err != nil {
				return "", fmt.Errorf("failed to NewSeedWithErrorChecking, err=%v", err)
			}
			defer Wipe(seed)

			k, err := bip.SeedToKeyForECDSA(seed, path)
			if err != nil {
				return "", fmt.Errorf("failed to SeedToECDSAKey, err=%v", err)
			}
			defer Wipe(k.Key)
			privateKey := hex.EncodeToString(k.Key)
			ecdsaKey, err := crypto.HexToECDSA(privateKey)
			if err != nil {
				return "", fmt.Errorf("failed to GenerateKey, err=%v", err)
			}

			publicKeyECDSA, ok := ecdsaKey.Public().(*ecdsa.PublicKey)
			if !ok {
				return "", fmt.Errorf("not a ecdsa.PublicKey")
			}
			address = crypto.PubkeyToAddress(*publicKeyECDSA).Hex()
		}
	}

	err := validatePasswordBytes(password)
	if err != nil {
		return "", err
	}

	scryptN := 262144
	scryptP := 1
	scryptR := 8
	dkLen := 32

	salt := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("failed to generate salt")
	}
	derivedKey, err := scrypt.Key(password, salt, scryptN, scryptR, scryptP, dkLen)
	if err != nil {
		return "", err
	}
	defer Wipe(derivedKey)

	encryptKey := derivedKey[:16]

	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", fmt.Errorf("reading from crypto/rand failed, err=%v", err.Error())
	}

	cipherText, err := AesEncryptCTR(text, encryptKey, iv)
	if err != nil {
		return "", err
	}
	mac := crypto.Keccak256(derivedKey[16:32], cipherText)

	activeAccount := ActiveAccount{
		Address:        address,
		Coin:           coin,
		DerivationPath: path,
	}
	cryptoPart := CryptoStruct{
		Cipher: "aes-128-ctr",
		Cipherparams: struct {
			Iv string `json:"iv"`
		}{hex.EncodeToString(iv)},
		Ciphertext: hex.EncodeToString(cipherText),
		Kdf:        "scrypt",
		Kdfparams: struct {
			Dklen int    `json:"dklen"`
			N     int    `json:"n"`
			P     int    `json:"p"`
			R     int    `json:"r"`
			Salt  string `json:"salt"`
		}{dkLen, scryptN, scryptP, scryptR, hex.EncodeToString(salt)},
		Mac: hex.EncodeToString(mac),
	}

	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("could not create random uuid, err=%v", err)
	}
	ks := &KeyStore{
		ActiveAccounts: []ActiveAccount{activeAccount},
		Crypto:         cryptoPart,
		Id:             id.String(),
		Name:           name,
		Type:           type1,
		Version:        3,
	}
	ksBytes, err := json.Marshal(ks)
	if err != nil {
		return "", err
	}
	return string(ksBytes), nil
}

func ReadFromKeyStore(ksString, password string, isMn bool) (string, error) {
	clearText, err := ReadFromKeyStoreBytes(ksString, []byte(password), isMn)
	if err != nil {
		return "", err
	}
	defer Wipe(clearText)
	return string(clearText), nil
}

func ReadFromKeyStoreBytes(ksString string, password []byte, isMn bool) ([]byte, error) {
	ks := &KeyStore{}
	if err := json.Unmarshal([]byte(ksString), ks); err != nil {
		return nil, err
	}
	err := validatePasswordBytes(password)
	if err != nil {
		return nil, err
	}
	if ks.Crypto.Kdf != "scrypt" {
		return nil, fmt.Errorf("unsupported kdf: %s", ks.Crypto.Kdf)
	}
	if ks.Crypto.Cipher != "aes-128-ctr" {
		return nil, fmt.Errorf("unsupported cipher: %s", ks.Crypto.Cipher)
	}

	scryptN := ks.Crypto.Kdfparams.N
	scryptP := ks.Crypto.Kdfparams.P
	scryptR := ks.Crypto.Kdfparams.R
	dkLen := ks.Crypto.Kdfparams.Dklen
	if dkLen != 32 {
		return nil, fmt.Errorf("unsupported dklen: %d", dkLen)
	}
	if err := checkScryptParams(scryptN, scryptR, scryptP); err != nil {
		return nil, err
	}

	salt, err := hex.DecodeString(ks.Crypto.Kdfparams.Salt)
	if err != nil {
		return nil, fmt.Errorf("invalid salt: %w", err)
	}

	derivedKey, err := scrypt.Key(password, salt, scryptN, scryptR, scryptP, dkLen)
	if err != nil {
		return nil, err
	}
	defer Wipe(derivedKey)

	ct, err := hex.DecodeString(ks.Crypto.Ciphertext)
	if err != nil {
		return nil, fmt.Errorf("invalid ciphertext: %w", err)
	}
	expectedMAC, err := hex.DecodeString(ks.Crypto.Mac)
	if err != nil {
		return nil, fmt.Errorf("invalid mac: %w", err)
	}
	computedMAC := crypto.Keccak256(derivedKey[16:32], ct)
	if subtle.ConstantTimeCompare(computedMAC, expectedMAC) != 1 {
		return nil, fmt.Errorf("invalid password or corrupted keystore (MAC mismatch)")
	}

	iv, err := hex.DecodeString(ks.Crypto.Cipherparams.Iv)
	if err != nil {
		return nil, fmt.Errorf("invalid iv: %w", err)
	}
	encryptKey := derivedKey[:16]
	clearText, err := AesDecryptCTR(ct, encryptKey, iv)
	if err != nil {
		return nil, err
	}

	if isMn {
		if !IsValidMnemonic(string(clearText)) {
			Wipe(clearText)
			return nil, fmt.Errorf("got invalid mnemonics")
		}
	}
	return clearText, nil
}

func ReadFromGethKeyStore(ksString string, password string) (string, error) {
	ks := &GethKeyStore{}
	if err := json.Unmarshal([]byte(ksString), ks); err != nil {
		return "", err
	}
	if ks.Crypto.Kdf != "scrypt" {
		return "", fmt.Errorf("unsupported kdf: %s", ks.Crypto.Kdf)
	}
	if ks.Crypto.Cipher != "aes-128-ctr" {
		return "", fmt.Errorf("unsupported cipher: %s", ks.Crypto.Cipher)
	}
	dkLen := ks.Crypto.Kdfparams.Dklen
	if dkLen != 32 {
		return "", fmt.Errorf("unsupported dklen: %d", dkLen)
	}

	salt, err := hex.DecodeString(ks.Crypto.Kdfparams.Salt)
	if err != nil {
		return "", fmt.Errorf("invalid salt: %w", err)
	}
	if err := checkScryptParams(ks.Crypto.Kdfparams.N, ks.Crypto.Kdfparams.R, ks.Crypto.Kdfparams.P); err != nil {
		return "", err
	}
	derivedKey, err := scrypt.Key([]byte(password), salt,
		ks.Crypto.Kdfparams.N, ks.Crypto.Kdfparams.R, ks.Crypto.Kdfparams.P, dkLen)
	if err != nil {
		return "", err
	}
	defer func() {
		for i := range derivedKey {
			derivedKey[i] = 0
		}
	}()

	ciphertext, err := hex.DecodeString(ks.Crypto.Ciphertext)
	if err != nil {
		return "", fmt.Errorf("invalid ciphertext: %w", err)
	}
	expectedMAC, err := hex.DecodeString(ks.Crypto.Mac)
	if err != nil {
		return "", fmt.Errorf("invalid mac: %w", err)
	}
	computedMAC := crypto.Keccak256(derivedKey[16:32], ciphertext)
	if subtle.ConstantTimeCompare(computedMAC, expectedMAC) != 1 {
		return "", fmt.Errorf("invalid password or corrupted keystore (MAC mismatch)")
	}

	iv, err := hex.DecodeString(ks.Crypto.Cipherparams.Iv)
	if err != nil {
		return "", fmt.Errorf("invalid iv: %w", err)
	}
	clearText, err := AesDecryptCTR(ciphertext, derivedKey[:16], iv)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(clearText), nil
}

func validatePassword(pw string) error {
	if len([]rune(pw)) < 8 {
		return fmt.Errorf("password too short; use at least 8 characters")
	}
	return nil
}

func validatePasswordBytes(pw []byte) error {
	if utf8.RuneCount(pw) < 8 {
		return fmt.Errorf("password too short; use at least 8 characters")
	}
	return nil
}

type (
	ActiveAccount struct {
		Address        string `json:"address"`
		Coin           int    `json:"coin"`
		DerivationPath string `json:"derivationPath"`
	}
	CryptoStruct struct {
		Cipher       string `json:"cipher"`
		Cipherparams struct {
			Iv string `json:"iv"`
		} `json:"cipherparams"`
		Ciphertext string `json:"ciphertext"`
		Kdf        string `json:"kdf"`
		Kdfparams  struct {
			Dklen int    `json:"dklen"`
			N     int    `json:"n"`
			P     int    `json:"p"`
			R     int    `json:"r"`
			Salt  string `json:"salt"`
		} `json:"kdfparams"`
		Mac string `json:"mac"`
	}

	KeyStore struct {
		ActiveAccounts []ActiveAccount `json:"activeAccounts"`
		Crypto         CryptoStruct    `json:"crypto"`
		Id             string          `json:"id"`
		Name           string          `json:"name"`
		Type           string          `json:"type"`
		Version        int             `json:"version"`
	}

	GethKeyStore struct {
		Address string       `json:"address"`
		Crypto  CryptoStruct `json:"crypto"`
		Id      string       `json:"id"`
		Version int          `json:"version"`
	}
)

func checkScryptParams(n, r, p int) error {
	if n <= 1 || n&(n-1) != 0 {
		return fmt.Errorf("invalid scrypt N: %d", n)
	}
	if n > 1<<20 {
		return fmt.Errorf("scrypt N too large: %d", n)
	}
	if r < 1 || r > 16 {
		return fmt.Errorf("invalid scrypt r: %d", r)
	}
	if p < 1 || p > 16 {
		return fmt.Errorf("invalid scrypt p: %d", p)
	}
	return nil
}
