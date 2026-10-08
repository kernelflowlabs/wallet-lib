package shamir

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
)

const MaxShares = 255

func Split(secret []byte, total, threshold int) ([][]byte, error) {
	if len(secret) == 0 {
		return nil, errors.New("cannot split an empty secret")
	}
	if threshold < 2 || threshold > MaxShares {
		return nil, errors.New("threshold must be between 2 and 255")
	}
	if total < threshold || total > MaxShares {
		return nil, errors.New("total shares must be between threshold and 255")
	}

	coordinates, err := randomCoordinates(total)
	if err != nil {
		return nil, fmt.Errorf("generate share coordinates: %w", err)
	}
	shares := make([][]byte, total)
	for i, coordinate := range coordinates {
		shares[i] = make([]byte, len(secret)+1)
		shares[i][len(secret)] = coordinate
	}

	coefficients := make([]byte, threshold)
	defer clear(coefficients)
	for offset, value := range secret {
		coefficients[0] = value
		if _, err := io.ReadFull(rand.Reader, coefficients[1:]); err != nil {
			return nil, fmt.Errorf("generate polynomial: %w", err)
		}
		for i, coordinate := range coordinates {
			shares[i][offset] = evaluate(coefficients, coordinate)
		}
	}
	return shares, nil
}

func Combine(shares [][]byte) ([]byte, error) {
	if len(shares) < 2 || len(shares) > MaxShares {
		return nil, errors.New("between 2 and 255 shares required")
	}
	shareLength := len(shares[0])
	if shareLength < 2 {
		return nil, errors.New("shares must contain data and a coordinate")
	}

	coordinates := make([]byte, len(shares))
	seen := make(map[byte]bool, len(shares))
	for i, share := range shares {
		if len(share) != shareLength {
			return nil, errors.New("share lengths differ")
		}
		coordinate := share[shareLength-1]
		if coordinate == 0 || seen[coordinate] {
			return nil, errors.New("invalid or duplicate share coordinate")
		}
		seen[coordinate] = true
		coordinates[i] = coordinate
	}

	secret := make([]byte, shareLength-1)
	values := make([]byte, len(shares))
	for offset := range secret {
		for i, share := range shares {
			values[i] = share[offset]
		}
		secret[offset] = interpolateAtZero(coordinates, values)
	}
	clear(values)
	return secret, nil
}

func randomCoordinates(total int) ([]byte, error) {
	pool := make([]byte, MaxShares)
	for i := range pool {
		pool[i] = byte(i + 1)
	}
	for i := len(pool) - 1; i > 0; i-- {
		j, err := randomIndex(i + 1)
		if err != nil {
			return nil, err
		}
		pool[i], pool[j] = pool[j], pool[i]
	}
	return pool[:total], nil
}

func randomIndex(limit int) (int, error) {
	ceiling := 256 - 256%limit
	var value [1]byte
	for {
		if _, err := io.ReadFull(rand.Reader, value[:]); err != nil {
			return 0, err
		}
		if int(value[0]) < ceiling {
			return int(value[0]) % limit, nil
		}
	}
}

func evaluate(coefficients []byte, x byte) byte {
	result := coefficients[len(coefficients)-1]
	for i := len(coefficients) - 2; i >= 0; i-- {
		result = multiply(result, x) ^ coefficients[i]
	}
	return result
}

func interpolateAtZero(coordinates, values []byte) byte {
	var result byte
	for i := range coordinates {
		numerator := byte(1)
		denominator := byte(1)
		for j := range coordinates {
			if i == j {
				continue
			}
			numerator = multiply(numerator, coordinates[j])
			denominator = multiply(denominator, coordinates[i]^coordinates[j])
		}
		result ^= multiply(values[i], divide(numerator, denominator))
	}
	return result
}

func divide(value, divisor byte) byte {
	return multiply(value, power(divisor, 254))
}

func power(value byte, exponent uint16) byte {
	result := byte(1)
	for exponent > 0 {
		mask := byte(0) - byte(exponent&1)
		result = (result &^ mask) | (multiply(result, value) & mask)
		value = multiply(value, value)
		exponent >>= 1
	}
	return result
}

func multiply(left, right byte) byte {
	var result byte
	for range 8 {
		result ^= left & (byte(0) - (right & 1))
		high := left >> 7
		left <<= 1
		left ^= 0x1b & (byte(0) - high)
		right >>= 1
	}
	return result
}
