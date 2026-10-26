package utxo

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/kernelflowlabs/wallet-lib/signing"
	"strconv"
	"strings"

	btcaddr "github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/psbt/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
)

func NewTxBuilder(ti *Ingredient, network string) *TxBuilder {
	return &TxBuilder{
		Ingredient: ti,
		network:    network,
	}
}

func NewTxBuilderFromUnsignedHex(unsignedHex, publicKey, network string) (*TxBuilder, error) {
	raw, err := hex.DecodeString(unsignedHex)
	if err != nil {
		return nil, fmt.Errorf("failed to DecodeString, err=%v", err)
	}

	packet, err := psbt.NewFromRawBytes(bytes.NewReader(raw), false)
	if err != nil {
		return nil, fmt.Errorf("failed to parse as PSBT, err=%v", err)
	}

	sigHashes, err := extractSigHashes(packet)
	if err != nil {
		return nil, fmt.Errorf("failed to extract sigHashes, err=%v", err)
	}

	return &TxBuilder{
		unsignedHex: unsignedHex,
		sigHash:     sigHashes,
		network:     network,
		Ingredient: &Ingredient{
			SenderPublicKey: publicKey,
			IsPSBT:          true,
		},
	}, nil
}

func (tx *TxBuilder) Build() error {
	if tx == nil {
		return fmt.Errorf("tx == nil")
	}
	if tx.Ingredient == nil {
		return fmt.Errorf("ingredient == nil")
	}
	if tx.Ingredient.Utxos == nil || len(tx.Ingredient.Utxos.List) == 0 {
		return fmt.Errorf("no utxos")
	}
	if !ValidAddress(tx.Ingredient.Recipient, tx.network) {
		return fmt.Errorf("invalid recipient address")
	}
	if !ValidAddress(tx.Ingredient.Sender, tx.network) {
		return fmt.Errorf("invalid sender address")
	}
	if byteFee, err := strconv.ParseInt(tx.Ingredient.ByteFee, 10, 64); err != nil || byteFee <= 0 {
		return fmt.Errorf("invalid byteFee %q", tx.Ingredient.ByteFee)
	}
	if tx.Ingredient.Amount != signing.MagicNumberForMaxAmount {
		if v, err := strconv.ParseInt(tx.Ingredient.Amount, 10, 64); err != nil || v <= 0 {
			return fmt.Errorf("invalid amount %q", tx.Ingredient.Amount)
		}
	}
	for _, u := range tx.Ingredient.Utxos.List {
		if v, err := strconv.ParseInt(u.Value, 10, 64); err != nil || v <= 0 {
			return fmt.Errorf("invalid utxo value %q", u.Value)
		}
	}
	tx.sigHash = nil
	tx.unsignedHex = ""
	tx.txHash = ""

	var sigHash []string
	var unsignedHex string
	var err error
	switch tx.network {
	case NetworkEnumForBTC:
		sigHash, unsignedHex, err = buildForBTC(tx.Ingredient)
	case NetworkEnumForBTCP2TR:
		sigHash, unsignedHex, err = buildForBTCP2TR(tx.Ingredient)
	case NetworkEnumForLTC:
		sigHash, unsignedHex, err = buildForLTC(tx.Ingredient)
	case NetworkEnumForDOGE:
		sigHash, unsignedHex, err = buildForDOGE(tx.Ingredient)
	case NetworkEnumForSYS:
		sigHash, unsignedHex, err = buildForSYS(tx.Ingredient)
	default:
		return fmt.Errorf("unsupported network")
	}
	if err != nil {
		return err
	}
	tx.sigHash = sigHash
	tx.unsignedHex = unsignedHex
	return nil
}

func (tx *TxBuilder) Sign(privateKey []byte) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("tx == nil")
	} else if len(tx.sigHash) < 1 {
		return "", fmt.Errorf("tx.SigHash too short")
	}
	if tx.network == NetworkEnumForBTCP2TR {
		return tx.signTaproot(privateKey)
	}
	var signatureList string
	for idx, v := range tx.sigHash {
		hash, err := hex.DecodeString(v)
		if err != nil {
			return "", fmt.Errorf("failed to DecodeString for hash, err=%v", err)
		}
		sig, err := key.SignWithPrivateKeyECDSAForUTXO(privateKey, hash)
		if err != nil {
			return "", fmt.Errorf("failed to SignWithPrivateKeyECDSAForUTXO, err=%v", err)
		}
		signature := append(sig, byte(txscript.SigHashAll))
		if idx == 0 {
			signatureList = hex.EncodeToString(signature)
		} else {
			signatureList = signatureList + "_" + hex.EncodeToString(signature)
		}
	}
	return signatureList, nil
}

func (tx *TxBuilder) signTaproot(privateKey []byte) (string, error) {
	prvKey, _ := btcec.PrivKeyFromBytes(privateKey)
	tweaked := txscript.TweakTaprootPrivKey(*prvKey, nil)
	signatures := make([]string, 0, len(tx.sigHash))
	for _, v := range tx.sigHash {
		hash, err := hex.DecodeString(v)
		if err != nil {
			return "", fmt.Errorf("failed to DecodeString for hash, err=%v", err)
		}
		sig, err := schnorr.Sign(tweaked, hash)
		if err != nil {
			return "", fmt.Errorf("failed to Sign with schnorr, err=%v", err)
		}
		signatures = append(signatures, hex.EncodeToString(sig.Serialize()))
	}
	return strings.Join(signatures, "_"), nil
}

