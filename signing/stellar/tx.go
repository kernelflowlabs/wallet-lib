package stellar

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/kernelflowlabs/wallet-lib/signing"
)

const (
	PublicNetworkPassphrase = "Public Global Stellar Network ; September 2015"
	TestNetworkPassphrase   = "Test SDF Network ; September 2015"
	MinBaseFee              = 100
	MinAccountBalance       = 10000000
)

func NewTxBuilder(ti *Ingredient) *TxBuilder {
	return &TxBuilder{Ingredient: ti}
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
	senderBytes, err := decode(VersionByteAccountID, tx.Ingredient.Sender)
	if err != nil {
		return fmt.Errorf("fail to decode Sender, err=%v", err)
	}
	recipientBytes, err := decode(VersionByteAccountID, tx.Ingredient.Recipient)
	if err != nil {
		return fmt.Errorf("fail to decode Recipient, err=%v", err)
	}
	amount, err := strconv.ParseInt(tx.Ingredient.Amount, 10, 64)
	if err != nil {
		return fmt.Errorf("fail to ParseInt for Amount, err=%v", err)
	}
	sequence, err := strconv.ParseInt(tx.Ingredient.Sequence, 10, 64)
	if err != nil {
		return fmt.Errorf("fail to ParseInt for Sequence, err=%v", err)
	}

	op := stellarOp{destKey: recipientBytes, amount: amount}
	if tx.Ingredient.IsRecipientActivated == "true" {
		op.isPayment = true
	} else if amount < MinAccountBalance {
		return fmt.Errorf("amount %d is below the %d stroops needed to create an account", amount, MinAccountBalance)
	}
	mtx := stellarTx{
		sourceKey: senderBytes,
		fee:       MinBaseFee,
		seqNum:    sequence + 1,
		memo:      tx.Ingredient.Memo,
		op:        op,
	}

	networkID := sha256.Sum256([]byte(PublicNetworkPassphrase))
	sigPayloadSum := sha256.Sum256(mtx.marshalSigPayload(networkID))
	tx.sigHash = append(tx.sigHash, hex.EncodeToString(sigPayloadSum[:]))
	tx.unsignedHex = hex.EncodeToString(mtx.marshalEnvelope(nil, nil))
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
	sk := ed25519.NewKeyFromSeed(privateKey)
	publicKey := sk.Public().(ed25519.PublicKey)
	var hint [4]byte
	copy(hint[:], publicKey[28:])
	sig := ed25519.Sign(sk, sigHash)
	return hex.EncodeToString(hint[:]) + "_" + hex.EncodeToString(sig), nil
}

func (tx *TxBuilder) ConcatSignature(signature string, isDerFormat bool) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("tx == nil")
	} else if tx.unsignedHex == "" {
		return "", fmt.Errorf("tx.UnsignedHex == nil")
	}
	envBytes, err := hex.DecodeString(tx.unsignedHex)
	if err != nil {
		return "", fmt.Errorf("fail to DecodeString for UnsignedHex, err=%v", err)
	}
	if isDerFormat {
		return "", fmt.Errorf("der format not supported")
	}
	tmp := strings.Split(signature, "_")
	if len(tmp) != 2 {
		return "", fmt.Errorf("invalid signature")
	}
	tmpHint, err := hex.DecodeString(tmp[0])
	if err != nil {
		return "", fmt.Errorf("fail to DecodeString for hint, err=%v", err)
	}
	sig, err := hex.DecodeString(tmp[1])
	if err != nil {
		return "", fmt.Errorf("fail to DecodeString for sig, err=%v", err)
	}
	if len(envBytes) < 4 {
		return "", fmt.Errorf("invalid envelope")
	}
	if binary.BigEndian.Uint32(envBytes[len(envBytes)-4:]) != 0 {
		return "", fmt.Errorf("envelope already signed")
	}
	w := &xw{b: append([]byte(nil), envBytes[:len(envBytes)-4]...)}
	w.u32(1)
	w.raw(tmpHint)
	w.opaque(sig)

	tx.txHash = tx.sigHash[0]
	return hex.EncodeToString(w.b), nil
}

func (tx *TxBuilder) GetTxHash() string       { return tx.txHash }
func (tx *TxBuilder) GetSigHash() []string    { return tx.sigHash }
func (tx *TxBuilder) GetUnsignedHex() string  { return tx.unsignedHex }
func (tx *TxBuilder) SetSigHash(s []string)   { tx.sigHash = s }
func (tx *TxBuilder) SetUnsignedHex(s string) { tx.unsignedHex = s }

type (
	Ingredient struct {
		TxType               string `json:"txType"`
		Sender               string `json:"sender"`
		Recipient            string `json:"recipient"`
		ContractAddress      string `json:"contractAddress"`
		Amount               string `json:"amount"`
		Memo                 string `json:"memo"`
		Sequence             string `json:"sequence"`
		IsRecipientActivated string `json:"isRecipientActivated"`
	}
	TxBuilder struct {
		*Ingredient
		unsignedHex string
		sigHash     []string
		txHash      string
	}
)
