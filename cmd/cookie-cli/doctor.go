package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"cookie/internal/cookie"
	"cookie/internal/native"
)

// handleDoctor 诊断各 Cookie 获取模式。
// browser 为空时默认 chrome。
func handleDoctor(browser string) {
	if browser == "" {
		browser = os.Getenv("COOKIE_BROWSER")
	}
	if browser == "" {
		browser = "chrome"
	}

	fmt.Println("=== Cookie 获取模式诊断 ===")
	fmt.Printf("浏览器: %s\n", browser)

	if browser != "chrome" && browser != "firefox" && browser != "edge" {
		fmt.Printf("\n错误: 不支持的浏览器: %s（支持: chrome, firefox, edge）\n", browser)
		return
	}
	fmt.Println()

	// [1/5] Native Messaging
	nmOK, nmMsg := checkNativeMessaging()
	fmt.Printf("[1/5] Native Messaging ..... %s\n", nmMsg)

	// [2/5] Bridge HTTP
	brOK, brMsg := checkBridgeHTTP()
	fmt.Printf("[2/5] Bridge HTTP .......... %s\n", brMsg)

	// [3/5] 文件导出
	exOK, exMsg := checkExportFile()
	fmt.Printf("[3/5] 文件导出 ............. %s\n", exMsg)

	// [4/5] SQLite 直读
	sqOK, sqMsg := checkSQLite(browser)
	fmt.Printf("[4/5] SQLite 直读 .......... %s\n", sqMsg)

	// [5/5] manifest 扩展 ID
	mfWarn, mfMsg := checkManifestExtensionID()
	fmt.Printf("[5/5] manifest 扩展 ID ..... %s\n", mfMsg)

	// 建议
	fmt.Println()
	fmt.Print(buildSuggestion(nmOK, brOK, exOK, sqOK, mfWarn))
}

// checkNativeMessaging 检查 Native Messaging unix socket 是否可达。
// socket 由 native-messaging-host 进程监听（Chrome 扩展自动启动），可达即 host 在运行。
func checkNativeMessaging() (bool, string) {
	sockPath := native.SocketPath()
	conn, err := net.DialTimeout("unix", sockPath, 2*time.Second)
	if err != nil {
		return false, fmt.Sprintf("不可用: socket %s 无法连接（%v）", sockPath, err)
	}
	conn.Close()
	return true, "可用: socket 已连接"
}

