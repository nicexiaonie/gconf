package gconf

import (
	"sync"

	"github.com/fsnotify/fsnotify"
)

var (
	// defaultInstance 全局默认配置实例
	defaultInstance     *Gconf
	defaultInstanceOnce sync.Once
	defaultInstanceMu   sync.RWMutex
)

// Init 初始化全局配置实例（建议在应用启动时调用）
func Init(opts ...Option) error {
	var err error
	defaultInstanceOnce.Do(func() {
		defaultInstanceMu.Lock()
		defer defaultInstanceMu.Unlock()
		defaultInstance, err = New(opts...)
	})
	return err
}

// InitWithConfig 使用自定义配置初始化全局实例
// 此方法允许在初始化后重新设置全局实例（慎用）
func InitWithConfig(opts ...Option) error {
	defaultInstanceMu.Lock()
	defer defaultInstanceMu.Unlock()

	instance, err := New(opts...)
	if err != nil {
		return err
	}
	defaultInstance = instance
	return nil
}

// GetInstance 获取全局配置实例
// 如果未初始化，将使用默认配置自动初始化
func GetInstance() *Gconf {
	if defaultInstance == nil {
		_ = Init() // 使用默认配置初始化
	}
	defaultInstanceMu.RLock()
	defer defaultInstanceMu.RUnlock()
	return defaultInstance
}

// 以下为全局实例的便捷方法

// Get 获取配置值
func Get(key string) interface{} {
	return GetInstance().Get(key)
}

// GetString 获取字符串类型配置
func GetString(key string) string {
	return GetInstance().GetString(key)
}

// GetBool 获取布尔类型配置
func GetBool(key string) bool {
	return GetInstance().GetBool(key)
}

// GetInt 获取整数类型配置
func GetInt(key string) int {
	return GetInstance().GetInt(key)
}

// GetInt32 获取 int32 类型配置
func GetInt32(key string) int32 {
	return GetInstance().GetInt32(key)
}

// GetInt64 获取 int64 类型配置
func GetInt64(key string) int64 {
	return GetInstance().GetInt64(key)
}

// GetUint 获取无符号整数类型配置
func GetUint(key string) uint {
	return GetInstance().GetUint(key)
}

// GetUint32 获取 uint32 类型配置
func GetUint32(key string) uint32 {
	return GetInstance().GetUint32(key)
}

// GetUint64 获取 uint64 类型配置
func GetUint64(key string) uint64 {
	return GetInstance().GetUint64(key)
}

// GetFloat64 获取浮点数类型配置
func GetFloat64(key string) float64 {
	return GetInstance().GetFloat64(key)
}

// GetStringSlice 获取字符串切片类型配置
func GetStringSlice(key string) []string {
	return GetInstance().GetStringSlice(key)
}

// GetStringMap 获取字符串映射类型配置
func GetStringMap(key string) map[string]interface{} {
	return GetInstance().GetStringMap(key)
}

// GetStringMapString 获取字符串到字符串映射类型配置
func GetStringMapString(key string) map[string]string {
	return GetInstance().GetStringMapString(key)
}

// Set 设置配置值
func Set(key string, value interface{}) {
	GetInstance().Set(key, value)
}

// SetDefault 设置默认值
func SetDefault(key string, value interface{}) {
	GetInstance().SetDefault(key, value)
}

// IsSet 检查配置键是否存在
func IsSet(key string) bool {
	return GetInstance().IsSet(key)
}

// AllKeys 获取所有配置键
func AllKeys() []string {
	return GetInstance().AllKeys()
}

// AllSettings 获取所有配置
func AllSettings() map[string]interface{} {
	return GetInstance().AllSettings()
}

// Unmarshal 将配置解析到结构体
func Unmarshal(rawVal interface{}) error {
	return GetInstance().Unmarshal(rawVal)
}

// UnmarshalKey 将指定键的配置解析到结构体
func UnmarshalKey(key string, rawVal interface{}) error {
	return GetInstance().UnmarshalKey(key, rawVal)
}

// WriteConfig 写入配置到文件
func WriteConfig() error {
	return GetInstance().WriteConfig()
}

// ConfigFileUsed 获取当前使用的配置文件路径
func ConfigFileUsed() string {
	return GetInstance().ConfigFileUsed()
}

// OnConfigChange 注册配置变化回调函数
func OnConfigChange(fn func(fsnotify.Event)) {
	GetInstance().OnConfigChange(fn)
}

// Debug 打印所有配置信息（用于调试）
func Debug() {
	GetInstance().Debug()
}
