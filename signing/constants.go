package signing

// Magic sentinel addresses used to denote a chain's native asset (as opposed to
// a token contract) when building transfers.
const (
	MagicAddressForZeroEVM          = "0x0000000000000000000000000000000000000000"
	MagicContactAddressForNative    = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	MagicContactAddressForNativeTRX = "TXka46PPwttNPWfFDPtt3GUodbPThyufaV"
	MagicContactAddressForNativeSOL = "So11111111111111111111111111111111111111112"
	MagicNumberForMaxAmount         = "115792089237316195423570985008687907853269984665640564039457584007913129639935"
)

// Transaction kinds understood by the per-chain transaction builders.
const (
	TxTypeTransfer        = "0"
	TxTypeMint            = "1"
	TxTypeBurn            = "2"
	TxTypeContractCall    = "3"
	TxTypeAccountActivate = "4"
	TxTypeKrc20Commit     = "5"
	TxTypeKrc20Reveal     = "6"
)

// Chain family identifiers, grouping chains that share an address/signature scheme.
const (
	FamilyOfNone      = 0
	FamilyOfEVM       = 1
	FamilyOfUTXO      = 2
	FamilyOfCOSMOS    = 3
	FamilyOfSUBSTRATE = 4
	FamilyOfTRX       = 5
	FamilyOfKAS       = 6
	FamilyOfSOL       = 7
	FamilyOfAPT       = 8
	FamilyOfSUI       = 9
	FamilyOfEGLD      = 10
	FamilyOfTON       = 11
	FamilyOfStellar   = 12
	FamilyOfNear      = 13
	FamilyOfFilecoin  = 14
	FamilyOfAlgorand  = 15
)
