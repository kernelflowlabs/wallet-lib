package multiversx

import (
	"github.com/kernelflowlabs/wallet-lib/contracts/esdt"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/kernelflowlabs/wallet-lib/signing"
)

func NewTxBuilder(ti *Ingredient) *TxBuilder {
	return &TxBuilder{
		Ingredient: ti,
	}
}

func (tx *TxBuilder) Build() error {
	if tx == nil {
		return fmt.Errorf("tx == nil")
	}
	tx.sigHash = nil
	tx.unsignedHex = ""
	tx.txHash = ""
	if err := signing.Validator.Struct(tx.Ingredient); err != nil {
		return fmt.Errorf("invalid ingredient: %v", err)
	}

	nonce, _ := strconv.ParseUint(tx.Ingredient.Nonce, 10, 64)
	gasPrice, _ := strconv.ParseUint(tx.Ingredient.GasPrice, 10, 64)
	gasLimit, _ := strconv.ParseUint(tx.Ingredient.GasLimit, 10, 64)
	version, _ := strconv.ParseUint(tx.Ingredient.Version, 10, 64)

	value := "0"
	var payload string
	var err error
	switch tx.Ingredient.TxType {
	case signing.TxTypeTransfer:
		if tx.Ingredient.ContractAddress == signing.MagicContactAddressForNative {
			value = tx.Ingredient.Amount
		} else {
			payload, err = esdt.PackPayloadForESDT(tx.Ingredient.ContractAddress, tx.Ingredient.Amount)
			if err != nil {
				return fmt.Errorf("failed to esdt.PackPayloadForESDT, err=%v", err)
			}
		}
	default:
		return fmt.Errorf("invalid txType")
	}
	ntx := &nativeTx{
		Value:    value,
		Nonce:    nonce,
		RcvAddr:  tx.Ingredient.Recipient,
		SndAddr:  tx.Ingredient.Sender,
		GasPrice: gasPrice,
		GasLimit: gasLimit,
		Data:     base64.StdEncoding.EncodeToString([]byte(payload)),
		ChainID:  tx.Ingredient.ChainID,
		Version:  uint32(version),
	}
	ntxBytes, err := json.Marshal(ntx)
	if err != nil {
		return fmt.Errorf("failed to Marshal, err=%v", err)
	}
	tx.unsignedHex = hex.EncodeToString(ntxBytes)
	tx.sigHash = append(tx.sigHash, tx.unsignedHex)
	return nil
}

func (tx *TxBuilder) Sign(privateKey []byte) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("tx == nil")
	} else if len(tx.sigHash) != 1 {
		return "", fmt.Errorf("tx.SigHash == nil")
	}
	signingMessage, err := hex.DecodeString(tx.sigHash[0])
	if err != nil {
		return "", fmt.Errorf("failed to DecodeString for SigHash, err=%v", err)
	}
	signatureBytes, err := key.SignWithPrivateKeyED25519(privateKey, signingMessage)
	if err != nil {
		return "", fmt.Errorf("failed to SignWithPrivateKeyED25519, err=%v", err)
	}
	return hex.EncodeToString(signatureBytes), nil
}

func (tx *TxBuilder) ConcatSignature(signature string, isDerFormat bool) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("tx == nil")
	} else if tx.unsignedHex == "" {
		return "", fmt.Errorf("tx.UnsignedHex == nil")
	}

	ntxBytes, err := hex.DecodeString(tx.unsignedHex)
	if err != nil {
		return "", fmt.Errorf("failed to DecodeString for UnsignedHex, err=%v", err)
	}
	var ntx nativeTx
	err = json.Unmarshal(ntxBytes, &ntx)
	if err != nil {
		return "", fmt.Errorf("failed to Unmarshal, err=%v", err)
	}

	if isDerFormat {
		return "", fmt.Errorf("DER signature format not supported")
	}
	sig := signature
	ntx.Signature = sig
	ntxBytes, err = json.Marshal(ntx)
	if err != nil {
		return "", fmt.Errorf("failed to Marshal for ntx, err=%v", err)
	}

	tx.txHash, err = computeTxHash(tx, signature)
	if err != nil {
		return "", fmt.Errorf("failed to ComputeTxHash, err=%v", err)
	}
	return hex.EncodeToString(ntxBytes), nil
}

func (tx *TxBuilder) GetTxHash() string {
	return tx.txHash
}

func (tx *TxBuilder) GetSigHash() []string {
	return tx.sigHash
}

func (tx *TxBuilder) GetUnsignedHex() string {
	return tx.unsignedHex
}

func (tx *TxBuilder) SetSigHash(sigHash []string) {
	tx.sigHash = sigHash
}

func (tx *TxBuilder) SetUnsignedHex(unsignedHex string) {
	tx.unsignedHex = unsignedHex
}

type (
	NetWorkConfig struct {
		GasPrice string `json:"gasPrice" validate:"required,u64_gt0"`
		GasLimit string `json:"gasLimit" validate:"required,u64_gt0"`
		ChainID  string `json:"chainID"  validate:"required,alphanum"`
		Version  string `json:"version"  validate:"required,u64"`
	}
	Ingredient struct {
		TxType          string `json:"txType" validate:"required,oneof=0 1 2 3 4 5 6"`
		ContractAddress string `json:"contractAddress" validate:"required"`
		Sender          string `json:"sender" validate:"required,egld_addr"`
		Recipient       string `json:"recipient" validate:"required,egld_addr"`
		Amount          string `json:"amount" validate:"required,bigint_gt0"`
		Nonce           string `json:"nonce" validate:"required,bigint_gte0"`
		*NetWorkConfig  `validate:"required"`
	}

	TxBuilder struct {
		*Ingredient
		unsignedHex string
		sigHash     []string
		txHash      string
	}
	nativeTx struct {
		Nonce     uint64 `json:"nonce"`
		Value     string `json:"value"`
		RcvAddr   string `json:"receiver"`
		SndAddr   string `json:"sender"`
		GasPrice  uint64 `json:"gasPrice,omitempty"`
		GasLimit  uint64 `json:"gasLimit,omitempty"`
		Data      string `json:"data,omitempty"`
		Signature string `json:"signature,omitempty"`
		ChainID   string `json:"chainID"`
		Version   uint32 `json:"version"`
	}
)
