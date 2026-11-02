package gconf

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestInit(t *testing.T) {
	// 重置全局实例
	defaultInstance = nil

	err := Init(
		WithConfigName("test"),
		WithConfigType("yaml"),
	)
	if err != nil {
		t.Logf("初始化全局实例时出错: %v", err)
	}

	instance := GetInstance()
	if instance == nil {
		t.Fatal("全局实例不应为 nil")
	}
}

// TestGetInstancePanicsBeforeInit 验证未调用 Init 时 GetInstance 与包级函数 panic，且不会创建全局实例
func TestGetInstancePanicsBeforeInit(t *testing.T) {
	defaultInstance = nil

	for name, call := range map[string]func(){
		"GetInstance": func() { GetInstance() },
		"GetString":   func() { GetString("app.name") },
	} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("%s 在 Init 之前调用应 panic", name)
				}
				if msg := fmt.Sprint(r); !strings.Contains(msg, "全局实例未初始化") {
					t.Errorf("%s panic 信息应说明未初始化，得到: %s", name, msg)
				}
			}()
			call()
		}()
		if defaultInstance != nil {
			t.Fatalf("%s 不应创建全局实例", name)
		}
	}
}

// TestInitOnlyOnce 验证 Init 只能成功一次，再次调用返回 ErrAlreadyInitialized 且全局实例不变
func TestInitOnlyOnce(t *testing.T) {
	dir, _ := writeVersionConfig(t, 1)
	defer os.RemoveAll(dir)
	defaultInstance = nil
	defer func() { defaultInstance = nil }()

	if err := Init(WithConfigPaths(dir)); err != nil {
		t.Fatal(err)
	}
	first := GetInstance()

	other, err := os.MkdirTemp("", "gconf-global")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(other)
	if err := os.WriteFile(filepath.Join(other, "config.yaml"), []byte("app:\n  version: 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for name, init := range map[string]func(...Option) error{"Init": Init, "InitWithConfig": InitWithConfig} {
		if err := init(WithConfigPaths(other)); !errors.Is(err, ErrAlreadyInitialized) {
			t.Errorf("再次调用 %s 应返回 ErrAlreadyInitialized，得到: %v", name, err)
		}
	}
	if GetInstance() != first {
		t.Error("再次调用 Init 不应替换全局实例")
	}
	if v := GetInt("app.version"); v != 1 {
		t.Errorf("全局配置应保持首次 Init 的值：期望 1，得到 %d", v)
	}
}

// TestInitRetryAfterFailure 验证 Init 失败时不设置全局实例，修正后可再次 Init
func TestInitRetryAfterFailure(t *testing.T) {
	dir, err := os.MkdirTemp("", "gconf-global")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(file, []byte("app: [1, 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	defaultInstance = nil
	defer func() { defaultInstance = nil }()

	if err := Init(WithConfigPaths(dir)); err == nil {
		t.Fatal("配置文件解析失败时 Init 应返回错误")
	}
	if defaultInstance != nil {
		t.Fatal("Init 失败时不应设置全局实例")
	}

	if err := os.WriteFile(file, []byte("app:\n  version: 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Init(WithConfigPaths(dir)); err != nil {
		t.Fatalf("修正配置后再次 Init 应成功: %v", err)
	}
	if v := GetInt("app.version"); v != 1 {
		t.Errorf("期望 1，得到 %d", v)
	}
}

// TestInitConcurrent 验证并发调用 Init 只有一次成功，其余返回 ErrAlreadyInitialized
func TestInitConcurrent(t *testing.T) {
	dir, _ := writeVersionConfig(t, 1)
	defer os.RemoveAll(dir)
	defaultInstance = nil
	defer func() { defaultInstance = nil }()

	const n = 20
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = Init(WithConfigPaths(dir))
		}(i)
	}
	wg.Wait()

	succeeded := 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case !errors.Is(err, ErrAlreadyInitialized):
			t.Errorf("并发 Init 返回了非预期错误: %v", err)
		}
	}
	if succeeded != 1 {
		t.Errorf("并发 Init 应恰好成功一次，实际 %d 次", succeeded)
	}
}

func TestGlobalFunctions(t *testing.T) {
	// 重置全局实例
	defaultInstance = nil

	_ = Init()

	// 测试全局函数
	SetDefault("global.test", "default_value")
	Set("global.test", "new_value")

	if v := GetString("global.test"); v != "new_value" {
		t.Errorf("全局 Get 失败: 期望 'new_value'，得到 '%s'", v)
	}

	if !IsSet("global.test") {
		t.Error("全局 IsSet 失败")
	}

	Set("global.number", 100)
	if v := GetInt("global.number"); v != 100 {
		t.Errorf("全局 GetInt 失败: 期望 100，得到 %d", v)
	}

	Set("global.flag", true)
	if !GetBool("global.flag") {
		t.Error("全局 GetBool 失败")
	}
}

