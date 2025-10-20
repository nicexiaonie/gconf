package main

import (
	"fmt"
	"log"

	"github.com/nicexiaonie/gconf"
)

func main() {
	fmt.Println("=== Gconf 测试示例 ===")

	// 创建配置实例
	conf, err := gconf.New(
		gconf.WithConfigName("test"),
		gconf.WithConfigType("yaml"),
		gconf.WithConfigPaths("./"),
		gconf.WithDebug(true),
	)
	if err != nil {
		log.Printf("创建配置实例失败: %v", err)
	}

	// 设置并写入配置
	fmt.Println("1. 设置配置值")
	conf.Set("aa", "cc")
	conf.Set("ab", "dda")

	err = conf.WriteConfig()
	if err != nil {
		log.Printf("写入配置失败: %v", err)
	} else {
		fmt.Println("配置已写入文件")
	}

	// 使用别名
	fmt.Println("\n2. 测试配置别名")
	conf.RegisterAlias("loud", "Verbose")
	conf.Set("loud", "aaa")
	fmt.Printf("通过别名 'loud' 设置，通过 'Verbose' 读取: %v\n", conf.Get("Verbose"))

	// 读取环境变量
	fmt.Println("\n3. 测试环境变量")
	// 创建新实例以启用环境变量
	confWithEnv, _ := gconf.New(
		gconf.WithConfigName("test"),
		gconf.WithConfigPaths("./"),
		gconf.WithAutomaticEnv(true),
	)

	fmt.Printf("PATH 环境变量: %v\n", confWithEnv.Get("PATH"))

	// 读取配置
	fmt.Println("\n4. 读取已保存的配置")
	fmt.Printf("aa = %v\n", conf.Get("aa"))
	fmt.Printf("ab = %v\n", conf.Get("ab"))

	// 显示所有配置
	fmt.Println("\n5. 所有配置键:")
	for _, key := range conf.AllKeys() {
		fmt.Printf("  %s = %v\n", key, conf.Get(key))
	}

	fmt.Println("\n=== 测试完成 ===")
}
