package solana

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"strconv"

	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/types"
	"github.com/mr-tron/base58"
)

const (
	KindSystemTransfer      = "system_transfer"
	KindSystemAssign        = "system_assign"
	KindSystemCreateAccount = "system_create_account"
	KindAdvanceNonce        = "advance_nonce"
	KindSplTransfer         = "spl_transfer"
	KindSplTransferChecked  = "spl_transfer_checked"
	KindSplApprove          = "spl_approve"
	KindSplRevoke           = "spl_revoke"
	KindSplSetAuthority     = "spl_set_authority"
	KindSplCloseAccount     = "spl_close_account"
	KindComputeBudget       = "compute_budget"
	KindUnknownProgram      = "unknown_program"
)

const (
	WarnDurableNonce     = "durable_nonce"
	WarnAuthorityChange  = "authority_change"
	WarnDelegateApproval = "delegate_approval"
	WarnCloseAccount     = "close_account"
	WarnExternalFeePayer = "external_fee_payer"
	WarnUnknownProgram   = "unknown_program"
	WarnUnresolvedLookup = "unresolved_lookup_accounts"
)

type AccountView struct {
	Address    string `json:"address,omitempty"`
	LookupKey  string `json:"lookupKey,omitempty"`
	LookupIdx  int    `json:"lookupIndex,omitempty"`
	Unresolved bool   `json:"unresolved,omitempty"`
	Signer     bool   `json:"signer,omitempty"`
	Writable   bool   `json:"writable,omitempty"`
}

type InstructionView struct {
	ProgramID string        `json:"programId"`
	Kind      string        `json:"kind"`
	Accounts  []AccountView `json:"accounts"`
	Amount    string        `json:"amount,omitempty"`
	Authority string        `json:"authority,omitempty"`
	Delegate  string        `json:"delegate,omitempty"`
	NewOwner  string        `json:"newOwner,omitempty"`
	Recipient string        `json:"recipient,omitempty"`
}

type TransactionDescription struct {
	Version           string            `json:"version"`
	FeePayer          string            `json:"feePayer,omitempty"`
	RecentBlockhash   string            `json:"recentBlockhash,omitempty"`
	ExistingSignature int               `json:"existingSignatures"`
	RequiredSigners   int               `json:"requiredSigners"`
	OwnerIsSigner     bool              `json:"ownerIsSigner"`
	LookupTables      []string          `json:"lookupTables,omitempty"`
	Instructions      []InstructionView `json:"instructions"`
	Warnings          []string          `json:"warnings"`
	DecodeError       string            `json:"decodeError,omitempty"`
}

var (
	computeBudgetProgram = common.PublicKeyFromString("ComputeBudget111111111111111111111111111111")
	token2022Program     = common.PublicKeyFromString("TokenzQdBNbLqP5VEhdkAS6EPFLC1PHnBqCXEpPxuEb")
)

