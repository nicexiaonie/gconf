package gconf

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-viper/encoding/hcl"
	"github.com/go-viper/encoding/ini"
	"github.com/go-viper/encoding/javaproperties"
	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

// Gconf 配置管理器，封装 viper，提供更便捷的配置管理功能
type Gconf struct {
	viper *viper.Viper
	// viperMu 保护 viper 实例：读取加读锁，修改与重载加写锁（viper 本身不保证并发安全）
	viperMu          sync.RWMutex
	mu               sync.RWMutex
	onChangeHandlers []func(fsnotify.Event)
	// decoderOptions 解析结构体时传给 viper 的解码选项，由 Options.TagName 决定
	decoderOptions []viper.DecoderConfigOption
	// watcher 监听配置文件变化，未启用监听时为 nil；done 在 Close 时关闭，通知监听循环退出
	watcher   *fsnotify.Watcher
	done      chan struct{}
	closeOnce sync.Once
}

// Options 配置选项
type Options struct {
	// 配置文件路径列表，支持多个路径
	ConfigPaths []string
	// 配置文件名（不包含扩展名）
	ConfigName string
	// 配置文件类型（yaml, json, toml, properties, hcl, env, ini）
	// 为空时按配置文件扩展名推断；显式指定时以指定类型为准
	ConfigType string
	// 是否自动监听配置文件变化
	WatchConfig bool
	// 是否自动读取环境变量
	AutomaticEnv bool
	// 环境变量前缀
	EnvPrefix string
	// 环境变量键的替换规则（例如将 . 替换为 _）
	EnvKeyReplacer *strings.Replacer
	// 配置变化回调函数
	OnConfigChange func(fsnotify.Event)
	// 是否启用调试日志
	Debug bool
	// 解析结构体时匹配字段使用的标签名（如 json、toml），为空时使用 viper 默认的 mapstructure
	TagName string
}

// New 创建一个新的配置管理器实例
func New(opts ...Option) (*Gconf, error) {
	options := &Options{
		ConfigPaths: []string{".", "./config"},
		ConfigName:  "config",
		WatchConfig: false,
		Debug:       false,
	}

	// 应用选项
	for _, opt := range opts {
		opt(options)
	}

	g := &Gconf{
		viper:            newViper(),
		onChangeHandlers: make([]func(fsnotify.Event), 0),
		decoderOptions:   decoderOptions(options.TagName),
	}

	// 设置配置文件路径
	for _, path := range options.ConfigPaths {
		g.viper.AddConfigPath(path)
	}

	// 设置配置文件名
	g.viper.SetConfigName(options.ConfigName)

	// 设置配置文件类型
	if options.ConfigType != "" {
		g.viper.SetConfigType(options.ConfigType)
	}

	// 设置环境变量
	if options.AutomaticEnv {
		g.viper.AutomaticEnv()
		if options.EnvPrefix != "" {
			g.viper.SetEnvPrefix(options.EnvPrefix)
		}
		if options.EnvKeyReplacer != nil {
			g.viper.SetEnvKeyReplacer(options.EnvKeyReplacer)
		}
	}

	// 读取配置文件
	if err := g.viper.ReadInConfig(); err != nil {
		if options.Debug {
			log.Printf("[gconf] 读取配置文件失败: %v", err)
		}
		// 配置文件不存在不算错误，可以使用默认值或环境变量
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
	} else if options.Debug {
		log.Printf("[gconf] 成功加载配置文件: %s", g.viper.ConfigFileUsed())
	}

	// 设置配置监听
	// viper 的 WatchConfig 会在其内部 goroutine 中直接重载自身，无法加锁，也无法停止，
	// 因此自行监听文件变化，再在写锁内重载主实例
	if options.WatchConfig {
		if err := g.watchConfig(options); err != nil {
			return nil, err
		}
	}

	return g, nil
}

// newViper 创建 viper 实例，并注册 viper 核心未内置的 HCL、Java properties、INI 解析器，
// 与 viper 支持的扩展名对应，使这些格式的配置文件可读写。
// 默认注册表的 RegisterCodec 不会返回错误，因此忽略其返回值
func newViper() *viper.Viper {
	registry := viper.NewCodecRegistry()
	hclCodec := hcl.Codec{}
	propertiesCodec := &javaproperties.Codec{}
	_ = registry.RegisterCodec("hcl", hclCodec)
	_ = registry.RegisterCodec("tfvars", hclCodec)
	_ = registry.RegisterCodec("properties", propertiesCodec)
	_ = registry.RegisterCodec("props", propertiesCodec)
	_ = registry.RegisterCodec("prop", propertiesCodec)
	_ = registry.RegisterCodec("ini", ini.Codec{})
	return viper.NewWithOptions(viper.WithCodecRegistry(registry))
}

