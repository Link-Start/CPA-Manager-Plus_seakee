package adminauth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/config"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/security"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/store"
)

func TestVerifyHeaderInvalidatesCachedSuccessAfterCredentialRotation(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	firstKey := "cpamp_FirstKey_0123456789"
	first, err := security.NewAdminCredential(firstKey, "test")
	if err != nil {
		t.Fatalf("create first credential: %v", err)
	}
	if err := st.SaveAdminCredential(context.Background(), first); err != nil {
		t.Fatalf("save first credential: %v", err)
	}
	service := New(config.Config{}, st)
	if ok, err := service.VerifyHeader(context.Background(), "Bearer "+firstKey); err != nil || !ok {
		t.Fatalf("verify first credential: ok=%v err=%v", ok, err)
	}

	secondKey := "cpamp_SecondKey_0123456789"
	second, err := security.NewAdminCredential(secondKey, "rotation")
	if err != nil {
		t.Fatalf("create second credential: %v", err)
	}
	if err := st.SaveAdminCredential(context.Background(), second); err != nil {
		t.Fatalf("save second credential: %v", err)
	}
	if ok, err := service.VerifyHeader(context.Background(), "Bearer "+firstKey); err != nil || ok {
		t.Fatalf("old cached credential after rotation: ok=%v err=%v", ok, err)
	}
	if ok, err := service.VerifyHeader(context.Background(), "Bearer "+secondKey); err != nil || !ok {
		t.Fatalf("verify rotated credential: ok=%v err=%v", ok, err)
	}
}

func TestVerifyHeaderPropagatesBoundedVerificationSaturation(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	credential, err := security.NewAdminCredential("cpamp_Admin!0123456789", "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveAdminCredential(t.Context(), credential); err != nil {
		t.Fatal(err)
	}
	service := New(config.Config{}, st)
	service.verifyAdminKey = func(model.AdminCredential, string) (bool, error) {
		return false, security.ErrAdminKeyVerificationBusy
	}
	if ok, err := service.VerifyHeader(t.Context(), "Bearer cpamp_Admin!0123456789"); ok ||
		!errors.Is(err, security.ErrAdminKeyVerificationBusy) {
		t.Fatalf("verification = ok=%v err=%v", ok, err)
	}
}
