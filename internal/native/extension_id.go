package native

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// extSetting 是 Chrome Preferences 中 extensions.settings 条目的结构。
// map 的 key 为扩展 ID（32 字符），path 为扩展目录路径，name 为扩展名称。
type extSetting struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

// chromeProfiles 是 Chrome/Edge 用户数据目录下的 profile 子目录候选。
// 多 profile 场景下扩展可能加载在任一 profile 中。
var chromeProfiles = []string{"Default", "Profile 1", "Profile 2", "Profile 3"}

// FindExtensionID 从 Chrome Preferences 解析已加载的 Cookie Bridge 扩展 ID。
// 返回 (id, nil)；扩展未加载/未找到时返回 ("", err)（err 描述原因）。
func FindExtensionID() (string, error) {
	candidates := preferencesCandidates()

	foundAny := false
	var parseErr error
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		foundAny = true
		id, err := parseExtensionID(data)
		if err != nil {
			// 记录首个解析错误并继续尝试其他候选（如多 profile 中某个损坏）
			if parseErr == nil {
				parseErr = fmt.Errorf("解析 %s 失败: %w", path, err)
			}
			continue
		}
		if id != "" {
			return id, nil
		}
	}

	if !foundAny {
		return "", fmt.Errorf("Chrome Preferences 未找到（Chrome 可能未运行）")
	}
	if parseErr != nil {
		return "", parseErr
	}
	return "", fmt.Errorf("Cookie Bridge 扩展未加载（请先在 chrome://extensions 加载扩展后重试）")
}

// preferencesCandidates 按优先级收集 Chrome/Edge Preferences 候选路径。
// WSL2 下通过 %USERPROFILE% 定位 Windows 家目录（复用 execWSL，避免 stderr 污染）。
func preferencesCandidates() []string {
	var paths []string

	if isWSL2() {
		if winProfile, err := execWSL("/mnt/c/Windows/System32/cmd.exe", "/c", "echo %USERPROFILE%"); err == nil {
			winHome := windowsPathToWSL(winProfile)
			chromeBase := filepath.Join(winHome, "AppData", "Local", "Google", "Chrome", "User Data")
			for _, p := range chromeProfiles {
				paths = append(paths, filepath.Join(chromeBase, p, "Preferences"))
			}
			// Edge 作为可选候选（读不到即跳过）
			edgeBase := filepath.Join(winHome, "AppData", "Local", "Microsoft", "Edge", "User Data")
			for _, p := range chromeProfiles {
				paths = append(paths, filepath.Join(edgeBase, p, "Preferences"))
			}
		}
	}

	// Linux 原生 Chrome 与 Chromium
	if home, err := os.UserHomeDir(); err == nil {
		for _, base := range []string{
			filepath.Join(home, ".config", "google-chrome"),
			filepath.Join(home, ".config", "chromium"),
		} {
			for _, p := range chromeProfiles {
				paths = append(paths, filepath.Join(base, p, "Preferences"))
			}
		}
	}
	return paths
}

// parseExtensionID 从 Preferences JSON 的 extensions.settings 中解析匹配的扩展 ID。
// 匹配条件：扩展路径含 cookie-bridge-extension，或扩展名称为 Cookie Bridge。
func parseExtensionID(data []byte) (string, error) {
	var prefs struct {
		Extensions struct {
			Settings map[string]extSetting `json:"settings"`
		} `json:"extensions"`
	}
	if err := json.Unmarshal(data, &prefs); err != nil {
		return "", err
	}
	for id, s := range prefs.Extensions.Settings {
		if strings.Contains(strings.ToLower(s.Path), "cookie-bridge-extension") ||
			s.Name == "Cookie Bridge" {
			return id, nil
		}
	}
	return "", nil
}