func (tx *TxBuilder) ConcatSignature(signature string, isDerFormat bool) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("tx == nil")
	} else if len(tx.sigHash) < 1 {
		return "", fmt.Errorf("tx.SigHash == nil")
	}

	pkData, err := hex.DecodeString(tx.Ingredient.SenderPublicKey)
	if err != nil {
		return "", fmt.Errorf("failed to DecodeString for SenderPublicKey, err=%v", err)
	}

	unsignedBytes, err := hex.DecodeString(tx.unsignedHex)
	if err != nil {
		return "", fmt.Errorf("failed to DecodeString, err=%v", err)
	}
	msgTx := &wire.MsgTx{}
	packet := &psbt.Packet{}
	if tx.Ingredient.IsPSBT {
		packet, err = psbt.NewFromRawBytes(bytes.NewReader(unsignedBytes), false)
		if err != nil {
			return "", fmt.Errorf("failed to Deserialize for packet, err=%v", err)
		}
		msgTx = packet.UnsignedTx
	} else {
		err = msgTx.Deserialize(bytes.NewReader(unsignedBytes))
		if err != nil {
			return "", fmt.Errorf("failed to Deserialize for msgTx, err=%v", err)
		}
	}

	tmp := strings.Split(signature, "_")
	if len(tmp) != len(msgTx.TxIn) {
		return "", fmt.Errorf("length of signature not match")
	}
	if isDerFormat {
		return "", fmt.Errorf("DER signature format not supported")
	}
	sig := make([][]byte, len(tmp))
	for k, v := range tmp {
		vv, err := hex.DecodeString(v)
		if err != nil {
			return "", fmt.Errorf("failed to DecodeString for signature, err=%v", err)
		}
		sig[k] = vv
	}

	var txHash string
	switch tx.network {
	case NetworkEnumForBTC:
		if tx.IsPSBT {
			updater, err := psbt.NewUpdater(packet)
			if err != nil {
				return "", fmt.Errorf("failed to NewUpdater, err=%v", err)
			}
			for idx := range msgTx.TxIn {
				outcome, err := updater.Sign(idx, sig[idx], pkData, nil, nil)
				if err != nil {
					return "", fmt.Errorf("failed to updater.Sign, err=%v", err)
				} else if outcome != 0 {
					return "", fmt.Errorf("failed to updater.Sign, outcome != 0")
				}
			}
			err = psbt.MaybeFinalizeAll(packet)
			if err != nil {
				return "", fmt.Errorf("failed to MaybeFinalizeAll, err=%v", err)
			}
			msgTx, err = psbt.Extract(packet)
			if err != nil {
				return "", fmt.Errorf("failed to Extract, err=%v", err)
			}
			txHash = msgTx.TxHash().String()
		} else {
			for idx, txin := range msgTx.TxIn {
				txin.Witness = wire.TxWitness{sig[idx], pkData}
			}
			txHash = msgTx.TxHash().String()
		}
	case NetworkEnumForBTCP2TR:
		for idx, txin := range msgTx.TxIn {
			if len(sig[idx]) != schnorr.SignatureSize {
				return "", fmt.Errorf("taproot signature %d has %d bytes, want %d", idx, len(sig[idx]), schnorr.SignatureSize)
			}
			txin.Witness = wire.TxWitness{sig[idx]}
		}
		txHash = msgTx.TxHash().String()
	case NetworkEnumForLTC:
		for idx, txin := range msgTx.TxIn {
			txin.Witness = wire.TxWitness{sig[idx], pkData}
		}
		txHash = msgTx.TxHash().String()
	case NetworkEnumForDOGE:
		for idx, txin := range msgTx.TxIn {
			scriptBuiler, err := txscript.NewScriptBuilder().AddData(sig[idx]).AddData(pkData).Script()
			if err != nil {
				return "", fmt.Errorf("failed to NewScriptBuilder, err=%v", err)
			}
			txin.SignatureScript = scriptBuiler
		}
		txHash = msgTx.TxHash().String()
	case NetworkEnumForSYS:
		updater, err := psbt.NewUpdater(packet)
		if err != nil {
			return "", fmt.Errorf("failed to NewUpdater, err=%v", err)
		}
		for idx, _ := range msgTx.TxIn {
			outcome, err := updater.Sign(idx, sig[idx], pkData, nil, nil)
			if err != nil {
				return "", fmt.Errorf("failed to updater.Sign, err=%v", err)
			} else if outcome != 0 {
				return "", fmt.Errorf("failed to updater.Sign, outcome != 0")
			}
		}
		err = psbt.MaybeFinalizeAll(packet)
		if err != nil {
			return "", fmt.Errorf("failed to MaybeFinalizeAll, err=%v", err)
		}
		msgTx, err = psbt.Extract(packet)
		if err != nil {
			return "", fmt.Errorf("failed to Extract, err=%v", err)
		}
		txHash = msgTx.TxHash().String()
	}
	var signedBytes bytes.Buffer
	err = msgTx.Serialize(&signedBytes)
	if err != nil {
		return "", fmt.Errorf("failed to Serialize, err=%v", err)
	}
	tx.txHash = txHash
	return hex.EncodeToString(signedBytes.Bytes()), nil
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

// types
const DefaultInputBytes = 148
const DefaultOutputBytes = 34
const DefaultDust = 546
const MaxMemoBytes = 80
const DogeDust = 100000

const (
	vbytesP2WPKHInput = 68
	vbytesP2TRInput   = 58
	bytesP2PKHInput   = 148
	bytesOutput       = 34
	bytesOverhead     = 11
)

func estimateVBytes(network string, numInputs, numOutputs int) int64 {
	return int64(numInputs)*perInputVBytes(network) + int64(numOutputs)*bytesOutput + bytesOverhead
}

