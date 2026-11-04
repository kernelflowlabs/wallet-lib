package solana

import (
	"encoding/json"
	"math/big"
)

type (
	BaseRequest struct {
		JsonRPC string      `json:"jsonrpc"`
		ID      uint64      `json:"id"`
		Method  string      `json:"method"`
		Params  interface{} `json:"params"`
	}
	BaseResponse struct {
		JsonRPC string `json:"jsonrpc"`
		ID      uint64 `json:"id"`
		Error   struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
	}
	Context struct {
		ApiVersion string `json:"apiVersion,omitempty"`
		Slot       uint64 `json:"slot"`
	}
	GetSlotRes struct {
		BaseResponse
		Result uint64 `json:"result"`
	}

	GetBlockHeightRes struct {
		BaseResponse
		Result uint64 `json:"result"`
	}

	GetBalanceRes struct {
		BaseResponse
		Result struct {
			Context *Context `json:"context"`
			Value   *big.Int `json:"value"`
		} `json:"result"`
	}

	AccountInfo struct {
		Data struct {
			Parsed struct {
				Info struct {
					Decimals        int    `json:"decimals"`
					FreezeAuthority string `json:"freezeAuthority"`
					IsInitialized   bool   `json:"isInitialized"`
					MintAuthority   string `json:"mintAuthority"`
					Supply          string `json:"supply"`

					Authority     string `json:"authority"`
					Blockhash     string `json:"blockhash"`
					FeeCalculator struct {
						LamportsPerSignature string `json:"lamportsPerSignature"`
					} `json:"feeCalculator"`
				} `json:"info"`
				Type string `json:"type"`
			} `json:"parsed"`
			Program string `json:"program"`
			Space   int    `json:"space"`
		} `json:"data,omitempty"`
		//Data1      []string `json:"data,omitempty"`
		Executable bool   `json:"executable"`
		Lamports   uint64 `json:"lamports"`
	}

	GetAccountInfoJsonRes struct {
		BaseResponse
		Result struct {
			Context *Context    `json:"context"`
			Value   AccountInfo `json:"value"`
		} `json:"result"`
	}

	GetTokenBalanceRes struct {
		BaseResponse
		Result struct {
			Context *Context `json:"context"`
			Value   struct {
				Amount   string  `json:"amount"`
				Decimals int     `json:"decimals"`
				UiAmount float64 `json:"uiAmount"`
			} `json:"value"`
		} `json:"result"`
	}

	GetLatestBlockhash struct {
		BaseResponse
		Result struct {
			Context struct {
				Slot int `json:"slot"`
			} `json:"context"`
			Value struct {
				Blockhash            string `json:"blockhash"`
				LastValidBlockHeight int    `json:"lastValidBlockHeight"`
			} `json:"value"`
		} `json:"result"`
	}
	IsBlockHashValidRes struct {
		BaseResponse
		Result struct {
			Context *Context `json:"context"`
			Value   bool     `json:"value"`
		} `json:"result"`
	}

	GetminimumbalanceforrentexemptionRes struct {
		BaseResponse
		Result uint64 `json:"result"`
	}

	SendTransactionRes struct {
		BaseResponse
		Result string `json:"result"`
	}

	GetSignatureStatus struct {
		Slot               uint64      `json:"slot"`
		Confirmations      uint64      `json:"confirmations"`
		Err                interface{} `json:"err"`
		ConfirmationStatus string      `json:"confirmationStatus"` //finalized, confirmed, processed
	}
	GetSignatureStatusRes struct {
		BaseResponse
		Result struct {
			Context *Context              `json:"context"`
			Value   []*GetSignatureStatus `json:"value"`
		} `json:"result"`
	}
	GetTokenAccountsByOwnerResponse struct {
		BaseResponse
		Result struct {
			Context *Context                   `json:"context"`
			Value   []*GetTokenAccountsByOwner `json:"value"`
		} `json:"result"`
	}
	GetTokenAccountsByOwner struct {
		Pubkey  string `json:"pubkey"`
		Account struct {
			Data struct {
				Parsed struct {
					Info struct {
						IsNative    bool   `json:"isNative"`
						Mint        string `json:"mint"`
						Owner       string `json:"owner"`
						State       string `json:"state"`
						TokenAmount struct {
							Amount   string  `json:"amount"`
							Decimals int     `json:"decimals"`
							UiAmount float64 `json:"uiAmount"`
						} `json:"tokenAmount"`
					} `json:"info"`
					Type string `json:"type"`
				} `json:"parsed"`
				Program string `json:"program"`
				Space   uint64 `json:"space"`
			} `json:"data"`
			Executable bool   `json:"executable"`
			Lamports   uint64 `json:"lamports"`
			Owner      string `json:"owner"`
			RentEpoch  uint64 `json:"rentEpoch"`
		} `json:"account"`
	}
	GetTransaction struct {
		BaseResponse
		Result struct {
			BlockTime int `json:"blockTime"`
			Meta      struct {
				ComputeUnitsConsumed int           `json:"computeUnitsConsumed"`
				Err                  interface{}   `json:"err"`
				Fee                  int           `json:"fee"`
				InnerInstructions    []interface{} `json:"innerInstructions"`
				LogMessages          []string      `json:"logMessages"`
				PostBalances         []int         `json:"postBalances"`
				PostTokenBalances    []struct {
					AccountIndex  int    `json:"accountIndex"`
					Mint          string `json:"mint"`
					Owner         string `json:"owner"`
					ProgramID     string `json:"programId"`
					UITokenAmount struct {
						Amount         string  `json:"amount"`
						Decimals       int     `json:"decimals"`
						UIAmount       float64 `json:"uiAmount"`
						UIAmountString string  `json:"uiAmountString"`
					} `json:"uiTokenAmount"`
				} `json:"postTokenBalances"`
				PreBalances      []int `json:"preBalances"`
				PreTokenBalances []struct {
					AccountIndex  int    `json:"accountIndex"`
					Mint          string `json:"mint"`
					Owner         string `json:"owner"`
					ProgramID     string `json:"programId"`
					UITokenAmount struct {
						Amount         string  `json:"amount"`
						Decimals       int     `json:"decimals"`
						UIAmount       float64 `json:"uiAmount"`
						UIAmountString string  `json:"uiAmountString"`
					} `json:"uiTokenAmount"`
				} `json:"preTokenBalances"`
				Rewards []interface{} `json:"rewards"`
				Status  struct {
					Ok interface{} `json:"Ok"`
				} `json:"status"`
			} `json:"meta"`
			Slot        int `json:"slot"`
			Transaction struct {
				Message struct {
					AccountKeys []struct {
						Pubkey   string `json:"pubkey"`
						Signer   bool   `json:"signer"`
						Source   string `json:"source"`
						Writable bool   `json:"writable"`
					} `json:"accountKeys"`
					//AccountKeys  []string `json:"accountKeys"`
					Instructions []struct {
						Accounts    []interface{}   `json:"accounts,omitempty"`
						Data        string          `json:"data,omitempty"`
						ProgramID   string          `json:"programId"`
						StackHeight interface{}     `json:"stackHeight"`
						Parsed      json.RawMessage `json:"parsed"`
						Program     string          `json:"program,omitempty"`
					} `json:"instructions"`
					RecentBlockhash string `json:"recentBlockhash"`
				} `json:"message"`
				Signatures []string `json:"signatures"`
			} `json:"transaction"`
		} `json:"result"`
		ID int `json:"id"`
	}
	GetTransactionWithLog struct {
		BaseResponse
		Result *struct {
			BlockTime int `json:"blockTime"`
			Meta      struct {
				ComputeUnitsConsumed int           `json:"computeUnitsConsumed"`
				Err                  interface{}   `json:"err"`
				Fee                  int           `json:"fee"`
				InnerInstructions    []interface{} `json:"innerInstructions"`
				LoadedAddresses      struct {
					Writable []string `json:"writable"`
					Readonly []string `json:"readonly"`
				} `json:"loadedAddresses"`
				LogMessages       []string `json:"logMessages"`
				PostBalances      []uint64 `json:"postBalances"`
				PostTokenBalances []struct {
					AccountIndex  int    `json:"accountIndex"`
					Mint          string `json:"mint"`
					Owner         string `json:"owner"`
					ProgramID     string `json:"programId"`
					UITokenAmount struct {
						Amount         string  `json:"amount"`
						Decimals       int     `json:"decimals"`
						UIAmount       float64 `json:"uiAmount"`
						UIAmountString string  `json:"uiAmountString"`
					} `json:"uiTokenAmount"`
				} `json:"postTokenBalances"`
				PreBalances      []uint64 `json:"preBalances"`
				PreTokenBalances []struct {
					AccountIndex  int    `json:"accountIndex"`
					Mint          string `json:"mint"`
					Owner         string `json:"owner"`
					ProgramID     string `json:"programId"`
					UITokenAmount struct {
						Amount         string  `json:"amount"`
						Decimals       int     `json:"decimals"`
						UIAmount       float64 `json:"uiAmount"`
						UIAmountString string  `json:"uiAmountString"`
					} `json:"uiTokenAmount"`
				} `json:"preTokenBalances"`
				Rewards []interface{} `json:"rewards"`
				Status  struct {
					Ok interface{} `json:"Ok"`
				} `json:"status"`
			} `json:"meta"`
			Slot        int `json:"slot"`
			Transaction struct {
				Message struct {
					AccountKeys  []string `json:"accountKeys"`
					Instructions []struct {
						Accounts    []interface{}   `json:"accounts,omitempty"`
						Data        string          `json:"data,omitempty"`
						ProgramID   string          `json:"programId"`
						StackHeight interface{}     `json:"stackHeight"`
						Parsed      json.RawMessage `json:"parsed"`
						Program     string          `json:"program,omitempty"`
					} `json:"instructions"`
					RecentBlockhash string `json:"recentBlockhash"`
				} `json:"message"`
				Signatures []string `json:"signatures"`
			} `json:"transaction"`
		} `json:"result"`
		ID int `json:"id"`
	}
	GetPrioritizationFee struct {
		BaseResponse
		Result []PrioritizationFeeItem `json:"result"`
	}
	PrioritizationFeeItem struct {
		Slot              uint64 `json:"slot"`
		PrioritizationFee uint64 `json:"prioritizationFee"`
	}

	TransferInfo struct {
		Info struct {
			Amount            string `json:"amount"`
			Lamports          int    `json:"lamports"`
			Authority         string `json:"authority"`
			MultisigAuthority string `json:"multisigAuthority"`
			Destination       string `json:"destination"`
			Source            string `json:"source"`
		} `json:"info"`
		Type string `json:"type"`
	}

	GetBlockByNumberRes struct {
		BaseResponse
		Result struct {
			BlockHeight       int64   `json:"blockHeight"`
			BlockTime         int64   `json:"blockTime"`
			Blockhash         string  `json:"blockhash"`
			ParentSlot        int64   `json:"parentSlot"`
			PreviousBlockhash string  `json:"previousBlockhash"`
			Transactions      []RpcTx `json:"transactions"`
		} `json:"result"`
		ID int `json:"id"`
	}

	SPLTokenTransfer struct {
		AccountIndex  int    `json:"accountIndex"`
		Mint          string `json:"mint"`
		Owner         string `json:"owner"`
		UiTokenAmount struct {
			Amount         string  `json:"amount"`
			Decimals       int     `json:"decimals"`
			UiAmount       float64 `json:"uiAmount"`
			UiAmountString string  `json:"uiAmountString"`
		} `json:"uiTokenAmount"`
	}
	RpcTx struct {
		Meta struct {
			ComputeUnitsConsumed int           `json:"computeUnitsConsumed"`
			Err                  interface{}   `json:"err"`
			Fee                  int           `json:"fee"`
			InnerInstructions    []interface{} `json:"innerInstructions"`
			LoadedAddresses      struct {
				Readonly []interface{} `json:"readonly"`
				Writable []interface{} `json:"writable"`
			} `json:"loadedAddresses"`
			LogMessages       []string            `json:"logMessages"`
			PreBalances       []uint64            `json:"preBalances"`
			PostBalances      []uint64            `json:"postBalances"`
			PreTokenBalances  []*SPLTokenTransfer `json:"preTokenBalances"`
			PostTokenBalances []*SPLTokenTransfer `json:"postTokenBalances"`
			Rewards           interface{}         `json:"rewards"`
			Status            struct {
				Ok interface{} `json:"Ok"`
			} `json:"status"`
		} `json:"meta"`
		Transaction struct {
			Message struct {
				AccountKeys []string `json:"accountKeys"`
				Header      struct {
					NumReadonlySignedAccounts   int `json:"numReadonlySignedAccounts"`
					NumReadonlyUnsignedAccounts int `json:"numReadonlyUnsignedAccounts"`
					NumRequiredSignatures       int `json:"numRequiredSignatures"`
				} `json:"header"`
				Instructions []struct {
					Accounts       []int       `json:"accounts"`
					Data           string      `json:"data"`
					ProgramIDIndex int         `json:"programIdIndex"`
					StackHeight    interface{} `json:"stackHeight"`
				} `json:"instructions"`
				RecentBlockhash string `json:"recentBlockhash"`
			} `json:"message"`
			Signatures []string `json:"signatures"`
		} `json:"transaction"`
		Version interface{} `json:"version"`
	}
	RpcBlock struct {
		BlockTime    int64   `json:"blockTime"`
		Transactions []RpcTx `json:"transactions"`
	}
)

