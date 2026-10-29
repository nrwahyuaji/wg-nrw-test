// Package auth: berkas ini membuat dan memeriksa token acak (sesi, tautan berbagi, kode pemulihan).
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// tokenBytes = 32 byte acak (256 bit): tidak mungkin ditebak, cukup pendek untuk cookie/URL.
const tokenBytes = 32

// NewToken membuat token acak berformat base64url tanpa padding (aman di cookie dan URL)
// beserta hash SHA-256-nya. Yang disimpan di basis data HANYA hash, sehingga bocornya berkas
// basis data tidak memberi siapa pun sesi atau tautan berbagi yang masih berlaku.
func NewToken() (plain string, hash string, err error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	plain = base64.RawURLEncoding.EncodeToString(raw)
	return plain, HashToken(plain), nil
}

// HashToken menghitung hash token yang dikirim klien untuk dicari di basis data.
// SHA-256 tanpa garam cukup di sini karena token sudah 256 bit acak (bukan password manusia).
func HashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}
