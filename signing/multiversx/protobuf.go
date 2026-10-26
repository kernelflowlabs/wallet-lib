package multiversx

import (
	"github.com/kernelflowlabs/wallet-lib/contracts/esdt"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"

	"github.com/kernelflowlabs/wallet-lib/signing"
	"golang.org/x/crypto/blake2b"
)

func computeTxHash(tx *TxBuilder, signature string) (string, error) {
	nonce, err := strconv.ParseUint(tx.Ingredient.Nonce, 10, 64)
	if err != nil {
		return "", fmt.Errorf("failed to ParseUint for Nonce %s, err=%v", tx.Ingredient.Nonce, err)
	}
	gasPrice, err := strconv.ParseUint(tx.Ingredient.GasPrice, 10, 64)
	if err != nil {
		return "", fmt.Errorf("failed to ParseUint for GasPrice %s, err=%v",
			tx.Ingredient.GasPrice, err)
	}
	gasLimit, err := strconv.ParseUint(tx.Ingredient.GasLimit, 10, 64)
	if err != nil {
		return "", fmt.Errorf("failed to ParseUint for GasLimit %s, err=%v",
			tx.Ingredient.GasLimit, err)
	}
	version, err := strconv.ParseUint(tx.Ingredient.Version, 10, 64)
	if err != nil {
		return "", fmt.Errorf("failed to ParseUint for Version %s, err=%v",
			tx.Ingredient.Version, err)
	}

	receiverBytes, err := decodeAddress(tx.Ingredient.Recipient)
	if err != nil {
		return "", fmt.Errorf("failed to decode Recipient %s, err=%v", tx.Ingredient.Recipient, err)
	}

	senderBytes, err := decodeAddress(tx.Ingredient.Sender)
	if err != nil {
		return "", fmt.Errorf("failed to decode Sender %s, err=%v", tx.Ingredient.Sender, err)
	}

	signaturesBytes, err := hex.DecodeString(signature)
	if err != nil {
		return "", err
	}

	var payload string
	amount := "0"
	if tx.Ingredient.ContractAddress == signing.MagicContactAddressForNative {
		amount = tx.Ingredient.Amount
	} else {
		payload, err = esdt.PackPayloadForESDT(tx.Ingredient.ContractAddress, tx.Ingredient.Amount)
		if err != nil {
			return "", fmt.Errorf("failed to esdt.PackPayloadForESDT, err=%v", err)
		}
	}
	valueBI, ok := big.NewInt(0).SetString(amount, 10)
	if !ok {
		return "", fmt.Errorf("failed to big.NewInt(0).SetString  Amount %s, err=%v", tx.Ingredient.Amount, err)
	}

	nodeTx := &protoTx{
		Nonce:     nonce,
		Value:     valueBI,
		RcvAddr:   receiverBytes,
		SndAddr:   senderBytes,
		GasPrice:  gasPrice,
		GasLimit:  gasLimit,
		Data:      []byte(payload),
		ChainID:   []byte(tx.ChainID),
		Version:   uint32(version),
		Signature: signaturesBytes,
	}
	txHash := blake2b.Sum256(nodeTx.Marshal())
	return hex.EncodeToString(txHash[:]), nil
}

type protoTx struct {
	Nonce     uint64
	Value     *big.Int
	RcvAddr   []byte
	SndAddr   []byte
	GasPrice  uint64
	GasLimit  uint64
	Data      []byte
	ChainID   []byte
	Version   uint32
	Signature []byte
}

func (t *protoTx) Marshal() []byte {
	var b []byte
	if t.Nonce != 0 {
		b = append(b, 0x08)
		b = appendVarint(b, t.Nonce)
	}
	value := marshalValue(t.Value)
	b = append(b, 0x12)
	b = appendVarint(b, uint64(len(value)))
	b = append(b, value...)
	if len(t.RcvAddr) > 0 {
		b = append(b, 0x1a)
		b = appendVarint(b, uint64(len(t.RcvAddr)))
		b = append(b, t.RcvAddr...)
	}
	if len(t.SndAddr) > 0 {
		b = append(b, 0x2a)
		b = appendVarint(b, uint64(len(t.SndAddr)))
		b = append(b, t.SndAddr...)
	}
	if t.GasPrice != 0 {
		b = append(b, 0x38)
		b = appendVarint(b, t.GasPrice)
	}
	if t.GasLimit != 0 {
		b = append(b, 0x40)
		b = appendVarint(b, t.GasLimit)
	}
	if len(t.Data) > 0 {
		b = append(b, 0x4a)
		b = appendVarint(b, uint64(len(t.Data)))
		b = append(b, t.Data...)
	}
	if len(t.ChainID) > 0 {
		b = append(b, 0x52)
		b = appendVarint(b, uint64(len(t.ChainID)))
		b = append(b, t.ChainID...)
	}
	if t.Version != 0 {
		b = append(b, 0x58)
		b = appendVarint(b, uint64(t.Version))
	}
	if len(t.Signature) > 0 {
		b = append(b, 0x62)
		b = appendVarint(b, uint64(len(t.Signature)))
		b = append(b, t.Signature...)
	}
	return b
}

func marshalValue(a *big.Int) []byte {
	if a == nil {
		return []byte{0}
	}
	raw := a.Bytes()
	if len(raw) == 0 {
		return []byte{0, 0}
	}
	out := make([]byte, len(raw)+1)
	if a.Sign() < 0 {
		out[0] = 1
	}
	copy(out[1:], raw)
	return out
}

func appendVarint(b []byte, v uint64) []byte {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}
