package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/kernelflowlabs/wallet-lib/mobile/acc"
	"github.com/kernelflowlabs/wallet-lib/mobile/tx"
	stellarrpc "github.com/kernelflowlabs/wallet-lib/rpc/stellar"
	"github.com/kernelflowlabs/wallet-lib/signing"
)

const path = "m/44'/148'/0"

const (
	mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	rpcURL   = "https://horizon.stellar.org"
)

func main() {
	ctx := context.Background()

	a, err := acc.NewStellarFromMnemonic(mnemonic, path)
	if err != nil {
		log.Fatalf("derive account: %v", err)
	}
	sender := a.Address()
	recipient := sender
	amount := "247040000000"
	fmt.Println("1. address from mnemonic:", sender)

	h, err := stellarrpc.NewHandler(rpcURL)
	if err != nil {
		log.Fatalf("rpc handler: %v", err)
	}

	activated, err := h.InquireChain(ctx, "IsAccountActivated", sender)
	if err != nil {
		log.Fatalf("IsAccountActivated: %v", err)
	}
	sequence := "0"
	if activated == "true" {
		sequence, err = h.InquireChain(ctx, "getAccountSequence", sender)
		if err != nil {
			log.Fatalf("getAccountSequence: %v", err)
		}
	}
	fmt.Printf("2. fetched from rpc: activated=%s sequence=%s\n", activated, sequence)

	b := tx.NewStellarTxBuilder().
		SetTxType(signing.TxTypeTransfer).
		SetContractAddress(signing.MagicContactAddressForNative).
		SetSender(sender).
		SetRecipient(recipient).
		SetAmount(amount).
		SetSequence(sequence).
		SetIsRecipientActivated(activated)
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
			fmt.Printf("   verify ok: tx succeeded at ledger %s\n", res.Height)
			return
		case signing.TxStatusFailed:
			log.Fatalf("   tx failed: %s", res.ErrMsg)
		default:
			fmt.Printf("   status=%s, polling...\n", res.Status)
		}
	}
	log.Fatal("timeout waiting for confirmation")
}
