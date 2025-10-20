# Changelog

本文档记录了 Gconf 的所有重要变更。

## [2.0.0] - 2025-10-20

### 重大重构 🎉

这是一个完全重构的版本，提供了更强大、更易用的配置管理功能。

### 新增功能 ✨

- **核心功能重构**
  - 完全重写配置管理器，提供更清晰的 API
  - 使用 Options 模式，配置更灵活
  - 提供全局单例模式，简化使用
  
- **配置管理**
  - 支持多配置文件路径搜索
  - 支持多种配置格式 (YAML, JSON, TOML, HCL, INI, ENV, Properties)
  - 配置文件热更新和监听
  - 支持配置别名
  - 支持子配置树访问
  
- **环境变量集成**
  - 自动读取环境变量
  - 环境变量前缀支持
  - 环境变量键替换规则
  - 绑定特定环境变量
  
- **类型安全**
  - 完整的类型转换方法 (String, Int, Bool, Float, Duration, Time, etc.)
  - 支持复杂类型 (Slice, Map, StringMap, etc.)
  - 结构体解析 (Unmarshal, UnmarshalKey, UnmarshalExact)
  
- **配置选项**
  - `WithConfigName` - 设置配置文件名
  - `WithConfigType` - 设置配置文件类型
  - `WithConfigPaths` - 设置配置文件搜索路径
  - `WithWatchConfig` - 启用配置监听
  - `WithAutomaticEnv` - 自动读取环境变量
  - `WithEnvPrefix` - 设置环境变量前缀
  - `WithEnvKeyReplacer` - 环境变量键替换
  - `WithOnConfigChange` - 配置变化回调
  - `WithDebug` - 启用调试模式
  
- **全局 API**
  - `Init()` - 初始化全局配置
  - `GetInstance()` - 获取全局实例
  - 所有便捷的全局方法 (Get, Set, Unmarshal, etc.)
  
- **实例 API**
  - 完整的 getter 方法 (Get, GetString, GetInt, GetBool, etc.)
  - 配置设置方法 (Set, SetDefault)
  - 配置检查方法 (IsSet, AllKeys, AllSettings)
  - 配置解析方法 (Unmarshal, UnmarshalKey, UnmarshalExact)
  - 配置写入方法 (WriteConfig, SafeWriteConfig, etc.)
  - 子配置方法 (Sub)
  - 别名支持 (RegisterAlias)
  
- **开发体验**
  - 详细的中英文文档
  - 完整的示例代码 (基础、高级、环境变量)
  - 全面的单元测试 (覆盖核心功能和全局 API)
  - 性能基准测试
  - Makefile 支持
  - 快速开始指南

### 文档 📚

- 完整的 README (中英文)
- 快速开始指南 (QUICKSTART.md)
- 详细的 API 文档
- 多个实用示例
- 最佳实践指南

### 测试 🧪

- 23 个单元测试，覆盖所有核心功能
- 环境变量集成测试
- 配置解析测试
- 全局 API 测试
- 性能基准测试

### 示例 📝

- 基础使用示例 (example/basic)
- 高级功能示例 (example/advanced)
- 环境变量示例 (example/env)
- 示例配置文件

### 破坏性变更 ⚠️

本版本是完全重构，API 有重大变化：

**旧版本 (1.x):**
```go
c := gconf.Config{
    ConfigPath: "./",
    ConfigName: "test",
}
gc, _ := gconf.New(c)
```

**新版本 (2.0):**
```go
// 方式 1: 全局实例
gconf.Init(
    gconf.WithConfigName("test"),
    gconf.WithConfigPaths("./"),
)

// 方式 2: 独立实例
conf, _ := gconf.New(
    gconf.WithConfigName("test"),
    gconf.WithConfigPaths("./"),
)
```

### 迁移指南

从 1.x 迁移到 2.0:

1. **更新配置初始化**
   ```go
   // 旧版本
   c := gconf.Config{ConfigPath: "./", ConfigName: "config"}
   gc, _ := gconf.New(c)
   
   // 新版本 - 使用全局实例
   gconf.Init(
       gconf.WithConfigName("config"),
       gconf.WithConfigPaths("./"),
   )
   ```

2. **更新配置读取**
   ```go
   // 旧版本
   value := gc.Get("key")
   
   // 新版本
   value := gconf.Get("key")  // 全局实例
   // 或
   value := conf.Get("key")   // 独立实例
   ```

3. **更新配置监听**
   ```go
   // 旧版本
   c.WatchConfig = true
   c.CallOnConfigChange = func(e fsnotify.Event) { }
   
   // 新版本
   gconf.Init(
       gconf.WithWatchConfig(true),
       gconf.WithOnConfigChange(func(e fsnotify.Event) { }),
   )
   ```

## [1.0.0] - 之前版本

初始版本，基础功能。

---

## 版本说明

版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)：

- **主版本号**：不兼容的 API 变更
- **次版本号**：向下兼容的功能新增
- **修订号**：向下兼容的问题修正

## 反馈

如有问题或建议，请提交 [Issue](https://github.com/nicexiaonie/gconf/issues)。

