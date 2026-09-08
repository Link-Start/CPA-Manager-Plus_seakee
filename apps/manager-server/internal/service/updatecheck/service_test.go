package updatecheck

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memoryStore struct {
	mu     sync.Mutex
	data   []byte
	claims map[string]bool
	fail   bool
}

func (m *memoryStore) LoadUpdateCheck(context.Context) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]byte(nil), m.data...), nil
}
func (m *memoryStore) SaveUpdateCheck(_ context.Context, b []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return errors.New("disk full")
	}
	m.data = append([]byte(nil), b...)
	return nil
}
func (m *memoryStore) ClaimUpdateNotification(_ context.Context, tag string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail {
		return false, errors.New("disk full")
	}
	if m.claims == nil {
		m.claims = map[string]bool{}
	}
	if m.claims[tag] {
		return false, nil
	}
	m.claims[tag] = true
	return true, nil
}
func (m *memoryStore) DismissUpdateNotification(context.Context, string) error { return nil }
func fixture(tag string) ReleaseInfo {
	var info ReleaseInfo
	info.SchemaVersion = 1
	info.Release.Version = tag
	v, _ := ParseVersion(tag)
	info.Release.Stage = v.Stage
	info.Release.SourceCommit = "0123456789012345678901234567890123456789"
	info.Content.Summary = map[string]string{"zh": "更新", "en": "Update"}
	info.Content.Notes = map[string]string{}
	for _, lang := range []string{"zh", "en"} {
		info.Content.Notes[lang] = Repository + "/blob/" + tag + "/docs/release-notes/" + tag + "-" + lang + ".md"
	}
	info.Update.UpgradeGuideURL = info.Content.Notes["zh"]
	info.Distribution.Docker.Image = "seakee/cpa-manager-plus"
	info.Distribution.Docker.VersionTag = tag
	for _, os := range []string{"darwin", "linux", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			ext := ".tar.gz"
			if os == "windows" {
				ext = ".zip"
			}
			info.Distribution.Native.Assets = append(info.Distribution.Native.Assets, "cpa-manager-plus_"+tag+"_"+os+"_"+arch+ext)
		}
	}
	return info
}
func testService(t *testing.T, current, tag string, m *memoryStore) (*Service, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	downloads, indices := &atomic.Int32{}, &atomic.Int32{}
	info := fixture(tag)
	idx := Index{SchemaVersion: 1, Revision: 1, GeneratedAt: time.Now(), Channels: map[string]*Target{"stable": {Version: tag}, "rc": {Version: tag}, "beta": {Version: tag}}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index" {
			indices.Add(1)
			_ = json.NewEncoder(w).Encode(idx)
		} else {
			downloads.Add(1)
			_ = json.NewEncoder(w).Encode(info)
		}
	}))
	t.Cleanup(srv.Close)
	s := New(m, current, "test", true)
	s.client = srv.Client()
	s.indexURL = srv.URL + "/index"
	s.releaseURL = func(string) string { return srv.URL + "/info" }
	return s, downloads, indices
}
func TestSharedVersionCorpus(t *testing.T) {
	data, err := os.ReadFile("../../../../../tests/fixtures/update-versions.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Ordered []string
		Equal   [][]string
		Invalid []string
	}
	if err = json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(corpus.Ordered); i++ {
		a, e := ParseVersion(corpus.Ordered[i-1])
		if e != nil {
			t.Fatal(e)
		}
		b, e := ParseVersion(corpus.Ordered[i])
		if e != nil {
			t.Fatal(e)
		}
		if Compare(a, b) >= 0 {
			t.Fatal(corpus.Ordered[i])
		}
	}
	for _, pair := range corpus.Equal {
		a, _ := ParseVersion(pair[0])
		b, _ := ParseVersion(pair[1])
		if Compare(a, b) != 0 {
			t.Fatal(pair)
		}
	}
	for _, tag := range corpus.Invalid {
		if _, err := ParseVersion(tag); err == nil {
			t.Fatal(tag)
		}
	}
}
func TestConcurrentChecksAndNotificationsSurviveRestart(t *testing.T) {
	ctx := context.Background()
	m := &memoryStore{}
	s, downloads, indices := testService(t, "v1.0.0", "v1.1.0", m)
	var wg sync.WaitGroup
	var claims atomic.Int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Check(ctx); err != nil {
				t.Error(err)
			}
			if info, err := s.Claim(ctx); err != nil {
				t.Error(err)
			} else if info != nil {
				claims.Add(1)
			}
		}()
	}
	wg.Wait()
	if downloads.Load() != 1 || indices.Load() != 1 || claims.Load() != 1 {
		t.Fatalf("downloads=%d indices=%d claims=%d", downloads.Load(), indices.Load(), claims.Load())
	}
	_ = s.Dismiss(ctx, "v1.1.0")
	restarted, d2, _ := testService(t, "v1.0.0", "v1.1.0", m)
	restarted.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	if _, err := restarted.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if info, err := restarted.Claim(ctx); info != nil || err != nil {
		t.Fatalf("repeated notification: %v %v", info, err)
	}
	if d2.Load() != 0 {
		t.Fatal("download after restart")
	}
}
func TestEqualAheadUnknownAndPersistenceFailure(t *testing.T) {
	for _, tc := range []struct {
		current, state string
		downloads      int32
	}{{"v1.1.0", "up_to_date", 1}, {"v2.0.0", "ahead_of_channel", 0}, {"dev", "unknown_version", 0}} {
		t.Run(tc.current, func(t *testing.T) {
			s, d, _ := testService(t, tc.current, "v1.1.0", &memoryStore{})
			st, err := s.Check(context.Background())
			if err != nil || st.State != tc.state || d.Load() != tc.downloads {
				t.Fatalf("%+v %v %d", st, err, d.Load())
			}
			info, _ := s.Claim(context.Background())
			if info != nil {
				t.Fatal("unexpected notification")
			}
		})
	}
	m := &memoryStore{fail: true}
	s, d, _ := testService(t, "v1.0.0", "v1.1.0", m)
	if _, err := s.Check(context.Background()); err == nil {
		t.Fatal("expected persistence error")
	}
	if info, _ := s.Claim(context.Background()); info != nil {
		t.Fatal("claim before durable metadata")
	}
	s.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	if _, err := s.Check(context.Background()); err == nil || d.Load() != 1 {
		t.Fatal("persistent storage failure redownloaded cached metadata")
	}
	m.fail = false
	if recovered, err := s.Check(context.Background()); err != nil || recovered.LastError != "" {
		t.Fatalf("persistence did not recover: %+v %v", recovered, err)
	}
	if d.Load() != 1 {
		t.Fatal("redownload after persistence error")
	}
	if info, err := s.Claim(context.Background()); info == nil || err != nil {
		t.Fatalf("notification suppressed after persistence recovered: %v %v", info, err)
	}
}
func TestChannelMigrationAndBadMetadata(t *testing.T) {
	ctx := context.Background()
	s, _, _ := testService(t, "v1.0.0", "v2.0.0", &memoryStore{})
	st, _ := s.Check(ctx)
	if st.UpgradeAction != "migration_guide" {
		t.Fatal(st)
	}
	if _, err := s.SetChannel(ctx, "bogus"); err == nil {
		t.Fatal("invalid channel")
	}
	if _, err := s.SetChannel(ctx, "beta"); err != nil {
		t.Fatal(err)
	}
	info := fixture("v2.0.0")
	info.Release.Version = "v3.0.0"
	if info.Validate("v2.0.0") == nil {
		t.Fatal("mismatched release")
	}
}

