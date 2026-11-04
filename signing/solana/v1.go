package solana

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math/bits"
	"strconv"

	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/types"
	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/mr-tron/base58"
)

type V1Transaction struct {
	Message      []byte
	Signatures   []types.Signature
	Header       types.MessageHeader
	Addresses    []common.PublicKey
	Blockhash    string
	ConfigMask   uint32
	ConfigValues []uint32
	Instructions []types.CompiledInstruction
}

func ParseV1Transaction(data []byte) (*V1Transaction, error) {
	if err := ValidateTransactionEnvelope(data); err != nil {
		return nil, err
	}
	if data[0] != 0x81 {
		return nil, fmt.Errorf("transaction is not v1")
	}
	data = append([]byte(nil), data...)
	end := len(data) - int(data[1])*ed25519.SignatureSize
	tx := &V1Transaction{
		Message:    data[:end:end],
		Header:     types.MessageHeader{NumRequireSignatures: data[1], NumReadonlySignedAccounts: data[2], NumReadonlyUnsignedAccounts: data[3]},
		Blockhash:  base58.Encode(data[8:40]),
		ConfigMask: binary.LittleEndian.Uint32(data[4:8]),
	}
	for at := end; at < len(data); at += ed25519.SignatureSize {
		tx.Signatures = append(tx.Signatures, types.Signature(data[at:at+ed25519.SignatureSize:at+ed25519.SignatureSize]))
	}
	at := 42
	for i := 0; i < int(data[41]); i++ {
		tx.Addresses = append(tx.Addresses, common.PublicKeyFromBytes(data[at:at+32]))
		at += 32
	}
	for i := 0; i < bits.OnesCount32(tx.ConfigMask); i++ {
		tx.ConfigValues = append(tx.ConfigValues, binary.LittleEndian.Uint32(data[at:at+4]))
		at += 4
	}
	headers := data[at : at+int(data[40])*4]
	at += len(headers)
	for i := 0; i < len(headers); i += 4 {
		h := headers[i : i+4]
		instruction := types.CompiledInstruction{ProgramIDIndex: int(h[0])}
		for _, index := range data[at : at+int(h[1])] {
			instruction.Accounts = append(instruction.Accounts, int(index))
		}
		at += int(h[1])
		size := int(binary.LittleEndian.Uint16(h[2:4]))
		instruction.Data = data[at : at+size : at+size]
		at += size
		tx.Instructions = append(tx.Instructions, instruction)
	}
	return tx, nil
}

func (tx *V1Transaction) descriptionMessage() types.Message {
	return types.Message{Version: "v1", Header: tx.Header, Accounts: tx.Addresses, RecentBlockHash: tx.Blockhash, Instructions: tx.Instructions}
}

func (tx *V1Transaction) withSignature(index int, signature []byte) ([]byte, error) {
	if index < 0 || index >= len(tx.Signatures) {
		return nil, fmt.Errorf("invalid v1 signer slot")
	}
	for i, existing := range tx.Signatures {
		if !isZeroSignature(existing) && !ed25519.Verify(tx.Addresses[i].Bytes(), tx.Message, existing) {
			return nil, fmt.Errorf("invalid existing signature in slot %d", i)
		}
	}
	if !ed25519.Verify(tx.Addresses[index].Bytes(), tx.Message, signature) {
		return nil, fmt.Errorf("invalid signature for slot %d", index)
	}
	data := append([]byte(nil), tx.Message...)
	for i, existing := range tx.Signatures {
		if i == index {
			data = append(data, signature...)
		} else {
			data = append(data, existing...)
		}
	}
	return data, nil
}

func signV1Transaction(privateKey, data []byte) (*SignedDappTransaction, error) {
	tx, err := ParseV1Transaction(data)
	if err != nil {
		return nil, err
	}
	publicKey, err := key.PrivateKey2PublicKeyED25519(privateKey)
	if err != nil {
		return nil, fmt.Errorf("derive public key: %w", err)
	}
	address, err := PublicKey2Address(publicKey)
	if err != nil {
		return nil, fmt.Errorf("derive address: %w", err)
	}
	index := -1
	for i := range tx.Signatures {
		if tx.Addresses[i].ToBase58() == address {
			index = i
			break
		}
	}
	if index < 0 {
		return nil, fmt.Errorf("%s is not among the required signers", address)
	}
	signature, err := key.SignWithPrivateKeyED25519(privateKey, tx.Message)
	if err != nil {
		return nil, fmt.Errorf("sign message: %w", err)
	}
	signed, err := tx.withSignature(index, signature)
	if err != nil {
		return nil, err
	}
	return &SignedDappTransaction{SignedTx: base64.StdEncoding.EncodeToString(signed), Signature: base58.Encode(signature), SignerIndex: index}, nil
}

func (tx *V1Transaction) descriptionConfig() map[string]interface{} {
	config := make(map[string]interface{})
	at := 0
	if tx.ConfigMask&3 == 3 {
		config["priorityFeeLamports"] = strconv.FormatUint(uint64(tx.ConfigValues[0])|uint64(tx.ConfigValues[1])<<32, 10)
		at = 2
	}
	for bit, name := range []string{"computeUnitLimit", "loadedAccountsDataSizeLimit", "heapSize"} {
		if tx.ConfigMask&(1<<uint(bit+2)) != 0 {
			config[name] = tx.ConfigValues[at]
			at++
		}
	}
	return config
}
