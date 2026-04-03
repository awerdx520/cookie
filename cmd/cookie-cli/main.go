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
	getBrowser := getCmd.String("browser", "", "浏览器类型: chrome, firefox（默认 chrome）")

	listCmd := flag.NewFlagSet("list", flag.ExitOnError)
	listBrowser := listCmd.String("browser", "", "浏览器类型: chrome, firefox（默认 chrome）")

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
		handleGet(*getDomain, *getName, *getBrowser)
	case "list":
		listCmd.Parse(os.Args[2:])
		handleList(*listBrowser)
	case "serve":
		serveCmd.Parse(os.Args[2:])
		handleServe(*servePort)
	default:
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(`cookie-cli - 浏览器 Cookie 提取工具

用法:
  cookie-cli get -domain <域名> [-name <Cookie 名称>] [-browser <浏览器>]
  cookie-cli list [-browser <浏览器>]
  cookie-cli serve [-port <端口>]

子命令:
  get     获取指定域名的 Cookie
  list    列出所有可用的域名
  serve   启动 HTTP 服务

浏览器:
  chrome    Google Chrome（默认）
  firefox   Mozilla Firefox

示例:
  cookie-cli get -domain example.com
  cookie-cli get -domain example.com -name sessionid
  cookie-cli get -domain example.com -browser firefox
  cookie-cli list -browser firefox
  cookie-cli serve -port 8080`)
}

// newStore 根据浏览器类型创建对应的 Store
func newStore(browser string) (cookie.Store, error) {
	if browser == "" {
		browser = os.Getenv("COOKIE_BROWSER")
	}
	if browser == "" {
		browser = "chrome"
	}

	switch browser {
	case "chrome":
		return cookie.NewChromeStore()
	case "firefox":
		return cookie.NewFirefoxStore()
	default:
		return nil, fmt.Errorf("不支持的浏览器: %s（支持: chrome, firefox）", browser)
	}
}

func handleGet(domain, name, browser string) {
	store, err := newStore(browser)
	if err != nil {
		log.Fatalf("创建 Store 失败: %v", err)
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

func handleList(browser string) {
	store, err := newStore(browser)
	if err != nil {
		log.Fatalf("创建 Store 失败: %v", err)
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
