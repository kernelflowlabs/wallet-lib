package solana

import (
	"encoding/hex"
	"fmt"
	"github.com/kernelflowlabs/wallet-lib/crypto/key"
	"github.com/kernelflowlabs/wallet-lib/signing"
	"strconv"

	"github.com/blocto/solana-go-sdk/common"
	"github.com/blocto/solana-go-sdk/program/assotokenprog"
	"github.com/blocto/solana-go-sdk/program/compute_budget"
	"github.com/blocto/solana-go-sdk/program/sysprog"
	"github.com/blocto/solana-go-sdk/program/tokenprog"
	"github.com/blocto/solana-go-sdk/types"
	"github.com/btcsuite/btcd/address/v2/base58"
)

func NewTxBuilder(ti *Ingredient) *TxBuilder {
	return &TxBuilder{
		Ingredient: ti,
	}
}

func NewTxBuilderFromUnsignedHex(unsignedHex string) (*TxBuilder, error) {
	txBytes, err := hex.DecodeString(unsignedHex)
	if err != nil {
		return nil, fmt.Errorf("failed to decode unsignedHex, err=%v", err)
	}

	ntx, err := types.TransactionDeserialize(txBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to TransactionDeserialize, err=%v", err)
	}

	msgBytes, err := ntx.Message.Serialize()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize message, err=%v", err)
	}

	return &TxBuilder{
		Ingredient:      &Ingredient{},
		unsignedHex:     hex.EncodeToString(txBytes),
		sigHash:         []string{hex.EncodeToString(msgBytes)},
		fromUnsignedHex: true,
	}, nil
}

