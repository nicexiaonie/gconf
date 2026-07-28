package apollo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStorePublishAndLoad(t *testing.T) {
	dir := t.TempDir()
	s := newFileStore(dir)
	snap := Snapshot{Namespace: "application", Values: map[string]any{"k": "v"}, ReleaseKey: "r1"}
	if err := s.Publish(snap, FormatJSON); err != nil {
		t.Fatalf("publish: %v", err)
	}
	loaded, err := s.Load("application", FormatJSON)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Values["k"] != "v" {
		t.Fatalf("got %v", loaded.Values)
	}
	if loaded.ReleaseKey != "r1" {
		t.Fatalf("releaseKey %v", loaded.ReleaseKey)
	}
}

func TestStorePublishDedup(t *testing.T) {
	dir := t.TempDir()
	s := newFileStore(dir)
	snap := Snapshot{Namespace: "ns", Values: map[string]any{"k": "v"}}
	if err := s.Publish(snap, FormatJSON); err != nil {
		t.Fatal(err)
	}
	first := s.CurrentHash("ns", FormatJSON)
	if err := s.Publish(snap, FormatJSON); err != nil {
		t.Fatal(err)
	}
	if s.CurrentHash("ns", FormatJSON) != first {
		t.Fatalf("hash changed on dedup publish")
	}
}

func TestStorePublishYAML(t *testing.T) {
	dir := t.TempDir()
	s := newFileStore(dir)
	snap := Snapshot{Namespace: "ns", Values: map[string]any{"k": "v"}}
	if err := s.Publish(snap, FormatYAML); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load("ns", FormatYAML)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Values["k"] != "v" {
		t.Fatalf("got %v", loaded.Values)
	}
}

func TestStorePublishXMLIndented(t *testing.T) {
	dir := t.TempDir()
	s := newFileStore(dir)
	snap := Snapshot{
		Namespace: "config.json",
		Values: map[string]any{
			"app": map[string]any{"name": "demo", "version": "1.0.0"},
		},
	}
	if err := s.Publish(snap, FormatXML); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.xml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "\n    <name>demo</name>") || !strings.Contains(content, "\n    <version>1.0.0</version>") {
		t.Fatalf("xml is not indented:\n%s", content)
	}
	loaded, err := s.Load("config.json", FormatXML)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(loaded.Values) == 0 {
		t.Fatal("loaded xml is empty")
	}
}

func TestStoreSymlinkValid(t *testing.T) {
	dir := t.TempDir()
	s := newFileStore(dir)
	snap := Snapshot{Namespace: "ns", Values: map[string]any{"k": "v"}}
	if err := s.Publish(snap, FormatJSON); err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(dir, "ns.json")
	target, err := os.Readlink(stable)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(stable), target)); err != nil {
		t.Fatalf("symlink target invalid: %v", err)
	}
}

func TestStoreLoadNoLKG(t *testing.T) {
	dir := t.TempDir()
	s := newFileStore(dir)
	if _, err := s.Load("nope", FormatJSON); err == nil {
		t.Fatal("expected error when no LKG")
	}
}

func TestStoreNewVersionDoesNotBreakOld(t *testing.T) {
	dir := t.TempDir()
	s := newFileStore(dir)
	if err := s.Publish(Snapshot{Namespace: "ns", Values: map[string]any{"k": "v1"}}, FormatJSON); err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(dir, "ns.json")
	first, _ := os.Readlink(stable)
	if err := s.Publish(Snapshot{Namespace: "ns", Values: map[string]any{"k": "v2"}}, FormatJSON); err != nil {
		t.Fatal(err)
	}
	second, _ := os.Readlink(stable)
	if first == second {
		t.Fatalf("symlink not updated on new version")
	}
	// 旧版本文件仍存在
	if _, err := os.Stat(filepath.Join(dir, first)); err != nil {
		t.Fatalf("old snapshot lost: %v", err)
	}
}

