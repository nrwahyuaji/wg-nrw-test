package auth

import "testing"

// TestNewToken: token unik, panjang tetap 43 karakter, hash cocok dengan HashToken.
func TestNewToken(t *testing.T) {
	first, firstHash, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	second, _, _ := NewToken()
	if first == second || len(first) != 43 || HashToken(first) != firstHash || len(firstHash) != 64 {
		t.Fatalf("token tidak sesuai: %q %q %q", first, second, firstHash)
	}
}
