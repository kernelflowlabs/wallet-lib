package solana

import (
	"encoding/binary"
	"fmt"
	"math/bits"

	"github.com/blocto/solana-go-sdk/types"
)

// MaxTransactionBytes is Solana's packet limit: 1280 bytes of UDP payload less
// 48 bytes of IPv6 and fragment headers. Nothing larger can ever land on
// chain, so nothing larger is worth parsing.
const MaxTransactionBytes = 1232

// This file walks the wire format and checks every declared count against the
// bytes actually left, before any of it reaches the SDK's deserializer.
//
// It is not belt-and-braces. The upstream parser allocates and slices from
// counts it reads out of the input without checking them:
//
//	make([]CompiledInstruction, 0, instructionCount)
//	make([]int, 0, accountCount)
//	data, messageData = messageData[:dataLen], messageData[dataLen:]
//	writableAccountIdxList, messageData = messageData[:count], messageData[count:]
//
// A few crafted bytes make it ask for gigabytes, and the resulting "out of
// memory" is a fatal error that recover() cannot catch; the unchecked slices
// panic outright. One malformed transaction from a dApp would take the wallet
// down with it.
//
// The walk below therefore mirrors MessageDeserialize field for field and —
// this is the part an earlier version got wrong — reads every field with the
// same encoding it does. The upstream parser uses an LEB128 uvarint for the
// three header bytes, the program id index and every instruction account
// index, not one raw byte each. Reading those as single bytes made the two
// walks diverge the moment any of them had its high bit set, and from there
// the lengths being checked were no longer the lengths being used.
func uvarint(buf []byte, at int, what string) (uint64, int, error) {
	if at >= len(buf) {
		return 0, at, fmt.Errorf("%s: truncated", what)
	}
	value, read := binary.Uvarint(buf[at:])
	if read <= 0 {
		return 0, at, fmt.Errorf("%s: malformed varint", what)
	}
	return value, at + read, nil
}

// ValidateTransactionEnvelope checks a serialized transaction before the SDK
// parses it.
func ValidateTransactionEnvelope(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("transaction is empty")
	}
	if data[0] == 0x81 {
		return validateTransactionV1(data)
	}
	if len(data) > MaxTransactionBytes {
		return fmt.Errorf("transaction is %d bytes, the chain accepts at most %d", len(data), MaxTransactionBytes)
	}

	signatures, at, err := uvarint(data, 0, "signature count")
	if err != nil {
		return err
	}
	if signatures < 1 {
		return fmt.Errorf("transaction carries no signature slot")
	}
	// Bounded before it reaches make([]Signature, 0, signatures).
	if signatures > uint64(len(data)-at)/64 {
		return fmt.Errorf("signature count %d does not fit in %d remaining bytes", signatures, len(data)-at)
	}
	at += int(signatures) * 64

	end, header, err := validateMessageAt(data, at)
	if err != nil {
		return err
	}
	if end != len(data) {
		return fmt.Errorf("transaction has %d trailing bytes after its message", len(data)-end)
	}
	if uint64(header) != signatures {
		return fmt.Errorf("transaction declares %d signatures but its message requires %d", signatures, header)
	}
	return nil
}

func validateTransactionV1(data []byte) error {
	if len(data) > 4096 {
		return fmt.Errorf("v1 transaction exceeds 4096 bytes")
	}
	if len(data) < 42 {
		return fmt.Errorf("v1 transaction header is truncated")
	}
	signatures := int(data[1])
	if int(data[2]) >= signatures || signatures > 12 {
		return fmt.Errorf("invalid v1 signature counts")
	}
	mask := binary.LittleEndian.Uint32(data[4:8])
	if mask&3 != 0 && mask&3 != 3 {
		return fmt.Errorf("v1 config bits 0 and 1 must match")
	}
	instructions, addresses := int(data[40]), int(data[41])
	if instructions > 64 {
		return fmt.Errorf("v1 instruction count exceeds 64")
	}
	if addresses < signatures+int(data[3]) || addresses > 96 {
		return fmt.Errorf("invalid v1 address count")
	}
	at := 42
	if addresses > (len(data)-at)/32 {
		return fmt.Errorf("v1 addresses are truncated")
	}
	at += addresses * 32
	configBytes := bits.OnesCount32(mask) * 4
	if configBytes > len(data)-at {
		return fmt.Errorf("v1 config values are truncated")
	}
	if mask&(1<<4) != 0 {
		heapAt := at + bits.OnesCount32(mask&15)*4
		heap := binary.LittleEndian.Uint32(data[heapAt : heapAt+4])
		if heap < 32768 || heap > 262144 || heap%1024 != 0 {
			return fmt.Errorf("invalid v1 heap value")
		}
	}
	at += configBytes
	if instructions > (len(data)-at)/4 {
		return fmt.Errorf("v1 instruction headers are truncated")
	}
	headers := data[at : at+instructions*4]
	at += instructions * 4
	for i := 0; i < instructions; i++ {
		header := headers[i*4 : i*4+4]
		if int(header[0]) >= addresses {
			return fmt.Errorf("invalid v1 program account index")
		}
		accounts := int(header[1])
		dataBytes := int(binary.LittleEndian.Uint16(header[2:4]))
		if accounts > len(data)-at {
			return fmt.Errorf("v1 instruction accounts are truncated")
		}
		for _, index := range data[at : at+accounts] {
			if int(index) >= addresses {
				return fmt.Errorf("invalid v1 instruction account index")
			}
		}
		at += accounts
		if dataBytes > len(data)-at {
			return fmt.Errorf("v1 instruction data is truncated")
		}
		at += dataBytes
	}
	if signatures > (len(data)-at)/64 {
		return fmt.Errorf("v1 signatures are truncated")
	}
	at += signatures * 64
	if at != len(data) {
		return fmt.Errorf("v1 transaction has trailing bytes")
	}
	return nil
}

