package apollo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/apolloconfig/agollo/v5/storage"
)

// newFakeApolloServer 模拟 Apollo Config Service 的 configfiles 接口，返回各 namespace 的 configurations。
func newFakeApolloServer(t *testing.T, configs map[string]map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/configfiles/json/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		ns := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		_ = json.NewEncoder(w).Encode(configs[ns])
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestAdapterSnapshotThroughAgollo 经真实 agollo 客户端拉取：文件型取到 Apollo 原文，properties 型取到键值
func TestAdapterSnapshotThroughAgollo(t *testing.T) {
	configs := map[string]map[string]string{
		"rules.yaml":  {"content": "# 服务配置\nserver:\n  port: 8080\nlist: [a, b]\n"},
		"rules.yml":   {"content": "server:\n  port: 8080\n"},
		"config.json": {"content": `{"server": {"port": 8080}}`},
		"TEST1.redis": {"host": "127.0.0.1"},
	}
	srv := newFakeApolloServer(t, configs)

	nss := make([]NamespaceConfig, 0, len(configs))
	for ns := range configs {
		nss = append(nss, NamespaceConfig{Name: ns})
	}
	a, err := newAgolloAdapter(Config{AppID: "app", Cluster: "default", MetaServer: srv.URL, Namespaces: nss})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	for _, ns := range []string{"rules.yaml", "rules.yml", "config.json"} {
		t.Run(ns, func(t *testing.T) {
			snap, err := a.Snapshot(ns)
			if err != nil {
				t.Fatal(err)
			}
			if snap.RawContent != configs[ns]["content"] {
				t.Errorf("期望原文 %q，得到 %q", configs[ns]["content"], snap.RawContent)
			}
		})
	}

	t.Run("TEST1.redis", func(t *testing.T) {
		snap, err := a.Snapshot("TEST1.redis")
		if err != nil {
			t.Fatal(err)
		}
		if want := map[string]any{"host": "127.0.0.1"}; !reflect.DeepEqual(snap.Values, want) {
			t.Errorf("期望 %v，得到 %v", want, snap.Values)
		}
	})
}

type captureSink struct {
	changes []Change
}

func (s *captureSink) OnChange(ch Change) {
	s.changes = append(s.changes, ch)
}

// TestChangeListenerDottedPropertiesNamespace 验证带点但非已知后缀的 namespace 按 properties 型投递变更
func TestChangeListenerDottedPropertiesNamespace(t *testing.T) {
	sink := &captureSink{}
	l := &changeListener{sink: sink}
	ev := &storage.FullChangeEvent{Changes: map[string]any{"host": "127.0.0.1"}}
	ev.Namespace = "TEST1.redis"
	l.OnNewestChange(ev)

	if len(sink.changes) != 1 {
		t.Fatalf("期望 1 次变更，得到 %d", len(sink.changes))
	}
	if got := sink.changes[0].Values; !reflect.DeepEqual(got, map[string]any{"host": "127.0.0.1"}) {
		t.Errorf("期望 host=127.0.0.1，得到 %v", got)
	}
}

func TestIsFileType(t *testing.T) {
	cases := map[string]bool{
		"application":       false,
		"TEST1.redis":       false,
		"middleware.redis":  false,
		"config.json":       true,
		"rules.yaml":        true,
		"rules.yml":         true,
		"mapping.xml":       true,
		"notice.txt":        true,
		"legacy.properties": true,
	}
	for ns, want := range cases {
		if got := isFileType(ns); got != want {
			t.Errorf("isFileType(%q) 期望 %v，得到 %v", ns, want, got)
		}
	}
}

func TestOutputFilenameDottedNamespace(t *testing.T) {
	cases := []struct {
		ns     string
		format Format
		want   string
	}{
		{"TEST1.redis", FormatJSON, "TEST1.redis.json"},
		{"application", FormatYAML, "application.yaml"},
		{"config.json", FormatYAML, "config.yaml"},
	}
	for _, c := range cases {
		got, err := outputFilename(NamespaceConfig{Name: c.ns}, c.format)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("outputFilename(%q, %s) 期望 %q，得到 %q", c.ns, c.format, c.want, got)
		}
	}
}

// TestAdapterSnapshotNotLoaded Apollo 不可用时 agollo 中只有预建的空缓存，不能当作空配置返回，否则会覆盖 LKG
func TestAdapterSnapshotNotLoaded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	a, err := newAgolloAdapter(Config{AppID: "app", Cluster: "default", MetaServer: srv.URL, Namespaces: []NamespaceConfig{{Name: "application"}, {Name: "config.json"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	for _, ns := range []string{"application", "config.json"} {
		if snap, err := a.Snapshot(ns); err == nil {
			t.Errorf("%s: expected error for unloaded namespace, got %+v", ns, snap)
		}
	}
}

// TestStartAdapterCanceled agollo 启动阻塞时，ctx 结束应让 Start 立即返回
func TestStartAdapterCanceled(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-block
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(block) })

	s, err := New(Config{AppID: "app", MetaServer: srv.URL, CacheDir: t.TempDir(), Namespaces: []NamespaceConfig{{Name: "application"}}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := s.Start(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("start did not return on ctx deadline")
	}
}
