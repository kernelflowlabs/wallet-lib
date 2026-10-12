package cosmos

import (
	"fmt"

	btcaddr "github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/address/v2/bech32"
)

const (
	NetworkEnumForCosmosHub = "cosmoshub-4"
	NetworkEnumForTerra     = "phoenix-1"
	NetworkEnumForTerrac    = "columbus-5"
	NetworkEnumForSei       = "pacific-1"
	NetworkEnumForCelestia  = "arabica-10"
	NetworkEnumForInjective = ""
)

func getParamsByNetwork(network string) (prefix, symbol, denom string) {
	switch network {
	case NetworkEnumForCosmosHub:
		return "cosmos", "ATOM", "uatom"
	case NetworkEnumForTerra:
		return "terra", "LUNA", "uluna"
	case NetworkEnumForTerrac:
		return "terra", "LUNC", "uluna"
	case NetworkEnumForSei:
		return "sei", "SEI", "usei"
	case NetworkEnumForCelestia:
		return "celestia", "TIA", "utia"
	case NetworkEnumForInjective:
		return "inj", "INJ", "uinj"
	}
	return "", "", ""
}

func PublicKey2Address(publicKey []byte, network string) (string, error) {
	prefix, _, _ := getParamsByNetwork(network)
	if prefix == "" {
		return "", fmt.Errorf("unsupported network")
	}
	hash160 := btcaddr.Hash160(publicKey)
	address, err := bech32.EncodeFromBase256(prefix, hash160)
	if err != nil {
		return "", err
	}
	return address, nil
}

func ValidAddress(address, network string) bool {
	prefix, _, _ := getParamsByNetwork(network)
	if prefix == "" {
		return false
	}
	hrp, _, err := bech32.DecodeToBase256(address)
	return err == nil && hrp == prefix
}

func Denom(network string) string {
	_, _, denom := getParamsByNetwork(network)
	return denom
}
