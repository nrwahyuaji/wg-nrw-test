// Package auth: berkas ini meng-hash dan memeriksa password akun panel (bcrypt).
package auth

import (
	"errors"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// passwordCost = 12: ±250 ms per percobaan di VPS kecil, cukup lambat untuk brute force offline
// tetapi masih nyaman untuk login manusia (DESIGN §12).
const passwordCost = 12

// minPasswordRunes dihitung dalam rune, bukan byte, agar password non-ASCII tidak lolos lebih pendek.
const minPasswordRunes = 12

// maxPasswordBytes: bcrypt hanya memakai 72 byte pertama dan menolak input lebih panjang.
const maxPasswordBytes = 72

// ErrPasswordTooShort dikembalikan HashPassword bila password kurang dari 12 karakter.
var ErrPasswordTooShort = errors.New("password minimal 12 karakter")

// ErrPasswordTooLong dikembalikan HashPassword bila password melebihi batas bcrypt (72 byte).
var ErrPasswordTooLong = errors.New("password maksimal 72 byte")

// dummyHash dibuat sekali saat init dan dipakai CheckPassword ketika username tidak ada,
// sehingga waktu respons login sama untuk username valid maupun tidak (DESIGN §7.3).
var dummyHash = mustDummyHash()

// HashPassword menolak password < 12 rune atau > 72 byte, lalu mengembalikan hash bcrypt cost 12.
func HashPassword(plain string) (string, error) {
	if utf8.RuneCountInString(plain) < minPasswordRunes {
		return "", ErrPasswordTooShort
	}
	if len(plain) > maxPasswordBytes {
		return "", ErrPasswordTooLong
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), passwordCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword mengembalikan true bila plain cocok dengan hash. Hash rusak dianggap tidak cocok.
func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// CheckDummyPassword menghabiskan waktu yang sama dengan CheckPassword untuk username yang tidak ada.
// Hasilnya selalu false.
func CheckDummyPassword(plain string) bool {
	CheckPassword(dummyHash, plain)
	return false
}

// mustDummyHash membuat hash dummy berbiaya sama dengan hash sungguhan. Panik hanya bila bcrypt
// sendiri rusak, yang berarti biner tidak boleh berjalan.
func mustDummyHash() string {
	hash, err := bcrypt.GenerateFromPassword([]byte("wgpanel-dummy-password"), passwordCost)
	if err != nil {
		panic(err)
	}
	return string(hash)
}
