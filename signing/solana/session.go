package solana

import (
	"fmt"

	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/kernelflowlabs/wallet-lib/session"
)

// SessionChain is this chain's name in a signing session.
const SessionChain = "solana"

const DerivationPathPrefix = "m/44'/501'/0'"

func DerivationPath(accountIndex int) string {
	return fmt.Sprintf("%s/%d'", DerivationPathPrefix, accountIndex)
}

func init() {
	err := session.RegisterChain(session.Chain{
		Name:   SessionChain,
		Path:   DerivationPath,
		Derive: key.DerivePrivateKeyED25519Bytes,
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
