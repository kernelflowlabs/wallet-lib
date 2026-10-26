package cosmos

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	bankv1beta1 "cosmossdk.io/api/cosmos/bank/v1beta1"
	basev1beta1 "cosmossdk.io/api/cosmos/base/v1beta1"
	secp256k1v1beta1 "cosmossdk.io/api/cosmos/crypto/secp256k1"
	signingv1beta1 "cosmossdk.io/api/cosmos/tx/signing/v1beta1"
	txv1beta1 "cosmossdk.io/api/cosmos/tx/v1beta1"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/kernelflowlabs/wallet-lib/signing"
)

func NewTxBuilder(ti *Ingredient, network string) *TxBuilder {
	return &TxBuilder{
		Ingredient: ti,
		network:    network,
	}
}

func isValidInt(s string) bool {
	_, ok := new(big.Int).SetString(s, 10)
	return ok
}

func cosmosAny(m proto.Message) (*anypb.Any, error) {
	value, err := proto.MarshalOptions{Deterministic: true}.Marshal(m)
	if err != nil {
		return nil, err
	}
	return &anypb.Any{TypeUrl: "/" + string(m.ProtoReflect().Descriptor().FullName()), Value: value}, nil
}

func (tx *TxBuilder) Build() error {
	if tx == nil {
		return fmt.Errorf("tx == nil")
	}

	if tx.Ingredient.TxType != signing.TxTypeTransfer ||
		tx.Ingredient.ContractAddress != signing.MagicContactAddressForNative {
		return fmt.Errorf("only basecoin transfer supported on this chain")
	}
	denom := Denom(tx.network)
	gasLimit, err := strconv.ParseUint(tx.Ingredient.GasLimit, 10, 64)
	if err != nil {
		return fmt.Errorf("fail to DecodeString for gasLimit, err=%v", err)
	}
	accountNumber, err := strconv.ParseUint(tx.Ingredient.AccountNumber, 10, 64)
	if err != nil {
		return fmt.Errorf("fail to DecodeString for accountNumber, err=%v", err)
	}
	sequence, err := strconv.ParseUint(tx.Ingredient.Sequence, 10, 64)
	if err != nil {
		return fmt.Errorf("fail to DecodeString for sequence, err=%v", err)
	}
	if !isValidInt(tx.Ingredient.Amount) {
		return fmt.Errorf("invalid amount")
	}
	if !isValidInt(tx.Ingredient.FeeAmount) {
		return fmt.Errorf("invalid feeAmount")
	}

	sendMsg := &bankv1beta1.MsgSend{
		FromAddress: tx.Ingredient.Sender,
		ToAddress:   tx.Ingredient.Recipient,
		Amount:      []*basev1beta1.Coin{{Denom: denom, Amount: tx.Ingredient.Amount}},
	}
	anySend, err := cosmosAny(sendMsg)
	if err != nil {
		return fmt.Errorf("fail to NewAnyWithValue for sendMsg, err=%v", err)
	}
	body := &txv1beta1.TxBody{Messages: []*anypb.Any{anySend}, Memo: tx.Ingredient.Memo, TimeoutHeight: 0}

	if tx.Ingredient.SenderPublicKey == "" {
		return fmt.Errorf("empty SenderPublicKey")
	}
	publickeyBytes, err := hex.DecodeString(tx.Ingredient.SenderPublicKey)
	if err != nil {
		return fmt.Errorf("fail to DecodeString for SenderPublicKey, err=%v", err)
	}
	anyPubkey, err := cosmosAny(&secp256k1v1beta1.PubKey{Key: publickeyBytes})
	if err != nil {
		return fmt.Errorf("fail to NewAnyWithValue for pubkey, err=%v", err)
	}

	modeInfo := &txv1beta1.ModeInfo{
		Sum: &txv1beta1.ModeInfo_Single_{
			Single: &txv1beta1.ModeInfo_Single{Mode: signingv1beta1.SignMode_SIGN_MODE_DIRECT},
		},
	}
	signerInfos := []*txv1beta1.SignerInfo{
		{PublicKey: anyPubkey, ModeInfo: modeInfo, Sequence: sequence},
	}
	fee := &txv1beta1.Fee{
		Amount:   []*basev1beta1.Coin{{Denom: denom, Amount: tx.Ingredient.FeeAmount}},
		GasLimit: gasLimit,
	}
	authInfo := &txv1beta1.AuthInfo{SignerInfos: signerInfos, Fee: fee}

	mo := proto.MarshalOptions{Deterministic: true}
	bodyBytes, err := mo.Marshal(body)
	if err != nil {
		return fmt.Errorf("fail to Marshal for body, err=%v", err)
	}
	authInfoBytes, err := mo.Marshal(authInfo)
	if err != nil {
		return fmt.Errorf("fail to Marshal for authInfo, err=%v", err)
	}
	signDoc := &txv1beta1.SignDoc{
		BodyBytes:     bodyBytes,
		AuthInfoBytes: authInfoBytes,
		ChainId:       tx.network,
		AccountNumber: accountNumber,
	}
	signDocBytes, err := mo.Marshal(signDoc)
	if err != nil {
		return fmt.Errorf("fail to Marshal for signDoc, err=%v", err)
	}
	sigHash := sha256.Sum256(signDocBytes)
	tx.sigHash = append(tx.sigHash, hex.EncodeToString(sigHash[:]))
	tx.unsignedHex = hex.EncodeToString(signDocBytes)
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
		return "", fmt.Errorf("fail to DecodeString for SigHash, err=%v", err)
	}
	btcPrivateKey, _ := btcec.PrivKeyFromBytes(privateKey)
	signData := ecdsa.SignCompact(btcPrivateKey, sigHash[:], false)
	return hex.EncodeToString(signData[1:]), nil
}

