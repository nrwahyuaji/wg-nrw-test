package secret

import (
	"bytes"
	"errors"
	"testing"
)

const testKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=" // 32 byte, hanya untuk uji

// TestSealOpen: bolak-balik berhasil, dua Seal berbeda, label lain & bit diubah gagal.
func TestSealOpen(t *testing.T) {
	box, err := NewBox(testKey, "wg-keys")
	if err != nil {
		t.Fatal(err)
	}
	first, _ := box.Seal([]byte("rahasia"))
	second, _ := box.Seal([]byte("rahasia"))
	if bytes.Equal(first, second) {
		t.Fatal("nonce harus acak")
	}
	if got, err := box.Open(first); err != nil || string(got) != "rahasia" {
		t.Fatalf("open: %q %v", got, err)
	}
	other, _ := NewBox(testKey, "totp")
	if _, err := other.Open(first); !errors.Is(err, ErrDecrypt) {
		t.Fatal("label berbeda harus gagal")
	}
	first[len(first)-1] ^= 1
	if _, err := box.Open(first); !errors.Is(err, ErrDecrypt) {
		t.Fatal("blob diubah harus gagal")
	}
	if _, err := NewBox("pendek", "x"); err == nil {
		t.Fatal("kunci bukan 32 byte harus ditolak")
	}
}
