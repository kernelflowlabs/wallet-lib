package evm

import (
	"fmt"

	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/kernelflowlabs/wallet-lib/session"
)

// SessionChain is this chain's name in a signing session.
const SessionChain = "evm"

const DerivationPathPrefix = "m/44'/60'/0'"

func DerivationPath(accountIndex int) string {
	return fmt.Sprintf("%s/0/%d", DerivationPathPrefix, accountIndex)
}

func init() {
	err := session.RegisterChain(session.Chain{
		Name:                   SessionChain,
		CaseInsensitiveAddress: true,
		Path:                   DerivationPath,
		Derive:                 key.DerivePrivateKeyECDSABytes,
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
