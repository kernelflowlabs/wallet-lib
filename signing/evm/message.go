package evm

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/kernelflowlabs/wallet-lib/crypto/key"
)

const MaxPersonalMessageBytes = 1 << 20

func PersonalSignDigest(message []byte) []byte {
	return accounts.TextHash(message)
}

func PersonalSign(privateKey, message []byte) ([]byte, error) {
	if len(message) > MaxPersonalMessageBytes {
		return nil, fmt.Errorf("message is %d bytes, limit is %d", len(message), MaxPersonalMessageBytes)
	}
	sig, err := key.SignWithPrivateKeyECDSAForEVM(privateKey, PersonalSignDigest(message))
	if err != nil {
		return nil, fmt.Errorf("sign personal message: %w", err)
	}
	if len(sig) != 65 {
		return nil, fmt.Errorf("unexpected signature length %d", len(sig))
	}
	if sig[64] < 27 {
		sig[64] += 27
	}
	return sig, nil
}
