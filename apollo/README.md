# Apollo 配置同步模块

`github.com/nicexiaonie/gconf/apollo` 是 gconf 的可选 Apollo 配置中心同步模块。它作为**独立 Go module** 发布，与 gconf 主 module 互不依赖，仅通过本地配置文件协作。

该模块负责从 Apollo 拉取配置、按配置原样或转换格式后可靠落盘；业务程序再使用 gconf 读取本地文件。未引入本模块时，gconf 的依赖和行为均不发生变化。

## 功能概览

- 按 Namespace 拉取 Apollo 完整配置
- 默认每个 Namespace 独立发布、独立降级
- 可选热更新，默认关闭
- 未配置目标格式时原样落地：文件型写 Apollo 原文，properties 型写为 properties 文件
- 支持同一 Namespace 同时输出多种格式
- 支持转换为 JSON、YAML、YML、XML、TOML、properties
- TXT 与带 `.properties` 后缀的文件型 Namespace 只能原样落地
- 版本文件、稳定 symlink 和索引均采用原子发布策略
- 使用目标文件 SHA-256 去重，无内容变化时不重复生成配置版本
- 保存 Apollo `releaseKey`，用于发布追踪和故障排查
- 支持 LKG（Last Known Good，最后一次已知有效配置）降级启动
- 提供 `Status()` 查询同步来源、目标格式、文件 hash、releaseKey 和最近错误

> 多 Namespace 合并为单文件的 `Merge` 模式当前尚未实现。默认且推荐的方式是每个 Namespace 独立发布。

## 运行架构

```text
Apollo Config Service
        │
        │ 首次拉取 / HTTP 长轮询变更通知
        ▼
apollo Syncer
  ├─ 原样落地：校验原文可解析（可关闭）
  ├─ 转换落地：解析原文并转换为目标格式
  ├─ 计算目标文件 SHA-256
  ├─ 写入版本文件并 fsync
  ├─ 原子切换稳定 symlink
  └─ 写入格式独立的 LKG 索引
        │
        ▼
本地配置文件
        │
        │ gconf + fsnotify
        ▼
业务程序
```

同步模块和 gconf 各自负责不同边界：

| 组件 | 职责 |
|---|---|
| Apollo 服务端 | 配置管理、发布和变更通知 |
| apollo Syncer | 远端同步、格式转换、去重、原子落盘和 LKG |
| gconf | 本地文件读取、类型转换、环境变量覆盖和文件热重载 |
| 业务程序 | 配置语义校验、结构体快照切换和资源重建 |

## 环境要求与安装

- Go 1.20 或更高版本
- 可访问的 Apollo Meta Server 或 Config Service

```bash
go get github.com/nicexiaonie/gconf/apollo
```

模块边界：

- `apollo/` 自带 `go.mod`
- Apollo SDK 和 XML 转换依赖只存在于 apollo module
- gconf 主 module 不 import apollo，主 module 的 `go.sum` 不包含 agollo 和 mxj

## 快速开始

### 仅首次同步

`Watch` 默认是 `false`。以下配置只在 `Start()` 时拉取并落盘一次：

```go
syncer, err := apollo.New(apollo.Config{
    AppID:      "my-app",
    Cluster:    "default",
    MetaServer: "http://apollo-meta:8080",
    CacheDir:   "/var/lib/my-app/apollo",
    Namespaces: []apollo.NamespaceConfig{
        // 未配置 Format：原样落地为 application.properties
        {Name: "application", Required: true},
    },
})
if err != nil {
    return err
}

if err := syncer.Start(context.Background()); err != nil {
    return err
}
defer syncer.Close()
```

### 启用持续热更新

设置 `Watch: true` 后，`Start()` 会先完成首次同步，再注册 Apollo 变更监听：

```go
syncer, err := apollo.New(apollo.Config{
    AppID:      "my-app",
    Cluster:    "default",
    MetaServer: "http://apollo-meta:8080",
    CacheDir:   "/var/lib/my-app/apollo",
    Watch:      true,
    Namespaces: []apollo.NamespaceConfig{
        {Name: "config.json", Required: true, Format: apollo.FormatYAML},
    },
})
if err != nil {
    return err
}

if err := syncer.Start(context.Background()); err != nil {
    return err
}
defer syncer.Close()
```

