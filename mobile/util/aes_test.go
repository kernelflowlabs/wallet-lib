package util

import (
	"bytes"
	"testing"
)

func TestAESGCMSealOpen(t *testing.T) {
	for _, length := range []int{0, 1, 15, 16, 17, 4096} {
		plain, key, aad := bytes.Repeat([]byte{7}, length), make([]byte, 32), []byte("format-v2")
		first, err := AesGcmSeal(plain, key, aad)
		if err != nil {
			t.Fatal(err)
		}
		second, err := AesGcmSeal(plain, key, aad)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(first[:12], second[:12]) {
			t.Fatal("nonce reused")
		}
		got, err := AesGcmOpen(first, key, aad)
		if err != nil || !bytes.Equal(got, plain) {
			t.Fatalf("round trip: %v", err)
		}
		if _, err := AesGcmOpen(first, key, []byte("other")); err == nil {
			t.Fatal("AAD ignored")
		}
	}
}

func TestAESMobileBounds(t *testing.T) {
	key := make([]byte, 32)
	if _, err := AesGcmSeal(make([]byte, maxCipherInput+1), key, nil); err == nil {
		t.Fatal("oversized input")
	}
	if _, err := AesGcmOpen(make([]byte, 27), key, nil); err == nil {
		t.Fatal("short input")
	}
	if _, err := AesGcmOpen(make([]byte, maxCipherInput+29), key, nil); err == nil {
		t.Fatal("oversized ciphertext")
	}
	if _, err := AesGcmSeal(nil, key, make([]byte, 65537)); err == nil {
		t.Fatal("oversized AAD")
	}
	if _, err := AesGcmSeal(nil, []byte("bad"), nil); err == nil {
		t.Fatal("invalid key")
	}
	for _, f := range []func([]byte, []byte, []byte) ([]byte, error){AesCbcEncrypt, AesCbcDecrypt} {
		if _, err := f(make([]byte, maxCipherInput+16), key, make([]byte, 16)); err == nil {
			t.Fatal("oversized CBC input")
		}
	}
}
