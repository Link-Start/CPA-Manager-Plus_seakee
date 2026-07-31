package managedruntime

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadOrCreateSecretRestrictsExistingFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix permission bits")
	}
	path := filepath.Join(t.TempDir(), "control.key")
	if err := os.WriteFile(path, []byte("runtime-secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	value, err := loadOrCreateSecret(path, "unused_")
	if err != nil {
		t.Fatal(err)
	}
	if value != "runtime-secret" {
		t.Fatalf("secret = %q", value)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("secret permissions = %o, want 600", got)
	}
}