`Start()` 返回成功后，热更新已运行，不需要调用其他同步方法。调用 `Close()` 后停止监听并关闭 Apollo 客户端。

## 配置说明

### `Config`

| 字段 | 必填 | 默认值 | 说明 |
|---|---:|---|---|
| `AppID` | 是 | — | Apollo 应用 ID |
| `Cluster` | 否 | 空 | Apollo 集群名称，常用值为 `default` |
| `MetaServer` | 是 | — | Apollo Meta Server 或 Config Service 地址 |
| `Secret` | 否 | 空 | Apollo AccessKey Secret，仅用于请求签名；不会加密本地文件 |
| `CacheDir` | 是 | — | 配置快照、稳定文件和 LKG 索引的持久化根目录 |
| `Namespaces` | 是 | — | 至少配置一个 Namespace 输出项 |
| `Watch` | 否 | `false` | 是否在首次同步后持续监听 Apollo 变更 |
| `Merge` | 否 | `false` | 多 Namespace 合并开关；当前尚未实现 |
| `Retry` | 否 | 零值 | 预留重试配置；当前尚未接入同步流程 |

### `NamespaceConfig`

| 字段 | 必填 | 默认值 | 说明 |
|---|---:|---|---|
| `Name` | 是 | — | Apollo Namespace 名称 |
| `Required` | 否 | `false` | 远端不可用且该目标格式没有 LKG 时，是否使 `Start()` 失败 |
| `Format` | 否 | 空 | 目标格式。为空或与源格式相同时原样落地；与源格式不同时解析后转换 |
| `Filename` | 否 | 空 | 输出文件名，严格按配置写入，不校验扩展名；为空时由 Namespace 基名和目标格式生成 |
| `SkipValidate` | 否 | `false` | 原样落地前跳过解析检查；仅适用于原样落地，与转换同时配置会校验失败 |

### `Required` 的准确语义

`Required` 只影响**首次启动时没有可用配置**的情况：

| 远端配置 | 对应格式 LKG | `Required` | 结果 |
|---|---|---:|---|
| 成功 | 任意 | 任意 | 使用远端配置，`Start()` 成功 |
| 失败 | 存在且有效 | 任意 | 使用 LKG 降级，`Start()` 成功 |
| 失败 | 不存在 | `false` | 状态为 `empty`，`Start()` 继续 |
| 失败 | 不存在 | `true` | `Start()` 返回错误 |

同一个 Namespace 输出 YAML 和 XML 时，两个格式分别维护 LKG；其中一个格式没有 LKG，不代表另一个格式也不可用。

以上仅针对远端不可用。**配置内容错误**（原文无法解析、转换失败）属于人为配置问题，首次启动时无论 `Required` 与 LKG 如何，`Start()` 都返回错误，且不会覆盖已有文件；运行中热更新遇到内容错误时不写入，错误记录在 `Status().LastError`。

## Namespace 与格式规则

### Apollo Namespace 类型

Apollo 中：

- properties 型 Namespace 通常没有格式后缀，例如 `application`
- 文件型 Namespace 使用名称加后缀，例如 `config.json`、`rules.yaml`、`mapping.xml`、`notice.txt`

模块根据 Namespace 后缀识别源格式，规则与 Apollo/agollo 一致：仅 `.json`、`.yaml`、`.yml`、`.xml`、`.txt`、`.properties` 视为文件型；其余带点名称（如公共 Namespace 的部门前缀 `TEST1.redis`）按 properties 型处理，输出文件名保留完整名称，例如 `TEST1.redis.properties`。

### 落地规则

| 配置 | 落地方式 |
|---|---|
| 未配置 `Format`，或与源格式相同 | **原样落地**：文件型写 Apollo 原文（注释、顺序均保留）；properties 型按 key 排序写为 `key=value` |
| 配置了与源格式不同的 `Format` | **转换落地**：解析原文后按目标格式生成 |

