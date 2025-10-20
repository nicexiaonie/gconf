[toc]

# Gconf 快速开始指南

这份指南将帮助你在 5 分钟内上手 Gconf。

## 安装

```bash
go get -u github.com/nicexiaonie/gconf
```

## 第一个示例

### 1. 创建配置文件

创建 `config.yaml`:

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

创建 `main.go`:

```go
package main

import (
    "fmt"
    "github.com/nicexiaonie/gconf"
)

func main() {
    // 初始化配置
    gconf.Init()
    
    // 读取配置
    appName := gconf.GetString("app.name")
    port := gconf.GetInt("app.port")
    debug := gconf.GetBool("app.debug")
    
    fmt.Printf("应用: %s\n", appName)
    fmt.Printf("端口: %d\n", port)
    fmt.Printf("调试模式: %v\n", debug)
}
```

### 3. 运行

```bash
go run main.go
```

输出：
```
应用: 我的应用
端口: 8080
调试模式: true
```

## 常用场景

### 场景 1: 使用结构体管理配置

```go
package main

import (
    "fmt"
    "github.com/nicexiaonie/gconf"
)

type Config struct {
    App struct {
        Name  string `mapstructure:"name"`
        Port  int    `mapstructure:"port"`
        Debug bool   `mapstructure:"debug"`
    } `mapstructure:"app"`
    
    Database struct {
        Host string `mapstructure:"host"`
        Port int    `mapstructure:"port"`
    } `mapstructure:"database"`
}

func main() {
    gconf.Init()
    
    var config Config
    if err := gconf.Unmarshal(&config); err != nil {
        panic(err)
    }
    
    fmt.Printf("应用: %s:%d\n", config.App.Name, config.App.Port)
    fmt.Printf("数据库: %s:%d\n", config.Database.Host, config.Database.Port)
}
```

### 场景 2: 使用环境变量

```go
package main

import (
    "fmt"
    "github.com/nicexiaonie/gconf"
)

func main() {
    // 启用环境变量读取
    gconf.Init(
        gconf.WithAutomaticEnv(true),
        gconf.WithEnvPrefix("APP"),
        gconf.WithEnvKeyReplacer(".", "_"),
    )
    
    // 配置键 app.name 会读取环境变量 APP_APP_NAME
    // 配置键 database.host 会读取环境变量 APP_DATABASE_HOST
    
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

### 场景 3: 配置热更新

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
    // 启用配置监听
    gconf.Init(
        gconf.WithWatchConfig(true),
        gconf.WithOnConfigChange(func(e fsnotify.Event) {
            log.Println("配置已更新!")
            // 重新读取配置
            fmt.Println("新端口:", gconf.GetInt("app.port"))
        }),
    )
    
    fmt.Println("当前端口:", gconf.GetInt("app.port"))
    fmt.Println("修改 config.yaml 文件试试...")
    
    // 保持程序运行
    time.Sleep(5 * time.Minute)
}
```

### 场景 4: 多环境配置

```go
package main

import (
    "fmt"
    "os"
    
    "github.com/nicexiaonie/gconf"
)

func main() {
    // 根据环境选择配置文件
    env := os.Getenv("ENV")
    if env == "" {
        env = "development"
    }
    
    configName := fmt.Sprintf("config.%s", env)
    
    gconf.Init(
        gconf.WithConfigName(configName),
        gconf.WithConfigPaths(".", "./config"),
    )
    
    fmt.Printf("环境: %s\n", env)
    fmt.Printf("配置文件: %s\n", gconf.ConfigFileUsed())
}
```

目录结构：
```
project/
├── config/
│   ├── config.development.yaml
│   ├── config.testing.yaml
│   └── config.production.yaml
└── main.go
```

运行：
```bash
# 开发环境
ENV=development go run main.go

# 生产环境
ENV=production go run main.go
```

### 场景 5: 设置默认值

```go
package main

import (
    "fmt"
    "github.com/nicexiaonie/gconf"
)

func main() {
    gconf.Init()
    
    // 设置默认值（如果配置文件中没有，就使用默认值）
    gconf.SetDefault("app.name", "默认应用名")
    gconf.SetDefault("app.port", 8080)
    gconf.SetDefault("app.timeout", "30s")
    
    fmt.Println("应用:", gconf.GetString("app.name"))
    fmt.Println("端口:", gconf.GetInt("app.port"))
    fmt.Println("超时:", gconf.GetDuration("app.timeout"))
}
```

## 配置优先级

配置的优先级从高到低：

1. **环境变量** - 最高优先级，适合容器部署
2. **Set() 设置的值** - 运行时修改
3. **配置文件** - YAML/JSON/TOML 等
4. **SetDefault() 默认值** - 兜底配置

示例：
```go
gconf.Init(gconf.WithAutomaticEnv(true))

// 设置默认值
gconf.SetDefault("port", 8080)  // 优先级 4

// 配置文件中: port: 9090     // 优先级 3

// 运行时设置
gconf.Set("port", 9091)          // 优先级 2

// 环境变量: PORT=9092         // 优先级 1（最高）

// 实际读取的是环境变量的值 9092
fmt.Println(gconf.GetInt("port"))
```

## 支持的配置格式

- YAML (`.yaml`, `.yml`)
- JSON (`.json`)
- TOML (`.toml`)
- HCL (`.hcl`)
- INI (`.ini`)
- ENV (`.env`)
- Properties (`.properties`)

## 常用 API

### 读取配置

```go
// 基本类型
gconf.GetString("key")
gconf.GetInt("key")
gconf.GetBool("key")
gconf.GetFloat64("key")

// 时间相关
gconf.GetDuration("key")      // "30s", "1m", "1h"
gconf.GetTime("key")

// 集合类型
gconf.GetStringSlice("key")
gconf.GetStringMap("key")
gconf.GetStringMapString("key")
```

### 设置配置

```go
// 设置默认值
gconf.SetDefault("key", "value")

// 设置配置
gconf.Set("key", "value")

// 检查配置是否存在
if gconf.IsSet("key") {
    // ...
}
```

### 结构体解析

```go
// 解析全部配置
var config Config
gconf.Unmarshal(&config)

// 解析部分配置
var dbConfig DatabaseConfig
gconf.UnmarshalKey("database", &dbConfig)
```

## Docker 部署示例

Dockerfile:
```dockerfile
FROM golang:1.20-alpine

WORKDIR /app
COPY . .
RUN go build -o myapp

# 通过环境变量配置
ENV APP_SERVER_PORT=8080
ENV APP_DATABASE_HOST=db.prod.com

CMD ["./myapp"]
```

docker-compose.yml:
```yaml
version: '3'
services:
  app:
    build: .
    environment:
      - APP_SERVER_PORT=8080
      - APP_DATABASE_HOST=db
      - APP_DATABASE_PASSWORD=secret
    ports:
      - "8080:8080"
```

## 下一步

- 查看 [完整文档](README.md)
- 浏览 [示例代码](example/)
- 阅读 [API 文档](https://pkg.go.dev/github.com/nicexiaonie/gconf)

## 获取帮助

- 提交 [Issue](https://github.com/nicexiaonie/gconf/issues)
- 查看 [FAQ](README.md#常见问题)

---

**Happy Coding! 🎉**

