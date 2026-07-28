package apollo

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	agollo "github.com/apolloconfig/agollo/v5"
	"github.com/apolloconfig/agollo/v5/env/config"
	"github.com/apolloconfig/agollo/v5/storage"
	"github.com/clbanning/mxj"
	"gopkg.in/yaml.v3"
)

// sdkClient 隔离 agollo.Client 的窄接口，便于测试。
type sdkClient interface {
	GetConfig(namespace string) *storage.Config
	AddChangeListener(listener storage.ChangeListener)
	RemoveChangeListener(listener storage.ChangeListener)
	Close()
}

// agolloAdapter 将 agollo 适配为 RemoteConfig，隔离 SDK 类型。
type agolloAdapter struct {
	client    sdkClient
	appConfig *config.AppConfig
	listener  *changeListener
	mu        sync.Mutex
	closed    bool
	closeOnce sync.Once
}

func newAgolloAdapter(cfg Config) (*agolloAdapter, error) {
	appConfig := &config.AppConfig{
		AppID:          cfg.AppID,
		Cluster:        cfg.Cluster,
		IP:             cfg.MetaServer,
		NamespaceName:  joinNames(cfg.Namespaces),
		Secret:         cfg.Secret,
		IsBackupConfig: false, // 使用自有 LKG，不依赖 SDK backup
	}
	client, err := agollo.StartWithConfig(func() (*config.AppConfig, error) {
		return appConfig, nil
	})
	if err != nil {
		return nil, fmt.Errorf("agollo start: %w", err)
	}
	return &agolloAdapter{client: client, appConfig: appConfig}, nil
}

// joinNames 拼接 namespace 列表，同名去重（agollo 订阅去重）。
func joinNames(nss []NamespaceConfig) string {
	seen := make(map[string]bool)
	out := ""
	for _, ns := range nss {
		if seen[ns.Name] {
			continue
		}
		seen[ns.Name] = true
		if len(out) > 0 {
			out += ","
		}
		out += ns.Name
	}
	return out
}

// isFileType 判断 namespace 是否为文件型（名带后缀）。
// Apollo 规则：非 properties namespace 必须带后缀（.json/.yaml/.xml 等）。
func isFileType(ns string) bool {
	dot := strings.LastIndex(ns, ".")
	return dot > 0 && dot < len(ns)-1
}

func (a *agolloAdapter) releaseKey(ns string) string {
	if a.appConfig == nil || a.appConfig.GetCurrentApolloConfig() == nil {
		return ""
	}
	return a.appConfig.GetCurrentApolloConfig().GetReleaseKey(ns)
}

func (a *agolloAdapter) Snapshot(ns string) (Snapshot, error) {
	cfg := a.client.GetConfig(ns)
	if cfg == nil {
		return Snapshot{}, fmt.Errorf("namespace %q unavailable", ns)
	}
	if !isFileType(ns) {
		// properties 无后缀：直接取 key-value
		cache := cfg.GetCache()
		values := make(map[string]any, cache.EntryCount())
		cache.Range(func(key, value any) bool {
			if ks, ok := key.(string); ok {
				values[ks] = value
			}
			return true
		})
		return Snapshot{Namespace: ns, Values: values, ReleaseKey: a.releaseKey(ns)}, nil
	}
	// 文件型：取 "content" key 的原始文本
	cache := cfg.GetCache()
	val, err := cache.Get("content")
	if err != nil {
		return Snapshot{}, fmt.Errorf("namespace %q get content: %w", ns, err)
	}
	content, _ := val.(string)
	src := formatFromExt(ns)
	if src == "txt" || src == "properties" {
		// 纯文本/properties 原样落盘，不解析
		return Snapshot{Namespace: ns, RawContent: content, ReleaseKey: a.releaseKey(ns)}, nil
	}
	values, err := parseContent(content, src)
	if err != nil {
		return Snapshot{}, fmt.Errorf("parse namespace %q: %w", ns, err)
	}
	return Snapshot{Namespace: ns, Values: values, ReleaseKey: a.releaseKey(ns)}, nil
}

// parseContent 将原始文本按源格式解析为 map。
func parseContent(content, format string) (map[string]any, error) {
	out := make(map[string]any)
	switch format {
	case "json":
		if err := json.Unmarshal([]byte(content), &out); err != nil {
			return nil, err
		}
		return out, nil
	case "yaml", "yml":
		if err := yaml.Unmarshal([]byte(content), &out); err != nil {
			return nil, err
		}
		return out, nil
	case "xml":
		m, err := mxj.NewMapXml([]byte(content))
		if err != nil {
			return nil, err
		}
		return map[string]any(m), nil
	}
	return nil, fmt.Errorf("unsupported source format: %s", format)
}

func (a *agolloAdapter) Subscribe(sink ChangeSink) (Subscription, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil, fmt.Errorf("adapter closed")
	}
	if a.listener != nil {
		return nil, fmt.Errorf("already subscribed")
	}
	a.listener = &changeListener{sink: sink, releaseKey: a.releaseKey}
	a.client.AddChangeListener(a.listener)
	return &adapterSubscription{}, nil
}

func (a *agolloAdapter) Close() {
	a.closeOnce.Do(func() {
		a.mu.Lock()
		a.closed = true
		l := a.listener
		a.mu.Unlock()
		if l != nil {
			a.client.RemoveChangeListener(l)
		}
		a.client.Close()
	})
}

// changeListener 实现 storage.ChangeListener。
type changeListener struct {
	sink       ChangeSink
	releaseKey func(string) string
}

// OnChange 增量变更事件，OnNewestChange 已提供完整快照，此处不处理。
func (l *changeListener) OnChange(event *storage.ChangeEvent) {}

// OnNewestChange 收到完整 namespace 快照，按类型转换后投递。
func (l *changeListener) OnNewestChange(event *storage.FullChangeEvent) {
	if l.sink == nil || event == nil {
		return
	}
	ns := event.Namespace
	releaseKey := ""
	if l.releaseKey != nil {
		releaseKey = l.releaseKey(ns)
	}
	if !isFileType(ns) {
		values := make(map[string]any, len(event.Changes))
		for k, v := range event.Changes {
			values[k] = v
		}
		l.sink.OnChange(Change{Namespace: ns, Values: values, ReleaseKey: releaseKey, NotificationID: event.NotificationID})
		return
	}
	content, _ := event.Changes["content"].(string)
	src := formatFromExt(ns)
	if src == "txt" || src == "properties" {
		l.sink.OnChange(Change{Namespace: ns, RawContent: content, ReleaseKey: releaseKey, NotificationID: event.NotificationID})
		return
	}
	values, err := parseContent(content, src)
	if err != nil {
		// 解析失败则丢弃本次变更，保留旧配置
		return
	}
	l.sink.OnChange(Change{Namespace: ns, Values: values, ReleaseKey: releaseKey, NotificationID: event.NotificationID})
}

type adapterSubscription struct{}

func (s *adapterSubscription) Close() {
	// 实际清理在 adapter.Close 统一处理。
}
