package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/kernelflowlabs/wallet-lib/mobile/acc"
	"github.com/kernelflowlabs/wallet-lib/mobile/tx"
	nearrpc "github.com/kernelflowlabs/wallet-lib/rpc/near"
	"github.com/kernelflowlabs/wallet-lib/signing"
)

const path = "m/44'/397'/0"

const (
	mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	rpcURL   = "https://rpc.mainnet.near.org"
)

func main() {
	ctx := context.Background()

	a, err := acc.NewNearFromMnemonic(mnemonic, path)
	if err != nil {
		log.Fatalf("derive account: %v", err)
	}
	sender := a.Address()
	pubHex := hex.EncodeToString(a.PublicKey())
	recipient := sender
	amount := "1000000000000000000000000"
	fmt.Println("1. address (implicit account) from mnemonic:", sender)

	h, err := nearrpc.NewHandler(rpcURL)
	if err != nil {
		log.Fatalf("rpc handler: %v", err)
	}

	blockHash, err := h.InquireChain(ctx, "getRefBlockHash", "")
	if err != nil {
		log.Fatalf("getRefBlockHash: %v", err)
	}
	nonce, err := h.InquireChain(ctx, "getNonce", pubHex+":"+sender)
	if err != nil {

		nonce = "1"
		fmt.Printf("2. account not activated (%v); using nonce=1 to demo build/sign\n", err)
	} else {
		fmt.Printf("2. fetched from rpc: nonce=%s blockHash=%s\n", nonce, blockHash)
	}

	b := tx.NewNearTxBuilder().
		SetTxType(signing.TxTypeTransfer).
		SetContractAddress(signing.MagicContactAddressForNative).
		SetSender(sender).
		SetSenderPublicKey(pubHex).
		SetRecipient(recipient).
		SetAmount(amount).
		SetNonce(nonce).
		SetBlockHash(blockHash)
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
		res, err := h.CheckTx(ctx, txHash+":"+sender)
		if err != nil {
			continue
		}
		switch res.Status {
		case signing.TxStatusSucceeded:
			fmt.Printf("   verify ok: tx succeeded at height %s\n", res.Height)
			return
		case signing.TxStatusFailed:
			log.Fatalf("   tx failed")
		default:
			fmt.Printf("   status=%s, polling...\n", res.Status)
		}
	}
	log.Fatal("timeout waiting for confirmation")
}
