package kaspa

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/kernelflowlabs/wallet-lib/signing"

	"github.com/kaspanet/kaspad/app/appmessage"
	"github.com/kaspanet/kaspad/domain/consensus/model/externalapi"
	"github.com/kaspanet/kaspad/domain/consensus/utils/consensushashing"
	"github.com/kaspanet/kaspad/domain/consensus/utils/constants"
	kasutxo "github.com/kaspanet/kaspad/domain/consensus/utils/utxo"
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
	senderVersion, senderPubkey, err := decodeAddress(tx.Ingredient.Sender, bech32PrefixKaspaMainnet)
	if err != nil {
		return fmt.Errorf("failed to DecodeAddress for Sender, err=%v", err)
	}
	if senderVersion != pubKeyAddrID {
		return fmt.Errorf("sender must be a Schnorr public key address, got version %d", senderVersion)
	}
	senderScript, err := NewScriptBuilder().AddData(senderPubkey).AddOp(OpCheckSig).Script()
	if err != nil {
		return fmt.Errorf("failed to NewScriptBuilder for sender, err=%v", err)
	}

	var recipientScript []byte
	fee, _ := strconv.ParseUint(tx.Ingredient.Fee, 10, 64)
	var redeemScript []byte
	totalSend := uint64(0)
	if tx.Ingredient.TxType == signing.TxTypeTransfer {
		recipientVersion, recipientPayload, err := decodeAddress(tx.Ingredient.Recipient, bech32PrefixKaspaMainnet)
		if err != nil {
			return fmt.Errorf("failed to DecodeAddress for Recipient=%s, err=%v",
				tx.Ingredient.Recipient, err)
		}
		recipientScript, err = payToAddressScript(recipientVersion, recipientPayload)
		if err != nil {
			return fmt.Errorf("failed to NewScriptBuilder for recipient, err=%v", err)
		}
		totalSend, _ = strconv.ParseUint(tx.Ingredient.Amount, 10, 64)
		if totalSend <= DefaultDust {
			return fmt.Errorf("dust amount")
		}
	} else if tx.Ingredient.TxType == signing.TxTypeKrc20Commit {
		redeemScript, _ = hex.DecodeString(tx.Ingredient.Krc20RedeemScript)
		p2shScript, err := PayToScriptHashScript(redeemScript)
		if err != nil {
			return fmt.Errorf("failed to PayToScriptHashScript, err=%v", err)
		}
		recipientScript = p2shScript
		totalSend, _ = strconv.ParseUint(tx.Ingredient.Amount, 10, 64)
		if totalSend <= DefaultDust {
			return fmt.Errorf("dust amount")
		}
	} else if tx.TxType == signing.TxTypeKrc20Reveal {
		recipientScript, err = NewScriptBuilder().AddData(senderPubkey).AddOp(OpCheckSig).Script()
		if err != nil {
			return fmt.Errorf("failed to NewScriptBuilder for recipient, err=%v", err)
		}
		if len(tx.Ingredient.Utxos.List) != 1 {
			return fmt.Errorf("only 1 utxo supported")
		}
		value, _ := strconv.ParseUint(tx.Ingredient.Utxos.List[0].Value, 10, 64)
		if fee >= value {
			return fmt.Errorf("fee %d exceeds the utxo value %d", fee, value)
		}
		totalSend = value - fee
		if totalSend <= DefaultDust {
			return fmt.Errorf("dust amount")
		}
	}

	totalNeed := totalSend + fee
	utxoList, totalHave, err := selectUtxosFilter(tx.Ingredient.Utxos, totalNeed)
	if err != nil {
		return fmt.Errorf("failed to selectUtxosFilter, err=%v", err)
	}
	if totalNeed > totalHave {
		return fmt.Errorf("totalNeed %d > totalHave %d", totalNeed, totalHave)
	}
	change := totalHave - totalNeed

	//bytesLen := int64(len(utxoList.List)*180 + (1)*34 + 10)
	//byteFee := bytesLen * feeRate
	inputs := make([]*externalapi.DomainTransactionInput, len(utxoList.List))
	for i, utxo := range utxoList.List {
		transactionIDBytes, _ := hex.DecodeString(utxo.Hash)
		var hashArray [32]byte
		copy(hashArray[:], transactionIDBytes)
		index, _ := strconv.ParseUint(utxo.Index, 10, 32)
		outpoint := externalapi.DomainOutpoint{
			TransactionID: *externalapi.NewDomainTransactionIDFromByteArray(&hashArray),
			Index:         uint32(index),
		}
		scriptPubkey, _ := hex.DecodeString(utxo.Script)
		amt, _ := strconv.ParseUint(utxo.Value, 10, 64)
		version, _ := strconv.ParseUint(utxo.Version, 10, 16)
		blockDAAScore := uint64(0)
		if tx.TxType != signing.TxTypeKrc20Reveal {
			blockDAAScore, _ = strconv.ParseUint(utxo.BlockDAAScore, 10, 64)
		}
		isCoinbase := utxo.IsCoinbase != "false"
		inputs[i] = &externalapi.DomainTransactionInput{
			PreviousOutpoint: outpoint,
			SigOpCount:       1,
			UTXOEntry: kasutxo.NewUTXOEntry(amt, &externalapi.ScriptPublicKey{
				Script:  scriptPubkey,
				Version: uint16(version),
			}, isCoinbase, blockDAAScore),
		}
	}

	var outputs []*externalapi.DomainTransactionOutput
	outputs = append(outputs, &externalapi.DomainTransactionOutput{
		Value: totalSend,
		ScriptPublicKey: &externalapi.ScriptPublicKey{
			Script:  recipientScript,
			Version: uint16(addressPublicKeyScriptPublicKeyVersion),
		},
	})
	if change >= DefaultDust {
		outputs = append(outputs, &externalapi.DomainTransactionOutput{
			Value: change,
			ScriptPublicKey: &externalapi.ScriptPublicKey{
				Script:  senderScript,
				Version: uint16(addressPublicKeyScriptPublicKeyVersion),
			},
		})
	}
	dtx := &externalapi.DomainTransaction{
		Version:      constants.MaxTransactionVersion,
		Inputs:       inputs,
		Outputs:      outputs,
		LockTime:     0,
		SubnetworkID: externalapi.DomainSubnetworkID{},
		Gas:          0,
		Payload:      nil,
	}
	for idx := range dtx.Inputs {
		sigHash, err := consensushashing.CalculateSignatureHashSchnorr(dtx, idx,
			consensushashing.SigHashAll, &consensushashing.SighashReusedValues{})
		if err != nil {
			return fmt.Errorf("failed to CalculateSignatureHashSchnorr for ntx, err=%v", err)
		}
		tx.sigHash = append(tx.sigHash, sigHash.String())
	}

	wrapper := DomainTransactionWrapper{
		dtx,
	}
	wrapperBytes, err := wrapper.MarshalJSON()
	if err != nil {
		return fmt.Errorf("failed to Marshal for ntx, err=%v", err)
	}

	tx.unsignedHex = hex.EncodeToString(wrapperBytes)

	return nil
}

