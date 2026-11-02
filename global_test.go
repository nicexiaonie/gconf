package gconf

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

func TestGetInstance(t *testing.T) {
	// 重置全局实例
	defaultInstance = nil

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

// TestGetInstanceWarnsOnAutoInit 验证未调用 Init 时自动初始化只输出一次警告，并注明使用的配置文件或未找到配置文件；调用 Init 不输出该警告
func TestGetInstanceWarnsOnAutoInit(t *testing.T) {
	var buf strings.Builder
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	defer func() {
		if defaultInstance != nil {
			defaultInstance.Close()
		}
		defaultInstance = nil
	}()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	const warning = "未调用 Init，已使用默认选项自动初始化"

	empty, err := os.MkdirTemp("", "gconf-global")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(empty)
	if err := os.Chdir(empty); err != nil {
		t.Fatal(err)
	}
	defaultInstance = nil
	GetInstance()
	GetInstance()
	if got := strings.Count(buf.String(), warning); got != 1 {
		t.Errorf("自动初始化警告应只输出一次，实际 %d 次: %s", got, buf.String())
	}
	if !strings.Contains(buf.String(), "未找到配置文件") {
		t.Errorf("未找到配置文件时警告应注明，得到: %s", buf.String())
	}

	dir, file := writeVersionConfig(t, 1)
	defer os.RemoveAll(dir)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	defaultInstance = nil
	GetInstance()
	if !strings.Contains(buf.String(), warning) || !strings.Contains(buf.String(), filepath.Base(file)) {
		t.Errorf("找到配置文件时警告应注明文件路径，得到: %s", buf.String())
	}

	buf.Reset()
	defaultInstance = nil
	if err := Init(); err != nil {
		t.Fatal(err)
	}
	GetInstance()
	if strings.Contains(buf.String(), warning) {
		t.Errorf("调用 Init 后不应输出自动初始化警告，得到: %s", buf.String())
	}
}

// TestGetInstancePanicsOnInvalidConfig 验证未调用 Init 且默认配置文件解析失败时，GetInstance 带原始错误 panic
func TestGetInstancePanicsOnInvalidConfig(t *testing.T) {
	dir, err := os.MkdirTemp("", "gconf-global")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("app: [1, 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	defaultInstance = nil
	defer func() { defaultInstance = nil }()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("期望 GetInstance panic")
		}
		if msg := fmt.Sprint(r); !strings.Contains(msg, "While parsing config") {
			t.Errorf("panic 信息应包含解析错误，得到: %s", msg)
		}
		if defaultInstance != nil {
			t.Error("自动初始化失败时不应设置全局实例")
		}
	}()
	GetInstance()
}

// TestReInitClosesOldInstance 验证再次 Init 成功后旧实例不再热更新
func TestReInitClosesOldInstance(t *testing.T) {
	dir, file := writeVersionConfig(t, 1)
	defer os.RemoveAll(dir)
	defer func() {
		if defaultInstance != nil {
			defaultInstance.Close()
		}
		defaultInstance = nil
	}()

	opts := []Option{WithConfigPaths(dir), WithWatchConfig(true)}
	if err := Init(opts...); err != nil {
		t.Fatal(err)
	}
	old := GetInstance()
	if err := Init(opts...); err != nil {
		t.Fatal(err)
	}
	if GetInstance() == old {
		t.Fatal("再次 Init 应替换全局实例")
	}

	if err := os.WriteFile(file, []byte("app:\n  version: 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !waitFor(2*time.Second, func() bool { return GetInt("app.version") == 2 }) {
		t.Fatal("新实例热更新未生效")
	}
	// 留出旧实例若仍在监听时完成重载的时间
	time.Sleep(200 * time.Millisecond)
	if v := old.GetInt("app.version"); v != 1 {
		t.Errorf("旧实例应已关闭不再重载：期望 1，得到 %d", v)
	}
}

// TestReInitKeepsHandlers 验证再次 Init 后，之前通过 OnConfigChange 注册的回调迁移到新实例且只触发一次
func TestReInitKeepsHandlers(t *testing.T) {
	dir, file := writeVersionConfig(t, 1)
	defer os.RemoveAll(dir)
	defer func() {
		if defaultInstance != nil {
			defaultInstance.Close()
		}
		defaultInstance = nil
	}()

	opts := []Option{WithConfigPaths(dir), WithWatchConfig(true)}
	if err := Init(opts...); err != nil {
		t.Fatal(err)
	}
	var called int32
	OnConfigChange(func(e ChangeEvent) {
		if e.Err == nil {
			atomic.AddInt32(&called, 1)
		}
	})
	if err := Init(opts...); err != nil {
		t.Fatal(err)
	}

	// 追加写入只产生一个写事件，便于断言回调恰好触发一次
	appendFile(t, file, "  name: demo\n")
	if !waitFor(2*time.Second, func() bool { return atomic.LoadInt32(&called) >= 1 }) {
		t.Fatal("再次 Init 后已注册的回调未触发")
	}
	// 留出旧实例若仍在监听时重复触发的时间
	time.Sleep(200 * time.Millisecond)
	if n := atomic.LoadInt32(&called); n != 1 {
		t.Errorf("回调应只触发一次，实际 %d 次", n)
	}
}

// TestReInitSerializesCallbacks 验证再次 Init 时，旧实例正在执行的回调结束前新实例不会开始执行回调，同一回调不会被新旧实例同时执行
func TestReInitSerializesCallbacks(t *testing.T) {
	dir, file := writeVersionConfig(t, 1)
	defer os.RemoveAll(dir)
	defer func() {
		if defaultInstance != nil {
			defaultInstance.Close()
		}
		defaultInstance = nil
	}()

	opts := []Option{WithConfigPaths(dir), WithWatchConfig(true)}
	if err := Init(opts...); err != nil {
		t.Fatal(err)
	}
	var entered, running, overlapped int32
	started := make(chan struct{})
	release := make(chan struct{})
	OnConfigChange(func(ChangeEvent) {
		if atomic.AddInt32(&running, 1) > 1 {
			atomic.StoreInt32(&overlapped, 1)
		}
		defer atomic.AddInt32(&running, -1)
		if atomic.AddInt32(&entered, 1) == 1 {
			close(started)
			<-release
		}
	})

	appendFile(t, file, "  name: demo\n")
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("旧实例回调未触发")
	}
	if err := Init(opts...); err != nil {
		t.Fatal(err)
	}
	appendFile(t, file, "  env: test\n")
	// 回调被阻塞不影响新实例重载
	if !waitFor(2*time.Second, func() bool { return GetString("app.env") == "test" }) {
		t.Fatal("新实例热更新未生效")
	}
	time.Sleep(200 * time.Millisecond)
	if n := atomic.LoadInt32(&entered); n != 1 {
		t.Errorf("旧实例回调结束前新实例不应执行回调，实际进入 %d 次", n)
	}

	close(release)
	if !waitFor(2*time.Second, func() bool { return atomic.LoadInt32(&entered) >= 2 }) {
		t.Fatal("旧实例回调结束后新实例应执行回调")
	}
	if atomic.LoadInt32(&overlapped) != 0 {
		t.Error("同一回调被新旧实例同时执行")
	}
}

