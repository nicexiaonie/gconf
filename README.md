# Gconf

[![Go Version](https://img.shields.io/badge/Go-%3E%3D%201.14-blue)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

Gconf 是基于 [spf13/viper](https://github.com/spf13/viper) 的 Go 配置管理封装，提供 Options 风格的初始化、全局实例、并发安全的读写与热更新，以及可选的 Apollo 配置中心同步模块。

## 目录

- [特性](#特性)
- [安装](#安装)
- [快速开始](#快速开始)
- [配置文件查找与格式识别](#配置文件查找与格式识别)
- [初始化选项](#初始化选项)
- [创建实例](#创建实例)
- [读取配置](#读取配置)
- [解析到结构体](#解析到结构体)
- [修改配置与优先级](#修改配置与优先级)
- [环境变量](#环境变量)
- [热更新](#热更新)
- [写入配置文件](#写入配置文件)
- [其他功能](#其他功能)
- [API 速查](#api-速查)
- [Apollo 配置中心](#apollo-配置中心)
- [示例与开发命令](#示例与开发命令)
- [常见问题](#常见问题)
- [版本与变更](#版本与变更)

## 特性

- **Options 初始化**：通过 `WithXxx` 选项组合配置路径、名称、类型、环境变量与热更新
- **多种格式**：YAML、JSON、TOML、properties、HCL、INI、env/dotenv，按扩展名自动识别
- **全局实例**：`Init` 一次初始化，任意位置通过包级函数读取
- **环境变量**：支持前缀、键名替换与显式绑定，环境变量可覆盖配置文件
- **结构体解析**：基于 mapstructure，支持 `time.Duration`、逗号分隔字符串转切片
- **热更新**：监听配置文件变化并自动重载，支持变更回调
- **并发安全**：所有读写与热更新重载之间加锁，读取不会看到新旧混合的配置
- **Apollo 同步（可选）**：独立子模块，将 Apollo 配置原样或转换格式后可靠落盘，供 gconf 读取

## 安装

```bash
go get github.com/nicexiaonie/gconf
```

- 主模块要求 Go 1.14 及以上
- 可选的 Apollo 子模块为独立 Go module，要求 Go 1.20 及以上，详见 [Apollo 配置中心](#apollo-配置中心)

## 快速开始

准备配置文件 `./config/config.yaml`：

```yaml
app:
  name: demo
server:
  port: 8080
  timeout: 30s
```

读取配置：

```go
package main

import (
    "fmt"
    "log"

    "github.com/nicexiaonie/gconf"
)

func main() {
    // 默认在 "." 与 "./config" 下查找名为 config 的配置文件，按扩展名识别格式
    if err := gconf.Init(); err != nil {
        log.Fatal(err)
    }

    fmt.Println(gconf.GetString("app.name"))                       // demo
    fmt.Println(gconf.GetInt("server.port"))                       // 8080
    fmt.Println(gconf.GetInstance().GetDuration("server.timeout")) // 30s
}
```

## 配置文件查找与格式识别

### 查找规则

1. 按 `ConfigPaths` 的顺序逐个目录查找，默认 `"."`、`"./config"`
2. 在每个目录中按以下扩展名顺序查找 `<ConfigName>.<扩展名>`，**第一个存在的文件生效**：
   `json`、`toml`、`yaml`、`yml`、`properties`、`props`、`prop`、`hcl`、`dotenv`、`env`、`ini`
3. 显式指定了 `ConfigType` 时，还会查找不带扩展名的 `<ConfigName>` 文件

同一目录同时存在 `config.json` 与 `config.yaml` 时，读取的是 `config.json`。

### 格式识别

| 情况 | 解析方式 |
|---|---|
| 未指定 `ConfigType` | 按找到的文件扩展名选择解析器 |
| 指定了 `ConfigType` | 一律按指定类型解析，与扩展名无关 |
| 文件无扩展名且未指定 `ConfigType` | **找不到文件**，配置为空且不报错 |

### 加载结果

| 情况 | `New` / `Init` 的结果 |
|---|---|
| 找到文件且解析成功 | 正常返回 |
| 未找到配置文件 | 正常返回，配置为空，可依赖默认值与环境变量 |
| 找到文件但解析失败 | 返回错误 |

可通过 `ConfigFileUsed()` 确认实际加载的文件，返回空字符串表示未加载任何文件。

### 支持的格式

| 格式 | 扩展名 | 说明 |
|---|---|---|
| YAML | `.yaml`、`.yml` | |
| JSON | `.json` | 数字统一读为 `float64`，通过 `GetInt` 等方法读取时会转换 |
| TOML | `.toml` | |
| properties | `.properties`、`.props`、`.prop` | key 中的 `.` 视为层级；值中的 `${key}` 会被展开，引用不存在的 key 时替换为空字符串，自引用报错 |
| HCL | `.hcl` | 块结构解析为列表，如 `server { port = 8080 }` 读为 `server: [{port: 8080}]`，不能用 `server.port` 访问 |
| INI | `.ini` | section 作为第一层，如 `[server]` 下的 `port` 读为 `server.port` |
| env / dotenv | `.env`、`.dotenv` | 扁平 key，如 `SERVER_PORT=8080` 读为 `server_port`，不会拆成层级 |

## 初始化选项

| 选项 | 默认值 | 说明 |
|---|---|---|
| `WithConfigPaths(paths ...string)` | `"."`、`"./config"` | 配置文件搜索目录，按顺序查找；传入后**替换**默认值 |
| `WithConfigName(name string)` | `"config"` | 配置文件名，不含扩展名；传空字符串无效，仍使用默认值 |
| `WithConfigType(t string)` | 空 | 配置类型，如 `yaml`、`json`、`toml`、`properties`；为空时按扩展名识别 |
| `WithWatchConfig(watch bool)` | `false` | 是否监听配置文件变化并自动重载 |
| `WithOnConfigChange(fn)` | 无 | 配置重载后执行的回调，见 [热更新](#热更新) |
| `WithAutomaticEnv(auto bool)` | `false` | 读取配置时自动查找对应的环境变量 |
| `WithEnvPrefix(prefix string)` | 空 | 环境变量前缀，仅在启用 `AutomaticEnv` 时生效 |
| `WithEnvKeyReplacer(oldNew ...string)` | 无 | 配置 key 到环境变量名的替换规则，如 `WithEnvKeyReplacer(".", "_")`；仅在启用 `AutomaticEnv` 时生效 |
| `WithDebug(debug bool)` | `false` | 输出加载与重载日志 |
| `WithTagName(name string)` | 空（`mapstructure`） | 解析结构体时匹配字段使用的标签名，如 `json`、`toml`，见 [使用 json / toml 标签](#使用-json--toml-标签) |

```go
conf, err := gconf.New(
    gconf.WithConfigPaths(".", "./config", "/etc/myapp"),
    gconf.WithConfigName("app"),
    gconf.WithConfigType("yaml"),
    gconf.WithAutomaticEnv(true),
    gconf.WithEnvPrefix("MYAPP"),
    gconf.WithEnvKeyReplacer(".", "_"),
    gconf.WithWatchConfig(true),
)
```

## 创建实例

### 独立实例

```go
conf, err := gconf.New(gconf.WithConfigName("app"))
if err != nil {
    return err
}
port := conf.GetInt("server.port")
```

适合读取多个配置文件、或需要显式管理生命周期的场景，每个配置文件创建一个实例。

### 全局实例

```go
if err := gconf.Init(gconf.WithConfigName("app")); err != nil {
    log.Fatal(err)
}
port := gconf.GetInt("server.port")
```

- `Init` 可重复调用，每次都用新选项创建新实例并替换全局实例；旧实例已启动的文件监听不会停止
- `InitWithConfig` 与 `Init` 行为一致，为兼容旧版本保留
- `GetInstance()` 返回全局实例；未调用 `Init` 时使用默认选项自动创建，**此时的创建错误（如配置文件解析失败）会被忽略，实例为 nil，后续调用会 panic**。生产代码应在启动时调用 `Init` 并检查错误
- 包级函数只覆盖常用方法，其余方法通过 `gconf.GetInstance()` 调用，见 [API 速查](#api-速查)

## 读取配置

### key 规则

- 使用 `.` 访问嵌套字段，如 `server.port`
- key **不区分大小写**，`SERVER.PORT` 与 `server.port` 等价
- key 不存在时，`Get` 返回 `nil`，类型化方法返回对应零值，不会报错；需要区分时使用 `IsSet`

### 类型化读取

| 方法 | 返回类型 |
|---|---|
| `Get` | `interface{}` |
| `GetString` | `string` |
| `GetBool` | `bool` |
| `GetInt` / `GetInt32` / `GetInt64` | `int` / `int32` / `int64` |
| `GetUint` / `GetUint32` / `GetUint64` | `uint` / `uint32` / `uint64` |
| `GetFloat64` | `float64` |
| `GetTime` | `time.Time` |
| `GetDuration` | `time.Duration`，如 `"30s"` |
| `GetSizeInBytes` | `uint`，支持 `"10MB"`、`"1GB"` 等 |
| `GetStringSlice` | `[]string` |
| `GetStringMap` | `map[string]interface{}` |
| `GetStringMapString` | `map[string]string` |
| `GetStringMapStringSlice` | `map[string][]string` |

类型化方法会自动转换，如字符串 `"8080"` 可用 `GetInt` 读取；`Get` 返回解析器给出的原始类型。

### 查询

```go
conf.IsSet("server.port") // key 是否存在
conf.AllKeys()            // 所有 key，展开为 a.b.c 形式
conf.AllSettings()        // 全部配置的嵌套 map
```

## 解析到结构体

```go
type Config struct {
    App struct {
        Name string
    }
    Server struct {
        Port    int
        Timeout time.Duration // "30s" 自动转换
        Hosts   []string      // "a,b" 自动拆分
        MaxConn int           `mapstructure:"max_conn"`
    }
}

var cfg Config
if err := conf.Unmarshal(&cfg); err != nil {
    return err
}
```

| 方法 | 说明 |
|---|---|
| `Unmarshal(&v)` | 解析全部配置 |
| `UnmarshalKey("server", &v)` | 解析指定 key 下的配置 |
| `UnmarshalExact(&v)` | 配置中存在结构体未定义的字段时返回错误 |

- 字段名匹配不区分大小写，名称不一致时使用 `mapstructure` tag；只认这一种 tag，`json`、`toml` tag 默认会被忽略
- 结构体是解析时刻的**副本**，配置重载后不会自动更新，需要在变更回调中重新解析，见 [热更新](#热更新)

### 使用 json / toml 标签

结构体已经使用 `json` 或 `toml` tag 时，通过 `WithTagName` 指定标签名，无需改写为 `mapstructure`：

```go
type Config struct {
    ListenAddr string `toml:"listen_addr"`
    DB         struct {
        MaxConn int `toml:"max_conn"`
    } `toml:"db_conf"`
}

conf, err := gconf.New(gconf.WithTagName("toml"))
```

- 对 `Unmarshal`、`UnmarshalKey`、`UnmarshalExact` 生效；全局函数使用 `Init` 传入的选项
- 只改变字段匹配所用的标签，环境变量覆盖、默认值、`"30s"` 转 `time.Duration` 等转换照常生效
- 一个实例只能指定一种标签名，同一结构体中混用多种 tag 时，只有指定的那种生效
- tag 中逗号后的修饰符（如 `omitempty`）被忽略；`json:"-"` 的字段不会被赋值
- 指定 `json` 或 `toml` 时，匿名嵌入的结构体字段会提升到上一层，与 `encoding/json` 及常见 TOML 库一致；以下两种写法与原生库不同：
  - 嵌入字段自身带名称 tag（如 `` `json:"base"` ``）时仍会被提升，原生库则作为名为 `base` 的普通字段
  - 以指针形式嵌入的结构体（如 `*Base`）不会被提升，而是按类型名作为普通字段匹配，原生库则会提升

## 修改配置与优先级

```go
conf.SetDefault("server.port", 8080) // 默认值，优先级最低
conf.Set("server.port", 9090)        // 运行时覆盖，优先级最高
conf.RegisterAlias("port", "server.port")
```

读取时的优先级从高到低：

```text
Set() > 环境变量 > 配置文件 > SetDefault()
```

- `Set` 只修改内存，不写入文件；对某个 key 调用 `Set` 后，配置文件重载不会改变该 key 的读取值
- `RegisterAlias(alias, key)` 注册别名，通过别名读写等同于操作原 key

## 环境变量

### 自动读取

```go
// export MYAPP_SERVER_PORT=9090
conf, _ := gconf.New(
    gconf.WithAutomaticEnv(true),
    gconf.WithEnvPrefix("MYAPP"),
    gconf.WithEnvKeyReplacer(".", "_"),
)
conf.GetInt("server.port") // 9090
```

key `server.port` 经替换为 `SERVER_PORT`，加上前缀后查找环境变量 `MYAPP_SERVER_PORT`。

### 显式绑定

```go
conf.BindEnv("db.password")               // 按前缀与替换规则推导环境变量名
conf.BindEnv("api.token", "CUSTOM_TOKEN") // 指定环境变量名
```

前缀与替换规则只在启用 `AutomaticEnv` 时设置；未启用时，`BindEnv("db.password")` 查找的是 `DB.PASSWORD`，此时应显式指定环境变量名。

### 与 Unmarshal 的关系

`AutomaticEnv` 只在按 key 读取时生效，`Unmarshal` 只包含已知的 key：

| key 的来源 | `GetXxx` 读取 | `Unmarshal` 解析 |
|---|---|---|
| 配置文件中存在，环境变量覆盖 | 环境变量值 | 环境变量值 |
| 仅存在于环境变量 | 环境变量值 | **不包含** |
| 仅存在于环境变量，已 `BindEnv` 或 `SetDefault` | 环境变量值 | 环境变量值 |

需要通过 `Unmarshal` 读取仅由环境变量提供的配置时，对这些 key 调用 `BindEnv` 或设置默认值。

## 热更新

```go
conf, err := gconf.New(
    gconf.WithWatchConfig(true),
    gconf.WithOnConfigChange(func(e fsnotify.Event) {
        log.Printf("配置已重载: %s", e.Name)
    }),
)

// 也可在创建后注册，可注册多个
conf.OnConfigChange(func(e fsnotify.Event) {
    // ...
})
```

### 回调语义

| 回调 | 执行时机 | 执行方式 |
|---|---|---|
| `WithOnConfigChange` | 配置重载完成后 | 在监听 goroutine 中同步执行 |
| `OnConfigChange` 注册的回调 | 配置重载完成后 | 每个回调在独立 goroutine 中执行 |

- 回调只在启用 `WithWatchConfig(true)` 时触发
- 回调执行时重载已完成且锁已释放，可在回调中安全读取配置
- 重载失败（如文件内容解析错误）会输出日志，回调仍会执行

### 并发语义

- 所有读取方法加读锁，`Set`、`SetDefault`、重载等修改操作加写锁
- 重载在写锁内完成，`Unmarshal`、`AllSettings` 不会读到新旧混合的配置
- `GetViper()` 返回的底层实例绕过 gconf 的锁，与热更新并发使用时不保证安全
- `Sub()` 返回的子配置是调用时刻的副本，父配置重载后不会更新

### 业务配置更新建议

对结构体配置，在回调中重新解析并校验，成功后原子替换：

```go
var current atomic.Value // 存放 *Config

reload := func() {
    var cfg Config
    if err := conf.UnmarshalExact(&cfg); err != nil {
        log.Printf("配置无效，保留旧配置: %v", err)
        return
    }
    // 业务字段校验 ...
    current.Store(&cfg)
}
reload()
conf.OnConfigChange(func(fsnotify.Event) { reload() })
```

连接池等资源应先创建并检查新实例，再切换引用，最后关闭旧实例。

## 写入配置文件

| 方法 | 写入位置 | 文件已存在时 |
|---|---|---|
| `WriteConfig()` | 当前加载的文件（`ConfigFileUsed()`） | 覆盖；未加载任何文件时返回错误 |
| `SafeWriteConfig()` | 第一个搜索目录下的 `<ConfigName>.<ConfigType>` | 返回错误，不覆盖 |
| `WriteConfigAs(filename)` | 指定文件 | 覆盖 |
| `SafeWriteConfigAs(filename)` | 指定文件 | 返回错误，不覆盖 |

- 写出格式：`WriteConfig` 与 `*As` 按文件扩展名决定，`SafeWriteConfig` 使用 `ConfigType`。**未指定 `ConfigType` 时 `SafeWriteConfig` 返回错误**，可改用 `SafeWriteConfigAs("config.yaml")`
- 写入内容为 `AllSettings()` 的完整结果，**包括默认值、`Set` 的值和已绑定的环境变量值**，敏感信息可能因此落盘
- 原文件中的注释与 key 顺序不会保留

## 其他功能

```go
// 子配置树：返回以 server 为根的新实例；key 不存在时返回 nil
serverConf := conf.Sub("server")
serverConf.GetInt("port")

// 重新读取配置文件
conf.ReadInConfig()

// 将找到的配置文件合并到当前配置
conf.MergeInConfig()

// 当前加载的配置文件路径
conf.ConfigFileUsed()

// 底层 viper 实例，用于未封装的高级功能；直接操作会绕过 gconf 的锁
conf.GetViper()

// 打印配置文件路径与全部配置，用于调试
conf.Debug()
```

## API 速查

### 实例方法（`*gconf.Gconf`）

| 分类 | 方法 |
|---|---|
| 读取 | `Get`、`GetString`、`GetBool`、`GetInt`、`GetInt32`、`GetInt64`、`GetUint`、`GetUint32`、`GetUint64`、`GetFloat64`、`GetTime`、`GetDuration`、`GetSizeInBytes`、`GetStringSlice`、`GetStringMap`、`GetStringMapString`、`GetStringMapStringSlice` |
| 查询 | `IsSet`、`AllKeys`、`AllSettings`、`ConfigFileUsed` |
| 解析 | `Unmarshal`、`UnmarshalKey`、`UnmarshalExact` |
| 修改 | `Set`、`SetDefault`、`RegisterAlias`、`BindEnv` |
| 文件 | `ReadInConfig`、`MergeInConfig`、`WriteConfig`、`SafeWriteConfig`、`WriteConfigAs`、`SafeWriteConfigAs` |
| 其他 | `OnConfigChange`、`Sub`、`GetViper`、`Debug` |

### 包级函数（操作全局实例）

| 分类 | 函数 |
|---|---|
| 初始化 | `Init`、`InitWithConfig`、`GetInstance` |
| 读取 | `Get`、`GetString`、`GetBool`、`GetInt`、`GetInt32`、`GetInt64`、`GetUint`、`GetUint32`、`GetUint64`、`GetFloat64`、`GetStringSlice`、`GetStringMap`、`GetStringMapString` |
| 查询 | `IsSet`、`AllKeys`、`AllSettings`、`ConfigFileUsed` |
| 解析 | `Unmarshal`、`UnmarshalKey` |
| 修改 | `Set`、`SetDefault` |
| 其他 | `WriteConfig`、`OnConfigChange`、`Debug` |

包级函数未覆盖的方法（如 `GetDuration`、`Sub`、`BindEnv`）通过 `gconf.GetInstance().Xxx()` 调用。

## Apollo 配置中心

`github.com/nicexiaonie/gconf/apollo` 是可选的独立 Go module，负责从 [Apollo](https://www.apolloconfig.com/) 拉取配置并可靠写入本地文件，业务再用 gconf 读取。不引入该模块时，gconf 的依赖与行为不受影响。

```bash
go get github.com/nicexiaonie/gconf/apollo
```

```go
syncer, err := apollo.New(apollo.Config{
    AppID:      "my-app",
    Cluster:    "default",
    MetaServer: "http://apollo-meta:8080",
    CacheDir:   "/var/lib/my-app/apollo",
    Watch:      true,
    Namespaces: []apollo.NamespaceConfig{
        {Name: "application", Required: true},            // 原样落地为 application.properties
        {Name: "config.json", Format: apollo.FormatTOML}, // 转换落地为 config.toml
    },
})
if err != nil {
    log.Fatal(err)
}
if err := syncer.Start(context.Background()); err != nil {
    log.Fatal(err)
}
defer syncer.Close()

// 每个输出文件使用一个 gconf 实例读取
appConf, err := gconf.New(
    gconf.WithConfigPaths("/var/lib/my-app/apollo"),
    gconf.WithConfigName("application"),
    gconf.WithWatchConfig(true),
)
```

- 未配置 `Format` 时原样落地：文件型 Namespace 写 Apollo 原文，properties 型写为 properties 文件
- 支持转换为 JSON、YAML、YML、XML、TOML、properties；无法表达的内容报错，不做自动修正
- 原子发布、内容去重、LKG 降级启动，可选持续热更新
- 端到端热更新需同时启用 Syncer 的 `Watch` 与 gconf 的 `WithWatchConfig(true)`

完整说明见 [apollo/README.md](apollo/README.md)。

## 示例与开发命令

| 目录 | 内容 |
|---|---|
| `example/basic` | 全局实例基础用法 |
| `example/advanced` | 环境变量、热更新与回调等进阶用法 |
| `example/env` | 环境变量覆盖与绑定 |
| `apollo/cmd/demo` | Apollo 同步演示 |

```bash
make test          # 运行测试
make test-verbose  # 带 -race 运行测试
make test-coverage # 生成覆盖率报告
make bench         # 运行基准测试
make example       # 运行全部示例
```

## 常见问题

### 配置文件读不到

1. 用 `ConfigFileUsed()` 确认实际加载的文件，空字符串表示没有找到
2. `ConfigPaths` 中的相对路径相对于进程工作目录
3. 文件无扩展名或扩展名不在支持列表中时，需要 `WithConfigType` 指定类型
4. 同一目录存在多个同名不同扩展名的文件时，按 [查找规则](#查找规则) 的顺序只读第一个

### 环境变量没有生效

1. 确认启用了 `WithAutomaticEnv(true)`
2. 嵌套 key 需要 `WithEnvKeyReplacer(".", "_")` 才能对应 `SERVER_PORT` 形式的变量名
3. 使用前缀时，变量名为 `<前缀>_<KEY>`
4. 通过 `Unmarshal` 读取仅由环境变量提供的配置，需要先 `BindEnv`，见 [与 Unmarshal 的关系](#与-unmarshal-的关系)

### 热更新没有触发

1. 确认启用了 `WithWatchConfig(true)`，且启动时找到了配置文件
2. 回调未执行时，确认回调注册在同一个实例上
3. 值没有变化时，检查该 key 是否被 `Set` 覆盖或被环境变量遮蔽
4. 结构体配置不会自动更新，需要在回调中重新 `Unmarshal`

### 容器中使用

配置文件通过挂载提供，敏感信息通过环境变量注入：

```go
gconf.Init(
    gconf.WithConfigPaths("/etc/myapp"),
    gconf.WithAutomaticEnv(true),
    gconf.WithEnvPrefix("MYAPP"),
    gconf.WithEnvKeyReplacer(".", "_"),
)
```

### 是否支持 etcd、Consul 等远程配置

gconf 未封装 Viper 的远程配置能力，可通过 `GetViper()` 使用，但直接操作底层实例会绕过 gconf 的并发保护。Apollo 推荐使用 [apollo 子模块](#apollo-配置中心)。

## 版本与变更

版本历史与不兼容变更见 [CHANGELOG.md](CHANGELOG.md)。

## 贡献

欢迎提交 Issue 与 Pull Request，参见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 许可证

本项目采用 MIT 许可证，详见 [LICENSE](LICENSE)。

## 致谢

本项目基于 [spf13/viper](https://github.com/spf13/viper) 构建。

## 联系方式

- GitHub: [https://github.com/nicexiaonie/gconf](https://github.com/nicexiaonie/gconf)
- Issues: [https://github.com/nicexiaonie/gconf/issues](https://github.com/nicexiaonie/gconf/issues)
