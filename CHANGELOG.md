# Changelog

本文档记录了 Gconf 的所有重要变更。

## [Unreleased]

### 新增 ✨

- **`Close()` 停止配置文件监听并释放监听资源**：可重复调用；Close 后不再重载与触发回调，仍可读取最后一次加载的配置

### 修复 🐛

- **再次调用 `Init` 时关闭被替换的旧实例**：此前旧实例的文件监听无法停止，goroutine 与文件句柄泄漏，旧实例的回调仍会触发
- **配置文件监听改由 gconf 自行实现**，不再借助额外的 viper 实例：每次变更只解析一次配置文件；触发重载的判定规则与 viper 一致，含 symlink 切换
- 创建或注册文件监听失败时 `New()` 返回错误；此前创建失败会由 viper 直接 `log.Fatal` 退出进程，注册目录失败被静默忽略

- **`Sub()` 返回的子配置沿用父实例的 `WithTagName`**：此前子配置的 `Unmarshal` 等方法会退回 mapstructure 标签，按 json/toml 标签定义的字段解析为零值
- **`GetInstance()` 自动初始化失败时立即 panic 并携带原始错误**：此前错误被忽略、返回 nil，后续调用以无错误信息的空指针 panic

### 不兼容变更 ⚠️

- 未调用 `Init` 且默认配置文件无法解析时，`GetInstance()` 不再返回 nil，而是直接 panic；依赖判断其返回值是否为 nil 的代码需改为在启动时调用 `Init` 并检查错误
- 主模块最低 Go 版本由 1.14 提升至 1.16；依赖版本不变

## [1.1.0] - 2026-10-30

主包 `v1.1.0`，Apollo 子包 `apollo/v1.1.0`。

### 变更 🔧

- **未指定 `ConfigType` 时按配置文件扩展名选择解析器**
  - 此前默认 `ConfigType` 为 `yaml`，任何扩展名的配置文件都交给 YAML 解析器，导致默认选项下 `config.toml` 初始化失败、`config.json` 中的 `\/`、代理对等合法转义报错
  - 默认 `ConfigType` 改为空，`.toml`、`.json`、`.yaml` 等文件按各自格式解析，开箱支持 TOML
  - 显式指定 `WithConfigType` 时仍以指定类型为准，与扩展名无关

### 不兼容变更 ⚠️

以下影响仅针对未调用 `WithConfigType` 的用户：

- `config.json` 改由 JSON 解析器读取：`Get()` 返回的数字由 `int` 变为 `float64`，超出 float64 精度的大整数会丢失精度；`GetInt` 等带类型转换的方法不受影响
- 无扩展名的配置文件（如 `config`）将不再被查找到，需显式指定 `WithConfigType`
- `SafeWriteConfig()` 无法确定要生成的文件类型而返回错误，需显式指定 `WithConfigType`，或改用 `SafeWriteConfigAs("config.yaml")`

### 变更 🔧（apollo 模块）

- **未配置目标格式时原样落地**
  - 文件型 Namespace 写入 Apollo 原文，注释、key 顺序、缩进均保留；此前会解析后重新序列化
  - properties 型 Namespace 按 key 排序写为 `key=value` 的 properties 文件；此前默认转为 JSON
  - 原样落地前默认检查原文能否解析，失败不写入；新增 `NamespaceConfig.SkipValidate` 可跳过检查
- **新增目标格式 `FormatTOML`、`FormatProperties`**
  - 转换遇到目标格式无法表达的内容时报错，不自动修正：TOML 的 `null`、元素类型不一致的数组、超出 int64 的整数；properties 的数组、`null`、展开后重复或无法正确读回的 key
- **JSON 源按原文区分整数与小数**，整数不再被读成浮点数，大整数不丢精度
- **启动时配置内容错误一律失败**：原文无法解析或转换失败时，无论 `Required` 与 LKG 如何，`Start()` 均返回错误且不覆盖已有文件；Apollo 不可用时仍按原有 `Required`/LKG 规则降级
- **热更新内容错误记录到 `Status().LastError`**；此前解析失败的变更被静默丢弃
- **非法格式组合在 `New()` 时报错**：未知目标格式、TXT/`.properties` 文件型配置转换、转换为 TXT、转换时配置 `SkipValidate`；此前未知格式会被静默写成 JSON

