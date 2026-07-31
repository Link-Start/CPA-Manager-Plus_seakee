package security

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
)

func TestAdminCredentialVerifiesOnlyAdminKey(t *testing.T) {
	const adminKey = "cpamp_test_key_0123456789abcdef"

	credential, err := NewAdminCredential(adminKey, "test")
	if err != nil {
		t.Fatalf("create credential: %v", err)
	}
	if !VerifyAdminKey(credential, adminKey) {
		t.Fatal("admin key did not verify")
	}
	if VerifyAdminKey(credential, "management-key") {
		t.Fatal("cpa management key should not verify as admin key")
	}
	if strings.Contains(credential.KeyHash, adminKey) || strings.Contains(credential.Salt, adminKey) {
		t.Fatalf("credential contains admin key material: %#v", credential)
	}
	if credential.Iterations != adminHashIterations || credential.Iterations <= legacyAdminHashIterations {
		t.Fatalf("credential iterations = %d, want %d", credential.Iterations, adminHashIterations)
	}
}

func TestAdminCredentialVerifiesLegacySingleRoundHashes(t *testing.T) {
	const adminKey = "cmp_admin_legacy_key_0123456789"
	salt := []byte("0123456789abcdef")
	keyHash, err := hashAdminKey(adminKey, salt, legacyAdminHashIterations)
	if err != nil {
		t.Fatalf("hash legacy admin key: %v", err)
	}
	credential := model.AdminCredential{
		Version:    1,
		Salt:       base64.RawStdEncoding.EncodeToString(salt),
		KeyHash:    base64.RawStdEncoding.EncodeToString(keyHash),
		Iterations: legacyAdminHashIterations,
	}
	if !VerifyAdminKey(credential, adminKey) {
		t.Fatal("legacy single-round admin key did not verify")
	}
	credential.Iterations = 0
	if !VerifyAdminKey(credential, adminKey) {
		t.Fatal("legacy credential without an iterations field did not verify")
	}
}

func TestAdminCredentialStillAcceptsLegacyAdminKeyPrefix(t *testing.T) {
	const adminKey = "cmp_admin_test_key_0123456789abcdef"

	credential, err := NewAdminCredential(adminKey, "test")
	if err != nil {
		t.Fatalf("create credential: %v", err)
	}
	if !VerifyAdminKey(credential, adminKey) {
		t.Fatal("legacy admin key did not verify")
	}
}

