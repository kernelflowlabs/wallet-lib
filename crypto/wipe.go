package crypto

import "github.com/kernelflowlabs/wallet-lib/crypto/bip"

func Wipe(b []byte) {
	bip.Wipe(b)
}
