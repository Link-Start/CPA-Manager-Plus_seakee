package managedruntime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestSelectReleaseAssetUsesPortableLinuxBuild(t *testing.T) {
	component := ReleaseComponent{Assets: []ReleaseAsset{
		{Name: "glibc.tar.gz", OS: "linux", Arch: "amd64", Libc: "glibc", Variant: "plugin"},
		{Name: "portable.tar.gz", OS: "linux", Arch: "amd64", Libc: "any", Variant: "no-plugin"},
	}}
	for _, libc := range []string{"musl", "glibc"} {
		t.Run(libc, func(t *testing.T) {
			asset, err := SelectReleaseAsset(component, RuntimePlatform{OS: "linux", Arch: "x86_64", Libc: libc})
			if err != nil {
				t.Fatalf("select release asset: %v", err)
			}
			if asset.Name != "portable.tar.gz" {
				t.Fatalf("asset = %q, want portable.tar.gz", asset.Name)
			}
		})
	}
}

func TestSelectReleaseAssetAcceptsLibcIndependentCPAMPBuild(t *testing.T) {
	component := ReleaseComponent{Assets: []ReleaseAsset{
		{Name: "cpamp-linux.tar.gz", OS: "linux", Arch: "amd64", Libc: "any", Variant: "slim"},
	}}
	for _, libc := range []string{"musl", "glibc"} {
		t.Run(libc, func(t *testing.T) {
			asset, err := SelectReleaseAsset(component, RuntimePlatform{OS: "linux", Arch: "amd64", Libc: libc})
			if err != nil {
				t.Fatalf("select release asset: %v", err)
			}
			if asset.Name != "cpamp-linux.tar.gz" {
				t.Fatalf("asset = %q, want cpamp-linux.tar.gz", asset.Name)
			}
		})
	}
}

