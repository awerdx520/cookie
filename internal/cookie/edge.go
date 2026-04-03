package cookie

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// EdgeStore 实现从 Edge 读取 Cookie
// Edge 基于 Chromium，数据库结构和加密方式与 Chrome 完全相同
type EdgeStore struct {
	ChromeStore
}

// NewEdgeStore 创建新的 EdgeStore 实例
func NewEdgeStore() (*EdgeStore, error) {
	if runtime.GOOS == "darwin" {
		return nil, fmt.Errorf("Edge 不支持 macOS 平台")
	}
	dbPath, err := findEdgeCookiePath()
	if err != nil {
		return nil, err
	}
	return &EdgeStore{ChromeStore{dbPath: dbPath}}, nil
}

// findEdgeCookiePath 查找 Edge Cookie 文件路径
func findEdgeCookiePath() (string, error) {
	if runtime.GOOS == "linux" && isWSL2() {
		home, err := getWSL2WindowsHome()
		if err == nil {
			basePath := filepath.Join(home, "AppData", "Local", "Microsoft", "Edge", "User Data")
			if path, err := findCookieInProfiles(basePath, "Edge"); err == nil {
				return path, nil
			}
		} else {
			log.Printf("警告: 无法获取 Windows 家目录: %v", err)
		}
	}

	var basePath string
	switch runtime.GOOS {
	case "windows":
		basePath = filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Edge", "User Data")
	case "linux":
		basePath = filepath.Join(os.Getenv("HOME"), ".config", "microsoft-edge")
	default:
		return "", fmt.Errorf("Edge 不支持的操作系统: %s", runtime.GOOS)
	}

	return findCookieInProfiles(basePath, "Edge")
}

// findCookieInProfiles 在 Chromium 系浏览器的 User Data 目录下搜索 Cookie 文件
func findCookieInProfiles(basePath, browserName string) (string, error) {
	candidates := []string{
		filepath.Join(basePath, "Default", "Network", "Cookies"),
		filepath.Join(basePath, "Profile 1", "Network", "Cookies"),
		filepath.Join(basePath, "Profile 2", "Network", "Cookies"),
		filepath.Join(basePath, "Default", "Cookies"),
		filepath.Join(basePath, "Profile 1", "Cookies"),
		filepath.Join(basePath, "Profile 2", "Cookies"),
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			log.Printf("找到 %s Cookie 文件: %s", browserName, path)
			return path, nil
		}
	}

	return "", fmt.Errorf("未找到 %s Cookie 文件，请确保 %s 已安装并至少运行过一次: %s", browserName, browserName, basePath)
}

// isSystemUser 判断是否为 Windows 系统用户目录
func isSystemUser(name string) bool {
	return name == "Default" || name == "Public" || name == "All Users" ||
		name == "Default User" || strings.HasSuffix(name, ".bak") ||
		name == "desktop.ini"
}
