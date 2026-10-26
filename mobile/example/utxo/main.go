package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/kernelflowlabs/wallet-lib/mobile/acc"
	"github.com/kernelflowlabs/wallet-lib/mobile/tx"
	utxorpc "github.com/kernelflowlabs/wallet-lib/rpc/utxo"
	"github.com/kernelflowlabs/wallet-lib/signing"
)

const path = "m/44'/0'/0'/0/0"

const (
	mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	rpcURL   = "https://your-bitcoin-node.example&user@pass;https://your-scan-api.example"
	network  = "0"
)

func main() {
	ctx := context.Background()

	a, err := acc.NewUtxoFromMnemonic(mnemonic, path, network)
	if err != nil {
		log.Fatalf("derive account: %v", err)
	}
	sender := a.Address()
	recipient := sender
	amount := "1000"
	fmt.Println("1. address from mnemonic:", sender)

	h, err := utxorpc.NewHandler(rpcURL, network, os.Getenv("BLOCKCYPHER_TOKEN"))
	if err != nil {
		log.Fatalf("rpc handler: %v", err)
	}

	utxoJSON, err := h.InquireChain(ctx, "getUtxo", sender)
	if err != nil {
		log.Fatalf("getUtxo: %v", err)
	}
	byteFee, err := h.InquireChain(ctx, "getByteFee", "")
	if err != nil {
		log.Fatalf("getByteFee: %v", err)
	}
	fmt.Printf("2. fetched from rpc: byteFee=%s utxos=%s\n", byteFee, utxoJSON)

	utxos := tx.NewMobileUtxoList()
	if err := utxos.SerializeFromStr(utxoJSON); err != nil {
		log.Fatalf("parse utxos: %v", err)
	}

	b := tx.NewUtxoTxBuilder(network).
		SetTxType(signing.TxTypeTransfer).
		SetContractAddress(signing.MagicContactAddressForNative).
		SetSender(sender).
		SetSenderPublicKey(a.PublicKeyHex()).
		SetRecipient(recipient).
		SetAmount(amount).
		SetByteFee(byteFee).
		SetUtxos(utxos)
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
			fmt.Printf("   verify ok: tx succeeded at height %s, gasUsed %s\n", res.Height, res.GasUsed)
			return
		case signing.TxStatusFailed, signing.TxStatusDropped:
			log.Fatalf("   tx %s: %s", res.Status, res.ErrMsg)
		default:
			fmt.Printf("   status=%s, polling...\n", res.Status)
		}
	}
	log.Fatal("timeout waiting for confirmation")
}
