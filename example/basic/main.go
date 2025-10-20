package main

import (
	"fmt"
	"log"

	"github.com/nicexiaonie/gconf"
)

func main() {
	// 示例1: 最简单的使用方式 - 使用全局实例
	fmt.Println("=== 示例1: 使用全局实例 ===")

	// 初始化全局配置（使用默认配置：从当前目录和./config目录查找config.yaml）
	err := gconf.Init(
		gconf.WithConfigName("config"),
		gconf.WithConfigType("yaml"),
		gconf.WithConfigPaths(".", "./config"),
	)
	if err != nil {
		log.Printf("初始化配置失败（将使用默认值和环境变量）: %v", err)
	}

	// 设置默认值
	gconf.SetDefault("app.name", "MyApp")
	gconf.SetDefault("app.version", "1.0.0")
	gconf.SetDefault("server.port", 8080)
	gconf.SetDefault("server.host", "localhost")

	// 获取配置
	appName := gconf.GetString("app.name")
	port := gconf.GetInt("server.port")

	fmt.Printf("应用名称: %s\n", appName)
	fmt.Printf("服务端口: %d\n", port)

	// 动态设置配置
	gconf.Set("app.debug", true)
	fmt.Printf("调试模式: %v\n", gconf.GetBool("app.debug"))

	fmt.Println()
}
