package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// extensionFiles 是 Cookie Bridge 扩展需要复制的文件清单。
var extensionFiles = []string{
	"manifest.json",
	"background.js",
	"offscreen.html",
	"offscreen.js",
}

// handleCopy 将 Cookie Bridge 扩展复制到目标目录。
// 源：系统扩展目录 /usr/share/cookie-cli/extension 或仓库 extension/；
// 目标：WSL2 下 C:\Users\<user>\cookie-bridge-extension，Linux 下 ~/cookie-bridge-extension。
func handleCopy() {
	srcDir := locateExtensionSource()
	if srcDir == "" {
		log.Fatal("未找到扩展源（系统目录或仓库 extension/）")
	}

	dstDir, err := resolveExtensionTarget()
	if err != nil {
		log.Fatalf("定位扩展目标目录失败: %v", err)
	}

	if err := copyExtensionFiles(srcDir, dstDir); err != nil {
		log.Fatalf("复制扩展失败: %v", err)
	}

	fmt.Println("Cookie Bridge 扩展已复制:")
	fmt.Printf("  源:  %s\n", srcDir)
	fmt.Printf("  目标: %s\n", dstDir)
	fmt.Printf("  文件: %s\n", strings.Join(extensionFiles, " "))
	fmt.Println("请到 chrome://extensions 开启开发者模式并「加载已解压的扩展程序」")
}

// locateExtensionSource 按优先级返回第一个存在且含 manifest.json 的扩展源目录；
// 全部不存在时返回空字符串。
func locateExtensionSource() string {
	// 候选 1：系统扩展目录（AUR 安装）
	if isExtensionDir("/usr/share/cookie-cli/extension") {
		return "/usr/share/cookie-cli/extension"
	}
	// 候选 2：仓库 extension/（相对当前工作目录）
	if cwd, err := os.Getwd(); err == nil {
		repoExt := filepath.Join(cwd, "extension")
		if isExtensionDir(repoExt) {
			return repoExt
		}
	}
	return ""
}

// isExtensionDir 判断 dir 是否为存在且含 manifest.json 的扩展目录。
func isExtensionDir(dir string) bool {
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return false
	}
	_, err = os.Stat(filepath.Join(dir, "manifest.json"))
	return err == nil
}

// resolveExtensionTarget 返回扩展的目标目录（WSL 路径格式）。
// WSL2 下为 /mnt/c/Users/<user>/cookie-bridge-extension（经 cmd.exe 取 %USERPROFILE%）；
// Linux 下为 ~/cookie-bridge-extension。
func resolveExtensionTarget() (string, error) {
	if isWSL2() {
		winHome, err := wsl2WindowsHome()
		if err != nil {
			return "", fmt.Errorf("获取 Windows 用户目录失败: %w", err)
		}
		return filepath.Join(winHome, "cookie-bridge-extension"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户家目录失败: %w", err)
	}
	return filepath.Join(home, "cookie-bridge-extension"), nil
}

// copyExtensionFiles 将 src 目录中的扩展文件逐个复制到 dst（幂等，已存在则覆盖），
// 复制完成后校验目标 manifest.json 存在。
func copyExtensionFiles(src, dst string) error {
	if err := os.MkdirAll(dst, 0755); err != nil {
		return fmt.Errorf("创建目标目录失败: %w", err)
	}
	for _, name := range extensionFiles {
		if err := copyFile(filepath.Join(src, name), filepath.Join(dst, name)); err != nil {
			return fmt.Errorf("复制 %s 失败: %w", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "manifest.json")); err != nil {
		return fmt.Errorf("复制后校验失败: 目标 manifest.json 不存在")
	}
	return nil
}

// copyFile 将 srcPath 复制到 dstPath（已存在则覆盖）。
func copyFile(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return dst.Close()
}
