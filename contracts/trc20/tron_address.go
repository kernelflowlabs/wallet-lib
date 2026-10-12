package trc20

import (
	"encoding/hex"
	"fmt"

	"github.com/btcsuite/btcd/address/v2/base58"
	"github.com/ethereum/go-ethereum/common"
)

func ConvertToBytes(address string) []byte {
	result, version, err := base58.CheckDecode(address)
	if err != nil {
		return nil
	}
	if version != 0x41 || len(result) != 20 {
		return nil
	}
	return append([]byte{version}, result...)
}

func ConvertToHex(address string) (string, error) {
	payload := ConvertToBytes(address)
	if payload == nil {
		return "", fmt.Errorf("invalid tron address %q", address)
	}
	return hex.EncodeToString(payload[1:]), nil
}

func toAddress(address string) (common.Address, error) {
	if address == "" {
		return common.Address{}, nil
	}
	h, err := ConvertToHex(address)
	if err != nil {
		return common.Address{}, err
	}
	return common.HexToAddress(h), nil
}
