# Gconf 快速开始

本指南介绍 Gconf 的常用场景，完整说明见 [README.md](README.md)。

## 安装

```bash
go get github.com/nicexiaonie/gconf
```

要求 Go 1.21 及以上。

## 第一个示例

### 1. 创建配置文件

创建 `config.yaml`：

```yaml
app:
  name: "我的应用"
  port: 8080
  debug: true

database:
  host: "localhost"
  port: 3306
```

### 2. 编写代码

创建 `main.go`：

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

    fmt.Printf("应用: %s\n", gconf.GetString("app.name"))
    fmt.Printf("端口: %d\n", gconf.GetInt("app.port"))
    fmt.Printf("调试模式: %v\n", gconf.GetBool("app.debug"))
}
```

### 3. 运行

```bash
go run main.go
```

输出：

```text
应用: 我的应用
端口: 8080
调试模式: true
```

未找到配置文件时 `Init` 不会报错，配置为空；配置文件存在但解析失败时 `Init` 返回错误。可用 `gconf.ConfigFileUsed()` 确认实际加载的文件。

## 常用场景

### 场景 1：解析到结构体

```go
type Config struct {
    App struct {
        Name  string
        Port  int
        Debug bool
    }
    Database struct {
        Host string
        Port int
    }
}

func main() {
    if err := gconf.Init(); err != nil {
        log.Fatal(err)
    }

    var config Config
    if err := gconf.Unmarshal(&config); err != nil {
        log.Fatal(err)
    }

    fmt.Printf("应用: %s:%d\n", config.App.Name, config.App.Port)
    fmt.Printf("数据库: %s:%d\n", config.Database.Host, config.Database.Port)
}
```

字段名匹配不区分大小写，名称不一致时使用 `mapstructure` tag，如 `` `mapstructure:"max_conn"` ``。结构体已使用 `json` 或 `toml` tag 时，可通过 `gconf.WithTagName("json")` 指定标签名，见 README 的“使用 json / toml 标签”。

### 场景 2：环境变量覆盖

```go
func main() {
    err := gconf.Init(
        gconf.WithAutomaticEnv(true),
        gconf.WithEnvPrefix("APP"),
        gconf.WithEnvKeyReplacer(".", "_"),
    )
    if err != nil {
        log.Fatal(err)
    }

    // 配置键 app.name 对应环境变量 APP_APP_NAME
    // 配置键 database.host 对应环境变量 APP_DATABASE_HOST
    fmt.Println("应用:", gconf.GetString("app.name"))
    fmt.Println("数据库:", gconf.GetString("database.host"))
}
```

运行：

```bash
export APP_APP_NAME="生产环境应用"
export APP_DATABASE_HOST="prod.db.com"
go run main.go
```

`Unmarshal` 只包含已知的 key：仅由环境变量提供、配置文件中不存在的 key，需要先 `gconf.GetInstance().BindEnv("key")` 或 `gconf.SetDefault("key", ...)` 才会被解析到结构体。

### 场景 3：配置热更新

```go
package main

import (
    "fmt"
    "log"
    "time"

    "github.com/fsnotify/fsnotify"
    "github.com/nicexiaonie/gconf"
)

