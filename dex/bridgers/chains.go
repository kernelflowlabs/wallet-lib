package bridgers

import (
	"fmt"
	"strings"
)

var chainToProvider = map[string]string{
	"ETH":       "ETH",
	"BNB":       "BSC",
	"BSC":       "BSC",
	"POLYGON":   "POLYGON",
	"HECO":      "HECO",
	"OKEX":      "OKEXCHAIN",
	"OKEXCHAIN": "OKEXCHAIN",
	"TRX":       "TRON",
	"TRON":      "TRON",
	"FTM":       "FANTOM",
	"FANTOM":    "FANTOM",
	"ARB":       "ARBITRUM",
	"ARBITRUM":  "ARBITRUM",
	"AVAXC":     "AVALANCHE",
	"AVALANCHE": "AVALANCHE",
	"OP":        "OPTIMISM",
	"OPTIMISM":  "OPTIMISM",
	"CRONOS":    "CRONOS",
	"SOL":       "SOLANA",
	"SOLANA":    "SOLANA",
	"APT":       "APT",
	"ZKSYNC":    "ZKSYNC",
	"MANTLE":    "MNT",
	"MNT":       "MNT",
	"CORE":      "CORE",
	"SUI":       "SUI",
	"CFX":       "CFX",
	"CFXEVM":    "CFX",
	"BASE":      "BASE",
	"OPBNB":     "opBNB",
	"ZKEVM":     "zkEVM",
	"SCROLL":    "SCROLL",
	"LINEA":     "LINEA",
	"BLAST":     "Blast",
	"MERLIN":    "Merlin",
	"ONCHAIN":   "Onchain",
	"CELO":      "CELO",
	"TON":       "TON",
	"XLAYER":    "XLayer",
	"APE":       "APE",
	"APECHAIN":  "APE",
	"XRP":       "XRP",
	"BTC":       "BTC",
	"DOGE":      "DOGE",
	"LTC":       "LTC",
	"BCH":       "BCH",
	"COSMOS":    "COSMOS",
	"DASH":      "DASH",
	"ETHF":      "ETHF",
	"SONIC":     "Sonic",
	"BERA":      "BERA",
	"UNICHAIN":  "Unichain",
	"HYPEEVM":   "HyperEVM",
	"HYPEREVM":  "HyperEVM",
	"MORPH":     "Morph",
	"MON":       "MONAD",
	"MONAD":     "MONAD",
	"ENI":       "ENI",
	"KASPLEX":   "Kasplex",
	"ROBINHOOD": "Robinhood",
	"MEGAETH":   "MegaETH",
	"BOTCHAIN":  "BOTCHAIN",
	"ANUBIS":    "Anubis",
	"STABLE":    "Stable",
	"ARC":       "ARC",
}

var providerToChain = map[string]string{
	"ETH":       "ETH",
	"BSC":       "BNB",
	"POLYGON":   "POLYGON",
	"HECO":      "HECO",
	"OKEXCHAIN": "OKEX",
	"TRON":      "TRX",
	"FANTOM":    "FTM",
	"ARBITRUM":  "ARB",
	"AVALANCHE": "AVAXC",
	"OPTIMISM":  "OP",
	"CRONOS":    "CRONOS",
	"SOLANA":    "SOL",
	"APT":       "APT",
	"ZKSYNC":    "ZKSYNC",
	"MNT":       "MANTLE",
	"CORE":      "CORE",
	"SUI":       "SUI",
	"CFX":       "CFXEVM",
	"BASE":      "BASE",
	"OPBNB":     "OPBNB",
	"ZKEVM":     "ZKEVM",
	"SCROLL":    "SCROLL",
	"LINEA":     "LINEA",
	"BLAST":     "BLAST",
	"MERLIN":    "MERLIN",
	"ONCHAIN":   "ONCHAIN",
	"CELO":      "CELO",
	"TON":       "TON",
	"XLAYER":    "XLAYER",
	"APE":       "APECHAIN",
	"XRP":       "XRP",
	"BTC":       "BTC",
	"DOGE":      "DOGE",
	"LTC":       "LTC",
	"BCH":       "BCH",
	"COSMOS":    "COSMOS",
	"DASH":      "DASH",
	"ETHF":      "ETHF",
	"SONIC":     "SONIC",
	"BERA":      "BERA",
	"UNICHAIN":  "UNICHAIN",
	"HYPEREVM":  "HYPEEVM",
	"MORPH":     "MORPH",
	"MONAD":     "MON",
	"ENI":       "ENI",
	"KASPLEX":   "KASPLEX",
	"ROBINHOOD": "ROBINHOOD",
	"MEGAETH":   "MEGAETH",
	"BOTCHAIN":  "BOTCHAIN",
	"ANUBIS":    "ANUBIS",
	"STABLE":    "STABLE",
	"ARC":       "ARC",
}

var evmProviderChains = map[string]struct{}{
	"ETH": {}, "BSC": {}, "POLYGON": {}, "HECO": {}, "OKEXCHAIN": {},
	"FANTOM": {}, "ARBITRUM": {}, "AVALANCHE": {}, "OPTIMISM": {},
	"CRONOS": {}, "ZKSYNC": {}, "MNT": {}, "CORE": {}, "CFX": {},
	"BASE": {}, "OPBNB": {}, "ZKEVM": {}, "SCROLL": {}, "LINEA": {},
	"BLAST": {}, "MERLIN": {}, "ONCHAIN": {}, "CELO": {}, "XLAYER": {},
	"APE": {}, "ETHF": {}, "SONIC": {}, "BERA": {}, "UNICHAIN": {},
	"HYPEREVM": {}, "MORPH": {}, "MONAD": {}, "ENI": {}, "KASPLEX": {},
	"ROBINHOOD": {}, "MEGAETH": {}, "BOTCHAIN": {}, "ANUBIS": {},
	"STABLE": {}, "ARC": {},
}

func providerChain(chain string) (string, error) {
	value, ok := chainToProvider[strings.ToUpper(strings.TrimSpace(chain))]
	if !ok {
		return "", fmt.Errorf("unsupported Bridgers chain %q", chain)
	}
	return value, nil
}

func canonicalChain(chain string) string {
	value := providerToChain[strings.ToUpper(strings.TrimSpace(chain))]
	if value == "" {
		return chain
	}
	return value
}

func isEVMProviderChain(chain string) bool {
	_, ok := evmProviderChains[strings.ToUpper(chain)]
	return ok
}
