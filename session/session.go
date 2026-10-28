package session

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kernelflowlabs/wallet-lib/crypto"
)

var (
	ErrLocked          = errors.New("locked")
	ErrAddressMismatch = errors.New("address_mismatch")
	ErrInvalidInput    = errors.New("invalid_input")
	ErrDecryptFailed   = errors.New("decrypt_failed")
)

const (
	DefaultTTLSeconds  = 1800
	DefaultIdleSeconds = 300
	MaxAccountIndex    = 1<<31 - 1
)

type Options struct {
	TTLSeconds  int
	IdleSeconds int
}

func (o Options) normalize() (time.Duration, time.Duration, error) {
	if o.TTLSeconds < 0 || o.IdleSeconds < 0 {
		return 0, 0, fmt.Errorf("%w: ttl and idle must not be negative", ErrInvalidInput)
	}
	ttl := o.TTLSeconds
	if ttl == 0 {
		ttl = DefaultTTLSeconds
	}
	idle := o.IdleSeconds
	if idle == 0 {
		idle = DefaultIdleSeconds
	}
	return time.Duration(ttl) * time.Second, time.Duration(idle) * time.Second, nil
}

type Addresses struct {
	Addresses map[string]string
	ExpiresAt time.Time
}

type entry struct {
	mu        sync.RWMutex
	locked    bool
	keys      map[string][]byte
	addresses map[string]string

	expiresAt time.Time
	idle      time.Duration
	lastUsed  atomic.Int64
}

func (e *entry) expired(now time.Time) bool {
	if now.After(e.expiresAt) {
		return true
	}
	return now.Sub(time.Unix(0, e.lastUsed.Load())) > e.idle
}

func (e *entry) wipe() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.locked {
		return
	}
	e.locked = true
	for _, k := range e.keys {
		crypto.Wipe(k)
	}
	e.keys = nil
}

var (
	tableMu sync.RWMutex
	table   = map[string]*entry{}
)

func Unlock(keystoreJSON string, password []byte, expectedEvmAddress string, accountIndex int, opt Options) (string, error) {
	if strings.TrimSpace(keystoreJSON) == "" {
		return "", fmt.Errorf("%w: keystore is empty", ErrInvalidInput)
	}
	if strings.TrimSpace(expectedEvmAddress) == "" {
		return "", fmt.Errorf("%w: expectedEvmAddress is required", ErrInvalidInput)
	}
	defer crypto.Wipe(password)
	mnemonic, err := crypto.ReadFromKeyStoreBytes(keystoreJSON, password, true)
	if err != nil {
		return "", fmt.Errorf("%w: cannot open keystore", ErrDecryptFailed)
	}
	defer crypto.Wipe(mnemonic)
	return newSession(mnemonic, expectedEvmAddress, accountIndex, opt)
}

func UnlockMnemonic(mnemonic []byte, expectedEvmAddress string, accountIndex int, opt Options) (string, error) {
	defer crypto.Wipe(mnemonic)
	if len(mnemonic) == 0 {
		return "", fmt.Errorf("%w: mnemonic is empty", ErrInvalidInput)
	}
	return newSession(mnemonic, expectedEvmAddress, accountIndex, opt)
}

