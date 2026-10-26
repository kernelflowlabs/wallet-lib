package sui

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/kernelflowlabs/wallet-lib/signing"

	"github.com/mr-tron/base58"
	"golang.org/x/crypto/blake2b"
)

func NewTxBuilder(ti *Ingredient) *TxBuilder {
	return &TxBuilder{
		Ingredient: ti,
	}
}

func NewTxBuilderFromUnsignedHex(unsignedHex string) (*TxBuilder, error) {
	txBytes, err := hex.DecodeString(unsignedHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode hex, err=%v", err)
	}

	// SUI intent: [0, 0, 0]
	intent := []byte{0, 0, 0}
	message := append(intent, txBytes...)
	hash := blake2b.Sum256(message)

	prefix := []byte("TransactionData::")
	toHash := append(prefix, txBytes...)
	txHasher, _ := blake2b.New256(nil)
	txHasher.Write(toHash)
	txHash := base58.Encode(txHasher.Sum(nil))

	return &TxBuilder{
		unsignedHex: hex.EncodeToString(txBytes),
		sigHash:     []string{hex.EncodeToString(hash[:])},
		txHash:      txHash,
	}, nil
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

	gasPrice, _ := strconv.ParseUint(tx.Ingredient.GasPrice, 10, 64)
	gasBudget, _ := strconv.ParseUint(tx.Ingredient.GasBudget, 10, 64)
	var txBytes []byte
	var err error
	switch tx.Ingredient.TxType {
	case signing.TxTypeTransfer:
		if tx.Ingredient.Sender == "" {
			return fmt.Errorf("tx.Ingredient.Sender is empty")
		} else if tx.Ingredient.Recipient == "" {
			return fmt.Errorf("tx.Ingredient.Recipient is empty")
		} else if tx.Ingredient.Recipient == signing.MagicContactAddressForNative {
			return fmt.Errorf("recipient must not be the native asset sentinel")
		} else if tx.Ingredient.Amount == "" {
			return fmt.Errorf("tx.Ingredient.Amount is empty")
		}
		amount, _ := strconv.ParseUint(tx.Ingredient.Amount, 10, 64)

		coinRefs, err := parseCoinRefs(tx.Coins)
		if err != nil {
			return fmt.Errorf("failed to parse Coins, err=%v", err)
		}

		if tx.Ingredient.ContractAddress == signing.MagicContactAddressForNative {
			txBytes, err = buildSuiTransfer(tx.Sender, tx.Recipient, amount, gasPrice, gasBudget, coinRefs, nil)
			if err != nil {
				return fmt.Errorf("failed to build native transfer, err=%v", err)
			}
		} else {
			gasRefs, err := parseCoinRefs(tx.GasCoins)
			if err != nil {
				return fmt.Errorf("failed to parse GasCoins, err=%v", err)
			}
			if len(gasRefs) == 0 {
				return fmt.Errorf("gas coins (SUI) are required for token transfer")
			}
			if len(coinRefs) == 0 {
				return fmt.Errorf("token coins are required for token transfer")
			}
			txBytes, err = buildSuiTransfer(tx.Sender, tx.Recipient, amount, gasPrice, gasBudget, gasRefs, coinRefs)
			if err != nil {
				return fmt.Errorf("failed to build token transfer, err=%v", err)
			}
		}
	default:
		return fmt.Errorf("invalid txType")
	}

	tx.unsignedHex = hex.EncodeToString(txBytes)
	toBeSigned := make([]byte, len(txBytes)+3)
	copy(toBeSigned[3:], txBytes)
	hasher, err := blake2b.New256(nil)
	if err != nil {
		return err
	}
	hasher.Write(toBeSigned)
	sigHash := hasher.Sum(nil)
	tx.sigHash = append(tx.sigHash, hex.EncodeToString(sigHash))
	prefix := []byte("TransactionData::")
	toHash := append(prefix, txBytes...)
	txHasher, _ := blake2b.New256(nil)
	txHasher.Write(toHash)
	tx.txHash = base58.Encode(txHasher.Sum(nil))
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
	if len(privateKey) != ed25519.SeedSize {
		return "", fmt.Errorf("invalid private key length %d", len(privateKey))
	}
	sk := ed25519.NewKeyFromSeed(privateKey)
	pk := sk.Public().(ed25519.PublicKey)
	signature, err := sk.Sign(rand.Reader, signingMessage, crypto.Hash(0))
	if err != nil {
		return "", fmt.Errorf("failed to Sign, err=%v", err)
	}
	sign := make([]byte, 1+len(pk)+len(signature))
	copy(sign[1:], signature)
	copy(sign[1+len(signature):], pk)
	return hex.EncodeToString(sign), nil
}

func (tx *TxBuilder) ConcatSignature(signature string, isDerFormat bool) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("tx == nil")
	} else if tx.unsignedHex == "" {
		return "", fmt.Errorf("tx.UnsignedHex == nil")
	}

	unsignedBytes, err := hex.DecodeString(tx.unsignedHex)
	if err != nil {
		return "", fmt.Errorf("failed to DecodeString for unsignedHex, err=%v", err)
	}

	if isDerFormat {
		return "", fmt.Errorf("DER signature format not supported")
	}
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return "", fmt.Errorf("failed to DecodeString for Signature, err=%v", err)
	}
	txBase64 := base64.StdEncoding.EncodeToString(unsignedBytes)
	sigBase64 := base64.StdEncoding.EncodeToString(sigBytes)
	signedTxBytes, err := json.Marshal(signedTx{
		Tx:        txBase64,
		Signature: sigBase64,
	})
	if err != nil {
		return "", fmt.Errorf("failed to Marshal for signedTx, err=%v", err)
	}
	return hex.EncodeToString(signedTxBytes), nil
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

// types
type (
	Ingredient struct {
		TxType          string `json:"txType" validate:"required,oneof=0 1 2 3 4 5 6"`
		ContractAddress string `json:"contractAddress" validate:"required"`
		Sender          string `json:"sender" validate:"required,sui_addr"`
		Recipient       string `json:"recipient" validate:"required,sui_addr"`
		Amount          string `json:"amount" validate:"required,u64_gt0"`
		GasPrice        string `json:"gasPrice" validate:"required,u64_gt0"`
		GasBudget       string `json:"gasBudget" validate:"required,u64_gt0"`
		Coins           string `json:"coins" validate:"required"`
		GasCoins        string `json:"gasCoins"` // SUI coins for gas payment

	}

	TxBuilder struct {
		*Ingredient
		unsignedHex string
		sigHash     []string
		txHash      string
	}
	signedTx struct {
		Tx        string
		Signature string
	}
)
