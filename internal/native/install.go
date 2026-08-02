package native

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	// nativeHostName 是 Native Messaging Host 的注册名，与扩展侧保持一致。
	nativeHostName = "com.cookie.bridge"
	// extensionIDPlaceholder 是 manifest 中扩展 ID 的占位符，
	// 安装后需由用户替换为真实扩展 ID。
	extensionIDPlaceholder = "EXTENSION_ID_HERE"
)

// nmManifest 描述 Chrome Native Messaging Host 的注册清单（manifest.json）内容。
type nmManifest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	Path           string   `json:"path"`
	Type           string   `json:"type"`
	AllowedOrigins []string `json:"allowed_origins"`
}

// isWSL2 检测是否运行在 WSL2 环境中。
// 判断依据：/mnt/c 存在且 /proc/version 包含 microsoft/wsl 标记。
func isWSL2() bool {
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

// windowsPathToWSL 将 Windows 路径转换为 WSL 路径。
// 例：C:\Users\WPS -> /mnt/c/Users/WPS
func windowsPathToWSL(winPath string) string {
	winPath = strings.TrimSpace(winPath)
	if len(winPath) < 2 || winPath[1] != ':' {
		return winPath
	}
	drive := strings.ToLower(string(winPath[0]))
	rest := strings.ReplaceAll(winPath[2:], `\`, "/")
	return "/mnt/" + drive + rest
}

// wslPathToWindows 将 WSL 路径转换为 Windows 路径。
// 例：/mnt/c/Users/foo -> C:\Users\foo
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

// execWSL 在 WSL 下执行 Windows 命令并返回 stdout（TrimSpace）。
// reg.exe / cmd.exe 在 WSL2 下可直接执行（/mnt/c/Windows/System32/...），
// wslpath 在 WSL2 也可执行。
func execWSL(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = "/mnt/c"       // 避免 cmd.exe 在 UNC 路径（如 /home/...）下打印警告到 stderr
	out, err := cmd.Output() // 丢弃 stderr，避免污染 %USERPROFILE% / wslpath 解析
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// InstallHost 注册 Native Messaging Host（自动检测 WSL2）。
// 扩展 ID 通过三级策略解析：已加载检测 → 预计算 → 占位符；
// 解析成功后提示使用 cookie-cli chrome 启动器加载扩展。
// 返回错误时上层负责 log.Fatalf。
func InstallHost() error {
	binary, err := os.Executable()
	if err != nil || binary == "" {
		return fmt.Errorf("无法确定当前可执行文件路径: %v", err)
	}

	// 三级策略解析扩展 ID（已加载检测 / 预计算 / 占位符）
	extID, source := resolveExtensionID()
	if extID == extensionIDPlaceholder {
		fmt.Printf("警告: %s\n", source)
		fmt.Println("将使用占位符，安装后请手动替换。")
	} else {
		fmt.Printf("%s扩展 ID: %s\n", source, extID)
		// 提示扩展加载方式（不再写入 External Extensions 注册表/JSON，该机制不支持未打包目录）
		notifyExtensionLoading(extID)
	}

	if isWSL2() {
		return installHostWSL(binary, extID)
	}
	return installHostLinux(binary, extID)
}

// extDirCandidate 描述一个 Cookie Bridge 扩展目录候选。
// wslPath 为 WSL 格式路径（本地存在性检查用）；winPath 为 Windows 格式路径
// （供 GenerateIDForPath 预计算与注册表注册用）。
type extDirCandidate struct {
	wslPath string
	winPath string
}

// extensionDirCandidates 返回当前实际存在的 Cookie Bridge 扩展目录候选，
// 供扩展 ID 预计算与自动安装使用。
// WSL2 下优先用 %USERPROFILE% 定位（复用 execWSL），再以 /mnt/c/Users/<user> 兜底；
// Linux 下取 $HOME/cookie-bridge-extension。
func extensionDirCandidates() []extDirCandidate {
	var dirs []extDirCandidate

	if isWSL2() {
		// 候选 1：%USERPROFILE% 定位的 Windows 家目录（与 native-messaging 安装逻辑一致）
		if winProfile, err := execWSL("/mnt/c/Windows/System32/cmd.exe", "/c", "echo %USERPROFILE%"); err == nil {
			winDir := strings.TrimRight(winProfile, "\r\n") + `\cookie-bridge-extension`
			if isDir(windowsPathToWSL(winDir)) {
				dirs = append(dirs, extDirCandidate{
					wslPath: windowsPathToWSL(winDir),
					winPath: winDir,
				})
			}
		}
		// 候选 2：/mnt/c/Users/<user> 兜底（Windows 用户名与目录名可能不一致）
		if user := os.Getenv("USER"); user != "" {
			wslDir := "/mnt/c/Users/" + user + "/cookie-bridge-extension"
			if isDir(wslDir) {
				dirs = append(dirs, extDirCandidate{
					wslPath: wslDir,
					winPath: wslPathToWindows(wslDir),
				})
			}
		}
	}

	// Linux 原生：$HOME/cookie-bridge-extension
	if home, err := os.UserHomeDir(); err == nil {
		absDir := filepath.Join(home, "cookie-bridge-extension")
		if isDir(absDir) {
			dirs = append(dirs, extDirCandidate{wslPath: absDir, winPath: absDir})
		}
	}
	return dirs
}

// isDir 判断 path 是否为存在的目录。
func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// resolveExtensionID 按三级策略解析 Cookie Bridge 扩展 ID，返回 (id, 来源说明)。
//  1. FindExtensionID 检测已加载扩展（来源: 已加载检测）
//  2. 对存在的扩展目录 GenerateIDForPath 预计算（来源: 预计算；
//     WSL2 场景传入 Windows 格式路径 C:\...，保证 UTF-16LE 编码正确）
//  3. 全部失败返回占位符与错误说明
func resolveExtensionID() (string, string) {
	if id, err := FindExtensionID(); err == nil && id != "" {
		return id, "已加载检测"
	}
	for _, dir := range extensionDirCandidates() {
		if id := GenerateIDForPath(dir.winPath); id != "" {
			return id, "预计算"
		}
	}
	return extensionIDPlaceholder, "未自动检测到扩展 ID，且未找到扩展目录（请先复制扩展: make ext-copy）"
}

// notifyExtensionLoading 提示用户通过 cookie-cli chrome 启动器加载扩展。
// 原 External Extensions 机制（WSL2 注册表 HKCU\Software\Google\Chrome\Extensions\<id>
// 的 path 值、Linux External Extensions JSON）不支持未打包目录：Chromium 源码
// external_registry_loader_win.cc 中 path 值须指向 .crx 文件，CanOpenFileForReading()
// 对目录返回 false，故注册不生效。改为 --load-extension 启动器方式。
// 扩展目录不存在时提示先复制。
func notifyExtensionLoading(extID string) {
	dirs := extensionDirCandidates()
	if len(dirs) == 0 {
		fmt.Println("提示: 未找到扩展目录，请先复制扩展（make ext-copy）")
		return
	}
	fmt.Println("扩展已就绪：运行 `cookie-cli chrome` 启动 Chrome 时自动加载扩展")
	fmt.Println("（External Extensions 注册表/JSON 不支持未打包目录，故采用 --load-extension 启动器方式）")
}

// UninstallHost 移除 Native Messaging Host 注册。
func UninstallHost() error {
	if isWSL2() {
		// 删除 Windows 注册表项；键可能不存在，忽略错误
		regKey := `HKCU\Software\Google\Chrome\NativeMessagingHosts\` + nativeHostName
		execWSL("reg.exe", "DELETE", regKey, "/f")

		// 删除 WSL 端生成的 manifest 与 .bat 启动器
		if winProfile, err := execWSL("/mnt/c/Windows/System32/cmd.exe", "/c", "echo %USERPROFILE%"); err == nil {
			winHome := windowsPathToWSL(strings.TrimRight(winProfile, "\r\n"))
			nmDir := filepath.Join(winHome, ".cookie", "native-messaging")
			os.Remove(filepath.Join(nmDir, nativeHostName+".json"))
			os.Remove(filepath.Join(nmDir, nativeHostName+".bat"))
		}
		fmt.Println("Windows Native Messaging Host 已移除")
		return nil
	}

	// 删除原生 Linux 下 Chrome / Chromium 的 manifest
	for _, dir := range nativeHostDirs() {
		os.Remove(filepath.Join(dir, nativeHostName+".json"))
	}
	fmt.Println("Linux Native Messaging Host 已移除")
	return nil
}

// installHostWSL 在 WSL2 环境将 Native Messaging Host 注册到 Windows 注册表。
// extID 为自动检测到的扩展 ID（可能为占位符）。
func installHostWSL(binary, extID string) error {
	// 通过 %USERPROFILE% 获取 Windows 用户家目录（避免用户名与目录名不一致）
	winProfile, err := execWSL("/mnt/c/Windows/System32/cmd.exe", "/c", "echo %USERPROFILE%")
	if err != nil {
		return fmt.Errorf("获取 Windows 用户目录失败: %w", err)
	}
	winHome := windowsPathToWSL(strings.TrimRight(winProfile, "\r\n"))
	nmDir := filepath.Join(winHome, ".cookie", "native-messaging")
	if err := os.MkdirAll(nmDir, 0755); err != nil {
		return fmt.Errorf("创建 native-messaging 目录失败: %w", err)
	}

	// 创建 .bat 启动器（必须 CRLF 换行，供 Windows cmd.exe 解析）
	batFile := filepath.Join(nmDir, nativeHostName+".bat")
	batContent := []byte("@echo off\r\nwsl.exe -- \"" + binary + "\" native-messaging-host\r\n")
	if err := os.WriteFile(batFile, batContent, 0644); err != nil {
		return fmt.Errorf("写入启动器失败: %w", err)
	}

	// 转换 .bat 路径为 Windows 格式。json.MarshalIndent 会把反斜杠自动转义
	// 为 JSON 中的 \\，等价于 shell 脚本中 sed 's|\\|\\\\|g' 的效果
	winBat, err := execWSL("wslpath", "-w", batFile)
	if err != nil {
		return fmt.Errorf("转换启动器路径失败: %w", err)
	}

	// 写入 manifest，path 指向 Windows 侧的 .bat 启动器
	jsonFile, err := writeManifest(nmDir, winBat, extID)
	if err != nil {
		return err
	}

	// 转换 manifest 路径为 Windows 格式并注册到 HKCU
	winJSON, err := execWSL("wslpath", "-w", jsonFile)
	if err != nil {
		return fmt.Errorf("转换 manifest 路径失败: %w", err)
	}
	regKey := `HKCU\Software\Google\Chrome\NativeMessagingHosts\` + nativeHostName
	if _, err := execWSL("reg.exe", "ADD", regKey, "/ve", "/t", "REG_SZ", "/d", winJSON, "/f"); err != nil {
		return fmt.Errorf("注册表写入失败: %w", err)
	}

	fmt.Println("检测到 WSL2 环境，Native Messaging Host 已注册")
	fmt.Printf("  Windows 家目录: %s\n", winHome)
	fmt.Printf("  Manifest: %s\n", jsonFile)
	fmt.Printf("  Launcher: %s\n", batFile)
	fmt.Println()
	if extID == extensionIDPlaceholder {
		fmt.Printf("重要: 请编辑 %s\n", jsonFile)
		fmt.Printf("将 %s 替换为你的扩展 ID（在 chrome://extensions 查看）\n", extensionIDPlaceholder)
	} else {
		fmt.Printf("扩展 ID 已自动写入 manifest: %s\n", extID)
	}
	return nil
}

// installHostLinux 在原生 Linux 环境注册 Native Messaging Host，
// 同时写入 Chrome 与 Chromium 两个配置目录。
// extID 为自动检测到的扩展 ID（可能为占位符）。
func installHostLinux(binary, extID string) error {
	for _, dir := range nativeHostDirs() {
		if _, err := writeManifest(dir, binary, extID); err != nil {
			return err
		}
	}
	fmt.Println("Native Messaging Host 已注册")
	for _, dir := range nativeHostDirs() {
		fmt.Printf("  Manifest: %s\n", filepath.Join(dir, nativeHostName+".json"))
	}
	fmt.Println()
	if extID == extensionIDPlaceholder {
		fmt.Printf("重要: 请编辑上述文件，将 %s 替换为你的扩展 ID\n", extensionIDPlaceholder)
	} else {
		fmt.Printf("扩展 ID 已自动写入 manifest: %s\n", extID)
	}
	return nil
}

// writeManifest 将 Native Messaging Host 清单写入 dir 目录并返回 JSON 文件路径。
// hostPath 作为清单的 path 字段：WSL2 下为 Windows 侧 .bat 路径，Linux 下为可执行文件路径；
// extID 作为 allowed_origins 中的扩展 ID（自动检测到时为真实 ID，否则为占位符）。
func writeManifest(dir, hostPath, extID string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建目录失败 %s: %w", dir, err)
	}
	m := nmManifest{
		Name:        nativeHostName,
		Description: "Cookie Bridge Native Messaging Host",
		Path:        hostPath,
		Type:        "stdio",
		AllowedOrigins: []string{
			"chrome-extension://" + extID + "/",
		},
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", fmt.Errorf("生成 manifest 失败: %w", err)
	}
	data = append(data, '\n')

	jsonFile := filepath.Join(dir, nativeHostName+".json")
	if err := os.WriteFile(jsonFile, data, 0644); err != nil {
		return "", fmt.Errorf("写入 manifest 失败: %w", err)
	}
	return jsonFile, nil
}

// nativeHostDirs 返回原生 Linux 下 Chrome 与 Chromium 的 Native Messaging Hosts 配置目录。
func nativeHostDirs() []string {
	home := os.Getenv("HOME")
	return []string{
		filepath.Join(home, ".config", "google-chrome", "NativeMessagingHosts"),
		filepath.Join(home, ".config", "chromium", "NativeMessagingHosts"),
	}
}