func (tx *TxBuilder) Build() error {
	if tx == nil {
		return fmt.Errorf("tx == nil")
	}
	tx.sigHash = nil
	tx.txHash = ""

	var ntx types.Transaction
	if tx.fromUnsignedHex {
		ntxBytes, err := hex.DecodeString(tx.unsignedHex)
		if err != nil {
			return fmt.Errorf("failed to DecodeString for UnsignedHex, err=%v", err)
		}
		ntx, err = types.TransactionDeserialize(ntxBytes)
		if err != nil {
			return fmt.Errorf("failed to TransactionDeserialize, err=%v", err)
		}
		tx.Ingredient = &Ingredient{}
		tx.RefBlockHash = ntx.Message.RecentBlockHash
	} else {
		if err := signing.Validator.Struct(tx.Ingredient); err != nil {
			return fmt.Errorf("invalid ingredient: %v", err)
		}
		senderPubkey := common.PublicKeyFromString(tx.Ingredient.Sender)

		var instructions []types.Instruction
		if tx.Ingredient.UseNonceAccount == "true" {
			if tx.Ingredient.NonceAccount == "" {
				return fmt.Errorf("empty nonce account")
			}
			noncePubkey := common.PublicKeyFromString(tx.Ingredient.NonceAccount)
			instructionAdvanceNonceAccount := sysprog.AdvanceNonceAccount(sysprog.AdvanceNonceAccountParam{
				Nonce: noncePubkey,
				Auth:  senderPubkey,
			})
			instructions = append(instructions, instructionAdvanceNonceAccount)
		}
		if tx.Ingredient.UnitPrice != "" && tx.Ingredient.UnitLimit != "" {
			unitPrice, err := strconv.ParseUint(tx.Ingredient.UnitPrice, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid unitPrice %q: %v", tx.Ingredient.UnitPrice, err)
			}
			unitLimit, err := strconv.ParseUint(tx.Ingredient.UnitLimit, 10, 32)
			if err != nil {
				return fmt.Errorf("invalid unitLimit %q: %v", tx.Ingredient.UnitLimit, err)
			}
			instructionUnitPrice := compute_budget.SetComputeUnitPrice(compute_budget.SetComputeUnitPriceParam{
				MicroLamports: unitPrice,
			})
			instructionUnitLimit := compute_budget.SetComputeUnitLimit(compute_budget.SetComputeUnitLimitParam{
				Units: uint32(unitLimit),
			})
			instructions = append(instructions, instructionUnitPrice)
			instructions = append(instructions, instructionUnitLimit)
		} else {
			return fmt.Errorf("UnitPrice or UnitLimit required")
		}
		var err error
		switch tx.Ingredient.TxType {
		case signing.TxTypeTransfer:
			if tx.Ingredient.ContractAddress == "" {
				return fmt.Errorf("contractAddress required")
			}
			if tx.Ingredient.Amount == "" {
				return fmt.Errorf("amount required")
			}
			if !ValidStrictAddress(tx.Ingredient.Recipient) {
				return fmt.Errorf("invalid recipient")
			}
			recipientPubkey := common.PublicKeyFromString(tx.Ingredient.Recipient)
			amount, err := strconv.ParseUint(tx.Ingredient.Amount, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid amount %q: %v", tx.Ingredient.Amount, err)
			}
			if IsNativeSentinel(tx.ContractAddress) {
				instructionTransfer := sysprog.Transfer(sysprog.TransferParam{
					From:   senderPubkey,
					To:     recipientPubkey,
					Amount: amount,
				})
				instructions = append(instructions, instructionTransfer)
			} else {
				is2022 := tx.Ingredient.Token2022 == "true"
				contractPubkey := common.PublicKeyFromString(tx.Ingredient.ContractAddress)
				senderTokenPubkey, _, err := FindAssociatedTokenAddressWithProgram(senderPubkey, contractPubkey, is2022)
				if err != nil {
					return fmt.Errorf("failed to find associated token address for sender, err=%v", err)
				}
				recipientTokenPubkey, _, err := FindAssociatedTokenAddressWithProgram(recipientPubkey, contractPubkey, is2022)
				if err != nil {
					return fmt.Errorf("failed to find associated token address for recipient, err=%v", err)
				}
				var instructionTokenTransfer types.Instruction
				if is2022 {
					decimals, err := strconv.ParseUint(tx.Ingredient.Decimals, 10, 8)
					if err != nil {
						return fmt.Errorf("decimals required for Token-2022 transfers, got %q", tx.Ingredient.Decimals)
					}
					instructionTokenTransfer = tokenprog.TransferChecked(tokenprog.TransferCheckedParam{
						From:     senderTokenPubkey,
						To:       recipientTokenPubkey,
						Mint:     contractPubkey,
						Auth:     senderPubkey,
						Amount:   amount,
						Decimals: uint8(decimals),
					})
				} else {
					instructionTokenTransfer = tokenprog.Transfer(tokenprog.TransferParam{
						From: senderTokenPubkey,
						To:   recipientTokenPubkey,
						Auth: senderPubkey,
						Signers: []common.PublicKey{
							senderPubkey,
						},
						Amount: amount,
					})
				}
				instructionTokenTransfer.ProgramID = TokenProgramOf(is2022)
				if tx.Ingredient.HasATA == "true" {
					instructions = append(instructions, instructionTokenTransfer)
				} else {
					instructionCreateAssociatedTokenAccount := assotokenprog.CreateAssociatedTokenAccount(
						assotokenprog.CreateAssociatedTokenAccountParam{
							Funder:                 senderPubkey,
							Owner:                  recipientPubkey,
							Mint:                   contractPubkey,
							AssociatedTokenAccount: recipientTokenPubkey,
						})
					for i := range instructionCreateAssociatedTokenAccount.Accounts {
						if instructionCreateAssociatedTokenAccount.Accounts[i].PubKey == common.TokenProgramID {
							instructionCreateAssociatedTokenAccount.Accounts[i].PubKey = TokenProgramOf(is2022)
						}
					}
					instructions = append(instructions, instructionCreateAssociatedTokenAccount)
					instructions = append(instructions, instructionTokenTransfer)
				}
			}
		case signing.TxTypeAccountActivate:
			if tx.Ingredient.MinimumBalanceForRentExemption == "" {
				return fmt.Errorf("MinimumBalanceForRentExemption required")
			}
			lamports, _ := strconv.ParseUint(tx.Ingredient.MinimumBalanceForRentExemption, 10, 64)
			noncePubkey := common.PublicKeyFromString(tx.Ingredient.NonceAccount)
			instructionAccountActivate := sysprog.CreateAccount(sysprog.CreateAccountParam{
				From:     senderPubkey,
				New:      noncePubkey,
				Owner:    common.SystemProgramID,
				Lamports: lamports,
				Space:    sysprog.NonceAccountSize,
			})
			instructionInitializeNonceAccount := sysprog.InitializeNonceAccount(sysprog.InitializeNonceAccountParam{
				Nonce: noncePubkey,
				Auth:  senderPubkey,
			})
			instructions = append(instructions, instructionAccountActivate)
			instructions = append(instructions, instructionInitializeNonceAccount)
		default:
			return fmt.Errorf("invalid TxType")
		}
		messageParam := types.NewMessageParam{
			FeePayer:        senderPubkey,
			RecentBlockhash: tx.Ingredient.RefBlockHash,
			Instructions:    instructions,
		}
		message := types.NewMessage(messageParam)
		transactionParams := types.NewTransactionParam{Message: message}
		ntx, err = types.NewTransaction(transactionParams)
		if err != nil {
			return fmt.Errorf("failed to build tx, err=%v", err)
		}
	}

	ntxBytes, err := ntx.Serialize()
	if err != nil {
		return fmt.Errorf("failed to Serialize ntx, err=%v", err)
	}
	msg, err := ntx.Message.Serialize()
	if err != nil {
		return fmt.Errorf("failed to Serialize ntx Message, err=%v", err)
	}
	tx.unsignedHex = hex.EncodeToString(ntxBytes)
	tx.sigHash = append(tx.sigHash, hex.EncodeToString(msg))
	return nil
}

