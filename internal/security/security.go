// Package security menangani JWT, enkripsi data pribadi (UU PDP), dan masking PII.
package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	tokenAccess  = "access"
	tokenRefresh = "refresh"
)

// newJTI membuat ID unik acak untuk token.
func newJTI() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// Claims adalah klaim JWT yang dipakai aplikasi.
type Claims struct {
	Role string `json:"role"`
	Type string `json:"typ"` // "access" | "refresh"
	jwt.RegisteredClaims
}

// TokenService membuat & memverifikasi JWT, serta mengelola blacklist/refresh.
type TokenService struct {
	secret     []byte
	ttl        time.Duration // masa hidup access token
	refreshTTL time.Duration // masa hidup refresh token
	blacklist  *blacklist
	refresh    *refreshStore
}

// NewTokenService membuat TokenService baru dengan TTL access token.
func NewTokenService(secret string, ttl time.Duration) *TokenService {
	return NewTokenServiceWithRefresh(secret, ttl, 7*24*time.Hour)
}

// NewTokenServiceWithRefresh membuat TokenService dengan TTL access & refresh.
func NewTokenServiceWithRefresh(secret string, ttl, refreshTTL time.Duration) *TokenService {
	return &TokenService{
		secret:     []byte(secret),
		ttl:        ttl,
		refreshTTL: refreshTTL,
		blacklist:  newBlacklist(),
		refresh:    newRefreshStore(),
	}
}

// IssueRefresh membuat refresh token untuk user.
func (t *TokenService) IssueRefresh(userID, role string) (string, error) {
	tok, jti, err := t.signWithJTI(userID, role, tokenRefresh, t.refreshTTL)
	if err != nil {
		return "", err
	}
	t.refresh.register(jti, userID, time.Now().Add(t.refreshTTL))
	return tok, nil
}

// Issue membuat access token untuk user.
func (t *TokenService) Issue(userID, role string) (string, error) {
	return t.sign(userID, role, tokenAccess, t.ttl)
}

func (t *TokenService) sign(userID, role, typ string, d time.Duration) (string, error) {
	tok, _, err := t.signWithJTI(userID, role, typ, d)
	return tok, err
}

func (t *TokenService) signWithJTI(userID, role, typ string, d time.Duration) (string, string, error) {
	now := time.Now()
	jti := newJTI()
	claims := Claims{
		Role: role,
		Type: typ,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(d)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(t.secret)
	return signed, jti, err
}

// Verify memvalidasi access token dan mengembalikan klaim.
func (t *TokenService) Verify(token string) (*Claims, error) {
	claims, err := t.parse(token)
	if err != nil {
		return nil, err
	}
	if claims.Type != tokenAccess {
		return nil, errors.New("jenis token tidak sesuai (butuh access token)")
	}
	if t.blacklist.has(claims.ID) {
		return nil, errors.New("token sudah dicabut")
	}
	return claims, nil
}

// VerifyRefresh memvalidasi refresh token.
func (t *TokenService) VerifyRefresh(token string) (*Claims, error) {
	claims, err := t.parse(token)
	if err != nil {
		return nil, err
	}
	if claims.Type != tokenRefresh {
		return nil, errors.New("jenis token tidak sesuai (butuh refresh token)")
	}
	if !t.refresh.valid(claims.ID) {
		return nil, errors.New("refresh token sudah tidak berlaku")
	}
	return claims, nil
}

func (t *TokenService) parse(token string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(tk *jwt.Token) (any, error) {
		if _, ok := tk.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("metode signing tidak didukung")
		}
		return t.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, errors.New("token tidak valid")
	}
	return claims, nil
}

// Revoke mencabut sebuah access token (memasukkan JTI ke blacklist sampai kedaluwarsa).
func (t *TokenService) Revoke(token string) error {
	claims, err := t.parse(token)
	if err != nil {
		return err
	}
	if claims.ExpiresAt != nil {
		t.blacklist.add(claims.ID, claims.ExpiresAt.Time)
	}
	// cabut juga refresh token terkait bila dilacak
	t.refresh.revokeBySubject(claims.Subject)
	return nil
}

