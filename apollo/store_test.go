package apollo

import (
	"io"
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
	// 读取方在发布前已打开旧版本
	reader, err := os.Open(stable)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := s.Publish("ns", FormatJSON, []byte(`{"k":"v2"}`), ""); err != nil {
		t.Fatal(err)
	}
	second, _ := os.Readlink(stable)
	if first == second {
		t.Fatalf("symlink not updated on new version")
	}
	// 旧版本文件被清理，但已打开的读取方仍能读到完整的旧内容
	if _, err := os.Stat(filepath.Join(dir, first)); !os.IsNotExist(err) {
		t.Fatalf("old snapshot should be removed, stat err: %v", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"k":"v1"}` {
		t.Fatalf("open reader got %s", data)
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

// TestStoreLoadIndexBehindSymlink 模拟 symlink 已切换、索引未更新时崩溃：版本文件完整，仍应作为 LKG
func TestStoreLoadIndexBehindSymlink(t *testing.T) {
	dir := t.TempDir()
	s := newFileStore(dir)
	if err := s.Publish("ns", FormatJSON, []byte(`{"k":"v1"}`), "r1"); err != nil {
		t.Fatal(err)
	}
	indexPath := s.indexPath("ns", FormatJSON)
	oldIndex, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Publish("ns", FormatJSON, []byte(`{"k":"v2"}`), "r2"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, oldIndex, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded := newFileStore(dir)
	releaseKey, err := loaded.Load("ns", FormatJSON)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if releaseKey != "" {
		t.Fatalf("stale index releaseKey should not be used, got %q", releaseKey)
	}
	if got, want := loaded.CurrentHash("ns", FormatJSON), hashBytes([]byte(`{"k":"v2"}`)); got != want {
		t.Fatalf("hash %s, want %s", got, want)
	}
}

// TestStoreLoadMissingIndex 索引丢失时，symlink 指向的版本文件完整即可作为 LKG
func TestStoreLoadMissingIndex(t *testing.T) {
	dir := t.TempDir()
	s := newFileStore(dir)
	if err := s.Publish("ns", FormatJSON, []byte(`{"k":"v1"}`), "r1"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(s.indexPath("ns", FormatJSON)); err != nil {
		t.Fatal(err)
	}
	if _, err := newFileStore(dir).Load("ns", FormatJSON); err != nil {
		t.Fatalf("load: %v", err)
	}
}

func snapshotFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, ".apollo", "snapshots"))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// TestStorePublishRemovesOldVersion 不保留历史版本：发布新版本后旧版本文件被删除
func TestStorePublishRemovesOldVersion(t *testing.T) {
	dir := t.TempDir()
	s := newFileStore(dir)
	for _, v := range []string{"v1", "v2", "v3"} {
		if err := s.Publish("ns", FormatJSON, []byte(`{"k":"`+v+`"}`), v); err != nil {
			t.Fatal(err)
		}
	}
	files := snapshotFiles(t, dir)
	if len(files) != 1 {
		t.Fatalf("expected only current version, got %v", files)
	}
	data, err := os.ReadFile(filepath.Join(dir, "ns.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"k":"v3"}` {
		t.Fatalf("got %s", data)
	}
}

// TestStorePruneKeepsOtherOutputs 清理只删除未被任何稳定路径引用的版本文件
func TestStorePruneKeepsOtherOutputs(t *testing.T) {
	dir := t.TempDir()
	s := newFileStore(dir)
	if err := s.Publish("config.json", FormatJSON, []byte(`{"k":"v1"}`), "r1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Publish("config.json", FormatYAML, []byte("k: v1\n"), "r1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Publish("config.json", FormatJSON, []byte(`{"k":"v2"}`), "r2"); err != nil {
		t.Fatal(err)
	}
	if files := snapshotFiles(t, dir); len(files) != 2 {
		t.Fatalf("expected 2 current versions, got %v", files)
	}
	for name, want := range map[string]string{"config.json": `{"k":"v2"}`, "config.yaml": "k: v1\n"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != want {
			t.Fatalf("%s: got %q", name, data)
		}
	}
}

// TestStorePruneLegacyHistory 旧版本遗留的历史文件在内容未变化的发布中也会被清理
func TestStorePruneLegacyHistory(t *testing.T) {
	dir := t.TempDir()
	if err := newFileStore(dir).Publish("ns", FormatJSON, []byte(`{"k":"v1"}`), "r1"); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(dir, ".apollo", "snapshots", "ns-"+hashBytes([]byte("old"))+".json")
	if err := os.WriteFile(legacy, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newFileStore(dir)
	if _, err := s.Load("ns", FormatJSON); err != nil {
		t.Fatal(err)
	}
	if err := s.Publish("ns", FormatJSON, []byte(`{"k":"v1"}`), "r2"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy history should be removed, stat err: %v", err)
	}
	if files := snapshotFiles(t, dir); len(files) != 1 {
		t.Fatalf("expected only current version, got %v", files)
	}
}
