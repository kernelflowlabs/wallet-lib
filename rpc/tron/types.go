package tron

import (
	"math/big"

	wallettron "github.com/kernelflowlabs/wallet-lib/signing/tron"
)

const SignatureTransferMethod = "a9059cbb"

const TransferEventTopic = "ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"

type (
	GetAccountReq struct {
		Address string `json:"address"`
	}
	GetAccountRes struct {
		Address    string `json:"address"`
		Balance    uint64 `json:"balance"`
		CreateTime int64  `json:"create_time"`
	}

	GetAccountResourceIn struct {
		Address string `json:"address"`
		Visible bool   `json:"visible"`
	}
	GetAccountResourceOut struct {
		FreeNetUsed  int64 `json:"freeNetUsed"`
		FreeNetLimit int64 `json:"freeNetLimit"`
	}

	GetTransactionInfoByIdReq struct {
		Value string `json:"value"`
	}
	GetTransactionInfoByIdRes struct {
		Result         string `json:"result"`
		TxID           string `json:"id"`
		BlockNumber    uint64 `json:"blockNumber"`
		BlockTimeStamp int64  `json:"blockTimeStamp"`
		Receipt        struct {
			NetUsage uint64 `json:"net_usage"`
			Result   string `json:"result"`
		} `json:"receipt"`
		Log []struct {
			Address string   `json:"address"`
			Topics  []string `json:"topics"`
			Data    string   `json:"data"`
		} `json:"log"`
	}

	SmartContractReq struct {
		OwnerAddress     string   `json:"owner_address"`
		ContractAddress  string   `json:"contract_address"`
		FunctionSelector string   `json:"function_selector"`
		Parameter        string   `json:"parameter"`
		FeeLimit         int64    `json:"fee_limit"`
		CallValue        *big.Int `json:"call_value"`
		Visible          bool     `json:"visible"`
	}

	ConstantSmartContractRes struct {
		Result struct {
			Result bool `json:"result"`
		} `json:"result"`
		ConstantResult []string `json:"constant_result"`
	}

	BlockRequest struct {
		StartNum uint64 `json:"startNum"`
		EndNum   uint64 `json:"endNum"`
	}

	Blocks struct {
		Blocks []Block `json:"block"`
	}

	Block struct {
		BlockId     string `json:"blockID"`
		Txs         []Tx   `json:"transactions"`
		BlockHeader struct {
			Data BlockData `json:"raw_data"`
		} `json:"block_header"`
	}

	BlockData struct {
		Number         uint64 `json:"number"`
		TxTrieRoot     string `json:"txTrieRoot"`
		WitnessAddress string `json:"witness_address"`
		ParentHash     string `json:"parentHash"`
		Version        int    `json:"version"`
		Timestamp      int64  `json:"timestamp"`
	}
	Tx struct {
		Ret       []TxRet  `json:"ret,omitempty"`
		Signature []string `json:"signature,omitempty"`
		ID        string   `json:"txID,omitempty"`
		BlockTime int64    `json:"block_timestamp,omitempty"`
		Data      TxData   `json:"raw_data,omitempty"`
		Visible   bool     `json:"visible,omitempty"`
	}

	TxRet struct {
		ContractRet string `json:"contractRet"`
	}

	TxData struct {
		Contracts     []TRXContract `json:"contract"`
		RefBlockBytes string        `json:"ref_block_bytes,omitempty"`
		RefBlockHash  string        `json:"ref_block_hash,omitempty"`
		Expiration    int64         `json:"expiration,omitempty"`
		FeeLimit      int64         `json:"fee_limit,omitempty"`
		Timestamp     int64         `json:"timestamp"`
	}

	TRXContract struct {
		Type      string `json:"type"`
		Parameter struct {
			Value   TransferValue `json:"value"`
			TypeUrl string        `json:"type_url"`
		} `json:"parameter"`
	}
	TransferValue struct {
		OwnerAddress    string   `json:"owner_address,omitempty"`
		ToAddress       string   `json:"to_address,omitempty"`
		Data            string   `json:"data,omitempty"`
		ContractAddress string   `json:"contract_address,omitempty"`
		Amount          *big.Int `json:"amount,omitempty"`
		CallValue       *big.Int `json:"call_value,omitempty"`
		Visible         bool     `json:"visible"`
	}

	BroadcastJsonRawTransactionReq struct {
		Signature []string          `json:"signature,omitempty"`
		ID        string            `json:"txID,omitempty"`
		RawData   *wallettron.TxRaw `json:"raw_data,omitempty"`
	}
	BroadcastJsonRawTransactionRes struct {
		Result  bool   `json:"result,omitempty"`
		Code    string `json:"code,omitempty"`
		TxID    string `json:"txid,omitempty"`
		Message string `json:"message,omitempty"`
	}

	BroadcastProtoRawTransactionReq struct {
		Transaction string `json:"transaction"`
	}
	BroadcastProtoRawTransactionRes struct {
		Result      bool   `json:"result"`
		Code        string `json:"code"`
		Error       string `json:"error"`
		TxID        string `json:"txID"`
		Message     string `json:"message"`
		Transaction string `json:"transaction"`
	}
	TriggerSmartContractReq struct {
		ContractAddress []byte
		Data            []byte
		Owner           []byte
		BlockNumber     int64
	}

	RpcBlock struct {
		BlockId     string `json:"blockID"`
		Txs         []Tx   `json:"transactions"`
		BlockHeader struct {
			Data BlockData `json:"raw_data"`
		} `json:"block_header"`
	}
)

type (
	GetChainParametersOut struct {
		ChainParameter []struct {
			Key   string `json:"key"`
			Value int64  `json:"value"`
		} `json:"chainParameter"`
	}
	ChainParameters struct {
		EnergyFeeSun                string `json:"energyFeeSun"`
		TransactionFeeSun           string `json:"transactionFeeSun"`
		MaxFeeLimitSun              string `json:"maxFeeLimitSun"`
		CreateAccountFeeSun         string `json:"createAccountFeeSun"`
		ContractCreateAccountFeeSun string `json:"contractCreateAccountFeeSun"`
	}
	AccountActivation struct {
		Address                string `json:"address"`
		Activated              bool   `json:"activated"`
		NativeTransferFeeSun   string `json:"nativeTransferFeeSun"`
		ContractTransferFeeSun string `json:"contractTransferFeeSun"`
	}
)
