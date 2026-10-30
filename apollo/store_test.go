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
	if err := s.Publish("application", FormatProperties, []byte("k=v\n"), "r1"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "application.properties"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "k=v\n" {
		t.Fatalf("got %q", data)
	}
	releaseKey, err := newFileStore(dir).Load("application", FormatProperties)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if releaseKey != "r1" {
		t.Fatalf("releaseKey %v", releaseKey)
	}
}

// TestStoreLoadDoesNotParse 跳过解析检查时落地的内容可能无法解析，仍应作为有效 LKG
func TestStoreLoadDoesNotParse(t *testing.T) {
	dir := t.TempDir()
	s := newFileStore(dir)
	if err := s.Publish("config.json", FormatJSON, []byte("{not json"), "r1"); err != nil {
		t.Fatal(err)
	}
	if _, err := newFileStore(dir).Load("config.json", FormatJSON); err != nil {
		t.Fatalf("load: %v", err)
	}
}

func TestStoreLoadHashMismatch(t *testing.T) {
	dir := t.TempDir()
	s := newFileStore(dir)
	if err := s.Publish("ns", FormatJSON, []byte(`{"k":"v"}`), ""); err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(dir, "ns.json")
	target, err := os.Readlink(stable)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, target), []byte(`{"k":"tampered"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := newFileStore(dir).Load("ns", FormatJSON); err == nil {
		t.Fatal("expected hash mismatch error")
	}
}

func TestStorePublishDedup(t *testing.T) {
	dir := t.TempDir()
	s := newFileStore(dir)
	data := []byte(`{"k":"v"}`)
	if err := s.Publish("ns", FormatJSON, data, ""); err != nil {
		t.Fatal(err)
	}
	first := s.CurrentHash("ns", FormatJSON)
	if err := s.Publish("ns", FormatJSON, data, ""); err != nil {
		t.Fatal(err)
	}
	if s.CurrentHash("ns", FormatJSON) != first {
		t.Fatalf("hash changed on dedup publish")
	}
}

func TestStoreSymlinkValid(t *testing.T) {
	dir := t.TempDir()
	s := newFileStore(dir)
	if err := s.Publish("ns", FormatJSON, []byte(`{"k":"v"}`), ""); err != nil {
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
	if err := s.Publish("ns", FormatJSON, []byte(`{"k":"v1"}`), ""); err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(dir, "ns.json")
	first, _ := os.Readlink(stable)
	if err := s.Publish("ns", FormatJSON, []byte(`{"k":"v2"}`), ""); err != nil {
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
	if err := s.Publish("config.json", FormatYAML, []byte("app:\n  version: 1.0.0\n"), "r-yaml"); err != nil {
		t.Fatal(err)
	}
	if err := s.Publish("config.json", FormatXML, []byte("<app><version>1.0.0</version></app>"), "r-xml"); err != nil {
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
	yamlKey, err := reloaded.Load("config.json", FormatYAML)
	if err != nil {
		t.Fatalf("load yaml: %v", err)
	}
	xmlKey, err := reloaded.Load("config.json", FormatXML)
	if err != nil {
		t.Fatalf("load xml: %v", err)
	}
	if yamlKey != "r-yaml" || xmlKey != "r-xml" {
		t.Fatalf("releaseKey mixed: yaml=%s xml=%s", yamlKey, xmlKey)
	}
	if reloaded.CurrentHash("config.json", FormatYAML) == reloaded.CurrentHash("config.json", FormatXML) {
		t.Fatal("different target formats should have different file hashes")
	}
}

func TestStoreReleaseKeyOnlyUpdate(t *testing.T) {
	dir := t.TempDir()
	s := newFileStore(dir)
	data := []byte(`{"k":"v"}`)
	if err := s.Publish("ns", FormatJSON, data, "r1"); err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(dir, "ns.json")
	firstTarget, err := os.Readlink(stable)
	if err != nil {
		t.Fatal(err)
	}
	firstHash := s.CurrentHash("ns", FormatJSON)

	if err := s.Publish("ns", FormatJSON, data, "r2"); err != nil {
		t.Fatal(err)
	}
	secondTarget, err := os.Readlink(stable)
	if err != nil {
		t.Fatal(err)
	}
	releaseKey, err := newFileStore(dir).Load("ns", FormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	if firstTarget != secondTarget {
		t.Fatal("releaseKey-only update should not switch snapshot")
	}
	if releaseKey != "r2" {
		t.Fatalf("releaseKey not updated: %s", releaseKey)
	}
	if s.CurrentHash("ns", FormatJSON) != firstHash {
		t.Fatal("hash changed on releaseKey-only update")
	}
}

func TestStoreCustomFilename(t *testing.T) {
	dir := t.TempDir()
	filenames := map[string]string{nsKey("config.json", FormatYAML): "application.conf"}
	s := newFileStore(dir, filenames)
	if err := s.Publish("config.json", FormatYAML, []byte("k: v\n"), "r1"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "application.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "k: v\n" {
		t.Fatalf("unexpected content: %s", data)
	}
	if _, err := newFileStore(dir, filenames).Load("config.json", FormatYAML); err != nil {
		t.Fatal(err)
	}
}

func TestStoreHashMatchesFileBytes(t *testing.T) {
	dir := t.TempDir()
	s := newFileStore(dir)
	if err := s.Publish("config.json", FormatYAML, []byte("k: v\n"), ""); err != nil {
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
