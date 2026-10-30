package apollo

import (
	"context"
	"fmt"
	"sync"

	agollo "github.com/apolloconfig/agollo/v5"
	"github.com/apolloconfig/agollo/v5/constant"
	"github.com/apolloconfig/agollo/v5/env/config"
	"github.com/apolloconfig/agollo/v5/extension"
	"github.com/apolloconfig/agollo/v5/storage"
	"github.com/apolloconfig/agollo/v5/utils/parse/normal"
)

// agollo 默认会把 yaml/yml namespace 预解析为扁平键值并丢弃 "content" 原文，
// 这里改为不预解析（同 json），保留原文由本模块统一解析。
// 该注册对整个进程生效，进程内其他直接使用 agollo 读 yaml 的代码将拿不到扁平键值。
func init() {
	extension.AddFormatParser(constant.YAML, &normal.Parser{})
	extension.AddFormatParser(constant.YML, &normal.Parser{})
}

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

// startAgolloAdapter 在 ctx 约束下创建 agollo 适配器。
// agollo 启动会阻塞到首次同步结束（远端不可用时可达十余秒）且不支持取消，因此放到 goroutine 中执行；
// ctx 先结束时立即返回，之后才创建成功的客户端随即关闭。
func startAgolloAdapter(ctx context.Context, cfg Config) (RemoteConfig, error) {
	type result struct {
		adapter *agolloAdapter
		err     error
	}
	done := make(chan result, 1)
	go func() {
		a, err := newAgolloAdapter(cfg)
		done <- result{adapter: a, err: err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			return nil, r.err
		}
		return r.adapter, nil
	case <-ctx.Done():
		go func() {
			if r := <-done; r.err == nil {
				r.adapter.Close()
			}
		}()
		return nil, fmt.Errorf("apollo: start canceled: %w", ctx.Err())
	}
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

// isFileType 判断 namespace 是否为文件型（名带已知格式后缀）。
// 与 Apollo/agollo 一致：仅 .json/.yaml/.yml/.xml/.txt/.properties 视为文件型，
// 其余带点名称（如公共 namespace 的部门前缀 TEST1.redis）按 properties 型处理。
func isFileType(ns string) bool {
	return formatFromExt(ns) != ""
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
	// agollo 启动时为每个 namespace 预建空缓存，首次成功同步后才标记为已初始化；
	// 未初始化的空缓存不代表远端配置为空，必须视为不可用，否则会以空配置覆盖 LKG。
	if !cfg.GetIsInit() {
		return Snapshot{}, fmt.Errorf("namespace %q not loaded from apollo", ns)
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
	// 原文交由 render 按配置原样落地或解析转换，此处不解析
	return Snapshot{Namespace: ns, RawContent: content, ReleaseKey: a.releaseKey(ns)}, nil
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

// OnNewestChange 收到完整 namespace 快照，按 namespace 类型投递键值或原文。
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
	l.sink.OnChange(Change{Namespace: ns, RawContent: content, ReleaseKey: releaseKey, NotificationID: event.NotificationID})
}

type adapterSubscription struct{}

func (s *adapterSubscription) Close() {
	// 实际清理在 adapter.Close 统一处理。
}
