package utxo

import (
	"encoding/hex"
	"fmt"

	"github.com/kernelflowlabs/wallet-lib/signing"

	btcaddr "github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/txscript/v2"
)

func PublicKey2Address(publicKey []byte, network string) string {
	var (
		params  chaincfg.Params
		address string
	)

	switch network {
	case NetworkEnumForBTC, NetworkEnumForBTCP2TR:
		params = btcParams
		program := btcaddr.Hash160(publicKey)
		if network == NetworkEnumForBTCP2TR {
			btcPub, err := schnorr.ParsePubKey(publicKey[1:])
			if err != nil {
				return ""
			}
			addr, err := btcaddr.NewAddressTaproot(
				schnorr.SerializePubKey(txscript.ComputeTaprootKeyNoScript(btcPub)),
				&params,
			)
			if err != nil {
				return ""
			}
			address = addr.String()
		} else {
			addr, err := btcaddr.NewAddressWitnessPubKeyHash(program, &params)
			if err != nil {
				return ""
			}
			address = addr.String()
		}

	case NetworkEnumForLTC:
		params = ltcParams
		program := btcaddr.Hash160(publicKey)
		addr, err := btcaddr.NewAddressWitnessPubKeyHash(program, &params)
		if err != nil {
			return ""
		}
		address = addr.String()

	case NetworkEnumForDOGE:
		params = dogeParams
		program := btcaddr.Hash160(publicKey)
		addr, err := btcaddr.NewAddressPubKeyHash(program, &params)
		if err != nil {
			return ""
		}
		address = addr.String()

	case NetworkEnumForSYS:
		params = sysParams
		program := btcaddr.Hash160(publicKey)
		addr, err := btcaddr.NewAddressWitnessPubKeyHash(program, &params)
		if err != nil {
			return ""
		}
		address = addr.String()

	default:
		return ""
	}

	return address
}

func ValidAddress(address, network string) bool {
	switch network {
	case NetworkEnumForBTC, NetworkEnumForBTCP2TR:
		return ValidAddressForBTC(address)
	case NetworkEnumForLTC:
		return ValidAddressForLTC(address)
	case NetworkEnumForDOGE:
		return ValidAddressForDOGE(address)
	case NetworkEnumForSYS:
		return ValidAddressForSYS(address)
	}
	return false
}

func decodeAddressForNet(address string, params *chaincfg.Params, allowSegwit bool) (btcaddr.Address, error) {
	addr, err := btcaddr.DecodeAddress(address, params)
	if err != nil {
		return nil, err
	}
	if !addr.IsForNet(params) {
		return nil, fmt.Errorf("address %s does not belong to this network", address)
	}
	switch addr.(type) {
	case *btcaddr.AddressPubKeyHash, *btcaddr.AddressScriptHash:
		return addr, nil
	case *btcaddr.AddressWitnessPubKeyHash, *btcaddr.AddressWitnessScriptHash, *btcaddr.AddressTaproot:
		if allowSegwit {
			return addr, nil
		}
	}
	return nil, fmt.Errorf("unsupported address type %T for this network", addr)
}

func validAddressForNet(address string, params *chaincfg.Params, allowSegwit bool) bool {
	if address == signing.MagicContactAddressForNative {
		return true
	}
	_, err := decodeAddressForNet(address, params, allowSegwit)
	return err == nil
}

func ValidAddressForBTC(address string) bool {
	return validAddressForNet(address, &btcParams, true)
}

func ValidAddressForLTC(address string) bool {
	return validAddressForNet(address, &ltcParams, true)
}

func ValidAddressForDOGE(address string) bool {
	return validAddressForNet(address, &dogeParams, false)
}

func ValidAddressForSYS(address string) bool {
	return validAddressForNet(address, &sysParams, true)
}

func AddressToScriptPubKey(address string, network string) (string, error) {
	var params *chaincfg.Params
	allowSegwit := true

	switch network {
	case NetworkEnumForBTC, NetworkEnumForBTCP2TR:
		params = &btcParams
	case NetworkEnumForLTC:
		params = &ltcParams
	case NetworkEnumForDOGE:
		params = &dogeParams
		allowSegwit = false
	default:
		return "", fmt.Errorf("unsupported network")
	}

	addr, err := decodeAddressForNet(address, params, allowSegwit)
	if err != nil {
		return "", fmt.Errorf("invalid address: %v", err)
	}

	script, err := txscript.PayToAddrScript(addr)
	if err != nil {
		return "", fmt.Errorf("failed to create script: %v", err)
	}

	return hex.EncodeToString(script), nil
}

func PrivateKeyHexToWIF(privateKey string) (string, error) {
	privateKeyBytes, err := hex.DecodeString(privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to decode hex: %w", err)
	}
	priv, _ := btcec.PrivKeyFromBytes(privateKeyBytes)

	privateKeyWif, err := btcutil.NewWIF(priv, &chaincfg.MainNetParams, true)
	if err != nil {
		return "", fmt.Errorf("failed to create WIF: %w", err)
	}
	return privateKeyWif.String(), nil
}

func PrivateKeyWIFToHex(privateKey string) (string, error) {
	wif, err := btcutil.DecodeWIF(privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to decode WIF: %w", err)
	}
	privateKeyBytes := wif.PrivKey.Serialize()
	return hex.EncodeToString(privateKeyBytes), nil
}

const (
	NetworkEnumForBTC     string = "0"
	NetworkEnumForLTC     string = "1"
	NetworkEnumForDOGE    string = "2"
	NetworkEnumForBTCP2TR string = "3"
	NetworkEnumForSYS     string = "4"
)

func init() {
	if err := signing.RegisterAddressValidator("btc_addr", ValidAddressForBTC); err != nil {
		panic(err)
	}
	if err := signing.RegisterAddressValidator("ltc_addr", ValidAddressForLTC); err != nil {
		panic(err)
	}
	if err := signing.RegisterAddressValidator("doge_addr", ValidAddressForDOGE); err != nil {
		panic(err)
	}
	if err := signing.RegisterAddressValidator("sys_addr", ValidAddressForSYS); err != nil {
		panic(err)
	}
}
