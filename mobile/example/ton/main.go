package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/kernelflowlabs/wallet-lib/mobile/acc"
	"github.com/kernelflowlabs/wallet-lib/mobile/tx"
	tonrpc "github.com/kernelflowlabs/wallet-lib/rpc/ton"
	"github.com/kernelflowlabs/wallet-lib/signing"
)

const path = ""

const (
	mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	rpcURL   = "https://ton.org/global.config.json;https://tonapi.io"
)

func main() {
	ctx := context.Background()

	a, err := acc.NewTonFromMnemonic(mnemonic, path)
	if err != nil {
		log.Fatalf("derive account: %v", err)
	}
	sender := a.Address()
	recipient := sender
	amount := "100000000"
	fmt.Println("1. address from mnemonic:", sender)

	h, err := tonrpc.NewHandler(rpcURL)
	if err != nil {
		log.Fatalf("rpc handler: %v", err)
	}

	nonce := "0"
	activated, err := h.InquireChain(ctx, "getAddressActivated", sender)
	if err != nil {
		log.Fatalf("getAddressActivated: %v", err)
	}
	if activated == "true" {
		nonce, err = h.InquireChain(ctx, "getNonce", sender)
		if err != nil {
			log.Fatalf("getNonce: %v", err)
		}
	}
	fmt.Printf("2. fetched from rpc: activated=%s nonce=%s\n", activated, nonce)

	b := tx.NewTonTxBuilder().
		SetTxType(signing.TxTypeTransfer).
		SetContractAddress(signing.MagicContactAddressForNative).
		SetSender(sender).
		SetSenderPublicKey(hex.EncodeToString(a.PublicKey())).
		SetRecipient(recipient).
		SetAmount(amount).
		SetNonce(nonce).
		SetMemo("").
		SetFee("50000000")
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

	if _, err := h.SendTx(ctx, signedHex); err != nil {
		log.Fatalf("send tx: %v", err)
	}
	txHash := b.TxHash()
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
			fmt.Println("   verify ok: tx succeeded")
			return
		case signing.TxStatusFailed:
			log.Fatalf("   tx failed: %s", res.ErrMsg)
		default:
			fmt.Printf("   status=%s, polling...\n", res.Status)
		}
	}
	log.Fatal("timeout waiting for confirmation")
}
