package substrate

import (
	"github.com/btcsuite/btcd/address/v2/base58"
	"golang.org/x/crypto/blake2b"
)

const (
	ss58Prefix = "SS58PRE"
)

func convert2PublicKey(address string) []byte {
	addrBytes := base58.Decode(address)
	if len(addrBytes) != 35 {
		return nil
	}
	addrBytes = addrBytes[1:]
	addrBytes = addrBytes[:len(addrBytes)-2]
	return addrBytes
}

func ss58Checksum(data []byte) ([]byte, error) {
	hasher, err := blake2b.New(64, nil)
	if err != nil {
		return nil, err
	}

	_, err = hasher.Write([]byte(ss58Prefix))
	if err != nil {
		return nil, err
	}

	_, err = hasher.Write(data)
	if err != nil {
		return nil, err
	}

	return hasher.Sum(nil), nil
}

func toBase58(buf, accountID []byte, network uint8) (string, error) {
	cs, err := ss58Checksum(buf)
	if err != nil {
		return "", err
	}

	fb := append([]byte{network}, accountID...)
	fb = append(fb, cs[0:2]...)
	return base58.Encode(fb), nil
}
