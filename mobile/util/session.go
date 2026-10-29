package util

import (
	"encoding/json"
	"strings"

	"github.com/kernelflowlabs/wallet-lib/session"
)

type sessionInfo struct {
	Locked      bool              `json:"locked,omitempty"`
	Addresses   map[string]string `json:"addresses,omitempty"`
	Unavailable map[string]string `json:"unavailable,omitempty"`
	ExpiresAt   int64             `json:"expiresAt,omitempty"`
}

// evmChain names the chain whose address the app checks against on unlock.
// It is a literal rather than signing/evm.SessionChain because mobile/util has
// no build tags: importing the EVM package here would link it into builds that
// deliberately leave EVM out. A test asserts the two stay equal.
const evmChain = "evm"

func SessionUnlock(keystoreJSON string, password []byte, expectedEvmAddress string, accountIndex, ttlSeconds, idleSeconds int) (string, error) {
	return SessionUnlockChains(keystoreJSON, password, expectedEvmAddress, "", accountIndex, ttlSeconds, idleSeconds)
}

// SessionUnlockChains opens a session for a chosen set of chains, named as a
// comma separated list ("evm,solana,tron"). An empty list opens every chain this
// build registered, which is thirty of them and costs a BIP39 derivation
// each, so an app that signs on three should name those three.
func SessionUnlockChains(keystoreJSON string, password []byte, expectedEvmAddress, chains string, accountIndex, ttlSeconds, idleSeconds int) (string, error) {
	selected := splitChains(chains)
	if len(selected) > 0 && !containsFold(selected, evmChain) {
		selected = append(selected, evmChain)
	}
	return session.Unlock(keystoreJSON, password, evmChain, expectedEvmAddress, accountIndex,
		session.Options{TTLSeconds: ttlSeconds, IdleSeconds: idleSeconds, Chains: selected})
}

func containsFold(list []string, name string) bool {
	for _, v := range list {
		if strings.EqualFold(strings.TrimSpace(v), name) {
			return true
		}
	}
	return false
}

func SessionUnlockMnemonic(mnemonic []byte, expectedEvmAddress string, accountIndex, ttlSeconds, idleSeconds int) (string, error) {
	return SessionUnlockMnemonicChains(mnemonic, expectedEvmAddress, "", accountIndex, ttlSeconds, idleSeconds)
}

// SessionUnlockMnemonicChains is SessionUnlockMnemonic for a chosen set of
// chains, named as a comma separated list.
func SessionUnlockMnemonicChains(mnemonic []byte, expectedEvmAddress, chains string, accountIndex, ttlSeconds, idleSeconds int) (string, error) {
	chain := evmChain
	selected := splitChains(chains)
	if expectedEvmAddress == "" {
		chain = ""
	} else if len(selected) > 0 && !containsFold(selected, evmChain) {
		selected = append(selected, evmChain)
	}
	return session.UnlockMnemonic(mnemonic, chain, expectedEvmAddress, accountIndex,
		session.Options{TTLSeconds: ttlSeconds, IdleSeconds: idleSeconds, Chains: selected})
}

// SessionChains lists every chain this build can open a session for, comma
// separated.
func SessionChains() string {
	return strings.Join(session.RegisteredChains(), ",")
}

func splitChains(chains string) []string {
	chains = strings.TrimSpace(chains)
	if chains == "" {
		return nil
	}
	parts := strings.Split(chains, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func SessionLock(handle string) {
	session.Lock(handle)
}

func SessionLockAll() {
	session.LockAll()
}

func SessionTouch(handle string) bool {
	return session.Touch(handle)
}

func SessionInfo(handle string) string {
	info, err := session.Info(handle)
	if err != nil {
		return marshalSessionInfo(&sessionInfo{Locked: true})
	}
	return marshalSessionInfo(&sessionInfo{
		Addresses:   info.Addresses,
		Unavailable: info.Unavailable,
		ExpiresAt:   info.ExpiresAt.Unix(),
	})
}

func marshalSessionInfo(info *sessionInfo) string {
	out, err := json.Marshal(info)
	if err != nil {
		return `{"locked":true}`
	}
	return string(out)
}
