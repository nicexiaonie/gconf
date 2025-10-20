package gconf

import (
	"os"
	"sync"
	"testing"
)

func TestInit(t *testing.T) {
	// 重置全局实例
	defaultInstance = nil
	defaultInstanceOnce = sync.Once{}

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

func TestGetInstance(t *testing.T) {
	// 重置全局实例
	defaultInstance = nil
	defaultInstanceOnce = sync.Once{}

	instance := GetInstance()
	if instance == nil {
		t.Fatal("GetInstance 应该返回非 nil 实例")
	}

	// 第二次调用应该返回相同实例
	instance2 := GetInstance()
	if instance != instance2 {
		t.Error("GetInstance 应该返回相同的单例")
	}
}

func TestGlobalFunctions(t *testing.T) {
	// 重置全局实例
	defaultInstance = nil
	defaultInstanceOnce = sync.Once{}

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
	defaultInstanceOnce = sync.Once{}

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
	defaultInstanceOnce = sync.Once{}

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
	defaultInstanceOnce = sync.Once{}

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
	defaultInstanceOnce = sync.Once{}

	// 第一次初始化
	err := Init(WithConfigName("test1"))
	if err != nil {
		t.Logf("第一次初始化时出错: %v", err)
	}

	// 使用 InitWithConfig 重新初始化
	err = InitWithConfig(WithConfigName("test2"))
	if err != nil {
		t.Logf("重新初始化时出错: %v", err)
	}

	instance := GetInstance()
	if instance == nil {
		t.Fatal("重新初始化后实例不应为 nil")
	}
}