func TestGlobalUnmarshal(t *testing.T) {
	// 重置全局实例
	defaultInstance = nil

	_ = Init()

	Set("app.name", "GlobalApp")
	Set("app.port", 8080)

	type AppConfig struct {
		Name string `mapstructure:"name"`
		Port int    `mapstructure:"port"`
	}

	var config AppConfig
	err := UnmarshalKey("app", &config)
	if err != nil {
		t.Fatalf("全局 UnmarshalKey 失败: %v", err)
	}

	if config.Name != "GlobalApp" {
		t.Errorf("Name: 期望 'GlobalApp'，得到 '%s'", config.Name)
	}
	if config.Port != 8080 {
		t.Errorf("Port: 期望 8080，得到 %d", config.Port)
	}
}

func TestGlobalWithEnv(t *testing.T) {
	// 重置全局实例
	defaultInstance = nil

	os.Setenv("GLOBAL_TEST_KEY", "from_env")
	defer os.Unsetenv("GLOBAL_TEST_KEY")

	_ = Init(
		WithAutomaticEnv(true),
		WithEnvPrefix("GLOBAL"),
		WithEnvKeyReplacer(".", "_"),
	)

	SetDefault("test.key", "default")

	if v := GetString("test.key"); v != "from_env" {
		t.Errorf("全局环境变量读取失败: 期望 'from_env'，得到 '%s'", v)
	}
}

func TestAllGlobalGetters(t *testing.T) {
	// 重置全局实例
	defaultInstance = nil

	_ = Init()

	// 测试所有类型的 getter
	Set("test.string", "hello")
	Set("test.int", 123)
	Set("test.int32", int32(32))
	Set("test.int64", int64(64))
	Set("test.uint", uint(456))
	Set("test.uint32", uint32(32))
	Set("test.uint64", uint64(64))
	Set("test.float", 3.14)
	Set("test.slice", []string{"a", "b"})
	Set("test.map", map[string]string{"key": "value"})

	// GetString
	if v := GetString("test.string"); v != "hello" {
		t.Errorf("GetString: 期望 'hello'，得到 '%s'", v)
	}

	// GetInt
	if v := GetInt("test.int"); v != 123 {
		t.Errorf("GetInt: 期望 123，得到 %d", v)
	}

	// GetInt32
	if v := GetInt32("test.int32"); v != 32 {
		t.Errorf("GetInt32: 期望 32，得到 %d", v)
	}

	// GetInt64
	if v := GetInt64("test.int64"); v != 64 {
		t.Errorf("GetInt64: 期望 64，得到 %d", v)
	}

	// GetUint
	if v := GetUint("test.uint"); v != 456 {
		t.Errorf("GetUint: 期望 456，得到 %d", v)
	}

	// GetUint32
	if v := GetUint32("test.uint32"); v != 32 {
		t.Errorf("GetUint32: 期望 32，得到 %d", v)
	}

	// GetUint64
	if v := GetUint64("test.uint64"); v != 64 {
		t.Errorf("GetUint64: 期望 64，得到 %d", v)
	}

	// GetFloat64
	if v := GetFloat64("test.float"); v != 3.14 {
		t.Errorf("GetFloat64: 期望 3.14，得到 %f", v)
	}

	// GetStringSlice
	if v := GetStringSlice("test.slice"); len(v) != 2 {
		t.Errorf("GetStringSlice: 期望长度 2，得到 %d", len(v))
	}

	// GetStringMapString
	if v := GetStringMapString("test.map"); v["key"] != "value" {
		t.Errorf("GetStringMapString: 期望 'value'，得到 '%s'", v["key"])
	}

	// AllKeys
	keys := AllKeys()
	if len(keys) == 0 {
		t.Error("AllKeys 应该返回非空切片")
	}

	// AllSettings
	settings := AllSettings()
	if len(settings) == 0 {
		t.Error("AllSettings 应该返回非空映射")
	}
}

func TestInitWithConfig(t *testing.T) {
	// 重置全局实例
	defaultInstance = nil

	err := InitWithConfig(WithConfigName("test2"))
	if err != nil {
		t.Logf("初始化时出错: %v", err)
	}

	instance := GetInstance()
	if instance == nil {
		t.Fatal("初始化后实例不应为 nil")
	}
}
