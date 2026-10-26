package evm

import (
	"fmt"

	"github.com/kernelflowlabs/wallet-lib/signing"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func PublicKey2Address(publicKey []byte) (string, error) {
	pubKey, err := btcec.ParsePubKey(publicKey)
	if err != nil {
		return "", fmt.Errorf("decode public key: %w", err)
	}
	ecdsaPub, err := crypto.UnmarshalPubkey(pubKey.SerializeUncompressed())
	if err != nil {
		return "", fmt.Errorf("decode public key: %w", err)
	}
	address := crypto.PubkeyToAddress(*ecdsaPub).Hex()
	return address, nil
}

func ValidAddress(address string) bool {
	return common.IsHexAddress(address)
}

func NormalizeAddress(address string) string {
	return common.HexToAddress(address).Hex()
}

func IsZeroAddress(address string) bool {
	return common.HexToAddress(address) == common.Address{}
}

func RecoverAddress(data, signature []byte) (string, error) {
	if len(signature) != 65 {
		return "", fmt.Errorf("invalid signature length: %d", len(signature))
	}

	if signature[64] >= 27 {
		sig := make([]byte, 65)
		copy(sig, signature)
		sig[64] -= 27
		signature = sig
	}
	pubKey, err := crypto.SigToPub(data, signature)
	if err != nil {
		return "", fmt.Errorf("recover public key: %w", err)
	}

	address := crypto.PubkeyToAddress(*pubKey)
	return address.Hex(), nil
}

const (
	NetworkEnumForETH      string = "1"
	NetworkEnumForBNB      string = "56"
	NetworkEnumForHT       string = "128"
	NetworkEnumForOKC      string = "66"
	NetworkEnumForAVAXC    string = "43114"
	NetworkEnumForFTM      string = "250"
	NetworkEnumForETC      string = "61"
	NetworkEnumForETHW     string = "10001"
	NetworkEnumForETHF     string = "513100"
	NetworkEnumForFUSE     string = "122"
	NetworkEnumForPOLYGON  string = "137"
	NetworkEnumForAURORA   string = "1313161554"
	NetworkEnumForCAD      string = "256256"
	NetworkEnumForMETIS    string = "1088"
	NetworkEnumForCIC      string = "1353"
	NetworkEnumForRSK      string = "30"
	NetworkEnumForTARA     string = "841"
	NetworkEnumForKLAY     string = "8217"
	NetworkEnumForCRO      string = "25"
	NetworkEnumForCANTO    string = "7700"
	NetworkEnumForCORE     string = "1116"
	NetworkEnumForELA      string = "20"
	NetworkEnumForCFXEVM   string = "1030"
	NetworkEnumForSYSEVM   string = "57"
	NetworkEnumForFEVM     string = "314"
	NetworkEnumForTELOSEVM string = "40"
	NetworkEnumForARB      string = "42161"
	NetworkEnumForOP       string = "10"
	NetworkEnumForBASE     string = "8453"
	NetworkEnumForLINEA    string = "59144"
	NetworkEnumForZKEVM    string = "1101"
	NetworkEnumForZKSYNC   string = "324"
	NetworkEnumForOPBNB    string = "204"
	NetworkEnumForROLLUX   string = "570"
	NetworkEnumForSCROLL   string = "534352"
	NetworkEnumForSHIBA    string = "109"
	NetworkEnumForMERLIN   string = "4200"
	NetworkEnumForBLAST    string = "81457"
	NetworkEnumForXLAYER   string = "196"
	NetworkEnumForB2       string = "223"
	NetworkEnumForBTR      string = "200901"
	NetworkEnumForMANTA    string = "169"
	NetworkEnumForSKL      string = "2046399126"
	NetworkEnumForODYSSEY  string = "153153"
)

func init() {
	if err := signing.RegisterAddressValidator("evm_addr", ValidAddress); err != nil {
		panic(err)
	}
}
