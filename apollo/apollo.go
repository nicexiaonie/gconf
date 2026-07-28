package apollo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Format 本地输出文件格式。
type Format string

const (
	// FormatJSON 以 JSON 输出。
	FormatJSON Format = "json"
	// FormatYAML 以 YAML 输出。
	FormatYAML Format = "yaml"
	// FormatYML 以 YML 输出（内容同 YAML，扩展名不同）。
	FormatYML Format = "yml"
	// FormatXML 以 XML 输出。
	FormatXML Format = "xml"
	// FormatTXT 以纯文本原样输出（不解析不转格式）。
	FormatTXT Format = "txt"
)

// NamespaceConfig 单个 Namespace 的同步配置。
type NamespaceConfig struct {
	// Name Apollo namespace 名称。
	Name string
	// Required 为 true 时，无远端且无 LKG 则 Start 失败。
	Required bool
	// Format 该 namespace 输出格式，空则用全局 Config.Format。
	Format Format
	// Filename 输出文件名（不含扩展名），空则用 Name。
	Filename string
}

// RetryConfig 重试配置。
type RetryConfig struct {
	MaxAttempts int
	Interval    time.Duration
}

// Config Syncer 配置。
type Config struct {
	AppID      string
	Cluster    string
	MetaServer string // Apollo meta server 或 config service 地址
	Secret     string
	CacheDir   string // 本地缓存与 LKG 根目录
	Namespaces []NamespaceConfig
	Format     Format // 全局默认输出格式，默认 json
	Watch      bool   // 是否监听 Apollo 变更并持续热更新，默认 false
	Merge      bool   // 合并多 namespace 为单文件，默认 false（本期仅占位）
	Retry      RetryConfig
}

// NamespaceStatus 单个 namespace 的运行状态。
type NamespaceStatus struct {
	Namespace   string
	Format      Format
	ReleaseKey  string
	Hash        string
	Source      string // "remote" | "lkg" | "empty"
	LastSuccess time.Time
	LastError   string
}

// Status Syncer 整体状态。
type Status struct {
	Namespaces []NamespaceStatus
}

// Snapshot 一份完整配置快照。
type Snapshot struct {
	Namespace      string
	Values         map[string]any // properties 型的 key-value（文件型为空）
	RawContent     string         // 文件型的原始文本（properties 型为空）
	ReleaseKey     string
	NotificationID int64
}

// Change 远端变更通知，携带完整快照。
type Change struct {
	Namespace      string
	Values         map[string]any // properties 型
	RawContent     string         // 文件型
	ReleaseKey     string
	NotificationID int64
}

// ChangeSink 变更接收方。
type ChangeSink interface {
	OnChange(Change)
}

// RemoteConfig 远端配置源抽象，由 adapter 实现。
type RemoteConfig interface {
	Snapshot(ns string) (Snapshot, error)
	Subscribe(sink ChangeSink) (Subscription, error)
	Close()
}

// Subscription 变更订阅句柄。
type Subscription interface {
	Close()
}

// SnapshotStore 本地快照存储抽象。
type SnapshotStore interface {
	Load(ns string, format Format) (Snapshot, error)
	Publish(snap Snapshot, format Format) error
	CurrentHash(ns string, format Format) string
}

// timeNow 便于测试替换时钟。
var timeNow = time.Now

// nsKey 返回 namespace+format 的状态 key，支持同名 namespace 多格式输出。
func nsKey(ns string, format Format) string {
	return ns + ":" + string(format)
}

// Syncer Apollo 配置同步器。
type Syncer struct {
	cfg    Config
	remote RemoteConfig
	store  SnapshotStore

	subs   []Subscription
	worker *worker

	mu        sync.Mutex
	status    map[string]*NamespaceStatus
	lastNID   map[string]int64
	closed    bool
	closeOnce sync.Once
}

// New 创建 Syncer，不连接远端。
func New(cfg Config) (*Syncer, error) {
	if err := validate(cfg); err != nil {
		return nil, err
	}
	if cfg.Format == "" {
		cfg.Format = FormatJSON
	}
	return &Syncer{
		cfg:     cfg,
		store:   newFileStore(cfg.CacheDir),
		status:  make(map[string]*NamespaceStatus),
		lastNID: make(map[string]int64),
	}, nil
}

func validate(cfg Config) error {
	if cfg.AppID == "" {
		return fmt.Errorf("apollo: AppID required")
	}
	if cfg.MetaServer == "" {
		return fmt.Errorf("apollo: MetaServer required")
	}
	if cfg.CacheDir == "" {
		return fmt.Errorf("apollo: CacheDir required")
	}
	if len(cfg.Namespaces) == 0 {
		return fmt.Errorf("apollo: at least one namespace required")
	}
	for i, ns := range cfg.Namespaces {
		if ns.Name == "" {
			return fmt.Errorf("apollo: namespace[%d].Name required", i)
		}
	}
	return nil
}

// resolveFormat 决定 namespace 的目标输出格式。
// 优先级：NamespaceConfig.Format > 文件型后缀格式（保持原格式）> Config.Format > json
func resolveFormat(nsCfg NamespaceConfig, global Format) Format {
	if nsCfg.Format != "" {
		return nsCfg.Format
	}
	if isFileType(nsCfg.Name) {
		if f := formatFromExt(nsCfg.Name); f != "" {
			return Format(f)
		}
	}
	if global != "" {
		return global
	}
	return FormatJSON
}

