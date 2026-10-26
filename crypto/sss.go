package crypto

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/kernelflowlabs/wallet-lib/crypto/bip"
	"github.com/kernelflowlabs/wallet-lib/crypto/internal/shamir"
)

func SSSSplit(data []byte, n, t int) ([][]byte, error) {
	hash := sha256.Sum256(data)
	payload := append(data, hash[:4]...)
	return shamir.Split(payload, n, t)
}

func SSSCombine(shares [][]byte) ([]byte, error) {
	combined, err := shamir.Combine(shares)
	if err != nil {
		return nil, err
	}

	if len(combined) < 4 {
		return nil, errors.New("invalid combined data")
	}

	data := combined[:len(combined)-4]
	checksum := combined[len(combined)-4:]

	hash := sha256.Sum256(data)
	for i := 0; i < 4; i++ {
		if hash[i] != checksum[i] {
			return nil, errors.New("checksum mismatch")
		}
	}

	return data, nil
}

func PerformSplit(str string, totalShares, thresholdShares int) ([]string, error) {
	entropy, err := bip.EntropyFromMnemonic(str)
	if err != nil {
		return nil, fmt.Errorf("invalid str: %v", err)
	}

	fullHash := sha256.Sum256(entropy)
	fingerprint := hex.EncodeToString(fullHash[:4])

	shares, err := SSSSplit(entropy, totalShares, thresholdShares)
	if err != nil {
		return nil, err
	}

	result := make([]string, len(shares))
	for i, s := range shares {
		result[i] = fmt.Sprintf("%s-%d-%s", fingerprint, i, hex.EncodeToString(s))
	}
	return result, nil
}

func PerformRecover(formattedShares []string) (string, error) {
	if len(formattedShares) < 2 || len(formattedShares) > 255 {
		return "", errors.New("between 2 and 255 shares required")
	}

	byteShares := make([][]byte, len(formattedShares))
	var expectedFingerprint string
	indexes := make(map[int]bool, len(formattedShares))
	coordinates := make(map[byte]bool, len(formattedShares))

	for i, s := range formattedShares {
		if len(s) > 87 {
			return "", errors.New("share exceeds size limit")
		}
		parts := strings.Split(s, "-")
		if len(parts) != 3 {
			return "", fmt.Errorf("invalid shard format at index %d", i)
		}

		fingerprint, err := hex.DecodeString(parts[0])
		if err != nil || len(fingerprint) != 4 {
			return "", errors.New("invalid share fingerprint")
		}
		index, err := strconv.Atoi(parts[1])
		if err != nil || index < 0 || index > 254 || strconv.Itoa(index) != parts[1] || indexes[index] {
			return "", errors.New("invalid or duplicate share index")
		}
		indexes[index] = true
		currentFingerprint := hex.EncodeToString(fingerprint)
		if i == 0 {
			expectedFingerprint = currentFingerprint
		} else if currentFingerprint != expectedFingerprint {
			return "", errors.New("shards belong to different sets (fingerprint mismatch)")
		}

		b, err := hex.DecodeString(parts[2])
		if err != nil {
			return "", fmt.Errorf("invalid hex data at index %d", i)
		}
		switch len(b) {
		case 21, 25, 29, 33, 37:
		default:
			return "", errors.New("invalid share payload length")
		}
		if i > 0 && len(b) != len(byteShares[0]) {
			return "", errors.New("share payload lengths differ")
		}
		coordinate := b[len(b)-1]
		if coordinate == 0 || coordinates[coordinate] {
			return "", errors.New("invalid or duplicate share coordinate")
		}
		coordinates[coordinate] = true
		byteShares[i] = b
	}

	entropy, err := SSSCombine(byteShares)
	if err != nil {
		return "", err
	}

	fullHash := sha256.Sum256(entropy)
	actualFingerprint := hex.EncodeToString(fullHash[:4])
	if actualFingerprint != expectedFingerprint {
		return "", errors.New("recovered data fingerprint mismatch")
	}

	return bip.NewMnemonic(entropy)
}