func (tx *TxBuilder) Sign(privateKey []byte) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("tx == nil")
	} else if len(tx.sigHash) < 1 {
		return "", fmt.Errorf("tx.SigHash == nil")
	}
	var signatureList string
	for idx, sigHash := range tx.sigHash {
		hash, err := hex.DecodeString(sigHash)
		if err != nil {
			return "", fmt.Errorf("failed to DecodeString for SigHash, err=%v", err)
		}
		signature, err := key.SignWithPrivateKeySchnorr(privateKey, hash)
		if err != nil {
			return "", fmt.Errorf("failed to SchnorrSign, err=%v", err)
		}
		sigBytes := append(signature, byte(consensushashing.SigHashAll))

		if idx == 0 {
			signatureList = hex.EncodeToString(sigBytes)
		} else {
			signatureList = signatureList + "_" + hex.EncodeToString(sigBytes)
		}
	}

	return signatureList, nil
}

func (tx *TxBuilder) ConcatSignature(signature string, isDerFormat bool) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("tx == nil")
	}

	tmpUnsignedHex := strings.Split(tx.unsignedHex, "_")
	if len(tmpUnsignedHex) == 2 {
		return tx.unsignedHex, nil
	}

	wrapperBytes, err := hex.DecodeString(tx.unsignedHex)
	if err != nil {
		return "", fmt.Errorf("failed to DecodeString for UnsignedHex, err=%v", err)
	}
	var wrapper DomainTransactionWrapper
	err = wrapper.UnmarshalJSON(wrapperBytes)
	if err != nil {
		return "", fmt.Errorf("failed to UnmarshalJSON for wrapperBytes, err=%v", err)
	}

	var tmp []string
	tmp = strings.Split(signature, "_")
	if len(tmp) != len(wrapper.DomainTransaction.Inputs) {
		return "", fmt.Errorf("length of signature not match")
	}
	sig := make([][]byte, len(tmp))
	if isDerFormat {
		for k, v := range tmp {
			vv, err := hex.DecodeString(v + "01")
			if err != nil {
				return "", fmt.Errorf("failed to DecodeString for signature, err=%v", err)
			}
			sig[k] = vv
		}
	} else {
		for k, v := range tmp {
			vv, err := hex.DecodeString(v)
			if err != nil {
				return "", fmt.Errorf("failed to DecodeString for signature, err=%v", err)
			}
			sig[k] = vv
		}
	}
	if tx.Ingredient.TxType == signing.TxTypeKrc20Reveal {
		redeemScript, err := hex.DecodeString(tx.Ingredient.Krc20RedeemScript)
		if err != nil {
			return "", fmt.Errorf("failed to DecodeString for RedeemScript, err=%v", err)
		}
		for idx, txin := range wrapper.DomainTransaction.Inputs {
			signatureScript, err := NewScriptBuilder().AddData(sig[idx]).AddData(redeemScript).Script()
			if err != nil {
				return "", fmt.Errorf("failed to NewScriptBuilder, err=%v", err)
			}
			txin.SignatureScript = signatureScript
		}
	} else {
		for idx, txin := range wrapper.DomainTransaction.Inputs {
			signatureScript, err := NewScriptBuilder().AddData(sig[idx]).Script()
			if err != nil {
				return "", fmt.Errorf("failed to NewScriptBuilder, err=%v", err)
			}
			txin.SignatureScript = signatureScript
		}
	}

	tx.txHash = consensushashing.TransactionID(wrapper.DomainTransaction).String()
	rpcTx := appmessage.DomainTransactionToRPCTransaction(wrapper.DomainTransaction)
	rpcTxBytes, err := json.Marshal(rpcTx)
	if err != nil {
		return "", fmt.Errorf("failed to Marshal for rpcTransaction, err=%v", err)
	}
	stx := signedTx{
		SignedHex: hex.EncodeToString(rpcTxBytes),
		TxHash:    tx.txHash,
	}
	stxBytes, err := json.Marshal(&stx)
	if err != nil {
		return "", fmt.Errorf("failed to Marshal for stx, err=%v", err)
	}
	return hex.EncodeToString(stxBytes), nil
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

