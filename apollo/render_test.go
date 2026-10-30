package apollo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRenderRawVerbatim 未配置 Format 时文件型原样落地，注释、key 顺序、缩进均保持不变
func TestRenderRawVerbatim(t *testing.T) {
	cases := map[string]string{
		"config.json": "{\n  \"b\": 1,\n  \"a\": {\"x\": 2}\n}\n",
		"rules.yaml":  "# 服务配置\nserver:\n  port: 8080 # 端口\nlist: [b, a]\n",
		"rules.yml":   "b: 1\na: 2\n",
		"mapping.xml": "<!-- 映射 --><root><b>1</b><a>2</a></root>",
		"notice.txt":  "任意文本 {not json",
	}
	for ns, raw := range cases {
		t.Run(ns, func(t *testing.T) {
			data, err := render(NamespaceConfig{Name: ns}, Snapshot{Namespace: ns, RawContent: raw})
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != raw {
				t.Errorf("期望原文 %q，得到 %q", raw, data)
			}
		})
	}
}

// TestRenderSameFormatIsRaw 配置的 Format 与源格式相同时同样原样落地
func TestRenderSameFormatIsRaw(t *testing.T) {
	raw := "{\"b\": 1, \"a\": 2}"
	data, err := render(NamespaceConfig{Name: "config.json", Format: FormatJSON}, Snapshot{RawContent: raw})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != raw {
		t.Errorf("期望原文 %q，得到 %q", raw, data)
	}
}

func TestRenderRawValidation(t *testing.T) {
	invalid := "{\"a\": 1"
	if _, err := render(NamespaceConfig{Name: "config.json"}, Snapshot{RawContent: invalid}); err == nil {
		t.Fatal("默认应检查解析，期望报错")
	}
	data, err := render(NamespaceConfig{Name: "config.json", SkipValidate: true}, Snapshot{RawContent: invalid})
	if err != nil {
		t.Fatalf("SkipValidate 时不应报错: %v", err)
	}
	if string(data) != invalid {
		t.Errorf("期望原文 %q，得到 %q", invalid, data)
	}
}

// TestRenderPropertiesNamespace properties 型 namespace 原样落地为按 key 排序的 properties 文件
func TestRenderPropertiesNamespace(t *testing.T) {
	values := map[string]any{"port": "6379", "host": "127.0.0.1", "url": "${base}/x"}
	data, err := render(NamespaceConfig{Name: "TEST1.redis"}, Snapshot{Values: values})
	if err != nil {
		t.Fatal(err)
	}
	want := "host=127.0.0.1\nport=6379\nurl=${base}/x\n"
	if string(data) != want {
		t.Errorf("期望 %q，得到 %q", want, data)
	}
}

func TestRenderConvert(t *testing.T) {
	cases := []struct {
		name   string
		ns     string
		format Format
		snap   Snapshot
		want   string
	}{
		{
			name:   "json 整数保持整数",
			ns:     "config.json",
			format: FormatTOML,
			snap:   Snapshot{RawContent: `{"server": {"port": 8080, "rate": 0.5}}`},
			want:   "\n[server]\n  port = 8080\n  rate = 0.5\n",
		},
		{
			name:   "yaml 对象数组转 toml",
			ns:     "rules.yaml",
			format: FormatTOML,
			snap:   Snapshot{RawContent: "servers:\n  - host: a\n  - host: b\n"},
			want:   "\n[[servers]]\n  host = \"a\"\n\n[[servers]]\n  host = \"b\"\n",
		},
		{
			name:   "properties 型转 toml",
			ns:     "application",
			format: FormatTOML,
			snap:   Snapshot{Values: map[string]any{"host": "127.0.0.1"}},
			want:   "host = \"127.0.0.1\"\n",
		},
		{
			name:   "嵌套展开为 properties",
			ns:     "config.json",
			format: FormatProperties,
			snap:   Snapshot{RawContent: `{"server": {"port": 8080, "debug": true}, "rate": 0.5}`},
			want:   "rate=0.5\nserver.debug=true\nserver.port=8080\n",
		},
		{
			name:   "json 大整数不丢精度",
			ns:     "config.json",
			format: FormatYAML,
			snap:   Snapshot{RawContent: `{"big": 9007199254740993}`},
			want:   "big: 9007199254740993\n",
		},
		{
			name:   "properties 型转 json",
			ns:     "application",
			format: FormatJSON,
			snap:   Snapshot{Values: map[string]any{"k": "v"}},
			want:   "{\n    \"k\": \"v\"\n}",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := render(NamespaceConfig{Name: c.ns, Format: c.format}, c.snap)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != c.want {
				t.Errorf("期望 %q，得到 %q", c.want, data)
			}
		})
	}
}

