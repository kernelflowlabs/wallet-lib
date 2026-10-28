package multiversx

import (
	"fmt"

	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/kernelflowlabs/wallet-lib/session"
)

// SessionChain is this chain's name in a signing session.
const SessionChain = "multiversx"

// DerivationPath places the account index where this chain's own wallets put
// it. Changing it changes every address, so it is pinned by a test.
func DerivationPath(accountIndex int) string {
	return fmt.Sprintf("m/44'/508'/0'/0'/%d'", accountIndex)
}

func init() {
	err := session.RegisterChain(session.Chain{
		Name:   SessionChain,
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
