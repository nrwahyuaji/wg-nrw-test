// Package secret mengenkripsi data rahasia (private key WireGuard, rahasia TOTP) sebelum
// disimpan ke SQLite, memakai AES-256-GCM dengan kunci dari WGPANEL_SECRET_KEY.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
)

// ErrDecrypt dikembalikan bila data rusak atau dienkripsi dengan kunci lain
// (paling sering: WGPANEL_SECRET_KEY di server baru tidak sama dengan server lama).
var ErrDecrypt = errors.New("secret: gagal membuka data terenkripsi (kunci berbeda atau data rusak)")

// Box memegang AEAD siap pakai. Aman dipakai bersamaan dari banyak goroutine.
type Box struct {
	aead cipher.AEAD
}

// NewBox membuat Box dari WGPANEL_SECRET_KEY (base64 standar, tepat 32 byte setelah didekode).
// label memisahkan kunci per keperluan, mis. "wg-keys" dan "totp", supaya satu kunci induk
// tidak dipakai langsung untuk dua jenis data.
func NewBox(masterKeyBase64, label string) (*Box, error) {
	master, err := base64.StdEncoding.DecodeString(masterKeyBase64)
	if err != nil || len(master) != 32 {
		return nil, fmt.Errorf("secret: WGPANEL_SECRET_KEY harus base64 dari 32 byte acak (openssl rand -base64 32)")
	}
	derived := sha256.Sum256(append([]byte("wgpanel|"+label+"|"), master...))
	block, err := aes.NewCipher(derived[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Seal mengenkripsi plaintext; hasil = nonce acak || ciphertext+tag.
// Nonce baru di setiap panggilan, jadi dua isi yang sama menghasilkan blob berbeda.
func (b *Box) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Open membuka blob hasil Seal. Return ErrDecrypt bila blob dipotong/diubah atau kuncinya lain.
func (b *Box) Open(blob []byte) ([]byte, error) {
	size := b.aead.NonceSize()
	if len(blob) < size {
		return nil, ErrDecrypt
	}
	plaintext, err := b.aead.Open(nil, blob[:size], blob[size:], nil)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plaintext, nil
}