func TestSetChannelChecksNewChannelImmediatelyWithIndependentCooldowns(t *testing.T) {
	ctx := context.Background()
	clock := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	stable := fixture("v1.1.0")
	beta := fixture("v1.2.0-beta.1")
	index := Index{
		SchemaVersion: 1,
		Revision:      1,
		GeneratedAt:   clock,
		Channels: map[string]*Target{
			"stable": {Version: stable.Release.Version},
			"rc":     {Version: "v1.1.0-rc.1"},
			"beta":   {Version: beta.Release.Version},
		},
	}
	var indexRequests, stableDownloads, betaDownloads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index" {
			indexRequests.Add(1)
			_ = json.NewEncoder(w).Encode(index)
			return
		}
		switch r.URL.Path {
		case "/v1.1.0":
			stableDownloads.Add(1)
			_ = json.NewEncoder(w).Encode(stable)
		case "/v1.2.0-beta.1":
			betaDownloads.Add(1)
			_ = json.NewEncoder(w).Encode(beta)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	store := &memoryStore{}
	s := New(store, "v1.0.0", "test", false)
	s.now = func() time.Time { return clock }
	s.client = server.Client()
	s.indexURL = server.URL + "/index"
	s.releaseURL = func(tag string) string { return server.URL + "/" + tag }

	if status, err := s.Check(ctx); err != nil || status.State != "update_available" {
		t.Fatalf("initial stable check: %+v %v", status, err)
	}
	if stableDownloads.Load() != 1 || betaDownloads.Load() != 0 {
		t.Fatalf("initial downloads: stable=%d beta=%d", stableDownloads.Load(), betaDownloads.Load())
	}

	status, err := s.SetChannel(ctx, "beta")
	if err != nil {
		t.Fatalf("switch channel: %v", err)
	}
	if status.Channel != "beta" || status.Target == nil || status.Target.Release.Version != beta.Release.Version || status.State != "update_available" {
		t.Fatalf("switched status: %+v", status)
	}
	if betaDownloads.Load() != 1 {
		t.Fatalf("beta target was not fetched immediately: %d", betaDownloads.Load())
	}

	if status, err = s.SetChannel(ctx, "stable"); err != nil || status.Target == nil || status.Target.Release.Version != stable.Release.Version {
		t.Fatalf("switch back to stable: %+v %v", status, err)
	}
	if indexRequests.Load() != 2 || stableDownloads.Load() != 1 || betaDownloads.Load() != 1 {
		t.Fatalf("independent cooldowns not respected: index=%d stable=%d beta=%d", indexRequests.Load(), stableDownloads.Load(), betaDownloads.Load())
	}
}

