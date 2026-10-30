package apollo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeRemote struct {
	mu      sync.Mutex
	snaps   map[string]Snapshot
	snapErr map[string]error
	// failTimes 指定 namespace 前若干次 Snapshot 失败，模拟 agollo 尚未加载完成
	failTimes map[string]int
	sink      ChangeSink
	closed    bool
}

func newFakeRemote() *fakeRemote {
	return &fakeRemote{
		snaps:     make(map[string]Snapshot),
		snapErr:   make(map[string]error),
		failTimes: make(map[string]int),
	}
}

func (f *fakeRemote) Snapshot(ns string) (Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.snapErr[ns]; ok {
		return Snapshot{}, err
	}
	if f.failTimes[ns] > 0 {
		f.failTimes[ns]--
		return Snapshot{}, fmt.Errorf("not loaded")
	}
	if s, ok := f.snaps[ns]; ok {
		return s, nil
	}
	return Snapshot{}, fmt.Errorf("unavailable")
}

func (f *fakeRemote) Subscribe(sink ChangeSink) (Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sink = sink
	return &fakeSub{}, nil
}

func (f *fakeRemote) emit(ch Change) {
	f.mu.Lock()
	sink := f.sink
	f.mu.Unlock()
	if sink != nil {
		sink.OnChange(ch)
	}
}

func (f *fakeRemote) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
}

type fakeSub struct{}

func (s *fakeSub) Close() {}

func newTestSyncer(t *testing.T, cfg Config, remote RemoteConfig) *Syncer {
	t.Helper()
	return &Syncer{
		cfg:     cfg,
		remote:  remote,
		store:   newFileStore(cfg.CacheDir, outputFilenames(cfg)),
		status:  make(map[string]*NamespaceStatus),
		lastNID: make(map[string]int64),
	}
}

func TestSyncerMultiNamespace(t *testing.T) {
	dir := t.TempDir()
	remote := newFakeRemote()
	remote.snaps["ns1"] = Snapshot{Namespace: "ns1", Values: map[string]any{"a": "1"}}
	remote.snaps["ns2"] = Snapshot{Namespace: "ns2", Values: map[string]any{"b": "2"}}

	cfg := Config{
		AppID:      "app",
		MetaServer: "http://x",
		CacheDir:   dir,
		Namespaces: []NamespaceConfig{{Name: "ns1"}, {Name: "ns2"}},
	}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer s.Close()

	if _, err := readNSFile(dir, "ns1", "ns1.properties"); err != nil {
		t.Fatalf("ns1 not published: %v", err)
	}
	if _, err := readNSFile(dir, "ns2", "ns2.properties"); err != nil {
		t.Fatalf("ns2 not published: %v", err)
	}
}

func TestSyncerCustomFilename(t *testing.T) {
	dir := t.TempDir()
	remote := newFakeRemote()
	remote.snaps["config.json"] = Snapshot{Namespace: "config.json", RawContent: `{"k": "v"}`}
	cfg := Config{
		AppID:      "app",
		MetaServer: "http://x",
		CacheDir:   dir,
		Namespaces: []NamespaceConfig{{Name: "config.json", Format: FormatYAML, Filename: "application.yaml"}},
	}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := os.ReadFile(filepath.Join(dir, "application.yaml")); err != nil {
		t.Fatalf("custom filename not published: %v", err)
	}
}

