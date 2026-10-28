package solana

import (
	"fmt"

	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/kernelflowlabs/wallet-lib/session"
)

const DerivationPathPrefix = "m/44'/501'/0'"

func DerivationPath(accountIndex int) string {
	return fmt.Sprintf("%s/%d'", DerivationPathPrefix, accountIndex)
}

func init() {
	err := session.RegisterChain(session.Chain{
		Name:   session.ChainSOL,
		Path:   DerivationPath,
		Derive: key.DerivePrivateKeyED25519,
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
