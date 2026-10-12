package near

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"

	"github.com/btcsuite/btcd/address/v2/base58"
	borsh "github.com/near/borsh-go"

	"github.com/kernelflowlabs/wallet-sdk/signing"
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

	nonce, err := strconv.ParseUint(tx.Ingredient.Nonce, 10, 64)
	if err != nil {
		return fmt.Errorf("fail to ParseInt for nonce, err=%v", err)
	}
	amount, ok := big.NewInt(0).SetString(tx.Ingredient.Amount, 10)
	if !ok {
		return fmt.Errorf("fail to SetString for Amount")
	}
	blockHashBytes := base58.Decode(tx.Ingredient.BlockHash)
	if len(blockHashBytes) == 0 {
		return fmt.Errorf("invalid blockhash")
	}
	publicKeyBytes, err := hex.DecodeString(tx.Ingredient.SenderPublicKey)
	if err != nil {
		return fmt.Errorf("fail to DecodeString for SenderPublicKey, err=%v", err)
	}
	var publicKey [32]byte
	copy(publicKey[:], publicKeyBytes)
	var blockHash [32]byte
	copy(blockHash[:], blockHashBytes)

	var receiverID string
	var actions []nearAction
	switch tx.Ingredient.TxType {
	case signing.TxTypeTransfer:
		if tx.Ingredient.ContractAddress == signing.MagicContactAddressForNative {
			receiverID = tx.Ingredient.Recipient
			actions = append(actions, nearAction{Enum: 3, Transfer: transferEvent{Deposit: *amount}})
		} else {
			receiverID = tx.Ingredient.ContractAddress
			if tx.Ingredient.RequiredAmount != "0" {
				args := map[string]interface{}{
					"account_id": tx.Ingredient.Recipient,
				}
				argsBytes, err := json.Marshal(args)
				if err != nil {
					return fmt.Errorf("fail to Marshal for args, err=%v", err)
				}
				storageBounds, ok := big.NewInt(0).SetString(tx.Ingredient.RequiredAmount, 10)
				if !ok {
					return fmt.Errorf("invalid requiredAmount %q", tx.Ingredient.RequiredAmount)
				}
				functionCall := functionCallEvent{
					MethodName: "storage_deposit",
					Args:       argsBytes,
					Gas:        30000000000000,
					Deposit:    *storageBounds,
				}
				actions = append(actions, nearAction{Enum: 2, FunctionCall: functionCall})
			}

			args := map[string]interface{}{
				"receiver_id": tx.Ingredient.Recipient,
				"amount":      tx.Ingredient.Amount,
			}
			argsBytes, err := json.Marshal(args)
			if err != nil {
				return fmt.Errorf("fail to Marshal for args, err=%v", err)
			}
			functionCall := functionCallEvent{
				MethodName: "ft_transfer",
				Args:       argsBytes,
				Gas:        30000000000000,
				Deposit:    *big.NewInt(1),
			}
			actions = append(actions, nearAction{Enum: 2, FunctionCall: functionCall})
		}
	default:
		return fmt.Errorf("invalid txType")
	}
	ntx := nativeTx{
		SignerID:   tx.Ingredient.Sender,
		PublicKey:  publicKeyData{0, publicKey},
		Nonce:      nonce,
		ReceiverID: receiverID,
		BlockHash:  blockHash,
		Actions:    actions,
	}
	ntxBytes, err := borsh.Serialize(ntx)
	if err != nil {
		return fmt.Errorf("fail to Serialize for ntx, err=%v", err)
	}
	sigHashBytes := sha256.Sum256(ntxBytes)
	tx.sigHash = append(tx.sigHash, hex.EncodeToString(sigHashBytes[:]))
	tx.unsignedHex = hex.EncodeToString(ntxBytes)
	return nil
}

func (tx *TxBuilder) Sign(privateKey []byte) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("tx == nil")
	} else if len(tx.sigHash) != 1 {
		return "", fmt.Errorf("tx.SigHash == nil")
	}

	sigHash, err := hex.DecodeString(tx.sigHash[0])
	if err != nil {
		return "", fmt.Errorf("fail to DecodeString for sigHash, err=%v", err)
	}
	if len(privateKey) != ed25519.SeedSize {
		return "", fmt.Errorf("invalid private key length %d", len(privateKey))
	}
	p := ed25519.NewKeyFromSeed(privateKey)
	signatureBytes := ed25519.Sign(p, sigHash)
	if len(signatureBytes) != 64 {
		return "", fmt.Errorf("sign error,length is not equal 64, length=%d", len(signatureBytes))
	}
	return hex.EncodeToString(signatureBytes), nil
}

