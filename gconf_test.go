package gconf

import (
	"os"
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
