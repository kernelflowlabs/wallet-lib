package ton

import (
	"github.com/kernelflowlabs/wallet-lib/session"
)

// SessionChain is this chain's name in a signing session.
const SessionChain = "ton"

// DerivationPath returns the empty string on purpose.
//
// TON does not use BIP32. Its key comes from the TON mnemonic scheme, which
// consumes the whole phrase and has no account index, so every index resolves
// to the same key. A caller asking for index 3 gets index 0's wallet. That is
// a property of the chain, not an oversight, and it is why this chain cannot
// carry a second account from one phrase.
func DerivationPath(accountIndex int) string {
	return ""
}

func init() {
	err := session.RegisterChain(session.Chain{
		Name: SessionChain,
		Path: DerivationPath,
		Derive: func(mnemonic, path string) ([]byte, error) {
			account, err := NewAccountFromMnemonic(mnemonic, path)
			if err != nil {
				return nil, err
			}
			return account.PrivateKey(), nil
		},
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