func (tx *TxBuilder) ConcatSignature(signature string, isDerFormat bool) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("tx == nil")
	} else if tx.unsignedHex == "" {
		return "", fmt.Errorf("tx.UnsignedHex == nil")
	}

	signDocBytes, err := hex.DecodeString(tx.unsignedHex)
	if err != nil {
		return "", fmt.Errorf("fail to DecodeString for UnsignedHex, err=%v", err)
	}
	var signDoc txv1beta1.SignDoc
	if err := proto.Unmarshal(signDocBytes, &signDoc); err != nil {
		return "", fmt.Errorf("fail to Unmarshal for signDocBytes, err=%v", err)
	}
	if isDerFormat {
		return "", fmt.Errorf("der format not supported")
	}
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return "", fmt.Errorf("fail to DecodeString for Signature, err=%v", err)
	}

	trans := &txv1beta1.TxRaw{
		BodyBytes:     signDoc.BodyBytes,
		AuthInfoBytes: signDoc.AuthInfoBytes,
		Signatures:    [][]byte{sigBytes},
	}
	transBytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(trans)
	if err != nil {
		return "", fmt.Errorf("fail to Marshal for trans, err=%v", err)
	}
	tx.txHash = strings.ToUpper(fmt.Sprintf("%x", sha256.Sum256(transBytes)))
	return hex.EncodeToString(transBytes), nil
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
	AccountInfo struct {
		AccountNumber string `json:"accountNumber"`
		Sequence      string `json:"sequence"`
	}
	Ingredient struct {
		TxType          string `json:"txType"`
		ContractAddress string `json:"contractAddress"`
		Sender          string `json:"sender"`
		SenderPublicKey string `json:"senderPublicKey"`
		Recipient       string `json:"recipient"`
		Amount          string `json:"amount"`
		FeeAmount       string `json:"feeAmount"`
		GasLimit        string `json:"gasLimit"`
		Memo            string `json:"memo"`
		*AccountInfo
	}
	TxBuilder struct {
		*Ingredient
		network     string
		unsignedHex string
		sigHash     []string
		txHash      string
	}
)
