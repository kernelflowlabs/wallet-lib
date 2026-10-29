package bip

import (
	"bytes"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"

	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/text/unicode/norm"
)

func Wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func normalizeMnemonicBytes(mnemonic []byte) []byte {
	joined := bytes.Join(bytes.Fields(mnemonic), []byte(" "))
	normalized := norm.NFKD.Bytes(joined)
	if len(joined) > 0 && len(normalized) > 0 && &normalized[0] != &joined[0] {
		Wipe(joined)
	}
	return normalized
}

func NewSeedBytes(mnemonic, password []byte) []byte {
	m := normalizeMnemonicBytes(mnemonic)
	p := norm.NFKD.Bytes(password)
	salt := make([]byte, 0, len("mnemonic")+len(p))
	salt = append(salt, "mnemonic"...)
	salt = append(salt, p...)
	seed := pbkdf2.Key(m, salt, 2048, 64, sha512.New)
	Wipe(m)
	Wipe(salt)
	if len(p) > 0 && len(password) > 0 && &p[0] != &password[0] {
		Wipe(p)
	}
	return seed
}

func NewSeedWithErrorCheckingBytes(mnemonic, password []byte) ([]byte, error) {
	if _, err := MnemonicToByteArrayBytes(mnemonic); err != nil {
		return nil, err
	}
	return NewSeedBytes(mnemonic, password), nil
}

func splitMnemonicWordsBytes(mnemonic []byte) ([][]byte, bool) {
	words := bytes.Fields(mnemonic)
	n := len(words)
	if n%3 != 0 || n < 12 || n > 24 {
		return nil, false
	}
	return words, true
}

func EntropyFromMnemonicBytes(mnemonic []byte) ([]byte, error) {
	mnemonicSlice, isValid := splitMnemonicWordsBytes(mnemonic)
	if !isValid {
		return nil, ErrInvalidMnemonic
	}
	b := big.NewInt(0)
	for _, v := range mnemonicSlice {
		index, found := wordMap[string(v)]
		if found == false {
			return nil, fmt.Errorf("word `%v` not found in reverse map", string(v))
		}
		var wordBytes [2]byte
		binary.BigEndian.PutUint16(wordBytes[:], uint16(index))
		b = b.Mul(b, shift11BitsMask)
		b = b.Or(b, big.NewInt(0).SetBytes(wordBytes[:]))
	}

	checksum := big.NewInt(0)
	checksumMask := wordLengthChecksumMasksMapping[len(mnemonicSlice)]
	checksum = checksum.And(b, checksumMask)

	b.Div(b, big.NewInt(0).Add(checksumMask, bigOne))

	entropy := b.Bytes()
	entropy = padByteSlice(entropy, len(mnemonicSlice)/3*4)

	entropyChecksumBytes := computeChecksum(entropy)
	entropyChecksum := big.NewInt(int64(entropyChecksumBytes[0]))
	if l := len(mnemonicSlice); l != 24 {
		checksumShift := wordLengthChecksumShiftMapping[l]
		entropyChecksum.Div(entropyChecksum, checksumShift)
	}

	if checksum.Cmp(entropyChecksum) != 0 {
		return nil, ErrChecksumIncorrect
	}

	return entropy, nil
}

func IsMnemonicValidBytes(mnemonic []byte) bool {
	_, err := EntropyFromMnemonicBytes(mnemonic)
	return err == nil
}

func MnemonicToByteArrayBytes(mnemonic []byte, raw ...bool) ([]byte, error) {
	words, ok := splitMnemonicWordsBytes(mnemonic)
	if !ok {
		return nil, ErrInvalidMnemonic
	}
	for _, w := range words {
		if _, found := wordMap[string(w)]; !found {
			return nil, ErrInvalidMnemonic
		}
	}

	entropyBitSize := len(words) * 11
	checksumBitSize := entropyBitSize % 32
	fullByteSize := (entropyBitSize-checksumBitSize)/8 + 1
	checksumByteSize := fullByteSize - (fullByteSize % 4)

	checksummedEntropy := big.NewInt(0)
	modulo := big.NewInt(2048)
	index := new(big.Int)
	for _, w := range words {
		index.SetInt64(int64(wordMap[string(w)]))
		checksummedEntropy.Mul(checksummedEntropy, modulo)
		checksummedEntropy.Add(checksummedEntropy, index)
	}

	checksumModulo := big.NewInt(0).Exp(bigTwo, big.NewInt(int64(checksumBitSize)), nil)
	rawEntropy := big.NewInt(0).Div(checksummedEntropy, checksumModulo)

	rawEntropyBytes := padByteSlice(rawEntropy.Bytes(), checksumByteSize)
	checksummedEntropyBytes := padByteSlice(checksummedEntropy.Bytes(), fullByteSize)
	newChecksummedEntropyBytes := padByteSlice(addChecksum(rawEntropyBytes), fullByteSize)
	if !compareByteSlices(checksummedEntropyBytes, newChecksummedEntropyBytes) {
		return nil, ErrChecksumIncorrect
	}

	if len(raw) > 0 && raw[0] {
		return rawEntropyBytes, nil
	}
	return checksummedEntropyBytes, nil
}

func NewSeedFromMnemonicSr25519Bytes(mnemonic, password []byte) ([64]byte, error) {
	entropy, err := EntropyFromMnemonicBytes(mnemonic)
	if err != nil {
		return [64]byte{}, err
	}
	if len(entropy) < 16 || len(entropy) > 32 || len(entropy)%4 != 0 {
		return [64]byte{}, errors.New("invalid entropy")
	}
	p := norm.NFKD.Bytes(password)
	salt := make([]byte, 0, len("mnemonic")+len(p))
	salt = append(salt, "mnemonic"...)
	salt = append(salt, p...)
	bz := pbkdf2.Key(entropy, salt, 2048, 64, sha512.New)
	var out [64]byte
	copy(out[:], bz[:64])
	Wipe(entropy)
	Wipe(bz)
	Wipe(salt)
	return out, nil
}