func main() {
    err := gconf.Init(
        gconf.WithWatchConfig(true),
        gconf.WithOnConfigChange(func(e fsnotify.Event) {
            // 回调在重载完成后执行，可直接读取新配置
            fmt.Println("配置已更新，新端口:", gconf.GetInt("app.port"))
        }),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("当前端口:", gconf.GetInt("app.port"))
    fmt.Println("修改 config.yaml 试试...")

    time.Sleep(5 * time.Minute)
}
```

通过 `Unmarshal` 得到的结构体不会自动更新，需要在回调中重新解析。

### 场景 4：多环境配置

```go
func main() {
    env := os.Getenv("ENV")
    if env == "" {
        env = "development"
    }

    err := gconf.Init(
        gconf.WithConfigName("config." + env), // 查找 config.development.yaml 等
        gconf.WithConfigPaths(".", "./config"),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("环境: %s\n", env)
    fmt.Printf("配置文件: %s\n", gconf.ConfigFileUsed())
}
```

目录结构：

```text
project/
├── config/
│   ├── config.development.yaml
│   ├── config.testing.yaml
│   └── config.production.yaml
└── main.go
```

运行：

```bash
ENV=development go run main.go
ENV=production go run main.go
```

### 场景 5：默认值

```go
func main() {
    if err := gconf.Init(); err != nil {
        log.Fatal(err)
    }

    // 配置文件与环境变量中都没有时使用默认值
    gconf.SetDefault("app.name", "默认应用名")
    gconf.SetDefault("app.port", 8080)
    gconf.SetDefault("app.timeout", "30s")

    fmt.Println("应用:", gconf.GetString("app.name"))
    fmt.Println("端口:", gconf.GetInt("app.port"))
    // GetDuration 没有对应的包级函数，通过全局实例调用
    fmt.Println("超时:", gconf.GetInstance().GetDuration("app.timeout"))
}
```

## 配置优先级

读取时的优先级从高到低：

1. **`Set()` 设置的值**：运行时覆盖
2. **环境变量**：需启用 `WithAutomaticEnv(true)` 或 `BindEnv`
3. **配置文件**
4. **`SetDefault()` 默认值**

```go
// 环境变量: PORT=9092
gconf.Init(gconf.WithAutomaticEnv(true))

gconf.SetDefault("port", 8080) // 优先级 4
// 配置文件中: port: 9090       // 优先级 3
// 环境变量 PORT=9092           // 优先级 2
gconf.Set("port", 9091)        // 优先级 1（最高）

fmt.Println(gconf.GetInt("port")) // 9091
```

## 支持的配置格式

未指定 `WithConfigType` 时按文件扩展名识别：

| 格式 | 扩展名 |
|---|---|
| YAML | `.yaml`、`.yml` |
| JSON | `.json` |
| TOML | `.toml` |
| properties | `.properties`、`.props`、`.prop` |
| HCL | `.hcl`、`.tfvars` |
| INI | `.ini` |
| env / dotenv | `.env`、`.dotenv` |

无扩展名的文件需要通过 `WithConfigType` 指定类型。各格式的读取差异见 [README.md](README.md#支持的格式)。

## 常用 API

### 读取配置

```go
gconf.GetString("key")
gconf.GetInt("key")
gconf.GetBool("key")
gconf.GetFloat64("key")
gconf.GetStringSlice("key")
gconf.GetStringMap("key")
gconf.GetStringMapString("key")

// 以下方法没有包级函数，通过全局实例调用
gconf.GetInstance().GetDuration("key") // "30s"、"1m"、"1h"
gconf.GetInstance().GetTime("key")
gconf.GetInstance().GetSizeInBytes("key") // "10MB"
```

### 设置与查询

```go
gconf.SetDefault("key", "value") // 默认值
gconf.Set("key", "value")        // 运行时覆盖，只修改内存
if gconf.IsSet("key") {          // key 是否存在
    // ...
}
```

### 结构体解析

```go
var config Config
if err := gconf.Unmarshal(&config); err != nil {
    // ...
}

var dbConfig DatabaseConfig
if err := gconf.UnmarshalKey("database", &dbConfig); err != nil {
    // ...
}
```

## Docker 部署示例

应用以环境变量前缀 `APP` 初始化：

```go
gconf.Init(
    gconf.WithConfigPaths("/etc/myapp"),
    gconf.WithAutomaticEnv(true),
    gconf.WithEnvPrefix("APP"),
    gconf.WithEnvKeyReplacer(".", "_"),
)
```

Dockerfile：

```dockerfile
FROM golang:1.21-alpine

WORKDIR /app
COPY . .
RUN go build -o myapp

# server.port 与 database.host 的值
ENV APP_SERVER_PORT=8080
ENV APP_DATABASE_HOST=db.prod.com

CMD ["./myapp"]
```

docker-compose.yml：

```yaml
version: '3'
services:
  app:
    build: .
    volumes:
      - ./config:/etc/myapp
    environment:
      - APP_SERVER_PORT=8080
      - APP_DATABASE_HOST=db
      - APP_DATABASE_PASSWORD=secret
    ports:
      - "8080:8080"
```

## 下一步

- [完整文档](README.md)
- [示例代码](example/)
- [Apollo 配置中心同步](apollo/README.md)
- [API 文档](https://pkg.go.dev/github.com/nicexiaonie/gconf)

## 获取帮助

- 提交 [Issue](https://github.com/nicexiaonie/gconf/issues)
- 查看 [常见问题](README.md#常见问题)
