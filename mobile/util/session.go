package util

import (
	"encoding/json"

	"github.com/kernelflowlabs/wallet-lib/session"
)

type sessionInfo struct {
	Locked    bool              `json:"locked,omitempty"`
	Addresses map[string]string `json:"addresses,omitempty"`
	ExpiresAt int64             `json:"expiresAt,omitempty"`
}

func SessionUnlock(keystoreJSON string, password []byte, expectedEvmAddress string, accountIndex, ttlSeconds, idleSeconds int) (string, error) {
	return session.Unlock(keystoreJSON, password, expectedEvmAddress, accountIndex,
		session.Options{TTLSeconds: ttlSeconds, IdleSeconds: idleSeconds})
}

func SessionUnlockMnemonic(mnemonic []byte, expectedEvmAddress string, accountIndex, ttlSeconds, idleSeconds int) (string, error) {
	return session.UnlockMnemonic(mnemonic, expectedEvmAddress, accountIndex,
		session.Options{TTLSeconds: ttlSeconds, IdleSeconds: idleSeconds})
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
		Addresses: info.Addresses,
		ExpiresAt: info.ExpiresAt.Unix(),
	})
}

func marshalSessionInfo(info *sessionInfo) string {
	out, err := json.Marshal(info)
	if err != nil {
		return `{"locked":true}`
	}
	return string(out)
}