// watchConfig 监听配置文件所在目录，判定规则与 viper 的 WatchConfig 一致：
// 配置文件被写入或创建，或配置路径解析出的真实文件变化（如 symlink 切换）时重载；配置文件被删除时停止监听。
// 启动时未找到配置文件则不监听。
func (g *Gconf) watchConfig(options *Options) error {
	filename := g.viper.ConfigFileUsed()
	if filename == "" {
		log.Printf("[gconf] 未找到配置文件，不监听配置变化")
		return nil
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("gconf: 创建配置文件监听失败: %w", err)
	}
	configFile := filepath.Clean(filename)
	if err := watcher.Add(filepath.Dir(configFile)); err != nil {
		watcher.Close()
		return fmt.Errorf("gconf: 监听配置文件目录失败: %w", err)
	}
	g.watcher = watcher
	g.done = make(chan struct{})

	realConfigFile, _ := filepath.EvalSymlinks(filename)
	go func() {
		defer watcher.Close()
		for {
			select {
			case <-g.done:
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// Close 之后收到的事件不再处理
				select {
				case <-g.done:
					return
				default:
				}
				currentConfigFile, _ := filepath.EvalSymlinks(filename)
				const writeOrCreateMask = fsnotify.Write | fsnotify.Create
				if (filepath.Clean(event.Name) == configFile && event.Op&writeOrCreateMask != 0) ||
					(currentConfigFile != "" && currentConfigFile != realConfigFile) {
					realConfigFile = currentConfigFile
					g.handleConfigChange(event, options)
				} else if filepath.Clean(event.Name) == configFile && event.Op&fsnotify.Remove != 0 {
					return
				}
			case err, ok := <-watcher.Errors:
				if ok {
					log.Printf("[gconf] 配置文件监听出错: %v", err)
				}
				return
			}
		}
	}()
	return nil
}

// handleConfigChange 重载配置并执行变更回调
func (g *Gconf) handleConfigChange(e fsnotify.Event, options *Options) {
	if options.Debug {
		log.Printf("[gconf] 配置文件变化: %s, 操作: %s", e.Name, e.Op)
	}

	// 回调中通常会读取配置，必须在释放写锁后再执行回调
	if err := g.ReadInConfig(); err != nil {
		log.Printf("[gconf] 重新加载配置文件失败: %v", err)
	}

	// 执行自定义回调
	if options.OnConfigChange != nil {
		options.OnConfigChange(e)
	}

	// 执行注册的所有回调
	g.mu.RLock()
	handlers := g.onChangeHandlers
	g.mu.RUnlock()

	for _, handler := range handlers {
		go handler(e)
	}
}

// Close 停止配置文件监听，可重复调用，未启用监听时直接返回 nil。
// Close 返回后不会再开始处理新的文件变化；已开始处理的变化不等待其重载与回调完成。
// Close 后仍可读取配置，值为最后一次成功加载的内容。
func (g *Gconf) Close() error {
	var err error
	g.closeOnce.Do(func() {
		if g.watcher == nil {
			return
		}
		close(g.done)
		err = g.watcher.Close()
	})
	return err
}

// Option 配置选项函数
type Option func(*Options)

// WithConfigPaths 设置配置文件路径
func WithConfigPaths(paths ...string) Option {
	return func(o *Options) {
		o.ConfigPaths = paths
	}
}

// WithConfigName 设置配置文件名
func WithConfigName(name string) Option {
	return func(o *Options) {
		o.ConfigName = name
	}
}

// WithConfigType 设置配置文件类型
func WithConfigType(configType string) Option {
	return func(o *Options) {
		o.ConfigType = configType
	}
}

// WithWatchConfig 启用配置文件监听
func WithWatchConfig(watch bool) Option {
	return func(o *Options) {
		o.WatchConfig = watch
	}
}

// WithAutomaticEnv 启用自动读取环境变量
func WithAutomaticEnv(auto bool) Option {
	return func(o *Options) {
		o.AutomaticEnv = auto
	}
}

