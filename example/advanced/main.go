package main

import (
	"fmt"
	"log"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/nicexiaonie/gconf"
)

// AppConfig 应用配置结构体
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

	Redis struct {
		Host     string `mapstructure:"host"`
		Port     int    `mapstructure:"port"`
		Password string `mapstructure:"password"`
		DB       int    `mapstructure:"db"`
	} `mapstructure:"redis"`

	Log struct {
		Level  string `mapstructure:"level"`
		Format string `mapstructure:"format"`
		Output string `mapstructure:"output"`
	} `mapstructure:"log"`
}

func main() {
	// 示例2: 高级用法 - 创建独立实例
	fmt.Println("=== 示例2: 高级用法 ===")

	// 创建配置实例，使用链式配置
	conf, err := gconf.New(
		gconf.WithConfigName("app"),
		gconf.WithConfigType("yaml"),
		gconf.WithConfigPaths(".", "./config", "/etc/myapp"),
		gconf.WithWatchConfig(true),        // 启用配置热更新
		gconf.WithAutomaticEnv(true),       // 自动读取环境变量
		gconf.WithEnvPrefix("MYAPP"),       // 环境变量前缀 MYAPP_
		gconf.WithEnvKeyReplacer(".", "_"), // 将配置键中的.替换为_用于环境变量
		gconf.WithDebug(true),              // 启用调试日志
		gconf.WithOnConfigChange(func(e fsnotify.Event) {
			log.Printf("配置文件发生变化: %s", e.Name)
		}),
	)

	if err != nil {
		log.Printf("创建配置实例失败: %v", err)
		// 即使配置文件不存在也继续执行，可以使用默认值
	}

	// 设置默认值
	setDefaults(conf)

	// 注册配置变化回调
	conf.OnConfigChange(func(e fsnotify.Event) {
		fmt.Printf(">>> 配置已更新，操作类型: %s\n", e.Op)
		// 在这里可以重新加载配置或执行其他操作
	})

	// 方式1: 直接读取配置
	fmt.Println("\n--- 方式1: 直接读取 ---")
	fmt.Printf("应用名称: %s\n", conf.GetString("app.name"))
	fmt.Printf("服务端口: %d\n", conf.GetInt("server.port"))
	fmt.Printf("数据库主机: %s\n", conf.GetString("database.host"))
	fmt.Printf("读取超时: %s\n", conf.GetDuration("server.read_timeout"))

	// 方式2: 解析到结构体
	fmt.Println("\n--- 方式2: 解析到结构体 ---")
	var config AppConfig
	if err := conf.Unmarshal(&config); err != nil {
		log.Printf("解析配置失败: %v", err)
	} else {
		fmt.Printf("应用配置: %+v\n", config.App)
		fmt.Printf("服务配置: %+v\n", config.Server)
		fmt.Printf("数据库配置: Host=%s:%d, DB=%s\n",
			config.Database.Host, config.Database.Port, config.Database.Database)
	}

	// 方式3: 解析子配置
	fmt.Println("\n--- 方式3: 解析子配置 ---")
	var serverConfig struct {
		Host         string        `mapstructure:"host"`
		Port         int           `mapstructure:"port"`
		ReadTimeout  time.Duration `mapstructure:"read_timeout"`
		WriteTimeout time.Duration `mapstructure:"write_timeout"`
	}
	if err := conf.UnmarshalKey("server", &serverConfig); err != nil {
		log.Printf("解析服务配置失败: %v", err)
	} else {
		fmt.Printf("服务器配置: %+v\n", serverConfig)
	}

	// 获取子配置树
	fmt.Println("\n--- 方式4: 使用子配置树 ---")
	dbConf := conf.Sub("database")
	if dbConf != nil {
		fmt.Printf("数据库驱动: %s\n", dbConf.GetString("driver"))
		fmt.Printf("数据库地址: %s:%d\n", dbConf.GetString("host"), dbConf.GetInt("port"))
	}

	// 获取各种类型的配置
	fmt.Println("\n--- 各种类型的配置 ---")
	fmt.Printf("字符串: %s\n", conf.GetString("app.name"))
	fmt.Printf("整数: %d\n", conf.GetInt("server.port"))
	fmt.Printf("布尔值: %v\n", conf.GetBool("app.debug"))
	fmt.Printf("浮点数: %.2f\n", conf.GetFloat64("app.version_float"))
	fmt.Printf("时间间隔: %s\n", conf.GetDuration("server.read_timeout"))

	// 字符串切片
	if conf.IsSet("app.features") {
		features := conf.GetStringSlice("app.features")
		fmt.Printf("功能列表: %v\n", features)
	}

	// Map 类型
	if conf.IsSet("app.metadata") {
		metadata := conf.GetStringMapString("app.metadata")
		fmt.Printf("元数据: %v\n", metadata)
	}

	// 检查配置是否存在
	fmt.Println("\n--- 检查配置是否存在 ---")
	fmt.Printf("app.name 是否存在: %v\n", conf.IsSet("app.name"))
	fmt.Printf("app.nonexistent 是否存在: %v\n", conf.IsSet("app.nonexistent"))

	// 获取所有配置键
	fmt.Println("\n--- 所有配置键 ---")
	allKeys := conf.AllKeys()
	fmt.Printf("配置键数量: %d\n", len(allKeys))
	if len(allKeys) > 0 {
		fmt.Printf("前5个配置键: %v\n", allKeys[:min(5, len(allKeys))])
	}

	// 使用别名
	fmt.Println("\n--- 配置别名 ---")
	conf.RegisterAlias("port", "server.port")
	fmt.Printf("使用别名获取端口: %d\n", conf.GetInt("port"))

	// 配置文件信息
	fmt.Println("\n--- 配置文件信息 ---")
	fmt.Printf("使用的配置文件: %s\n", conf.ConfigFileUsed())

	// 调试信息
	fmt.Println("\n--- 调试信息 ---")
	conf.Debug()

	fmt.Println("\n提示: 修改配置文件后会自动重新加载（如果启用了 WatchConfig）")
}

// setDefaults 设置默认配置值
func setDefaults(conf *gconf.Gconf) {
	// 应用配置
	conf.SetDefault("app.name", "MyApp")
	conf.SetDefault("app.version", "1.0.0")
	conf.SetDefault("app.debug", false)
	conf.SetDefault("app.version_float", 1.0)
	conf.SetDefault("app.features", []string{"feature1", "feature2"})
	conf.SetDefault("app.metadata", map[string]string{
		"author":  "gconf",
		"license": "MIT",
	})

	// 服务器配置
	conf.SetDefault("server.host", "0.0.0.0")
	conf.SetDefault("server.port", 8080)
	conf.SetDefault("server.read_timeout", "30s")
	conf.SetDefault("server.write_timeout", "30s")

	// 数据库配置
	conf.SetDefault("database.driver", "mysql")
	conf.SetDefault("database.host", "localhost")
	conf.SetDefault("database.port", 3306)
	conf.SetDefault("database.username", "root")
	conf.SetDefault("database.password", "")
	conf.SetDefault("database.database", "myapp")
	conf.SetDefault("database.max_conns", 100)

	// Redis配置
	conf.SetDefault("redis.host", "localhost")
	conf.SetDefault("redis.port", 6379)
	conf.SetDefault("redis.password", "")
	conf.SetDefault("redis.db", 0)

	// 日志配置
	conf.SetDefault("log.level", "info")
	conf.SetDefault("log.format", "json")
	conf.SetDefault("log.output", "stdout")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
