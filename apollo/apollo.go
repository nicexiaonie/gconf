package apollo

import (
	"context"
	"fmt"
	"path/filepath"
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
	// FormatTOML 以 TOML 输出（仅作为转换目标）。
	FormatTOML Format = "toml"
	// FormatProperties 以 properties 输出，也是 properties 型 namespace 的原样落地格式。
	FormatProperties Format = "properties"
	// FormatTXT 以纯文本原样输出（不解析不转格式）。
	FormatTXT Format = "txt"
)

// NamespaceConfig 单个 Namespace 的同步配置。
type NamespaceConfig struct {
	// Name Apollo namespace 名称。
	Name string
	// Required 为 true 时，无远端且无 LKG 则 Start 失败。
	Required bool
	// Format 目标格式。为空或与源格式相同时原样落地：文件型写 Apollo 原文，
	// properties 型写为 properties 文件；与源格式不同时解析后转换。
	Format Format
	// Filename 输出文件名，严格按配置写入，不校验扩展名；空则根据 Name 和目标格式生成。
	Filename string
	// SkipValidate 为 true 时，原样落地前跳过解析检查；仅适用于原样落地，配置转换时不可用。
	SkipValidate bool
}

// RetryConfig Start 首次拉取的重试配置。
// agollo 首次同步失败后由后台长轮询继续加载，重试即按间隔等待其加载完成；零值表示不重试。
type RetryConfig struct {
	// MaxAttempts 首次拉取的总尝试次数（含第一次），0 或 1 表示不重试。
	MaxAttempts int
	// Interval 两次尝试的间隔，MaxAttempts 大于 1 时必填。
	Interval time.Duration
}

// Config Syncer 配置。
type Config struct {
	AppID      string
	Cluster    string
	MetaServer string // Apollo meta server 或 config service 地址
	Secret     string
	CacheDir   string // 本地缓存与 LKG 根目录
	Namespaces []NamespaceConfig
	Watch      bool // 是否监听 Apollo 变更并持续热更新，默认 false
	Merge      bool // 合并多 namespace 为单文件，默认 false（本期仅占位）
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
	// Load 校验 LKG 完整性并返回其 releaseKey。
	Load(ns string, format Format) (string, error)
	// Publish 原子写入已生成的文件内容。
	Publish(ns string, format Format, data []byte, releaseKey string) error
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
	closeOnce sync.Once
}

// New 创建 Syncer，不连接远端。
func New(cfg Config) (*Syncer, error) {
	if err := validate(cfg); err != nil {
		return nil, err
	}
	return &Syncer{
		cfg:     cfg,
		store:   newFileStore(cfg.CacheDir, outputFilenames(cfg)),
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
	if cfg.Retry.MaxAttempts < 0 || cfg.Retry.Interval < 0 {
		return fmt.Errorf("apollo: Retry.MaxAttempts and Retry.Interval must not be negative")
	}
	if cfg.Retry.MaxAttempts > 1 && cfg.Retry.Interval == 0 {
		return fmt.Errorf("apollo: Retry.Interval required when Retry.MaxAttempts > 1")
	}
	outputs := make(map[string]string, len(cfg.Namespaces))
	configured := make(map[string]struct{}, len(cfg.Namespaces))
	for i, ns := range cfg.Namespaces {
		if ns.Name == "" {
			return fmt.Errorf("apollo: namespace[%d].Name required", i)
		}
		if ns.Format != "" && !isKnownFormat(ns.Format) {
			return fmt.Errorf("apollo: namespace[%d].Format %q unsupported", i, ns.Format)
		}
		format := resolveFormat(ns)
		if src := sourceFormat(ns.Name); format != src {
			if isFileType(ns.Name) && !isStructured(src) {
				return fmt.Errorf("apollo: namespace %s (%s) supports raw output only, cannot convert to %s", ns.Name, src, format)
			}
			if format == FormatTXT {
				return fmt.Errorf("apollo: namespace %s cannot convert to %s", ns.Name, format)
			}
			if ns.SkipValidate {
				return fmt.Errorf("apollo: namespace %s SkipValidate applies to raw output only, but converts %s to %s", ns.Name, src, format)
			}
		}
		key := nsKey(ns.Name, format)
		if _, ok := configured[key]; ok {
			return fmt.Errorf("apollo: namespace %s with format %s configured more than once", ns.Name, format)
		}
		configured[key] = struct{}{}
		filename, err := outputFilename(ns, format)
		if err != nil {
			return fmt.Errorf("apollo: namespace[%d].Filename: %w", i, err)
		}
		if previous, ok := outputs[filename]; ok {
			return fmt.Errorf("apollo: output filename %q conflicts between %s and %s", filename, previous, ns.Name)
		}
		outputs[filename] = ns.Name
	}
	return nil
}

func outputFilenames(cfg Config) map[string]string {
	filenames := make(map[string]string, len(cfg.Namespaces))
	for _, ns := range cfg.Namespaces {
		format := resolveFormat(ns)
		filename, _ := outputFilename(ns, format)
		filenames[nsKey(ns.Name, format)] = filename
	}
	return filenames
}

func outputFilename(ns NamespaceConfig, format Format) (string, error) {
	if ns.Filename == "" {
		return nsBase(ns.Name) + "." + string(format), nil
	}
	if filepath.Base(ns.Filename) != ns.Filename || ns.Filename == "." || ns.Filename == ".." {
		return "", fmt.Errorf("must be a file name without a directory")
	}
	return ns.Filename, nil
}

// resolveFormat 决定 namespace 的目标格式：配置了 Format 用配置值，否则为源格式（原样落地）。
func resolveFormat(nsCfg NamespaceConfig) Format {
	if nsCfg.Format != "" {
		return nsCfg.Format
	}
	return sourceFormat(nsCfg.Name)
}

// Start 连接 Apollo、拉取首份快照、启动变更监听。
// ctx 用于取消启动过程（连接 Apollo 与重试等待），结束时 Start 关闭已建立的连接并返回 ctx 的错误；Start 返回后不再使用 ctx。
func (s *Syncer) Start(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("apollo: start canceled: %w", err)
	}
	if s.remote == nil {
		remote, err := startAgolloAdapter(ctx, s.cfg)
		if err != nil {
			return err
		}
		s.remote = remote
	}

	// 启用 Watch 时先订阅再拉取首份快照：拉取期间的发布暂存在 worker 中，首次同步完成后再应用，避免丢失。
	if s.cfg.Watch {
		s.worker = newWorker(s)
		sub, err := s.remote.Subscribe(s.worker)
		if err != nil {
			s.Close()
			return fmt.Errorf("apollo: subscribe failed: %w", err)
		}
		s.subs = append(s.subs, sub)
	}

	// 初始化每 namespace 状态，尝试加载 LKG。
	for _, ns := range s.cfg.Namespaces {
		format := resolveFormat(ns)
		key := nsKey(ns.Name, format)
		st := &NamespaceStatus{Namespace: ns.Name, Format: format, Source: "empty"}
		if releaseKey, err := s.store.Load(ns.Name, format); err == nil {
			st.ReleaseKey = releaseKey
			st.Hash = s.store.CurrentHash(ns.Name, format)
			st.Source = "lkg"
		}
		s.status[key] = st
	}

	snaps, snapErrs, err := s.fetchSnapshots(ctx)
	if err != nil {
		s.Close()
		return err
	}

	// 逐输出项发布首份快照。
	// 远端不可用与写盘失败按 Required/LKG 决定是否失败；配置内容错误属人为问题，一律失败。
	var firstErr error
	for _, ns := range s.cfg.Namespaces {
		format := resolveFormat(ns)
		key := nsKey(ns.Name, format)
		snap, ok := snaps[ns.Name]
		if !ok {
			err := snapErrs[ns.Name]
			s.status[key].LastError = err.Error()
			if s.status[key].Source == "lkg" {
				continue // 远端失败但 LKG 可用，降级
			}
			if ns.Required {
				if firstErr == nil {
					firstErr = fmt.Errorf("apollo: namespace %s required but unavailable: %w", ns.Name, err)
				}
			}
			continue
		}
		data, err := render(ns, snap)
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("apollo: namespace %s invalid content: %w", ns.Name, err)
			}
			s.status[key].LastError = err.Error()
			continue
		}
		if err := s.store.Publish(ns.Name, format, data, snap.ReleaseKey); err != nil {
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

	if s.worker != nil {
		s.worker.start()
	}
	return nil
}