func selectUtxosFilter(ori *signing.UtxoList, totalNeed uint64) (*signing.UtxoList, uint64, error) {
	sort.Slice(ori.List, func(i, j int) bool {
		amountOne, _ := strconv.ParseInt(ori.List[i].Value, 10, 64)
		amountTwo, _ := strconv.ParseInt(ori.List[j].Value, 10, 64)
		return amountOne < amountTwo
	})
	selectedValue := uint64(0)
	dst := &signing.UtxoList{}
	for _, v := range ori.List {
		amount, _ := strconv.ParseUint(v.Value, 10, 64)
		if amount <= DefaultDust {
			continue
		}
		selectedValue += amount
		dst.List = append(dst.List, v)
		if selectedValue >= totalNeed {
			break
		}
	}
	return dst, selectedValue, nil
}

// types
type (
	Ingredient struct {
		TxType            string            `json:"txType" validate:"required,oneof=0 1 2 3 4 5 6"`
		ContractAddress   string            `json:"contractAddress,omitempty" validate:"omitempty,native"`
		Sender            string            `json:"sender" validate:"required,kas_addr"`
		Recipient         string            `json:"recipient,omitempty" validate:"omitempty,kas_addr"`
		Amount            string            `json:"amount,omitempty" validate:"omitempty,bigint_gt0"`
		Fee               string            `json:"fee,omitempty" validate:"omitempty,u64_gt0"`
		Utxos             *signing.UtxoList `json:"utxos" validate:"required"`
		Krc20RedeemScript string            `json:"krc20RedeemScript,omitempty" validate:"omitempty,hex_str"`
		//Krc20P2SHAddress  string          `json:"krcP2SHAddress,omitempty" validate:"omitempty,hex_str"`
	}
	TxBuilder struct {
		*Ingredient
		unsignedHex string
		sigHash     []string
		txHash      string
	}
	signedTx struct {
		SignedHex string
		TxHash    string
	}
)

const DefaultDust = 2000000

const (
	OpCheckSig = 172
	OpFalse    = 0
	OpIf       = 99
	OpEndIf    = 104
)