func buildMemoScript(memo string) ([]byte, error) {
	if memo == "" {
		return nil, nil
	}
	data, err := hex.DecodeString(strings.TrimPrefix(memo, "0x"))
	if err != nil {
		return nil, fmt.Errorf("invalid memo: %v", err)
	}
	if len(data) > MaxMemoBytes {
		return nil, fmt.Errorf("memo too long: %d bytes, max %d", len(data), MaxMemoBytes)
	}
	script, err := txscript.NewScriptBuilder().AddOp(txscript.OP_RETURN).AddData(data).Script()
	if err != nil {
		return nil, fmt.Errorf("failed to Script for memo, err=%v", err)
	}
	return script, nil
}

func memoOutputVBytes(script []byte) int64 {
	if script == nil {
		return 0
	}
	return outputVBytes(script)
}

func estimateFee(network string, byteFee int64, numInputs, numOutputs int) int64 {
	return estimateVBytes(network, numInputs, numOutputs) * byteFee
}

func outputVBytes(script []byte) int64 {
	return int64(8 + wire.VarIntSerializeSize(uint64(len(script))) + len(script))
}

func perInputVBytes(network string) int64 {
	switch network {
	case NetworkEnumForBTCP2TR:
		return vbytesP2TRInput
	case NetworkEnumForDOGE:
		return bytesP2PKHInput
	}
	return vbytesP2WPKHInput
}

func estimateFeeForScripts(network string, byteFee int64, numInputs int, outScripts ...[]byte) int64 {
	vbytes := int64(numInputs)*perInputVBytes(network) + bytesOverhead
	for _, script := range outScripts {
		vbytes += outputVBytes(script)
	}
	return vbytes * byteFee
}

func scriptForAddress(address string, params *chaincfg.Params, allowSegwit bool) ([]byte, error) {
	addr, err := decodeAddressForNet(address, params, allowSegwit)
	if err != nil {
		return nil, err
	}
	return txscript.PayToAddrScript(addr)
}

type (
	Ingredient struct {
		TxType                string            `json:"txType" validate:"required,oneof=0 1 2 3 4 5 6"`
		ContractAddress       string            `json:"contractAddress" validate:"required,native"`
		Sender                string            `json:"sender" validate:"required"`
		SenderPublicKey       string            `json:"senderPublicKey" validate:"required,hex_str"`
		Recipient             string            `json:"recipient" validate:"required"`
		Amount                string            `json:"amount" validate:"required,i64"`
		ByteFee               string            `json:"byteFee" validate:"required,i64"`
		Utxos                 *signing.UtxoList `json:"utxos" validate:"required"`
		IsPSBT                bool              `json:"isPSBT,omitempty"`
		InscriptionIdsForHash map[string]string `json:"inscriptionIdsForHash,omitempty"`
		Memo                  string            `json:"memo,omitempty" validate:"omitempty,hex_str"`
	}
	TxBuilder struct {
		*Ingredient
		network     string
		unsignedHex string
		sigHash     []string
		txHash      string
	}
)

func ToBigEndianHash(hash string) string {
	if len(hash) != 64 {
		return ""
	}
	var hashBigEndian string
	for i := 0; i < 64; {
		hashBigEndian = hashBigEndian + hash[64-i-2:64-i]
		i = i + 2
	}
	return hashBigEndian
}

