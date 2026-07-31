package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
)

const encryptedPrefix = "enc:v1:"
const adminKeyPrefix = "cpamp_"
const bootstrapTokenPrefix = "cpamp_bootstrap_"
const generatedAdminKeyLength = 32
const generatedSecretAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
const adminHashIterations = 120000
const legacyAdminHashIterations = 1
const bootstrapHashIterations = 1

var ErrAdminKeyTooShort = errors.New("admin key must be at least 16 characters")
var ErrAdminKeyCharacterClasses = errors.New("admin key must contain at least three of uppercase, lowercase, number, and special character")
var ErrAdminKeyVerificationBusy = errors.New("admin key verification is temporarily busy")

type adminKeyVerifier struct {
	slots  chan struct{}
	verify func(model.AdminCredential, string) bool
}

var boundedAdminKeyVerifier = newAdminKeyVerifier(defaultAdminKeyVerificationConcurrency(), VerifyAdminKey)

func GenerateAdminKey() (string, error) {
	random, err := randomAdminKeyBody(generatedAdminKeyLength)
	if err != nil {
		return "", err
	}
	return adminKeyPrefix + random, nil
}

func ValidateAdminKey(adminKey string) error {
	adminKey = strings.TrimSpace(adminKey)
	if len([]rune(adminKey)) < 16 {
		return ErrAdminKeyTooShort
	}
	classes := 0
	var upper, lower, digit, special bool
	for _, value := range adminKey {
		switch {
		case value >= 'A' && value <= 'Z':
			upper = true
		case value >= 'a' && value <= 'z':
			lower = true
		case value >= '0' && value <= '9':
			digit = true
		default:
			special = true
		}
	}
	for _, present := range []bool{upper, lower, digit, special} {
		if present {
			classes++
		}
	}
	if classes < 3 {
		return ErrAdminKeyCharacterClasses
	}
	return nil
}

func GenerateBootstrapToken() (string, error) {
	return GenerateServiceSecret(bootstrapTokenPrefix)
}

func GenerateServiceSecret(prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", errors.New("secret prefix is required")
	}
	for _, value := range prefix {
		if (value >= 'A' && value <= 'Z') || (value >= 'a' && value <= 'z') || (value >= '0' && value <= '9') || value == '_' || value == '-' {
			continue
		}
		return "", errors.New("secret prefix contains unsupported characters")
	}
	random, err := randomAlnum(generatedAdminKeyLength)
	if err != nil {
		return "", err
	}
	return prefix + random, nil
}

func NewBootstrapCredential(token string, ttl time.Duration) (model.BootstrapCredential, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return model.BootstrapCredential{}, errors.New("bootstrap token is required")
	}
	if ttl <= 0 {
		return model.BootstrapCredential{}, errors.New("bootstrap token ttl must be positive")
	}
	salt, err := randomBytes(16)
	if err != nil {
		return model.BootstrapCredential{}, err
	}
	tokenHash, err := hashAdminKey(token, salt, bootstrapHashIterations)
	if err != nil {
		return model.BootstrapCredential{}, err
	}
	now := time.Now()
	return model.BootstrapCredential{
		Version:     1,
		Salt:        base64.RawStdEncoding.EncodeToString(salt),
		TokenHash:   base64.RawStdEncoding.EncodeToString(tokenHash),
		CreatedAtMS: now.UnixMilli(),
		ExpiresAtMS: now.Add(ttl).UnixMilli(),
	}, nil
}