// WithEnvPrefix 设置环境变量前缀
func WithEnvPrefix(prefix string) Option {
	return func(o *Options) {
		o.EnvPrefix = prefix
	}
}

// WithEnvKeyReplacer 设置环境变量键替换器
func WithEnvKeyReplacer(oldNew ...string) Option {
	return func(o *Options) {
		o.EnvKeyReplacer = strings.NewReplacer(oldNew...)
	}
}

// WithOnConfigChange 设置配置变化回调
func WithOnConfigChange(fn func(fsnotify.Event)) Option {
	return func(o *Options) {
		o.OnConfigChange = fn
	}
}

// WithDebug 启用调试模式
func WithDebug(debug bool) Option {
	return func(o *Options) {
		o.Debug = debug
	}
}

// WithTagName 设置解析结构体时匹配字段使用的标签名，如 "json"、"toml"
func WithTagName(tagName string) Option {
	return func(o *Options) {
		o.TagName = tagName
	}
}

// decoderOptions 根据标签名生成解码选项。
// json、toml 的原生库会把匿名嵌入结构体的字段提升到上一层，为保持一致同时开启 Squash。
func decoderOptions(tagName string) []viper.DecoderConfigOption {
	if tagName == "" || tagName == "mapstructure" {
		return nil
	}
	return []viper.DecoderConfigOption{func(c *mapstructure.DecoderConfig) {
		c.TagName = tagName
		c.Squash = tagName == "json" || tagName == "toml"
	}}
}

// OnConfigChange 注册配置变化回调函数
func (g *Gconf) OnConfigChange(fn func(fsnotify.Event)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.onChangeHandlers = append(g.onChangeHandlers, fn)
}

// Get 获取配置值
func (g *Gconf) Get(key string) interface{} {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.Get(key)
}

// GetString 获取字符串类型配置
func (g *Gconf) GetString(key string) string {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.GetString(key)
}

// GetBool 获取布尔类型配置
func (g *Gconf) GetBool(key string) bool {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.GetBool(key)
}

// GetInt 获取整数类型配置
func (g *Gconf) GetInt(key string) int {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.GetInt(key)
}

// GetInt32 获取 int32 类型配置
func (g *Gconf) GetInt32(key string) int32 {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.GetInt32(key)
}

// GetInt64 获取 int64 类型配置
func (g *Gconf) GetInt64(key string) int64 {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.GetInt64(key)
}

// GetUint 获取无符号整数类型配置
func (g *Gconf) GetUint(key string) uint {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.GetUint(key)
}

// GetUint32 获取 uint32 类型配置
func (g *Gconf) GetUint32(key string) uint32 {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.GetUint32(key)
}

// GetUint64 获取 uint64 类型配置
func (g *Gconf) GetUint64(key string) uint64 {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.GetUint64(key)
}

// GetFloat64 获取浮点数类型配置
func (g *Gconf) GetFloat64(key string) float64 {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.GetFloat64(key)
}

// GetTime 获取时间类型配置
func (g *Gconf) GetTime(key string) time.Time {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.GetTime(key)
}

// GetDuration 获取时间间隔类型配置
func (g *Gconf) GetDuration(key string) time.Duration {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.GetDuration(key)
}

// GetStringSlice 获取字符串切片类型配置
func (g *Gconf) GetStringSlice(key string) []string {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.GetStringSlice(key)
}

// GetStringMap 获取字符串映射类型配置
func (g *Gconf) GetStringMap(key string) map[string]interface{} {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.GetStringMap(key)
}

// GetStringMapString 获取字符串到字符串映射类型配置
func (g *Gconf) GetStringMapString(key string) map[string]string {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.GetStringMapString(key)
}

// GetStringMapStringSlice 获取字符串到字符串切片映射类型配置
func (g *Gconf) GetStringMapStringSlice(key string) map[string][]string {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.GetStringMapStringSlice(key)
}

// GetSizeInBytes 获取字节大小类型配置（支持 KB, MB, GB 等）
func (g *Gconf) GetSizeInBytes(key string) uint {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.GetSizeInBytes(key)
}

// Set 设置配置值
func (g *Gconf) Set(key string, value interface{}) {
	g.viperMu.Lock()
	defer g.viperMu.Unlock()
	g.viper.Set(key, value)
}