type (
	SendTransactionConfig struct {
		SkipPreflight       bool   `json:"skipPreflight"` // default: false
		MaxRetries          int64  `json:"maxRetries"`
		PreflightCommitment string `json:"preflightCommitment"` // default: max
		Encoding            string `json:"encoding"`            // base58 or base64
	}
	SearchTransactionHistory struct {
		SearchTransactionHistory bool `json:"searchTransactionHistory"`
	}
	SearchTransaction struct {
		Commitment                     string `json:"commitment"`
		MaxSupportedTransactionVersion int64  `json:"maxSupportedTransactionVersion"`
	}
)

type (
	HeliusTokenMetaReq struct {
		MintAccounts []string `json:"mintAccounts"`
	}
	HeliusTokenMetaResItem struct {
		Account            string `json:"account"`
		OnChainAccountInfo struct {
			AccountInfo struct {
				Key        string `json:"key"`
				IsSigner   bool   `json:"isSigner"`
				IsWritable bool   `json:"isWritable"`
				Lamports   int64  `json:"lamports"`
				Data       struct {
					Parsed struct {
						Info struct {
							Decimals        int    `json:"decimals"`
							FreezeAuthority string `json:"freezeAuthority"`
							IsInitialized   bool   `json:"isInitialized"`
							MintAuthority   string `json:"mintAuthority"`
							Supply          string `json:"supply"`
						} `json:"info"`
						Type string `json:"type"`
					} `json:"parsed"`
					Program string `json:"program"`
					Space   int    `json:"space"`
				} `json:"data"`
				Owner      string  `json:"owner"`
				Executable bool    `json:"executable"`
				RentEpoch  float64 `json:"rentEpoch"`
			} `json:"accountInfo"`
			Error string `json:"error"`
		} `json:"onChainAccountInfo"`
		OnChainMetadata struct {
			Metadata struct {
				TokenStandard   string `json:"tokenStandard"`
				Key             string `json:"key"`
				UpdateAuthority string `json:"updateAuthority"`
				Mint            string `json:"mint"`
				Data            struct {
					Name                 string      `json:"name"`
					Symbol               string      `json:"symbol"`
					Uri                  string      `json:"uri"`
					SellerFeeBasisPoints int         `json:"sellerFeeBasisPoints"`
					Creators             interface{} `json:"creators"`
				} `json:"data"`
				PrimarySaleHappened bool `json:"primarySaleHappened"`
				IsMutable           bool `json:"isMutable"`
				EditionNonce        int  `json:"editionNonce"`
				Uses                struct {
					UseMethod string `json:"useMethod"`
					Remaining int    `json:"remaining"`
					Total     int    `json:"total"`
				} `json:"uses"`
				Collection        interface{} `json:"collection"`
				CollectionDetails interface{} `json:"collectionDetails"`
			} `json:"metadata"`
			Error string `json:"error"`
		} `json:"onChainMetadata"`
		LegacyMetadata struct {
			ChainId    int      `json:"chainId"`
			Address    string   `json:"address"`
			Symbol     string   `json:"symbol"`
			Name       string   `json:"name"`
			Decimals   int      `json:"decimals"`
			LogoURI    string   `json:"logoURI"`
			Tags       []string `json:"tags"`
			Extensions struct {
				CoingeckoId string `json:"coingeckoId"`
				SerumV3Usdc string `json:"serumV3Usdc"`
				Website     string `json:"website"`
			} `json:"extensions"`
		} `json:"legacyMetadata"`
	}
	HeliusTokenMetaMainReq struct {
		Jsonrpc string `json:"jsonrpc"`
		Id      string `json:"id"`
		Method  string `json:"method"`
		Params  struct {
			Id             string `json:"id"`
			DisplayOptions struct {
				ShowFungible bool `json:"showFungible"`
			} `json:"displayOptions"`
		} `json:"params"`
	}
	HeliusTokenMetaMainRes struct {
		Jsonrpc string `json:"jsonrpc"`
		Result  struct {
			Content struct {
				Metadata struct {
					Name   string `json:"name"`
					Symbol string `json:"symbol"`
				} `json:"metadata"`
			} `json:"content"`
			TokenInfo struct {
				Decimals int64 `json:"decimals"`
			} `json:"token_info"`
		} `json:"result"`
	}
)
