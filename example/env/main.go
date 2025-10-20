package main

import (
	"fmt"
	"log"
	"os"

	"github.com/nicexiaonie/gconf"
)

func main() {
	fmt.Println("=== 示例3: 环境变量集成 ===")

	// 设置一些环境变量用于演示
	os.Setenv("MYAPP_APP_NAME", "MyApp from ENV")
	os.Setenv("MYAPP_SERVER_PORT", "9090")
	os.Setenv("MYAPP_DATABASE_HOST", "db.example.com")
	os.Setenv("MYAPP_DEBUG", "true")

	// 创建配置实例，启用环境变量
	conf, err := gconf.New(
		gconf.WithConfigName("config"),
		gconf.WithConfigType("yaml"),
		gconf.WithConfigPaths("."),
		gconf.WithAutomaticEnv(true),       // 自动读取环境变量
		gconf.WithEnvPrefix("MYAPP"),       // 环境变量前缀
		gconf.WithEnvKeyReplacer(".", "_"), // 配置键中的.替换为_
		gconf.WithDebug(true),
	)

	if err != nil {
		log.Printf("初始化配置失败: %v", err)
	}

	// 设置默认值
	conf.SetDefault("app.name", "DefaultApp")
	conf.SetDefault("server.port", 8080)
	conf.SetDefault("database.host", "localhost")
	conf.SetDefault("debug", false)

	// 读取配置（优先级：环境变量 > 配置文件 > 默认值）
	fmt.Println("\n配置读取结果（环境变量会覆盖配置文件和默认值）:")
	fmt.Printf("app.name = %s (来自环境变量: MYAPP_APP_NAME)\n", conf.GetString("app.name"))
	fmt.Printf("server.port = %d (来自环境变量: MYAPP_SERVER_PORT)\n", conf.GetInt("server.port"))
	fmt.Printf("database.host = %s (来自环境变量: MYAPP_DATABASE_HOST)\n", conf.GetString("database.host"))
	fmt.Printf("debug = %v (来自环境变量: MYAPP_DEBUG)\n", conf.GetBool("debug"))

	// 绑定特定的环境变量
	fmt.Println("\n--- 绑定特定的环境变量 ---")
	os.Setenv("CUSTOM_TOKEN", "secret-token-123")
	conf.BindEnv("api.token", "CUSTOM_TOKEN")
	fmt.Printf("api.token = %s (绑定到环境变量: CUSTOM_TOKEN)\n", conf.GetString("api.token"))

	// 展示配置优先级
	fmt.Println("\n--- 配置优先级演示 ---")

	// 1. 仅有默认值
	conf.SetDefault("priority.test1", "default")
	fmt.Printf("test1 (仅默认值): %s\n", conf.GetString("priority.test1"))

	// 2. 设置运行时值（会覆盖默认值）
	conf.SetDefault("priority.test2", "default")
	conf.Set("priority.test2", "runtime")
	fmt.Printf("test2 (运行时值覆盖默认值): %s\n", conf.GetString("priority.test2"))

	// 3. 环境变量（会覆盖一切）
	conf.SetDefault("priority.test3", "default")
	conf.Set("priority.test3", "runtime")
	os.Setenv("MYAPP_PRIORITY_TEST3", "env")
	fmt.Printf("test3 (环境变量覆盖一切): %s\n", conf.GetString("priority.test3"))

	fmt.Println("\n提示:")
	fmt.Println("- 配置优先级: 环境变量 > Set设置的值 > 配置文件 > 默认值")
	fmt.Println("- 使用环境变量可以方便地在不同环境（开发/测试/生产）使用不同配置")
	fmt.Println("- 特别适合容器化部署（Docker/Kubernetes）场景")
}