// RotateRefresh menukar refresh token lama dengan set token baru (rotasi).
// Refresh token lama otomatis tidak berlaku lagi.
func (t *TokenService) RotateRefresh(oldRefresh string) (access, refresh string, claims *Claims, err error) {
	claims, err = t.VerifyRefresh(oldRefresh)
	if err != nil {
		return "", "", nil, err
	}
	t.refresh.revoke(claims.ID)
	access, err = t.Issue(claims.Subject, claims.Role)
	if err != nil {
		return "", "", nil, err
	}
	refresh, err = t.IssueRefresh(claims.Subject, claims.Role)
	if err != nil {
		return "", "", nil, err
	}
	return access, refresh, claims, nil
}

// --- blacklist access token ---

type blacklist struct {
	mu     sync.Mutex
	tokens map[string]time.Time // jti -> kedaluwarsa
}

func newBlacklist() *blacklist {
	bl := &blacklist{tokens: make(map[string]time.Time)}
	go bl.cleanup()
	return bl
}

func (b *blacklist) add(jti string, exp time.Time) {
	if jti == "" {
		return
	}
	b.mu.Lock()
	b.tokens[jti] = exp
	b.mu.Unlock()
}

func (b *blacklist) has(jti string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	exp, ok := b.tokens[jti]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(b.tokens, jti)
		return false
	}
	return true
}

func (b *blacklist) cleanup() {
	for range time.Tick(10 * time.Minute) {
		b.mu.Lock()
		for jti, exp := range b.tokens {
			if time.Now().After(exp) {
				delete(b.tokens, jti)
			}
		}
		b.mu.Unlock()
	}
}

// --- refresh token store ---

type refreshStore struct {
	mu     sync.Mutex
	active map[string]time.Time // jti -> kedaluwarsa
	bySub  map[string]string    // subject -> jti terakhir
}

func newRefreshStore() *refreshStore {
	return &refreshStore{active: make(map[string]time.Time), bySub: make(map[string]string)}
}

func (r *refreshStore) register(jti, subject string, exp time.Time) {
	r.mu.Lock()
	// cabut refresh lama milik subject yang sama (single active session)
	if old, ok := r.bySub[subject]; ok {
		delete(r.active, old)
	}
	r.active[jti] = exp
	r.bySub[subject] = jti
	r.mu.Unlock()
}

func (r *refreshStore) valid(jti string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	exp, ok := r.active[jti]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(r.active, jti)
		return false
	}
	return true
}

func (r *refreshStore) revoke(jti string) {
	r.mu.Lock()
	delete(r.active, jti)
	r.mu.Unlock()
}

func (r *refreshStore) revokeBySubject(subject string) {
	r.mu.Lock()
	if jti, ok := r.bySub[subject]; ok {
		delete(r.active, jti)
		delete(r.bySub, subject)
	}
	r.mu.Unlock()
}

// HashPassword membuat hash bcrypt.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword memverifikasi password terhadap hash.
func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// Cipher mengenkripsi data pribadi (mis. nomor HP) dengan AES-GCM.
// Kunci diturunkan dari secret via SHA-256 sehingga panjang selalu 32 byte.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher membuat Cipher dari secret.
func NewCipher(secret string) (*Cipher, error) {
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt mengembalikan base64(nonce||ciphertext).
func (c *Cipher) Encrypt(plain string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	out := c.aead.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(out), nil
}

// Decrypt membalikkan Encrypt.
func (c *Cipher) Decrypt(enc string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return "", errors.New("ciphertext tidak valid")
	}
	nonce, ct := raw[:ns], raw[ns:]
	pt, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// MaskPhone menyamarkan nomor telepon untuk ditampilkan/di-log.
// Contoh: "+628123456789" -> "+62812****789".
func MaskPhone(phone string) string {
	p := strings.TrimSpace(phone)
	if len(p) <= 6 {
		return "******"
	}
	prefix := p
	if len(p) > 8 {
		prefix = p[:8]
	} else {
		prefix = p[:len(p)-3]
	}
	suffix := p[len(p)-3:]
	stars := strings.Repeat("*", 4)
	return fmt.Sprintf("%s%s%s", prefix, stars, suffix)
}
