package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/kernelflowlabs/wallet-lib/mobile/acc"
	"github.com/kernelflowlabs/wallet-lib/mobile/tx"
	solrpc "github.com/kernelflowlabs/wallet-lib/rpc/solana"
	"github.com/kernelflowlabs/wallet-lib/signing"
)

const path = "m/44'/501'/0'/0'"

const (
	mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	rpcURL   = "https://api.mainnet-beta.solana.com"
)

func main() {
	ctx := context.Background()

	a, err := acc.NewSolFromMnemonic(mnemonic, path)
	if err != nil {
		log.Fatalf("derive account: %v", err)
	}
	sender := a.Address()
	recipient := sender
	amount := "1"
	fmt.Println("1. address from mnemonic:", sender)

	h, err := solrpc.NewHandler(rpcURL, os.Getenv("HELIUS_API_KEY"))
	if err != nil {
		log.Fatalf("rpc handler: %v", err)
	}

	blockHash, err := h.InquireChain(ctx, "getLatestBlockHash", "")
	if err != nil {
		log.Fatalf("getLatestBlockHash: %v", err)
	}
	priorityFee, err := h.InquireChain(ctx, "getPriorityFee", "")
	if err != nil {
		log.Fatalf("getPriorityFee: %v", err)
	}
	fmt.Printf("2. fetched from rpc: blockHash=%s priorityFee=%s\n", blockHash, priorityFee)

	b := tx.NewSolTxBuilder().
		SetTxType(signing.TxTypeTransfer).
		SetContractAddress(signing.MagicContactAddressForNative).
		SetSender(sender).
		SetRecipient(recipient).
		SetAmount(amount).
		SetFee(priorityFee).
		SetRefBlockHash(blockHash)
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
		case signing.TxStatusFailed, signing.TxStatusDropped:
			log.Fatalf("   tx %s: %s", res.Status, res.ErrMsg)
		default:
			fmt.Printf("   status=%s, polling...\n", res.Status)
		}
	}
	log.Fatal("timeout waiting for confirmation")
}
