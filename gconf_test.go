package gconf

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestNew(t *testing.T) {
	// 测试创建配置实例
	conf, err := New(
		WithConfigName("test"),
		WithConfigType("yaml"),
		WithConfigPaths("./test"),
	)
	if err != nil {
		t.Logf("创建配置实例时出错（配置文件可能不存在）: %v", err)
	}
	if conf == nil {
		t.Fatal("配置实例不应为 nil")
	}
}

func TestDefaultValues(t *testing.T) {
	conf, err := New()
	if err != nil {
		t.Logf("创建配置实例时出错: %v", err)
	}

	// 设置默认值
	conf.SetDefault("test.string", "hello")
	conf.SetDefault("test.int", 123)
	conf.SetDefault("test.bool", true)
	conf.SetDefault("test.float", 3.14)

	// 测试获取默认值
	if v := conf.GetString("test.string"); v != "hello" {
		t.Errorf("期望 'hello'，得到 '%s'", v)
	}
	if v := conf.GetInt("test.int"); v != 123 {
		t.Errorf("期望 123，得到 %d", v)
	}
	if v := conf.GetBool("test.bool"); !v {
		t.Error("期望 true，得到 false")
	}
	if v := conf.GetFloat64("test.float"); v != 3.14 {
		t.Errorf("期望 3.14，得到 %f", v)
	}
}

func TestSetAndGet(t *testing.T) {
	conf, _ := New()

	// 测试设置和获取各种类型
	conf.Set("key.string", "value")
	conf.Set("key.int", 42)
	conf.Set("key.bool", false)
	conf.Set("key.slice", []string{"a", "b", "c"})

	if v := conf.GetString("key.string"); v != "value" {
		t.Errorf("GetString 失败: 期望 'value'，得到 '%s'", v)
	}
	if v := conf.GetInt("key.int"); v != 42 {
		t.Errorf("GetInt 失败: 期望 42，得到 %d", v)
	}
	if v := conf.GetBool("key.bool"); v {
		t.Error("GetBool 失败: 期望 false，得到 true")
	}
	slice := conf.GetStringSlice("key.slice")
	if len(slice) != 3 || slice[0] != "a" {
		t.Errorf("GetStringSlice 失败: %v", slice)
	}
}

func TestIsSet(t *testing.T) {
	conf, _ := New()

	conf.Set("exists", "value")

	if !conf.IsSet("exists") {
		t.Error("IsSet 应该返回 true 对于存在的键")
	}
	if conf.IsSet("not.exists") {
		t.Error("IsSet 应该返回 false 对于不存在的键")
	}
}

func TestAllKeys(t *testing.T) {
	conf, _ := New()

	conf.Set("key1", "value1")
	conf.Set("key2", "value2")
	conf.Set("key3", "value3")

	keys := conf.AllKeys()
	if len(keys) < 3 {
		t.Errorf("期望至少 3 个键，得到 %d", len(keys))
	}
}

func TestUnmarshal(t *testing.T) {
	conf, _ := New()

	// 设置测试数据
	conf.Set("name", "TestApp")
	conf.Set("port", 8080)
	conf.Set("debug", true)

	// 定义结构体
	type Config struct {
		Name  string `mapstructure:"name"`
		Port  int    `mapstructure:"port"`
		Debug bool   `mapstructure:"debug"`
	}

	var config Config
	err := conf.Unmarshal(&config)
	if err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}

	if config.Name != "TestApp" {
		t.Errorf("Name: 期望 'TestApp'，得到 '%s'", config.Name)
	}
	if config.Port != 8080 {
		t.Errorf("Port: 期望 8080，得到 %d", config.Port)
	}
	if !config.Debug {
		t.Error("Debug: 期望 true，得到 false")
	}
}

func TestUnmarshalKey(t *testing.T) {
	conf, _ := New()

	// 设置嵌套配置
	conf.Set("server.host", "localhost")
	conf.Set("server.port", 9090)

	// 定义结构体
	type ServerConfig struct {
		Host string `mapstructure:"host"`
		Port int    `mapstructure:"port"`
	}

	var serverConfig ServerConfig
	err := conf.UnmarshalKey("server", &serverConfig)
	if err != nil {
		t.Fatalf("UnmarshalKey 失败: %v", err)
	}

	if serverConfig.Host != "localhost" {
		t.Errorf("Host: 期望 'localhost'，得到 '%s'", serverConfig.Host)
	}
	if serverConfig.Port != 9090 {
		t.Errorf("Port: 期望 9090，得到 %d", serverConfig.Port)
	}
}

