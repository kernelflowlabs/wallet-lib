package solana

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"unicode/utf8"

	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/mr-tron/base58"
)

const MaxDappMessageBytes = 16 * 1024

type SignedDappTransaction struct {
	SignedTx    string `json:"signedTx"`
	Signature   string `json:"signature"`
	SignerIndex int    `json:"signerIndex"`
}

// SignSerializedTransaction signs a transaction a dApp built, keeping whatever
// signatures it already carries.
//
// The message is deserialized and serialized again and the result must equal
// the input byte for byte. Without that check we would sign our own
// re-encoding of the transaction rather than the bytes the dApp produced and
// the user was shown, and any disagreement between the two encoders would be
// invisible.
func SignSerializedTransaction(privateKey, txBytes []byte) (*SignedDappTransaction, error) {
	tx, err := SafeTransactionDeserialize(txBytes)
	if err != nil {
		return nil, err
	}
	roundTrip, err := tx.Serialize()
	if err != nil {
		return nil, fmt.Errorf("transaction cannot be re-serialized: %w", err)
	}
	if !bytes.Equal(roundTrip, txBytes) {
		return nil, fmt.Errorf("transaction does not survive a serialization round trip; refusing to sign a re-encoding")
	}

	publicKey, err := key.PrivateKey2PublicKeyED25519(privateKey)
	if err != nil {
		return nil, fmt.Errorf("derive public key: %w", err)
	}
	address, err := PublicKey2Address(publicKey)
	if err != nil {
		return nil, fmt.Errorf("derive address: %w", err)
	}

	signerIndex := -1
	for i := 0; i < int(tx.Message.Header.NumRequireSignatures) && i < len(tx.Message.Accounts); i++ {
		if tx.Message.Accounts[i].ToBase58() == address {
			signerIndex = i
			break
		}
	}
	if signerIndex < 0 {
		return nil, fmt.Errorf("%s is not among the required signers", address)
	}

	messageBytes, err := tx.Message.Serialize()
	if err != nil {
		return nil, fmt.Errorf("serialize message: %w", err)
	}
	signature, err := key.SignWithPrivateKeyED25519(privateKey, messageBytes)
	if err != nil {
		return nil, fmt.Errorf("sign message: %w", err)
	}
	if err := tx.AddSignature(signature); err != nil {
		return nil, fmt.Errorf("attach signature: %w", err)
	}
	signed, err := tx.Serialize()
	if err != nil {
		return nil, fmt.Errorf("serialize signed transaction: %w", err)
	}
	return &SignedDappTransaction{
		SignedTx:    base64.StdEncoding.EncodeToString(signed),
		Signature:   base58.Encode(signature),
		SignerIndex: signerIndex,
	}, nil
}

// SignMessage signs arbitrary bytes with ed25519, which is what a dApp's
// signMessage expects.
//
// Solana has no domain separator for signed messages, so a signature over a
// payload that happens to be a valid transaction message can be lifted
// straight into a transaction and broadcast. Anything that deserializes as a
// message is refused for that reason, as is the v0 version byte, which no
// legitimate text starts with.
func SignMessage(privateKey, message []byte) (string, error) {
	if len(message) == 0 {
		return "", fmt.Errorf("message is empty")
	}
	if len(message) > MaxDappMessageBytes {
		return "", fmt.Errorf("message is %d bytes, limit is %d", len(message), MaxDappMessageBytes)
	}
	if !utf8.Valid(message) {
		return "", fmt.Errorf("message is not valid UTF-8; signMessage is for text a person can read")
	}
	if looksLikeTransaction(message) {
		return "", fmt.Errorf("message parses as a Solana transaction message; refusing to sign a transaction disguised as text")
	}
	signature, err := key.SignWithPrivateKeyED25519(privateKey, message)
	if err != nil {
		return "", fmt.Errorf("sign message: %w", err)
	}
	return base58.Encode(signature), nil
}

// looksLikeTransaction is deliberately not a byte-prefix test. An earlier
// version rejected anything whose first byte had the high bit set, meaning to
// catch the v0 marker 0x80 — but every Chinese, Japanese and emoji character
// also starts that way in UTF-8, so a message beginning "登录" was refused.
// The UTF-8 check in SignMessage already excludes 0x80, which is never a valid
// leading byte, so what is left is the only question that matters: does this
// parse as something that could be broadcast?
func looksLikeTransaction(message []byte) bool {
	if _, err := SafeMessageDeserialize(message); err == nil {
		return true
	}
	if _, err := SafeTransactionDeserialize(message); err == nil {
		return true
	}
	return false
}
