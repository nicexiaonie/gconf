[toc]

# Gconf - 强大的 Go 配置管理工具

<div align="center">

[![Go Version](https://img.shields.io/badge/Go-%3E%3D%201.14-blue)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

**Gconf** 是一个基于 [Viper](https://github.com/spf13/viper) 的配置管理工具封装，提供开箱即用、功能强大且简洁的配置管理解决方案。

[English](README.md) | 简体中文

</div>

## ✨ 核心特性

- 🚀 **开箱即用** - 零配置快速启动，合理的默认配置让您立即上手
- 🔧 **功能全面** - 支持 YAML、JSON、TOML、HCL、INI 等多种配置格式
- 🌍 **环境变量集成** - 自动读取环境变量，完美适配容器化和 Kubernetes 部署
- 🔄 **配置热更新** - 实时监听配置文件变化，自动重载无需重启
- 🎯 **类型安全** - 提供完整的类型转换方法，避免类型错误
- 📦 **结构体解析** - 一键将配置解析到 Go 结构体，简化配置管理
- 🌳 **嵌套配置** - 支持多层配置和子配置树独立访问
- 🔌 **灵活配置** - 采用 Options 模式，链式调用简洁优雅
- 🌐 **单例模式** - 提供全局实例，随处可用
- ⚡ **高性能** - 基于成熟的 Viper 库，性能卓越
- 🔒 **并发安全** - 内置线程安全机制，多协程访问无忧

## 📦 安装

```bash
go get -u github.com/nicexiaonie/gconf
```

## 🚀 快速开始

### 最简使用

```go
package main

import (
    "fmt"
    "github.com/nicexiaonie/gconf"
)

func main() {
    // 初始化配置（自动查找 config.yaml）
    gconf.Init()
    
    // 设置默认值
    gconf.SetDefault("app.name", "我的应用")
    gconf.SetDefault("server.port", 8080)
    
    // 读取配置
    fmt.Println("应用名称:", gconf.GetString("app.name"))
    fmt.Println("端口:", gconf.GetInt("server.port"))
}
```

### 使用配置文件

创建 `config.yaml`:

```yaml
app:
  name: "我的应用"
  version: "1.0.0"
  debug: false

server:
  host: "0.0.0.0"
  port: 8080
  read_timeout: "30s"

database:
  driver: "mysql"
  host: "localhost"
  port: 3306
  username: "root"
  password: "password"
```

读取配置：

```go
func main() {
    // 初始化
    gconf.Init(
        gconf.WithConfigName("config"),
        gconf.WithConfigType("yaml"),
        gconf.WithConfigPaths(".", "./config"),
    )
    
    // 使用配置
    fmt.Println("应用:", gconf.GetString("app.name"))
    fmt.Println("端口:", gconf.GetInt("server.port"))
    fmt.Println("数据库:", gconf.GetString("database.host"))
}
```

## 📖 详细文档

### 初始化配置

#### 方式一：全局实例（推荐）

适合大多数应用场景，配置全局可用：

```go
// 在 main 函数中初始化
err := gconf.Init(
    gconf.WithConfigName("config"),
    gconf.WithConfigType("yaml"),
    gconf.WithConfigPaths("."),
)

// 在任何地方使用
func someFunction() {
    port := gconf.GetInt("server.port")
}
```

#### 方式二：独立实例

适合需要管理多个配置文件的场景：

```go
conf, err := gconf.New(
    gconf.WithConfigName("app"),
    gconf.WithConfigType("yaml"),
)

port := conf.GetInt("server.port")
```

### 配置选项详解

```go
conf, err := gconf.New(
    // 配置文件名（不含扩展名）
    gconf.WithConfigName("config"),
    
    // 配置文件类型（yaml/json/toml/hcl/ini/env）
    gconf.WithConfigType("yaml"),
    
    // 配置文件搜索路径（可指定多个）
    gconf.WithConfigPaths(".", "./config", "/etc/myapp"),
    
    // 启用配置文件监听和热更新
    gconf.WithWatchConfig(true),
    
    // 自动读取环境变量
    gconf.WithAutomaticEnv(true),
    
    // 环境变量前缀（例如：MYAPP_）
    gconf.WithEnvPrefix("MYAPP"),
    
    // 环境变量键替换规则（配置键中的 . 替换为 _）
    gconf.WithEnvKeyReplacer(".", "_"),
    
    // 配置变化回调函数
    gconf.WithOnConfigChange(func(e fsnotify.Event) {
        log.Println("配置文件已更新:", e.Name)
    }),
    
    // 启用调试模式（输出详细日志）
    gconf.WithDebug(true),
)
```

### 读取配置

#### 基本类型

```go
// 字符串
name := conf.GetString("app.name")

// 整数类型
port := conf.GetInt("server.port")           // int
port32 := conf.GetInt32("server.port")       // int32
port64 := conf.GetInt64("server.port")       // int64

// 无符号整数
count := conf.GetUint("app.count")           // uint
count32 := conf.GetUint32("app.count")       // uint32
count64 := conf.GetUint64("app.count")       // uint64

// 浮点数
pi := conf.GetFloat64("math.pi")

// 布尔值
debug := conf.GetBool("app.debug")

// 时间间隔（支持 "30s", "1m", "1h" 等格式）
timeout := conf.GetDuration("server.timeout")

// 时间类型
startTime := conf.GetTime("app.start_time")

// 任意类型
value := conf.Get("app.custom")
```

#### 复杂类型

```go
// 字符串数组
features := conf.GetStringSlice("app.features")
// ["api", "admin", "monitoring"]

// 映射类型
metadata := conf.GetStringMap("app.metadata")
// map[string]interface{}

// 字符串到字符串的映射
labels := conf.GetStringMapString("app.labels")
// map[string]string

// 字符串到字符串数组的映射
tags := conf.GetStringMapStringSlice("app.tags")
// map[string][]string

// 文件大小（支持 "1KB", "10MB", "1GB" 等）
maxSize := conf.GetSizeInBytes("upload.max_size")
```

### 解析到结构体

这是推荐的使用方式，类型安全且易于维护：

```go
// 定义配置结构体
type AppConfig struct {
    App struct {
        Name    string `mapstructure:"name"`
        Version string `mapstructure:"version"`
        Debug   bool   `mapstructure:"debug"`
    } `mapstructure:"app"`
    
    Server struct {
        Host         string        `mapstructure:"host"`
        Port         int           `mapstructure:"port"`
        ReadTimeout  time.Duration `mapstructure:"read_timeout"`
        WriteTimeout time.Duration `mapstructure:"write_timeout"`
    } `mapstructure:"server"`
    
    Database struct {
        Driver   string `mapstructure:"driver"`
        Host     string `mapstructure:"host"`
        Port     int    `mapstructure:"port"`
        Username string `mapstructure:"username"`
        Password string `mapstructure:"password"`
    } `mapstructure:"database"`
}

var config AppConfig

// 解析全部配置
if err := conf.Unmarshal(&config); err != nil {
    log.Fatal(err)
}

// 只解析指定部分
var serverConfig struct {
    Host string `mapstructure:"host"`
    Port int    `mapstructure:"port"`
}
if err := conf.UnmarshalKey("server", &serverConfig); err != nil {
    log.Fatal(err)
}

// 严格解析（配置中有结构体未定义的字段会报错）
if err := conf.UnmarshalExact(&config); err != nil {
    log.Fatal(err)
}
```

### 设置和修改配置

```go
// 设置默认值（优先级最低，会被配置文件和环境变量覆盖）
conf.SetDefault("app.name", "默认应用名")
conf.SetDefault("server.port", 8080)

// 设置配置值（运行时修改）
conf.Set("app.debug", true)
conf.Set("server.port", 9090)

// 检查配置是否存在
if conf.IsSet("app.name") {
    fmt.Println("app.name 配置存在")
}

// 获取所有配置键
keys := conf.AllKeys()
fmt.Println("所有配置键:", keys)

// 获取所有配置（返回 map）
settings := conf.AllSettings()
fmt.Println("所有配置:", settings)
```

### 环境变量集成

环境变量是配置优先级最高的方式，特别适合容器化部署：

```go
// 自动读取环境变量
conf, _ := gconf.New(
    gconf.WithAutomaticEnv(true),
    gconf.WithEnvPrefix("MYAPP"),
    gconf.WithEnvKeyReplacer(".", "_"),
)

// 配置键 app.name 会自动映射到环境变量 MYAPP_APP_NAME
// 配置键 database.host 会映射到 MYAPP_DATABASE_HOST
name := conf.GetString("app.name")
dbHost := conf.GetString("database.host")

// 绑定特定的环境变量
conf.BindEnv("api.token", "API_TOKEN")
token := conf.GetString("api.token")
```

**配置优先级**（从高到低）：

1. 环境变量（最高优先级）
2. `Set()` 方法设置的值
3. 配置文件中的值
4. `SetDefault()` 设置的默认值

### 配置热更新

监听配置文件变化，实时更新配置无需重启应用：

```go
conf, _ := gconf.New(
    gconf.WithConfigName("config"),
    gconf.WithWatchConfig(true),
    gconf.WithOnConfigChange(func(e fsnotify.Event) {
        log.Printf("配置文件已更新: %s (操作: %s)", e.Name, e.Op)
        
        // 重新加载配置到结构体
        var config AppConfig
        if err := conf.Unmarshal(&config); err != nil {
            log.Printf("重新加载配置失败: %v", err)
            return
        }
        
        // 执行配置更新后的处理逻辑
        onConfigUpdate(&config)
    }),
)

// 也可以注册多个回调函数
conf.OnConfigChange(func(e fsnotify.Event) {
    log.Println("回调2:", e.Name)
})

conf.OnConfigChange(func(e fsnotify.Event) {
    log.Println("回调3:", e.Name)
})
```

### 配置文件写入

```go
// 修改配置
conf.Set("app.version", "2.0.0")
conf.Set("server.port", 9090)

// 写入到当前使用的配置文件
err := conf.WriteConfig()

// 安全写入（文件已存在时不覆盖，返回错误）
err := conf.SafeWriteConfig()

// 写入到指定文件
err := conf.WriteConfigAs("/path/to/new-config.yaml")

// 安全写入到指定文件
err := conf.SafeWriteConfigAs("/path/to/new-config.yaml")
```

### 子配置树

处理嵌套配置时特别有用：

```go
// 设置嵌套配置
conf.Set("database.mysql.host", "localhost")
conf.Set("database.mysql.port", 3306)
conf.Set("database.redis.host", "localhost")
conf.Set("database.redis.port", 6379)

// 获取子配置树
mysqlConf := conf.Sub("database.mysql")
if mysqlConf != nil {
    host := mysqlConf.GetString("host")
    port := mysqlConf.GetInt("port")
    fmt.Printf("MySQL: %s:%d\n", host, port)
}

redisConf := conf.Sub("database.redis")
if redisConf != nil {
    host := redisConf.GetString("host")
    port := redisConf.GetInt("port")
    fmt.Printf("Redis: %s:%d\n", host, port)
}
```

### 配置别名

为长配置键创建短别名：

```go
// 注册别名
conf.RegisterAlias("port", "server.port")
conf.RegisterAlias("db", "database.host")

// 使用别名访问
port := conf.GetInt("port")         // 等同于 conf.GetInt("server.port")
dbHost := conf.GetString("db")      // 等同于 conf.GetString("database.host")
```

### 其他实用功能

```go
// 合并其他配置文件
err := conf.MergeInConfig()

// 重新读取配置文件
err := conf.ReadInConfig()

// 获取当前使用的配置文件路径
configFile := conf.ConfigFileUsed()
fmt.Println("配置文件:", configFile)

// 获取底层的 viper 实例（用于高级操作）
viper := conf.GetViper()

// 调试：打印所有配置信息
conf.Debug()
```

## 📝 实战示例

### 示例1：Web 应用配置管理

```go
package main

import (
    "fmt"
    "log"
    "time"
    
    "github.com/nicexiaonie/gconf"
)

// 定义配置结构
type AppConfig struct {
    App struct {
        Name    string `mapstructure:"name"`
        Version string `mapstructure:"version"`
        Debug   bool   `mapstructure:"debug"`
    } `mapstructure:"app"`
    
    Server struct {
        Host         string        `mapstructure:"host"`
        Port         int           `mapstructure:"port"`
        ReadTimeout  time.Duration `mapstructure:"read_timeout"`
        WriteTimeout time.Duration `mapstructure:"write_timeout"`
    } `mapstructure:"server"`
    
    Database struct {
        Driver   string `mapstructure:"driver"`
        Host     string `mapstructure:"host"`
        Port     int    `mapstructure:"port"`
        Username string `mapstructure:"username"`
        Password string `mapstructure:"password"`
        Database string `mapstructure:"database"`
        MaxConns int    `mapstructure:"max_conns"`
    } `mapstructure:"database"`
}

func main() {
    // 初始化配置
    err := gconf.Init(
        gconf.WithConfigName("app"),
        gconf.WithConfigPaths(".", "./config"),
        gconf.WithAutomaticEnv(true),
        gconf.WithEnvPrefix("APP"),
        gconf.WithWatchConfig(true),
    )
    if err != nil {
        log.Printf("加载配置文件失败，使用默认配置: %v", err)
    }
    
    // 设置默认值
    setDefaults()
    
    // 解析配置
    var config AppConfig
    if err := gconf.Unmarshal(&config); err != nil {
        log.Fatal("解析配置失败:", err)
    }
    
    // 启动服务
    startServer(&config)
}

func setDefaults() {
    gconf.SetDefault("app.name", "MyApp")
    gconf.SetDefault("app.version", "1.0.0")
    gconf.SetDefault("app.debug", false)
    
    gconf.SetDefault("server.host", "0.0.0.0")
    gconf.SetDefault("server.port", 8080)
    gconf.SetDefault("server.read_timeout", "30s")
    gconf.SetDefault("server.write_timeout", "30s")
    
    gconf.SetDefault("database.driver", "mysql")
    gconf.SetDefault("database.host", "localhost")
    gconf.SetDefault("database.port", 3306)
    gconf.SetDefault("database.max_conns", 100)
}

func startServer(config *AppConfig) {
    fmt.Printf("=== %s v%s ===\n", config.App.Name, config.App.Version)
    fmt.Printf("服务器监听: %s:%d\n", config.Server.Host, config.Server.Port)
    fmt.Printf("数据库连接: %s@%s:%d/%s\n",
        config.Database.Username,
        config.Database.Host,
        config.Database.Port,
        config.Database.Database)
    
    // 启动 HTTP 服务器...
}
```

### 示例2：多环境配置

```go
package main

import (
    "fmt"
    "os"
    
    "github.com/nicexiaonie/gconf"
)

func main() {
    // 根据环境变量选择配置文件
    env := os.Getenv("ENV")
    if env == "" {
        env = "development"
    }
    
    configName := fmt.Sprintf("config.%s", env)
    
    // 初始化配置
    err := gconf.Init(
        gconf.WithConfigName(configName),
        gconf.WithConfigPaths(".", "./config"),
        gconf.WithAutomaticEnv(true),
        gconf.WithDebug(true),
    )
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("当前环境: %s\n", env)
    fmt.Printf("配置文件: %s\n", gconf.ConfigFileUsed())
    
    // 读取配置
    dbHost := gconf.GetString("database.host")
    apiEndpoint := gconf.GetString("api.endpoint")
    
    fmt.Printf("数据库地址: %s\n", dbHost)
    fmt.Printf("API 端点: %s\n", apiEndpoint)
}
```

配置文件结构：
```
config/
  ├── config.development.yaml
  ├── config.testing.yaml
  └── config.production.yaml
```

### 示例3：Docker 容器化配置

```dockerfile
# Dockerfile
FROM golang:1.20-alpine

WORKDIR /app
COPY . .
RUN go build -o myapp

# 设置环境变量
ENV APP_SERVER_PORT=8080
ENV APP_DATABASE_HOST=db.prod.com
ENV APP_DATABASE_PASSWORD=secret

CMD ["./myapp"]
```

```go
// main.go
func main() {
    // 配置会自动从环境变量读取
    gconf.Init(
        gconf.WithAutomaticEnv(true),
        gconf.WithEnvPrefix("APP"),
        gconf.WithEnvKeyReplacer(".", "_"),
    )
    
    // APP_SERVER_PORT -> server.port
    // APP_DATABASE_HOST -> database.host
    // APP_DATABASE_PASSWORD -> database.password
    
    port := gconf.GetInt("server.port")
    dbHost := gconf.GetString("database.host")
    dbPassword := gconf.GetString("database.password")
}
```

### 示例4：配置热更新

```go
package main

import (
    "log"
    "time"
    
    "github.com/fsnotify/fsnotify"
    "github.com/nicexiaonie/gconf"
)

var currentConfig *AppConfig

func main() {
    // 初始化配置（启用热更新）
    err := gconf.Init(
        gconf.WithConfigName("config"),
        gconf.WithWatchConfig(true),
        gconf.WithOnConfigChange(onConfigChange),
    )
    if err != nil {
        log.Fatal(err)
    }
    
    // 加载初始配置
    currentConfig = loadConfig()
    
    // 启动应用
    log.Printf("应用启动，调试模式: %v", currentConfig.App.Debug)
    
    // 保持运行
    select {}
}

func loadConfig() *AppConfig {
    var config AppConfig
    if err := gconf.Unmarshal(&config); err != nil {
        log.Printf("加载配置失败: %v", err)
        return nil
    }
    return &config
}

func onConfigChange(e fsnotify.Event) {
    log.Printf("检测到配置文件变化: %s", e.Name)
    
    // 重新加载配置
    newConfig := loadConfig()
    if newConfig == nil {
        log.Println("配置重载失败，保持当前配置")
        return
    }
    
    // 检查关键配置变化
    if currentConfig.App.Debug != newConfig.App.Debug {
        log.Printf("调试模式变更: %v -> %v", 
            currentConfig.App.Debug, newConfig.App.Debug)
    }
    
    if currentConfig.Server.Port != newConfig.Server.Port {
        log.Printf("端口变更: %d -> %d", 
            currentConfig.Server.Port, newConfig.Server.Port)
        // 注意：端口变更可能需要重启服务器
    }
    
    // 更新当前配置
    currentConfig = newConfig
    log.Println("配置已成功更新")
}
```

## 🎯 最佳实践

### 1. 配置文件与环境变量结合

**推荐做法**：
- 配置文件存放非敏感的默认配置
- 敏感信息（密码、密钥等）通过环境变量传入
- 不同环境使用不同的配置文件

```yaml
# config.yaml
database:
  host: "localhost"
  port: 3306
  username: "root"
  # password 通过环境变量 APP_DATABASE_PASSWORD 提供
  max_conns: 100
```

```go
gconf.Init(
    gconf.WithAutomaticEnv(true),
    gconf.WithEnvPrefix("APP"),
)

// 从配置文件读取
host := gconf.GetString("database.host")
port := gconf.GetInt("database.port")

// 从环境变量读取（优先级更高）
password := gconf.GetString("database.password")
```

### 2. 结构化配置管理

**推荐做法**：定义清晰的配置结构体

```go
// config/config.go
package config

import "time"

type Config struct {
    App      AppConfig      `mapstructure:"app"`
    Server   ServerConfig   `mapstructure:"server"`
    Database DatabaseConfig `mapstructure:"database"`
    Redis    RedisConfig    `mapstructure:"redis"`
    Log      LogConfig      `mapstructure:"log"`
}

type AppConfig struct {
    Name        string   `mapstructure:"name"`
    Version     string   `mapstructure:"version"`
    Debug       bool     `mapstructure:"debug"`
    Environment string   `mapstructure:"environment"`
}

type ServerConfig struct {
    Host         string        `mapstructure:"host"`
    Port         int           `mapstructure:"port"`
    ReadTimeout  time.Duration `mapstructure:"read_timeout"`
    WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

// 更多配置结构...
```

### 3. 配置验证

```go
import "github.com/go-playground/validator/v10"

type Config struct {
    Server struct {
        Port int `mapstructure:"port" validate:"required,min=1024,max=65535"`
        Host string `mapstructure:"host" validate:"required,hostname"`
    } `mapstructure:"server"`
}

func LoadConfig() (*Config, error) {
    var config Config
    if err := gconf.Unmarshal(&config); err != nil {
        return nil, err
    }
    
    // 验证配置
    validate := validator.New()
    if err := validate.Struct(config); err != nil {
        return nil, fmt.Errorf("配置验证失败: %w", err)
    }
    
    return &config, nil
}
```

### 4. 多环境配置管理

```
project/
├── config/
│   ├── config.yaml              # 公共配置
│   ├── config.development.yaml  # 开发环境
│   ├── config.testing.yaml      # 测试环境
│   └── config.production.yaml   # 生产环境
└── main.go
```

```go
func InitConfig() error {
    env := os.Getenv("GO_ENV")
    if env == "" {
        env = "development"
    }
    
    return gconf.Init(
        gconf.WithConfigName(fmt.Sprintf("config.%s", env)),
        gconf.WithConfigPaths("./config"),
        gconf.WithAutomaticEnv(true),
    )
}
```

### 5. 配置热更新最佳实践

```go
type ConfigManager struct {
    mu     sync.RWMutex
    config *Config
}

func (cm *ConfigManager) Get() *Config {
    cm.mu.RLock()
    defer cm.mu.RUnlock()
    return cm.config
}

func (cm *ConfigManager) Update(newConfig *Config) {
    cm.mu.Lock()
    defer cm.mu.Unlock()
    cm.config = newConfig
}

var configManager = &ConfigManager{}

func init() {
    gconf.Init(
        gconf.WithWatchConfig(true),
        gconf.WithOnConfigChange(func(e fsnotify.Event) {
            var newConfig Config
            if err := gconf.Unmarshal(&newConfig); err != nil {
                log.Printf("重载配置失败: %v", err)
                return
            }
            configManager.Update(&newConfig)
            log.Println("配置已更新")
        }),
    )
    
    // 加载初始配置
    var initialConfig Config
    gconf.Unmarshal(&initialConfig)
    configManager.Update(&initialConfig)
}

// 在应用中使用
func someHandler() {
    config := configManager.Get()
    if config.App.Debug {
        // ...
    }
}
```

## 🔍 支持的配置格式

| 格式 | 扩展名 | 说明 |
|-----|--------|------|
| **YAML** | `.yaml`, `.yml` | 推荐使用，可读性好 |
| **JSON** | `.json` | 适合程序间交互 |
| **TOML** | `.toml` | 语义清晰 |
| **HCL** | `.hcl` | HashiCorp 配置语言 |
| **INI** | `.ini` | 简单的配置格式 |
| **ENV** | `.env` | 环境变量文件 |
| **Properties** | `.properties` | Java 风格配置 |

## 🆚 对比其他配置库

| 特性 | Gconf | Viper | Config |
|-----|-------|-------|--------|
| 易用性 | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐ |
| 功能完整性 | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ |
| 全局实例 | ✅ | ❌ | ✅ |
| 热更新 | ✅ | ✅ | ❌ |
| 环境变量 | ✅ | ✅ | ✅ |
| 文档完善度 | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ |

## ❓ 常见问题

### Q: 配置文件找不到怎么办？

A: Gconf 会按照指定的路径顺序查找配置文件。如果找不到，可以：
1. 使用 `WithDebug(true)` 查看详细日志
2. 确认配置文件路径和文件名是否正确
3. 即使没有配置文件，也可以使用默认值和环境变量

### Q: 如何在 Docker 容器中使用？

A: 推荐使用环境变量方式：
```go
gconf.Init(
    gconf.WithAutomaticEnv(true),
    gconf.WithEnvPrefix("APP"),
)
```

在 docker-compose.yml 或 Kubernetes ConfigMap 中设置环境变量。

### Q: 配置优先级是什么？

A: 优先级从高到低：
1. 环境变量
2. Set() 设置的值
3. 配置文件
4. SetDefault() 默认值

### Q: 支持远程配置中心吗？

A: 目前基于 Viper，支持 etcd、Consul 等远程配置中心。可以通过 `GetViper()` 方法访问底层 Viper 实例进行高级配置。

## 🤝 贡献

欢迎提交 Issue 和 Pull Request！

## 📄 开源协议

本项目采用 MIT 协议开源。详见 [LICENSE](LICENSE) 文件。

## 🙏 致谢

本项目基于优秀的 [spf13/viper](https://github.com/spf13/viper) 项目构建。

---

**如果这个项目对您有帮助，请给个 ⭐️ Star 支持一下！**

