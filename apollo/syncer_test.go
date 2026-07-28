package apollo

import (
	"context"
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
	sink    ChangeSink
	closed  bool
}

func newFakeRemote() *fakeRemote {
	return &fakeRemote{
		snaps:   make(map[string]Snapshot),
		snapErr: make(map[string]error),
	}
}

func (f *fakeRemote) Snapshot(ns string) (Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.snapErr[ns]; ok {
		return Snapshot{}, err
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
		store:   newFileStore(cfg.CacheDir),
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
		Format:     FormatJSON,
		Namespaces: []NamespaceConfig{{Name: "ns1"}, {Name: "ns2"}},
	}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer s.Close()

	if _, err := readNSFile(dir, "ns1", "ns1.json"); err != nil {
		t.Fatalf("ns1 not published: %v", err)
	}
	if _, err := readNSFile(dir, "ns2", "ns2.json"); err != nil {
		t.Fatalf("ns2 not published: %v", err)
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
	if err := store.Publish(Snapshot{Namespace: "ns", Values: map[string]any{"k": "lkg"}}, FormatJSON); err != nil {
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
		Format:     FormatJSON,
		Watch:      true,
		Namespaces: []NamespaceConfig{{Name: "ns"}},
	}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	remote.emit(Change{Namespace: "ns", Values: map[string]any{"k": "v2"}, NotificationID: 2})
	if !waitForValue(dir, "ns", "ns.json", "v2") {
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
		Format:     FormatJSON,
		Watch:      true,
		Namespaces: []NamespaceConfig{{Name: "ns"}},
	}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	remote.emit(Change{Namespace: "ns", Values: map[string]any{"k": "new"}, NotificationID: 5})
	if !waitForValue(dir, "ns", "ns.json", "new") {
		t.Fatalf("new version not applied")
	}
	// 旧版本应被丢弃
	remote.emit(Change{Namespace: "ns", Values: map[string]any{"k": "old"}, NotificationID: 3})
	time.Sleep(100 * time.Millisecond)
	data, _ := readNSFile(dir, "ns", "ns.json")
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
		Format:     FormatJSON,
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
	data, err := readNSFile(dir, "ns", "ns.json")
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
	return os.ReadFile(filepath.Join(dir, ns, name))
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
	values := map[string]any{"app": map[string]any{"version": "1.0.0"}}
	store := newFileStore(dir)
	if err := store.Publish(Snapshot{Namespace: "config.json", Values: values, ReleaseKey: "r1"}, FormatYAML); err != nil {
		t.Fatal(err)
	}
	if err := store.Publish(Snapshot{Namespace: "config.json", Values: values, ReleaseKey: "r1"}, FormatXML); err != nil {
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
	remote.snaps["config.json"] = Snapshot{Namespace: "config.json", Values: map[string]any{"version": "1"}, ReleaseKey: "r1"}
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
	remote.emit(Change{Namespace: "config.json", Values: map[string]any{"version": "2"}, ReleaseKey: "r2", NotificationID: 2})
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
