package session

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Chain is what a chain package registers about itself from its own init.
// The session package deliberately knows no chain names: adding a chain must
// never mean editing this package.
type Chain struct {
	Name    string
	Path    func(accountIndex int) string
	Derive  func(mnemonic []byte, path string) ([]byte, error)
	Address func(privateKey []byte) (string, error)

	// CaseInsensitiveAddress says whether two spellings that differ only in
	// case name the same account. It is true for EVM, whose hex addresses
	// carry an optional EIP-55 checksum in their capitalisation, and false
	// for base58 chains such as Solana and TRON, where case is part of the
	// address and folding it would accept a different account.
	CaseInsensitiveAddress bool
}

var (
	chainMu sync.RWMutex
	chains  = map[string]Chain{}
)

func RegisterChain(c Chain) error {
	if c.Name == "" {
		return fmt.Errorf("chain name is required")
	}
	if c.Path == nil || c.Derive == nil || c.Address == nil {
		return fmt.Errorf("chain %q: Path, Derive and Address are required", c.Name)
	}
	chainMu.Lock()
	defer chainMu.Unlock()
	if _, exists := chains[c.Name]; exists {
		return fmt.Errorf("chain %q is already registered", c.Name)
	}
	chains[c.Name] = c
	return nil
}

func RegisteredChains() []string {
	chainMu.RLock()
	defer chainMu.RUnlock()
	return sortedNames()
}

func registeredChains() []Chain {
	chainMu.RLock()
	defer chainMu.RUnlock()
	out := make([]Chain, 0, len(chains))
	for _, name := range sortedNames() {
		out = append(out, chains[name])
	}
	return out
}

func sortedNames() []string {
	names := make([]string, 0, len(chains))
	for name := range chains {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func chainByName(name string) (Chain, bool) {
	chainMu.RLock()
	defer chainMu.RUnlock()
	c, ok := chains[name]
	return c, ok
}

// selectChains resolves the caller's list to registered chains, or returns
// every registered chain when the list is empty.
func selectChains(names []string) ([]Chain, error) {
	if len(names) == 0 {
		return registeredChains(), nil
	}
	chainMu.RLock()
	defer chainMu.RUnlock()
	out := make([]Chain, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		c, ok := chains[name]
		if !ok {
			return nil, fmt.Errorf("chain %q is not registered in this build", name)
		}
		seen[name] = true
		out = append(out, c)
	}
	return out, nil
}

func containsChain(list []Chain, name string) bool {
	for _, c := range list {
		if c.Name == name {
			return true
		}
	}
	return false
}