// by network
func buildForBTC(i *Ingredient) ([]string, string, error) {
	var sigHash []string
	var unsignedHex string
	txVersion := int32(wire.TxVersion)
	sequenceNum := wire.MaxTxInSequenceNum
	params := btcParams
	totalHas := int64(0)
	for _, v := range i.Utxos.List {
		value, _ := strconv.ParseInt(v.Value, 10, 64)
		totalHas += value
	}

	byteFee, _ := strconv.ParseInt(i.ByteFee, 10, 64)
	numInputs := len(i.Utxos.List)
	recipientScript, err := scriptForAddress(i.Recipient, &params, true)
	if err != nil {
		return nil, "", fmt.Errorf("failed to PayToAddrScript for recipient, err=%v", err)
	}
	changeScript, err := scriptForAddress(i.Sender, &params, true)
	if err != nil {
		return nil, "", fmt.Errorf("failed to PayToAddrScript for sender, err=%v", err)
	}
	memoScript, err := buildMemoScript(i.Memo)
	if err != nil {
		return nil, "", err
	}
	memoFee := memoOutputVBytes(memoScript) * byteFee
	var toAddrArr []string
	var toAmountArr []int64
	if i.Amount == signing.MagicNumberForMaxAmount {
		fee := estimateFeeForScripts(NetworkEnumForBTC, byteFee, numInputs, recipientScript) + memoFee
		send := totalHas - fee
		if send < DefaultDust {
			return nil, "", fmt.Errorf("sweep amount below dust: totalHas=%d, fee=%d", totalHas, fee)
		}
		toAddrArr = append(toAddrArr, i.Recipient)
		toAmountArr = append(toAmountArr, send)
	} else {
		value, _ := strconv.ParseInt(i.Amount, 10, 64)
		if value < DefaultDust {
			return nil, "", fmt.Errorf("amount below dust: %d", value)
		}
		toAddrArr = append(toAddrArr, i.Recipient)
		toAmountArr = append(toAmountArr, value)
		feeWithChange := estimateFeeForScripts(NetworkEnumForBTC, byteFee, numInputs, recipientScript, changeScript) + memoFee
		change := totalHas - feeWithChange - value
		if change >= DefaultDust {
			toAddrArr = append(toAddrArr, i.Sender)
			toAmountArr = append(toAmountArr, change)
		} else {
			feeNoChange := estimateFeeForScripts(NetworkEnumForBTC, byteFee, numInputs, recipientScript) + memoFee
			if totalHas-feeNoChange-value < 0 {
				return nil, "", fmt.Errorf("value too large, totalHas=%d, value=%d, fee=%d", totalHas, value, feeNoChange)
			}
		}
	}

	//build msgTx
	msgTx := wire.NewMsgTx(txVersion)
	for _, utxo := range i.Utxos.List {
		utxoHash, err := chainhash.NewHashFromStr(utxo.Hash)
		if err != nil {
			return nil, "", fmt.Errorf("failed to NewHashFromStr, err=%v", err)
		}
		index, _ := strconv.ParseUint(utxo.Index, 10, 64)
		outPoint := wire.NewOutPoint(utxoHash, uint32(index))
		txIn := wire.NewTxIn(outPoint, nil, nil)
		txIn.Sequence = sequenceNum
		msgTx.AddTxIn(txIn)
	}
	for i, toAddr := range toAddrArr {
		toAddress, err := btcaddr.DecodeAddress(toAddr, &params)
		if err != nil {
			return nil, "", fmt.Errorf("failed to DecodeAddress for toAddr, err=%v", err)
		}
		toAddressBytes, err := txscript.PayToAddrScript(toAddress)
		if err != nil {
			return nil, "", fmt.Errorf("failed to PayToAddrScript for toAddress, err=%v", err)
		}
		txOut := wire.NewTxOut(toAmountArr[i], toAddressBytes)
		msgTx.AddTxOut(txOut)
	}

	if memoScript != nil {
		msgTx.AddTxOut(wire.NewTxOut(0, memoScript))
	}
	/*
		{
			if i.Memo != "" {
				memoArr := util.StringSplitByLen(i.Memo, 1)
				for _, v := range memoArr {
					sb := txscript.NewScriptBuilder()
					sb.AddOp(txscript.OP_RETURN)
					sb.AddData([]byte(v))
					sbScript, err := sb.Script()
					if err != nil {
						return nil, "", fmt.Errorf("failed to Script for memo, err=%v", err)
					}
					msgTx.AddTxOut(wire.NewTxOut(0, sbScript))
				}
			}
		}
	*/

	//cal txhash
	senderAddr, err := btcaddr.DecodeAddress(i.Sender, &params)
	if err != nil {
		return nil, "", fmt.Errorf("failed to DecodeAddress for sender, err=%v", err)
	}
	pkData, err := txscript.PayToAddrScript(senderAddr)
	if err != nil {
		return nil, "", fmt.Errorf("failed to PayToAddrScript for senderAddr, err=%v", err)
	}
	for idx, utxo := range i.Utxos.List {
		var pkScript []byte
		if utxo.Script == "" {
			pkScript = pkData
		} else {
			pkScript, _ = hex.DecodeString(utxo.Script)
		}
		var hash []byte
		amount, _ := strconv.ParseInt(utxo.Value, 10, 64)
		txSigHashes := txscript.NewTxSigHashes(msgTx, txscript.NewCannedPrevOutputFetcher(pkScript, amount))
		hash, err = txscript.CalcWitnessSigHash(pkScript, txSigHashes, txscript.SigHashAll, msgTx, idx, amount)
		if err != nil {
			return nil, "", fmt.Errorf("failed to CalcWitnessSigHash, err=%v", err)
		}
		sigHash = append(sigHash, hex.EncodeToString(hash))
	}

	var unsignedBytes bytes.Buffer
	err = msgTx.Serialize(&unsignedBytes)
	if err != nil {
		return nil, "", fmt.Errorf("failed to Serialize for msgTx, err=%v", err)
	}
	unsignedHex = hex.EncodeToString(unsignedBytes.Bytes())

	return sigHash, unsignedHex, nil
}
func buildForBTCP2TR(i *Ingredient) ([]string, string, error) {
	if i.IsPSBT {
		return nil, "", fmt.Errorf("PSBT is not supported for taproot")
	}
	var sigHash []string
	txVersion := int32(wire.TxVersion)
	sequenceNum := wire.MaxTxInSequenceNum
	params := btcParams
	totalHas := int64(0)
	for _, v := range i.Utxos.List {
		value, _ := strconv.ParseInt(v.Value, 10, 64)
		totalHas += value
	}

	senderAddr, err := decodeAddressForNet(i.Sender, &params, true)
	if err != nil {
		return nil, "", fmt.Errorf("failed to DecodeAddress for sender, err=%v", err)
	}
	if _, ok := senderAddr.(*btcaddr.AddressTaproot); !ok {
		return nil, "", fmt.Errorf("sender %s is not a taproot address", i.Sender)
	}
	pkData, err := txscript.PayToAddrScript(senderAddr)
	if err != nil {
		return nil, "", fmt.Errorf("failed to PayToAddrScript for senderAddr, err=%v", err)
	}

	recipientScript, err := scriptForAddress(i.Recipient, &params, true)
	if err != nil {
		return nil, "", fmt.Errorf("failed to PayToAddrScript for recipient, err=%v", err)
	}

	byteFee, _ := strconv.ParseInt(i.ByteFee, 10, 64)
	numInputs := len(i.Utxos.List)
	memoScript, err := buildMemoScript(i.Memo)
	if err != nil {
		return nil, "", err
	}
	memoFee := memoOutputVBytes(memoScript) * byteFee
	var toAddrArr []string
	var toAmountArr []int64
	if i.Amount == signing.MagicNumberForMaxAmount {
		fee := estimateFeeForScripts(NetworkEnumForBTCP2TR, byteFee, numInputs, recipientScript) + memoFee
		send := totalHas - fee
		if send < DefaultDust {
			return nil, "", fmt.Errorf("sweep amount below dust: totalHas=%d, fee=%d", totalHas, fee)
		}
		toAddrArr = append(toAddrArr, i.Recipient)
		toAmountArr = append(toAmountArr, send)
	} else {
		value, _ := strconv.ParseInt(i.Amount, 10, 64)
		if value < DefaultDust {
			return nil, "", fmt.Errorf("amount below dust: %d", value)
		}
		toAddrArr = append(toAddrArr, i.Recipient)
		toAmountArr = append(toAmountArr, value)
		feeWithChange := estimateFeeForScripts(NetworkEnumForBTCP2TR, byteFee, numInputs, recipientScript, pkData) + memoFee
		change := totalHas - feeWithChange - value
		if change >= DefaultDust {
			toAddrArr = append(toAddrArr, i.Sender)
			toAmountArr = append(toAmountArr, change)
		} else {
			feeNoChange := estimateFeeForScripts(NetworkEnumForBTCP2TR, byteFee, numInputs, recipientScript) + memoFee
			if totalHas-feeNoChange-value < 0 {
				return nil, "", fmt.Errorf("value too large, totalHas=%d, value=%d, fee=%d", totalHas, value, feeNoChange)
			}
		}
	}

	msgTx := wire.NewMsgTx(txVersion)
	prevOuts := make(map[wire.OutPoint]*wire.TxOut, numInputs)
	for _, utxo := range i.Utxos.List {
		utxoHash, err := chainhash.NewHashFromStr(utxo.Hash)
		if err != nil {
			return nil, "", fmt.Errorf("failed to NewHashFromStr, err=%v", err)
		}
		index, _ := strconv.ParseUint(utxo.Index, 10, 64)
		outPoint := wire.NewOutPoint(utxoHash, uint32(index))
		txIn := wire.NewTxIn(outPoint, nil, nil)
		txIn.Sequence = sequenceNum
		msgTx.AddTxIn(txIn)

		pkScript := pkData
		if utxo.Script != "" {
			pkScript, err = hex.DecodeString(utxo.Script)
			if err != nil {
				return nil, "", fmt.Errorf("failed to DecodeString for utxo script, err=%v", err)
			}
		}
		if !txscript.IsPayToTaproot(pkScript) {
			return nil, "", fmt.Errorf("utxo %s:%d is not a taproot output", utxo.Hash, index)
		}
		amount, _ := strconv.ParseInt(utxo.Value, 10, 64)
		prevOuts[*outPoint] = wire.NewTxOut(amount, pkScript)
	}
	for idx, toAddr := range toAddrArr {
		toAddress, err := decodeAddressForNet(toAddr, &params, true)
		if err != nil {
			return nil, "", fmt.Errorf("failed to DecodeAddress for toAddr, err=%v", err)
		}
		toAddressBytes, err := txscript.PayToAddrScript(toAddress)
		if err != nil {
			return nil, "", fmt.Errorf("failed to PayToAddrScript for toAddress, err=%v", err)
		}
		msgTx.AddTxOut(wire.NewTxOut(toAmountArr[idx], toAddressBytes))
	}
	if memoScript != nil {
		msgTx.AddTxOut(wire.NewTxOut(0, memoScript))
	}

	fetcher := txscript.NewMultiPrevOutFetcher(prevOuts)
	txSigHashes := txscript.NewTxSigHashes(msgTx, fetcher)
	for idx := range msgTx.TxIn {
		hash, err := txscript.CalcTaprootSignatureHash(txSigHashes, txscript.SigHashDefault, msgTx, idx, fetcher)
		if err != nil {
			return nil, "", fmt.Errorf("failed to CalcTaprootSignatureHash, err=%v", err)
		}
		sigHash = append(sigHash, hex.EncodeToString(hash))
	}

	var unsignedBytes bytes.Buffer
	if err := msgTx.Serialize(&unsignedBytes); err != nil {
		return nil, "", fmt.Errorf("failed to Serialize for msgTx, err=%v", err)
	}
	return sigHash, hex.EncodeToString(unsignedBytes.Bytes()), nil
}

