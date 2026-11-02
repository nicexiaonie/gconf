package gconf

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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

	conf.OnConfigChange(func(e ChangeEvent) {
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

// waitFor 在 timeout 内轮询 cond，满足时返回 true
func waitFor(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
	return true
}

// writeVersionConfig 在临时目录写入 config.yaml，返回目录与文件路径
func writeVersionConfig(t *testing.T, version int) (string, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "gconf-watch")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(file, []byte(fmt.Sprintf("app:\n  version: %d\n", version)), 0644); err != nil {
		os.RemoveAll(dir)
		t.Fatal(err)
	}
	return dir, file
}

// TestCloseStopsReload 验证 Close 后不再重载配置、不再触发回调，且仍可读取最后一次加载的配置
func TestCloseStopsReload(t *testing.T) {
	dir, file := writeVersionConfig(t, 1)
	defer os.RemoveAll(dir)

	var called int32
	conf, err := New(
		WithConfigPaths(dir),
		WithWatchConfig(true),
		WithOnConfigChange(func(ChangeEvent) { atomic.AddInt32(&called, 1) }),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Close 前热更新正常生效
	if err := os.WriteFile(file, []byte("app:\n  version: 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !waitFor(2*time.Second, func() bool { return conf.GetInt("app.version") == 2 }) {
		t.Fatal("Close 前热更新未生效")
	}

	if err := conf.Close(); err != nil {
		t.Fatal(err)
	}
	before := atomic.LoadInt32(&called)
	if err := os.WriteFile(file, []byte("app:\n  version: 3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)

	if v := conf.GetInt("app.version"); v != 2 {
		t.Errorf("Close 后不应重载：期望 2，得到 %d", v)
	}
	if after := atomic.LoadInt32(&called); after != before {
		t.Errorf("Close 后不应触发回调：调用次数由 %d 变为 %d", before, after)
	}
}

// appendFile 以单次写入向文件末尾追加内容，不截断文件，每次只产生一个写事件
func appendFile(t *testing.T, file, content string) {
	t.Helper()
	f, err := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
}

// TestReloadCallbackReportsResult 验证重载失败与成功都执行回调：失败时 Err 非 nil 且保留旧配置，成功时 Err 为 nil 且读到新配置
func TestReloadCallbackReportsResult(t *testing.T) {
	dir, file := writeVersionConfig(t, 1)
	defer os.RemoveAll(dir)

	type result struct {
		event   ChangeEvent
		version int
	}
	optionEvents := make(chan result, 10)
	handlerEvents := make(chan result, 10)
	// 监听 goroutine 在 New 返回前已启动，经原子指针读取实例，避免与赋值产生数据竞争
	var instance atomic.Pointer[Gconf]
	conf, err := New(
		WithConfigPaths(dir),
		WithWatchConfig(true),
		WithOnConfigChange(func(e ChangeEvent) { optionEvents <- result{e, instance.Load().GetInt("app.version")} }),
	)
	if err != nil {
		t.Fatal(err)
	}
	instance.Store(conf)
	defer conf.Close()
	conf.OnConfigChange(func(e ChangeEvent) { handlerEvents <- result{e, conf.GetInt("app.version")} })

	// receive 接收回调结果直到 done 满足；覆盖写入时截断与写入可能各触发一次重载，中间结果跳过
	receive := func(ch chan result, name string, done func(result) bool) result {
		t.Helper()
		timeout := time.After(2 * time.Second)
		for {
			select {
			case r := <-ch:
				if done(r) {
					return r
				}
			case <-timeout:
				t.Fatalf("%s 未执行", name)
				return result{}
			}
		}
	}
	first := func(result) bool { return true }
	reloaded := func(r result) bool { return r.event.Err == nil && r.version == 2 }
	wantFile, _ := filepath.EvalSymlinks(file)
	check := func(r result, name string, wantErr bool, wantVersion int) {
		t.Helper()
		if gotFile, _ := filepath.EvalSymlinks(r.event.File); gotFile != wantFile {
			t.Errorf("%s: File 期望 %s，得到 %s", name, wantFile, r.event.File)
		}
		if (r.event.Err != nil) != wantErr {
			t.Errorf("%s: Err 期望非 nil=%v，得到 %v", name, wantErr, r.event.Err)
		}
		if r.version != wantVersion {
			t.Errorf("%s: 回调中读到 version 期望 %d，得到 %d", name, wantVersion, r.version)
		}
	}

	// 追加使 YAML 非法，不截断文件，确保失败前不会出现读到空文件的中间重载
	appendFile(t, file, "app: [\n")
	check(receive(optionEvents, "失败时 WithOnConfigChange 回调", first), "失败时 WithOnConfigChange 回调", true, 1)
	check(receive(handlerEvents, "失败时 OnConfigChange 回调", first), "失败时 OnConfigChange 回调", true, 1)

	if err := os.WriteFile(file, []byte("app:\n  version: 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	check(receive(optionEvents, "成功时 WithOnConfigChange 回调", reloaded), "成功时 WithOnConfigChange 回调", false, 2)
	check(receive(handlerEvents, "成功时 OnConfigChange 回调", reloaded), "成功时 OnConfigChange 回调", false, 2)
}

// TestCallbacksRunInOrder 验证回调按 WithOnConfigChange、注册顺序同步执行，前一个回调返回后才执行下一个
func TestCallbacksRunInOrder(t *testing.T) {
	dir, file := writeVersionConfig(t, 1)
	defer os.RemoveAll(dir)

	var mu sync.Mutex
	var order []string
	record := func(name string) {
		mu.Lock()
		order = append(order, name)
		mu.Unlock()
	}
	conf, err := New(
		WithConfigPaths(dir),
		WithWatchConfig(true),
		WithOnConfigChange(func(ChangeEvent) { record("option") }),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conf.Close()
	conf.OnConfigChange(func(ChangeEvent) {
		// 若回调并发执行，后注册的回调会先记录
		time.Sleep(100 * time.Millisecond)
		record("first")
	})
	conf.OnConfigChange(func(ChangeEvent) { record("second") })

	appendFile(t, file, "  name: demo\n")
	if !waitFor(2*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(order) >= 3
	}) {
		t.Fatal("回调未全部执行")
	}
	mu.Lock()
	defer mu.Unlock()
	if got := fmt.Sprint(order[:3]); got != "[option first second]" {
		t.Errorf("回调执行顺序期望 [option first second]，得到 %s", got)
	}
}

// eventRecorder 并发安全地记录回调收到的事件
type eventRecorder struct {
	mu     sync.Mutex
	events []ChangeEvent
}

func (r *eventRecorder) record(e ChangeEvent) {
	r.mu.Lock()
	r.events = append(r.events, e)
	r.mu.Unlock()
}

func (r *eventRecorder) snapshot() []ChangeEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ChangeEvent(nil), r.events...)
}

// TestReloadAfterRenameReplace 验证以临时文件 rename 覆盖配置文件后重载，之后的修改仍能重载，且不产生错误事件
func TestReloadAfterRenameReplace(t *testing.T) {
	dir, file := writeVersionConfig(t, 1)
	defer os.RemoveAll(dir)

	conf, err := New(WithConfigPaths(dir), WithWatchConfig(true))
	if err != nil {
		t.Fatal(err)
	}
	defer conf.Close()
	var recorder eventRecorder
	conf.OnConfigChange(recorder.record)

	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, []byte("app:\n  version: 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, file); err != nil {
		t.Fatal(err)
	}
	if !waitFor(2*time.Second, func() bool { return conf.GetInt("app.version") == 2 }) {
		t.Fatal("rename 覆盖后未重载")
	}

	if err := os.WriteFile(file, []byte("app:\n  version: 3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !waitFor(2*time.Second, func() bool { return conf.GetInt("app.version") == 3 }) {
		t.Fatal("rename 覆盖后再次修改未重载，监听已停止")
	}
	for _, e := range recorder.snapshot() {
		if e.Err != nil {
			t.Errorf("rename 覆盖不应产生错误事件: %v", e.Err)
		}
	}
}

// TestDeleteNotifiesAndRecovers 验证配置文件被删除时回调收到错误且保留旧配置，文件重新创建后恢复重载
func TestDeleteNotifiesAndRecovers(t *testing.T) {
	dir, file := writeVersionConfig(t, 1)
	defer os.RemoveAll(dir)

	conf, err := New(WithConfigPaths(dir), WithWatchConfig(true))
	if err != nil {
		t.Fatal(err)
	}
	defer conf.Close()
	var recorder eventRecorder
	conf.OnConfigChange(recorder.record)

	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if !waitFor(2*time.Second, func() bool {
		for _, e := range recorder.snapshot() {
			if errors.Is(e.Err, os.ErrNotExist) {
				return true
			}
		}
		return false
	}) {
		t.Fatal("删除配置文件后回调未收到错误事件")
	}
	if v := conf.GetInt("app.version"); v != 1 {
		t.Errorf("删除后应保留旧配置：期望 1，得到 %d", v)
	}

	if err := os.WriteFile(file, []byte("app:\n  version: 2\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !waitFor(2*time.Second, func() bool { return conf.GetInt("app.version") == 2 }) {
		t.Fatal("重新创建配置文件后未恢复重载")
	}
}

// TestHandleWatchError 验证监听出错时回调收到错误；事件队列溢出时额外重载一次，其他错误不重载
func TestHandleWatchError(t *testing.T) {
	dir, file := writeVersionConfig(t, 1)
	defer os.RemoveAll(dir)

	// 不启用监听，直接调用错误处理，避免文件事件干扰
	conf, err := New(WithConfigPaths(dir))
	if err != nil {
		t.Fatal(err)
	}
	var recorder eventRecorder
	conf.OnConfigChange(recorder.record)
	if err := os.WriteFile(file, []byte("app:\n  version: 2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	other := errors.New("watch failed")
	conf.handleWatchError(other, &Options{})
	if events := recorder.snapshot(); len(events) != 1 || !errors.Is(events[0].Err, other) {
		t.Fatalf("其他错误应只产生一个携带原错误的事件，得到 %v", events)
	}
	if v := conf.GetInt("app.version"); v != 1 {
		t.Errorf("其他错误不应重载：期望 1，得到 %d", v)
	}

	conf.handleWatchError(fsnotify.ErrEventOverflow, &Options{})
	events := recorder.snapshot()
	if len(events) != 3 || !errors.Is(events[1].Err, fsnotify.ErrEventOverflow) || events[2].Err != nil {
		t.Fatalf("溢出应先产生错误事件再产生重载成功事件，得到 %v", events)
	}
	if v := conf.GetInt("app.version"); v != 2 {
		t.Errorf("溢出后应重载：期望 2，得到 %d", v)
	}
}

// TestCloseIdempotent 验证 Close 可重复调用，未启用监听的实例与子配置调用 Close 返回 nil
func TestCloseIdempotent(t *testing.T) {
	dir, _ := writeVersionConfig(t, 1)
	defer os.RemoveAll(dir)

	conf, err := New(WithConfigPaths(dir), WithWatchConfig(true))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := conf.Close(); err != nil {
			t.Errorf("第 %d 次 Close 返回错误: %v", i+1, err)
		}
	}

	plain, err := New(WithConfigPaths(dir))
	if err != nil {
		t.Fatal(err)
	}
	if err := plain.Close(); err != nil {
		t.Errorf("未启用监听的实例 Close 返回错误: %v", err)
	}
	if err := plain.Sub("app").Close(); err != nil {
		t.Errorf("子配置 Close 返回错误: %v", err)
	}
}

// TestReloadOnSymlinkSwitch 验证以临时 symlink + rename 切换配置文件指向时触发重载（Apollo 同步的发布方式）
func TestReloadOnSymlinkSwitch(t *testing.T) {
	dir, err := os.MkdirTemp("", "gconf-symlink")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	snapDir := filepath.Join(dir, "snapshots")
	if err := os.Mkdir(snapDir, 0755); err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(dir, "config.yaml")
	publish := func(version int) {
		name := fmt.Sprintf("config-%d.yaml", version)
		content := fmt.Sprintf("app:\n  version: %d\n", version)
		if err := os.WriteFile(filepath.Join(snapDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		tmp := stable + ".tmp"
		if err := os.Symlink(filepath.Join("snapshots", name), tmp); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(tmp, stable); err != nil {
			t.Fatal(err)
		}
	}
	publish(1)

	conf, err := New(WithConfigPaths(dir), WithWatchConfig(true))
	if err != nil {
		t.Fatal(err)
	}
	defer conf.Close()
	if v := conf.GetInt("app.version"); v != 1 {
		t.Fatalf("初始加载：期望 1，得到 %d", v)
	}

	publish(2)
	if !waitFor(2*time.Second, func() bool { return conf.GetInt("app.version") == 2 }) {
		t.Errorf("symlink 切换后未重载：得到 %d", conf.GetInt("app.version"))
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
		// properties、ini 不在 viper 核心内置格式中，由 gconf 注册解析器
		{"properties", "server.port=8080\nserver.name=a/b\n"},
		{"props", "server.port=8080\nserver.name=a/b\n"},
		{"prop", "server.port=8080\nserver.name=a/b\n"},
		{"ini", "[server]\nport=8080\nname=a/b\n"},
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

// TestNonCoreFormats 验证 viper 核心未内置的 HCL、properties、INI 读取语义不变，且可写回文件
func TestNonCoreFormats(t *testing.T) {
	dir, err := os.MkdirTemp("", "gconf-fmt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// HCL 块结构解析为列表，tfvars 同样按 HCL 解析
	for _, ext := range []string{"hcl", "tfvars"} {
		sub := filepath.Join(dir, ext)
		if err := os.MkdirAll(sub, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "config."+ext), []byte("name = \"demo\"\nserver { port = 8080 }\n"), 0644); err != nil {
			t.Fatal(err)
		}
		conf, err := New(WithConfigPaths(sub))
		if err != nil {
			t.Fatalf("%s: %v", ext, err)
		}
		if v := conf.GetString("name"); v != "demo" {
			t.Errorf("%s: 期望 'demo'，得到 '%s'", ext, v)
		}
		servers, ok := conf.Get("server").([]map[string]interface{})
		if !ok || len(servers) != 1 || servers[0]["port"] != 8080 {
			t.Errorf("%s: 期望 server 为含 port=8080 的列表，得到 %#v", ext, conf.Get("server"))
		}
	}

	// properties 值中的 ${key} 会被展开
	propDir := filepath.Join(dir, "properties")
	if err := os.MkdirAll(propDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(propDir, "config.properties"), []byte("host=localhost\nurl=http://${host}:8080\n"), 0644); err != nil {
		t.Fatal(err)
	}
	conf, err := New(WithConfigPaths(propDir))
	if err != nil {
		t.Fatal(err)
	}
	if v := conf.GetString("url"); v != "http://localhost:8080" {
		t.Errorf("期望 'http://localhost:8080'，得到 '%s'", v)
	}

	// 写回文件后可按同一格式重新读取
	for _, ext := range []string{"properties", "ini", "hcl"} {
		out := filepath.Join(dir, "out")
		if err := os.MkdirAll(out, 0755); err != nil {
			t.Fatal(err)
		}
		w, err := New(WithConfigPaths(out), WithConfigName("none"))
		if err != nil {
			t.Fatal(err)
		}
		w.Set("app.name", "demo")
		file := filepath.Join(out, "written."+ext)
		if err := w.WriteConfigAs(file); err != nil {
			t.Fatalf("%s: 写入失败: %v", ext, err)
		}
		r, err := New(WithConfigPaths(out), WithConfigName("written"))
		if err != nil {
			t.Fatalf("%s: 读取失败: %v", ext, err)
		}
		if ext == "hcl" {
			if r.Get("app") == nil {
				t.Errorf("%s: 写回后读不到 app", ext)
			}
		} else if v := r.GetString("app.name"); v != "demo" {
			t.Errorf("%s: 期望 'demo'，得到 '%s'", ext, v)
		}
		os.Remove(file)
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