func TestValidateFilename(t *testing.T) {
	base := Config{
		AppID:      "app",
		MetaServer: "http://x",
		CacheDir:   t.TempDir(),
	}
	tests := []struct {
		name       string
		namespaces []NamespaceConfig
	}{
		{name: "directory", namespaces: []NamespaceConfig{{Name: "ns", Format: FormatJSON, Filename: "sub/config.json"}}},
		{name: "conflict", namespaces: []NamespaceConfig{{Name: "ns1", Format: FormatJSON, Filename: "config.json"}, {Name: "ns2", Format: FormatJSON, Filename: "config.json"}}},
		{name: "duplicate namespace format", namespaces: []NamespaceConfig{{Name: "ns", Format: FormatJSON, Filename: "one.json"}, {Name: "ns", Format: FormatJSON, Filename: "two.json"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := base
			cfg.Namespaces = tt.namespaces
			if _, err := New(cfg); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestSyncerRequiredFailsWithoutLKG(t *testing.T) {
	dir := t.TempDir()
	remote := newFakeRemote()
	cfg := Config{
		AppID:      "app",
		MetaServer: "http://x",
		CacheDir:   dir,
		Namespaces: []NamespaceConfig{{Name: "ns", Required: true}},
	}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err == nil {
		s.Close()
		t.Fatal("expected start error when required and no LKG")
	}
}

func TestSyncerDegradedWithLKG(t *testing.T) {
	dir := t.TempDir()
	store := newFileStore(dir)
	if err := store.Publish("ns", FormatProperties, []byte("k=lkg\n"), ""); err != nil {
		t.Fatal(err)
	}

	remote := newFakeRemote()
	remote.snapErr["ns"] = fmt.Errorf("network down")

	cfg := Config{
		AppID:      "app",
		MetaServer: "http://x",
		CacheDir:   dir,
		Namespaces: []NamespaceConfig{{Name: "ns", Required: true}},
	}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("start should succeed with LKG: %v", err)
	}
	defer s.Close()

	st := s.Status()
	if len(st.Namespaces) != 1 || st.Namespaces[0].Source != "lkg" {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestSyncerChangeUpdates(t *testing.T) {
	dir := t.TempDir()
	remote := newFakeRemote()
	remote.snaps["ns"] = Snapshot{Namespace: "ns", Values: map[string]any{"k": "v1"}}
	cfg := Config{
		AppID:      "app",
		MetaServer: "http://x",
		CacheDir:   dir,
		Watch:      true,
		Namespaces: []NamespaceConfig{{Name: "ns"}},
	}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	remote.emit(Change{Namespace: "ns", Values: map[string]any{"k": "v2"}, NotificationID: 2})
	if !waitForValue(dir, "ns", "ns.properties", "v2") {
		t.Fatalf("change not applied")
	}
}

func TestSyncerDropStaleNotification(t *testing.T) {
	dir := t.TempDir()
	remote := newFakeRemote()
	remote.snaps["ns"] = Snapshot{Namespace: "ns", Values: map[string]any{"k": "v1"}}
	cfg := Config{
		AppID:      "app",
		MetaServer: "http://x",
		CacheDir:   dir,
		Watch:      true,
		Namespaces: []NamespaceConfig{{Name: "ns"}},
	}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	remote.emit(Change{Namespace: "ns", Values: map[string]any{"k": "new"}, NotificationID: 5})
	if !waitForValue(dir, "ns", "ns.properties", "new") {
		t.Fatalf("new version not applied")
	}
	// 旧版本应被丢弃
	remote.emit(Change{Namespace: "ns", Values: map[string]any{"k": "old"}, NotificationID: 3})
	time.Sleep(100 * time.Millisecond)
	data, _ := readNSFile(dir, "ns", "ns.properties")
	if strings.Contains(string(data), "old") {
		t.Fatalf("stale notification applied: %s", data)
	}
}

func TestSyncerWatchDisabledDoesNotApplyChanges(t *testing.T) {
	dir := t.TempDir()
	remote := newFakeRemote()
	remote.snaps["ns"] = Snapshot{Namespace: "ns", Values: map[string]any{"k": "v1"}}
	cfg := Config{
		AppID:      "app",
		MetaServer: "http://x",
		CacheDir:   dir,
		Watch:      false,
		Namespaces: []NamespaceConfig{{Name: "ns"}},
	}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	remote.emit(Change{Namespace: "ns", Values: map[string]any{"k": "v2"}, NotificationID: 2})
	time.Sleep(100 * time.Millisecond)
	data, err := readNSFile(dir, "ns", "ns.properties")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "v2") {
		t.Fatalf("watch=false should not apply changes: %s", data)
	}
}

func TestSyncerCloseIdempotent(t *testing.T) {
	dir := t.TempDir()
	remote := newFakeRemote()
	remote.snaps["ns"] = Snapshot{Namespace: "ns", Values: map[string]any{"k": "v"}}
	cfg := Config{
		AppID:      "app",
		MetaServer: "http://x",
		CacheDir:   dir,
		Namespaces: []NamespaceConfig{{Name: "ns"}},
	}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second close failed: %v", err)
	}
}

func readNSFile(dir, ns, name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(dir, name))
}

func waitForReleaseKey(s *Syncer, want string) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st := s.Status()
		if len(st.Namespaces) > 0 {
			all := true
			for _, ns := range st.Namespaces {
				if ns.ReleaseKey != want {
					all = false
					break
				}
			}
			if all {
				return true
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func waitForValue(dir, ns, name, want string) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := readNSFile(dir, ns, name)
		if err == nil && strings.Contains(string(data), want) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func TestSyncerMultiFormatLKGRestart(t *testing.T) {
	dir := t.TempDir()
	store := newFileStore(dir)
	if err := store.Publish("config.json", FormatYAML, []byte("app:\n  version: 1.0.0\n"), "r1"); err != nil {
		t.Fatal(err)
	}
	if err := store.Publish("config.json", FormatXML, []byte("<app><version>1.0.0</version></app>"), "r1"); err != nil {
		t.Fatal(err)
	}

	remote := newFakeRemote()
	remote.snapErr["config.json"] = fmt.Errorf("network down")
	cfg := Config{
		AppID:      "app",
		MetaServer: "http://x",
		CacheDir:   dir,
		Namespaces: []NamespaceConfig{
			{Name: "config.json", Required: true, Format: FormatYAML},
			{Name: "config.json", Required: true, Format: FormatXML},
		},
	}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("start with multi-format LKG: %v", err)
	}
	defer s.Close()
	st := s.Status()
	if len(st.Namespaces) != 2 {
		t.Fatalf("status count: %+v", st)
	}
	if st.Namespaces[0].Format != FormatYAML || st.Namespaces[1].Format != FormatXML {
		t.Fatalf("formats: %+v", st)
	}
	for _, ns := range st.Namespaces {
		if ns.Source != "lkg" || ns.ReleaseKey != "r1" || ns.Hash == "" {
			t.Fatalf("invalid LKG status: %+v", ns)
		}
	}
}

func TestSyncerMultiFormatChange(t *testing.T) {
	dir := t.TempDir()
	remote := newFakeRemote()
	remote.snaps["config.json"] = Snapshot{Namespace: "config.json", RawContent: `{"version": "1"}`, ReleaseKey: "r1"}
	cfg := Config{
		AppID:      "app",
		MetaServer: "http://x",
		CacheDir:   dir,
		Watch:      true,
		Namespaces: []NamespaceConfig{
			{Name: "config.json", Format: FormatYAML},
			{Name: "config.json", Format: FormatXML},
		},
	}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	remote.emit(Change{Namespace: "config.json", RawContent: `{"version": "2"}`, ReleaseKey: "r2", NotificationID: 2})
	if !waitForReleaseKey(s, "r2") {
		t.Fatal("multi-format change status not applied")
	}
	if !waitForValue(dir, "config.json", "config.yaml", "2") || !waitForValue(dir, "config.json", "config.xml", "2") {
		t.Fatal("multi-format change files not applied")
	}
	st := s.Status()
	for _, ns := range st.Namespaces {
		if ns.ReleaseKey != "r2" || ns.Hash == "" {
			t.Fatalf("invalid change status: %+v", ns)
		}
	}
}

// TestSyncerChangeAfterCloseNoPanic agollo 在独立 goroutine 中回调，Close 之后仍可能有变更到达，不应 panic
func TestSyncerChangeAfterCloseNoPanic(t *testing.T) {
	dir := t.TempDir()
	remote := newFakeRemote()
	remote.snaps["ns"] = Snapshot{Namespace: "ns", Values: map[string]any{"k": "v1"}}
	cfg := Config{AppID: "app", MetaServer: "http://x", CacheDir: dir, Watch: true, Namespaces: []NamespaceConfig{{Name: "ns"}}}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.Close()
	remote.emit(Change{Namespace: "ns", Values: map[string]any{"k": "v2"}, NotificationID: 2})
	data, err := readNSFile(dir, "ns", "ns.properties")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "k=v1\n" {
		t.Fatalf("closed syncer should not apply changes: %s", data)
	}
}

// TestSyncerBurstKeepsLatest 变更突发时可合并中间版本，但最新版本必须落地
func TestSyncerBurstKeepsLatest(t *testing.T) {
	dir := t.TempDir()
	remote := newFakeRemote()
	remote.snaps["ns"] = Snapshot{Namespace: "ns", Values: map[string]any{"k": "v0"}}
	cfg := Config{AppID: "app", MetaServer: "http://x", CacheDir: dir, Watch: true, Namespaces: []NamespaceConfig{{Name: "ns"}}}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	const n = 500
	for i := 1; i <= n; i++ {
		remote.emit(Change{Namespace: "ns", Values: map[string]any{"k": fmt.Sprintf("v%d", i)}, ReleaseKey: fmt.Sprintf("r%d", i), NotificationID: int64(i)})
	}
	if !waitForReleaseKey(s, fmt.Sprintf("r%d", n)) {
		t.Fatalf("latest change not applied: %+v", s.Status())
	}
	data, err := readNSFile(dir, "ns", "ns.properties")
	if err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("k=v%d\n", n); string(data) != want {
		t.Fatalf("got %q, want %q", data, want)
	}
}

// emitOnSnapshotRemote 在首次拉取期间模拟 Apollo 发布，用于验证启动窗口内的变更不丢失
type emitOnSnapshotRemote struct {
	*fakeRemote
	changes []Change
}

func (r *emitOnSnapshotRemote) Snapshot(ns string) (Snapshot, error) {
	snap, err := r.fakeRemote.Snapshot(ns)
	for _, ch := range r.changes {
		r.emit(ch)
	}
	return snap, err
}

func TestSyncerChangeDuringStartApplied(t *testing.T) {
	dir := t.TempDir()
	fake := newFakeRemote()
	fake.snaps["ns"] = Snapshot{Namespace: "ns", Values: map[string]any{"k": "v1"}, ReleaseKey: "r1"}
	remote := &emitOnSnapshotRemote{
		fakeRemote: fake,
		changes:    []Change{{Namespace: "ns", Values: map[string]any{"k": "v2"}, ReleaseKey: "r2", NotificationID: 2}},
	}
	cfg := Config{AppID: "app", MetaServer: "http://x", CacheDir: dir, Watch: true, Namespaces: []NamespaceConfig{{Name: "ns"}}}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !waitForValue(dir, "ns", "ns.properties", "v2") {
		t.Fatal("change published during start was lost")
	}
}

// TestSyncerDegradedRecordsLastError 降级到 LKG 时仍需暴露远端错误，便于监控
func TestSyncerDegradedRecordsLastError(t *testing.T) {
	dir := t.TempDir()
	if err := newFileStore(dir).Publish("ns", FormatProperties, []byte("k=lkg\n"), "r1"); err != nil {
		t.Fatal(err)
	}
	remote := newFakeRemote()
	remote.snapErr["ns"] = fmt.Errorf("network down")
	cfg := Config{AppID: "app", MetaServer: "http://x", CacheDir: dir, Namespaces: []NamespaceConfig{{Name: "ns", Required: true}}}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	st := s.Status().Namespaces[0]
	if st.Source != "lkg" || !strings.Contains(st.LastError, "network down") {
		t.Fatalf("unexpected status: %+v", st)
	}
}

// TestSyncerOutOfOrderDuringStart agollo 回调无序，暂存期间较旧的通知不能覆盖较新的通知
func TestSyncerOutOfOrderDuringStart(t *testing.T) {
	dir := t.TempDir()
	fake := newFakeRemote()
	fake.snaps["ns"] = Snapshot{Namespace: "ns", Values: map[string]any{"k": "v1"}}
	remote := &emitOnSnapshotRemote{
		fakeRemote: fake,
		changes: []Change{
			{Namespace: "ns", Values: map[string]any{"k": "new"}, ReleaseKey: "r5", NotificationID: 5},
			{Namespace: "ns", Values: map[string]any{"k": "old"}, ReleaseKey: "r3", NotificationID: 3},
		},
	}
	cfg := Config{AppID: "app", MetaServer: "http://x", CacheDir: dir, Watch: true, Namespaces: []NamespaceConfig{{Name: "ns"}}}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !waitForReleaseKey(s, "r5") {
		t.Fatalf("newest change not applied: %+v", s.Status())
	}
	time.Sleep(100 * time.Millisecond)
	data, _ := readNSFile(dir, "ns", "ns.properties")
	if string(data) != "k=new\n" {
		t.Fatalf("got %q", data)
	}
}

// TestSyncerConcurrentChangesDuringClose 并发回调与 Close 交错时不 panic、无数据竞争
func TestSyncerConcurrentChangesDuringClose(t *testing.T) {
	dir := t.TempDir()
	remote := newFakeRemote()
	remote.snaps["ns"] = Snapshot{Namespace: "ns", Values: map[string]any{"k": "v0"}}
	cfg := Config{AppID: "app", MetaServer: "http://x", CacheDir: dir, Watch: true, Namespaces: []NamespaceConfig{{Name: "ns"}}}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 1; i <= 100; i++ {
				remote.emit(Change{Namespace: "ns", Values: map[string]any{"k": fmt.Sprintf("v%d-%d", g, i)}, NotificationID: int64(g*100 + i)})
			}
		}(g)
	}
	time.Sleep(5 * time.Millisecond)
	s.Close()
	wg.Wait()
}

// TestSyncerRetryUntilAvailable 首次拉取失败时按 Retry 重试，等待 agollo 后台加载完成
func TestSyncerRetryUntilAvailable(t *testing.T) {
	dir := t.TempDir()
	remote := newFakeRemote()
	remote.snaps["ns"] = Snapshot{Namespace: "ns", Values: map[string]any{"k": "v1"}}
	remote.failTimes["ns"] = 2
	cfg := Config{
		AppID: "app", MetaServer: "http://x", CacheDir: dir,
		Namespaces: []NamespaceConfig{{Name: "ns", Required: true}},
		Retry:      RetryConfig{MaxAttempts: 3, Interval: 10 * time.Millisecond},
	}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("start should succeed after retry: %v", err)
	}
	defer s.Close()
	if st := s.Status().Namespaces[0]; st.Source != "remote" || st.LastError != "" {
		t.Fatalf("unexpected status: %+v", st)
	}
}

func TestSyncerRetryExhausted(t *testing.T) {
	dir := t.TempDir()
	remote := newFakeRemote()
	remote.snaps["ns"] = Snapshot{Namespace: "ns", Values: map[string]any{"k": "v1"}}
	remote.failTimes["ns"] = 3
	cfg := Config{
		AppID: "app", MetaServer: "http://x", CacheDir: dir,
		Namespaces: []NamespaceConfig{{Name: "ns", Required: true}},
		Retry:      RetryConfig{MaxAttempts: 3, Interval: 10 * time.Millisecond},
	}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err == nil {
		s.Close()
		t.Fatal("expected start error after retries exhausted")
	}
}

// TestSyncerStartCanceledDuringRetry ctx 结束时立即停止重试并返回 ctx 错误
func TestSyncerStartCanceledDuringRetry(t *testing.T) {
	dir := t.TempDir()
	remote := newFakeRemote()
	cfg := Config{
		AppID: "app", MetaServer: "http://x", CacheDir: dir,
		Namespaces: []NamespaceConfig{{Name: "ns", Required: true}},
		Retry:      RetryConfig{MaxAttempts: 2, Interval: time.Hour},
	}
	s := newTestSyncer(t, cfg, remote)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := s.Start(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("start did not stop on ctx deadline")
	}
	if !remote.closed {
		t.Fatal("remote should be closed when start fails")
	}
}

func TestSyncerStartCanceledContext(t *testing.T) {
	dir := t.TempDir()
	remote := newFakeRemote()
	remote.snaps["ns"] = Snapshot{Namespace: "ns", Values: map[string]any{"k": "v1"}}
	cfg := Config{AppID: "app", MetaServer: "http://x", CacheDir: dir, Namespaces: []NamespaceConfig{{Name: "ns"}}}
	s := newTestSyncer(t, cfg, remote)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ns.properties")); !os.IsNotExist(err) {
		t.Fatalf("canceled start should not publish, stat err: %v", err)
	}
}

func TestValidateRetry(t *testing.T) {
	base := Config{AppID: "app", MetaServer: "http://x", CacheDir: "/tmp", Namespaces: []NamespaceConfig{{Name: "ns"}}}
	cases := []struct {
		name  string
		retry RetryConfig
		ok    bool
	}{
		{"zero", RetryConfig{}, true},
		{"single attempt", RetryConfig{MaxAttempts: 1}, true},
		{"with interval", RetryConfig{MaxAttempts: 3, Interval: time.Second}, true},
		{"negative attempts", RetryConfig{MaxAttempts: -1}, false},
		{"negative interval", RetryConfig{Interval: -time.Second}, false},
		{"missing interval", RetryConfig{MaxAttempts: 3}, false},
	}
	for _, c := range cases {
		cfg := base
		cfg.Retry = c.retry
		if err := validate(cfg); (err == nil) != c.ok {
			t.Errorf("%s: validate error = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}
