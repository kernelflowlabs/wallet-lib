package sui

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/address/v2/base58"
)

type suiObjectRef struct {
	objectID string
	version  uint64
	digest   string
}

type suiEnc struct {
	buf []byte
	err error
}

func (e *suiEnc) u8(v byte) {
	e.buf = append(e.buf, v)
}

func (e *suiEnc) u16(v uint16) {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], v)
	e.buf = append(e.buf, b[:]...)
}

func (e *suiEnc) u64(v uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	e.buf = append(e.buf, b[:]...)
}

func (e *suiEnc) uleb(v uint64) {
	for v >= 0x80 {
		e.buf = append(e.buf, byte(v)|0x80)
		v >>= 7
	}
	e.buf = append(e.buf, byte(v))
}

func (e *suiEnc) lenPrefixed(b []byte) {
	e.uleb(uint64(len(b)))
	e.buf = append(e.buf, b...)
}

func (e *suiEnc) address(addr string) {
	if e.err != nil {
		return
	}
	s := strings.TrimPrefix(addr, "0x")
	s = strings.TrimPrefix(s, "0X")
	if len(s)%2 != 0 {
		s = "0" + s
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		e.err = err
		return
	}
	if len(b) > 32 {
		e.err = fmt.Errorf("invalid address length")
		return
	}
	out := make([]byte, 32)
	copy(out[32-len(b):], b)
	e.buf = append(e.buf, out...)
}

func (e *suiEnc) argGasCoin()        { e.u8(0) }
func (e *suiEnc) argInput(i uint16)  { e.u8(1); e.u16(i) }
func (e *suiEnc) argResult(i uint16) { e.u8(2); e.u16(i) }

func (e *suiEnc) pureU64(v uint64) {
	e.u8(0)
	e.uleb(8)
	e.u64(v)
}

func (e *suiEnc) pureAddress(addr string) {
	e.u8(0)
	e.uleb(32)
	e.address(addr)
}

func (e *suiEnc) objectInput(ref suiObjectRef) {
	e.u8(1)
	e.u8(0)
	e.address(ref.objectID)
	e.u64(ref.version)
	e.lenPrefixed(base58.Decode(ref.digest))
}

func (e *suiEnc) paymentRef(ref suiObjectRef) {
	e.address(ref.objectID)
	e.u64(ref.version)
	e.lenPrefixed(base58.Decode(ref.digest))
}

func buildSuiTransfer(sender, recipient string, amount, gasPrice, gasBudget uint64, payment, tokens []suiObjectRef) ([]byte, error) {
	e := &suiEnc{}
	e.u8(0)
	e.u8(0)

	switch n := len(tokens); {
	case n == 0:
		e.uleb(2)
		e.pureU64(amount)
		e.pureAddress(recipient)
		e.uleb(2)
		e.u8(2)
		e.argGasCoin()
		e.uleb(1)
		e.argInput(0)
		e.u8(1)
		e.uleb(1)
		e.argResult(0)
		e.argInput(1)
	case n == 1:
		e.uleb(3)
		e.objectInput(tokens[0])
		e.pureU64(amount)
		e.pureAddress(recipient)
		e.uleb(2)
		e.u8(2)
		e.argInput(0)
		e.uleb(1)
		e.argInput(1)
		e.u8(1)
		e.uleb(1)
		e.argResult(0)
		e.argInput(2)
	default:
		e.uleb(uint64(n + 2))
		for _, t := range tokens {
			e.objectInput(t)
		}
		e.pureU64(amount)
		e.pureAddress(recipient)
		e.uleb(3)
		e.u8(3)
		e.argInput(uint16(n - 1))
		e.uleb(uint64(n - 1))
		for i := 0; i < n-1; i++ {
			e.argInput(uint16(i))
		}
		e.u8(2)
		e.argInput(uint16(n - 1))
		e.uleb(1)
		e.argInput(uint16(n))
		e.u8(1)
		e.uleb(1)
		e.argResult(1)
		e.argInput(uint16(n + 1))
	}

	e.address(sender)
	e.uleb(uint64(len(payment)))
	for _, p := range payment {
		e.paymentRef(p)
	}
	e.address(sender)
	e.u64(gasPrice)
	e.u64(gasBudget)
	e.u8(0)

	if e.err != nil {
		return nil, e.err
	}
	return e.buf, nil
}