### 不兼容变更 ⚠️（apollo 模块）

- 删除 `Config.Format`：设置该字段的代码将编译失败，需改为在 `NamespaceConfig.Format` 上按 Namespace 配置
- properties 型 Namespace 默认输出由 `<name>.json` 改为 `<name>.properties`；旧的 `.json` 文件不会被删除也不再更新，读取方需切换到新文件
- 文件型原样落地后文件内容变为 Apollo 原文，升级后首次同步会生成新版本并触发一次 gconf 热更新
- 升级后首次启动时，目标格式发生变化的 Namespace 没有对应 LKG，需能连接 Apollo；此时若 Apollo 不可用且 `Required: true`，`Start()` 将失败
- `Filename` 不再校验扩展名与目标格式一致，严格按配置写入；扩展名与内容不一致时 gconf 需通过 `WithConfigType` 指定类型
- `SnapshotStore` 接口改为写入已生成的文件内容：`Load` 返回 releaseKey，`Publish` 接收文件字节

### 修复 🐛（apollo 模块）

- **修复 yaml/yml Namespace 无法同步**
  - agollo 默认将 yaml/yml 预解析为扁平键值并丢弃 `content` 原文，导致读取原文失败
  - 改为在 agollo 中注册不预解析的 yaml/yml 解析器，保留原文由模块解析；该注册对整个进程生效
- **修复带点的 properties 型 Namespace（如 `TEST1.redis`）无法同步**
  - 此前名称含点即视为文件型；现与 Apollo/agollo 一致，仅已知格式后缀视为文件型
  - 此类 Namespace 的输出文件名保留完整名称，例如 `TEST1.redis.properties`

### 文档 📚

- 按当前实现重写 `README.md`、`apollo/README.md`、`QUICKSTART.md`，删除与 `README.md` 重复的 `README_CN.md`
- 修正 apollo 本地目录结构与 gconf 读取路径的错误描述，注明 gconf 不支持读取 XML、`Merge`/`Retry` 当前无效果
- 修正 QUICKSTART 中错误的配置优先级（`Set()` 高于环境变量）及不存在的包级函数 `GetDuration`、`GetTime`

### 测试 🧪

- 新增 `TestConfigTypeInferredFromExt`、`TestExplicitConfigTypeOverridesExt`
- apollo 模块新增基于模拟 Apollo 服务的 `TestAdapterSnapshotThroughAgollo`，以及 `TestChangeListenerDottedPropertiesNamespace`、`TestIsFileType`、`TestOutputFilenameDottedNamespace`
- apollo 模块新增 `render_test.go`，覆盖原样落地、解析检查开关、TOML/properties 转换及报错、格式组合校验、启动与热更新时的内容错误处理

## [1.0.4] - 2026-10-26

### 修复 🐛

- **修复热更新与并发读写之间的数据竞争**
  - 此前 `Get*`、`Unmarshal` 等方法未加锁，与 viper 内部的配置文件重载同时执行时存在数据竞争；`Set` 与读取并发时可能触发 `concurrent map read and map write` 导致进程崩溃
  - 所有访问底层 viper 的方法加读写锁：读取加读锁，修改与重载加写锁
  - 改由影子 viper 监听配置文件，在写锁内重载主实例；回调在重载完成并释放锁后执行
  - `Unmarshal`、`AllSettings` 在热更新期间不再读到新旧混合的配置

### 文档 📚

- README 补充热更新的并发语义说明及 `GetViper()` 绕过锁的注意事项
- 修正 README 热更新最佳实践示例中存在数据竞争的写法

### 测试 🧪

- 新增 `TestConcurrentReadDuringReload` 并发测试（需配合 `-race` 运行）

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