原样落地前默认检查原文能否按源格式解析（JSON/YAML/YML/XML），失败则不写入并报错；`SkipValidate: true` 时跳过检查，原文照写，问题由读取方暴露。TXT 与带 `.properties` 后缀的文件型 Namespace 不做检查。

### 可转换的源与目标

| 源 Namespace | 原样落地格式 | 可转换为 |
|---|---|---|
| properties 型（无后缀或非格式后缀） | properties | JSON、YAML、YML、XML、TOML |
| `.json` / `.yaml` / `.yml` / `.xml` | 源格式 | JSON、YAML、YML、XML、TOML、properties |
| `.txt` | txt | 不可转换 |
| `.properties` 文件型 | properties | 不可转换 |

```go
apollo.FormatJSON       // .json
apollo.FormatYAML       // .yaml
apollo.FormatYML        // .yml
apollo.FormatXML        // .xml
apollo.FormatTOML       // .toml
apollo.FormatProperties // .properties
apollo.FormatTXT        // .txt，仅适用于原样落地
```

例如：

```go
Namespaces: []apollo.NamespaceConfig{
    // config.json 原样落地为 config.json
    {Name: "config.json"},

    // config.json 转换为 TOML，落地为 config.toml
    {Name: "config.json", Format: apollo.FormatTOML},

    // application 原样落地为 application.properties
    {Name: "application"},

    // application 转换为 YAML，并自定义文件名
    {Name: "application", Format: apollo.FormatYAML, Filename: "app.yaml"},
}
```

非法组合在 `New()` 时直接报错：未知目标格式、TXT/`.properties` 文件型配置转换、转换为 TXT、转换时配置 `SkipValidate`。

### 转换限制

转换遇到目标格式无法表达的内容时报错，不做任何自动修正：

| 目标格式 | 报错的情况 |
|---|---|
| TOML | `null`；数组元素类型不一致（包括整数与小数混合）；超出 int64 的整数 |
| properties | 数组；`null`；嵌套展开后 key 重复；key 含 `=` 等无法正确读回的字符 |

JSON 源按原文区分整数与小数：`8080` 保持整数，大整数不丢精度。

### 文件名

未配置 `Filename` 时，文件名为 Namespace 去掉格式后缀后加目标格式扩展名，例如 `config.json` 转 TOML 为 `config.toml`，`TEST1.redis` 原样落地为 `TEST1.redis.properties`。

配置 `Filename` 时严格按配置写入，不校验扩展名与内容是否一致；只要求不含目录且多个输出项不重名。扩展名与内容不一致或无扩展名时，gconf 需通过 `WithConfigType` 指定类型，否则可能读不到配置。

### XML 转换约定

XML 必须存在单一根节点。结构化 map 转为 XML 时，多个顶层字段会被包装在 `<doc>` 根节点下，并以四个空格缩进输出：

```xml
<doc>
    <app>
        <name>demo</name>
    </app>
</doc>
```

XML 再由 gconf/Viper 读取时，应根据实际解析结构确认根节点路径。

## 同一 Namespace 输出多种格式

同一个远端 Namespace 只订阅一次，但可以配置多个输出项：

```go
Namespaces: []apollo.NamespaceConfig{
    {Name: "config.json", Required: true, Format: apollo.FormatYAML},
    {Name: "config.json", Required: true, Format: apollo.FormatXML},
},
```

首次同步和每次热更新都会从同一份结构化配置生成：

```text
<CacheDir>/config.json/config.yaml
<CacheDir>/config.json/config.xml
```

每种目标格式独立维护：

- 目标文件 hash
- 稳定 symlink
- 版本文件
- LKG 索引
- `Status()` 状态项

## 本地目录与文件契约

以 `config.json` 同时输出 YAML 和 XML 为例：

```text
<CacheDir>/
  config.json/
    config.yaml -> snapshots/config-<yaml-hash>.yaml
    config.xml  -> snapshots/config-<xml-hash>.xml
    index.yaml.json
    index.xml.json
    snapshots/
      config-<yaml-hash>.yaml
      config-<xml-hash>.xml
```

各文件用途：

