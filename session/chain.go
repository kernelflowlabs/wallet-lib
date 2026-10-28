package session

import (
	"fmt"
	"sort"
	"sync"
)

type Chain struct {
	Name    string
	Path    func(accountIndex int) string
	Derive  func(mnemonic, path string) ([]byte, error)
	Address func(privateKey []byte) (string, error)
}

const (
	ChainEVM = "evm"
	ChainSOL = "sol"
	ChainTRX = "trx"
)

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