// TestInitInCallback 验证在回调中再次调用 Init 不会死锁，回调继承到新实例后继续生效
func TestInitInCallback(t *testing.T) {
	dir, file := writeVersionConfig(t, 1)
	defer os.RemoveAll(dir)
	defer func() {
		if defaultInstance != nil {
			defaultInstance.Close()
		}
		defaultInstance = nil
	}()

	opts := []Option{WithConfigPaths(dir), WithWatchConfig(true)}
	if err := Init(opts...); err != nil {
		t.Fatal(err)
	}
	old := GetInstance()
	var calls int32
	OnConfigChange(func(ChangeEvent) {
		if atomic.AddInt32(&calls, 1) == 1 {
			if err := Init(opts...); err != nil {
				t.Errorf("回调中 Init 失败: %v", err)
			}
		}
	})

	appendFile(t, file, "  name: demo\n")
	if !waitFor(2*time.Second, func() bool { return atomic.LoadInt32(&calls) >= 1 }) {
		t.Fatal("回调未触发")
	}
	if !waitFor(2*time.Second, func() bool { return GetInstance() != old }) {
		t.Fatal("回调中调用 Init 未完成，可能死锁")
	}
	appendFile(t, file, "  env: test\n")
	if !waitFor(2*time.Second, func() bool { return atomic.LoadInt32(&calls) >= 2 }) {
		t.Fatal("回调中 Init 后回调未在新实例上继续生效")
	}
}

// TestReInitConcurrentWrites 反复 Init 的同时持续以 rename 覆盖方式修改配置文件，
// 验证回调不会重叠执行，且最终读到最后写入的值。时序窗口无法稳定复现，属概率性验证
func TestReInitConcurrentWrites(t *testing.T) {
	dir, file := writeVersionConfig(t, 0)
	defer os.RemoveAll(dir)
	defer func() {
		if defaultInstance != nil {
			defaultInstance.Close()
		}
		defaultInstance = nil
	}()

	// 临时文件放在监听目录之外：macOS 上 fsnotify 注册目录监听时会逐个监听目录内文件，
	// 目录内文件恰好被 rename 移走会导致注册失败，与本测试验证的 Init 行为无关
	tmpDir, err := os.MkdirTemp(filepath.Dir(dir), "gconf-tmp")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)
	write := func(version int) error {
		tmp := filepath.Join(tmpDir, "config.yaml")
		if err := os.WriteFile(tmp, fmt.Appendf(nil, "app:\n  version: %d\n", version), 0644); err != nil {
			return err
		}
		return os.Rename(tmp, file)
	}
	opts := []Option{WithConfigPaths(dir), WithWatchConfig(true)}
	if err := Init(opts...); err != nil {
		t.Fatal(err)
	}
	var running, overlapped int32
	OnConfigChange(func(ChangeEvent) {
		if atomic.AddInt32(&running, 1) > 1 {
			atomic.StoreInt32(&overlapped, 1)
		}
		time.Sleep(time.Millisecond)
		atomic.AddInt32(&running, -1)
	})

	stop := make(chan struct{})
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for v := 1; ; v++ {
			select {
			case <-stop:
				return
			default:
			}
			if err := write(v); err != nil {
				t.Errorf("写入配置失败: %v", err)
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	var initErr error
	for i := 0; i < 20 && initErr == nil; i++ {
		if err := Init(opts...); err != nil {
			initErr = fmt.Errorf("第 %d 次 Init 失败: %w", i+1, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(stop)
	<-writerDone
	if initErr != nil {
		t.Fatal(initErr)
	}

	if err := write(-1); err != nil {
		t.Fatal(err)
	}
	if !waitFor(2*time.Second, func() bool { return GetInt("app.version") == -1 }) {
		t.Fatalf("未读到最后写入的值，得到 %d", GetInt("app.version"))
	}
	if atomic.LoadInt32(&overlapped) != 0 {
		t.Error("回调重叠执行")
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
