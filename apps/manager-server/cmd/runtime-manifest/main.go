package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	managedruntime "github.com/seakee/cpa-manager-plus/apps/manager-server/internal/managedruntime"
)

const (
	defaultPrivateKeyEnv = "CPA_MANAGER_RELEASE_PRIVATE_KEY"
	defaultPublicKeyEnv  = "CPA_MANAGER_RELEASE_PUBLIC_KEY"
)

var (
	repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	releaseTagPattern = regexp.MustCompile(`^[vV]?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$`)
)

type assetRequirement struct {
	OS      string
	Arch    string
	Libc    string
	Variant string
}

type generateOptions struct {
	CPAMPVersion       string
	CPAVersion         string
	CPAMPAssetsDir     string
	CPAAssetsDir       string
	CPAMPRepository    string
	CPARepository      string
	CPAMPNotesPath     string
	CPANotesPath       string
	CPAMinCPAMPVersion string
	Channel            string
	GeneratedAt        string
	OutputPath         string
	PrivateKey         ed25519.PrivateKey
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: runtime-manifest <public-key|generate-release> [flags]")
	}
	switch args[0] {
	case "public-key":
		flags := flag.NewFlagSet("public-key", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		privateKeyEnv := flags.String("private-key-env", defaultPrivateKeyEnv, "environment variable containing the Ed25519 private key")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		privateKey, err := loadPrivateKeyFromEnv(*privateKeyEnv)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(output, encodePublicKey(privateKey.Public().(ed25519.PublicKey)))
		return err
	case "generate-release":
		return runGenerate(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runGenerate(args []string) error {
	flags := flag.NewFlagSet("generate-release", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	options := generateOptions{}
	privateKeyEnv := flags.String("private-key-env", defaultPrivateKeyEnv, "environment variable containing the Ed25519 private key")
	publicKeyEnv := flags.String("public-key-env", defaultPublicKeyEnv, "environment variable containing the expected Ed25519 public key")
	flags.StringVar(&options.CPAMPVersion, "cpamp-version", "", "CPA Manager Plus release tag")
	flags.StringVar(&options.CPAVersion, "cpa-version", "", "CPA release tag")
	flags.StringVar(&options.CPAMPAssetsDir, "cpamp-assets-dir", "", "directory containing CPAMP native release assets")
	flags.StringVar(&options.CPAAssetsDir, "cpa-assets-dir", "", "directory containing downloaded CPA release assets")
	flags.StringVar(&options.CPAMPRepository, "cpamp-repository", "seakee/CPA-Manager-Plus", "CPAMP GitHub repository")
	flags.StringVar(&options.CPARepository, "cpa-repository", "router-for-me/CLIProxyAPI", "CPA GitHub repository")
	flags.StringVar(&options.CPAMPNotesPath, "cpamp-release-notes", "", "CPAMP release notes file")
	flags.StringVar(&options.CPANotesPath, "cpa-release-notes", "", "CPA release notes file")
	flags.StringVar(&options.CPAMinCPAMPVersion, "cpa-min-cpamp-version", "", "minimum CPAMP version for the CPA release")
	flags.StringVar(&options.Channel, "channel", "stable", "release channel")
	flags.StringVar(&options.GeneratedAt, "generated-at", "", "manifest generation timestamp")
	flags.StringVar(&options.OutputPath, "output", "runtime-manifest.json", "manifest output path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	privateKey, err := loadPrivateKeyFromEnv(*privateKeyEnv)
	if err != nil {
		return err
	}
	if err := verifyExpectedPublicKey(privateKey, os.Getenv(*publicKeyEnv)); err != nil {
		return fmt.Errorf("verify release signing key: %w", err)
	}
	options.PrivateKey = privateKey
	manifest, err := buildReleaseManifest(options)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(options.OutputPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(options.OutputPath, data, 0o644); err != nil {
		return err
	}
	return nil
}

func buildReleaseManifest(options generateOptions) (managedruntime.ReleaseManifest, error) {
	options.CPAMPVersion = strings.TrimSpace(options.CPAMPVersion)
	options.CPAVersion = strings.TrimSpace(options.CPAVersion)
	if options.CPAMPVersion == "" || options.CPAVersion == "" {
		return managedruntime.ReleaseManifest{}, errors.New("CPAMP and CPA versions are required")
	}
	if !releaseTagPattern.MatchString(options.CPAMPVersion) || !releaseTagPattern.MatchString(options.CPAVersion) {
		return managedruntime.ReleaseManifest{}, errors.New("CPAMP and CPA versions must be semantic release tags")
	}
	if minimum := strings.TrimSpace(options.CPAMinCPAMPVersion); minimum != "" && !releaseTagPattern.MatchString(minimum) {
		return managedruntime.ReleaseManifest{}, errors.New("minimum CPAMP version must be a semantic release tag")
	}
	channel, err := normalizeReleaseChannel(options.Channel, options.CPAMPVersion, options.CPAVersion)
	if err != nil {
		return managedruntime.ReleaseManifest{}, err
	}
	if !repositoryPattern.MatchString(options.CPAMPRepository) || !repositoryPattern.MatchString(options.CPARepository) {
		return managedruntime.ReleaseManifest{}, errors.New("GitHub repositories must use owner/name syntax")
	}
	cpampAssets, err := scanCPAMPAssets(options)
	if err != nil {
		return managedruntime.ReleaseManifest{}, err
	}
	cpaAssets, err := scanCPAAssets(options)
	if err != nil {
		return managedruntime.ReleaseManifest{}, err
	}
	cpampNotes, err := readOptionalText(options.CPAMPNotesPath)
	if err != nil {
		return managedruntime.ReleaseManifest{}, err
	}
	cpaNotes, err := readOptionalText(options.CPANotesPath)
	if err != nil {
		return managedruntime.ReleaseManifest{}, err
	}
	generatedAt := strings.TrimSpace(options.GeneratedAt)
	if generatedAt == "" {
		generatedAt = time.Now().UTC().Format(time.RFC3339)
	} else if _, err := time.Parse(time.RFC3339, generatedAt); err != nil {
		return managedruntime.ReleaseManifest{}, fmt.Errorf("parse generated-at: %w", err)
	}
	manifest := managedruntime.ReleaseManifest{
		SchemaVersion: 1,
		GeneratedAt:   generatedAt,
		Channel:       channel,
		Components: map[string]managedruntime.ReleaseComponent{
			"cpamp": {
				Version:      options.CPAMPVersion,
				ReleaseURL:   releasePageURL(options.CPAMPRepository, options.CPAMPVersion),
				ReleaseNotes: cpampNotes,
				Assets:       cpampAssets,
			},
			"cpa": {
				Version:         options.CPAVersion,
				ReleaseURL:      releasePageURL(options.CPARepository, options.CPAVersion),
				ReleaseNotes:    cpaNotes,
				MinCPAMPVersion: strings.TrimSpace(options.CPAMinCPAMPVersion),
				Assets:          cpaAssets,
			},
		},
	}
	return managedruntime.SignReleaseManifest(manifest, options.PrivateKey)
}

func scanCPAMPAssets(options generateOptions) ([]managedruntime.ReleaseAsset, error) {
	entries, err := os.ReadDir(options.CPAMPAssetsDir)
	if err != nil {
		return nil, fmt.Errorf("read CPAMP assets: %w", err)
	}
	prefix := "cpa-manager-plus_" + options.CPAMPVersion + "_"
	assets := make([]managedruntime.ReleaseAsset, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		base, format, ok := archiveBase(name)
		if !ok || !strings.HasPrefix(base, prefix) {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(base, prefix), "_")
		if len(parts) != 3 || parts[2] != "slim" || !supportedPlatform(parts[0], parts[1]) {
			continue
		}
		binaryName := "cpa-manager-plus"
		if parts[0] == "windows" {
			binaryName += ".exe"
		}
		asset, err := releaseAsset(
			filepath.Join(options.CPAMPAssetsDir, name),
			assetDownloadURL(options.CPAMPRepository, options.CPAMPVersion, name),
			name,
			parts[0],
			parts[1],
			"any",
			"slim",
			format,
			base+"/"+binaryName,
		)
		if err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}
	if len(assets) == 0 {
		return nil, errors.New("no CPAMP Slim release assets were found")
	}
	sortReleaseAssets(assets)
	if err := validateReleaseAssetSet("CPAMP", assets, []assetRequirement{
		{OS: "linux", Arch: "amd64", Libc: "any", Variant: "slim"},
		{OS: "linux", Arch: "arm64", Libc: "any", Variant: "slim"},
		{OS: "darwin", Arch: "amd64", Libc: "any", Variant: "slim"},
		{OS: "darwin", Arch: "arm64", Libc: "any", Variant: "slim"},
		{OS: "windows", Arch: "amd64", Libc: "any", Variant: "slim"},
		{OS: "windows", Arch: "arm64", Libc: "any", Variant: "slim"},
	}); err != nil {
		return nil, err
	}
	return assets, nil
}

func scanCPAAssets(options generateOptions) ([]managedruntime.ReleaseAsset, error) {
	entries, err := os.ReadDir(options.CPAAssetsDir)
	if err != nil {
		return nil, fmt.Errorf("read CPA assets: %w", err)
	}
	prefix := "CLIProxyAPI_" + strings.TrimLeft(options.CPAVersion, "vV") + "_"
	assets := make([]managedruntime.ReleaseAsset, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		base, format, ok := archiveBase(name)
		if !ok || !strings.HasPrefix(base, prefix) {
			continue
		}
		parts := strings.Split(strings.TrimPrefix(base, prefix), "_")
		if len(parts) < 2 || len(parts) > 3 {
			continue
		}
		arch := normalizeAssetArch(parts[1])
		if !supportedPlatform(parts[0], arch) {
			continue
		}
		variant := "plugin"
		libc := ""
		if parts[0] == "linux" {
			libc = "glibc"
		}
		if len(parts) == 3 {
			if parts[2] != "no-plugin" || parts[0] != "linux" {
				continue
			}
			variant = "no-plugin"
			libc = "any"
		}
		binaryPath := "cli-proxy-api"
		if parts[0] == "windows" {
			binaryPath += ".exe"
		}
		asset, err := releaseAsset(
			filepath.Join(options.CPAAssetsDir, name),
			assetDownloadURL(options.CPARepository, options.CPAVersion, name),
			name,
			parts[0],
			arch,
			libc,
			variant,
			format,
			binaryPath,
		)
		if err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}
	if len(assets) == 0 {
		return nil, errors.New("no compatible CPA release assets were found")
	}
	sortReleaseAssets(assets)
	if err := validateReleaseAssetSet("CPA", assets, []assetRequirement{
		{OS: "linux", Arch: "amd64", Libc: "any", Variant: "no-plugin"},
		{OS: "linux", Arch: "arm64", Libc: "any", Variant: "no-plugin"},
		{OS: "darwin", Arch: "amd64", Variant: "plugin"},
		{OS: "darwin", Arch: "arm64", Variant: "plugin"},
		{OS: "windows", Arch: "amd64", Variant: "plugin"},
		{OS: "windows", Arch: "arm64", Variant: "plugin"},
	}); err != nil {
		return nil, err
	}
	return assets, nil
}

func validateReleaseAssetSet(component string, assets []managedruntime.ReleaseAsset, required []assetRequirement) error {
	seen := make(map[string]string, len(assets))
	for _, asset := range assets {
		key := releaseAssetKey(asset.OS, asset.Arch, asset.Libc, asset.Variant)
		if previous, ok := seen[key]; ok {
			return fmt.Errorf("duplicate %s runtime assets %q and %q for %s", component, previous, asset.Name, key)
		}
		seen[key] = asset.Name
	}
	for _, requirement := range required {
		key := releaseAssetKey(requirement.OS, requirement.Arch, requirement.Libc, requirement.Variant)
		if _, ok := seen[key]; !ok {
			return fmt.Errorf("missing required %s runtime asset for %s", component, key)
		}
	}
	return nil
}

func releaseAssetKey(goos string, arch string, libc string, variant string) string {
	return strings.Join([]string{
		strings.ToLower(strings.TrimSpace(goos)),
		normalizeAssetArch(arch),
		strings.ToLower(strings.TrimSpace(libc)),
		strings.ToLower(strings.TrimSpace(variant)),
	}, "/")
}

func normalizeReleaseChannel(channel string, versions ...string) (string, error) {
	channel = strings.ToLower(strings.TrimSpace(channel))
	switch channel {
	case "stable":
		for _, version := range versions {
			if releaseTagHasPrerelease(version) {
				return "", fmt.Errorf("stable channel cannot contain prerelease version %q", version)
			}
		}
	case "prerelease":
	default:
		return "", errors.New("release channel must be stable or prerelease")
	}
	return channel, nil
}

func releaseTagHasPrerelease(version string) bool {
	version = strings.TrimLeft(strings.TrimSpace(version), "vV")
	withoutBuild, _, _ := strings.Cut(version, "+")
	return strings.Contains(withoutBuild, "-")
}

func releaseAsset(source string, downloadURL string, name string, goos string, arch string, libc string, variant string, format string, binaryPath string) (managedruntime.ReleaseAsset, error) {
	file, err := os.Open(source)
	if err != nil {
		return managedruntime.ReleaseAsset{}, err
	}
	defer file.Close()
	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return managedruntime.ReleaseAsset{}, err
	}
	return managedruntime.ReleaseAsset{
		Name:       name,
		URL:        downloadURL,
		SHA256:     hex.EncodeToString(hasher.Sum(nil)),
		Size:       size,
		OS:         goos,
		Arch:       arch,
		Libc:       libc,
		Variant:    variant,
		Format:     format,
		BinaryPath: binaryPath,
	}, nil
}

func sortReleaseAssets(assets []managedruntime.ReleaseAsset) {
	sort.Slice(assets, func(i, j int) bool {
		return assets[i].Name < assets[j].Name
	})
}

func archiveBase(name string) (string, string, bool) {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"):
		return name[:len(name)-len(".tar.gz")], "tar.gz", true
	case strings.HasSuffix(lower, ".zip"):
		return name[:len(name)-len(".zip")], "zip", true
	default:
		return "", "", false
	}
}

func supportedPlatform(goos string, arch string) bool {
	if arch != "amd64" && arch != "arm64" {
		return false
	}
	switch goos {
	case "linux", "darwin", "windows":
		return true
	default:
		return false
	}
}

func normalizeAssetArch(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "aarch64", "arm64":
		return "arm64"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func releasePageURL(repository string, version string) string {
	return "https://github.com/" + repository + "/releases/tag/" + version
}

func assetDownloadURL(repository string, version string, name string) string {
	return "https://github.com/" + repository + "/releases/download/" + version + "/" + name
}

func readOptionalText(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func loadPrivateKeyFromEnv(name string) (ed25519.PrivateKey, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("private key environment variable name is required")
	}
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return nil, fmt.Errorf("%s is not configured", name)
	}
	return decodePrivateKey(value)
}

func decodePrivateKey(value string) (ed25519.PrivateKey, error) {
	value = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "ed25519:"))
	for _, decoder := range []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
		hex.DecodeString,
	} {
		decoded, err := decoder(value)
		if err != nil {
			continue
		}
		switch len(decoded) {
		case ed25519.SeedSize:
			return ed25519.NewKeyFromSeed(decoded), nil
		case ed25519.PrivateKeySize:
			return ed25519.PrivateKey(decoded), nil
		}
	}
	return nil, errors.New("release private key must decode to a 32-byte Ed25519 seed or 64-byte private key")
}

func verifyExpectedPublicKey(privateKey ed25519.PrivateKey, expected string) error {
	expected = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(expected), "ed25519:"))
	if expected == "" {
		return errors.New("release public key is not configured")
	}
	for _, decoder := range []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
		hex.DecodeString,
	} {
		decoded, err := decoder(expected)
		if err == nil && len(decoded) == ed25519.PublicKeySize {
			actual := privateKey.Public().(ed25519.PublicKey)
			if !strings.EqualFold(hex.EncodeToString(decoded), hex.EncodeToString(actual)) {
				return errors.New("configured public key does not match the signing private key")
			}
			return nil
		}
	}
	return errors.New("release public key must decode to 32 bytes")
}

func encodePublicKey(publicKey ed25519.PublicKey) string {
	return base64.StdEncoding.EncodeToString(publicKey)
}
