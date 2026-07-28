package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nicexiaonie/gconf/apollo"
)

func main() {
	dir := "./apollo-demo-output"
	os.RemoveAll(dir)

	syncer, err := apollo.New(apollo.Config{
		AppID:      "Conduit-Service",
		Cluster:    "default",
		MetaServer: "http://159.75.184.171:32369",
		CacheDir:   dir,
		Watch:      true,
		Namespaces: []apollo.NamespaceConfig{
			{Name: "config.json", Required: true, Format: apollo.FormatYAML},
			{Name: "config.json", Required: true, Format: apollo.FormatXML},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := syncer.Start(context.Background()); err != nil {
		log.Fatal(err)
	}
	defer syncer.Close()

	st := syncer.Status()
	for _, ns := range st.Namespaces {
		fmt.Printf("[%s] 启动: namespace=%s source=%s hash=%s\n",
			time.Now().Format("15:04:05"), ns.Namespace, ns.Source, ns.Hash)
	}
	fmt.Println("监听变更中... 在 Apollo 修改 config.json，YAML 与 XML 都会更新（Ctrl+C 退出）")

	lastSig := statusSig(st)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			st := syncer.Status()
			if sig := statusSig(st); sig != lastSig {
				lastSig = sig
				fmt.Printf("\n[%s] 检测到配置更新:\n", time.Now().Format("15:04:05"))
				for _, ns := range st.Namespaces {
					fmt.Printf("  namespace=%s hash=%s\n", ns.Namespace, ns.Hash)
				}
				printFile(dir + "/config.json/config.yaml")
				printFile(dir + "/config.json/config.xml")
			}
		case s := <-sig:
			fmt.Printf("\n收到信号 %v，退出\n", s)
			return
		}
	}
}

func statusSig(st apollo.Status) string {
	sig := ""
	for _, ns := range st.Namespaces {
		sig += ns.Hash + "|"
	}
	return sig
}

func printFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Println("读取失败:", path, err)
		return
	}
	s := string(data)
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	fmt.Printf("--- %s ---\n%s\n", path, s)
}
