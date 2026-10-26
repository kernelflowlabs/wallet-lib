package algorand

import (
	"bytes"
	"crypto/sha512"
	"encoding/base32"
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
	if tx.Ingredient.ContractAddress == "" {
		return fmt.Errorf("empty ContractAddress")
	}
	senderBytes, err := decodeAddress(tx.Ingredient.Sender)
	if err != nil {
		return fmt.Errorf("fail to DecodeAddress for Sender, err=%v", err)
	}
	fee, err := strconv.ParseUint(tx.Ingredient.Fee, 10, 64)
	if err != nil {
		return fmt.Errorf("fail to ParseUint for Fee, err=%v", err)
	}
	genesisHashTmp, err := base64.StdEncoding.DecodeString(tx.Ingredient.GenesisHash)
	if err != nil {
		return fmt.Errorf("fail to DecodeString for GenesisHash, err=%v", err)
	}
	genesisHash := [hashLenBytes]byte{}
	copy(genesisHash[:], genesisHashTmp)
	firstValid, err := strconv.ParseUint(tx.Ingredient.FirstValid, 10, 64)
	if err != nil {
		return fmt.Errorf("fail to ParseUint for FirstValid, err=%v", err)
	}
	ntx := nativeTx{
		header: header{
			Sender:      senderBytes,
			Fee:         fee,
			FirstValid:  firstValid,
			LastValid:   firstValid + 1000,
			GenesisID:   tx.Ingredient.GenesisID,
			GenesisHash: genesisHash,
		},
	}

	switch tx.Ingredient.TxType {
	case signing.TxTypeTransfer:
		recipientBytes, err := decodeAddress(tx.Ingredient.Recipient)
		if err != nil {
			return fmt.Errorf("fail to DecodeAddress for recipientBytes, err=%v", err)
		}
		amount, err := strconv.ParseUint(tx.Ingredient.Amount, 10, 64)
		if err != nil {
			return fmt.Errorf("fail to ParseUint for Amount, err=%v", err)
		}
		if tx.Ingredient.ContractAddress == signing.MagicContactAddressForNative {
			ntx.Type = "pay"
			ntx.paymentTxnFields = paymentTxnFields{
				Receiver: recipientBytes,
				Amount:   amount,
			}
		} else {
			assetId, err := strconv.ParseUint(tx.Ingredient.ContractAddress, 10, 64)
			if err != nil {
				return fmt.Errorf("fail to ParseUint for AssetsID, err=%v", err)
			}
			ntx.Type = "axfer"
			ntx.assetTransferTxnFields = assetTransferTxnFields{
				XferAsset:     assetId,
				AssetReceiver: recipientBytes,
				AssetAmount:   amount,
			}
		}
	case signing.TxTypeAccountActivate:
		assetId, err := strconv.ParseUint(tx.Ingredient.ContractAddress, 10, 64)
		if err != nil {
			return fmt.Errorf("fail to ParseUint for AssetsID, err=%v", err)
		}
		ntx.Type = "axfer"
		ntx.assetTransferTxnFields = assetTransferTxnFields{
			XferAsset:     assetId,
			AssetReceiver: senderBytes,
			AssetAmount:   0,
		}
	default:
		return fmt.Errorf("invalid txType")
	}
	ntxBytes, err := json.Marshal(ntx)
	if err != nil {
		return fmt.Errorf("fail to Marshal for ntx, err=%v", err)
	}
	encodedTx := Encode(ntx)
	msgParts := [][]byte{[]byte("TX"), encodedTx}
	tx.sigHash = append(tx.sigHash, hex.EncodeToString(bytes.Join(msgParts, nil)))
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
		return "", fmt.Errorf("fail to DecodeString for SigHash, err=%v", err)
	}
	signature, err := key.SignWithPrivateKeyED25519(privateKey, sigHash)
	if err != nil {
		return "", fmt.Errorf("fail to Sign, err=%v", err)
	}
	return hex.EncodeToString(signature), nil
}

func (tx *TxBuilder) ConcatSignature(signature string, isDerFormat bool) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("tx == nil")
	} else if tx.unsignedHex == "" {
		return "", fmt.Errorf("tx.UnsignedHex == nil")
	}

	ntxBytes, err := hex.DecodeString(tx.unsignedHex)
	if err != nil {
		return "", fmt.Errorf("fail to DecodeString for UnsignedHex, err=%v", err)
	}
	var ntx nativeTx
	if err := json.Unmarshal(ntxBytes, &ntx); err != nil {
		return "", fmt.Errorf("fail to Unmarshal for ntxBytes, err=%v", err)
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
	stx := signedNativeTx{
		Txn:      ntx,
		Sig:      sig,
		AuthAddr: ntx.header.Sender,
	}
	signedHex := hex.EncodeToString(Encode(stx))
	sigHash, err := hex.DecodeString(tx.sigHash[0])
	if err != nil {
		return "", fmt.Errorf("fail to DecodeString for SigHash, err=%v", err)
	}
	txidBytes := sha512.Sum512_256(sigHash)
	tx.txHash = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(txidBytes[:])
	return signedHex, nil
}

func (tx *TxBuilder) GetTxHash() string       { return tx.txHash }
func (tx *TxBuilder) GetSigHash() []string    { return tx.sigHash }
func (tx *TxBuilder) GetUnsignedHex() string  { return tx.unsignedHex }
func (tx *TxBuilder) SetSigHash(s []string)   { tx.sigHash = s }
func (tx *TxBuilder) SetUnsignedHex(s string) { tx.unsignedHex = s }

type (
	Ingredient struct {
		TxType          string `json:"txType"`
		ContractAddress string `json:"contractAddress"`
		Sender          string `json:"sender"`
		Recipient       string `json:"recipient"`
		Amount          string `json:"amount"`
		Fee             string `json:"fee"`
		GenesisID       string `json:"genesisID"`
		GenesisHash     string `json:"genesisHash"`
		FirstValid      string `json:"firstValid"`
	}
	TxBuilder struct {
		*Ingredient
		unsignedHex string
		sigHash     []string
		txHash      string
	}
)

type (
	header struct {
		_struct     struct{} `codec:",omitempty,omitemptyarray"`
		Sender      Address  `codec:"snd"`
		Fee         uint64   `codec:"fee"`
		FirstValid  uint64   `codec:"fv"`
		LastValid   uint64   `codec:"lv"`
		Note        []byte   `codec:"note"`
		GenesisID   string   `codec:"gen"`
		GenesisHash Digest   `codec:"gh"`
		Group       Digest   `codec:"grp"`
		Lease       [32]byte `codec:"lx"`
		RekeyTo     Address  `codec:"rekey"`
	}
	paymentTxnFields struct {
		Receiver         Address `codec:"rcv"`
		Amount           uint64  `codec:"amt"`
		CloseRemainderTo Address `codec:"close"`
	}
	assetTransferTxnFields struct {
		XferAsset     uint64  `codec:"xaid"`
		AssetAmount   uint64  `codec:"aamt"`
		AssetSender   Address `codec:"asnd"`
		AssetReceiver Address `codec:"arcv"`
		AssetCloseTo  Address `codec:"aclose"`
	}
	nativeTx struct {
		_struct struct{} `codec:",omitempty,omitemptyarray"`
		Type    string   `codec:"type"`
		header
		paymentTxnFields
		assetTransferTxnFields
	}
	signedNativeTx struct {
		_struct  struct{} `codec:",omitempty,omitemptyarray"`
		Sig      [64]byte `codec:"sig"`
		Txn      nativeTx `codec:"txn"`
		AuthAddr Address  `codec:"sgnr"`
	}
)
