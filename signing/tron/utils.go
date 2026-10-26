package tron

import (
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"strings"

	"github.com/kernelflowlabs/wallet-lib/signing"

	"github.com/btcsuite/btcd/address/v2/base58"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/ethereum/go-ethereum/crypto"
)

func PublicKey2Address(publicKey []byte) string {
	pubKey, err := btcec.ParsePubKey(publicKey)
	if err != nil {
		return ""
	}
	ecdsaPub, err := crypto.UnmarshalPubkey(pubKey.SerializeUncompressed())
	if err != nil {
		return ""
	}
	addressHex := "41" + strings.ToLower(crypto.PubkeyToAddress(*ecdsaPub).String()[2:])
	bytes, err := hex.DecodeString(addressHex)
	if err != nil {
		return ""
	}
	return base58.CheckEncode(bytes[1:], bytes[0])
}

func ValidAddress(address string) bool {
	if address == signing.MagicContactAddressForNative {
		return true
	}
	if len(address) != 34 || address[0] != 'T' {
		return false
	}
	result, version, err := base58.CheckDecode(address)
	if err != nil {
		return false
	}
	return version == 0x41 && len(result) == 20
}

func ConvertToBytes(address string) []byte {
	result, version, err := base58.CheckDecode(address)
	if err != nil {
		return nil
	}
	return append([]byte{version}, result...)
}

func ConvertToHex(address string) string {
	payload := ConvertToBytes(address)
	if payload == nil {
		return ""
	}
	if payload[0] == 0 {
		return new(big.Int).SetBytes(payload).String()
	}
	h := hex.EncodeToString(payload)
	if h == "" {
		h = "0"
	}
	return h
}

const (
	EnergyFeeSun            = 100
	ProtocolMaxFeeLimitSun  = 15_000_000_000
	MaxFeeLimitSun          = 300_000_000
	DefaultFeeLimitHeadroom = 2
)

func FeeLimitFromSun(sun, headroom int64) (int64, bool) {
	if sun <= 0 || headroom <= 0 || headroom > 100 {
		return 0, false
	}
	if sun > MaxFeeLimitSun/headroom {
		return MaxFeeLimitSun, true
	}
	return sun * headroom, true
}

func FeeLimitForEnergy(energy, headroom int64) (int64, bool) {
	return FeeLimitForEnergyAt(energy, EnergyFeeSun, headroom)
}

func FeeLimitForEnergyAt(energy, energyFeeSun, headroom int64) (int64, bool) {
	if energy <= 0 || energyFeeSun <= 0 || energy > ProtocolMaxFeeLimitSun/energyFeeSun {
		return 0, false
	}
	return FeeLimitFromSun(energy*energyFeeSun, headroom)
}

func normalizeAddressHex(address string) string {
	if len(address) == 34 && ValidAddress(address) {
		return ConvertToHex(address)
	}
	return address
}

func ConvertFromHex(hexAddress string) string {
	b, err := hex.DecodeString(hexAddress)
	if err != nil || len(b) == 0 {
		return ""
	}
	return base58.CheckEncode(b[1:], b[0])
}

func init() {
	if err := signing.RegisterAddressValidator("trx_addr", ValidAddress); err != nil {
		panic(err)
	}
}

func Int64ToBytes(n int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(n))
	return b
}
