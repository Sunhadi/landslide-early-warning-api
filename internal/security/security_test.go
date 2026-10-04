package security

import (
	"strings"
	"testing"
	"time"
)

func TestHashAndCheckPassword(t *testing.T) {
	hash, err := HashPassword("rahasia123")
	if err != nil {
		t.Fatalf("hash gagal: %v", err)
	}
	if hash == "rahasia123" {
		t.Fatal("hash tidak boleh sama dengan password asli")
	}
	if !CheckPassword(hash, "rahasia123") {
		t.Fatal("password benar harus cocok")
	}
	if CheckPassword(hash, "salah") {
		t.Fatal("password salah tidak boleh cocok")
	}
}

func TestCipherRoundTrip(t *testing.T) {
	c, err := NewCipher("kunci-rahasia")
	if err != nil {
		t.Fatalf("new cipher: %v", err)
	}
	plain := "+628123456789"
	enc, err := c.Encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == plain {
		t.Fatal("ciphertext tidak boleh sama dengan plaintext")
	}
	dec, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if dec != plain {
		t.Fatalf("hasil dekripsi berbeda: %q", dec)
	}
}

func TestCipherNonceUnique(t *testing.T) {
	c, _ := NewCipher("kunci")
	e1, _ := c.Encrypt("sama")
	e2, _ := c.Encrypt("sama")
	if e1 == e2 {
		t.Fatal("dua enkripsi nilai sama harus menghasilkan ciphertext berbeda (nonce acak)")
	}
}

func TestCipherWrongKeyFails(t *testing.T) {
	c1, _ := NewCipher("kunci-satu")
	c2, _ := NewCipher("kunci-dua")
	enc, _ := c1.Encrypt("data pribadi")
	if _, err := c2.Decrypt(enc); err == nil {
		t.Fatal("dekripsi dengan kunci berbeda seharusnya gagal")
	}
}

func TestMaskPhone(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"+628123456789", "+6281234****789"},
		{"628123456789", "62812345****789"},
		{"12345", "******"},
		{"", "******"},
	}
	for _, c := range cases {
		if got := MaskPhone(c.in); got != c.want {
			t.Errorf("MaskPhone(%q) = %q, mau %q", c.in, got, c.want)
		}
	}
	if strings.Contains(MaskPhone("+628123456789"), "567") {
		t.Error("angka tengah seharusnya disamarkan")
	}
}

func TestTokenIssueAndVerify(t *testing.T) {
	ts := NewTokenService("secret-test", time.Hour)
	token, err := ts.Issue("user-1", "officer")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	claims, err := ts.Verify(token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Subject != "user-1" {
		t.Errorf("subject = %q, mau user-1", claims.Subject)
	}
	if claims.Role != "officer" {
		t.Errorf("role = %q, mau officer", claims.Role)
	}
}

func TestTokenTamperedFails(t *testing.T) {
	ts := NewTokenService("secret-test", time.Hour)
	token, _ := ts.Issue("user-1", "admin")
	if _, err := ts.Verify(token + "x"); err == nil {
		t.Fatal("token yang diubah seharusnya gagal diverifikasi")
	}
}

func TestTokenWrongSecretFails(t *testing.T) {
	a := NewTokenService("secret-a", time.Hour)
	b := NewTokenService("secret-b", time.Hour)
	token, _ := a.Issue("u", "admin")
	if _, err := b.Verify(token); err == nil {
		t.Fatal("token dari secret berbeda seharusnya gagal")
	}
}

func TestTokenExpiredFails(t *testing.T) {
	ts := NewTokenService("secret-test", -time.Minute)
	token, _ := ts.Issue("u", "admin")
	if _, err := ts.Verify(token); err == nil {
		t.Fatal("token kedaluwarsa seharusnya gagal")
	}
}

func TestRevokeToken(t *testing.T) {
	ts := NewTokenService("secret-test", time.Hour)
	token, _ := ts.Issue("u1", "admin")
	if _, err := ts.Verify(token); err != nil {
		t.Fatalf("token valid sebelum dicabut: %v", err)
	}
	if err := ts.Revoke(token); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := ts.Verify(token); err == nil {
		t.Fatal("token yang dicabut harus ditolak")
	}
}

func TestRefreshTokenFlow(t *testing.T) {
	ts := NewTokenServiceWithRefresh("secret-test", time.Hour, 24*time.Hour)
	refresh, err := ts.IssueRefresh("u1", "officer")
	if err != nil {
		t.Fatalf("issue refresh: %v", err)
	}
	claims, err := ts.VerifyRefresh(refresh)
	if err != nil {
		t.Fatalf("verify refresh: %v", err)
	}
	if claims.Subject != "u1" || claims.Role != "officer" {
		t.Fatalf("claims salah: %+v", claims)
	}
}

func TestAccessTokenRejectedAsRefresh(t *testing.T) {
	ts := NewTokenService("secret-test", time.Hour)
	access, _ := ts.Issue("u1", "admin")
	if _, err := ts.VerifyRefresh(access); err == nil {
		t.Fatal("access token tidak boleh diterima sebagai refresh token")
	}
}

func TestRefreshTokenRejectedAsAccess(t *testing.T) {
	ts := NewTokenServiceWithRefresh("secret-test", time.Hour, time.Hour)
	refresh, _ := ts.IssueRefresh("u1", "admin")
	if _, err := ts.Verify(refresh); err == nil {
		t.Fatal("refresh token tidak boleh diterima sebagai access token")
	}
}

func TestRotateRefreshInvalidatesOld(t *testing.T) {
	ts := NewTokenServiceWithRefresh("secret-test", time.Hour, 24*time.Hour)
	old, _ := ts.IssueRefresh("u1", "admin")

	access, newRefresh, claims, err := ts.RotateRefresh(old)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if access == "" || newRefresh == "" || claims.Subject != "u1" {
		t.Fatalf("hasil rotasi salah: %+v", claims)
	}
	// refresh lama harus tidak berlaku
	if _, err := ts.VerifyRefresh(old); err == nil {
		t.Fatal("refresh token lama harus tidak berlaku setelah rotasi")
	}
	// refresh baru valid
	if _, err := ts.VerifyRefresh(newRefresh); err != nil {
		t.Fatalf("refresh token baru harus valid: %v", err)
	}
	// access baru valid
	if _, err := ts.Verify(access); err != nil {
		t.Fatalf("access token baru harus valid: %v", err)
	}
}

func TestRevokeAlsoRevokesRefresh(t *testing.T) {
	ts := NewTokenServiceWithRefresh("secret-test", time.Hour, 24*time.Hour)
	access, _ := ts.Issue("u1", "admin")
	refresh, _ := ts.IssueRefresh("u1", "admin")

	if err := ts.Revoke(access); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := ts.VerifyRefresh(refresh); err == nil {
		t.Fatal("refresh token harus ikut dicabut saat access token dicabut")
	}
}
