package auth

import (
	"errors"
	"strings"
	"testing"
)

// TestPasswordHashAndCheck: pendek/terlalu panjang ditolak, benar=true, salah=false, dummy selalu false.
func TestPasswordHashAndCheck(t *testing.T) {
	if _, err := HashPassword("pendek-11ch"); !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("11 karakter harus ditolak, got %v", err)
	}
	// 11 rune multibyte (> 12 byte) tetap terlalu pendek: batas dihitung per rune.
	if _, err := HashPassword(strings.Repeat("é", 11)); !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("11 rune multibyte harus ditolak, got %v", err)
	}
	if _, err := HashPassword(strings.Repeat("a", 73)); !errors.Is(err, ErrPasswordTooLong) {
		t.Fatalf("73 byte harus ditolak, got %v", err)
	}
	hash, err := HashPassword("kata-sandi-uji-12")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "$2a$12$") {
		t.Fatalf("hash harus bcrypt cost 12, got prefix %q", hash[:7])
	}
	if !CheckPassword(hash, "kata-sandi-uji-12") {
		t.Fatal("password benar harus cocok")
	}
	if CheckPassword(hash, "kata-sandi-uji-13") {
		t.Fatal("password salah tidak boleh cocok")
	}
	if CheckPassword("bukan-hash", "kata-sandi-uji-12") {
		t.Fatal("hash rusak tidak boleh cocok")
	}
	if CheckDummyPassword("wgpanel-dummy-password") {
		t.Fatal("dummy selalu false")
	}
}
