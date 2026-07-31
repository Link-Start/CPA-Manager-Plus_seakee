package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	managedruntime "github.com/seakee/cpa-manager-plus/apps/manager-server/internal/managedruntime"
)

func TestBuildReleaseManifestScansSlimAndCPAAssets(t *testing.T) {
	cpampDir := t.TempDir()
	cpaDir := t.TempDir()
	for _, name := range []string{
		"cpa-manager-plus_v1.12.0_linux_amd64_slim.tar.gz",
		"cpa-manager-plus_v1.12.0_linux_arm64_slim.tar.gz",
		"cpa-manager-plus_v1.12.0_darwin_amd64_slim.tar.gz",
		"cpa-manager-plus_v1.12.0_darwin_arm64_slim.tar.gz",
		"cpa-manager-plus_v1.12.0_windows_amd64_slim.zip",
		"cpa-manager-plus_v1.12.0_windows_arm64_slim.zip",
	} {
		writeFixture(t, filepath.Join(cpampDir, name), name)
	}
	writeFixture(t, filepath.Join(cpampDir, "cpa-manager-plus_v1.12.0_linux_amd64_full.tar.gz"), "ignored-full")
	for _, name := range []string{
		"CLIProxyAPI_7.2.92_linux_amd64_no-plugin.tar.gz",
		"CLIProxyAPI_7.2.92_linux_aarch64_no-plugin.tar.gz",
		"CLIProxyAPI_7.2.92_darwin_amd64.tar.gz",
		"CLIProxyAPI_7.2.92_darwin_aarch64.tar.gz",
		"CLIProxyAPI_7.2.92_windows_amd64.zip",
		"CLIProxyAPI_7.2.92_windows_aarch64.zip",
	} {
		writeFixture(t, filepath.Join(cpaDir, name), name)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := buildReleaseManifest(generateOptions{
		CPAMPVersion:       "v1.12.0",
		CPAVersion:         "v7.2.92",
		CPAMPAssetsDir:     cpampDir,
		CPAAssetsDir:       cpaDir,
		CPAMPRepository:    "seakee/CPA-Manager-Plus",
		CPARepository:      "router-for-me/CLIProxyAPI",
		CPAMinCPAMPVersion: "v1.12.0",
		Channel:            "stable",
		GeneratedAt:        "2026-07-29T00:00:00Z",
		PrivateKey:         privateKey,
	})
	if err != nil {
		t.Fatalf("build release manifest: %v", err)
	}
	if err := managedruntime.VerifyReleaseManifest(manifest, base64.StdEncoding.EncodeToString(publicKey)); err != nil {
		t.Fatalf("verify release manifest: %v", err)
	}
	if got := len(manifest.Components["cpamp"].Assets); got != 6 {
		t.Fatalf("CPAMP assets = %d, want 6", got)
	}
	cpaAssets := manifest.Components["cpa"].Assets
	if len(cpaAssets) != 6 {
		t.Fatalf("CPA assets = %d, want 6", len(cpaAssets))
	}
	var portable managedruntime.ReleaseAsset
	for _, asset := range cpaAssets {
		if asset.Name == "CLIProxyAPI_7.2.92_linux_aarch64_no-plugin.tar.gz" {
			portable = asset
			break
		}
	}
	if portable.Arch != "arm64" || portable.Variant != "no-plugin" || portable.Libc != "any" {
		t.Fatalf("portable CPA asset = %#v", portable)
	}
}

func TestBuildReleaseManifestRejectsPrereleaseInStableChannel(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, err = buildReleaseManifest(generateOptions{
		CPAMPVersion:    "v1.12.0",
		CPAVersion:      "v7.3.0-rc.1",
		CPAMPAssetsDir:  t.TempDir(),
		CPAAssetsDir:    t.TempDir(),
		CPAMPRepository: "seakee/CPA-Manager-Plus",
		CPARepository:   "router-for-me/CLIProxyAPI",
		Channel:         "stable",
		PrivateKey:      privateKey,
	})
	if err == nil || !strings.Contains(err.Error(), "stable channel cannot contain prerelease") {
		t.Fatalf("buildReleaseManifest() error = %v", err)
	}
}

func TestValidateReleaseAssetSetRejectsDuplicatePlatform(t *testing.T) {
	assets := []managedruntime.ReleaseAsset{
		{Name: "first.tar.gz", OS: "linux", Arch: "aarch64", Libc: "any", Variant: "no-plugin"},
		{Name: "second.tar.gz", OS: "linux", Arch: "arm64", Libc: "any", Variant: "no-plugin"},
	}
	err := validateReleaseAssetSet("CPA", assets, nil)
	if err == nil || !strings.Contains(err.Error(), "duplicate CPA runtime assets") {
		t.Fatalf("validateReleaseAssetSet() error = %v", err)
	}
}

func TestValidateReleaseAssetSetRejectsMissingPlatform(t *testing.T) {
	assets := []managedruntime.ReleaseAsset{{
		Name: "linux.tar.gz", OS: "linux", Arch: "amd64", Libc: "any", Variant: "slim",
	}}
	err := validateReleaseAssetSet("CPAMP", assets, []assetRequirement{{
		OS: "windows", Arch: "amd64", Libc: "any", Variant: "slim",
	}})
	if err == nil || !strings.Contains(err.Error(), "missing required CPAMP runtime asset") {
		t.Fatalf("validateReleaseAssetSet() error = %v", err)
	}
}

func TestVerifyExpectedPublicKeyRejectsMismatch(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherPublicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyExpectedPublicKey(privateKey, encodePublicKey(otherPublicKey)); err == nil {
		t.Fatal("verifyExpectedPublicKey() error = nil")
	}
}

func TestDecodePrivateKeyAcceptsSeed(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	privateKey, err := decodePrivateKey(base64.StdEncoding.EncodeToString(seed))
	if err != nil {
		t.Fatal(err)
	}
	if len(privateKey) != ed25519.PrivateKeySize {
		t.Fatalf("private key length = %d", len(privateKey))
	}
}

func writeFixture(t testing.TB, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