// Start 连接 Apollo、拉取首份快照、启动变更监听。
func (s *Syncer) Start(ctx context.Context) error {
	if s.remote == nil {
		remote, err := newAgolloAdapter(s.cfg)
		if err != nil {
			return err
		}
		s.remote = remote
	}

	// 初始化每 namespace 状态，尝试加载 LKG。
	for _, ns := range s.cfg.Namespaces {
		format := resolveFormat(ns, s.cfg.Format)
		key := nsKey(ns.Name, format)
		st := &NamespaceStatus{Namespace: ns.Name, Format: format, Source: "empty"}
		if lkg, err := s.store.Load(ns.Name, format); err == nil {
			st.ReleaseKey = lkg.ReleaseKey
			st.Hash = s.store.CurrentHash(ns.Name, format)
			st.Source = "lkg"
		}
		s.status[key] = st
	}

	// 逐 namespace 拉取首份快照。
	var firstErr error
	for _, ns := range s.cfg.Namespaces {
		format := resolveFormat(ns, s.cfg.Format)
		key := nsKey(ns.Name, format)
		snap, err := s.remote.Snapshot(ns.Name)
		if err != nil {
			if s.status[key].Source == "lkg" {
				continue // 远端失败但 LKG 可用，降级
			}
			if ns.Required {
				if firstErr == nil {
					firstErr = fmt.Errorf("apollo: namespace %s required but unavailable: %w", ns.Name, err)
				}
			}
			s.status[key].LastError = err.Error()
			continue
		}
		if err := s.store.Publish(snap, format); err != nil {
			if ns.Required && s.status[key].Source != "lkg" {
				if firstErr == nil {
					firstErr = fmt.Errorf("apollo: namespace %s publish failed: %w", ns.Name, err)
				}
			}
			s.status[key].LastError = err.Error()
			continue
		}
		s.markSuccess(snap, format)
	}
	if firstErr != nil {
		s.Close()
		return firstErr
	}

	if !s.cfg.Watch {
		return nil
	}

	// 启用 Watch 时才启动变更监听。
	s.worker = newWorker(s)
	sub, err := s.remote.Subscribe(s.worker)
	if err != nil {
		s.Close()
		return fmt.Errorf("apollo: subscribe failed: %w", err)
	}
	s.subs = append(s.subs, sub)
	return nil
}

// handleChange 在 worker goroutine 中处理变更。
// 收到一个 namespace 的变更后，遍历所有同名 nsCfg，分别按各自 Format 发布。
func (s *Syncer) handleChange(ch Change) {
	s.mu.Lock()
	// 丢弃过期 NotificationID（按 namespace，同名多格式共用）。
	if ch.NotificationID > 0 {
		if last := s.lastNID[ch.Namespace]; ch.NotificationID < last {
			s.mu.Unlock()
			return
		}
		s.lastNID[ch.Namespace] = ch.NotificationID
	}
	s.mu.Unlock()

	for _, nsCfg := range s.cfg.Namespaces {
		if nsCfg.Name != ch.Namespace {
			continue
		}
		format := resolveFormat(nsCfg, s.cfg.Format)
		snap := Snapshot{
			Namespace:      ch.Namespace,
			Values:         ch.Values,
			RawContent:     ch.RawContent,
			ReleaseKey:     ch.ReleaseKey,
			NotificationID: ch.NotificationID,
		}
		if err := s.store.Publish(snap, format); err != nil {
			s.markError(ch.Namespace, format, err)
			continue
		}
		s.markSuccess(snap, format)
	}
}

func (s *Syncer) markSuccess(snap Snapshot, format Format) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := nsKey(snap.Namespace, format)
	st := s.status[key]
	if st == nil {
		st = &NamespaceStatus{Namespace: snap.Namespace, Format: format}
		s.status[key] = st
	}
	st.ReleaseKey = snap.ReleaseKey
	st.Hash = s.store.CurrentHash(snap.Namespace, format)
	st.Source = "remote"
	st.LastSuccess = timeNow()
	st.LastError = ""
}

func (s *Syncer) markError(ns string, format Format, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := nsKey(ns, format)
	st := s.status[key]
	if st == nil {
		st = &NamespaceStatus{Namespace: ns, Format: format}
		s.status[key] = st
	}
	st.LastError = err.Error()
}

// Status 返回当前状态快照。
func (s *Syncer) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Status{}
	for _, ns := range s.cfg.Namespaces {
		format := resolveFormat(ns, s.cfg.Format)
		key := nsKey(ns.Name, format)
		if st := s.status[key]; st != nil {
			out.Namespaces = append(out.Namespaces, *st)
		}
	}
	return out
}

// Close 停止同步，幂等。
func (s *Syncer) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		for _, sub := range s.subs {
			sub.Close()
		}
		s.subs = nil
		if s.worker != nil {
			s.worker.stop()
		}
		if s.remote != nil {
			s.remote.Close()
		}
	})
	return nil
}

// contentHashOf 计算 values 的稳定内容 hash，用于状态展示与去重后备。
func contentHashOf(values map[string]any) string {
	data, err := json.Marshal(canonicalize(values))
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// canonicalize 返回 key 排序后的 map，保证序列化稳定。
func canonicalize(values map[string]any) map[string]any {
	if values == nil {
		return map[string]any{}
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]any, len(keys))
	for _, k := range keys {
		out[k] = values[k]
	}
	return out
}