| 文件 | 用途 |
|---|---|
| `config.yaml` / `config.xml` | 业务读取的稳定路径；是指向当前有效版本的 symlink |
| `snapshots/*` | 不可变完整版本文件；文件名包含目标文件 SHA-256 |
| `index.<format>.json` | 对应格式的 LKG 元数据，记录 hash、format 和 releaseKey |

`CacheDir` 是持久化目录，不是进程内的易失缓存。进程退出不会自动删除；只有外部删除、临时目录清理或容器文件系统重建才会丢失。

容器部署时应把 `CacheDir` 挂载到持久卷，否则 Pod 重建后无法使用 LKG。

### 原子发布顺序

内容变化时：

1. 生成目标文件内容：原样落地取原文并按需检查解析；转换落地解析、转换并验证可解析回来
2. 计算实际目标文件字节的 SHA-256
3. 写入不可变版本文件
4. 对版本文件执行 fsync
5. 使用临时 symlink + rename 原子切换稳定路径
6. fsync Namespace 目录
7. 使用临时文件 + rename 原子更新格式索引

内容 hash 不变、仅 releaseKey 改变时：

- 不创建新版本文件
- 不切换稳定 symlink
- 只更新对应格式的 LKG 索引和状态

## LKG 降级

LKG 是 Last Known Good，即“最后一次已知有效配置”。它由当前稳定文件、版本文件和格式索引共同组成。

启动时，每个 `Namespace + Format` 独立执行：

```text
读取对应格式 LKG
  ├─ 存在且 hash 校验通过：作为降级候选
  └─ 不存在或校验失败：无降级候选
        ↓
拉取 Apollo 最新配置
  ├─ 成功且内容有效：发布远端配置，Source=remote
  ├─ 成功但内容错误：不写入，Start() 返回错误（与 LKG、Required 无关）
  ├─ 失败且有 LKG：保留 LKG，Source=lkg
  └─ 失败且无 LKG：Source=empty，按 Required 决定是否报错
```

LKG 保证可用性，不保证配置一定是最新版本。关键安全策略、计费规则等不能长期使用旧值的配置，应由业务增加最大陈旧时间和告警策略。

## 热更新

### Syncer 热更新

`Watch: true` 时：

1. `Start()` 完成首次同步
2. 注册 agollo 变更监听
3. Apollo 发布后，agollo 长轮询收到通知
4. listener 获取完整 Namespace 快照及当前 releaseKey
5. 串行 worker 处理变更
6. 为所有同名输出项分别转换、去重和原子落盘

`Watch: false` 时只执行首次同步，后续 Apollo 发布不会更新本地文件。

### gconf 文件热重载

Syncer 的 `Watch` 和 gconf 的 `WithWatchConfig` 是两层独立开关：

| Syncer `Watch` | gconf `WithWatchConfig` | 效果 |
|---:|---:|---|
| `false` | `false` | 仅启动时同步一次，gconf 读取一次 |
| `true` | `false` | 本地文件持续更新，但已有 gconf 实例不自动重载 |
| `false` | `true` | gconf 监听本地文件，但 Syncer 不产生后续远端更新 |
| `true` | `true` | Apollo 发布后，同步到本地并由 gconf 自动重载 |

需要完整端到端热更新时，两个开关都必须启用。

## 与 gconf 集成

### properties 型 Namespace

```go
// application -> <CacheDir>/application/application.yaml
syncer, err := apollo.New(apollo.Config{
    AppID:      "my-app",
    MetaServer: "http://apollo-meta:8080",
    CacheDir:   "/var/lib/my-app/apollo",
    Watch:      true,
    Namespaces: []apollo.NamespaceConfig{
        {Name: "application", Required: true, Format: apollo.FormatYAML},
    },
})

conf, err := gconf.New(
    gconf.WithConfigPaths("/var/lib/my-app/apollo/application"),
    gconf.WithConfigName("application"),
    gconf.WithConfigType("yaml"),
    gconf.WithWatchConfig(true),
)
```

### 文件型 Namespace 转格式