// DescribeSolTransaction explains a serialized transaction a dApp sent. It is
// offline: an account that lives in an address lookup table is reported with
// its table and index rather than resolved, because resolving it would mean a
// network call.
func DescribeSolTransaction(txBytes []byte, ownerAddress string) string {
	out := &TransactionDescription{Warnings: []string{}, Instructions: []InstructionView{}}

	tx, err := SafeTransactionDeserialize(txBytes)
	if err != nil {
		out.DecodeError = err.Error()
		return marshalSolDescription(out)
	}
	roundTrip, err := tx.Serialize()
	if err != nil {
		out.DecodeError = err.Error()
		return marshalSolDescription(out)
	}
	if !bytes.Equal(roundTrip, txBytes) {
		out.DecodeError = "transaction does not survive a serialization round trip; refusing to describe a re-encoding"
		return marshalSolDescription(out)
	}
	message := tx.Message

	out.Version = string(message.Version)
	if out.Version == "" {
		out.Version = string(types.MessageVersionLegacy)
	}
	out.RecentBlockhash = message.RecentBlockHash
	out.RequiredSigners = int(message.Header.NumRequireSignatures)
	for _, sig := range tx.Signatures {
		if !isZeroSignature(sig) {
			out.ExistingSignature++
		}
	}
	if len(message.Accounts) > 0 {
		out.FeePayer = message.Accounts[0].ToBase58()
	}
	for _, table := range message.AddressLookupTables {
		out.LookupTables = append(out.LookupTables, table.AccountKey.ToBase58())
	}

	if ownerAddress != "" {
		for i := 0; i < int(message.Header.NumRequireSignatures) && i < len(message.Accounts); i++ {
			if message.Accounts[i].ToBase58() == ownerAddress {
				out.OwnerIsSigner = true
				break
			}
		}
		if out.FeePayer != "" && out.FeePayer != ownerAddress {
			out.Warnings = append(out.Warnings, WarnExternalFeePayer)
		}
	}

	unresolved := false
	for index, compiled := range message.Instructions {
		view := describeInstruction(message, compiled, &unresolved)
		if index == 0 && view.Kind == KindAdvanceNonce {
			out.Warnings = append(out.Warnings, WarnDurableNonce)
		}
		switch view.Kind {
		case KindSplSetAuthority:
			out.Warnings = appendOnce(out.Warnings, WarnAuthorityChange)
		case KindSplApprove:
			out.Warnings = appendOnce(out.Warnings, WarnDelegateApproval)
		case KindSplCloseAccount:
			out.Warnings = appendOnce(out.Warnings, WarnCloseAccount)
		case KindUnknownProgram:
			out.Warnings = appendOnce(out.Warnings, WarnUnknownProgram)
		}
		out.Instructions = append(out.Instructions, view)
	}
	if unresolved {
		out.Warnings = append(out.Warnings, WarnUnresolvedLookup)
	}
	return marshalSolDescription(out)
}

// describeInstruction resolves account indexes by hand instead of calling the
// SDK's DecompileInstructions, which panics on a v0 message.
func describeInstruction(m types.Message, compiled types.CompiledInstruction, unresolved *bool) InstructionView {
	view := InstructionView{Accounts: []AccountView{}}
	view.ProgramID = accountAt(m, compiled.ProgramIDIndex, unresolved).Address
	for _, index := range compiled.Accounts {
		view.Accounts = append(view.Accounts, accountAt(m, index, unresolved))
	}
	classifyInstruction(&view, compiled.Data)
	return view
}

// accountAt maps a compiled index onto the static account list, or onto the
// lookup tables that follow it. Only the static part can be named offline.
func accountAt(m types.Message, index int, unresolved *bool) AccountView {
	if index < len(m.Accounts) {
		static := len(m.Accounts)
		signers := int(m.Header.NumRequireSignatures)
		readonlySigned := int(m.Header.NumReadonlySignedAccounts)
		readonlyUnsigned := int(m.Header.NumReadonlyUnsignedAccounts)
		return AccountView{
			Address:  m.Accounts[index].ToBase58(),
			Signer:   index < signers,
			Writable: index < signers-readonlySigned || (index >= signers && index < static-readonlyUnsigned),
		}
	}
	offset := index - len(m.Accounts)
	for _, table := range m.AddressLookupTables {
		if offset < len(table.WritableIndexes) {
			*unresolved = true
			return AccountView{
				LookupKey: table.AccountKey.ToBase58(),
				LookupIdx: int(table.WritableIndexes[offset]),
				Writable:  true, Unresolved: true,
			}
		}
		offset -= len(table.WritableIndexes)
	}
	for _, table := range m.AddressLookupTables {
		if offset < len(table.ReadonlyIndexes) {
			*unresolved = true
			return AccountView{
				LookupKey:  table.AccountKey.ToBase58(),
				LookupIdx:  int(table.ReadonlyIndexes[offset]),
				Unresolved: true,
			}
		}
		offset -= len(table.ReadonlyIndexes)
	}
	*unresolved = true
	return AccountView{Unresolved: true}
}

