package tron

import (
	"fmt"

	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/kernelflowlabs/wallet-lib/session"
)

// SessionChain is this chain's name in a signing session.
const SessionChain = "tron"

const DerivationPathPrefix = "m/44'/195'/0'"

func DerivationPath(accountIndex int) string {
	return fmt.Sprintf("%s/0/%d", DerivationPathPrefix, accountIndex)
}

func init() {
	err := session.RegisterChain(session.Chain{
		Name:   SessionChain,
		Path:   DerivationPath,
		Derive: key.DerivePrivateKeyECDSA,
		Address: func(privateKey []byte) (string, error) {
			account, err := NewAccountFromPrivateKey(privateKey)
			if err != nil {
				return "", err
			}
			return account.Address(), nil
		},
	})
	if err != nil {
		panic(err)
	}
}