func TestSub(t *testing.T) {
	conf, _ := New()

	conf.Set("database.host", "localhost")
	conf.Set("database.port", 3306)
	conf.Set("database.username", "root")

	subConf := conf.Sub("database")
	if subConf == nil {
		t.Fatal("Sub 返回 nil")
	}

	if v := subConf.GetString("host"); v != "localhost" {
		t.Errorf("Sub.GetString: 期望 'localhost'，得到 '%s'", v)
	}
	if v := subConf.GetInt("port"); v != 3306 {
		t.Errorf("Sub.GetInt: 期望 3306，得到 %d", v)
	}
}

func TestRegisterAlias(t *testing.T) {
	conf, _ := New()

	conf.Set("original.key", "value")
	conf.RegisterAlias("alias", "original.key")

	if v := conf.GetString("alias"); v != "value" {
		t.Errorf("别名获取失败: 期望 'value'，得到 '%s'", v)
	}
}

func TestEnvironmentVariables(t *testing.T) {
	// 设置环境变量
	os.Setenv("TEST_APP_NAME", "TestFromEnv")
	os.Setenv("TEST_APP_PORT", "8888")
	defer func() {
		os.Unsetenv("TEST_APP_NAME")
		os.Unsetenv("TEST_APP_PORT")
	}()

	conf, _ := New(
		WithAutomaticEnv(true),
		WithEnvPrefix("TEST"),
		WithEnvKeyReplacer(".", "_"),
	)

	conf.SetDefault("app.name", "default")
	conf.SetDefault("app.port", 8080)

	// 环境变量应该覆盖默认值
	if v := conf.GetString("app.name"); v != "TestFromEnv" {
		t.Errorf("环境变量读取失败: 期望 'TestFromEnv'，得到 '%s'", v)
	}
	if v := conf.GetInt("app.port"); v != 8888 {
		t.Errorf("环境变量读取失败: 期望 8888，得到 %d", v)
	}
}

func TestBindEnv(t *testing.T) {
	os.Setenv("CUSTOM_VAR", "custom_value")
	defer os.Unsetenv("CUSTOM_VAR")

	conf, _ := New()
	conf.BindEnv("my.custom.key", "CUSTOM_VAR")

	if v := conf.GetString("my.custom.key"); v != "custom_value" {
		t.Errorf("BindEnv 失败: 期望 'custom_value'，得到 '%s'", v)
	}
}

func TestDuration(t *testing.T) {
	conf, _ := New()

	conf.Set("timeout", "30s")

	duration := conf.GetDuration("timeout")
	if duration != 30*time.Second {
		t.Errorf("GetDuration 失败: 期望 30s，得到 %v", duration)
	}
}

func TestAllSettings(t *testing.T) {
	conf, _ := New()

	conf.Set("key1", "value1")
	conf.Set("key2", 123)

	settings := conf.AllSettings()
	if len(settings) < 2 {
		t.Errorf("AllSettings: 期望至少 2 个设置，得到 %d", len(settings))
	}
}

func TestOnConfigChange(t *testing.T) {
	conf, _ := New()

	conf.OnConfigChange(func(e fsnotify.Event) {
		// 回调函数
	})

	// 注意：这只是测试回调注册，实际的文件监听需要真实的文件系统
	if len(conf.onChangeHandlers) != 1 {
		t.Error("OnConfigChange 回调未正确注册")
	}
}

func TestOptions(t *testing.T) {
	// 测试各种选项
	conf, err := New(
		WithConfigName("custom"),
		WithConfigType("json"),
		WithConfigPaths("/custom/path"),
		WithWatchConfig(false),
		WithAutomaticEnv(false),
		WithEnvPrefix("CUSTOM"),
		WithDebug(false),
	)

	if err != nil {
		t.Logf("创建配置实例时出错: %v", err)
	}
	if conf == nil {
		t.Fatal("配置实例不应为 nil")
	}
}