func TestReleaseInstallerVerifiesManifestChecksumAndInstallsBinary(t *testing.T) {
	archive := buildTarGzip(t, "cli-proxy-api", []byte("test-cpa-binary"))
	checksum := sha256.Sum256(archive)
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var manifest ReleaseManifest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/runtime-manifest.json":
			_ = json.NewEncoder(w).Encode(manifest)
		case "/cpa.tar.gz":
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	manifest, err = SignReleaseManifest(ReleaseManifest{
		SchemaVersion: releaseManifestSchemaVersion,
		GeneratedAt:   "2026-07-29T00:00:00Z",
		Channel:       "stable",
		Components: map[string]ReleaseComponent{
			"cpa": {
				Version: "v7.2.0",
				Assets: []ReleaseAsset{{
					Name:       "cpa.tar.gz",
					URL:        server.URL + "/cpa.tar.gz",
					SHA256:     hex.EncodeToString(checksum[:]),
					Size:       int64(len(archive)),
					OS:         "linux",
					Arch:       "amd64",
					Libc:       "glibc",
					Variant:    "plugin",
					Format:     "tar.gz",
					BinaryPath: "cli-proxy-api",
				}},
			},
		},
	}, privateKey)
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	installer := &ReleaseInstaller{
		manifestURL:  server.URL + "/runtime-manifest.json",
		publicKey:    base64.StdEncoding.EncodeToString(publicKey),
		downloadDir:  filepath.Join(root, "downloads"),
		componentDir: filepath.Join(root, "components"),
		client:       server.Client(),
		platform:     RuntimePlatform{OS: "linux", Arch: "amd64", Libc: "glibc"},
	}
	installed, err := installer.InstallLatest(context.Background(), "cpa")
	if err != nil {
		t.Fatalf("install latest CPA: %v", err)
	}
	if installed.Version != "v7.2.0" {
		t.Fatalf("installed version = %q", installed.Version)
	}
	data, err := os.ReadFile(installed.BinaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "test-cpa-binary" {
		t.Fatalf("installed binary = %q", data)
	}
	if _, err := os.Stat(filepath.Join(root, "components", "cpa", "current.json")); err != nil {
		t.Fatalf("current component pointer: %v", err)
	}
	reinstalled, err := installer.InstallLatest(context.Background(), "cpa")
	if err != nil {
		t.Fatalf("retry installed CPA version: %v", err)
	}
	if reinstalled.BinaryPath != installed.BinaryPath || reinstalled.Version != installed.Version {
		t.Fatalf("reinstalled component = %+v, want %+v", reinstalled, installed)
	}
	if err := os.WriteFile(installed.BinaryPath, []byte("tampered-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	repaired, err := installer.InstallLatest(context.Background(), "cpa")
	if err != nil {
		t.Fatalf("repair installed CPA version: %v", err)
	}
	repairedData, err := os.ReadFile(repaired.BinaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(repairedData) != "test-cpa-binary" {
		t.Fatalf("repaired binary = %q", repairedData)
	}
}

func TestSelectReleaseAssetRejectsAmbiguousMatches(t *testing.T) {
	component := ReleaseComponent{Assets: []ReleaseAsset{
		{Name: "first.zip", OS: "windows", Arch: "amd64"},
		{Name: "second.zip", OS: "windows", Arch: "amd64"},
	}}
	if _, err := SelectReleaseAsset(component, RuntimePlatform{OS: "windows", Arch: "amd64"}); err == nil ||
		!strings.Contains(err.Error(), "ambiguous compatible release assets") {
		t.Fatalf("SelectReleaseAsset() error = %v", err)
	}
}

func TestVerifyReleaseManifestRejectsTampering(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := SignReleaseManifest(ReleaseManifest{
		SchemaVersion: releaseManifestSchemaVersion,
		Components: map[string]ReleaseComponent{
			"cpa": {Version: "v7.2.0"},
		},
	}, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Components["cpa"] = ReleaseComponent{Version: "v7.2.1"}
	if err := VerifyReleaseManifest(manifest, base64.StdEncoding.EncodeToString(publicKey)); err == nil {
		t.Fatal("VerifyReleaseManifest() error = nil after tampering")
	}
}

func TestVerifyReleaseManifestRejectsPrereleaseInStableChannel(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := SignReleaseManifest(ReleaseManifest{
		SchemaVersion: releaseManifestSchemaVersion,
		GeneratedAt:   "2026-07-29T00:00:00Z",
		Channel:       "stable",
		Components: map[string]ReleaseComponent{
			"cpa": {
				Version: "v7.3.0-rc.1",
				Assets: []ReleaseAsset{{
					Name:       "cpa.tar.gz",
					URL:        "https://releases.example/cpa.tar.gz",
					SHA256:     strings.Repeat("0", sha256.Size*2),
					OS:         "linux",
					Arch:       "amd64",
					Libc:       "any",
					Variant:    "no-plugin",
					BinaryPath: "cli-proxy-api",
				}},
			},
		},
	}, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	err = VerifyReleaseManifest(manifest, base64.StdEncoding.EncodeToString(publicKey))
	if err == nil || !strings.Contains(err.Error(), "stable manifest cannot contain prerelease") {
		t.Fatalf("VerifyReleaseManifest() error = %v", err)
	}
}

func TestVerifyReleaseManifestRejectsNonStandardCoreVersion(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := SignReleaseManifest(ReleaseManifest{
		SchemaVersion: releaseManifestSchemaVersion,
		GeneratedAt:   "2026-07-29T00:00:00Z",
		Channel:       "prerelease",
		Components: map[string]ReleaseComponent{
			"cpa": {
				Version: "v7.2",
				Assets: []ReleaseAsset{{
					Name:       "cpa.tar.gz",
					URL:        "https://releases.example/cpa.tar.gz",
					SHA256:     strings.Repeat("0", sha256.Size*2),
					OS:         "linux",
					Arch:       "amd64",
					Libc:       "any",
					Variant:    "no-plugin",
					BinaryPath: "cli-proxy-api",
				}},
			},
		},
	}, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyReleaseManifest(manifest, base64.StdEncoding.EncodeToString(publicKey)); err == nil ||
		!strings.Contains(err.Error(), "release version is invalid") {
		t.Fatalf("VerifyReleaseManifest() error = %v", err)
	}
}

func TestReleaseInstallerRejectsUnsafeRedirectBeforeFollowingIt(t *testing.T) {
	unsafeRequests := 0
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "169.254.169.254" {
			unsafeRequests++
		}
		response := &http.Response{
			StatusCode: http.StatusFound,
			Status:     "302 Found",
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(nil)),
			Request:    request,
		}
		response.Header.Set("Location", "http://169.254.169.254/latest/meta-data")
		return response, nil
	})}
	installer := &ReleaseInstaller{
		manifestURL: "https://releases.example/runtime-manifest.json",
		publicKey:   "unused",
		client:      client,
	}
	_, err := installer.FetchManifest(context.Background())
	if err == nil || !strings.Contains(err.Error(), "unsafe release redirect") {
		t.Fatalf("FetchManifest() error = %v", err)
	}
	if unsafeRequests != 0 {
		t.Fatalf("unsafe redirect target received %d requests", unsafeRequests)
	}
}

func TestArchiveEntryMatchesRejectsTraversal(t *testing.T) {
	if archiveEntryMatches("../cli-proxy-api", "../cli-proxy-api") {
		t.Fatal("traversal archive entry was accepted")
	}
}

func TestWriteAtomicRuntimeFileReplacesExistingTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), "current.json")
	if err := writeAtomicRuntimeFile(target, []byte("old\n"), 0o600); err != nil {
		t.Fatalf("write initial file: %v", err)
	}
	if err := writeAtomicRuntimeFile(target, []byte("new\n"), 0o600); err != nil {
		t.Fatalf("replace existing file: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new\n" {
		t.Fatalf("target contents = %q", data)
	}
}

func TestSafeVersionDirectoryPreservesSemVerAndAvoidsCollisions(t *testing.T) {
	if got := safeVersionDirectory("v1.2.3+linux.1"); got != "1.2.3+linux.1" {
		t.Fatalf("safeVersionDirectory() = %q, want build metadata preserved", got)
	}
	left := safeVersionDirectory("v1:2")
	right := safeVersionDirectory("v1?2")
	if left == right {
		t.Fatalf("transformed version directories collided: %q", left)
	}
	if left == "1_2" || right == "1_2" {
		t.Fatalf("transformed version directory lacks collision suffix: left=%q right=%q", left, right)
	}
}

func TestSafeVersionDirectoryCannotEscapeComponentRoot(t *testing.T) {
	root := t.TempDir()
	for _, version := range []string{
		"v.",
		"v..",
		"vCON",
		"v1.2.3.",
		"v" + strings.Repeat("1", maxVersionDirectoryLength*2),
	} {
		directory := safeVersionDirectory(version)
		if directory == "" || directory == "." || directory == ".." || len(directory) > maxVersionDirectoryLength {
			t.Fatalf("unsafe version directory for %q: %q", version, directory)
		}
		target := filepath.Join(root, directory)
		relative, err := filepath.Rel(root, target)
		if err != nil {
			t.Fatal(err)
		}
		if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			t.Fatalf("version %q escaped component root via %q", version, directory)
		}
	}
}

func buildTarGzip(t testing.TB, name string, content []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gz)
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: name,
		Mode: 0o755,
		Size: int64(len(content)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
