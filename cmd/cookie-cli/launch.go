package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// handleChrome 启动浏览器并自动加载 Cookie Bridge 扩展（--load-extension）。
// WSL2 下通过 cmd.exe 启动 Windows Chrome；Linux 下直接启动 Chrome/Chromium。
// 注意: 若 Chrome 已在运行，--load-extension 参数不会生效（Chrome 单实例会忽略新参数）。
// dryRun 为 true 时仅打印将执行的启动命令，不实际启动（供验证/预览）。
func handleChrome(dryRun bool) {
	// 启动前提示（Chrome 单实例时新参数不生效，先完全退出才能重新加载）
	fmt.Println("提示: 若 Chrome 已在运行请先完全退出，否则 --load-extension 参数不会生效")

	dir := findExtensionDir()
	if dir == "" {
		fmt.Println("未找到扩展目录，请先复制（make ext-copy）")
		os.Exit(1)
	}
	fmt.Printf("扩展目录: %s\n", dir)

	if isWSL2() {
		launchChromeWSL(dir, dryRun)
		return
	}
	launchChromeLinux(dir, dryRun)
}

// findExtensionDir 定位 Cookie Bridge 扩展目录（WSL 路径格式）。
// WSL2: %USERPROFILE%\cookie-bridge-extension（经 cmd.exe 取 Windows 家目录）；
// Linux: $HOME/cookie-bridge-extension，不存在则尝试 ~/.config/cookie-bridge-extension。
// 找不到返回空串。
func findExtensionDir() string {
	if isWSL2() {
		if winHome, err := wsl2WindowsHome(); err == nil {
			dir := filepath.Join(winHome, "cookie-bridge-extension")
			if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
				return dir
			}
		}
		return ""
	}

	if home, err := os.UserHomeDir(); err == nil {
		dir := filepath.Join(home, "cookie-bridge-extension")
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			return dir
		}
		// 兜底: ~/.config/cookie-bridge-extension
		dir = filepath.Join(home, ".config", "cookie-bridge-extension")
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			return dir
		}
	}
	return ""
}

// launchChromeWSL 在 WSL2 下通过 cmd.exe 启动 Windows Chrome。
// 命令构造: cmd.exe /c start "" <chrome 路径或名称> "--load-extension=C:\...\cookie-bridge-extension"
// 扩展目录转换为 Windows 路径后作为 --load-extension 参数（Chrome 需 Windows 格式路径）。
func launchChromeWSL(wslDir string, dryRun bool) {
	winDir := wslPathToWindows(wslDir)
	extArg := "--load-extension=" + winDir

	chromePath := findWindowsChrome()
	var args []string
	if chromePath != "" {
		// 找到 chrome.exe 完整路径（含空格，cmd.exe start 解析时按引号处理）
		args = []string{"/c", "start", "", chromePath, extArg}
	} else {
		// 未探测到完整路径，回退: cmd /c start chrome（依赖 Windows PATH）
		args = []string{"/c", "start", "", "chrome", extArg}
	}

	if dryRun {
		fmt.Printf("[dry-run] 将执行: %s %s\n", "/mnt/c/Windows/System32/cmd.exe", displayArgs(args))
		return
	}

	cmd := exec.Command("/mnt/c/Windows/System32/cmd.exe", args...)
	cmd.Dir = "/mnt/c" // 避免 cmd.exe 在 UNC 路径（如 /home/...）下打印警告到 stderr
	if err := cmd.Start(); err != nil {
		fmt.Printf("启动 Chrome 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("已启动 Chrome（--load-extension）")
}

// findWindowsChrome 探测 Windows Chrome 可执行文件路径（Windows 格式）。
// 依次检查标准安装位置与 ProgramFiles / LocalAppData 环境变量变体；
// 返回空串表示未找到。
func findWindowsChrome() string {
	var candidates []string
	candidates = append(candidates,
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
	)
	// 经 cmd.exe 读取 Windows 环境变量（WSL 环境变量与 Windows 不同源）
	for _, v := range []string{"ProgramFiles", "ProgramFiles(x86)", "LocalAppData"} {
		if val, err := windowsEnv(v); err == nil && val != "" && val != "%"+v+"%" {
			candidates = append(candidates,
				filepath.Join(val, "Google", "Chrome", "Application", "chrome.exe"))
		}
	}
	for _, c := range candidates {
		wsl := windowsPathToWSL(c)
		if fi, err := os.Stat(wsl); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

// launchChromeLinux 在原生 Linux 启动 Chrome/Chromium（--load-extension）。
// 探测顺序: google-chrome / google-chrome-stable / chromium / chromium-browser；
// 全部找不到时提示并退出。
func launchChromeLinux(dir string, dryRun bool) {
	browser := findLinuxBrowser()
	if browser == "" {
		fmt.Println("未找到 Chrome/Chromium 浏览器，请安装或手动启动 Chrome 并加载扩展")
		os.Exit(1)
	}
	extArg := "--load-extension=" + dir

	if dryRun {
		fmt.Printf("[dry-run] 将执行: %s %s\n", browser, extArg)
		return
	}

	cmd := exec.Command(browser, extArg)
	if err := cmd.Start(); err != nil {
		fmt.Printf("启动 %s 失败: %v\n", browser, err)
		os.Exit(1)
	}
	fmt.Printf("已启动 Chrome（--load-extension）: %s\n", browser)
}

// findLinuxBrowser 探测可用的 Chrome/Chromium 可执行文件。
func findLinuxBrowser() string {
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// windowsEnv 读取 Windows 环境变量（通过 cmd.exe echo，规避 WSL 与 Windows 环境变量不同源）。
// 失败或变量未定义时返回错误。
func windowsEnv(name string) (string, error) {
	cmd := exec.Command("/mnt/c/Windows/System32/cmd.exe", "/c", "echo %"+name+"%")
	cmd.Dir = "/mnt/c"
	out, err := cmd.Output() // 丢弃 stderr，避免 UNC 路径警告污染输出
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(strings.TrimRight(string(out), "\r\n")), nil
}

// displayArgs 将命令参数数组转为可读字符串（含空格参数加引号，供 dry-run 展示）。
// 空字符串参数（如 cmd start 的空标题）显示为 ""，避免与参数分隔混淆。
func displayArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		switch {
		case a == "":
			quoted[i] = `""`
		case strings.ContainsAny(a, " "):
			quoted[i] = `"` + a + `"`
		default:
			quoted[i] = a
		}
	}
	return strings.Join(quoted, " ")
}