// checkBridgeHTTP 检查 Bridge HTTP 服务及扩展连接状态。
// 端口逻辑与 getCookiesViaBridge 一致：COOKIE_PORT 环境变量或默认 8008。
func checkBridgeHTTP() (bool, string) {
	port := os.Getenv("COOKIE_PORT")
	if port == "" {
		port = "8008"
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%s/health", port))
	if err != nil {
		return false, fmt.Sprintf("不可用: 服务未运行（%v）", err)
	}
	defer resp.Body.Close()

	var health struct {
		Service   string `json:"service"`
		Extension bool   `json:"extension"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return false, fmt.Sprintf("不可用: /health 响应解析失败（%v）", err)
	}
	if health.Extension {
		return true, "可用: 服务运行中，扩展已连接"
	}
	return false, "警告: 服务运行中，扩展未连接"
}

// checkExportFile 检查导出文件是否存在及其年龄。
func checkExportFile() (bool, string) {
	path, err := native.ExportFilePath()
	if err != nil {
		return false, fmt.Sprintf("不可用: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Sprintf("不可用: %s 不存在", path)
		}
		return false, fmt.Sprintf("不可用: %v", err)
	}
	return true, fmt.Sprintf("可用: 文件存在，年龄 %s", time.Since(fi.ModTime()).Round(time.Second))
}

// checkSQLite 通过只读查询验证 SQLite 直读模式。
// 复用 main.go 的 newStore 逻辑；Chrome 直读返回加密值，提示用户。
func checkSQLite(browser string) (bool, string) {
	var store cookie.Store
	store, err := newStore(browser)
	if err != nil {
		return false, fmt.Sprintf("不可用: %v", err)
	}

	// 只读验证：列出域名（不写任何文件）
	domains, err := store.ListDomains()
	if err != nil {
		return false, fmt.Sprintf("不可用: %v", err)
	}

	msg := fmt.Sprintf("可用: %s（%d 个域名）", browser, len(domains))
	if browser == "chrome" {
		msg = fmt.Sprintf("可用: %s（%d 个域名，注意: 值为加密标记 [ENCRYPTED]，建议扩展模式）",
			browser, len(domains))
	}
	return true, msg
}

// checkManifestExtensionID 检查 native messaging manifest 中扩展 ID 是否已替换。
// 检测顺序：FindExtensionID（已加载检测）→ GenerateIDForPath（预计算，扩展目录存在时）。
// 任一成功则检查 manifest 是否已含该 ID；都失败时提示扩展未加载。
// 返回是否有警告（EXTENSION_ID_HERE 未替换或 manifest 未安装）。
func checkManifestExtensionID() (bool, string) {
	detectedID, _ := native.FindExtensionID()
	source := "自动检测"
	if detectedID == "" {
		detectedID = precomputedExtensionID()
		source = "预计算"
	}
	manifestData, _ := firstManifest()

	if detectedID != "" {
		if manifestData == "" {
			return true, fmt.Sprintf("警告: 已检测到扩展 ID %s...，但 manifest 未安装，请运行 cookie-cli native-install", shortID(detectedID))
		}
		if strings.Contains(manifestData, "EXTENSION_ID_HERE") {
			return true, fmt.Sprintf("警告: 已检测到扩展 ID %s...，但 manifest 未更新，请运行 cookie-cli native-install", shortID(detectedID))
		}
		return false, fmt.Sprintf("通过: 扩展 ID 已配置（%s: %s...）", source, shortID(detectedID))
	}

	// 未检测到扩展 ID：manifest 已是真实 ID（不含占位符）则通过，否则提示加载扩展
	if manifestData != "" && !strings.Contains(manifestData, "EXTENSION_ID_HERE") {
		return false, "通过: manifest 已安装且扩展 ID 已替换"
	}
	return true, "警告: 扩展未加载或未检测到（请先在 chrome://extensions 加载 Cookie Bridge 扩展）"
}

// precomputedExtensionID 在扩展目录存在时按 Chromium GenerateIdForPath 算法
// 预计算扩展 ID（与 internal/native 的预计算逻辑一致）。
// WSL2 下使用 Windows 格式路径（C:\...，UTF-16LE 编码），Linux 下使用绝对路径（UTF-8）。
// 扩展目录不存在时返回空串。
func precomputedExtensionID() string {
	winDir := ""
	if isWSL2() {
		if winHome, err := wsl2WindowsHome(); err == nil {
			dir := filepath.Join(winHome, "cookie-bridge-extension")
			if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
				winDir = wslPathToWindows(dir)
			}
		}
	} else {
		if home, err := os.UserHomeDir(); err == nil {
			dir := filepath.Join(home, "cookie-bridge-extension")
			if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
				winDir = dir
			}
		}
	}
	if winDir == "" {
		return ""
	}
	return native.GenerateIDForPath(winDir)
}

// firstManifest 读取第一个存在的 manifest 内容，返回 (内容, 路径)。
// 找不到任何 manifest 时返回 ("", "")。
func firstManifest() (string, string) {
	for _, p := range manifestCandidates() {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		return string(data), p
	}
	return "", ""
}

// shortID 截取扩展 ID 前 8 位用于展示，避免输出完整 ID。
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// manifestCandidates 返回所有可能存在的 native messaging manifest 路径。
func manifestCandidates() []string {
	var paths []string
	home, _ := os.UserHomeDir()

	if isWSL2() {
		// WSL2: manifest 装在 Windows 家目录（先经 cmd.exe 取 %USERPROFILE%）
		if winHome, err := wsl2WindowsHome(); err == nil {
			paths = append(paths, filepath.Join(winHome, ".cookie", "native-messaging", "com.cookie.bridge.json"))
		}
		// 兜底：常见可能位置
		paths = append(paths,
			filepath.Join(home, ".cookie", "native-messaging", "com.cookie.bridge.json"),
		)
	}

	// Linux 原生: google-chrome 与 chromium
	paths = append(paths,
		filepath.Join(home, ".config", "google-chrome", "NativeMessagingHosts", "com.cookie.bridge.json"),
		filepath.Join(home, ".config", "chromium", "NativeMessagingHosts", "com.cookie.bridge.json"),
	)
	return paths
}

// buildSuggestion 根据各模式状态生成建议文本。
// 任一模式可用 → 提示直接使用（优先 Native Messaging）；全不可用 → 列出修复步骤。
func buildSuggestion(nmOK, brOK, exOK, sqOK, mfWarn bool) string {
	var avail []string
	if nmOK {
		avail = append(avail, "Native Messaging")
	}
	if brOK {
		avail = append(avail, "Bridge HTTP")
	}
	if exOK {
		avail = append(avail, "文件导出")
	}
	if sqOK {
		avail = append(avail, "SQLite 直读")
	}

	var sb strings.Builder
	if len(avail) > 0 {
		sb.WriteString("建议: 可直接使用 " + strings.Join(avail, " / ") + " 模式")
		if nmOK {
			sb.WriteString("（优先 Native Messaging）")
		}
		sb.WriteString("\n")
	} else {
		sb.WriteString("建议: 所有模式均不可用，按以下步骤修复:\n")
		sb.WriteString("  1. 运行 cookie-cli native-install 注册 Native Messaging Host，并确认 Chrome 扩展已加载\n")
		sb.WriteString("  2. 或运行 cookie-cli serve 启动 Bridge 服务，并确保扩展已连接\n")
		sb.WriteString("  3. 或关闭浏览器后重试 SQLite 直读（浏览器运行时会锁定数据库文件）\n")
	}
	if mfWarn {
		sb.WriteString("注意: manifest 扩展 ID 未就绪（未替换或未安装），Chrome 将无法启动 Native Messaging Host\n")
	}
	return sb.String()
}

// isWSL2 检测是否运行在 WSL2 环境中。
func isWSL2() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if _, err := os.Stat("/mnt/c"); err != nil {
		return false
	}
	if data, err := os.ReadFile("/proc/version"); err == nil {
		version := strings.ToLower(string(data))
		if strings.Contains(version, "microsoft") || strings.Contains(version, "wsl") {
			return true
		}
	}
	return false
}

// wsl2WindowsHome 获取 WSL2 中 Windows 用户家目录（WSL 路径格式）。
// 通过 cmd.exe 取 %USERPROFILE%，避免用户名与家目录名不一致的问题。
func wsl2WindowsHome() (string, error) {
	cmd := exec.Command("/mnt/c/Windows/System32/cmd.exe", "/c", "echo %USERPROFILE%")
	cmd.Dir = "/mnt/c" // 避免 cmd.exe 在 UNC 路径（如 /home/...）下打印警告到 stderr
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	winPath := strings.TrimSpace(strings.TrimRight(string(out), "\r\n"))
	if winPath == "" || winPath == "%USERPROFILE%" {
		return "", fmt.Errorf("无法获取 Windows 用户目录")
	}
	return windowsPathToWSL(winPath), nil
}

// windowsPathToWSL 将 Windows 路径转换为 WSL 路径。
// C:\Users\WPS -> /mnt/c/Users/WPS
func windowsPathToWSL(winPath string) string {
	winPath = strings.TrimSpace(winPath)
	if len(winPath) < 2 || winPath[1] != ':' {
		return winPath
	}
	drive := strings.ToLower(string(winPath[0]))
	return "/mnt/" + drive + strings.ReplaceAll(winPath[2:], `\`, "/")
}

// wslPathToWindows 将 WSL 路径转换为 Windows 路径（与 internal/native 实现一致）。
// /mnt/c/Users/foo -> C:\Users\foo；非 /mnt/ 路径原样返回。
func wslPathToWindows(wslPath string) string {
	if !strings.HasPrefix(wslPath, "/mnt/") {
		return wslPath
	}
	parts := strings.SplitN(wslPath, "/", 4)
	if len(parts) < 3 {
		return wslPath
	}
	drive := strings.ToUpper(parts[2])
	if len(parts) == 3 {
		return drive + `:\`
	}
	return drive + `:\` + strings.ReplaceAll(parts[3], "/", `\`)
}
