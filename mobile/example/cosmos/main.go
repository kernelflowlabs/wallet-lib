package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/kernelflowlabs/wallet-lib/mobile/acc"
	"github.com/kernelflowlabs/wallet-lib/mobile/tx"
	cosmosrpc "github.com/kernelflowlabs/wallet-lib/rpc/cosmos"
	"github.com/kernelflowlabs/wallet-lib/signing"
)

const path = "m/44'/118'/0'/0/0"

const (
	mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	rpcURL   = "https://cosmos-rest.publicnode.com"
	network  = "cosmoshub-4"
)

func main() {
	ctx := context.Background()

	a, err := acc.NewCosmosFromMnemonic(mnemonic, path, network)
	if err != nil {
		log.Fatalf("derive account: %v", err)
	}
	sender := a.Address()
	recipient := sender
	amount := "1"
	fmt.Println("1. address from mnemonic:", sender)

	h, err := cosmosrpc.NewHandler(rpcURL, network)
	if err != nil {
		log.Fatalf("rpc handler: %v", err)
	}

	accInfoStr, err := h.InquireChain(ctx, "getAccountInfo", sender)
	if err != nil {
		log.Fatalf("getAccountInfo: %v", err)
	}
	var accInfo struct {
		AccountNumber string `json:"accountNumber"`
		Sequence      string `json:"sequence"`
	}
	if err := json.Unmarshal([]byte(accInfoStr), &accInfo); err != nil {
		log.Fatalf("parse account info: %v", err)
	}
	fmt.Printf("2. fetched from rpc: accountNumber=%s sequence=%s\n", accInfo.AccountNumber, accInfo.Sequence)

	b := tx.NewCosmosTxBuilder(network).
		SetTxType(signing.TxTypeTransfer).
		SetContractAddress(signing.MagicContactAddressForNative).
		SetSender(sender).
		SetSenderPublicKey(hex.EncodeToString(a.PublicKey())).
		SetRecipient(recipient).
		SetAmount(amount).
		SetFeeAmount("3000").
		SetGasLimit("200000").
		SetAccountNumber(accInfo.AccountNumber).
		SetSequence(accInfo.Sequence)
	if err := b.Build(); err != nil {
		log.Fatalf("build: %v", err)
	}
	fmt.Println("3. built tx, sigHash:", b.SigHash())

	sig, err := b.Sign(a.PrivateKey())
	if err != nil {
		log.Fatalf("sign: %v", err)
	}
	signedHex, err := b.ConcatSignature(sig)
	if err != nil {
		log.Fatalf("concat signature: %v", err)
	}
	fmt.Println("4. signed raw tx:", signedHex)

	txHash, err := h.SendTx(ctx, signedHex)
	if err != nil {
		log.Fatalf("send tx: %v", err)
	}
	fmt.Println("5. broadcasted, txHash:", txHash)

	fmt.Println("6. waiting for confirmation...")
	for i := 0; i < 40; i++ {
		time.Sleep(3 * time.Second)
		res, err := h.CheckTx(ctx, txHash)
		if err != nil {
			continue
		}
		switch res.Status {
		case signing.TxStatusSucceeded:
			fmt.Printf("   verify ok: tx succeeded at height %s\n", res.Height)
			return
		case signing.TxStatusFailed:
			log.Fatalf("   tx %s: %s", res.Status, res.ErrMsg)
		default:
			fmt.Printf("   status=%s, polling...\n", res.Status)
		}
	}
	log.Fatal("timeout waiting for confirmation")
}
