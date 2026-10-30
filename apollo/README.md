# Apollo 配置同步模块

`github.com/nicexiaonie/gconf/apollo` 是 gconf 的可选 Apollo 配置中心同步模块。它作为**独立 Go module** 发布，与 gconf 主 module 互不依赖，仅通过本地配置文件协作：本模块负责从 Apollo 拉取配置并可靠写入本地文件，业务程序再用 gconf 读取。

## 目录

- [功能概览](#功能概览)
- [安装](#安装)
- [快速开始](#快速开始)
- [配置说明](#配置说明)
- [Namespace 类型](#namespace-类型)
- [落地规则](#落地规则)
- [格式转换](#格式转换)
- [输出文件名](#输出文件名)
- [配置校验](#配置校验)
- [启动与错误处理](#启动与错误处理)
- [热更新](#热更新)
- [本地目录结构](#本地目录结构)
- [与 gconf 集成](#与-gconf-集成)
- [Status 状态查询](#status-状态查询)
- [生命周期](#生命周期)
- [安全与运维](#安全与运维)
- [常见问题](#常见问题)
- [已知限制](#已知限制)

## 功能概览

- 按 Namespace 拉取 Apollo 完整配置，每个输出项独立发布、独立降级
- **原样落地**：未配置目标格式时，文件型 Namespace 写入 Apollo 原文，properties 型 Namespace 写为 properties 文件
- **格式转换**：可转换为 JSON、YAML、YML、XML、TOML、properties；无法表达的内容报错，不做自动修正
- 同一 Namespace 可同时输出多种格式
- 原子发布：版本文件 + 稳定 symlink + fsync，中断不会留下半写文件；只保留当前版本，不累积历史版本
- 首次拉取可配置重试，`Start(ctx)` 可通过 ctx 取消或设置超时
- 按目标文件 SHA-256 去重，内容未变化时不生成新版本
- LKG（Last Known Good，最后一次已知有效配置）降级启动
- 可选持续热更新，默认关闭
- `Status()` 查询同步来源、目标格式、文件 hash、releaseKey 与最近错误

## 安装

```bash
go get github.com/nicexiaonie/gconf/apollo
```

- 要求 Go 1.21 及以上
- 依赖 Apollo 官方 Go SDK [agollo](https://github.com/apolloconfig/agollo) v5
- 需要可访问的 Apollo Meta Server 或 Config Service
- 依赖只存在于本模块，gconf 主 module 的 `go.sum` 不包含 agollo

## 快速开始

### 仅首次同步

`Watch` 默认为 `false`，以下配置只在 `Start()` 时拉取并落盘一次：

```go
syncer, err := apollo.New(apollo.Config{
    AppID:      "my-app",
    Cluster:    "default",
    MetaServer: "http://apollo-meta:8080",
    CacheDir:   "/var/lib/my-app/apollo",
    Namespaces: []apollo.NamespaceConfig{
        // 未配置 Format：原样落地为 /var/lib/my-app/apollo/application.properties
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

### 持续热更新

设置 `Watch: true` 后，`Start()` 先完成首次同步，再注册 Apollo 变更监听：

```go
syncer, err := apollo.New(apollo.Config{
    AppID:      "my-app",
    Cluster:    "default",
    MetaServer: "http://apollo-meta:8080",
    CacheDir:   "/var/lib/my-app/apollo",
    Watch:      true,
    Namespaces: []apollo.NamespaceConfig{
        // config.json 转换为 TOML，落地为 /var/lib/my-app/apollo/config.toml
        {Name: "config.json", Required: true, Format: apollo.FormatTOML},
    },
})
```

`Start()` 返回成功后热更新已在运行，无需调用其他同步方法。

## 配置说明

### `Config`

| 字段 | 必填 | 默认值 | 说明 |
|---|---:|---|---|
| `AppID` | 是 | — | Apollo 应用 ID |
| `Cluster` | 否 | 空 | Apollo 集群名称，通常为 `default` |
| `MetaServer` | 是 | — | Apollo Meta Server 或 Config Service 地址 |
| `Secret` | 否 | 空 | Apollo AccessKey Secret，仅用于请求签名，不加密本地文件 |
| `CacheDir` | 是 | — | 输出文件、版本文件与 LKG 索引的持久化根目录 |
| `Namespaces` | 是 | — | 输出项列表，至少一个 |
| `Watch` | 否 | `false` | 首次同步后是否持续监听 Apollo 变更 |
| `Merge` | 否 | `false` | 预留字段，**当前无任何效果**，设置为 `true` 也不会报错 |
| `Retry` | 否 | 零值（不重试） | 首次拉取的重试策略，见 [首次拉取重试](#首次拉取重试) |

### `NamespaceConfig`

每个 `NamespaceConfig` 是一个输出项，同一 Namespace 可配置多个输出项以输出多种格式。

| 字段 | 必填 | 默认值 | 说明 |
|---|---:|---|---|
| `Name` | 是 | — | Apollo Namespace 名称 |
| `Required` | 否 | `false` | Apollo 不可用且没有 LKG 时，是否让 `Start()` 失败，见 [启动与错误处理](#启动与错误处理) |
| `Format` | 否 | 空 | 目标格式。为空或与源格式相同时原样落地；与源格式不同时解析后转换 |
| `Filename` | 否 | 空 | 输出文件名，严格按配置写入，不校验扩展名；为空时自动生成，见 [输出文件名](#输出文件名) |
| `SkipValidate` | 否 | `false` | 原样落地前跳过解析检查；仅适用于原样落地 |

### 格式常量

```go
apollo.FormatJSON       // json
apollo.FormatYAML       // yaml
apollo.FormatYML        // yml，内容同 YAML
apollo.FormatXML        // xml
apollo.FormatTOML       // toml，仅作为转换目标
apollo.FormatProperties // properties
apollo.FormatTXT        // txt，仅适用于原样落地
```

## Namespace 类型

Apollo 的 Namespace 分为两类，本模块按名称后缀区分，规则与 Apollo 及 agollo 一致：

| 类型 | 判断规则 | 示例 | 从 Apollo 取得的内容 |
|---|---|---|---|
| 文件型 | 名称后缀为 `.json`、`.yaml`、`.yml`、`.xml`、`.txt`、`.properties` 之一 | `config.json`、`rules.yaml`、`notice.txt` | 整段原文 |
| properties 型 | 其余所有名称，包括带点但后缀不是上述格式的名称 | `application`、`TEST1.redis` | 一组键值对 |

- 文件型的**源格式**为名称后缀，properties 型的源格式为 `properties`
- 公共 Namespace 常带部门前缀（如 `TEST1.redis`），仍属于 properties 型
- properties 型在 Apollo 中逐项存储，不存在原文；落地时按 key 排序写出，Apollo 后台中的配置项顺序与备注不会保留

## 落地规则

| 配置 | 落地方式 |
|---|---|
| 未配置 `Format`，或 `Format` 与源格式相同 | **原样落地** |
| `Format` 与源格式不同 | **转换落地** |

### 原样落地

| 源 | 写入内容 |
|---|---|
| 文件型 | Apollo 原文，逐字节一致，注释、key 顺序、缩进均保留 |
| properties 型 | 按 key 排序的 `key=value` 文件，必要时按 properties 规则转义 |

写入前默认按源格式检查原文能否解析：

| 源格式 | 默认检查 | `SkipValidate: true` |
|---|---|---|
| JSON、YAML、YML、XML | 解析失败则不写入并报错 | 跳过检查，原文照写，问题由读取方暴露 |
| TXT、`.properties` 文件型 | 不检查 | 不检查 |
| properties 型 | 写出后逐项读回比对，不一致则报错 | 同左 |

### 转换落地

解析源内容后按目标格式生成，并验证生成结果能按目标格式解析回来。规则见 [格式转换](#格式转换)。

## 格式转换

### 源与目标

| 源 | 原样落地格式 | 可转换为 |
|---|---|---|
| properties 型 | properties | JSON、YAML、YML、XML、TOML |
| `.json`、`.yaml`、`.yml`、`.xml` | 源格式 | JSON、YAML、YML、XML、TOML、properties |
| `.txt` | txt | 不可转换 |
| `.properties` 文件型 | properties | 不可转换 |

### 转换限制

遇到目标格式无法表达的内容时报错，不做任何自动修正：

| 目标格式 | 报错的情况 |
|---|---|
| TOML | `null`；数组元素类型不一致（包括整数与小数混合）；超出 int64 范围的整数 |
| properties | 数组；`null`；嵌套展开后 key 重复；key 含 `=` 等写出后无法正确读回的字符 |

### 转换细节

- **JSON 数字**：按原文区分整数与小数，`8080` 保持整数，大整数不丢精度
- **properties 展开**：嵌套对象展开为点分 key，如 `{"server": {"port": 8080}}` 写为 `server.port=8080`
- **properties 型的值**：Apollo 中的值均为字符串，转换为 JSON、YAML 等格式后仍是字符串
- **XML 根节点**：XML 只能有一个根节点。只有一个顶层 key 时以该 key 为根；多个顶层 key 时以 `<doc>` 包装：

```xml
<doc>
    <a>1</a>
    <b>2</b>
</doc>
```

- **注释**：转换落地基于解析后的数据重新生成，原文中的注释与 key 顺序不保留

## 输出文件名

| 配置 | 文件名 |
|---|---|
| 未配置 `Filename` | Namespace 名去掉格式后缀，加目标格式扩展名 |
| 配置了 `Filename` | 严格按配置写入，不校验扩展名与内容是否一致 |

自动生成示例：

| Namespace | `Format` | 文件名 |
|---|---|---|
| `application` | 空 | `application.properties` |
| `TEST1.redis` | 空 | `TEST1.redis.properties` |
| `config.json` | 空 | `config.json` |
| `config.json` | `FormatTOML` | `config.toml` |
| `rules.yml` | 空 | `rules.yml` |

配置 `Filename` 时的约束：

- 只能是文件名，不能包含目录，不能是 `.` 或 `..`
- 所有输出项写入同一个 `CacheDir`，文件名不能重复

扩展名与内容不一致或没有扩展名时，gconf 需要通过 `WithConfigType` 指定类型，否则读不到配置。

## 配置校验

`New()` 只校验配置、不连接 Apollo。以下情况直接返回错误：

| 校验项 | 示例 |
|---|---|
| `AppID`、`MetaServer`、`CacheDir` 为空 | — |
| 未配置 Namespace，或 `Name` 为空 | — |
| 未知目标格式 | `Format: "ini"` |
| TXT 或 `.properties` 文件型配置转换 | `{Name: "notice.txt", Format: FormatJSON}` |
| 转换为 TXT | `{Name: "config.json", Format: FormatTXT}` |
| 配置转换的同时设置 `SkipValidate` | `{Name: "config.json", Format: FormatTOML, SkipValidate: true}` |
| 同一 Namespace 的同一目标格式重复配置 | 两个 `{Name: "config.json", Format: FormatYAML}` |
| `Filename` 含目录 | `Filename: "../config.json"` |
| 多个输出项文件名重复 | 两个输出项都生成 `config.yaml` |

## 启动与错误处理

### 错误分类

| 错误类型 | 示例 | `Start()` 时 | 热更新时 |
|---|---|---|---|
| 配置内容错误 | 原文无法解析；转换遇到无法表达的内容 | **一律失败**，与 `Required`、LKG 无关；不覆盖已有文件 | 不写入，记录到 `LastError` |
| Apollo 不可用 | 网络故障、Namespace 不存在 | 按下表 `Required` 与 LKG 决定 | 不处理，保留当前文件 |
| 本地写入失败 | 磁盘满、无权限 | 按下表 `Required` 与 LKG 决定 | 记录到 `LastError` |

配置内容错误是人为发布的错误配置，在启动阶段暴露，由人修正 Apollo 上的配置。

### `Required` 与 LKG

仅适用于 Apollo 不可用与本地写入失败：

| 远端配置 | 该输出项的 LKG | `Required` | 结果 |
|---|---|---:|---|
| 成功 | 任意 | 任意 | 使用远端配置，`Source=remote` |
| 失败 | 存在且有效 | 任意 | 使用 LKG 降级，`Source=lkg`，远端错误记录到 `LastError`，`Start()` 成功 |
| 失败 | 不存在 | `false` | `Source=empty`，`Start()` 继续 |
| 失败 | 不存在 | `true` | `Start()` 返回错误 |

LKG 按输出项独立维护：同一 Namespace 输出 YAML 和 XML 时，一个格式没有 LKG 不影响另一个格式。

### 首次拉取重试

agollo 启动时若首次同步失败，会在后台每 2 秒长轮询继续加载。`Retry` 让 `Start()` 按间隔重新检查尚未加载成功的 Namespace，等待其加载完成，用尽次数后再按 `Required` 与 LKG 处理：

| 字段 | 说明 |
|---|---|
| `MaxAttempts` | 总尝试次数（含第一次），`0` 或 `1` 表示不重试 |
| `Interval` | 两次尝试的间隔，`MaxAttempts` 大于 1 时必填 |

```go
Retry: apollo.RetryConfig{MaxAttempts: 5, Interval: 2 * time.Second},
```

- 重试只针对 Apollo 不可用；配置内容错误不重试，直接失败
- 有 LKG 的 Namespace 同样会重试，以便尽量使用最新配置；希望尽快以 LKG 启动时减小重试次数
- 不同 Namespace 在同一轮中检查，总等待时间上限约为 `(MaxAttempts-1) × Interval`
- 判断依据是 agollo 是否已成功加载该 Namespace：Apollo 中没有任何配置项的 properties 型 Namespace 在 agollo 中始终处于未加载状态，会被视为不可用

### 取消启动

`Start(ctx)` 的 ctx 用于取消启动过程，包括连接 Apollo（agollo 启动在远端不可用时可能阻塞十余秒）与重试等待。ctx 结束时 `Start()` 关闭已建立的连接并返回 ctx 的错误，可用 `errors.Is(err, context.DeadlineExceeded)` 判断；此时不会以 LKG 降级启动。`Start()` 返回后不再使用 ctx，停止同步应调用 `Close()`。

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
if err := syncer.Start(ctx); err != nil {
    return err
}
```

### 启动流程

```text
Watch=true 时先注册 agollo 变更监听，期间到达的变更暂存，首次同步完成后再处理
对每个输出项：
  读取该输出项的 LKG
    ├─ 存在且 hash 校验通过：作为降级候选，Source=lkg
    └─ 不存在或校验失败：Source=empty
  拉取 Apollo 配置
    ├─ 失败：按 Required 与 LKG 决定是否报错，错误记录到 LastError
    └─ 成功：生成落地内容
          ├─ 内容错误：Start() 返回错误，不写入
          └─ 成功：原子发布，Source=remote
```

LKG 只校验文件完整性，不解析内容：稳定路径 symlink 指向的版本文件，其内容 SHA-256 必须与文件名中的 hash 一致。索引仅提供 releaseKey；索引丢失或落后于 symlink（如切换 symlink 后、更新索引前进程崩溃）时，LKG 仍然有效，`ReleaseKey` 为空。LKG 保证可用性，不保证配置是最新的；对陈旧配置敏感的业务应自行增加最大陈旧时间与告警。

## 热更新

`Watch: true` 时：

1. `Start()` 先注册 agollo 变更监听，再完成首次同步；同步期间到达的变更暂存，同步完成后再处理，不会丢失
2. Apollo 发布后，agollo 收到完整 Namespace 快照及 releaseKey
3. 变更进入待处理表，由单个 worker 串行处理
4. 为该 Namespace 的所有输出项分别生成内容、去重并原子发布

补充说明：

- 同一 Namespace 的多个输出项共用一次订阅
- 通知 ID 小于已处理值的过期变更会被丢弃
- 待处理表中每个 Namespace 只保留通知 ID 最新的一条：短时间内多次发布时会合并中间版本，但最新版本一定落地，内存占用不随发布次数增长
- `Close()` 后到达的变更直接忽略
- `Watch: false` 时只执行首次同步，后续 Apollo 发布不会更新本地文件

## 本地目录结构

以 `config.json` 原样落地、同时转换为 YAML，以及 `application` 原样落地为例：

```text
<CacheDir>/
  config.json            -> .apollo/snapshots/config-<hash>.json
  config.yaml            -> .apollo/snapshots/config-<hash>.yaml
  application.properties -> .apollo/snapshots/application-<hash>.properties
  .apollo/
    index-<id>.json
    index-<id>.json
    index-<id>.json
    snapshots/
      config-<hash>.json
      config-<hash>.yaml
      application-<hash>.properties
```

| 路径 | 用途 |
|---|---|
| `<CacheDir>/<文件名>` | 业务读取的稳定路径，是指向当前版本的 symlink |
| `.apollo/snapshots/<文件名去扩展名>-<hash>.<格式>` | 当前版本的完整文件，内容不可变，`<hash>` 为文件内容的 SHA-256；每个输出项只保留当前版本 |
| `.apollo/index-<id>.json` | 每个输出项的 LKG 索引，记录 `hash`、`format`、`release_key`；`<id>` 为 `Namespace:格式` 的 SHA-256 |

### 发布顺序

内容变化时：

1. 生成目标文件内容并完成检查
2. 计算文件内容的 SHA-256
3. 写入不可变版本文件并 fsync
4. 以临时 symlink + rename 原子切换稳定路径
5. fsync 目录
6. 以临时文件 + rename 原子更新索引
7. 删除不再被任何稳定路径 symlink 引用的版本文件

内容 hash 不变、仅 releaseKey 变化时，只更新索引，不生成新版本、不切换 symlink，同样执行第 7 步。

清理规则：

- 以 `CacheDir` 下所有 symlink 的实际指向判断引用，其他输出项的当前版本、已从配置中移除但稳定路径仍在的输出项的版本都不会被删除
- 发布前已打开旧版本文件的读取方不受影响，仍能读到完整的旧内容
- 清理失败不影响本次发布，残留文件在下次发布时再次清理；旧版本遗留的历史文件在升级后的首次发布时清理

### 持久化

`CacheDir` 是持久化目录，进程退出不会删除。容器部署时应挂载到持久卷，否则重建后无 LKG 可用。

## 与 gconf 集成

一个 gconf 实例读取一个文件，`ConfigPaths` 指向 `CacheDir`，`ConfigName` 为输出文件名去掉扩展名：

```go
// application -> <CacheDir>/application.properties
appConf, err := gconf.New(
    gconf.WithConfigPaths("/var/lib/my-app/apollo"),
    gconf.WithConfigName("application"),
    gconf.WithWatchConfig(true),
)

// config.json 转 TOML -> <CacheDir>/config.toml
tomlConf, err := gconf.New(
    gconf.WithConfigPaths("/var/lib/my-app/apollo"),
    gconf.WithConfigName("config"),
    gconf.WithConfigType("toml"),
    gconf.WithWatchConfig(true),
)
```

注意事项：

- **同名不同格式**：`CacheDir` 下同时存在 `config.json` 与 `config.toml` 时，gconf 按扩展名顺序会先找到 `config.json`，此时需要 `WithConfigType` 指定类型，如上例
- **XML**：gconf（Viper）不支持读取 XML，XML 输出只适用于其他程序读取
- **properties 占位符**：gconf 读取 properties 时会展开值中的 `${key}`，引用不存在的 key 会被替换为空字符串，自引用报错。Apollo 中的值含 `${...}` 时，经 gconf 读到的值会与 Apollo 中不同；可为该 Namespace 配置 `Format: apollo.FormatJSON` 等格式规避
- **热更新**：端到端热更新需同时启用 Syncer 的 `Watch` 与 gconf 的 `WithWatchConfig(true)`；gconf 能感知 symlink 切换并自动重载

| Syncer `Watch` | gconf `WithWatchConfig` | 效果 |
|---:|---:|---|
| `false` | `false` | 启动时同步一次，gconf 读取一次 |
| `true` | `false` | 本地文件持续更新，gconf 不自动重载 |
| `false` | `true` | gconf 监听文件，但没有后续远端更新 |
| `true` | `true` | Apollo 发布后同步到本地，gconf 自动重载 |

- **优先级**：gconf 中 `Set()` 与环境变量优先于配置文件，被覆盖的 key 不会随 Apollo 变化

## Status 状态查询

`Status()` 只返回内存中的状态副本，不访问 Apollo、不读磁盘、不触发同步。

```go
for _, item := range syncer.Status().Namespaces {
    log.Printf("namespace=%s format=%s source=%s releaseKey=%s hash=%s lastSuccess=%s lastError=%s",
        item.Namespace, item.Format, item.Source, item.ReleaseKey, item.Hash, item.LastSuccess, item.LastError)
}
```

| 字段 | 说明 |
|---|---|
| `Namespace` | Apollo Namespace 名称 |
| `Format` | 该输出项的目标格式 |
| `ReleaseKey` | Apollo 发布版本标识，仅用于追踪排障，不参与去重 |
| `Hash` | 当前文件内容的 SHA-256 |
| `Source` | `remote`：已发布 Apollo 配置；`lkg`：使用本地 LKG；`empty`：无可用配置 |
| `LastSuccess` | 最近一次成功发布远端配置的时间 |
| `LastError` | 最近一次错误，下一次成功后清空 |

- 返回顺序与 `Namespaces` 配置顺序一致，同一 Namespace 的多个输出项各占一条
- 运行中出现的配置内容错误只记录在 `LastError`，建议监控该字段

## 生命周期

```go
syncer, err := apollo.New(cfg)         // 只校验配置并构造对象，不连接远端
if err != nil {
    return err
}
if err := syncer.Start(ctx); err != nil { // 首次同步；Watch=true 时同时启动监听
    return err
}
defer syncer.Close()                    // 停止监听并关闭 agollo 客户端，可重复调用
```

- 一个 Syncer 只应调用一次 `Start()`
- `Start()` 失败时内部已关闭，无需再调用 `Close()`
- `Start(ctx)` 的 ctx 只作用于启动过程，见 [取消启动](#取消启动)；停止同步应调用 `Close()`

## 安全与运维

- `Secret` 应通过环境变量或密钥管理服务注入，不应硬编码或写入日志
- Apollo AccessKey 只用于请求认证，不加密本地文件
- `CacheDir` 可能包含数据库地址、令牌等敏感配置，应限制目录与文件权限
- 不要把 `CacheDir` 放在系统会自动清理的临时目录
- 监控 `Status()` 的 `Source`、`LastError`、`LastSuccess`
- 多个 Namespace 独立发布，不保证同时生效
- Apollo 配置传播为最终一致，不应作为分布式锁或事务协调机制

## 常见问题

### 为什么修改 Apollo 后本地文件没有更新

依次检查：

1. Syncer 是否配置 `Watch: true`
2. Apollo 是否已发布，而不只是保存
3. `Status().LastError` 是否有错误，如内容无法解析或无法转换
4. 文件已更新但 gconf 未更新：检查 `WithWatchConfig(true)`
5. gconf 已更新但业务值未变：检查 `Set()`、环境变量与业务结构体缓存

### 为什么 releaseKey 变化但没有新的版本文件

最终文件内容没有变化。模块按文件内容 hash 去重，此时只更新索引中的 releaseKey。

### 升级后 properties 型 Namespace 的输出文件变了

旧版本默认将 properties 型 Namespace 转为 JSON（如 `application.json`），当前版本默认原样落地为 properties（如 `application.properties`）。旧文件不会被删除也不再更新，读取方需切换到新文件，或配置 `Format: apollo.FormatJSON` 保持原格式。

### 为什么启动失败提示 invalid content

Apollo 上发布的配置无法解析或无法转换为目标格式。按错误信息修正 Apollo 上的配置；若读取方能处理该内容且无需转换，可设置 `SkipValidate: true`。

## 已知限制

- `Merge` 为预留字段，当前无任何效果
- Apollo 中没有任何配置项的 properties 型 Namespace 在 agollo 中始终处于未加载状态，被视为 Apollo 不可用
- 热更新时 releaseKey 取自 agollo 回调执行时的当前值，连续快速发布时可能与内容相差一个版本；releaseKey 仅用于排障，去重与过期判断不依赖它
- `.txt` 与带 `.properties` 后缀的文件型 Namespace 只能原样落地；带 `.properties` 后缀的文件型 Namespace 未在真实 Apollo 上验证
- 为保留 yaml/yml Namespace 的原文，本模块在 agollo 中将 yaml/yml 解析器替换为不预解析；该替换对整个进程生效，同一进程内直接使用 agollo 按 key 读取 yaml Namespace 的代码将取不到值
- properties 型 Namespace 的配置项顺序与备注不会保留
- 多个 Namespace 之间的发布非原子