func buildForLTC(i *Ingredient) ([]string, string, error) {
	var sigHash []string
	var unsignedHex string
	txVersion := int32(wire.TxVersion)
	sequenceNum := wire.MaxTxInSequenceNum
	params := ltcParams
	totalHas := int64(0)
	for _, v := range i.Utxos.List {
		value, _ := strconv.ParseInt(v.Value, 10, 64)
		totalHas += value
	}
	byteFee, _ := strconv.ParseInt(i.ByteFee, 10, 64)
	numInputs := len(i.Utxos.List)
	recipientScript, err := scriptForAddress(i.Recipient, &params, true)
	if err != nil {
		return nil, "", fmt.Errorf("failed to PayToAddrScript for recipient, err=%v", err)
	}
	changeScript, err := scriptForAddress(i.Sender, &params, true)
	if err != nil {
		return nil, "", fmt.Errorf("failed to PayToAddrScript for sender, err=%v", err)
	}
	var toAddrArr []string
	var toAmountArr []int64
	if i.Amount == signing.MagicNumberForMaxAmount {
		fee := estimateFeeForScripts(NetworkEnumForLTC, byteFee, numInputs, recipientScript)
		send := totalHas - fee
		if send < DefaultDust {
			return nil, "", fmt.Errorf("sweep amount below dust: totalHas=%d, fee=%d", totalHas, fee)
		}
		toAddrArr = append(toAddrArr, i.Recipient)
		toAmountArr = append(toAmountArr, send)
	} else {
		value, _ := strconv.ParseInt(i.Amount, 10, 64)
		if value < DefaultDust {
			return nil, "", fmt.Errorf("amount below dust: %d", value)
		}
		toAddrArr = append(toAddrArr, i.Recipient)
		toAmountArr = append(toAmountArr, value)
		feeWithChange := estimateFeeForScripts(NetworkEnumForLTC, byteFee, numInputs, recipientScript, changeScript)
		change := totalHas - feeWithChange - value
		if change >= DefaultDust {
			toAddrArr = append(toAddrArr, i.Sender)
			toAmountArr = append(toAmountArr, change)
		} else {
			feeNoChange := estimateFeeForScripts(NetworkEnumForLTC, byteFee, numInputs, recipientScript)
			if totalHas-feeNoChange-value < 0 {
				return nil, "", fmt.Errorf("value too large, totalHas=%d, value=%d, fee=%d", totalHas, value, feeNoChange)
			}
		}
	}

	//build msgTx
	msgTx := wire.NewMsgTx(txVersion)
	for _, utxo := range i.Utxos.List {
		utxoHash, err := chainhash.NewHashFromStr(utxo.Hash)
		if err != nil {
			return nil, "", fmt.Errorf("failed to NewHashFromStr, err=%v", err)
		}
		index, _ := strconv.ParseInt(utxo.Index, 10, 64)
		outPoint := wire.NewOutPoint(utxoHash, uint32(index))
		txIn := wire.NewTxIn(outPoint, nil, nil)
		txIn.Sequence = sequenceNum
		msgTx.AddTxIn(txIn)
	}
	for i, toAddr := range toAddrArr {
		toAddress, err := btcaddr.DecodeAddress(toAddr, &params)
		if err != nil {
			return nil, "", fmt.Errorf("failed to DecodeAddress for toAddr, err=%v", err)
		}
		toAddressBytes, err := txscript.PayToAddrScript(toAddress)
		if err != nil {
			return nil, "", fmt.Errorf("failed to PayToAddrScript for toAddress, err=%v", err)
		}
		txOut := wire.NewTxOut(toAmountArr[i], toAddressBytes)
		msgTx.AddTxOut(txOut)
	}

	//cal txhash
	senderAddr, err := btcaddr.DecodeAddress(i.Sender, &params)
	if err != nil {
		return nil, "", fmt.Errorf("failed to DecodeAddress for sender, err=%v", err)
	}
	pkData, err := txscript.PayToAddrScript(senderAddr)
	if err != nil {
		return nil, "", fmt.Errorf("failed to PayToAddrScript for senderAddr, err=%v", err)
	}

	for idx, utxo := range i.Utxos.List {
		var pkScript []byte
		if utxo.Script == "" {
			pkScript = pkData
		} else {
			pkScript, _ = hex.DecodeString(utxo.Script)
		}

		amount, _ := strconv.ParseInt(utxo.Value, 10, 64)
		txSigHashes := txscript.NewTxSigHashes(msgTx, txscript.NewCannedPrevOutputFetcher(pkScript, amount))
		hash, err := txscript.CalcWitnessSigHash(pkScript, txSigHashes, txscript.SigHashAll, msgTx, idx, amount)
		if err != nil {
			return nil, "", fmt.Errorf("failed to CalcWitnessSigHash, err=%v", err)
		}
		sigHash = append(sigHash, hex.EncodeToString(hash))
	}

	var unsignedBytes bytes.Buffer
	err = msgTx.Serialize(&unsignedBytes)
	if err != nil {
		return nil, "", fmt.Errorf("failed to Serialize for msgTx, err=%v", err)
	}
	unsignedHex = hex.EncodeToString(unsignedBytes.Bytes())

	return sigHash, unsignedHex, nil
}
func buildForDOGE(i *Ingredient) ([]string, string, error) {
	var sigHash []string
	var unsignedHex string
	txVersion := int32(wire.TxVersion)
	sequenceNum := wire.MaxTxInSequenceNum
	params := dogeParams
	totalHas := int64(0)
	for _, v := range i.Utxos.List {
		value, _ := strconv.ParseInt(v.Value, 10, 64)
		totalHas += value
	}

	byteFee, _ := strconv.ParseInt(i.ByteFee, 10, 64)
	numInputs := len(i.Utxos.List)
	const dogeMinFee = 5000000
	memoScript, err := buildMemoScript(i.Memo)
	if err != nil {
		return nil, "", err
	}
	memoFee := memoOutputVBytes(memoScript) * byteFee
	var toAddrArr []string
	var toAmountArr []int64
	if i.Amount == signing.MagicNumberForMaxAmount {
		fee := estimateFee(NetworkEnumForDOGE, byteFee, numInputs, 1) + memoFee
		if fee < dogeMinFee {
			fee = dogeMinFee
		}
		send := totalHas - fee
		if send < DogeDust {
			return nil, "", fmt.Errorf("sweep amount below dust: totalHas=%d, fee=%d", totalHas, fee)
		}
		toAddrArr = append(toAddrArr, i.Recipient)
		toAmountArr = append(toAmountArr, send)
	} else {
		value, _ := strconv.ParseInt(i.Amount, 10, 64)
		if value < DogeDust {
			return nil, "", fmt.Errorf("amount below dust: %d", value)
		}
		feeWithChange := estimateFee(NetworkEnumForDOGE, byteFee, numInputs, 2) + memoFee
		if feeWithChange < dogeMinFee {
			feeWithChange = dogeMinFee
		}
		toAddrArr = append(toAddrArr, i.Recipient)
		toAmountArr = append(toAmountArr, value)
		change := totalHas - feeWithChange - value
		if change >= DogeDust {
			toAddrArr = append(toAddrArr, i.Sender)
			toAmountArr = append(toAmountArr, change)
		} else {
			feeNoChange := estimateFee(NetworkEnumForDOGE, byteFee, numInputs, 1) + memoFee
			if feeNoChange < dogeMinFee {
				feeNoChange = dogeMinFee
			}
			if totalHas-feeNoChange-value < 0 {
				return nil, "", fmt.Errorf("value too large")
			}
		}
	}

	//build msgTx
	msgTx := wire.NewMsgTx(txVersion)
	for _, utxo := range i.Utxos.List {
		utxoHash, err := chainhash.NewHashFromStr(utxo.Hash)
		if err != nil {
			return nil, "", fmt.Errorf("failed to NewHashFromStr, err=%v", err)
		}
		index, _ := strconv.ParseUint(utxo.Index, 10, 64)
		outPoint := wire.NewOutPoint(utxoHash, uint32(index))
		txIn := wire.NewTxIn(outPoint, nil, nil)
		txIn.Sequence = sequenceNum
		msgTx.AddTxIn(txIn)
	}
	for i, toAddr := range toAddrArr {
		toAddress, err := btcaddr.DecodeAddress(toAddr, &params)
		if err != nil {
			return nil, "", fmt.Errorf("failed to DecodeAddress for toAddr, err=%v", err)
		}
		toAddressBytes, err := txscript.PayToAddrScript(toAddress)
		if err != nil {
			return nil, "", fmt.Errorf("failed to PayToAddrScript for toAddress, err=%v", err)
		}
		txOut := wire.NewTxOut(toAmountArr[i], toAddressBytes)
		msgTx.AddTxOut(txOut)
	}

	if memoScript != nil {
		msgTx.AddTxOut(wire.NewTxOut(0, memoScript))
	}
	/*
		{
			if i.Memo != "" {
				memoArr := util.StringSplitByLen(i.Memo, 1)
				for _, v := range memoArr {
					sb := txscript.NewScriptBuilder()
					sb.AddOp(txscript.OP_RETURN)
					sb.AddData([]byte(v))
					sbScript, err := sb.Script()
					if err != nil {
						return nil, "", fmt.Errorf("failed to Script for memo, err=%v", err)
					}
					msgTx.AddTxOut(wire.NewTxOut(0, sbScript))
				}
			}
		}
	*/

	//cal txhash
	senderAddr, err := btcaddr.DecodeAddress(i.Sender, &params)
	if err != nil {
		return nil, "", fmt.Errorf("failed to DecodeAddress for sender, err=%v", err)
	}
	pkData, err := txscript.PayToAddrScript(senderAddr)
	if err != nil {
		return nil, "", fmt.Errorf("failed to PayToAddrScript for senderAddr, err=%v", err)
	}
	for idx, utxo := range i.Utxos.List {
		var pkScript []byte
		if utxo.Script == "" {
			pkScript = pkData
		} else {
			pkScript, _ = hex.DecodeString(utxo.Script)
		}
		var hash []byte
		hash, err = txscript.CalcSignatureHash(pkScript, txscript.SigHashAll, msgTx, idx)
		if err != nil {
			return nil, "", fmt.Errorf("failed to CalcSignatureHash, err=%v", err)
		}
		sigHash = append(sigHash, hex.EncodeToString(hash))
	}
	var unsignedBytes bytes.Buffer
	err = msgTx.Serialize(&unsignedBytes)
	if err != nil {
		return nil, "", fmt.Errorf("failed to Serialize for msgTx, err=%v", err)
	}
	unsignedHex = hex.EncodeToString(unsignedBytes.Bytes())

	return sigHash, unsignedHex, nil
}
func buildForSYS(i *Ingredient) ([]string, string, error) {
	i.IsPSBT = true
	var sigHash []string
	var unsignedHex string
	txVersion := int32(2)
	sequenceNum := wire.MaxTxInSequenceNum - 2
	params := sysParams
	totalHas := int64(0)
	for _, v := range i.Utxos.List {
		value, _ := strconv.ParseInt(v.Value, 10, 64)
		totalHas += value
	}

	byteFee, _ := strconv.ParseInt(i.ByteFee, 10, 64)
	numInputs := len(i.Utxos.List)
	recipientScript, err := scriptForAddress(i.Recipient, &params, true)
	if err != nil {
		return nil, "", fmt.Errorf("failed to PayToAddrScript for recipient, err=%v", err)
	}
	changeScript, err := scriptForAddress(i.Sender, &params, true)
	if err != nil {
		return nil, "", fmt.Errorf("failed to PayToAddrScript for sender, err=%v", err)
	}
	var toAddrArr []string
	var toAmountArr []int64
	if i.Amount == signing.MagicNumberForMaxAmount {
		fee := estimateFeeForScripts(NetworkEnumForSYS, byteFee, numInputs, recipientScript)
		send := totalHas - fee
		if send < DefaultDust {
			return nil, "", fmt.Errorf("sweep amount below dust: totalHas=%d, fee=%d", totalHas, fee)
		}
		toAddrArr = append(toAddrArr, i.Recipient)
		toAmountArr = append(toAmountArr, send)
	} else {
		value, _ := strconv.ParseInt(i.Amount, 10, 64)
		if value < DefaultDust {
			return nil, "", fmt.Errorf("amount below dust: %d", value)
		}
		toAddrArr = append(toAddrArr, i.Recipient)
		toAmountArr = append(toAmountArr, value)
		feeWithChange := estimateFeeForScripts(NetworkEnumForSYS, byteFee, numInputs, recipientScript, changeScript)
		change := totalHas - feeWithChange - value
		if change >= DefaultDust {
			toAddrArr = append(toAddrArr, i.Sender)
			toAmountArr = append(toAmountArr, change)
		} else {
			feeNoChange := estimateFeeForScripts(NetworkEnumForSYS, byteFee, numInputs, recipientScript)
			if totalHas-feeNoChange-value < 0 {
				return nil, "", fmt.Errorf("value too large")
			}
		}
	}

	//build msgTx
	msgTx := wire.NewMsgTx(txVersion)
	for _, utxo := range i.Utxos.List {
		utxoHash, err := chainhash.NewHashFromStr(utxo.Hash)
		if err != nil {
			return nil, "", fmt.Errorf("failed to NewHashFromStr, err=%v", err)
		}
		index, _ := strconv.ParseUint(utxo.Index, 10, 64)
		outPoint := wire.NewOutPoint(utxoHash, uint32(index))
		txIn := wire.NewTxIn(outPoint, nil, nil)
		txIn.Sequence = sequenceNum
		msgTx.AddTxIn(txIn)
	}
	for i, toAddr := range toAddrArr {
		toAddress, err := btcaddr.DecodeAddress(toAddr, &params)
		if err != nil {
			return nil, "", fmt.Errorf("failed to DecodeAddress for toAddr, err=%v", err)
		}
		toAddressBytes, err := txscript.PayToAddrScript(toAddress)
		if err != nil {
			return nil, "", fmt.Errorf("failed to PayToAddrScript for toAddress, err=%v", err)
		}
		txOut := wire.NewTxOut(toAmountArr[i], toAddressBytes)
		msgTx.AddTxOut(txOut)
	}

	//cal txhash
	senderAddr, err := btcaddr.DecodeAddress(i.Sender, &params)
	if err != nil {
		return nil, "", fmt.Errorf("failed to DecodeAddress for sender, err=%v", err)
	}
	pkData, err := txscript.PayToAddrScript(senderAddr)
	if err != nil {
		return nil, "", fmt.Errorf("failed to PayToAddrScript for senderAddr, err=%v", err)
	}
	for idx, utxo := range i.Utxos.List {
		var pkScript []byte
		if utxo.Script == "" {
			pkScript = pkData
		} else {
			pkScript, _ = hex.DecodeString(utxo.Script)
		}
		var hash []byte
		amount, _ := strconv.ParseInt(utxo.Value, 10, 64)
		txSigHashes := txscript.NewTxSigHashes(msgTx, txscript.NewCannedPrevOutputFetcher(pkScript, amount))
		hash, err = txscript.CalcWitnessSigHash(pkScript, txSigHashes, txscript.SigHashAll, msgTx, idx, amount)
		if err != nil {
			return nil, "", fmt.Errorf("failed to CalcWitnessSigHash, err=%v", err)
		}
		sigHash = append(sigHash, hex.EncodeToString(hash))
	}
	packet, err := psbt.NewFromUnsignedTx(msgTx)
	if err != nil {
		return nil, "", fmt.Errorf("failed to NewFromUnsignedTx, err=%v", err)
	}
	for idx, utxo := range i.Utxos.List {
		amount, _ := strconv.ParseInt(utxo.Value, 10, 64)
		packet.Inputs[idx].WitnessUtxo = wire.NewTxOut(amount, pkData)
		if err := packet.SanityCheck(); err != nil {
			return nil, "", fmt.Errorf("failed to SanityCheck, err=%v", err)
		}
	}
	var unsignedBytes bytes.Buffer
	err = packet.Serialize(&unsignedBytes)
	if err != nil {
		return nil, "", fmt.Errorf("failed to Serialize for packet, err=%v", err)
	}
	unsignedHex = hex.EncodeToString(unsignedBytes.Bytes())

	return sigHash, unsignedHex, nil
}

