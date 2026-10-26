package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/kernelflowlabs/wallet-lib/mobile/acc"
	"github.com/kernelflowlabs/wallet-lib/mobile/tx"
	algorandrpc "github.com/kernelflowlabs/wallet-lib/rpc/algorand"
	"github.com/kernelflowlabs/wallet-lib/signing"
)

const path = "m/44'/283'/0'/0/0"

const (
	mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	rpcURL   = "https://mainnet-api.algonode.cloud"
)

func main() {
	ctx := context.Background()

	a, err := acc.NewAlgorandFromMnemonic(mnemonic, path)
	if err != nil {
		log.Fatalf("derive account: %v", err)
	}
	sender := a.Address()
	recipient := sender
	amount := "300000"
	fmt.Println("1. address from mnemonic:", sender)

	h, err := algorandrpc.NewHandler(rpcURL)
	if err != nil {
		log.Fatalf("rpc handler: %v", err)
	}

	paramsStr, err := h.InquireChain(ctx, "transactionsParams", "")
	if err != nil {
		log.Fatalf("transactionsParams: %v", err)
	}
	var params struct {
		GenesisID   string `json:"genesisID"`
		GenesisHash string `json:"genesisHash"`
		FirstValid  string `json:"firstValid"`
	}
	if err := json.Unmarshal([]byte(paramsStr), &params); err != nil {
		log.Fatalf("parse params: %v", err)
	}
	fee, err := h.InquireChain(ctx, "getMinFee", "")
	if err != nil {
		log.Fatalf("getMinFee: %v", err)
	}
	fmt.Printf("2. fetched from rpc: firstValid=%s genesisID=%s fee=%s\n", params.FirstValid, params.GenesisID, fee)

	b := tx.NewAlgorandTxBuilder().
		SetTxType(signing.TxTypeTransfer).
		SetContractAddress(signing.MagicContactAddressForNative).
		SetSender(sender).
		SetRecipient(recipient).
		SetAmount(amount).
		SetFee(fee).
		SetGenesisID(params.GenesisID).
		SetGenesisHash(params.GenesisHash).
		SetFirstValid(params.FirstValid)
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
	fmt.Println("5. broadcasted, txId:", txHash)

	fmt.Println("6. waiting for confirmation...")
	for i := 0; i < 40; i++ {
		time.Sleep(3 * time.Second)
		res, err := h.CheckTx(ctx, txHash)
		if err != nil {
			continue
		}
		switch res.Status {
		case signing.TxStatusSucceeded:
			fmt.Printf("   verify ok: tx succeeded at round %s\n", res.Height)
			return
		case signing.TxStatusFailed:
			log.Fatalf("   tx failed: %s", res.ErrMsg)
		default:
			fmt.Printf("   status=%s, polling...\n", res.Status)
		}
	}
	log.Fatal("timeout waiting for confirmation")
}