```go
// config.json -> <CacheDir>/config.json/config.yaml
Namespaces: []apollo.NamespaceConfig{
    {Name: "config.json", Required: true, Format: apollo.FormatYAML},
}

conf, err := gconf.New(
    gconf.WithConfigPaths("/var/lib/my-app/apollo/config.json"),
    gconf.WithConfigName("config"),
    gconf.WithConfigType("yaml"),
    gconf.WithWatchConfig(true),
)
```

### 多 Namespace

每个 Namespace 使用独立 gconf 实例：

```go
appConf, _ := gconf.New(
    gconf.WithConfigPaths("/var/lib/my-app/apollo/application"),
    gconf.WithConfigName("application"),
    gconf.WithConfigType("yaml"),
    gconf.WithWatchConfig(true),
)

datasourceConf, _ := gconf.New(
    gconf.WithConfigPaths("/var/lib/my-app/apollo/datasource"),
    gconf.WithConfigName("datasource"),
    gconf.WithConfigType("json"),
    gconf.WithWatchConfig(true),
)
```

不要把 `ConfigPaths` 理解为“读取目录下所有文件”；一个 gconf 实例一次读取一个配置文件，多 Namespace 应创建多个实例。

## `Status()` 状态查询

`Status()` 只读取同步器当前的内存状态，不访问 Apollo、不读取磁盘，也不触发同步。它是监控和诊断接口，不是同步入口。

```go
status := syncer.Status()
for _, item := range status.Namespaces {
    log.Printf(
        "namespace=%s format=%s source=%s releaseKey=%s hash=%s lastSuccess=%s lastError=%s",
        item.Namespace,
        item.Format,
        item.Source,
        item.ReleaseKey,
        item.Hash,
        item.LastSuccess,
        item.LastError,
    )
}
```

字段定义：

| 字段 | 说明 |
|---|---|
| `Namespace` | Apollo Namespace 名称 |
| `Format` | 该状态项对应的目标文件格式 |
| `ReleaseKey` | Apollo 发布版本标识，仅用于追踪和排障，不参与内容去重 |
| `Hash` | 实际目标文件字节的 SHA-256，与对应稳定文件内容一致 |
| `Source` | 当前配置来源：`remote`、`lkg` 或 `empty` |
| `LastSuccess` | 最近一次成功发布远端配置的时间 |
| `LastError` | 最近一次拉取或发布错误；下一次成功后清空 |

`Source` 语义：

| 值 | 含义 |
|---|---|
| `remote` | 已从 Apollo 成功拉取并发布当前配置 |
| `lkg` | 本次远端不可用，当前使用本地最后有效配置 |
| `empty` | 没有远端配置，也没有可用 LKG |

同一 Namespace 输出多种格式时，`Status().Namespaces` 会返回多条记录，通过 `Format` 区分。

如果业务不需要健康检查、日志或指标，可以不调用 `Status()`；不会影响同步和热更新。

## releaseKey 与内容 hash

两者职责不同：

- **内容 hash**：主版本依据。针对实际目标文件字节计算，用于去重、文件完整性验证和 LKG 校验。
- **releaseKey**：辅助元数据。表示 Apollo 服务端发布版本，用于日志、状态展示和故障定位。

设计规则：

- releaseKey 变化但内容 hash 不变：只更新索引，不生成新配置版本
- 内容 hash 变化：生成新版本文件并原子切换
- releaseKey 暂时获取不到：允许为空，不影响同步正确性
- 同一远端配置输出 YAML/XML：releaseKey 通常相同，目标文件 hash 不同

## 业务配置更新建议

`gconf.GetString` 等 getter 会在下一次调用时读取 gconf 当前配置；`Unmarshal` 得到的结构体是一次性副本，不会自动变化。

复杂配置建议：

1. 在 gconf 文件变更回调中创建新的局部结构体
2. 执行 `UnmarshalExact` 或 `Unmarshal`
3. 做业务字段、范围和兼容性校验
4. 校验成功后通过 `atomic.Value` 或锁原子替换业务快照
5. 校验失败时保留上一份业务配置并告警

连接池、HTTP 客户端等资源应先创建和健康检查新实例，再切换引用，最后关闭旧实例。

## 配置优先级与遮蔽

gconf 读取时底层优先级为：

