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

	"github.com/clbanning/mxj"
	"gopkg.in/yaml.v3"
)

// fileStore 实现 SnapshotStore：版本文件 + 稳定 symlink + LKG 索引。
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
	IsRaw      bool   `json:"is_raw"`
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
	dot := strings.LastIndex(ns, ".")
	if dot <= 0 {
		return ns
	}
	return ns[:dot]
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

// Load 从指定格式的 LKG 恢复快照。
func (f *fileStore) Load(ns string, format Format) (Snapshot, error) {
	idx, err := f.readIndex(ns, format)
	if err != nil {
		return Snapshot{}, err
	}
	stable := f.stablePath(ns, format)
	target, err := os.Readlink(stable)
	if err != nil {
		return Snapshot{}, err
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(stable), target))
	if err != nil {
		return Snapshot{}, err
	}
	if hashBytes(data) != idx.Hash {
		return Snapshot{}, fmt.Errorf("apollo: LKG hash mismatch for %s:%s", ns, format)
	}
	f.mu.Lock()
	f.hash[nsKey(ns, format)] = idx.Hash
	f.mu.Unlock()
	if idx.IsRaw {
		return Snapshot{Namespace: ns, RawContent: string(data), ReleaseKey: idx.ReleaseKey}, nil
	}
	values, err := decode(data, format)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Namespace: ns, Values: values, ReleaseKey: idx.ReleaseKey}, nil
}

// Publish 原子发布某 namespace 的指定格式快照。
// 内容 hash 相同仅更新 releaseKey 索引，不创建新版本文件或切换 symlink。
func (f *fileStore) Publish(snap Snapshot, format Format) error {
	var data []byte
	isRaw := snap.Values == nil
	if isRaw {
		data = []byte(snap.RawContent)
	} else {
		var err error
		data, err = encode(snap.Values, format)
		if err != nil {
			return fmt.Errorf("encode: %w", err)
		}
	}
	hash := hashBytes(data)
	key := nsKey(snap.Namespace, format)
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
		idx := nsIndex{Hash: hash, Format: formatStr, IsRaw: isRaw, ReleaseKey: snap.ReleaseKey}
		return f.writeIndex(snap.Namespace, format, idx)
	}

	snapDir := filepath.Join(metadataDir, "snapshots")
	if err := os.MkdirAll(snapDir, 0o755); err != nil {
		return err
	}

	base := strings.TrimSuffix(filepath.Base(f.stablePath(snap.Namespace, format)), filepath.Ext(f.stablePath(snap.Namespace, format)))
	snapFile := fmt.Sprintf("%s-%s.%s", base, hash, formatStr)
	snapPath := filepath.Join(snapDir, snapFile)
	if err := os.WriteFile(snapPath, data, 0o644); err != nil {
		return err
	}
	if err := fsyncFile(snapPath); err != nil {
		return err
	}
	if !isRaw {
		if _, err := decode(data, format); err != nil {
			return fmt.Errorf("validate: %w", err)
		}
	}

	stable := f.stablePath(snap.Namespace, format)
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

	idx := nsIndex{Hash: hash, Format: formatStr, IsRaw: isRaw, ReleaseKey: snap.ReleaseKey}
	if err := f.writeIndex(snap.Namespace, format, idx); err != nil {
		return err
	}
	f.hash[key] = hash
	return nil
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

func encode(values map[string]any, format Format) ([]byte, error) {
	clean := canonicalize(values)
	switch format {
	case FormatYAML, FormatYML:
		return yaml.Marshal(clean)
	case FormatXML:
		return mxj.Map(clean).XmlIndent("", "    ")
	default:
		return json.MarshalIndent(clean, "", "    ")
	}
}

func decode(data []byte, format Format) (map[string]any, error) {
	switch format {
	case FormatYAML, FormatYML:
		out := make(map[string]any)
		if err := yaml.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return out, nil
	case FormatXML:
		m, err := mxj.NewMapXml(data)
		if err != nil {
			return nil, err
		}
		return map[string]any(m), nil
	default:
		out := make(map[string]any)
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, err
		}
		return out, nil
	}
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
