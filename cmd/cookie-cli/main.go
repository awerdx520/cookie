package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"cookie/internal/bridge"
	"cookie/internal/cookie"
)

func main() {
	getCmd := flag.NewFlagSet("get", flag.ExitOnError)
	getDomain := getCmd.String("domain", "", "域名")
	getName := getCmd.String("name", "", "Cookie 名称（可选）")
	getBrowser := getCmd.String("browser", "", "浏览器类型: chrome, firefox, edge（默认 chrome）")

	listCmd := flag.NewFlagSet("list", flag.ExitOnError)
	listBrowser := listCmd.String("browser", "", "浏览器类型: chrome, firefox, edge（默认 chrome）")

	serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
	servePort := serveCmd.String("port", "8008", "HTTP 服务端口")

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
  serve   启动 Cookie Bridge 服务（配合 Chrome 扩展使用）

浏览器:
  chrome    Google Chrome（默认，推荐配合 serve + 扩展使用）
  firefox   Mozilla Firefox
  edge      Microsoft Edge

Chrome/Edge Cookie 获取方式:
  1. 推荐: 先运行 cookie-cli serve，安装 Cookie Bridge 扩展，
     然后 get/list 命令会自动通过扩展获取明文 Cookie（无需关闭浏览器）
  2. 回退: 若 Bridge 服务不可用，会尝试直接读取数据库（需关闭浏览器）

示例:
  cookie-cli serve                              # 启动 Bridge 服务
  cookie-cli get -domain example.com            # 获取 Cookie
  cookie-cli get -domain example.com -name sid  # 获取特定 Cookie
  cookie-cli list                               # 列出所有域名
  curl http://127.0.0.1:8008/cookies?domain=example.com  # HTTP API`)
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
	case "edge":
		return cookie.NewEdgeStore()
	default:
		return nil, fmt.Errorf("不支持的浏览器: %s（支持: chrome, firefox, edge）", browser)
	}
}

func handleGet(domain, name, browser string) {
	if browser == "" {
		browser = os.Getenv("COOKIE_BROWSER")
	}
	if browser == "" {
		browser = "chrome"
	}

	// Chrome/Edge: 优先通过 bridge 服务获取（不需要解密，不需要关闭浏览器）
	if browser == "chrome" || browser == "edge" {
		cookies, err := getCookiesViaBridge(domain, name)
		if err == nil {
			printCookies(cookies, name)
			return
		}
		log.Printf("Bridge 服务不可用 (%v)，回退到直接读取数据库", err)
	}

	store, err := newStore(browser)
	if err != nil {
		log.Fatalf("创建 Store 失败: %v", err)
	}

	cookies, err := store.GetCookies(domain)
	if err != nil {
		log.Fatalf("获取 Cookie 失败: %v", err)
	}

	printCookies(cookiesToPairs(cookies), name)
}

type cookiePair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func printCookies(cookies []cookiePair, filterName string) {
	if filterName != "" {
		for _, c := range cookies {
			if c.Name == filterName {
				fmt.Println(c.Value)
				return
			}
		}
		fmt.Printf("未找到 Cookie: %s\n", filterName)
		os.Exit(1)
	}
	for _, c := range cookies {
		fmt.Printf("%s=%s\n", c.Name, c.Value)
	}
}

func cookiesToPairs(cookies []cookie.Cookie) []cookiePair {
	pairs := make([]cookiePair, len(cookies))
	for i, c := range cookies {
		pairs[i] = cookiePair{Name: c.Name, Value: c.Value}
	}
	return pairs
}

// getCookiesViaBridge 通过本地 bridge 服务获取 Cookie
func getCookiesViaBridge(domain, name string) ([]cookiePair, error) {
	port := os.Getenv("COOKIE_PORT")
	if port == "" {
		port = "8008"
	}

	u := fmt.Sprintf("http://127.0.0.1:%s/cookies?domain=%s", port, domain)
	if name != "" {
		u += "&name=" + name
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var result struct {
		OK      bool         `json:"ok"`
		Cookies []cookiePair `json:"cookies"`
		Error   string       `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, fmt.Errorf("%s", result.Error)
	}

	return result.Cookies, nil
}

func handleList(browser string) {
	if browser == "" {
		browser = os.Getenv("COOKIE_BROWSER")
	}
	if browser == "" {
		browser = "chrome"
	}

	if browser == "chrome" || browser == "edge" {
		domains, err := listDomainsViaBridge()
		if err == nil {
			for _, d := range domains {
				fmt.Println(d)
			}
			return
		}
		log.Printf("Bridge 服务不可用 (%v)，回退到直接读取数据库", err)
	}

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

func listDomainsViaBridge() ([]string, error) {
	port := os.Getenv("COOKIE_PORT")
	if port == "" {
		port = "8008"
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%s/domains", port))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		OK      bool     `json:"ok"`
		Domains []string `json:"domains"`
		Error   string   `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, fmt.Errorf("%s", result.Error)
	}

	return result.Domains, nil
}

func handleServe(port string) {
	s := bridge.NewServer("127.0.0.1:" + port)
	if err := s.ListenAndServe(); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}