func TestLegacyGlobalAttemptMigratesToEffectiveChannel(t *testing.T) {
	clock := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	legacy := State{
		SchemaVersion: 1,
		Preference:    "stable",
		LastAttempt:   clock,
		Releases:      map[string]ReleaseInfo{},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryStore{data: data}
	s := New(store, "v1.0.0", "test", false)
	s.now = func() time.Time { return clock }
	if err := s.load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := s.state.LastAttemptByChannel["stable"]; !got.Equal(clock) {
		t.Fatalf("legacy attempt was not assigned to effective channel: %v", got)
	}
	if _, ok := s.state.LastAttemptByChannel["beta"]; ok {
		t.Fatal("legacy attempt incorrectly assigned to beta")
	}
}

func TestSetChannelKeepsPreferenceWhenImmediateCheckFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "offline", http.StatusBadGateway)
	}))
	defer server.Close()

	store := &memoryStore{}
	s := New(store, "v1.0.0", "test", false)
	s.client = server.Client()
	s.indexURL = server.URL + "/index"

	status, err := s.SetChannel(context.Background(), "beta")
	if err != nil {
		t.Fatalf("failed channel check should return its status: %v", err)
	}
	if status.ChannelPreference != "beta" || status.LastError == "" {
		t.Fatalf("unexpected failed channel status: %+v", status)
	}

	var persisted State
	if err := json.Unmarshal(store.data, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Preference != "beta" {
		t.Fatalf("channel preference was not persisted: %q", persisted.Preference)
	}
}

func TestReleasePolicyAndLinksFailClosed(t *testing.T) {
	for _, guide := range []string{
		Repository + "/../../untrusted/guide",
		Repository + "/%2e%2e/%2e%2e/untrusted/guide",
		Repository + `/\..\..\untrusted/guide`,
		"http://github.com/seakee/CPA-Manager-Plus/releases",
		"https://github.com.example/seakee/CPA-Manager-Plus/releases",
	} {
		info := fixture("v2.0.0")
		info.Update.UpgradeGuideURL = guide
		if err := info.Validate("v2.0.0"); err == nil {
			t.Fatalf("accepted guide outside trusted repository: %q", guide)
		}
	}
	data, err := json.Marshal(fixture("v2.0.0"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"breaking", "migration_required", "minimum_direct_upgrade_version"} {
		t.Run(key, func(t *testing.T) {
			var copy map[string]any
			if err := json.Unmarshal(data, &copy); err != nil {
				t.Fatal(err)
			}
			delete(copy["update"].(map[string]any), key)
			invalid, _ := json.Marshal(copy)
			var info ReleaseInfo
			if err := json.Unmarshal(invalid, &info); err == nil {
				t.Fatal("accepted omitted policy")
			}
		})
	}
}
