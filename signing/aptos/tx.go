package aptos

import (
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/kernelflowlabs/wallet-lib/signing"

	"golang.org/x/crypto/sha3"
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

	sender, err := parseHexAddress(tx.Ingredient.Sender)
	if err != nil {
		return fmt.Errorf("failed to parse sender %s, err=%v", tx.Ingredient.Sender, err)
	}
	if tx.Ingredient.Recipient == signing.MagicContactAddressForNative {
		return fmt.Errorf("recipient must not be the native asset sentinel")
	}
	recipient, err := parseHexAddress(tx.Ingredient.Recipient)
	if err != nil {
		return fmt.Errorf("failed to parse recipient %s, err=%v", tx.Ingredient.Recipient, err)
	}
	nonce, _ := strconv.ParseUint(tx.Ingredient.Nonce, 10, 64)
	gasPrice, _ := strconv.ParseUint(tx.Ingredient.GasPrice, 10, 64)
	gasLimit, _ := strconv.ParseUint(tx.Ingredient.GasLimit, 10, 64)
	expirationTimestamp, _ := strconv.ParseUint(tx.Ingredient.LedgerInfoParams.ExpirationTimestamp, 10, 64)
	chainId, _ := strconv.ParseUint(tx.Ingredient.ChainId, 10, 64)

	var token typeTagStruct
	if tx.Ingredient.ContractAddress != signing.MagicContactAddressForNative {
		token, err = parseTypeTagStruct(tx.Ingredient.ContractAddress)
		if err != nil {
			return fmt.Errorf("failed to parse contract %s, err=%v", tx.Ingredient.ContractAddress, err)
		}
	}

	var amount []byte
	if tx.Ingredient.Amount != "" {
		amountUint64, _ := strconv.ParseUint(tx.Ingredient.Amount, 10, 64)
		amount = encodeU64Arg(amountUint64)
	}

	var payload entryFunction
	switch tx.Ingredient.TxType {
	case signing.TxTypeTransfer:
		if tx.Ingredient.ContractAddress == signing.MagicContactAddressForNative {
			addr, name, _ := parseModuleId("0x1::aptos_account")
			payload = entryFunction{
				moduleAddr: addr,
				moduleName: name,
				function:   "transfer",
				typeArgs:   nil,
				args:       [][]byte{recipient[:], amount},
			}
		} else {
			addr, name, _ := parseModuleId("0x1::coin")
			payload = entryFunction{
				moduleAddr: addr,
				moduleName: name,
				function:   "transfer",
				typeArgs:   []typeTag{token},
				args:       [][]byte{recipient[:], amount},
			}
		}
	case signing.TxTypeMint:
		addr, name, _ := parseModuleId("0x1::managed_coin")
		payload = entryFunction{
			moduleAddr: addr,
			moduleName: name,
			function:   "mint",
			typeArgs:   []typeTag{token},
			args:       [][]byte{recipient[:], amount},
		}
	case signing.TxTypeBurn:
		addr, name, _ := parseModuleId("0x1::managed_coin")
		payload = entryFunction{
			moduleAddr: addr,
			moduleName: name,
			function:   "burn",
			typeArgs:   []typeTag{token},
			args:       [][]byte{amount},
		}
	case signing.TxTypeAccountActivate:
		addr, name, _ := parseModuleId("0x1::managed_coin")
		payload = entryFunction{
			moduleAddr: addr,
			moduleName: name,
			function:   "register",
			typeArgs:   []typeTag{token},
			args:       [][]byte{},
		}
	default:
		return fmt.Errorf("invalid txType")
	}

	raw := rawTransaction{
		sender:                  sender,
		sequenceNumber:          nonce,
		payload:                 payload,
		maxGasAmount:            gasLimit,
		gasUnitPrice:            gasPrice,
		expirationTimestampSecs: expirationTimestamp,
		chainId:                 uint8(chainId),
	}
	rawBytes := raw.encode()
	tx.unsignedHex = hex.EncodeToString(rawBytes)

	prefix := sha3.Sum256([]byte(rawTransactionSalt))
	signingMessage := make([]byte, 0, len(prefix)+len(rawBytes))
	signingMessage = append(signingMessage, prefix[:]...)
	signingMessage = append(signingMessage, rawBytes...)
	tx.sigHash = append(tx.sigHash, hex.EncodeToString(signingMessage))
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
	if isDerFormat {
		return "", fmt.Errorf("DER signature format not supported")
	}

	rawBytes, err := hex.DecodeString(tx.unsignedHex)
	if err != nil {
		return "", fmt.Errorf("failed to DecodeString for unsignedHex, err=%v", err)
	}
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return "", fmt.Errorf("failed to DecodeString for Signature, err=%v", err)
	}
	pubBytes, err := hex.DecodeString(tx.SenderPublicKey)
	if err != nil {
		return "", fmt.Errorf("failed to DecodeString for SenderPublicKey, err=%v", err)
	}
	if len(pubBytes) != 32 {
		return "", fmt.Errorf("invalid public key length %d", len(pubBytes))
	}
	if len(sigBytes) != 64 {
		return "", fmt.Errorf("invalid signature length %d", len(sigBytes))
	}

	signedBytes := encodeSignedTransaction(rawBytes, pubBytes, sigBytes)

	prefix := sha3.Sum256([]byte(transactionSalt))
	message := make([]byte, 0, len(prefix)+1+len(signedBytes))
	message = append(message, prefix[:]...)
	message = append(message, 0x00)
	message = append(message, signedBytes...)
	hash := sha3.Sum256(message)
	tx.txHash = "0x" + hex.EncodeToString(hash[:])

	return hex.EncodeToString(signedBytes), nil
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
	LedgerInfoParams struct {
		ExpirationTimestamp string `json:"expirationTimestamp" validate:"required,u64_gt0"`
		ChainId             string `json:"chainId" validate:"required,u64"`
	}
	Ingredient struct {
		TxType          string `json:"txType" validate:"required,oneof=0 1 2 3 4 5 6"`
		ContractAddress string `json:"contractAddress" validate:"required"`
		Sender          string `json:"sender" validate:"required,apt_addr"`
		SenderPublicKey string `json:"senderPublicKey" validate:"required,hex_str"`
		Recipient       string `json:"recipient" validate:"required,apt_addr"`
		Amount          string `json:"amount,omitempty" validate:"omitempty,u64_gt0"`
		Nonce           string `json:"nonce" validate:"required,u64"`
		GasPrice        string `json:"gasPrice" validate:"required,u64_gt0"`
		GasLimit        string `json:"gasLimit" validate:"required,u64_gt0"`
		*LedgerInfoParams
	}
	TxBuilder struct {
		*Ingredient
		unsignedHex string
		sigHash     []string
		txHash      string
	}
)