func VerifyBootstrapToken(credential model.BootstrapCredential, token string, now time.Time) bool {
	token = strings.TrimSpace(token)
	if token == "" || credential.Salt == "" || credential.TokenHash == "" || credential.ConsumedAtMS != 0 {
		return false
	}
	if credential.ExpiresAtMS > 0 && now.UnixMilli() >= credential.ExpiresAtMS {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(credential.Salt)
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(credential.TokenHash)
	if err != nil {
		return false
	}
	got, err := hashAdminKey(token, salt, bootstrapHashIterations)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

func NewAdminCredential(adminKey string, source string) (model.AdminCredential, error) {
	adminKey = strings.TrimSpace(adminKey)
	if adminKey == "" {
		return model.AdminCredential{}, errors.New("admin key is required")
	}
	salt, err := randomBytes(16)
	if err != nil {
		return model.AdminCredential{}, err
	}
	keyHash, err := hashAdminKey(adminKey, salt, adminHashIterations)
	if err != nil {
		return model.AdminCredential{}, err
	}
	return model.AdminCredential{
		Version:     1,
		Salt:        base64.RawStdEncoding.EncodeToString(salt),
		KeyHash:     base64.RawStdEncoding.EncodeToString(keyHash),
		Iterations:  adminHashIterations,
		CreatedAtMS: time.Now().UnixMilli(),
		Source:      source,
	}, nil
}

func VerifyAdminKey(credential model.AdminCredential, adminKey string) bool {
	adminKey = strings.TrimSpace(adminKey)
	if adminKey == "" || credential.Salt == "" || credential.KeyHash == "" {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(credential.Salt)
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(credential.KeyHash)
	if err != nil {
		return false
	}
	iterations := credential.Iterations
	if iterations <= 0 {
		iterations = legacyAdminHashIterations
	}
	got, err := hashAdminKey(adminKey, salt, iterations)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(got, want) == 1
}

func VerifyAdminKeyBounded(credential model.AdminCredential, adminKey string) (bool, error) {
	return boundedAdminKeyVerifier.Verify(credential, adminKey)
}

func newAdminKeyVerifier(maxConcurrent int, verify func(model.AdminCredential, string) bool) *adminKeyVerifier {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	if verify == nil {
		verify = VerifyAdminKey
	}
	return &adminKeyVerifier{
		slots:  make(chan struct{}, maxConcurrent),
		verify: verify,
	}
}

func (v *adminKeyVerifier) Verify(credential model.AdminCredential, adminKey string) (bool, error) {
	select {
	case v.slots <- struct{}{}:
		defer func() { <-v.slots }()
		return v.verify(credential, adminKey), nil
	default:
		return false, ErrAdminKeyVerificationBusy
	}
}

func defaultAdminKeyVerificationConcurrency() int {
	concurrency := runtime.GOMAXPROCS(0)
	if concurrency < 1 {
		return 1
	}
	if concurrency > 4 {
		return 4
	}
	return concurrency
}

func ExtractBearerToken(header string) string {
	header = strings.TrimSpace(header)
	const prefix = "Bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

func LoadOrCreateDataKey(rawValue string, keyPath string) ([]byte, bool, error) {
	if strings.TrimSpace(rawValue) != "" {
		key, err := deriveDataKey(rawValue)
		return key, false, err
	}
	keyPath = strings.TrimSpace(keyPath)
	if keyPath == "" {
		return nil, false, errors.New("data key path is required")
	}
	if data, err := os.ReadFile(keyPath); err == nil {
		key, err := parseStoredDataKey(strings.TrimSpace(string(data)))
		return key, false, err
	} else if !os.IsNotExist(err) {
		return nil, false, fmt.Errorf("read data key %s: %w", keyPath, err)
	}
	key, err := randomBytes(32)
	if err != nil {
		return nil, false, err
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		return nil, false, fmt.Errorf("create data key directory %s: %w", filepath.Dir(keyPath), err)
	}
	content := base64.RawStdEncoding.EncodeToString(key) + "\n"
	if err := os.WriteFile(keyPath, []byte(content), 0o600); err != nil {
		return nil, false, fmt.Errorf("write data key %s: %w", keyPath, err)
	}
	return key, true, nil
}

type Protector struct {
	key []byte
}

func NewProtector(key []byte) (*Protector, error) {
	if len(key) == 0 {
		return nil, errors.New("data key is required")
	}
	derived, err := hkdf.Key(sha256.New, key, []byte("cpa-manager-plus"), "settings-secrets-v1", 32)
	if err != nil {
		return nil, err
	}
	return &Protector{key: derived}, nil
}

func (p *Protector) ProtectString(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || IsProtected(value) {
		return value, nil
	}
	nonce, err := randomBytes(12)
	if err != nil {
		return "", err
	}
	aead, err := p.aead()
	if err != nil {
		return "", err
	}
	ciphertext := aead.Seal(nil, nonce, []byte(value), []byte("settings-secret"))
	return encryptedPrefix +
		base64.RawStdEncoding.EncodeToString(nonce) + ":" +
		base64.RawStdEncoding.EncodeToString(ciphertext), nil
}

func (p *Protector) UnprotectString(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !IsProtected(value) {
		return value, nil
	}
	payload := strings.TrimPrefix(value, encryptedPrefix)
	parts := strings.Split(payload, ":")
	if len(parts) != 2 {
		return "", errors.New("invalid encrypted secret format")
	}
	nonce, err := base64.RawStdEncoding.DecodeString(parts[0])
	if err != nil {
		return "", err
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	aead, err := p.aead()
	if err != nil {
		return "", err
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, []byte("settings-secret"))
	if err != nil {
		return "", errors.New("decrypt secret: invalid data key or corrupted ciphertext")
	}
	return string(plaintext), nil
}

func IsProtected(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), encryptedPrefix)
}

func (p *Protector) aead() (cipher.AEAD, error) {
	block, err := aes.NewCipher(p.key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func hashAdminKey(adminKey string, salt []byte, iterations int) ([]byte, error) {
	if iterations <= 1 {
		mac := hmac.New(sha256.New, salt)
		_, _ = mac.Write([]byte(adminKey))
		return mac.Sum(nil), nil
	}
	return pbkdf2.Key(sha256.New, adminKey, salt, iterations, 32)
}

func deriveDataKey(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("data key is required")
	}
	if key, err := parseStoredDataKey(value); err == nil {
		return key, nil
	}
	return hkdf.Key(sha256.New, []byte(value), []byte("cpa-manager-plus"), "data-key-v1", 32)
}

func parseStoredDataKey(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("data key is empty")
	}
	if key, err := base64.RawStdEncoding.DecodeString(value); err == nil && len(key) == 32 {
		return key, nil
	}
	if key, err := base64.StdEncoding.DecodeString(value); err == nil && len(key) == 32 {
		return key, nil
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(value); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if len([]byte(value)) == 32 {
		return []byte(value), nil
	}
	return nil, errors.New("data key must be 32 bytes or base64-encoded 32 bytes")
}

func randomBytes(size int) ([]byte, error) {
	out := make([]byte, size)
	if _, err := rand.Read(out); err != nil {
		return nil, err
	}
	return out, nil
}

func randomAlnum(length int) (string, error) {
	if length <= 0 {
		return "", errors.New("random alphanumeric length must be positive")
	}
	out := make([]byte, length)
	buf := make([]byte, length*2)
	limit := byte(len(generatedSecretAlphabet) * (256 / len(generatedSecretAlphabet)))
	for i := 0; i < length; {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, b := range buf {
			if b >= limit {
				continue
			}
			out[i] = generatedSecretAlphabet[int(b)%len(generatedSecretAlphabet)]
			i++
			if i == length {
				break
			}
		}
	}
	return string(out), nil
}

func randomAdminKeyBody(length int) (string, error) {
	if length < 3 {
		return "", errors.New("admin key body length must be at least 3")
	}
	body, err := randomAlnum(length)
	if err != nil {
		return "", err
	}
	out := []byte(body)
	required := []string{"ABCDEFGHIJKLMNOPQRSTUVWXYZ", "abcdefghijklmnopqrstuvwxyz", "0123456789"}
	for index, alphabet := range required {
		value, err := randomIndex(len(alphabet))
		if err != nil {
			return "", err
		}
		out[index] = alphabet[value]
	}
	for index := len(out) - 1; index > 0; index-- {
		swap, err := randomIndex(index + 1)
		if err != nil {
			return "", err
		}
		out[index], out[swap] = out[swap], out[index]
	}
	return string(out), nil
}

func randomIndex(limit int) (int, error) {
	if limit <= 0 || limit > 256 {
		return 0, errors.New("random index limit must be between 1 and 256")
	}
	threshold := 256 - (256 % limit)
	buffer := []byte{0}
	for {
		if _, err := rand.Read(buffer); err != nil {
			return 0, err
		}
		if int(buffer[0]) < threshold {
			return int(buffer[0]) % limit, nil
		}
	}
}

func EqualHMAC(left string, right string) bool {
	return hmac.Equal([]byte(left), []byte(right))
}