func newSession(mnemonic []byte, expectedEvmAddress string, accountIndex int, opt Options) (string, error) {
	if accountIndex < 0 || accountIndex > MaxAccountIndex {
		return "", fmt.Errorf("%w: accountIndex out of range", ErrInvalidInput)
	}
	ttl, idle, err := opt.normalize()
	if err != nil {
		return "", err
	}
	registered := registeredChains()
	if len(registered) == 0 {
		return "", fmt.Errorf("%w: no chain is registered in this build", ErrInvalidInput)
	}

	keys := make(map[string][]byte, len(registered))
	addresses := make(map[string]string, len(registered))
	wipeAll := func() {
		for _, k := range keys {
			crypto.Wipe(k)
		}
	}
	for _, c := range registered {
		priv, err := c.Derive(string(mnemonic), c.Path(accountIndex))
		if err != nil {
			wipeAll()
			return "", fmt.Errorf("%w: cannot derive %s key", ErrInvalidInput, c.Name)
		}
		address, err := c.Address(priv)
		if err != nil {
			crypto.Wipe(priv)
			wipeAll()
			return "", fmt.Errorf("%w: cannot derive %s address", ErrInvalidInput, c.Name)
		}
		keys[c.Name] = priv
		addresses[c.Name] = address
	}

	if expected := strings.TrimSpace(expectedEvmAddress); expected != "" {
		got, ok := addresses[ChainEVM]
		if !ok {
			wipeAll()
			return "", fmt.Errorf("%w: evm is not registered in this build", ErrInvalidInput)
		}
		if !strings.EqualFold(got, expected) {
			wipeAll()
			return "", fmt.Errorf("%w: keystore does not hold the expected account", ErrAddressMismatch)
		}
	}

	handle, err := newHandle()
	if err != nil {
		wipeAll()
		return "", err
	}

	now := time.Now()
	e := &entry{
		keys:      keys,
		addresses: addresses,
		expiresAt: now.Add(ttl),
		idle:      idle,
	}
	e.lastUsed.Store(now.UnixNano())

	sweep(now)

	tableMu.Lock()
	table[handle] = e
	tableMu.Unlock()
	return handle, nil
}

func newHandle() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("%w: cannot generate handle", ErrInvalidInput)
	}
	return hex.EncodeToString(b), nil
}

func Lock(handle string) {
	tableMu.Lock()
	e := table[handle]
	delete(table, handle)
	tableMu.Unlock()
	if e != nil {
		e.wipe()
	}
	sweep(time.Now())
}

func LockAll() {
	tableMu.Lock()
	dead := make([]*entry, 0, len(table))
	for _, e := range table {
		dead = append(dead, e)
	}
	table = map[string]*entry{}
	tableMu.Unlock()
	for _, e := range dead {
		e.wipe()
	}
}

func sweep(now time.Time) {
	tableMu.Lock()
	var dead []*entry
	for handle, e := range table {
		if e.expired(now) {
			dead = append(dead, e)
			delete(table, handle)
		}
	}
	tableMu.Unlock()
	for _, e := range dead {
		e.wipe()
	}
}

func lookup(handle string) (*entry, error) {
	tableMu.RLock()
	e := table[handle]
	tableMu.RUnlock()
	if e == nil {
		return nil, fmt.Errorf("%w: no such session", ErrLocked)
	}
	if e.expired(time.Now()) {
		Lock(handle)
		return nil, fmt.Errorf("%w: session expired", ErrLocked)
	}
	return e, nil
}

func Info(handle string) (*Addresses, error) {
	e, err := lookup(handle)
	if err != nil {
		return nil, err
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.locked {
		return nil, fmt.Errorf("%w: session was locked", ErrLocked)
	}
	out := make(map[string]string, len(e.addresses))
	for name, address := range e.addresses {
		out[name] = address
	}
	return &Addresses{Addresses: out, ExpiresAt: e.expiresAt}, nil
}

func Touch(handle string) bool {
	e, err := lookup(handle)
	if err != nil {
		return false
	}
	e.lastUsed.Store(time.Now().UnixNano())
	return true
}

func WithKey(handle, chain string, fn func(privateKey []byte, address string) error) error {
	if fn == nil {
		return fmt.Errorf("%w: no action given", ErrInvalidInput)
	}
	e, err := lookup(handle)
	if err != nil {
		return err
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.locked {
		return fmt.Errorf("%w: session was locked", ErrLocked)
	}
	priv, ok := e.keys[chain]
	if !ok {
		return fmt.Errorf("%w: chain %q is not in this session", ErrInvalidInput, chain)
	}
	e.lastUsed.Store(time.Now().UnixNano())
	return fn(priv, e.addresses[chain])
}

func Address(handle, chain string) (string, error) {
	var address string
	err := WithKey(handle, chain, func(_ []byte, addr string) error {
		address = addr
		return nil
	})
	return address, err
}
