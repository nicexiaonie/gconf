package apollo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// fileStore 实现 SnapshotStore：当前版本文件 + 稳定 symlink + LKG 索引，不保留历史版本。
type fileStore struct {
	root      string
	filenames map[string]string
	mu        sync.Mutex
	hash      map[string]string // namespace:format -> 当前已发布文件 hash
}

func newFileStore(root string, filenames ...map[string]string) *fileStore {
	configured := make(map[string]string)
	if len(filenames) > 0 {
		configured = filenames[0]
	}
	return &fileStore{root: root, filenames: configured, hash: make(map[string]string)}
}

type nsIndex struct {
	Hash       string `json:"hash"`
	Format     string `json:"format"`
	ReleaseKey string `json:"release_key"`
}

func (f *fileStore) metadataDir() string {
	return filepath.Join(f.root, ".apollo")
}

func (f *fileStore) indexPath(ns string, format Format) string {
	return filepath.Join(f.metadataDir(), "index-"+hashBytes([]byte(nsKey(ns, format)))+".json")
}

func (f *fileStore) stablePath(ns string, format Format) string {
	filename := f.filenames[nsKey(ns, format)]
	if filename == "" {
		filename = nsBase(ns) + "." + string(format)
	}
	return filepath.Join(f.root, filename)
}

func nsBase(ns string) string {
	if !isFileType(ns) {
		return ns
	}
	return ns[:strings.LastIndex(ns, ".")]
}

func nsExt(ns string) string {
	dot := strings.LastIndex(ns, ".")
	if dot <= 0 {
		return ""
	}
	return ns[dot:]
}

func formatFromExt(ns string) string {
	switch nsExt(ns) {
	case ".json":
		return "json"
	case ".yaml":
		return "yaml"
	case ".yml":
		return "yml"
	case ".xml":
		return "xml"
	case ".txt":
		return "txt"
	case ".properties":
		return "properties"
	}
	return ""
}

// Load 校验指定格式的 LKG 文件完整性，返回其 releaseKey。
// 完整性以版本文件名中的内容 hash 为准：symlink 只会指向已校验并 fsync 的版本文件，
// 因此即使索引落后（切换 symlink 后、写索引前崩溃）或丢失，该文件仍是有效的 LKG，仅 releaseKey 未知返回空。
// 不解析内容：跳过解析检查时落地的内容本就可能无法解析，仍属有效的 LKG。
func (f *fileStore) Load(ns string, format Format) (string, error) {
	stable := f.stablePath(ns, format)
	target, err := os.Readlink(stable)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(stable), target))
	if err != nil {
		return "", err
	}
	hash := hashBytes(data)
	if hash != snapshotHash(target, format) {
		return "", fmt.Errorf("apollo: LKG hash mismatch for %s:%s", ns, format)
	}
	releaseKey := ""
	if idx, err := f.readIndex(ns, format); err == nil && idx.Hash == hash {
		releaseKey = idx.ReleaseKey
	}
	f.mu.Lock()
	f.hash[nsKey(ns, format)] = hash
	f.mu.Unlock()
	return releaseKey, nil
}

// snapshotHash 从版本文件名 <base>-<hash>.<format> 中取出内容 hash，格式不符返回空。
func snapshotHash(target string, format Format) string {
	name := strings.TrimSuffix(filepath.Base(target), "."+string(format))
	dash := strings.LastIndex(name, "-")
	if dash < 0 {
		return ""
	}
	return name[dash+1:]
}

// Publish 原子发布某 namespace 的指定格式文件内容，发布后清理不再被引用的版本文件。
// 内容 hash 相同仅更新 releaseKey 索引，不创建新版本文件或切换 symlink。
func (f *fileStore) Publish(ns string, format Format, data []byte, releaseKey string) error {
	hash := hashBytes(data)
	key := nsKey(ns, format)
	formatStr := string(format)

	f.mu.Lock()
	defer f.mu.Unlock()
	dir := f.root
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	metadataDir := f.metadataDir()
	if err := os.MkdirAll(metadataDir, 0o755); err != nil {
		return err
	}
	if h, ok := f.hash[key]; ok && h == hash {
		idx := nsIndex{Hash: hash, Format: formatStr, ReleaseKey: releaseKey}
		if err := f.writeIndex(ns, format, idx); err != nil {
			return err
		}
		f.prune()
		return nil
	}

	snapDir := filepath.Join(metadataDir, "snapshots")
	if err := os.MkdirAll(snapDir, 0o755); err != nil {
		return err
	}

	base := strings.TrimSuffix(filepath.Base(f.stablePath(ns, format)), filepath.Ext(f.stablePath(ns, format)))
	snapFile := fmt.Sprintf("%s-%s.%s", base, hash, formatStr)
	snapPath := filepath.Join(snapDir, snapFile)
	if err := os.WriteFile(snapPath, data, 0o644); err != nil {
		return err
	}
	if err := fsyncFile(snapPath); err != nil {
		return err
	}

	stable := f.stablePath(ns, format)
	tmpLink := stable + ".tmp"
	_ = os.Remove(tmpLink)
	relTarget, err := filepath.Rel(filepath.Dir(stable), snapPath)
	if err != nil {
		return err
	}
	if err := os.Symlink(relTarget, tmpLink); err != nil {
		return err
	}
	if err := os.Rename(tmpLink, stable); err != nil {
		return err
	}
	if err := fsyncDir(dir); err != nil {
		return err
	}

	idx := nsIndex{Hash: hash, Format: formatStr, ReleaseKey: releaseKey}
	if err := f.writeIndex(ns, format, idx); err != nil {
		return err
	}
	f.hash[key] = hash
	f.prune()
	return nil
}

// prune 删除不再被任何稳定路径引用的版本文件，不保留历史版本。调用方需持有 f.mu。
// 引用关系以 CacheDir 下所有 symlink 的实际指向为准，不会误删其他输出项（含已从配置中移除的输出项）的当前版本；
// 任一 symlink 无法读取时放弃本次清理。清理失败不影响已完成的发布：残留文件无害，下次发布时会再次清理。
func (f *fileStore) prune() {
	entries, err := os.ReadDir(f.root)
	if err != nil {
		return
	}
	referenced := make(map[string]bool)
	for _, e := range entries {
		if e.Type()&os.ModeSymlink == 0 {
			continue
		}
		target, err := os.Readlink(filepath.Join(f.root, e.Name()))
		if err != nil {
			return
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(f.root, target)
		}
		referenced[filepath.Clean(target)] = true
	}
	snapDir := filepath.Join(f.metadataDir(), "snapshots")
	snaps, err := os.ReadDir(snapDir)
	if err != nil {
		return
	}
	for _, e := range snaps {
		path := filepath.Join(snapDir, e.Name())
		if !referenced[path] {
			_ = os.Remove(path)
		}
	}
}

func (f *fileStore) CurrentHash(ns string, format Format) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hash[nsKey(ns, format)]
}

func (f *fileStore) readIndex(ns string, format Format) (nsIndex, error) {
	var idx nsIndex
	data, err := os.ReadFile(f.indexPath(ns, format))
	if err != nil {
		return idx, err
	}
	err = json.Unmarshal(data, &idx)
	return idx, err
}

// writeIndex 以临时文件 + rename 原子更新索引。
func (f *fileStore) writeIndex(ns string, format Format, idx nsIndex) error {
	data, err := json.Marshal(idx)
	if err != nil {
		return err
	}
	path := f.indexPath(ns, format)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := fsyncFile(tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return fsyncDir(f.metadataDir())
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func fsyncFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func fsyncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
