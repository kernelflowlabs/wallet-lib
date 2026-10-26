package evm

import (
	"encoding/json"
	"fmt"

	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

func SignTypedDataJSON(privateKey []byte, typedDataJSON []byte) ([]byte, error) {
	digest, err := HashTypedDataJSON(typedDataJSON)
	if err != nil {
		return nil, err
	}
	sig, err := key.SignWithPrivateKeyECDSAForEVM(privateKey, digest)
	if err != nil {
		return nil, fmt.Errorf("sign EIP-712 digest: %w", err)
	}
	if len(sig) != 65 {
		return nil, fmt.Errorf("unexpected sig len %d", len(sig))
	}
	if sig[64] < 27 {
		sig[64] += 27
	}
	return sig, nil
}

func HashTypedDataJSON(typedDataJSON []byte) ([]byte, error) {
	var td apitypes.TypedData
	if err := json.Unmarshal(typedDataJSON, &td); err != nil {
		return nil, fmt.Errorf("parse typed data: %w", err)
	}
	if td.PrimaryType == "" {
		return nil, fmt.Errorf("typed data missing primaryType")
	}
	domainSep, err := td.HashStruct("EIP712Domain", td.Domain.Map())
	if err != nil {
		return nil, fmt.Errorf("hash EIP712Domain: %w", err)
	}
	structHash, err := td.HashStruct(td.PrimaryType, td.Message)
	if err != nil {
		return nil, fmt.Errorf("hash %s: %w", td.PrimaryType, err)
	}
	return EIP712Hash(domainSep, structHash), nil
}
