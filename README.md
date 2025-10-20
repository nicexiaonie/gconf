[toc]

# Gconf - 强大的 Go 配置管理工具

<div align="center">

[![Go Version](https://img.shields.io/badge/Go-%3E%3D%201.14-blue)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

**Gconf** 是一个基于 [Viper](https://github.com/spf13/viper) 的配置管理工具封装，提供开箱即用、功能强大且简洁的配置管理解决方案。

</div>

## ✨ 特性

- 🚀 **开箱即用** - 零配置快速启动，提供合理的默认值
- 🔧 **功能全面** - 支持多种配置格式（YAML、JSON、TOML、HCL、INI 等）
- 🌍 **环境变量** - 自动读取和绑定环境变量，完美支持容器化部署
- 🔄 **热更新** - 支持配置文件监听和自动重载
- 🎯 **类型安全** - 提供完整的类型转换方法
- 📦 **结构体解析** - 支持将配置直接解析到结构体
- 🌳 **配置树** - 支持嵌套配置和子配置树访问
- 🔌 **灵活配置** - 使用 Options 模式，配置灵活简洁
- 🌐 **全局实例** - 提供单例模式的全局配置访问
- ⚡ **高性能** - 基于 Viper，性能卓越
- 🔒 **线程安全** - 内置并发安全机制

## 📦 安装

```bash
go get -u github.com/nicexiaonie/gconf
```

## 🚀 快速开始

### 最简单的使用方式

```go
package main

import (
    "fmt"
    "github.com/nicexiaonie/gconf"
)

func main() {
    // 初始化全局配置（自动查找 config.yaml）
    gconf.Init()
    
    // 设置默认值
    gconf.SetDefault("app.name", "MyApp")
    gconf.SetDefault("server.port", 8080)
    
    // 读取配置
    fmt.Println("App Name:", gconf.GetString("app.name"))
    fmt.Println("Port:", gconf.GetInt("server.port"))
}
```

### 使用配置文件

创建 `config.yaml`:

```yaml
app:
  name: "MyApp"
  version: "1.0.0"
  debug: false

server:
  host: "0.0.0.0"
  port: 8080
  read_timeout: "30s"
  write_timeout: "30s"

database:
  driver: "mysql"
  host: "localhost"
  port: 3306
  username: "root"
  password: "password"
  database: "myapp"
```

读取配置：

```go
package main

import (
    "fmt"
    "github.com/nicexiaonie/gconf"
)

func main() {
    // 初始化配置
    err := gconf.Init(
        gconf.WithConfigName("config"),
        gconf.WithConfigType("yaml"),
        gconf.WithConfigPaths(".", "./config"),
    )
    if err != nil {
        panic(err)
    }
    
    // 读取配置
    fmt.Println("App Name:", gconf.GetString("app.name"))
    fmt.Println("Server Port:", gconf.GetInt("server.port"))
    fmt.Println("DB Host:", gconf.GetString("database.host"))
}
```

## 📖 使用文档

### 创建配置实例

#### 方式1: 使用全局实例（推荐）

```go
// 初始化全局配置
err := gconf.Init(
    gconf.WithConfigName("config"),
    gconf.WithConfigType("yaml"),
    gconf.WithConfigPaths("."),
)

// 在任何地方使用
port := gconf.GetInt("server.port")
```

#### 方式2: 创建独立实例

```go
conf, err := gconf.New(
    gconf.WithConfigName("app"),
    gconf.WithConfigType("yaml"),
    gconf.WithConfigPaths("./config"),
)

port := conf.GetInt("server.port")
```

### 配置选项

Gconf 使用 Options 模式提供灵活的配置：

```go
conf, err := gconf.New(
    // 配置文件名（不含扩展名）
    gconf.WithConfigName("config"),
    
    // 配置文件类型
    gconf.WithConfigType("yaml"),
    
    // 配置文件搜索路径（支持多个）
    gconf.WithConfigPaths(".", "./config", "/etc/myapp"),
    
    // 启用配置文件监听和热更新
    gconf.WithWatchConfig(true),
    
    // 自动读取环境变量
    gconf.WithAutomaticEnv(true),
    
    // 环境变量前缀
    gconf.WithEnvPrefix("MYAPP"),
    
    // 环境变量键替换规则（将 . 替换为 _）
    gconf.WithEnvKeyReplacer(".", "_"),
    
    // 配置变化回调
    gconf.WithOnConfigChange(func(e fsnotify.Event) {
        log.Println("配置已更新:", e.Name)
    }),
    
    // 启用调试模式
    gconf.WithDebug(true),
)
```

### 读取配置

#### 基本类型

```go
// 字符串
name := conf.GetString("app.name")

// 整数
port := conf.GetInt("server.port")
port32 := conf.GetInt32("server.port")
port64 := conf.GetInt64("server.port")

// 无符号整数
count := conf.GetUint("app.count")

// 浮点数
pi := conf.GetFloat64("math.pi")

// 布尔值
debug := conf.GetBool("app.debug")

// 时间间隔
timeout := conf.GetDuration("server.timeout") // 支持 "30s", "1m", "1h" 等

// 任意类型
value := conf.Get("app.custom")
```

#### 复杂类型

```go
// 字符串切片
features := conf.GetStringSlice("app.features")

// 字符串映射
metadata := conf.GetStringMap("app.metadata")

// 字符串到字符串映射
labels := conf.GetStringMapString("app.labels")

// 字符串到字符串切片映射
tags := conf.GetStringMapStringSlice("app.tags")

// 字节大小（支持 "1KB", "1MB", "1GB" 等）
maxSize := conf.GetSizeInBytes("upload.max_size")
```

### 解析到结构体

```go
type Config struct {
    App struct {
        Name    string `mapstructure:"name"`
        Version string `mapstructure:"version"`
        Debug   bool   `mapstructure:"debug"`
    } `mapstructure:"app"`
    
    Server struct {
        Host string `mapstructure:"host"`
        Port int    `mapstructure:"port"`
    } `mapstructure:"server"`
}

var config Config

// 解析全部配置
err := conf.Unmarshal(&config)

// 解析指定键的配置
var serverConfig struct {
    Host string `mapstructure:"host"`
    Port int    `mapstructure:"port"`
}
err := conf.UnmarshalKey("server", &serverConfig)

// 严格解析（结构体未定义的字段会报错）
err := conf.UnmarshalExact(&config)
```

### 设置和修改配置

```go
// 设置默认值（优先级最低）
conf.SetDefault("app.name", "DefaultApp")

// 设置配置值（运行时）
conf.Set("app.debug", true)

// 检查配置是否存在
if conf.IsSet("app.name") {
    // ...
}

// 获取所有配置键
keys := conf.AllKeys()

// 获取所有配置
settings := conf.AllSettings()
```

### 环境变量集成

```go
// 方式1: 自动读取环境变量
conf, _ := gconf.New(
    gconf.WithAutomaticEnv(true),
    gconf.WithEnvPrefix("MYAPP"),
    gconf.WithEnvKeyReplacer(".", "_"),
)

// 配置键 app.name 会自动读取环境变量 MYAPP_APP_NAME
name := conf.GetString("app.name")

// 方式2: 绑定特定的环境变量
conf.BindEnv("api.token", "API_TOKEN")
token := conf.GetString("api.token")
```

**配置优先级**（从高到低）：
1. 环境变量
2. `Set()` 设置的值
3. 配置文件
4. `SetDefault()` 设置的默认值

### 配置文件监听和热更新

```go
conf, _ := gconf.New(
    gconf.WithConfigName("config"),
    gconf.WithWatchConfig(true),
    gconf.WithOnConfigChange(func(e fsnotify.Event) {
        log.Println("配置文件已更新:", e.Name)
        // 在这里处理配置更新逻辑
        // 例如：重新加载配置到结构体
    }),
)

// 也可以注册多个回调
conf.OnConfigChange(func(e fsnotify.Event) {
    log.Println("另一个回调:", e.Name)
})
```

### 写入配置文件

```go
// 修改配置
conf.Set("app.version", "2.0.0")

// 写入到当前配置文件
err := conf.WriteConfig()

// 安全写入（文件存在时不覆盖）
err := conf.SafeWriteConfig()

// 写入到指定文件
err := conf.WriteConfigAs("/path/to/config.yaml")

// 安全写入到指定文件
err := conf.SafeWriteConfigAs("/path/to/config.yaml")
```

### 子配置树

```go
// 设置嵌套配置
conf.Set("database.host", "localhost")
conf.Set("database.port", 3306)

// 获取子配置树
dbConf := conf.Sub("database")
host := dbConf.GetString("host")
port := dbConf.GetInt("port")
```

### 配置别名

```go
// 为长配置键注册别名
conf.RegisterAlias("port", "server.port")

// 使用别名访问
port := conf.GetInt("port")
```

### 高级功能

#### 合并配置文件

```go
conf, _ := gconf.New(gconf.WithConfigName("base"))
// 合并其他配置文件
conf.MergeInConfig()
```

#### 重新加载配置

```go
err := conf.ReadInConfig()
```

#### 获取 Viper 实例（用于高级操作）

```go
viper := conf.GetViper()
// 使用 viper 的高级功能
```

#### 调试

```go
// 打印所有配置信息
conf.Debug()
```

## 📝 完整示例

### 示例1: Web 应用配置

```go
package main

import (
    "fmt"
    "log"
    "time"
    
    "github.com/nicexiaonie/gconf"
)

type AppConfig struct {
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
        gconf.WithDebug(true),
    )
    if err != nil {
        log.Fatal(err)
    }
    
    // 设置默认值
    setDefaults()
    
    // 解析配置
    var config AppConfig
    if err := gconf.Unmarshal(&config); err != nil {
        log.Fatal(err)
    }
    
    // 使用配置
    fmt.Printf("服务启动在 %s:%d\n", config.Server.Host, config.Server.Port)
    fmt.Printf("数据库连接: %s@%s:%d/%s\n", 
        config.Database.Username,
        config.Database.Host,
        config.Database.Port,
        config.Database.Database)
}

func setDefaults() {
    gconf.SetDefault("server.host", "0.0.0.0")
    gconf.SetDefault("server.port", 8080)
    gconf.SetDefault("server.read_timeout", "30s")
    gconf.SetDefault("server.write_timeout", "30s")
    
    gconf.SetDefault("database.driver", "mysql")
    gconf.SetDefault("database.host", "localhost")
    gconf.SetDefault("database.port", 3306)
}
```

### 示例2: 微服务配置

配置文件 `config.yaml`:

```yaml
service:
  name: "user-service"
  version: "1.0.0"
  port: 8080

registry:
  type: "consul"
  address: "localhost:8500"

tracing:
  enabled: true
  endpoint: "http://jaeger:14268/api/traces"

logging:
  level: "info"
  format: "json"
```

Go 代码:

```go
package main

import (
    "github.com/nicexiaonie/gconf"
)

func main() {
    // 初始化配置，支持多环境
    env := os.Getenv("ENV")
    if env == "" {
        env = "development"
    }
    
    err := gconf.Init(
        gconf.WithConfigName(fmt.Sprintf("config.%s", env)),
        gconf.WithConfigPaths(".", "./configs"),
        gconf.WithAutomaticEnv(true),
        gconf.WithEnvPrefix("SERVICE"),
    )
    if err != nil {
        // 配置文件不存在时，使用默认配置
        log.Printf("使用默认配置: %v", err)
    }
    
    // 读取配置
    serviceName := gconf.GetString("service.name")
    port := gconf.GetInt("service.port")
    tracingEnabled := gconf.GetBool("tracing.enabled")
    
    // 启动服务
    startService(serviceName, port, tracingEnabled)
}
```

## 🎯 最佳实践

### 1. 使用环境变量管理敏感信息

```go
// config.yaml 不包含密码等敏感信息
database:
  host: "localhost"
  port: 3306
  username: "root"
  # password 通过环境变量提供

// 代码中
gconf.Init(
    gconf.WithAutomaticEnv(true),
    gconf.WithEnvPrefix("APP"),
)

// 从环境变量读取密码: APP_DATABASE_PASSWORD
password := gconf.GetString("database.password")
```

### 2. 多环境配置

```go
// 根据环境加载不同配置文件
env := os.Getenv("ENV")
if env == "" {
    env = "development"
}

gconf.Init(
    gconf.WithConfigName(fmt.Sprintf("config.%s", env)),
    gconf.WithConfigPaths("./config"),
)
```

### 3. 配置验证

```go
type Config struct {
    Server struct {
        Port int `mapstructure:"port" validate:"required,min=1024,max=65535"`
    } `mapstructure:"server"`
}

var config Config
gconf.Unmarshal(&config)

// 使用 validator 验证
validate := validator.New()
if err := validate.Struct(config); err != nil {
    log.Fatal("配置验证失败:", err)
}
```

### 4. 配置热更新处理

```go
var config AppConfig

gconf.Init(
    gconf.WithWatchConfig(true),
    gconf.WithOnConfigChange(func(e fsnotify.Event) {
        // 重新加载配置
        if err := gconf.Unmarshal(&config); err != nil {
            log.Printf("重新加载配置失败: %v", err)
            return
        }
        log.Println("配置已热更新")
        
        // 执行相应的更新操作
        updateComponents(&config)
    }),
)
```

## 🔍 支持的配置格式

- **YAML** - `config.yaml`, `config.yml`
- **JSON** - `config.json`
- **TOML** - `config.toml`
- **HCL** - `config.hcl`
- **INI** - `config.ini`
- **ENV** - `.env`
- **Properties** - `config.properties`

## 🤝 贡献

欢迎贡献代码、报告问题和提出建议！

## 📄 许可证

本项目采用 MIT 许可证。详见 [LICENSE](LICENSE) 文件。

## 🙏 致谢

本项目基于 [spf13/viper](https://github.com/spf13/viper) 构建，感谢 Viper 提供的强大基础。

## 📞 联系方式

- GitHub: [https://github.com/nicexiaonie/gconf](https://github.com/nicexiaonie/gconf)
- Issues: [https://github.com/nicexiaonie/gconf/issues](https://github.com/nicexiaonie/gconf/issues)

---

**如果觉得这个项目有帮助，请给一个 ⭐️ Star！**

