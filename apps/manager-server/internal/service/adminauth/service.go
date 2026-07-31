package adminauth

import (
	"context"
	"crypto/sha256"
	"errors"
	"strconv"
	"sync"

	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/config"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/model"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/security"
	"github.com/seakee/cpa-manager-plus/apps/manager-server/internal/store"
)

type Service struct {
	cfg            config.Config
	store          *store.Store
	verifyAdminKey func(model.AdminCredential, string) (bool, error)

	cacheMu sync.RWMutex
	cache   adminVerificationCache
}

type adminVerificationCache struct {
	credential [sha256.Size]byte
	token      [sha256.Size]byte
	valid      bool
}

func New(cfg config.Config, store *store.Store) *Service {
	return &Service{cfg: cfg, store: store, verifyAdminKey: security.VerifyAdminKeyBounded}
}

func (s *Service) VerifyHeader(ctx context.Context, authorizationHeader string) (bool, error) {
	token := security.ExtractBearerToken(authorizationHeader)
	if token == "" {
		return false, nil
	}
	credential, ok, err := s.store.LoadAdminCredential(ctx)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, errors.New("admin credential is not initialized")
	}
	credentialFingerprint := fingerprintAdminCredential(credential)
	tokenFingerprint := sha256.Sum256([]byte(token))
	s.cacheMu.RLock()
	cached := s.cache.valid &&
		s.cache.credential == credentialFingerprint &&
		s.cache.token == tokenFingerprint
	s.cacheMu.RUnlock()
	if cached {
		return true, nil
	}
	verifyAdminKey := s.verifyAdminKey
	if verifyAdminKey == nil {
		verifyAdminKey = security.VerifyAdminKeyBounded
	}
	verified, err := verifyAdminKey(credential, token)
	if err != nil {
		return false, err
	}
	if !verified {
		return false, nil
	}
	s.cacheMu.Lock()
	s.cache = adminVerificationCache{
		credential: credentialFingerprint,
		token:      tokenFingerprint,
		valid:      true,
	}
	s.cacheMu.Unlock()
	return true, nil
}

func (s *Service) VerifyPanelHeader(ctx context.Context, authorizationHeader string) (bool, error) {
	return s.VerifyHeader(ctx, authorizationHeader)
}

func (s *Service) VerifySubmittedExternalConfigHeader(ctx context.Context, authorizationHeader string, cfg store.ManagerConfig) (bool, error) {
	return s.VerifyHeader(ctx, authorizationHeader)
}

func (s *Service) PanelUsesExternalManagementKey(ctx context.Context) (bool, error) {
	return false, nil
}

func fingerprintAdminCredential(credential model.AdminCredential) [sha256.Size]byte {
	hasher := sha256.New()
	for _, value := range []string{
		strconv.Itoa(credential.Version),
		credential.Salt,
		credential.KeyHash,
		strconv.Itoa(credential.Iterations),
		strconv.FormatInt(credential.CreatedAtMS, 10),
		strconv.FormatInt(credential.RotatedAtMS, 10),
		credential.Source,
	} {
		_, _ = hasher.Write([]byte(value))
		_, _ = hasher.Write([]byte{0})
	}
	var result [sha256.Size]byte
	copy(result[:], hasher.Sum(nil))
	return result
}
