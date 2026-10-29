//go:build sol || !custom

package tx

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kernelflowlabs/wallet-lib/session"
	"github.com/kernelflowlabs/wallet-lib/signing/solana"
)

func DescribeSolTransaction(tx []byte, ownerAddress string) string {
	return solana.DescribeSolTransaction(tx, ownerAddress)
}

// SolSignSerializedTransaction signs a transaction a dApp sent, keeping any
// signatures it already carries. The caller decodes base58 or base64 first.
func SolSignSerializedTransaction(handle string, tx []byte, expectedHash string) (string, error) {
	if err := matchesExpectedHash(tx, expectedHash); err != nil {
		return "", err
	}
	var out string
	err := session.WithKey(handle, solana.SessionChain, func(privateKey []byte, _ string) error {
		signed, signErr := solana.SignSerializedTransaction(privateKey, tx)
		if signErr != nil {
			return signErr
		}
		encoded, encErr := json.Marshal(signed)
		if encErr != nil {
			return encErr
		}
		out = string(encoded)
		return nil
	})
	if err != nil {
		return "", err
	}
	return out, nil
}

// SolSignMessage signs raw bytes. It refuses anything that parses as a Solana
// message, because Solana's signMessage has no domain separator and such a
// signature can be lifted into a transaction.
func SolSignMessage(handle string, message []byte, expectedHash string) (string, error) {
	if err := matchesExpectedHash(message, expectedHash); err != nil {
		return "", err
	}
	var signature string
	err := session.WithKey(handle, solana.SessionChain, func(privateKey []byte, _ string) error {
		sig, signErr := solana.SignMessage(privateKey, message)
		if signErr != nil {
			return signErr
		}
		signature = sig
		return nil
	})
	if err != nil {
		return "", err
	}
	return signature, nil
}

func matchesExpectedHash(payload []byte, expected string) error {
	if expected == "" {
		return nil
	}
	sum := sha256.Sum256(payload)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), strings.TrimPrefix(expected, "0x")) {
		return fmt.Errorf("digest_mismatch: the preview does not describe what would be signed")
	}
	return nil
}

func SolPayloadHash(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