func (tx *TxBuilder) Sign(privateKey []byte) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("tx == nil")
	} else if len(tx.sigHash) != 1 {
		return "", fmt.Errorf("tx.SigHash == nil")
	}
	if tx.Ingredient != nil && tx.Ingredient.Sender != "" {
		pub, err := key.PrivateKey2PublicKeyED25519(privateKey)
		if err != nil {
			return "", fmt.Errorf("failed to derive public key, err=%v", err)
		}
		signer, err := PublicKey2Address(pub)
		if err != nil {
			return "", fmt.Errorf("failed to derive signer address, err=%v", err)
		}
		if signer != tx.Ingredient.Sender {
			return "", fmt.Errorf("private key does not match sender %s", tx.Ingredient.Sender)
		}
	}
	sigHash, err := hex.DecodeString(tx.sigHash[0])
	if err != nil {
		return "", fmt.Errorf("failed to DecodeString for sigHash, err=%v", err)
	}
	signature, err := key.SignWithPrivateKeyED25519(privateKey, sigHash)
	if err != nil {
		return "", fmt.Errorf("failed to SignWithPrivateKeyED25519, err=%v", err)
	}
	return hex.EncodeToString(signature), nil
}

func (tx *TxBuilder) ConcatSignature(signature string, isDerFormat bool) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("tx == nil")
	} else if tx.unsignedHex == "" {
		return "", fmt.Errorf("tx.UnsignedHex == nil")
	}

	ntxBytes, err := hex.DecodeString(tx.unsignedHex)
	if err != nil {
		return "", fmt.Errorf("failed to DecodeString for UnsignedHex, err=%v", err)
	}
	ntx, err := types.TransactionDeserialize(ntxBytes)
	if err != nil {
		return "", fmt.Errorf("failed to TransactionDeserialize, err=%v", err)
	}

	if isDerFormat {
		return "", fmt.Errorf("DER signature format not supported")
	}
	sig, err := hex.DecodeString(signature)
	if err != nil {
		return "", fmt.Errorf("failed to DecodeString for signature, err=%v", err)
	}
	err = ntx.AddSignature(sig)
	if err != nil {
		return "", fmt.Errorf("failed to AddSignature for sig, err=%v", err)
	}

	if tx.Ingredient.TxType == signing.TxTypeAccountActivate {
		noncePrivateKeyBytes, err := hex.DecodeString(tx.Ingredient.NonceAccountPrivateKey)
		if err != nil {
			return "", fmt.Errorf("failed to DecodeString NonceAccountPrivateKey, err=%v", err)
		}
		sigHash, err := hex.DecodeString(tx.sigHash[0])
		if err != nil {
			return "", fmt.Errorf("failed to DecodeString for sigHash, err=%v", err)
		}
		nonceSig, err := key.SignWithPrivateKeyED25519(noncePrivateKeyBytes, sigHash)
		if err != nil {
			return "", fmt.Errorf("invalid NonceAccountPrivateKey, err=%v", err)
		}
		err = ntx.AddSignature(nonceSig)
		if err != nil {
			return "", fmt.Errorf("failed to AddSignature for nonceSig, err=%v", err)
		}
	}
	signedTxnBytes, err := ntx.Serialize()
	if err != nil {
		return "", fmt.Errorf("failed to Serialize, err=%v", err)
	}

	tx.txHash = base58.Encode(ntx.Signatures[0])

	return hex.EncodeToString(signedTxnBytes), nil
}