func TestAdminKeyVerifierBoundsConcurrentPBKDF2Work(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	verifier := newAdminKeyVerifier(1, func(model.AdminCredential, string) bool {
		close(started)
		<-release
		return true
	})
	firstDone := make(chan error, 1)
	go func() {
		_, err := verifier.Verify(model.AdminCredential{}, "first")
		firstDone <- err
	}()
	<-started
	if ok, err := verifier.Verify(model.AdminCredential{}, "second"); ok || !errors.Is(err, ErrAdminKeyVerificationBusy) {
		t.Fatalf("second verification = ok=%v err=%v", ok, err)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}

func TestGenerateAdminKeyUsesExpectedPrefixAndEntropyLength(t *testing.T) {
	adminKey, err := GenerateAdminKey()
	if err != nil {
		t.Fatalf("generate admin key: %v", err)
	}
	if !strings.HasPrefix(adminKey, "cpamp_") {
		t.Fatalf("admin key = %q", adminKey)
	}
	secret := strings.TrimPrefix(adminKey, "cpamp_")
	if got, want := len(secret), 32; got != want {
		t.Fatalf("random length = %d, want %d", got, want)
	}
	if !isAlnum(secret) {
		t.Fatalf("admin key contains non-alphanumeric characters: %q", adminKey)
	}
	if err := ValidateAdminKey(adminKey); err != nil {
		t.Fatalf("generated admin key does not satisfy policy: %v", err)
	}
}

func TestValidateAdminKeyRequiresLengthAndThreeCharacterClasses(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		key     string
		wantErr error
	}{
		{name: "too short", key: "Aa1!short", wantErr: ErrAdminKeyTooShort},
		{name: "two classes", key: "abcdefghijklmnop1", wantErr: ErrAdminKeyCharacterClasses},
		{name: "three classes", key: "abcdefghijklmnop1_"},
		{name: "four classes", key: "Abcdefghijklmn1_"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := ValidateAdminKey(testCase.key)
			if testCase.wantErr == nil && err != nil {
				t.Fatalf("ValidateAdminKey() error = %v", err)
			}
			if testCase.wantErr != nil && !errors.Is(err, testCase.wantErr) {
				t.Fatalf("ValidateAdminKey() error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestBootstrapCredentialExpiresAndCanBeConsumed(t *testing.T) {
	const token = "cpamp_bootstrap_test_token_0123456789"
	credential, err := NewBootstrapCredential(token, time.Minute)
	if err != nil {
		t.Fatalf("create bootstrap credential: %v", err)
	}
	createdAt := time.UnixMilli(credential.CreatedAtMS)
	if !VerifyBootstrapToken(credential, token, createdAt) {
		t.Fatal("bootstrap token did not verify")
	}
	if VerifyBootstrapToken(credential, "wrong-token", createdAt) {
		t.Fatal("wrong bootstrap token verified")
	}
	if VerifyBootstrapToken(credential, token, time.UnixMilli(credential.ExpiresAtMS)) {
		t.Fatal("expired bootstrap token verified")
	}
	credential.ConsumedAtMS = createdAt.UnixMilli()
	if VerifyBootstrapToken(credential, token, createdAt) {
		t.Fatal("consumed bootstrap token verified")
	}
}

func TestRandomAlnumRejectsInvalidLength(t *testing.T) {
	if value, err := randomAlnum(0); err == nil || value != "" {
		t.Fatalf("randomAlnum(0) = %q, %v; want empty value and error", value, err)
	}
}

func TestProtectorEncryptsAndDecryptsString(t *testing.T) {
	protector, err := NewProtector([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("create protector: %v", err)
	}

	encrypted, err := protector.ProtectString("management-key")
	if err != nil {
		t.Fatalf("protect string: %v", err)
	}
	if encrypted == "management-key" || !IsProtected(encrypted) {
		t.Fatalf("encrypted value = %q", encrypted)
	}

	plaintext, err := protector.UnprotectString(encrypted)
	if err != nil {
		t.Fatalf("unprotect string: %v", err)
	}
	if plaintext != "management-key" {
		t.Fatalf("plaintext = %q", plaintext)
	}

	otherProtector, err := NewProtector([]byte("abcdef0123456789abcdef0123456789"))
	if err != nil {
		t.Fatalf("create other protector: %v", err)
	}
	if _, err := otherProtector.UnprotectString(encrypted); err == nil {
		t.Fatal("decrypt with wrong data key succeeded")
	}
}

func TestLoadOrCreateDataKeyCreatesStableRestrictedFile(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "data.key")

	first, created, err := LoadOrCreateDataKey("", keyPath)
	if err != nil {
		t.Fatalf("create data key: %v", err)
	}
	if !created || len(first) != 32 {
		t.Fatalf("created=%v len=%d", created, len(first))
	}

	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat data key: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("data key permissions = %o, want 600", info.Mode().Perm())
	}

	second, created, err := LoadOrCreateDataKey("", keyPath)
	if err != nil {
		t.Fatalf("load data key: %v", err)
	}
	if created || string(second) != string(first) {
		t.Fatalf("second created=%v key stable=%v", created, string(second) == string(first))
	}
}

func isAlnum(value string) bool {
	for _, r := range value {
		if r >= '0' && r <= '9' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= 'a' && r <= 'z' {
			continue
		}
		return false
	}
	return value != ""
}