func (tx *TxBuilder) ConcatSignature(signature string, isDerFormat bool) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("tx == nil")
	} else if tx.unsignedHex == "" {
		return "", fmt.Errorf("tx.UnsignedHex == nil")
	}

	var ntx nativeTx
	ntxBytes, err := hex.DecodeString(tx.unsignedHex)
	if err != nil {
		return "", fmt.Errorf("fail to DecodeString for UnsignedHex, err=%v", err)
	}
	if err := borsh.Deserialize(&ntx, ntxBytes); err != nil {
		return "", fmt.Errorf("fail to Deserialize for ntxBytes, err=%v", err)
	}

	if isDerFormat {
		return "", fmt.Errorf("der format not supported")
	}
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return "", fmt.Errorf("fail to DecodeString for Signature, err=%v", err)
	}
	var sig [64]byte
	copy(sig[:], sigBytes)
	sntx := signedNativeTx{
		Transaction: ntx,
		Signature: signatureData{
			KeyType: ntx.PublicKey.KeyType,
			Data:    sig,
		},
	}
	sntxBytes, err := borsh.Serialize(sntx)
	if err != nil {
		return "", fmt.Errorf("fail to Serialize for sntx, err=%v", err)
	}

	hashBytes, err := hex.DecodeString(tx.sigHash[0])
	if err != nil {
		return "", fmt.Errorf("fail to DecodeString for SigHash, err=%v", err)
	}
	tx.txHash = base58.Encode(hashBytes)
	return hex.EncodeToString(sntxBytes), nil
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
	Ingredient struct {
		TxType          string `json:"txType"`
		ContractAddress string `json:"contractAddress"`
		Sender          string `json:"sender"`
		SenderPublicKey string `json:"senderPublicKey"`
		Recipient       string `json:"recipient"`
		Amount          string `json:"amount"`
		Nonce           string `json:"nonce"`
		RequiredAmount  string `json:"requiredAmount,omitempty"`
		BlockHash       string `json:"blockHash"`
	}
	TxBuilder struct {
		*Ingredient
		unsignedHex string
		sigHash     []string
		txHash      string
	}

	createAccountEvent  struct{}
	deployContractEvent struct {
		Code []byte
	}
	functionCallEvent struct {
		MethodName string
		Args       []byte
		Gas        uint64
		Deposit    big.Int
	}
	transferEvent struct {
		Deposit big.Int
	}
	stakeEvent struct {
		Stake     big.Int
		PublicKey publicKeyData
	}
	addKeyEvent struct {
		PublicKey publicKeyData
		AccessKey accessKeyEvent
	}
	accessKeyEvent struct {
		Nonce      uint64
		Permission accessKeyPermission
	}
	accessKeyPermission struct {
		Enum         borsh.Enum `borsh_enum:"true"`
		FunctionCall functionCallPermission
		FullAccess   fullAccessPermission
	}
	functionCallPermission struct {
		Allowance   *big.Int
		ReceiverID  string
		MethodNames []string
	}
	fullAccessPermission struct{}
	deleteKeyEvent       struct {
		PublicKey publicKeyData
	}
	deleteAccountEvent struct {
		BeneficiaryID string
	}
	nearAction struct {
		Enum           borsh.Enum `borsh_enum:"true"`
		CreateAccount  createAccountEvent
		DeployContract deployContractEvent
		FunctionCall   functionCallEvent
		Transfer       transferEvent
		Stake          stakeEvent
		AddKey         addKeyEvent
		DeleteKey      deleteKeyEvent
		DeleteAccount  deleteAccountEvent
	}
	publicKeyData struct {
		KeyType uint8
		Data    [32]byte
	}
	nativeTx struct {
		SignerID   string
		PublicKey  publicKeyData
		Nonce      uint64
		ReceiverID string
		BlockHash  [32]byte
		Actions    []nearAction
	}
	signatureData struct {
		KeyType uint8
		Data    [64]byte
	}
	signedNativeTx struct {
		Transaction nativeTx
		Signature   signatureData
	}
)