// fetchSnapshots 拉取所有 namespace 的首份快照，同名 namespace 只拉取一次。
// 返回成功的快照与最后一次失败的错误；按 Retry 对失败的 namespace 重试，ctx 结束时返回其错误。
func (s *Syncer) fetchSnapshots(ctx context.Context) (map[string]Snapshot, map[string]error, error) {
	snaps := make(map[string]Snapshot)
	errs := make(map[string]error)
	seen := make(map[string]bool)
	var pending []string
	for _, ns := range s.cfg.Namespaces {
		if seen[ns.Name] {
			continue
		}
		seen[ns.Name] = true
		pending = append(pending, ns.Name)
	}
	attempts := s.cfg.Retry.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 1; ; attempt++ {
		var failed []string
		for _, name := range pending {
			snap, err := s.remote.Snapshot(name)
			if err != nil {
				errs[name] = err
				failed = append(failed, name)
				continue
			}
			snaps[name] = snap
			delete(errs, name)
		}
		pending = failed
		if len(pending) == 0 || attempt >= attempts {
			return snaps, errs, nil
		}
		timer := time.NewTimer(s.cfg.Retry.Interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, nil, fmt.Errorf("apollo: start canceled: %w", ctx.Err())
		case <-timer.C:
		}
	}
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
		format := resolveFormat(nsCfg)
		snap := Snapshot{
			Namespace:      ch.Namespace,
			Values:         ch.Values,
			RawContent:     ch.RawContent,
			ReleaseKey:     ch.ReleaseKey,
			NotificationID: ch.NotificationID,
		}
		data, err := render(nsCfg, snap)
		if err != nil {
			s.markError(ch.Namespace, format, err)
			continue
		}
		if err := s.store.Publish(ch.Namespace, format, data, snap.ReleaseKey); err != nil {
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
		format := resolveFormat(ns)
		key := nsKey(ns.Name, format)
		if st := s.status[key]; st != nil {
			out.Namespaces = append(out.Namespaces, *st)
		}
	}
	return out
}

// Close 停止同步，幂等。
// 先关闭远端以停止新的回调，再停止 worker；已在途的回调由 worker 在关闭后忽略。
func (s *Syncer) Close() error {
	s.closeOnce.Do(func() {
		for _, sub := range s.subs {
			sub.Close()
		}
		s.subs = nil
		if s.remote != nil {
			s.remote.Close()
		}
		if s.worker != nil {
			s.worker.stop()
		}
	})
	return nil
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