```text
Set() > 环境变量 > 本地配置文件 > SetDefault()
```

因此：

- 对某个 key 调用 `gconf.Set()` 后，Apollo 对该 key 的后续修改不会成为最终读取值
- 同名环境变量会遮蔽 Apollo 落盘文件中的值

排查“Apollo 文件已更新但业务值没变”时，应先检查 `Set()` 和环境变量。

## 生命周期

```go
syncer, err := apollo.New(cfg) // 只校验配置和构造对象，不连接远端
if err != nil {
    return err
}

if err := syncer.Start(ctx); err != nil { // 首次同步；Watch=true 时同时启动监听
    return err
}

defer syncer.Close() // 幂等关闭
```

约束：

- 一个 Syncer 实例只应调用一次 `Start()`
- `Close()` 可重复调用
- `Start()` 成功后应在进程退出时调用 `Close()`
- `context.Context` 当前用于 API 形态，尚未驱动内部 agollo 取消；停止同步应调用 `Close()`

## 安全与运维

- `Secret` 不应硬编码或写入日志，应通过环境变量或 Secret Manager 注入
- Apollo AccessKey 只保护请求认证，不加密本地快照
- `CacheDir` 可能包含数据库地址、令牌等敏感配置，应限制目录和文件权限
- 容器中应把 `CacheDir` 挂载到持久卷
- 不要把 `CacheDir` 放在系统会自动清理的临时目录
- 监控 `Status().Source`、`LastError`、`LastSuccess` 和 releaseKey
- 跨 Namespace 独立发布，不保证多个 Namespace 原子同时生效
- Apollo 配置传播属于最终一致，不应作为分布式锁或事务协调机制

## 常见问题

### `Start()` 是否已经包含热更新？

当 `Watch: true` 时是。`Start()` 先完成首次同步，再注册变更监听；返回成功后热更新已经运行。`Watch: false` 时仅同步一次。

### `Status()` 是否触发同步？

不会。它只返回内存中的状态副本。

### 为什么 Namespace 目录名和文件名看起来重复？

目录用于隔离该 Namespace 的 stable 文件、历史版本和多个格式索引；文件名用于 gconf 选择具体目标格式。例如：

```text
config.json/config.yaml
```

目录名标识 Apollo Namespace，文件名标识目标输出。

### 为什么 XML 多一层 `<doc>`？

XML 只能有一个根节点。结构化配置存在多个顶层 key 时，转换器使用 `<doc>` 包装。

### 为什么修改 Apollo 后文件没有更新？

依次检查：

1. Syncer 是否配置 `Watch: true`
2. Apollo 是否已发布而不只是保存草稿
3. `Status().LastError` 是否有错误
4. releaseKey 或 hash 是否变化
5. 如果文件已更新但 gconf 未更新，检查 `WithWatchConfig(true)`
6. 如果 gconf 已更新但业务值未变，检查 `Set()`、环境变量和业务结构体缓存

### 为什么 releaseKey 变化但没有新的版本文件？

因为最终文件内容没有变化。模块使用目标文件内容 hash 去重，此时只更新格式索引中的 releaseKey。

## 已知限制

- `Merge=true` 当前未实现
- `Retry` 当前为预留字段，尚未接入同步流程
- `.txt` 和带 `.properties` 后缀的文件型 Namespace 只能原样落地，不支持转格式
- gconf（Viper）读取 properties 文件时会展开 `${key}` 占位符：引用存在的 key 会被替换，引用不存在的 key 会被替换为空字符串，自引用报错。Apollo properties 型 Namespace 的值含 `${...}` 时，原样落地后经 gconf 读到的值会与 Apollo 中不同，可为该 Namespace 配置 `Format: apollo.FormatJSON` 等格式规避
- 为保留 yaml/yml Namespace 的原文，模块在 agollo 中将 yaml/yml 解析器替换为不预解析；该替换对整个进程生效，同一进程内直接使用 agollo 按 key 读取 yaml Namespace 的代码将无法取值
- 跨 Namespace 非原子
- 跨实例配置传播为最终一致
- 历史快照自动清理尚未实现