func TestRenderConvertErrors(t *testing.T) {
	cases := []struct {
		name   string
		format Format
		raw    string
		errHas string
	}{
		{"toml 不支持 null", FormatTOML, `{"a": null}`, "null"},
		{"toml 数组内 null", FormatTOML, `{"a": [1, null]}`, "null"},
		{"toml 混合类型数组", FormatTOML, `{"a": [1, "x", true]}`, "same type"},
		{"toml 整数与小数混合", FormatTOML, `{"a": [1, 2.5]}`, "same type"},
		{"properties 不支持数组", FormatProperties, `{"a": [1, 2]}`, "array"},
		{"properties 不支持 null", FormatProperties, `{"a": null}`, "null"},
		{"properties 展开后 key 重复", FormatProperties, `{"a": {"b": 1}, "a.b": 2}`, "duplicate"},
		{"properties key 含 = 无法表达", FormatProperties, `{"k=x": "v"}`, "cannot be represented"},
		{"源内容解析失败", FormatTOML, `{"a": `, "parse json"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := render(NamespaceConfig{Name: "config.json", Format: c.format}, Snapshot{RawContent: c.raw})
			if err == nil {
				t.Fatal("期望报错")
			}
			if !strings.Contains(err.Error(), c.errHas) {
				t.Errorf("错误信息应包含 %q，得到 %v", c.errHas, err)
			}
		})
	}
}

func TestRenderTOMLRoundTrip(t *testing.T) {
	raw := `{"a": [[1, 2], ["x"]], "e": [], "t": {"n": 1}}`
	data, err := render(NamespaceConfig{Name: "config.json", Format: FormatTOML}, Snapshot{RawContent: raw})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decode(data, FormatTOML); err != nil {
		t.Fatalf("toml 无法解析回来: %v\n%s", err, data)
	}
}

func TestValidateFormatRules(t *testing.T) {
	base := Config{AppID: "app", MetaServer: "http://x", CacheDir: t.TempDir()}
	invalid := []struct {
		name string
		ns   NamespaceConfig
	}{
		{"未知格式", NamespaceConfig{Name: "config.json", Format: "ini"}},
		{"txt 不能转换", NamespaceConfig{Name: "notice.txt", Format: FormatJSON}},
		{".properties 文件型不能转换", NamespaceConfig{Name: "legacy.properties", Format: FormatJSON}},
		{"properties 型不能转 txt", NamespaceConfig{Name: "application", Format: FormatTXT}},
		{"结构化源不能转 txt", NamespaceConfig{Name: "config.json", Format: FormatTXT}},
		{"转换时不能跳过检查", NamespaceConfig{Name: "config.json", Format: FormatTOML, SkipValidate: true}},
		{"文件名带目录", NamespaceConfig{Name: "config.json", Filename: "../config.json"}},
	}
	for _, c := range invalid {
		t.Run(c.name, func(t *testing.T) {
			cfg := base
			cfg.Namespaces = []NamespaceConfig{c.ns}
			if _, err := New(cfg); err == nil {
				t.Fatal("期望校验失败")
			}
		})
	}

	valid := []struct {
		name string
		ns   NamespaceConfig
	}{
		{"原样落地", NamespaceConfig{Name: "config.json"}},
		{"txt 原样", NamespaceConfig{Name: "notice.txt", Format: FormatTXT}},
		{"原样落地跳过检查", NamespaceConfig{Name: "config.json", SkipValidate: true}},
		{"转 toml", NamespaceConfig{Name: "config.json", Format: FormatTOML}},
		{"properties 型转 json", NamespaceConfig{Name: "application", Format: FormatJSON}},
		{"文件名扩展名与格式不一致", NamespaceConfig{Name: "config.json", Filename: "aa.xml"}},
		{"文件名无扩展名", NamespaceConfig{Name: "TEST1.redis", Filename: "redis"}},
	}
	for _, c := range valid {
		t.Run(c.name, func(t *testing.T) {
			cfg := base
			cfg.Namespaces = []NamespaceConfig{c.ns}
			if _, err := New(cfg); err != nil {
				t.Fatalf("期望校验通过: %v", err)
			}
		})
	}
}