func extractSigHashes(packet *psbt.Packet) ([]string, error) {
	var sigHashes []string

	for i, input := range packet.Inputs {
		switch {
		case input.WitnessUtxo != nil:
			prevOuts := txscript.NewCannedPrevOutputFetcher(
				input.WitnessUtxo.PkScript,
				input.WitnessUtxo.Value,
			)
			sigHash := txscript.NewTxSigHashes(packet.UnsignedTx, prevOuts)

			hash, err := txscript.CalcWitnessSigHash(
				input.WitnessUtxo.PkScript,
				sigHash,
				txscript.SigHashAll,
				packet.UnsignedTx,
				i,
				input.WitnessUtxo.Value,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to CalcWitnessSigHash for input %d, err=%v", i, err)
			}
			sigHashes = append(sigHashes, hex.EncodeToString(hash))

		case input.NonWitnessUtxo != nil:
			utxo := input.NonWitnessUtxo
			if i >= len(packet.UnsignedTx.TxIn) {
				return nil, fmt.Errorf("input %d out of range for unsigned tx", i)
			}
			txIn := packet.UnsignedTx.TxIn[i]
			if int(txIn.PreviousOutPoint.Index) >= len(utxo.TxOut) {
				return nil, fmt.Errorf("input %d prevout index %d out of range", i, txIn.PreviousOutPoint.Index)
			}
			pkScript := utxo.TxOut[txIn.PreviousOutPoint.Index].PkScript

			hash, err := txscript.CalcSignatureHash(
				pkScript,
				txscript.SigHashAll,
				packet.UnsignedTx,
				i,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to CalcSignatureHash for input %d, err=%v", i, err)
			}
			sigHashes = append(sigHashes, hex.EncodeToString(hash))

		default:
			return nil, fmt.Errorf("input %d missing both WitnessUtxo and NonWitnessUtxo", i)
		}
	}

	return sigHashes, nil
}