// SetDefault 设置默认值
func (g *Gconf) SetDefault(key string, value interface{}) {
	g.viperMu.Lock()
	defer g.viperMu.Unlock()
	g.viper.SetDefault(key, value)
}

// IsSet 检查配置键是否存在
func (g *Gconf) IsSet(key string) bool {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.IsSet(key)
}

// AllKeys 获取所有配置键
func (g *Gconf) AllKeys() []string {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.AllKeys()
}

// AllSettings 获取所有配置
func (g *Gconf) AllSettings() map[string]interface{} {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.AllSettings()
}

// Unmarshal 将配置解析到结构体
func (g *Gconf) Unmarshal(rawVal interface{}) error {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.Unmarshal(rawVal, g.decoderOptions...)
}

// UnmarshalKey 将指定键的配置解析到结构体
func (g *Gconf) UnmarshalKey(key string, rawVal interface{}) error {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.UnmarshalKey(key, rawVal, g.decoderOptions...)
}

// UnmarshalExact 严格解析配置到结构体（结构体中未定义的字段会报错）
func (g *Gconf) UnmarshalExact(rawVal interface{}) error {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.UnmarshalExact(rawVal, g.decoderOptions...)
}

// WriteConfig 写入配置到文件
func (g *Gconf) WriteConfig() error {
	g.viperMu.Lock()
	defer g.viperMu.Unlock()
	return g.viper.WriteConfig()
}

// SafeWriteConfig 安全写入配置（文件存在时不覆盖）
func (g *Gconf) SafeWriteConfig() error {
	g.viperMu.Lock()
	defer g.viperMu.Unlock()
	return g.viper.SafeWriteConfig()
}

// WriteConfigAs 写入配置到指定文件
func (g *Gconf) WriteConfigAs(filename string) error {
	g.viperMu.Lock()
	defer g.viperMu.Unlock()
	return g.viper.WriteConfigAs(filename)
}

// SafeWriteConfigAs 安全写入配置到指定文件（文件存在时不覆盖）
func (g *Gconf) SafeWriteConfigAs(filename string) error {
	g.viperMu.Lock()
	defer g.viperMu.Unlock()
	return g.viper.SafeWriteConfigAs(filename)
}

// ReadInConfig 重新读取配置文件
func (g *Gconf) ReadInConfig() error {
	g.viperMu.Lock()
	defer g.viperMu.Unlock()
	return g.viper.ReadInConfig()
}

// MergeInConfig 合并配置文件
func (g *Gconf) MergeInConfig() error {
	g.viperMu.Lock()
	defer g.viperMu.Unlock()
	return g.viper.MergeInConfig()
}

// ConfigFileUsed 获取当前使用的配置文件路径
func (g *Gconf) ConfigFileUsed() string {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	return g.viper.ConfigFileUsed()
}

// BindEnv 绑定环境变量到配置键
func (g *Gconf) BindEnv(keys ...string) error {
	g.viperMu.Lock()
	defer g.viperMu.Unlock()
	return g.viper.BindEnv(keys...)
}

// RegisterAlias 注册配置键别名
func (g *Gconf) RegisterAlias(alias string, key string) {
	g.viperMu.Lock()
	defer g.viperMu.Unlock()
	g.viper.RegisterAlias(alias, key)
}

// Sub 获取子配置树
func (g *Gconf) Sub(key string) *Gconf {
	g.viperMu.RLock()
	defer g.viperMu.RUnlock()
	subViper := g.viper.Sub(key)
	if subViper == nil {
		return nil
	}
	return &Gconf{
		viper:            subViper,
		onChangeHandlers: make([]func(fsnotify.Event), 0),
		decoderOptions:   g.decoderOptions,
	}
}

// GetViper 获取底层的 viper 实例（用于高级操作）
// 注意：直接操作返回的实例会绕过 gconf 的锁，与热更新并发时不保证安全
func (g *Gconf) GetViper() *viper.Viper {
	return g.viper
}

// Debug 打印所有配置信息（用于调试）
func (g *Gconf) Debug() {
	fmt.Println("=== Gconf Debug Info ===")
	fmt.Printf("Config File: %s\n", g.ConfigFileUsed())
	fmt.Println("All Settings:")
	for k, v := range g.AllSettings() {
		fmt.Printf("  %s: %v\n", k, v)
	}
	fmt.Println("========================")
}
