package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"cookie/internal/cookie"
)

func main() {
	getCmd := flag.NewFlagSet("get", flag.ExitOnError)
	getDomain := getCmd.String("domain", "", "域名")
	getName := getCmd.String("name", "", "Cookie 名称（可选）")

	listCmd := flag.NewFlagSet("list", flag.ExitOnError)

	serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
	servePort := serveCmd.String("port", "8080", "HTTP 服务端口")

	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "get":
		getCmd.Parse(os.Args[2:])
		if *getDomain == "" {
			getCmd.PrintDefaults()
			os.Exit(1)
		}
		handleGet(*getDomain, *getName)
	case "list":
		listCmd.Parse(os.Args[2:])
		handleList()
	case "serve":
		serveCmd.Parse(os.Args[2:])
		handleServe(*servePort)
	default:
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(`cookie-cli - Chrome Cookie 提取工具

用法:
  cookie-cli get -domain <域名> [-name <Cookie 名称>]
  cookie-cli list
  cookie-cli serve [-port <端口>]

子命令:
  get     获取指定域名的 Cookie
  list    列出所有可用的域名
  serve   启动 HTTP 服务

示例:
  cookie-cli get -domain example.com
  cookie-cli get -domain example.com -name sessionid
  cookie-cli serve -port 8080`)
}

func handleGet(domain, name string) {
	store, err := cookie.NewChromeStore()
	if err != nil {
		log.Fatalf("创建 Chrome Store 失败: %v", err)
	}

	cookies, err := store.GetCookies(domain)
	if err != nil {
		log.Fatalf("获取 Cookie 失败: %v", err)
	}

	if name != "" {
		for _, c := range cookies {
			if c.Name == name {
				fmt.Println(c.Value)
				return
			}
		}
		fmt.Printf("未找到 Cookie: %s\n", name)
		os.Exit(1)
	} else {
		for _, c := range cookies {
			fmt.Printf("%s=%s\n", c.Name, c.Value)
		}
	}
}

func handleList() {
	store, err := cookie.NewChromeStore()
	if err != nil {
		log.Fatalf("创建 Chrome Store 失败: %v", err)
	}

	domains, err := store.ListDomains()
	if err != nil {
		log.Fatalf("获取域名列表失败: %v", err)
	}

	for _, domain := range domains {
		fmt.Println(domain)
	}
}

func handleServe(port string) {
	fmt.Printf("HTTP 服务启动中，端口 %s...\n", port)
	// TODO: 实现 HTTP 服务
	fmt.Println("功能尚未实现")
	os.Exit(1)
}
