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
	ErrLocked           = errors.New("locked")
	ErrAddressMismatch  = errors.New("address_mismatch")
	ErrInvalidInput     = errors.New("invalid_input")
	ErrDecryptFailed    = errors.New("decrypt_failed")
	ErrChainUnavailable = errors.New("chain_unavailable")
)

const (
	DefaultTTLSeconds  = 1800
	DefaultIdleSeconds = 300
	MaxAccountIndex    = 1<<31 - 1
)

type Options struct {
	TTLSeconds  int
	IdleSeconds int

	// Chains limits which registered chains this session derives a key for.
	// Empty means every chain the build registered.
	//
	// It matters: deriving one key costs a BIP39 PBKDF2 pass, so opening all
	// thirty registered chains takes ~180ms on a desktop and well over a
	// second on a phone, on top of the keystore's scrypt. An app that only
	// signs on three chains should say so.
	Chains []string
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
	// Chains whose key could not be derived for this account. They stay in the
	// session as a named failure rather than taking the whole unlock down with
	// them, so one broken chain cannot lock a user out of the others.
	unavailable map[string]string

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
	// Bumped by LockAll. An unlock reads it before deriving and will not
	// register its session if it changed meanwhile: deriving takes seconds,
	// and without this a LockAll issued while the app goes to the background
	// would clear the table and then have a freshly derived session appear
	// behind it.
	lockGeneration atomic.Uint64
)

// Unlock opens the keystore and derives a key for every registered chain.
// expectChain names the chain whose address expectAddress is checked against;
// it must be registered in this build.
func Unlock(keystoreJSON string, password []byte, expectChain, expectAddress string, accountIndex int, opt Options) (string, error) {
	if strings.TrimSpace(keystoreJSON) == "" {
		return "", fmt.Errorf("%w: keystore is empty", ErrInvalidInput)
	}
	if strings.TrimSpace(expectAddress) == "" {
		return "", fmt.Errorf("%w: an expected address is required", ErrInvalidInput)
	}
	defer crypto.Wipe(password)
	// Read the lock generation before the keystore is opened, not after.
	// scrypt takes a second or more, and a LockAll issued during it has to
	// win: otherwise the app goes to the background, everything is wiped, and
	// this unlock registers a live session behind the lock a moment later.
	generation := lockGeneration.Load()
	mnemonic, err := crypto.ReadFromKeyStoreBytes(keystoreJSON, password, true)
	if err != nil {
		return "", fmt.Errorf("%w: cannot open keystore", ErrDecryptFailed)
	}
	defer crypto.Wipe(mnemonic)
	return newSessionAt(generation, mnemonic, expectChain, expectAddress, accountIndex, opt)
}

// UnlockMnemonic is for wallet creation and import, where the caller already
// holds the plaintext and no address is known yet. expectAddress may be empty
// there; when it is given, expectChain must be too.
func UnlockMnemonic(mnemonic []byte, expectChain, expectAddress string, accountIndex int, opt Options) (string, error) {
	generation := lockGeneration.Load()
	defer crypto.Wipe(mnemonic)
	if len(mnemonic) == 0 {
		return "", fmt.Errorf("%w: mnemonic is empty", ErrInvalidInput)
	}
	return newSessionAt(generation, mnemonic, expectChain, expectAddress, accountIndex, opt)
}

func newSessionAt(generation uint64, mnemonic []byte, expectChain, expectAddress string, accountIndex int, opt Options) (string, error) {
	if accountIndex < 0 || accountIndex > MaxAccountIndex {
		return "", fmt.Errorf("%w: accountIndex out of range", ErrInvalidInput)
	}
	ttl, idle, err := opt.normalize()
	if err != nil {
		return "", err
	}
	registered, err := selectChains(opt.Chains)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if len(registered) == 0 {
		return "", fmt.Errorf("%w: no chain is registered in this build", ErrInvalidInput)
	}

	expected := strings.TrimSpace(expectAddress)
	expectChain = strings.TrimSpace(expectChain)
	var checkChain Chain
	if expected != "" {
		if expectChain == "" {
			return "", fmt.Errorf("%w: an expected address needs the chain it belongs to", ErrInvalidInput)
		}
		var ok bool
		checkChain, ok = chainByName(expectChain)
		if !ok {
			return "", fmt.Errorf("%w: chain %q is not registered in this build", ErrInvalidInput, expectChain)
		}
		// Checking an address against a chain this session will not open
		// would silently skip the check.
		if !containsChain(registered, expectChain) {
			return "", fmt.Errorf("%w: chain %q is not among the chains this session opens", ErrInvalidInput, expectChain)
		}
	}

	keys := make(map[string][]byte, len(registered))
	addresses := make(map[string]string, len(registered))
	unavailable := map[string]string{}
	wipeAll := func() {
		for _, k := range keys {
			crypto.Wipe(k)
		}
	}
	for _, c := range registered {
		priv, err := c.Derive(string(mnemonic), c.Path(accountIndex))
		if err != nil {
			unavailable[c.Name] = "key derivation failed"
			continue
		}
		address, err := c.Address(priv)
		if err != nil {
			crypto.Wipe(priv)
			unavailable[c.Name] = "address derivation failed"
			continue
		}
		keys[c.Name] = priv
		addresses[c.Name] = address
	}

	if expected != "" {
		got, ok := addresses[expectChain]
		if !ok {
			wipeAll()
			return "", fmt.Errorf("%w: cannot derive a %s address to check against", ErrInvalidInput, expectChain)
		}
		if !addressesMatch(got, expected, checkChain.CaseInsensitiveAddress) {
			wipeAll()
			return "", fmt.Errorf("%w: keystore does not hold the expected account", ErrAddressMismatch)
		}
	}

	// #5: a handle with no usable key is worse than a failure. It looks like a
	// working wallet and only falls over at the first signature.
	if len(keys) == 0 {
		return "", fmt.Errorf("%w: no chain could derive a key from this mnemonic", ErrInvalidInput)
	}

	handle, err := newHandle()
	if err != nil {
		wipeAll()
		return "", err
	}

	now := time.Now()
	e := &entry{
		keys:        keys,
		addresses:   addresses,
		unavailable: unavailable,
		expiresAt:   now.Add(ttl),
		idle:        idle,
	}
	e.lastUsed.Store(now.UnixNano())

	sweep(now)

	tableMu.Lock()
	if lockGeneration.Load() != generation {
		tableMu.Unlock()
		e.wipe()
		return "", fmt.Errorf("%w: locked while unlocking", ErrLocked)
	}
	table[handle] = e
	tableMu.Unlock()
	return handle, nil
}

func addressesMatch(got, expected string, caseInsensitive bool) bool {
	if caseInsensitive {
		return strings.EqualFold(got, expected)
	}
	return got == expected
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
	lockGeneration.Add(1)
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
		if reason, known := e.unavailable[chain]; known {
			return fmt.Errorf("%w: %s (%s)", ErrChainUnavailable, chain, reason)
		}
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
