package substrate

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/kernelflowlabs/wallet-lib/signing"

	"golang.org/x/crypto/blake2b"
)

func NewTxBuilder(ti *Ingredient, network string) *TxBuilder {
	return &TxBuilder{Ingredient: ti, network: network}
}

func (tx *TxBuilder) Build() error {
	if tx == nil {
		return fmt.Errorf("tx == nil")
	}
	tx.sigHash = nil
	tx.unsignedHex = ""
	tx.txHash = ""
	if tx.Ingredient.ContractAddress != signing.MagicContactAddressForNative {
		return fmt.Errorf("only basecoin supported on this chain")
	}
	if !ValidAddress(tx.Ingredient.Recipient, tx.network) {
		return fmt.Errorf("invalid recipient address")
	}
	recipientPub := convert2PublicKey(tx.Ingredient.Recipient)
	if len(recipientPub) != 32 {
		return fmt.Errorf("invalid recipient address")
	}
	amount, ok := big.NewInt(0).SetString(tx.Ingredient.Amount, 10)
	if !ok {
		return fmt.Errorf("failed to SetString for Amount")
	}
	callIndex, err := hex.DecodeString(strings.TrimPrefix(tx.Ingredient.ChainInfo.CallIndex, "0x"))
	if err != nil || len(callIndex) == 0 {
		return fmt.Errorf("invalid callIndex %q", tx.Ingredient.ChainInfo.CallIndex)
	}
	genesisHash, err := hex.DecodeString(strings.TrimPrefix(tx.Ingredient.ChainInfo.GenesisHash, "0x"))
	if err != nil || len(genesisHash) != 32 {
		return fmt.Errorf("invalid genesisHash")
	}
	specVersion, err := strconv.ParseUint(tx.Ingredient.ChainInfo.SpecVersion, 10, 32)
	if err != nil {
		return fmt.Errorf("failed to ParseUint for SpecVersion,err=%v", err)
	}
	txVersion, err := strconv.ParseUint(tx.Ingredient.ChainInfo.TransactionVersion, 10, 32)
	if err != nil {
		return fmt.Errorf("failed to ParseUint for TransactionVersion,err=%v", err)
	}
	nonceInt, err := strconv.ParseInt(tx.Ingredient.Nonce, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to ParseInt for Nonce,err=%v", err)
	}
	feeInt, err := strconv.ParseInt(tx.Ingredient.Fee, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to ParseInt for Fee,err=%v", err)
	}

	call := encodeTransferCall(callIndex, recipientPub, amount)
	era := encodeEraImmortal()
	payload := encodeSigningPayload(call, era, big.NewInt(nonceInt), big.NewInt(feeInt),
		uint32(specVersion), uint32(txVersion), genesisHash, genesisHash,
		tx.Ingredient.ChainInfo.HasAssetTxPayment, tx.Ingredient.ChainInfo.HasCheckMetadataHash)

	sigHashBytes := payload
	if len(sigHashBytes) > 256 {
		h := blake2b.Sum256(sigHashBytes)
		sigHashBytes = h[:]
	}
	tx.sigHash = append(tx.sigHash, hex.EncodeToString(sigHashBytes))

	ntx := nativeTx{
		Sender: tx.Ingredient.Sender,
		Call:   call,
		Era:    era,
		Nonce:  nonceInt,
		Tip:    feeInt,
	}
	ntxBytes, err := json.Marshal(ntx)
	if err != nil {
		return fmt.Errorf("failed to Marshal ntx, err=%v", err)
	}
	tx.unsignedHex = hex.EncodeToString(ntxBytes)
	return nil
}

func (tx *TxBuilder) Sign(privateKey []byte) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("tx == nil")
	} else if len(tx.sigHash) != 1 {
		return "", fmt.Errorf("tx.SigHash == nil")
	}
	sigHashBytes, err := hex.DecodeString(tx.sigHash[0])
	if err != nil {
		return "", fmt.Errorf("failed to DecodeString for SigHash, err=%v", err)
	}
	signature, err := key.SignWithPrivateKeySR25519(privateKey, sigHashBytes)
	if err != nil {
		return "", fmt.Errorf("failed to SignWithPrivateKeySR25519,err=%v", err)
	}
	return hex.EncodeToString(signature[:]), nil
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
	ntxBytes, err := hex.DecodeString(tx.unsignedHex)
	if err != nil {
		return "", fmt.Errorf("failed to DecodeString for UnsignedHex, err=%v", err)
	}
	var ntx nativeTx
	if err = json.Unmarshal(ntxBytes, &ntx); err != nil {
		return "", fmt.Errorf("failed to Unmarshal for ntxBytes, err=%v", err)
	}
	senderPub := convert2PublicKey(ntx.Sender)
	if len(senderPub) != 32 {
		return "", fmt.Errorf("invalid sender address")
	}
	sigBytes, err := hex.DecodeString(strings.TrimPrefix(signature, "0x"))
	if err != nil {
		return "", fmt.Errorf("failed to DecodeString for Signature, err=%v", err)
	}
	signedExt := encodeSignedExtrinsic(senderPub, 0x01, sigBytes, ntx.Era,
		big.NewInt(ntx.Nonce), big.NewInt(ntx.Tip), ntx.Call,
		tx.Ingredient.ChainInfo.HasAssetTxPayment, tx.Ingredient.ChainInfo.HasCheckMetadataHash)

	h := blake2b.Sum256(signedExt)
	tx.txHash = "0x" + hex.EncodeToString(h[:])
	return "0x" + hex.EncodeToString(signedExt), nil
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
	ChainInfo struct {
		CallIndex            string `json:"callIndex"`
		Metadata             string `json:"metadata"`
		BlockHash            string `json:"blockHash"`
		GenesisHash          string `json:"genesisHash"`
		SpecVersion          string `json:"specVersion"`
		TransactionVersion   string `json:"transactionVersion"`
		ExistentialDeposit   string `json:"existentialDeposit"`
		TypeSource           string `json:"typeSource"`
		HasCheckMetadataHash bool   `json:"hasCheckMetadataHash"`
		HasAssetTxPayment    bool   `json:"hasAssetTxPayment"`
	}
	Ingredient struct {
		TxType          string `json:"txType"`
		ContractAddress string `json:"contractAddress"`
		Sender          string `json:"sender"`
		Recipient       string `json:"recipient"`
		Amount          string `json:"amount"`
		Nonce           string `json:"nonce"`
		Fee             string `json:"fee"`
		*ChainInfo
	}
	TxBuilder struct {
		*Ingredient
		network     string
		unsignedHex string
		sigHash     []string
		txHash      string
	}
)

type nativeTx struct {
	Sender string `json:"sender"`
	Call   []byte `json:"call"`
	Era    []byte `json:"era"`
	Nonce  int64  `json:"nonce"`
	Tip    int64  `json:"tip"`
}
