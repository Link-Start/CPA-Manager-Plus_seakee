package managedruntime

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	releaseManifestSchemaVersion = 1
	maxReleaseManifestBytes      = 4 << 20
	maxReleaseAssetBytes         = 512 << 20
	maxExtractedBinaryBytes      = 256 << 20
	maxVersionDirectoryLength    = 96
)

type ReleaseManifest struct {
	SchemaVersion int                         `json:"schemaVersion"`
	GeneratedAt   string                      `json:"generatedAt"`
	Channel       string                      `json:"channel,omitempty"`
	Components    map[string]ReleaseComponent `json:"components"`
	Signature     string                      `json:"signature"`
}

type ReleaseComponent struct {
	Version         string         `json:"version"`
	ReleaseURL      string         `json:"releaseUrl,omitempty"`
	ReleaseNotes    string         `json:"releaseNotes,omitempty"`
	MinCPAMPVersion string         `json:"minCpampVersion,omitempty"`
	Assets          []ReleaseAsset `json:"assets"`
}

type ReleaseAsset struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	SHA256     string `json:"sha256"`
	Size       int64  `json:"size,omitempty"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	Libc       string `json:"libc,omitempty"`
	Variant    string `json:"variant,omitempty"`
	Format     string `json:"format,omitempty"`
	BinaryPath string `json:"binaryPath"`
}

type RuntimePlatform struct {
	OS   string
	Arch string
	Libc string
}

type InstalledComponent struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	BinaryPath string `json:"binaryPath"`
	ReleaseURL string `json:"releaseUrl,omitempty"`
	Notes      string `json:"releaseNotes,omitempty"`
}

type ReleaseInstaller struct {
	manifestURL  string
	publicKey    string
	downloadDir  string
	componentDir string
	client       *http.Client
	platform     RuntimePlatform
}

func NewReleaseInstaller(cfg Config) *ReleaseInstaller {
	return &ReleaseInstaller{
		manifestURL:  cfg.ReleaseManifestURL,
		publicKey:    cfg.ReleasePublicKey,
		downloadDir:  cfg.ReleaseDownloadDir,
		componentDir: cfg.ComponentDir,
		client: &http.Client{
			Timeout: 10 * time.Minute,
		},
		platform: DetectRuntimePlatform(),
	}
}

func DetectRuntimePlatform() RuntimePlatform {
	platform := RuntimePlatform{OS: runtime.GOOS, Arch: normalizeRuntimeArch(runtime.GOARCH)}
	if platform.OS == "linux" {
		platform.Libc = detectLinuxLibc()
	}
	return platform
}

func (i *ReleaseInstaller) FetchManifest(ctx context.Context) (ReleaseManifest, error) {
	if strings.TrimSpace(i.manifestURL) == "" {
		return ReleaseManifest{}, errors.New("release manifest URL is not configured")
	}
	if err := validateReleaseURL(i.manifestURL); err != nil {
		return ReleaseManifest{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, i.manifestURL, nil)
	if err != nil {
		return ReleaseManifest{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := i.doReleaseRequest(req)
	if err != nil {
		return ReleaseManifest{}, fmt.Errorf("download release manifest: %w", err)
	}
	defer resp.Body.Close()
	if err := validateReleaseURL(resp.Request.URL.String()); err != nil {
		return ReleaseManifest{}, fmt.Errorf("release manifest redirect: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ReleaseManifest{}, fmt.Errorf("download release manifest: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReleaseManifestBytes+1))
	if err != nil {
		return ReleaseManifest{}, err
	}
	if len(data) > maxReleaseManifestBytes {
		return ReleaseManifest{}, errors.New("release manifest exceeds size limit")
	}
	var manifest ReleaseManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return ReleaseManifest{}, fmt.Errorf("parse release manifest: %w", err)
	}
	if err := VerifyReleaseManifest(manifest, i.publicKey); err != nil {
		return ReleaseManifest{}, err
	}
	return manifest, nil
}

func VerifyReleaseManifest(manifest ReleaseManifest, publicKey string) error {
	if manifest.SchemaVersion != releaseManifestSchemaVersion {
		return fmt.Errorf("unsupported release manifest schema version %d", manifest.SchemaVersion)
	}
	if len(manifest.Components) == 0 {
		return errors.New("release manifest contains no components")
	}
	key, err := decodeEd25519Value(publicKey, ed25519.PublicKeySize)
	if err != nil {
		return fmt.Errorf("decode release public key: %w", err)
	}
	signature, err := decodeEd25519Value(manifest.Signature, ed25519.SignatureSize)
	if err != nil {
		return fmt.Errorf("decode release manifest signature: %w", err)
	}
	unsigned := manifest
	unsigned.Signature = ""
	payload, err := json.Marshal(unsigned)
	if err != nil {
		return err
	}
	if !ed25519.Verify(ed25519.PublicKey(key), payload, signature) {
		return errors.New("release manifest signature is invalid")
	}
	channel := strings.ToLower(strings.TrimSpace(manifest.Channel))
	if channel != "stable" && channel != "prerelease" {
		return errors.New("release manifest channel must be stable or prerelease")
	}
	if _, err := time.Parse(time.RFC3339, strings.TrimSpace(manifest.GeneratedAt)); err != nil {
		return fmt.Errorf("release manifest generatedAt is invalid: %w", err)
	}
	for name, component := range manifest.Components {
		if strings.TrimSpace(name) == "" || strings.TrimSpace(component.Version) == "" {
			return errors.New("release manifest component name and version are required")
		}
		_, prerelease, ok := parseReleaseVersion(component.Version)
		if !ok {
			return fmt.Errorf("component %s: release version is invalid", name)
		}
		if channel == "stable" && len(prerelease) > 0 {
			return fmt.Errorf("component %s: stable manifest cannot contain prerelease version %q", name, component.Version)
		}
		if minimum := strings.TrimSpace(component.MinCPAMPVersion); minimum != "" {
			if _, _, ok := parseReleaseVersion(minimum); !ok {
				return fmt.Errorf("component %s: minimum CPAMP version is invalid", name)
			}
		}
		if len(component.Assets) == 0 {
			return fmt.Errorf("component %s: release manifest contains no assets", name)
		}
		assetNames := make(map[string]struct{}, len(component.Assets))
		assetPlatforms := make(map[string]string, len(component.Assets))
		for _, asset := range component.Assets {
			if err := validateReleaseAsset(asset); err != nil {
				return fmt.Errorf("component %s: %w", name, err)
			}
			if _, exists := assetNames[asset.Name]; exists {
				return fmt.Errorf("component %s: duplicate release asset name %q", name, asset.Name)
			}
			assetNames[asset.Name] = struct{}{}
			platformKey := releaseAssetPlatformKey(asset)
			if previous, exists := assetPlatforms[platformKey]; exists {
				return fmt.Errorf(
					"component %s: duplicate release assets %q and %q for %s",
					name,
					previous,
					asset.Name,
					platformKey,
				)
			}
			assetPlatforms[platformKey] = asset.Name
		}
	}
	return nil
}

func releaseAssetPlatformKey(asset ReleaseAsset) string {
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(asset.OS)),
		normalizeRuntimeArch(asset.Arch),
		strings.ToLower(strings.TrimSpace(asset.Libc)),
		strings.ToLower(strings.TrimSpace(asset.Variant)),
	}, "/")
}

func SignReleaseManifest(manifest ReleaseManifest, privateKey ed25519.PrivateKey) (ReleaseManifest, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return ReleaseManifest{}, errors.New("invalid Ed25519 private key length")
	}
	manifest.Signature = ""
	payload, err := json.Marshal(manifest)
	if err != nil {
		return ReleaseManifest{}, err
	}
	manifest.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	return manifest, nil
}

func (i *ReleaseInstaller) LatestComponent(ctx context.Context, name string) (ReleaseComponent, error) {
	manifest, err := i.FetchManifest(ctx)
	if err != nil {
		return ReleaseComponent{}, err
	}
	component, ok := manifest.Components[strings.TrimSpace(name)]
	if !ok {
		return ReleaseComponent{}, fmt.Errorf("release manifest does not contain component %q", name)
	}
	return component, nil
}

func (i *ReleaseInstaller) InstallLatest(ctx context.Context, name string) (InstalledComponent, error) {
	component, err := i.LatestComponent(ctx, name)
	if err != nil {
		return InstalledComponent{}, err
	}
	return i.InstallComponent(ctx, name, component)
}

func (i *ReleaseInstaller) InstallComponent(ctx context.Context, name string, component ReleaseComponent) (InstalledComponent, error) {
	asset, err := SelectReleaseAsset(component, i.platform)
	if err != nil {
		return InstalledComponent{}, err
	}
	archivePath, err := i.downloadAsset(ctx, asset)
	if err != nil {
		return InstalledComponent{}, err
	}
	binaryPath, err := i.installAsset(name, component.Version, asset, archivePath)
	if err != nil {
		return InstalledComponent{}, err
	}
	return InstalledComponent{
		Name:       name,
		Version:    component.Version,
		BinaryPath: binaryPath,
		ReleaseURL: component.ReleaseURL,
		Notes:      component.ReleaseNotes,
	}, nil
}

func SelectReleaseAsset(component ReleaseComponent, platform RuntimePlatform) (ReleaseAsset, error) {
	bestScore := -1
	var best ReleaseAsset
	for _, asset := range component.Assets {
		if strings.ToLower(strings.TrimSpace(asset.OS)) != strings.ToLower(platform.OS) ||
			normalizeRuntimeArch(asset.Arch) != normalizeRuntimeArch(platform.Arch) {
			continue
		}
		score := 10
		if platform.OS == "linux" {
			assetLibc := strings.ToLower(strings.TrimSpace(asset.Libc))
			variant := strings.ToLower(strings.TrimSpace(asset.Variant))
			switch {
			case variant == "no-plugin" && (assetLibc == "" || assetLibc == "any"):
				score = 100
			case platform.Libc == "glibc" && assetLibc == "glibc":
				score = 90
			case assetLibc == "any":
				score = 80
			case assetLibc == "" || assetLibc == platform.Libc:
				score = 40
			default:
				continue
			}
		}
		if score == bestScore {
			return ReleaseAsset{}, fmt.Errorf(
				"ambiguous compatible release assets %q and %q for %s/%s libc=%s",
				best.Name,
				asset.Name,
				platform.OS,
				platform.Arch,
				platform.Libc,
			)
		}
		if score > bestScore {
			bestScore = score
			best = asset
		}
	}
	if bestScore < 0 {
		return ReleaseAsset{}, fmt.Errorf(
			"no compatible release asset for %s/%s libc=%s",
			platform.OS,
			platform.Arch,
			platform.Libc,
		)
	}
	return best, nil
}

func (i *ReleaseInstaller) downloadAsset(ctx context.Context, asset ReleaseAsset) (string, error) {
	if err := validateReleaseURL(asset.URL); err != nil {
		return "", err
	}
	if err := os.MkdirAll(i.downloadDir, 0o700); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return "", err
	}
	resp, err := i.doReleaseRequest(req)
	if err != nil {
		return "", fmt.Errorf("download release asset: %w", err)
	}
	defer resp.Body.Close()
	if err := validateReleaseURL(resp.Request.URL.String()); err != nil {
		return "", fmt.Errorf("release asset redirect: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("download release asset: %s", resp.Status)
	}
	limit := int64(maxReleaseAssetBytes)
	if asset.Size > 0 && asset.Size < limit {
		limit = asset.Size
	}
	temporary, err := os.CreateTemp(i.downloadDir, ".asset-*")
	if err != nil {
		return "", err
	}
	temporaryPath := temporary.Name()
	ok := false
	defer func() {
		_ = temporary.Close()
		if !ok {
			_ = os.Remove(temporaryPath)
		}
	}()
	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(temporary, hasher), io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return "", err
	}
	if written > limit {
		return "", errors.New("release asset exceeds size limit")
	}
	if asset.Size > 0 && written != asset.Size {
		return "", fmt.Errorf("release asset size = %d, want %d", written, asset.Size)
	}
	if !strings.EqualFold(hex.EncodeToString(hasher.Sum(nil)), strings.TrimSpace(asset.SHA256)) {
		return "", errors.New("release asset checksum mismatch")
	}
	if err := temporary.Sync(); err != nil {
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	finalPath := filepath.Join(i.downloadDir, filepath.Base(asset.Name))
	if err := replaceRuntimeFile(temporaryPath, finalPath); err != nil {
		return "", err
	}
	ok = true
	return finalPath, nil
}

func (i *ReleaseInstaller) doReleaseRequest(req *http.Request) (*http.Response, error) {
	base := i.client
	if base == nil {
		base = &http.Client{Timeout: 10 * time.Minute}
	}
	client := *base
	checkRedirect := client.CheckRedirect
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if err := validateReleaseURL(next.URL.String()); err != nil {
			return fmt.Errorf("unsafe release redirect: %w", err)
		}
		if checkRedirect != nil {
			return checkRedirect(next, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 release redirects")
		}
		return nil
	}
	return client.Do(req)
}

func (i *ReleaseInstaller) installAsset(componentName string, version string, asset ReleaseAsset, archivePath string) (string, error) {
	componentName = strings.TrimSpace(componentName)
	version = strings.TrimSpace(version)
	if componentName == "" || version == "" {
		return "", errors.New("component name and version are required")
	}
	if componentName == "." || componentName == ".." || strings.ContainsAny(componentName, `/\\`) {
		return "", errors.New("component name is invalid")
	}
	componentRoot := filepath.Join(i.componentDir, componentName)
	targetDir := filepath.Join(componentRoot, safeVersionDirectory(version))
	binaryName := filepath.Base(strings.ReplaceAll(asset.BinaryPath, "\\", "/"))
	if binaryName == "." || binaryName == "" {
		return "", errors.New("release asset binary path is invalid")
	}
	targetBinary := filepath.Join(targetDir, binaryName)
	if err := os.MkdirAll(componentRoot, 0o700); err != nil {
		return "", err
	}
	temporaryDir, err := os.MkdirTemp(componentRoot, ".install-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temporaryDir)
	temporaryBinary := filepath.Join(temporaryDir, binaryName)
	if err := extractReleaseBinary(archivePath, asset, temporaryBinary); err != nil {
		return "", err
	}
	if err := os.Chmod(temporaryBinary, 0o755); err != nil {
		return "", err
	}
	if targetInfo, err := os.Lstat(targetDir); errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(temporaryDir, targetDir); err != nil {
			return "", err
		}
	} else if err == nil {
		if !targetInfo.IsDir() || targetInfo.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("component directory %s is not a regular directory", targetDir)
		}
		replaceExisting := true
		if info, statErr := os.Lstat(targetBinary); statErr == nil && info.Mode().IsRegular() {
			equal, compareErr := runtimeFilesEqual(targetBinary, temporaryBinary)
			if compareErr != nil {
				return "", compareErr
			}
			replaceExisting = !equal
		} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
		}
		if replaceExisting {
			if err := replaceRuntimeFile(temporaryBinary, targetBinary); err != nil {
				return "", fmt.Errorf("replace unverified component binary: %w", err)
			}
		}
		if err := os.Chmod(targetBinary, 0o755); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	if err := writeCurrentComponent(componentRoot, version, targetBinary); err != nil {
		return "", err
	}
	return targetBinary, nil
}

func runtimeFilesEqual(left string, right string) (bool, error) {
	leftHash, leftSize, err := runtimeFileDigest(left)
	if err != nil {
		return false, err
	}
	rightHash, rightSize, err := runtimeFileDigest(right)
	if err != nil {
		return false, err
	}
	return leftSize == rightSize && leftHash == rightHash, nil
}

func runtimeFileDigest(filePath string) ([sha256.Size]byte, int64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return [sha256.Size]byte{}, 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return [sha256.Size]byte{}, 0, err
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return [sha256.Size]byte{}, 0, err
	}
	var digest [sha256.Size]byte
	copy(digest[:], hasher.Sum(nil))
	return digest, info.Size(), nil
}

func extractReleaseBinary(archivePath string, asset ReleaseAsset, destination string) error {
	format := strings.ToLower(strings.TrimSpace(asset.Format))
	if format == "" {
		switch {
		case strings.HasSuffix(strings.ToLower(asset.Name), ".tar.gz"), strings.HasSuffix(strings.ToLower(asset.Name), ".tgz"):
			format = "tar.gz"
		case strings.HasSuffix(strings.ToLower(asset.Name), ".zip"):
			format = "zip"
		default:
			format = "binary"
		}
	}
	switch format {
	case "tar.gz", "tgz":
		return extractTarGzipBinary(archivePath, asset.BinaryPath, destination)
	case "zip":
		return extractZipBinary(archivePath, asset.BinaryPath, destination)
	case "binary":
		return copyLimitedFile(archivePath, destination, maxExtractedBinaryBytes)
	default:
		return fmt.Errorf("unsupported release asset format %q", format)
	}
}

func extractTarGzipBinary(archivePath string, expectedPath string, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg || !archiveEntryMatches(header.Name, expectedPath) {
			continue
		}
		return writeLimitedReader(destination, reader, maxExtractedBinaryBytes)
	}
	return fmt.Errorf("release archive does not contain %s", expectedPath)
}

func extractZipBinary(archivePath string, expectedPath string, destination string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() || !archiveEntryMatches(entry.Name, expectedPath) {
			continue
		}
		if entry.UncompressedSize64 > maxExtractedBinaryBytes {
			return errors.New("release binary exceeds size limit")
		}
		stream, err := entry.Open()
		if err != nil {
			return err
		}
		err = writeLimitedReader(destination, stream, maxExtractedBinaryBytes)
		_ = stream.Close()
		return err
	}
	return fmt.Errorf("release archive does not contain %s", expectedPath)
}

func archiveEntryMatches(candidate string, expected string) bool {
	candidate = strings.TrimPrefix(path.Clean(strings.ReplaceAll(candidate, "\\", "/")), "./")
	expected = strings.TrimPrefix(path.Clean(strings.ReplaceAll(expected, "\\", "/")), "./")
	if candidate == "." || strings.HasPrefix(candidate, "../") || path.IsAbs(candidate) {
		return false
	}
	if expected == "." || expected == "" {
		return path.Base(candidate) == path.Base(expected)
	}
	return candidate == expected
}

func writeLimitedReader(destination string, source io.Reader, limit int64) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(destination)
		}
	}()
	written, err := io.Copy(file, io.LimitReader(source, limit+1))
	if err != nil {
		return err
	}
	if written > limit {
		return errors.New("release binary exceeds size limit")
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}

func copyLimitedFile(source string, destination string, limit int64) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	return writeLimitedReader(destination, file, limit)
}

func writeCurrentComponent(componentRoot string, version string, binaryPath string) error {
	payload, err := json.MarshalIndent(map[string]string{
		"version":    version,
		"binaryPath": binaryPath,
	}, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return writeAtomicRuntimeFile(filepath.Join(componentRoot, "current.json"), payload, 0o600)
}

func syncCurrentComponentPointer(componentDir string, name string, version string, binaryPath string) error {
	componentDir = strings.TrimSpace(componentDir)
	name = strings.TrimSpace(name)
	version = strings.TrimSpace(version)
	binaryPath = strings.TrimSpace(binaryPath)
	if version == "" || binaryPath == "" {
		return nil
	}
	if componentDir == "" {
		return errors.New("runtime component directory is required")
	}
	if name != "cpamp" && name != "cpa" {
		return fmt.Errorf("unsupported runtime component %q", name)
	}
	return writeCurrentComponent(filepath.Join(componentDir, name), version, binaryPath)
}

func syncCurrentComponentPointers(componentDir string, components map[string]ComponentState) error {
	var result error
	for _, name := range []string{"cpamp", "cpa"} {
		component, ok := components[name]
		if !ok {
			continue
		}
		if err := syncCurrentComponentPointer(componentDir, name, component.Version, component.BinaryPath); err != nil {
			result = errors.Join(result, err)
		}
	}
	return result
}

func writeAtomicRuntimeFile(target string, payload []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".atomic-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return replaceRuntimeFile(temporaryPath, target)
}

func validateReleaseAsset(asset ReleaseAsset) error {
	if strings.TrimSpace(asset.Name) == "" || filepath.Base(asset.Name) != asset.Name {
		return errors.New("release asset name is invalid")
	}
	if err := validateReleaseURL(asset.URL); err != nil {
		return err
	}
	checksum := strings.TrimSpace(asset.SHA256)
	if len(checksum) != sha256.Size*2 {
		return errors.New("release asset SHA256 is invalid")
	}
	if _, err := hex.DecodeString(checksum); err != nil {
		return errors.New("release asset SHA256 is invalid")
	}
	if asset.Size < 0 || asset.Size > maxReleaseAssetBytes {
		return errors.New("release asset size is invalid")
	}
	if strings.TrimSpace(asset.OS) == "" || strings.TrimSpace(asset.Arch) == "" || strings.TrimSpace(asset.BinaryPath) == "" {
		return errors.New("release asset platform and binary path are required")
	}
	if !archiveEntryMatches(asset.BinaryPath, asset.BinaryPath) {
		return errors.New("release asset binary path is unsafe")
	}
	return nil
}

func validateReleaseURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return errors.New("release URL is invalid")
	}
	if parsed.Scheme == "https" {
		return nil
	}
	if parsed.Scheme == "http" {
		host := parsed.Hostname()
		if strings.EqualFold(host, "localhost") {
			return nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return nil
		}
	}
	return errors.New("release URL must use HTTPS")
}

func decodeEd25519Value(value string, expected int) ([]byte, error) {
	value = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "ed25519:"))
	if value == "" {
		return nil, errors.New("value is not configured")
	}
	for _, decoder := range []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
		hex.DecodeString,
	} {
		decoded, err := decoder(value)
		if err == nil && len(decoded) == expected {
			return decoded, nil
		}
	}
	return nil, fmt.Errorf("value must decode to %d bytes", expected)
}

func normalizeRuntimeArch(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "x86_64", "x64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func detectLinuxLibc() string {
	if output, err := exec.Command("ldd", "--version").CombinedOutput(); err == nil {
		lower := strings.ToLower(string(output))
		if strings.Contains(lower, "musl") {
			return "musl"
		}
		if strings.Contains(lower, "glibc") || strings.Contains(lower, "gnu libc") {
			return "glibc"
		}
	}
	for _, pattern := range []string{"/lib/ld-musl-*.so.1", "/usr/lib/ld-musl-*.so.1"} {
		if matches, _ := filepath.Glob(pattern); len(matches) > 0 {
			return "musl"
		}
	}
	return "glibc"
}

func safeVersionDirectory(version string) string {
	value := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(version), "v"))
	transformed := false
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_', r == '+':
			return r
		default:
			transformed = true
			return '_'
		}
	}, value)
	if safe == "" || safe == "." || safe == ".." || strings.HasSuffix(safe, ".") || windowsReservedPathName(safe) {
		safe = "version"
		transformed = true
	}
	suffix := ""
	if transformed || len(safe) > maxVersionDirectoryLength {
		sum := sha256.Sum256([]byte(value))
		suffix = "-" + hex.EncodeToString(sum[:8])
	}
	if len(safe)+len(suffix) > maxVersionDirectoryLength {
		safe = strings.TrimRight(safe[:maxVersionDirectoryLength-len(suffix)], ".")
		if safe == "" {
			safe = "version"
		}
	}
	return safe + suffix
}

func windowsReservedPathName(value string) bool {
	base, _, _ := strings.Cut(strings.ToUpper(value), ".")
	switch base {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	default:
		return false
	}
}