func (tx *TxBuilder) GetTxHash() string {
	return tx.txHash
}

func (tx *TxBuilder) GetSigHash() []string {
	return tx.sigHash
}

func (tx *TxBuilder) GetUnsignedHex() string {
	return tx.unsignedHex
}

func (tx *TxBuilder) SetSigHash(sigHash []string) {
	tx.sigHash = sigHash
}

func (tx *TxBuilder) SetUnsignedHex(unsignedHex string) {
	tx.unsignedHex = unsignedHex
}

// types
type (
	Ingredient struct {
		TxType                         string `json:"txType" validate:"required,oneof=0 1 2 3 4 5 6"`
		ContractAddress                string `json:"contractAddress,omitempty" validate:"omitempty,sol_addr"`
		Sender                         string `json:"sender" validate:"required,sol_addr"`
		Recipient                      string `json:"recipient,omitempty" validate:"omitempty,sol_addr"`
		Amount                         string `json:"amount,omitempty" validate:"omitempty,bigint_gt0"`
		UnitPrice                      string `json:"unitPrice,omitempty" validate:"omitempty,u64_gt0"`
		UnitLimit                      string `json:"unitLimit,omitempty" validate:"omitempty,u64_gt0"`
		HasATA                         string `json:"hasATA,omitempty" validate:"omitempty,bool_str"`
		Token2022                      string `json:"token2022,omitempty" validate:"omitempty,bool_str"`
		Decimals                       string `json:"decimals,omitempty" validate:"omitempty,u64"`
		RefBlockHash                   string `json:"refBlockHash,omitempty"`
		UseNonceAccount                string `json:"useNonceAccount,omitempty"`
		NonceAccount                   string `json:"nonceAccount,omitempty"`
		NonceAccountPrivateKey         string `json:"nonceAccountPrivateKey,omitempty"`
		MinimumBalanceForRentExemption string `json:"minimumBalanceForRentExemption,omitempty" validate:"omitempty,u64_gt0"`
	}

	TxBuilder struct {
		*Ingredient
		unsignedHex     string
		sigHash         []string
		txHash          string
		fromUnsignedHex bool
	}
)

func RecommendComputeBudget(maxFeeStr string, isToken bool) (string, string) {
	maxFee, err := strconv.ParseUint(maxFeeStr, 10, 64)
	if err != nil {
		return "100", "250000"
	}

	var unitPrice, unitLimit string
	if maxFee == 0 {
		unitPrice = "1"
	} else if maxFee < 100 {
		unitPrice = "100"
	} else if maxFee > 300000 {
		unitPrice = "300000"
	} else {
		unitPrice = maxFeeStr
	}
	if isToken {
		unitLimit = "220000"
	} else {
		unitLimit = "150000"
	}
	return unitPrice, unitLimit
}