// TestConcurrentReadDuringReload 验证热更新重载与并发读写之间没有数据竞争（需配合 -race 运行）
func TestConcurrentReadDuringReload(t *testing.T) {
	dir, err := os.MkdirTemp("", "gconf-race")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	file := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(file, []byte("app:\n  version: 0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	conf, err := New(
		WithConfigPaths(dir),
		WithConfigName("config"),
		WithConfigType("yaml"),
		WithWatchConfig(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	worker := func(fn func(i int)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
					fn(i)
				}
			}
		}()
	}
	worker(func(int) { conf.GetInt("app.version") })
	worker(func(int) {
		var c struct {
			App struct{ Version int }
		}
		_ = conf.Unmarshal(&c)
	})
	worker(func(i int) { conf.Set("runtime.counter", i) })

	const last = 20
	for i := 1; i <= last; i++ {
		content := fmt.Sprintf("app:\n  version: %d\n", i)
		if err := os.WriteFile(file, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	deadline := time.Now().Add(2 * time.Second)
	for conf.GetInt("app.version") != last && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	close(stop)
	wg.Wait()

	if v := conf.GetInt("app.version"); v != last {
		t.Errorf("热更新未生效：期望 %d，得到 %d", last, v)
	}
}

// TestConfigTypeInferredFromExt 验证未指定 ConfigType 时按扩展名选择解析器
func TestConfigTypeInferredFromExt(t *testing.T) {
	cases := []struct {
		ext     string
		content string
	}{
		{"toml", "[server]\nport = 8080\nname = \"a/b\"\n"},
		// JSON 合法的 \/ 转义只有 JSON 解析器能处理，YAML 解析器会报错
		{"json", `{"server": {"port": 8080, "name": "a` + "\x5c" + `/b"}}`},
		{"yaml", "server:\n  port: 8080\n  name: a/b\n"},
	}
	for _, c := range cases {
		t.Run(c.ext, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "gconf-ext")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)

			if err := os.WriteFile(filepath.Join(dir, "config."+c.ext), []byte(c.content), 0644); err != nil {
				t.Fatal(err)
			}

			conf, err := New(WithConfigPaths(dir))
			if err != nil {
				t.Fatal(err)
			}
			if v := conf.GetInt("server.port"); v != 8080 {
				t.Errorf("期望 8080，得到 %d", v)
			}
			if v := conf.GetString("server.name"); v != "a/b" {
				t.Errorf("期望 'a/b'，得到 '%s'", v)
			}
		})
	}
}

// TestExplicitConfigTypeOverridesExt 验证显式指定 ConfigType 时优先于扩展名
func TestExplicitConfigTypeOverridesExt(t *testing.T) {
	dir, err := os.MkdirTemp("", "gconf-ext")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// 扩展名为 toml，内容为 YAML，只有按 yaml 解析才能成功
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("server:\n  port: 8080\n"), 0644); err != nil {
		t.Fatal(err)
	}

	conf, err := New(WithConfigPaths(dir), WithConfigType("yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if v := conf.GetInt("server.port"); v != 8080 {
		t.Errorf("期望 8080，得到 %d", v)
	}
}

type TagBase struct {
	AppName string `json:"app_name" toml:"app_name"`
}

type tagDB struct {
	MaxConn int           `json:"max_conn" toml:"max_conn"`
	Timeout time.Duration `json:"timeout" toml:"timeout"`
}

type tagConf struct {
	TagBase
	ListenAddr string `json:"listen_addr,omitempty" toml:"listen_addr,omitempty"`
	DB         tagDB  `json:"db_conf" toml:"db_conf"`
	Ignored    string `json:"-" toml:"-"`
}

func writeTagConfig(t *testing.T, ext, content string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "gconf-tag")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config."+ext), []byte(content), 0644); err != nil {
		os.RemoveAll(dir)
		t.Fatal(err)
	}
	return dir
}

