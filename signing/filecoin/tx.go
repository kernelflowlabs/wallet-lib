package filecoin

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"

	"github.com/dchest/blake2b"
	"github.com/fxamacker/cbor"
	b58 "github.com/mr-tron/base58/base58"
	mbase "github.com/multiformats/go-multibase"
	mh "github.com/multiformats/go-multihash"

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
	if tx.Ingredient.TxType != signing.TxTypeTransfer ||
		tx.Ingredient.ContractAddress != signing.MagicContactAddressForNative {
		return fmt.Errorf("only basecoin transfer supported on this chain")
	}
	amount, ok := big.NewInt(0).SetString(tx.Ingredient.Amount, 10)
	if !ok {
		return fmt.Errorf("fail to SetString for Amount")
	}
	if amount.Sign() < 0 {
		return fmt.Errorf("amount must not be negative")
	}
	amountBigInt := BigInt(*amount)
	nonce, err := strconv.ParseUint(tx.Ingredient.Nonce, 10, 64)
	if err != nil {
		return fmt.Errorf("fail to ParseUint for Nonce %s, err=%v", tx.Ingredient.Nonce, err)
	}
	gasLimit, err := strconv.ParseInt(tx.Ingredient.GasLimit, 10, 64)
	if err != nil {
		return fmt.Errorf("fail to ParseInt for GasLimit %s, err=%v", tx.Ingredient.GasLimit, err)
	}
	gasFeeCap, ok := big.NewInt(0).SetString(tx.Ingredient.GasFeeCap, 10)
	if !ok {
		return fmt.Errorf("fail to SetString for GasFeeCap")
	}
	if gasFeeCap.Sign() < 0 {
		return fmt.Errorf("gasFeeCap must not be negative")
	}
	gasFeeCapBigInt := BigInt(*gasFeeCap)
	gasPremium, ok := big.NewInt(0).SetString(tx.Ingredient.GasPremium, 10)
	if !ok {
		return fmt.Errorf("fail to SetString for GasPremium")
	}
	if gasPremium.Sign() < 0 {
		return fmt.Errorf("gasPremium must not be negative")
	}
	gasPremiumBigInt := BigInt(*gasPremium)

	ntx := &nativeTx{
		Version:    0,
		From:       tx.Ingredient.Sender,
		To:         tx.Ingredient.Recipient,
		Value:      &amountBigInt,
		Nonce:      nonce,
		GasLimit:   gasLimit,
		GasFeeCap:  &gasFeeCapBigInt,
		GasPremium: &gasPremiumBigInt,
		Method:     0,
		Params:     []byte{},
	}
	ntxBytes, err := json.Marshal(ntx)
	if err != nil {
		return fmt.Errorf("fail to Marshal for ntx, err=%v", err)
	}
	hasher := blake2b.Sum256(ntx.Cid())
	tx.sigHash = append(tx.sigHash, hex.EncodeToString(hasher[:]))
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
	signature, err := key.SignWithPrivateKeyECDSAForEVM(privateKey, sigHash)
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
		return "", fmt.Errorf("fail to Unmarshal for ntx, err=%v", err)
	}
	if isDerFormat {
		return "", fmt.Errorf("der format not supported")
	}
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return "", fmt.Errorf("fail to DecodeString for Signature, err=%v", err)
	}
	signedTx := &signedNativeTx{
		Message: &ntx,
		Signature: struct {
			Type byte
			Data []byte
		}{Type: byte(SECP256K1), Data: sigBytes},
	}
	signedTxBytes, err := json.Marshal(signedTx)
	if err != nil {
		return "", fmt.Errorf("fail to Marshal for signedTx, err=%v", err)
	}
	tx.txHash = ntx.TxHash()
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

type (
	GasModel struct {
		GasLimit   string `json:"gasLimit"`
		GasFeeCap  string `json:"gasFeeCap"`
		GasPremium string `json:"gasPremium"`
	}
	Ingredient struct {
		TxType          string `json:"txType"`
		ContractAddress string `json:"contractAddress"`
		Sender          string `json:"sender"`
		Recipient       string `json:"recipient"`
		Amount          string `json:"amount"`
		Nonce           string `json:"nonce"`
		Method          string `json:"method"`
		Params          string `json:"params"`
		*GasModel
	}
	TxBuilder struct {
		*Ingredient
		unsignedHex string
		sigHash     []string
		txHash      string
	}
)

type nativeTx struct {
	Version    uint64  `json:"Version"`
	To         string  `json:"To"`
	From       string  `json:"From"`
	Nonce      uint64  `json:"Nonce"`
	Value      *BigInt `json:"Value"`
	GasLimit   int64   `json:"GasLimit"`
	GasFeeCap  *BigInt `json:"GasFeeCap"`
	GasPremium *BigInt `json:"GasPremium"`
	Method     uint64  `json:"Method"`
	Params     []byte  `json:"Params"`
}

type BigInt big.Int

func (bn *BigInt) String() string {
	b := big.Int(*bn)
	return b.String()
}

func (bn *BigInt) MarshalJSON() ([]byte, error) {
	b := big.Int(*bn)
	return json.Marshal(b.String())
}

func (bi *BigInt) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	i, ok := big.NewInt(0).SetString(s, 10)
	if !ok {
		return fmt.Errorf("failed to parse big string: '%s'", string(b))
	}
	*bi = BigInt(*i)
	return nil
}

func (bn BigInt) Bytes() []byte {
	b := big.Int(bn)
	return b.Bytes()
}

func (bn *BigInt) cborBytes() []byte {
	b := (*big.Int)(bn)
	switch b.Sign() {
	case 0:
		return []byte{}
	case -1:
		return append([]byte{1}, b.Bytes()...)
	}
	return append([]byte{0}, b.Bytes()...)
}

func (m *nativeTx) Serialize() []byte {
	i := []interface{}{
		0,
		addressToBytes(m.To),
		addressToBytes(m.From),
		m.Nonce,
		m.Value.cborBytes(),
		m.GasLimit,
		m.GasFeeCap.cborBytes(),
		m.GasPremium.cborBytes(),
		m.Method,
		m.Params,
	}
	bytes, _ := cbor.Marshal(i, cbor.EncOptions{})
	return bytes
}

func (m *nativeTx) Cid() []byte {
	bytes := m.Serialize()
	h, _ := blake2b.New(&blake2b.Config{Size: uint8(32)})
	h.Write(bytes)
	sum := h.Sum(nil)
	prefix := []byte{0x01, 0x71, 0xa0, 0xe4, 0x02, 0x20}
	return append(prefix, sum...)
}

func (m *nativeTx) TxHash() string {
	cid := m.Cid()
	if len(cid) == 34 && cid[0] == 18 && cid[1] == 32 {
		mt := mh.Multihash(cid)
		return b58.Encode(mt)
	}
	mbstr, err := mbase.Encode(mbase.Base32, cid)
	if err != nil {
		return ""
	}
	return mbstr
}

type signedNativeTx struct {
	Message   *nativeTx `json:"Message"`
	Signature struct {
		Type byte
		Data []byte
	} `json:"Signature"`
}