func TestStoreMultiFormatLKG(t *testing.T) {
	dir := t.TempDir()
	s := newFileStore(dir)
	values := map[string]any{"app": map[string]any{"version": "1.0.0"}}
	if err := s.Publish(Snapshot{Namespace: "config.json", Values: values, ReleaseKey: "r-yaml"}, FormatYAML); err != nil {
		t.Fatal(err)
	}
	if err := s.Publish(Snapshot{Namespace: "config.json", Values: values, ReleaseKey: "r-xml"}, FormatXML); err != nil {
		t.Fatal(err)
	}

	metadataDir := filepath.Join(dir, ".apollo")
	entries, err := os.ReadDir(metadataDir)
	if err != nil {
		t.Fatal(err)
	}
	indexCount := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "index-") && strings.HasSuffix(entry.Name(), ".json") {
			indexCount++
		}
	}
	if indexCount != 2 {
		t.Fatalf("index count: got=%d want=2", indexCount)
	}

	// 重建 store，模拟进程重启后分别恢复两种格式。
	reloaded := newFileStore(dir)
	yamlSnap, err := reloaded.Load("config.json", FormatYAML)
	if err != nil {
		t.Fatalf("load yaml: %v", err)
	}
	xmlSnap, err := reloaded.Load("config.json", FormatXML)
	if err != nil {
		t.Fatalf("load xml: %v", err)
	}
	if yamlSnap.ReleaseKey != "r-yaml" || xmlSnap.ReleaseKey != "r-xml" {
		t.Fatalf("releaseKey mixed: yaml=%s xml=%s", yamlSnap.ReleaseKey, xmlSnap.ReleaseKey)
	}
	if reloaded.CurrentHash("config.json", FormatYAML) == reloaded.CurrentHash("config.json", FormatXML) {
		t.Fatal("different target formats should have different file hashes")
	}
}

func TestStoreReleaseKeyOnlyUpdate(t *testing.T) {
	dir := t.TempDir()
	s := newFileStore(dir)
	values := map[string]any{"k": "v"}
	if err := s.Publish(Snapshot{Namespace: "ns", Values: values, ReleaseKey: "r1"}, FormatJSON); err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(dir, "ns.json")
	firstTarget, err := os.Readlink(stable)
	if err != nil {
		t.Fatal(err)
	}
	firstHash := s.CurrentHash("ns", FormatJSON)

	if err := s.Publish(Snapshot{Namespace: "ns", Values: values, ReleaseKey: "r2"}, FormatJSON); err != nil {
		t.Fatal(err)
	}
	secondTarget, err := os.Readlink(stable)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := newFileStore(dir).Load("ns", FormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	if firstTarget != secondTarget {
		t.Fatal("releaseKey-only update should not switch snapshot")
	}
	if loaded.ReleaseKey != "r2" {
		t.Fatalf("releaseKey not updated: %s", loaded.ReleaseKey)
	}
	if s.CurrentHash("ns", FormatJSON) != firstHash {
		t.Fatal("hash changed on releaseKey-only update")
	}
}

func TestStoreCustomFilename(t *testing.T) {
	dir := t.TempDir()
	filenames := map[string]string{nsKey("config.json", FormatYAML): "application.yaml"}
	s := newFileStore(dir, filenames)
	if err := s.Publish(Snapshot{Namespace: "config.json", Values: map[string]any{"k": "v"}}, FormatYAML); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "application.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "k: v") {
		t.Fatalf("unexpected content: %s", data)
	}
	reloaded := newFileStore(dir, filenames)
	loaded, err := reloaded.Load("config.json", FormatYAML)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Values["k"] != "v" {
		t.Fatalf("got %v", loaded.Values)
	}
}

func TestStoreHashMatchesFileBytes(t *testing.T) {
	dir := t.TempDir()
	s := newFileStore(dir)
	if err := s.Publish(Snapshot{Namespace: "config.json", Values: map[string]any{"k": "v"}}, FormatYAML); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := s.CurrentHash("config.json", FormatYAML), hashBytes(data); got != want {
		t.Fatalf("hash mismatch: got=%s want=%s", got, want)
	}
}