// TestUnmarshalWithTagName 验证 WithTagName 后按 json/toml 标签匹配字段，并像原生库一样展开匿名嵌入结构体
func TestUnmarshalWithTagName(t *testing.T) {
	cases := []struct {
		tag     string
		content string
	}{
		{"json", `{"app_name": "demo", "listen_addr": "0.0.0.0:8080", "db_conf": {"max_conn": 10, "timeout": "5s"}}`},
		{"toml", "app_name = \"demo\"\nlisten_addr = \"0.0.0.0:8080\"\n[db_conf]\nmax_conn = 10\ntimeout = \"5s\"\n"},
	}
	for _, c := range cases {
		t.Run(c.tag, func(t *testing.T) {
			dir := writeTagConfig(t, c.tag, c.content)
			defer os.RemoveAll(dir)

			conf, err := New(WithConfigPaths(dir), WithTagName(c.tag))
			if err != nil {
				t.Fatal(err)
			}
			want := tagConf{
				TagBase:    TagBase{AppName: "demo"},
				ListenAddr: "0.0.0.0:8080",
				DB:         tagDB{MaxConn: 10, Timeout: 5 * time.Second},
			}

			var got tagConf
			if err := conf.Unmarshal(&got); err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Errorf("Unmarshal 期望 %+v，得到 %+v", want, got)
			}

			var exact tagConf
			if err := conf.UnmarshalExact(&exact); err != nil {
				t.Fatal(err)
			}
			if exact != want {
				t.Errorf("UnmarshalExact 期望 %+v，得到 %+v", want, exact)
			}

			var db tagDB
			if err := conf.UnmarshalKey("db_conf", &db); err != nil {
				t.Fatal(err)
			}
			if db != want.DB {
				t.Errorf("UnmarshalKey 期望 %+v，得到 %+v", want.DB, db)
			}
		})
	}
}

// TestUnmarshalWithTagNameEnvOverride 验证指定标签名后仍保留 viper 的环境变量覆盖与弱类型转换
func TestUnmarshalWithTagNameEnvOverride(t *testing.T) {
	dir := writeTagConfig(t, "json", `{"db_conf": {"max_conn": 10}}`)
	defer os.RemoveAll(dir)
	os.Setenv("GCONFTAG_DB_CONF_MAX_CONN", "20")
	defer os.Unsetenv("GCONFTAG_DB_CONF_MAX_CONN")

	conf, err := New(
		WithConfigPaths(dir),
		WithTagName("json"),
		WithAutomaticEnv(true),
		WithEnvPrefix("GCONFTAG"),
		WithEnvKeyReplacer(".", "_"),
	)
	if err != nil {
		t.Fatal(err)
	}
	var got tagConf
	if err := conf.Unmarshal(&got); err != nil {
		t.Fatal(err)
	}
	if got.DB.MaxConn != 20 {
		t.Errorf("期望环境变量覆盖为 20，得到 %d", got.DB.MaxConn)
	}
}

// TestSubKeepsTagName 验证 Sub 返回的子配置沿用父实例的标签名
func TestSubKeepsTagName(t *testing.T) {
	dir := writeTagConfig(t, "json", `{"db_conf": {"max_conn": 10, "timeout": "5s"}}`)
	defer os.RemoveAll(dir)

	conf, err := New(WithConfigPaths(dir), WithTagName("json"))
	if err != nil {
		t.Fatal(err)
	}
	sub := conf.Sub("db_conf")
	if sub == nil {
		t.Fatal("Sub 返回 nil")
	}
	var got tagDB
	if err := sub.Unmarshal(&got); err != nil {
		t.Fatal(err)
	}
	if want := (tagDB{MaxConn: 10, Timeout: 5 * time.Second}); got != want {
		t.Errorf("Sub.Unmarshal 期望 %+v，得到 %+v", want, got)
	}
}

// TestUnmarshalDefaultTagName 验证未指定标签名时仍只认 mapstructure 标签，行为不变
func TestUnmarshalDefaultTagName(t *testing.T) {
	dir := writeTagConfig(t, "json", `{"listen_addr": "0.0.0.0:8080"}`)
	defer os.RemoveAll(dir)

	conf, err := New(WithConfigPaths(dir))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		JSONTagged      string `json:"listen_addr"`
		MapstructTagged string `mapstructure:"listen_addr"`
	}
	if err := conf.Unmarshal(&got); err != nil {
		t.Fatal(err)
	}
	if got.JSONTagged != "" || got.MapstructTagged != "0.0.0.0:8080" {
		t.Errorf("默认应只认 mapstructure 标签，得到 %+v", got)
	}
}

func TestGetViper(t *testing.T) {
	conf, _ := New()

	viper := conf.GetViper()
	if viper == nil {
		t.Error("GetViper 应该返回非 nil 的 viper 实例")
	}
}

// Benchmark 测试
func BenchmarkGetString(b *testing.B) {
	conf, _ := New()
	conf.Set("test.key", "test.value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conf.GetString("test.key")
	}
}

func BenchmarkGetInt(b *testing.B) {
	conf, _ := New()
	conf.Set("test.number", 12345)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conf.GetInt("test.number")
	}
}

func BenchmarkSet(b *testing.B) {
	conf, _ := New()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conf.Set("test.key", i)
	}
}