func TestOutputFilenameDefault(t *testing.T) {
	cases := []struct {
		ns   NamespaceConfig
		want string
	}{
		{NamespaceConfig{Name: "application"}, "application.properties"},
		{NamespaceConfig{Name: "TEST1.redis"}, "TEST1.redis.properties"},
		{NamespaceConfig{Name: "config.json"}, "config.json"},
		{NamespaceConfig{Name: "rules.yml"}, "rules.yml"},
		{NamespaceConfig{Name: "config.json", Format: FormatTOML}, "config.toml"},
		{NamespaceConfig{Name: "config.json", Format: FormatTOML, Filename: "app.conf"}, "app.conf"},
	}
	for _, c := range cases {
		got, err := outputFilename(c.ns, resolveFormat(c.ns))
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("%+v 期望 %q，得到 %q", c.ns, c.want, got)
		}
	}
}

// TestSyncerInvalidContentFailsWithLKG 启动时内容错误属人为配置问题，即使有 LKG 也启动失败
func TestSyncerInvalidContentFailsWithLKG(t *testing.T) {
	dir := t.TempDir()
	if err := newFileStore(dir).Publish("config.json", FormatJSON, []byte(`{"k":"lkg"}`), "r1"); err != nil {
		t.Fatal(err)
	}
	remote := newFakeRemote()
	remote.snaps["config.json"] = Snapshot{Namespace: "config.json", RawContent: `{"k": `}
	cfg := Config{AppID: "app", MetaServer: "http://x", CacheDir: dir, Namespaces: []NamespaceConfig{{Name: "config.json"}}}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err == nil {
		s.Close()
		t.Fatal("期望启动失败")
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"k":"lkg"}` {
		t.Fatalf("内容错误不应覆盖已有文件，得到 %s", data)
	}
}

// TestSyncerInvalidChangeKeepsFile 运行中收到无法解析的变更，不写入并记录 LastError
func TestSyncerInvalidChangeKeepsFile(t *testing.T) {
	dir := t.TempDir()
	remote := newFakeRemote()
	remote.snaps["config.json"] = Snapshot{Namespace: "config.json", RawContent: `{"k": "v1"}`}
	cfg := Config{AppID: "app", MetaServer: "http://x", CacheDir: dir, Watch: true, Namespaces: []NamespaceConfig{{Name: "config.json"}}}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	remote.emit(Change{Namespace: "config.json", RawContent: `{"k": `, NotificationID: 2})
	deadline := time.Now().Add(2 * time.Second)
	for s.Status().Namespaces[0].LastError == "" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if s.Status().Namespaces[0].LastError == "" {
		t.Fatal("期望记录 LastError")
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"k": "v1"}` {
		t.Fatalf("无效变更不应覆盖文件，得到 %s", data)
	}
}

// TestSyncerSkipValidateWritesInvalid 跳过检查时原样写入，问题交由读取方暴露
func TestSyncerSkipValidateWritesInvalid(t *testing.T) {
	dir := t.TempDir()
	remote := newFakeRemote()
	remote.snaps["config.json"] = Snapshot{Namespace: "config.json", RawContent: `{"k": `}
	cfg := Config{AppID: "app", MetaServer: "http://x", CacheDir: dir, Namespaces: []NamespaceConfig{{Name: "config.json", SkipValidate: true}}}
	s := newTestSyncer(t, cfg, remote)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"k": ` {
		t.Fatalf("期望原样写入，得到 %s", data)
	}
}