// ValidateMessageEnvelope is the same walk for a bare message, which is what
// reaches us when a dApp asks for a "message" signature.
func ValidateMessageEnvelope(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("message is empty")
	}
	if len(data) > MaxTransactionBytes {
		return fmt.Errorf("message is longer than any Solana message can be")
	}
	_, _, err := validateMessageAt(data, 0)
	return err
}

// validateMessageAt returns the offset just past the message and the number of
// required signatures it declares.
func validateMessageAt(data []byte, at int) (int, uint8, error) {
	if at >= len(data) {
		return at, 0, fmt.Errorf("truncated before the message body")
	}

	versioned := data[at] > 127
	if versioned {
		if version := data[at] & 0x7f; version != 0 {
			return at, 0, fmt.Errorf("message version %d is not supported", version)
		}
		at++
	}

	// Three header values, each an LEB128 uvarint upstream, each capped at 255.
	var header [3]uint8
	for i := range header {
		value, next, err := uvarint(data, at, "message header")
		if err != nil {
			return at, 0, err
		}
		if value > 255 {
			return at, 0, fmt.Errorf("message header #%d is %d, which does not fit a byte", i+1, value)
		}
		header[i] = uint8(value)
		at = next
	}

	accounts, at, err := uvarint(data, at, "account count")
	if err != nil {
		return at, 0, err
	}
	if accounts > uint64(len(data)-at)/32 {
		return at, 0, fmt.Errorf("account count %d does not fit in %d remaining bytes", accounts, len(data)-at)
	}
	at += int(accounts) * 32

	if len(data)-at < 32 {
		return at, 0, fmt.Errorf("recent blockhash needs 32 bytes, %d remain", len(data)-at)
	}
	at += 32

	instructions, at, err := uvarint(data, at, "instruction count")
	if err != nil {
		return at, 0, err
	}
	// Every instruction costs at least three bytes on the wire, so a count
	// larger than a third of what is left cannot be honest. Checked before it
	// reaches make([]CompiledInstruction, 0, instructions).
	if instructions > uint64(len(data)-at)/3 {
		return at, 0, fmt.Errorf("instruction count %d does not fit in %d remaining bytes", instructions, len(data)-at)
	}
	for i := uint64(0); i < instructions; i++ {
		if _, at, err = uvarint(data, at, "program id index"); err != nil {
			return at, 0, err
		}
		var accountCount uint64
		if accountCount, at, err = uvarint(data, at, "instruction account count"); err != nil {
			return at, 0, err
		}
		// Each index is at least one byte, and this count also feeds
		// make([]int, 0, accountCount).
		if accountCount > uint64(len(data)-at) {
			return at, 0, fmt.Errorf("instruction account count %d does not fit in %d remaining bytes", accountCount, len(data)-at)
		}
		for j := uint64(0); j < accountCount; j++ {
			if _, at, err = uvarint(data, at, "instruction account index"); err != nil {
				return at, 0, err
			}
		}
		var dataLen uint64
		if dataLen, at, err = uvarint(data, at, "instruction data length"); err != nil {
			return at, 0, err
		}
		if dataLen > uint64(len(data)-at) {
			return at, 0, fmt.Errorf("instruction data needs %d bytes, %d remain", dataLen, len(data)-at)
		}
		at += int(dataLen)
	}

	if !versioned {
		return at, header[0], nil
	}

	tables, at, err := uvarint(data, at, "lookup table count")
	if err != nil {
		return at, 0, err
	}
	// A table costs at least 34 bytes: a 32 byte key and two counts.
	if tables > uint64(len(data)-at)/34 {
		return at, 0, fmt.Errorf("lookup table count %d does not fit in %d remaining bytes", tables, len(data)-at)
	}
	for i := uint64(0); i < tables; i++ {
		if len(data)-at < 32 {
			return at, 0, fmt.Errorf("lookup table key needs 32 bytes, %d remain", len(data)-at)
		}
		at += 32
		for _, what := range []string{"writable index count", "readonly index count"} {
			var count uint64
			if count, at, err = uvarint(data, at, what); err != nil {
				return at, 0, err
			}
			if count > uint64(len(data)-at) {
				return at, 0, fmt.Errorf("%s %d does not fit in %d remaining bytes", what, count, len(data)-at)
			}
			at += int(count)
		}
	}
	return at, header[0], nil
}

// SafeTransactionDeserialize validates the wire format first, then parses.
// The recover is a backstop for anything the walk above does not anticipate.
func SafeTransactionDeserialize(data []byte) (tx types.Transaction, err error) {
	if err = ValidateTransactionEnvelope(data); err != nil {
		return types.Transaction{}, err
	}
	if data[0] == 0x81 {
		return types.Transaction{}, fmt.Errorf("v1 transactions cannot be parsed")
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("transaction cannot be parsed: %v", r)
		}
	}()
	tx, err = types.TransactionDeserialize(data)
	if err != nil {
		return types.Transaction{}, fmt.Errorf("transaction cannot be deserialized: %w", err)
	}
	return tx, nil
}

func SafeMessageDeserialize(data []byte) (message types.Message, err error) {
	if err = ValidateMessageEnvelope(data); err != nil {
		return types.Message{}, err
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("message cannot be parsed: %v", r)
		}
	}()
	message, err = types.MessageDeserialize(data)
	if err != nil {
		return types.Message{}, fmt.Errorf("message cannot be deserialized: %w", err)
	}
	return message, nil
}