func classifyInstruction(view *InstructionView, data []byte) {
	switch view.ProgramID {
	case common.SystemProgramID.ToBase58():
		classifySystemInstruction(view, data)
	case common.TokenProgramID.ToBase58(), token2022Program.ToBase58():
		classifyTokenInstruction(view, data)
	case computeBudgetProgram.ToBase58():
		view.Kind = KindComputeBudget
	default:
		view.Kind = KindUnknownProgram
	}
}

func classifySystemInstruction(view *InstructionView, data []byte) {
	if len(data) < 4 {
		view.Kind = KindUnknownProgram
		return
	}
	switch binary.LittleEndian.Uint32(data[:4]) {
	case 0:
		view.Kind = KindSystemCreateAccount
	case 1:
		view.Kind = KindSystemAssign
	case 2:
		view.Kind = KindSystemTransfer
		if len(data) >= 12 {
			view.Amount = strconv.FormatUint(binary.LittleEndian.Uint64(data[4:12]), 10)
		}
		if len(view.Accounts) > 1 {
			view.Recipient = view.Accounts[1].Address
		}
	case 4:
		view.Kind = KindAdvanceNonce
	default:
		view.Kind = KindUnknownProgram
	}
}

func classifyTokenInstruction(view *InstructionView, data []byte) {
	if len(data) < 1 {
		view.Kind = KindUnknownProgram
		return
	}
	switch data[0] {
	case 3: // Transfer
		view.Kind = KindSplTransfer
		if len(data) >= 9 {
			view.Amount = strconv.FormatUint(binary.LittleEndian.Uint64(data[1:9]), 10)
		}
		if len(view.Accounts) > 1 {
			view.Recipient = view.Accounts[1].Address
		}
	case 4: // Approve
		view.Kind = KindSplApprove
		if len(data) >= 9 {
			view.Amount = strconv.FormatUint(binary.LittleEndian.Uint64(data[1:9]), 10)
		}
		if len(view.Accounts) > 1 {
			view.Delegate = view.Accounts[1].Address
		}
	case 5: // Revoke
		view.Kind = KindSplRevoke
	case 6: // SetAuthority
		view.Kind = KindSplSetAuthority
		// layout: tag, authorityType, hasNewAuthority, [32-byte new authority]
		if len(data) >= 35 && data[2] == 1 {
			view.NewOwner = base58.Encode(data[3:35])
		}
	case 9: // CloseAccount
		view.Kind = KindSplCloseAccount
		if len(view.Accounts) > 1 {
			view.Recipient = view.Accounts[1].Address
		}
	case 12: // TransferChecked
		view.Kind = KindSplTransferChecked
		if len(data) >= 9 {
			view.Amount = strconv.FormatUint(binary.LittleEndian.Uint64(data[1:9]), 10)
		}
		if len(view.Accounts) > 2 {
			view.Recipient = view.Accounts[2].Address
		}
	case 13: // ApproveChecked
		view.Kind = KindSplApprove
		if len(data) >= 9 {
			view.Amount = strconv.FormatUint(binary.LittleEndian.Uint64(data[1:9]), 10)
		}
		if len(view.Accounts) > 2 {
			view.Delegate = view.Accounts[2].Address
		}
	default:
		view.Kind = KindUnknownProgram
	}
}

func isZeroSignature(sig types.Signature) bool {
	for _, b := range sig {
		if b != 0 {
			return false
		}
	}
	return true
}

func appendOnce(list []string, value string) []string {
	for _, existing := range list {
		if existing == value {
			return list
		}
	}
	return append(list, value)
}

func marshalSolDescription(d *TransactionDescription) string {
	if d.Warnings == nil {
		d.Warnings = []string{}
	}
	out, err := json.Marshal(d)
	if err != nil {
		return `{"version":"legacy","instructions":[],"warnings":[],"decodeError":"cannot encode description"}`
	}
	return string(out)
}
