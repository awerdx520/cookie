package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"cookie/internal/bridge"
	"cookie/internal/cookie"
	"cookie/internal/native"
)

const defaultCacheExpireSeconds = 300

// exportMaxAgeSeconds 返回 ~/.cookie/export.json 作为回退来源时的最大允许年龄（秒）。
// 可通过环境变量 COOKIE_CACHE_EXPIRE 覆盖；未设置或非正整数时默认为 defaultCacheExpireSeconds。
// 设为 0 表示不限制导出文件年龄（与 native.ReadExportCookies 的 maxAge 语义一致）。
func exportMaxAgeSeconds() int {
	s := os.Getenv("COOKIE_CACHE_EXPIRE")
	if s == "" {
		return defaultCacheExpireSeconds
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return defaultCacheExpireSeconds
	}
	return n
}

// resolveExportMaxAge 确定 export.json 回退时的最大文件年龄（秒）。
// cliFlag 为 -1 时使用 COOKIE_CACHE_EXPIRE / 默认 300；否则以命令行值为准（0 表示不限制）。
func resolveExportMaxAge(cliFlag int) int {
	if cliFlag >= 0 {
		return cliFlag
	}
	return exportMaxAgeSeconds()
}

func main() {
	getCmd := flag.NewFlagSet("get", flag.ExitOnError)
	getDomain := getCmd.String("domain", "", "域名")
	getName := getCmd.String("name", "", "Cookie 名称（可选）")
	getBrowser := getCmd.String("browser", "", "浏览器类型: chrome, firefox, edge（默认 chrome）")
	getFormat := getCmd.String("format", "", "输出格式: 默认 name=value 逐行，header 输出为 Cookie 头格式")
	getCacheExpire := getCmd.Int("cache-expire", -1, "export.json 回退最大文件年龄（秒）；-1 使用 COOKIE_CACHE_EXPIRE 或默认 300；0 不限制")

	listCmd := flag.NewFlagSet("list", flag.ExitOnError)
	listBrowser := listCmd.String("browser", "", "浏览器类型: chrome, firefox, edge（默认 chrome）")
	listCacheExpire := listCmd.Int("cache-expire", -1, "export.json 回退最大文件年龄（秒）；-1 使用 COOKIE_CACHE_EXPIRE 或默认 300；0 不限制")

	serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
	servePort := serveCmd.String("port", "8008", "HTTP 服务端口")

	exportCmd := flag.NewFlagSet("export", flag.ExitOnError)
	exportDomain := exportCmd.String("domain", "", "要导出的域名（留空导出全部）")

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
		handleGet(*getDomain, *getName, *getBrowser, *getFormat, resolveExportMaxAge(*getCacheExpire))
	case "list":
		listCmd.Parse(os.Args[2:])
		handleList(*listBrowser, resolveExportMaxAge(*listCacheExpire))
	case "serve":
		serveCmd.Parse(os.Args[2:])
		handleServe(*servePort)
	case "native-messaging-host":
		handleNativeMessagingHost()
	case "export":
		exportCmd.Parse(os.Args[2:])
		handleExport(*exportDomain)
	default:
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(`cookie-cli - 浏览器 Cookie 提取工具

用法:
  cookie-cli get -domain <域名> [-name <名称>] [-browser <浏览器>] [-format <格式>] [-cache-expire <秒>]
  cookie-cli list [-browser <浏览器>] [-cache-expire <秒>]
  cookie-cli serve [-port <端口>]
  cookie-cli export [-domain <域名>]
  cookie-cli native-messaging-host

子命令:
  get                     获取指定域名的 Cookie
  list                    列出所有可用的域名
  serve                   启动 Cookie Bridge HTTP + WebSocket 服务
  export                  通过 Native Messaging 导出 Cookie 到本地文件
  native-messaging-host   作为 Chrome Native Messaging Host 运行（由扩展自动启动）

浏览器:
  chrome    Google Chrome（默认）
  firefox   Mozilla Firefox
  edge      Microsoft Edge

输出格式 (-format):
  (默认)    每行一个 name=value
  header    Cookie 头格式: name1=val1; name2=val2
  json      JSON 数组格式

Cookie 获取优先级 (Chrome/Edge):
  1. Native Messaging (扩展自动启动 native-messaging-host，无需 serve)
  2. Bridge HTTP 服务 (cookie-cli serve)
  3. 本地导出文件 (~/.cookie/export.json)
  4. 直接读取 SQLite 数据库（需关闭浏览器）

示例:
  cookie-cli get -domain example.com                      # 获取 Cookie（自动选择最佳方式）
  cookie-cli get -domain example.com -name sid            # 获取特定 Cookie 值
  cookie-cli get -domain example.com -format header       # 输出为 Cookie 头格式
  cookie-cli get -domain example.com -cache-expire 600   # export 回退文件最长 600 秒内有效
  cookie-cli list                                         # 列出所有域名
  cookie-cli export -domain example.com                   # 导出 Cookie 到本地文件
  cookie-cli serve                                        # 启动 Bridge 服务

HTTP API (serve 模式):
  curl 'http://127.0.0.1:8008/cookies?domain=example.com'
  curl 'http://127.0.0.1:8008/cookies?domain=example.com&format=header'
  curl 'http://127.0.0.1:8008/cookies?domain=example.com&format=raw'`)
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

func handleGet(domain, name, browser, format string, exportMaxAge int) {
	if browser == "" {
		browser = os.Getenv("COOKIE_BROWSER")
	}
	if browser == "" {
		browser = "chrome"
	}

	if browser == "chrome" || browser == "edge" {
		// 优先级 1: Native Messaging unix socket
		nativeCookies, err := native.GetCookiesViaSocket(domain, name)
		if err == nil {
			printCookies(nativePairsToCookiePairs(nativeCookies), name, format)
			return
		}
		log.Printf("Native Messaging 不可用 (%v)，尝试 Bridge HTTP", err)

		// 优先级 2: Bridge HTTP 服务
		cookies, err := getCookiesViaBridge(domain, name)
		if err == nil {
			printCookies(cookies, name, format)
			return
		}
		log.Printf("Bridge 服务不可用 (%v)，尝试导出文件", err)

		// 优先级 3: 本地导出文件
		exportCookies, err := native.ReadExportCookies(domain, exportMaxAge)
		if err == nil {
			printCookies(nativePairsToCookiePairs(exportCookies), name, format)
			return
		}
		log.Printf("导出文件不可用 (%v)，回退到数据库直读", err)
	}

	// 优先级 4: SQLite 数据库直读
	store, err := newStore(browser)
	if err != nil {
		log.Fatalf("创建 Store 失败: %v", err)
	}

	cookies, err := store.GetCookies(domain)
	if err != nil {
		log.Fatalf("获取 Cookie 失败: %v", err)
	}

	printCookies(cookiesToPairs(cookies), name, format)
}

func nativePairsToCookiePairs(pairs []native.CookiePair) []cookiePair {
	result := make([]cookiePair, len(pairs))
	for i, p := range pairs {
		result[i] = cookiePair{Name: p.Name, Value: p.Value}
	}
	return result
}

type cookiePair struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func printCookies(cookies []cookiePair, filterName, format string) {
	if filterName != "" {
		for _, c := range cookies {
			if c.Name == filterName {
				fmt.Print(c.Value)
				return
			}
		}
		fmt.Fprintf(os.Stderr, "未找到 Cookie: %s\n", filterName)
		os.Exit(1)
	}

	switch format {
	case "header":
		parts := make([]string, len(cookies))
		for i, c := range cookies {
			parts[i] = c.Name + "=" + c.Value
		}
		fmt.Print(strings.Join(parts, "; "))
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.Encode(cookies)
	default:
		for _, c := range cookies {
			fmt.Printf("%s=%s\n", c.Name, c.Value)
		}
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

func handleList(browser string, exportMaxAge int) {
	if browser == "" {
		browser = os.Getenv("COOKIE_BROWSER")
	}
	if browser == "" {
		browser = "chrome"
	}

	if browser == "chrome" || browser == "edge" {
		// 优先级 1: Native Messaging
		domains, err := native.ListDomainsViaSocket()
		if err == nil {
			for _, d := range domains {
				fmt.Println(d)
			}
			return
		}
		log.Printf("Native Messaging 不可用 (%v)，尝试 Bridge HTTP", err)

		// 优先级 2: Bridge HTTP
		domains, err = listDomainsViaBridge()
		if err == nil {
			for _, d := range domains {
				fmt.Println(d)
			}
			return
		}
		log.Printf("Bridge 服务不可用 (%v)，尝试导出文件", err)

		// 优先级 3: 导出文件
		domains, err = native.ReadExportDomains(exportMaxAge)
		if err == nil {
			for _, d := range domains {
				fmt.Println(d)
			}
			return
		}
		log.Printf("导出文件不可用 (%v)，回退到数据库直读", err)
	}

	// 优先级 4: SQLite 直读
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

func handleNativeMessagingHost() {
	if err := native.RunHost(); err != nil {
		log.Fatalf("Native Messaging Host 启动失败: %v", err)
	}
}

func handleExport(domain string) {
	err := native.ExportCookiesViaSocket(domain)
	if err != nil {
		log.Fatalf("导出 Cookie 失败: %v", err)
	}
	path, _ := native.ExportFilePath()
	fmt.Printf("Cookie 已导出到 %s\n", path)
}
