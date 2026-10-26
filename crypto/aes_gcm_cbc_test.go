package crypto

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestAESGCMKnownVector(t *testing.T) {
	key, nonce, plain := make([]byte, 16), make([]byte, 12), make([]byte, 16)
	want, _ := hex.DecodeString("0388dace60b6a392f328c2b971b2fe78ab6e47d42cec13bdf53a67b21257bddf")
	got, err := AesEncryptGCM(plain, key, nonce, nil)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("vector mismatch: %v", err)
	}
	opened, err := AesDecryptGCM(want, key, nonce, nil)
	if err != nil || !bytes.Equal(opened, plain) {
		t.Fatalf("decrypt: %v", err)
	}
	for _, position := range []int{0, len(got) - 1} {
		bad := bytes.Clone(got)
		bad[position] ^= 1
		if out, err := AesDecryptGCM(bad, key, nonce, nil); err == nil || out != nil {
			t.Fatal("tampered ciphertext accepted")
		}
	}
	if _, err := AesDecryptGCM(got, key, nonce, []byte("other")); err == nil {
		t.Fatal("wrong AAD accepted")
	}
}

func TestAESRejectsInvalidInputs(t *testing.T) {
	for _, keyLen := range []int{0, 15, 17, 31, 33} {
		if _, err := AesEncryptGCM(nil, make([]byte, keyLen), make([]byte, 12), nil); err == nil {
			t.Fatal("key accepted")
		}
	}
	for _, nonceLen := range []int{0, 8, 11, 13, 16} {
		if _, err := AesDecryptGCM(nil, make([]byte, 32), make([]byte, nonceLen), nil); err == nil {
			t.Fatal("nonce accepted")
		}
	}
	for _, f := range []func([]byte, []byte, []byte) ([]byte, error){AesEncryptCBC, AesDecryptCBC} {
		for _, args := range [][][]byte{{make([]byte, 15), make([]byte, 16), make([]byte, 16)}, {make([]byte, 16), make([]byte, 15), make([]byte, 16)}, {make([]byte, 16), make([]byte, 16), make([]byte, 15)}} {
			if _, err := f(args[0], args[1], args[2]); err == nil {
				t.Fatal("invalid CBC input accepted")
			}
		}
	}
}

func TestAESCBCCompatibilityVector(t *testing.T) {
	key, _ := hex.DecodeString("2b7e151628aed2a6abf7158809cf4f3c")
	iv, _ := hex.DecodeString("000102030405060708090a0b0c0d0e0f")
	plain, _ := hex.DecodeString("6bc1bee22e409f96e93d7e117393172a")
	want, _ := hex.DecodeString("7649abac8119b246cee98e9b12e9197d")
	out, err := AesEncryptCBC(plain, key, iv)
	if err != nil || !bytes.Equal(out, want) {
		t.Fatalf("CBC vector: %v", err)
	}
	got, err := AesDecryptCBC(want, key, iv)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("CBC decrypt: %v", err)
	}
	if !bytes.Equal(iv, []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}) {
		t.Fatal("IV mutated")
	}
}
