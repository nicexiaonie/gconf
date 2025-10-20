package gconf

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

// Gconf 配置管理器，封装 viper，提供更便捷的配置管理功能
type Gconf struct {
	viper            *viper.Viper
	mu               sync.RWMutex
	onChangeHandlers []func(fsnotify.Event)
}

// Options 配置选项
type Options struct {
	// 配置文件路径列表，支持多个路径
	ConfigPaths []string
	// 配置文件名（不包含扩展名）
	ConfigName string
	// 配置文件类型（yaml, json, toml, properties, hcl, env, ini）
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
}

// New 创建一个新的配置管理器实例
func New(opts ...Option) (*Gconf, error) {
	options := &Options{
		ConfigPaths: []string{".", "./config"},
		ConfigName:  "config",
		ConfigType:  "yaml",
		WatchConfig: false,
		Debug:       false,
	}

	// 应用选项
	for _, opt := range opts {
		opt(options)
	}

	g := &Gconf{
		viper:            viper.New(),
		onChangeHandlers: make([]func(fsnotify.Event), 0),
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
	if options.WatchConfig {
		g.viper.WatchConfig()
		g.viper.OnConfigChange(func(e fsnotify.Event) {
			if options.Debug {
				log.Printf("[gconf] 配置文件变化: %s, 操作: %s", e.Name, e.Op)
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
		})
	}

	return g, nil
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

// OnConfigChange 注册配置变化回调函数
func (g *Gconf) OnConfigChange(fn func(fsnotify.Event)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.onChangeHandlers = append(g.onChangeHandlers, fn)
}

// Get 获取配置值
func (g *Gconf) Get(key string) interface{} {
	return g.viper.Get(key)
}

// GetString 获取字符串类型配置
func (g *Gconf) GetString(key string) string {
	return g.viper.GetString(key)
}

// GetBool 获取布尔类型配置
func (g *Gconf) GetBool(key string) bool {
	return g.viper.GetBool(key)
}

// GetInt 获取整数类型配置
func (g *Gconf) GetInt(key string) int {
	return g.viper.GetInt(key)
}

// GetInt32 获取 int32 类型配置
func (g *Gconf) GetInt32(key string) int32 {
	return g.viper.GetInt32(key)
}

// GetInt64 获取 int64 类型配置
func (g *Gconf) GetInt64(key string) int64 {
	return g.viper.GetInt64(key)
}

// GetUint 获取无符号整数类型配置
func (g *Gconf) GetUint(key string) uint {
	return g.viper.GetUint(key)
}

// GetUint32 获取 uint32 类型配置
func (g *Gconf) GetUint32(key string) uint32 {
	return g.viper.GetUint32(key)
}

// GetUint64 获取 uint64 类型配置
func (g *Gconf) GetUint64(key string) uint64 {
	return g.viper.GetUint64(key)
}

// GetFloat64 获取浮点数类型配置
func (g *Gconf) GetFloat64(key string) float64 {
	return g.viper.GetFloat64(key)
}

// GetTime 获取时间类型配置
func (g *Gconf) GetTime(key string) time.Time {
	return g.viper.GetTime(key)
}

// GetDuration 获取时间间隔类型配置
func (g *Gconf) GetDuration(key string) time.Duration {
	return g.viper.GetDuration(key)
}

// GetStringSlice 获取字符串切片类型配置
func (g *Gconf) GetStringSlice(key string) []string {
	return g.viper.GetStringSlice(key)
}

// GetStringMap 获取字符串映射类型配置
func (g *Gconf) GetStringMap(key string) map[string]interface{} {
	return g.viper.GetStringMap(key)
}

// GetStringMapString 获取字符串到字符串映射类型配置
func (g *Gconf) GetStringMapString(key string) map[string]string {
	return g.viper.GetStringMapString(key)
}

// GetStringMapStringSlice 获取字符串到字符串切片映射类型配置
func (g *Gconf) GetStringMapStringSlice(key string) map[string][]string {
	return g.viper.GetStringMapStringSlice(key)
}

// GetSizeInBytes 获取字节大小类型配置（支持 KB, MB, GB 等）
func (g *Gconf) GetSizeInBytes(key string) uint {
	return g.viper.GetSizeInBytes(key)
}

// Set 设置配置值
func (g *Gconf) Set(key string, value interface{}) {
	g.viper.Set(key, value)
}

// SetDefault 设置默认值
func (g *Gconf) SetDefault(key string, value interface{}) {
	g.viper.SetDefault(key, value)
}

// IsSet 检查配置键是否存在
func (g *Gconf) IsSet(key string) bool {
	return g.viper.IsSet(key)
}

// AllKeys 获取所有配置键
func (g *Gconf) AllKeys() []string {
	return g.viper.AllKeys()
}

// AllSettings 获取所有配置
func (g *Gconf) AllSettings() map[string]interface{} {
	return g.viper.AllSettings()
}

// Unmarshal 将配置解析到结构体
func (g *Gconf) Unmarshal(rawVal interface{}) error {
	return g.viper.Unmarshal(rawVal)
}

// UnmarshalKey 将指定键的配置解析到结构体
func (g *Gconf) UnmarshalKey(key string, rawVal interface{}) error {
	return g.viper.UnmarshalKey(key, rawVal)
}

// UnmarshalExact 严格解析配置到结构体（结构体中未定义的字段会报错）
func (g *Gconf) UnmarshalExact(rawVal interface{}) error {
	return g.viper.UnmarshalExact(rawVal)
}

// WriteConfig 写入配置到文件
func (g *Gconf) WriteConfig() error {
	return g.viper.WriteConfig()
}

// SafeWriteConfig 安全写入配置（文件存在时不覆盖）
func (g *Gconf) SafeWriteConfig() error {
	return g.viper.SafeWriteConfig()
}

// WriteConfigAs 写入配置到指定文件
func (g *Gconf) WriteConfigAs(filename string) error {
	return g.viper.WriteConfigAs(filename)
}

// SafeWriteConfigAs 安全写入配置到指定文件（文件存在时不覆盖）
func (g *Gconf) SafeWriteConfigAs(filename string) error {
	return g.viper.SafeWriteConfigAs(filename)
}

// ReadInConfig 重新读取配置文件
func (g *Gconf) ReadInConfig() error {
	return g.viper.ReadInConfig()
}

// MergeInConfig 合并配置文件
func (g *Gconf) MergeInConfig() error {
	return g.viper.MergeInConfig()
}

// ConfigFileUsed 获取当前使用的配置文件路径
func (g *Gconf) ConfigFileUsed() string {
	return g.viper.ConfigFileUsed()
}

// BindEnv 绑定环境变量到配置键
func (g *Gconf) BindEnv(keys ...string) error {
	return g.viper.BindEnv(keys...)
}

// RegisterAlias 注册配置键别名
func (g *Gconf) RegisterAlias(alias string, key string) {
	g.viper.RegisterAlias(alias, key)
}

// Sub 获取子配置树
func (g *Gconf) Sub(key string) *Gconf {
	subViper := g.viper.Sub(key)
	if subViper == nil {
		return nil
	}
	return &Gconf{
		viper:            subViper,
		onChangeHandlers: make([]func(fsnotify.Event), 0),
	}
}

// GetViper 获取底层的 viper 实例（用于高级操作）
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
