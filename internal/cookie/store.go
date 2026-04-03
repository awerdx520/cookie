package cookie

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// getWSL2WindowsUsername 获取 WSL2 中 Windows 用户名
func getWSL2WindowsUsername() (string, error) {
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("非 Linux 环境")
	}

	windowsMount := "/mnt/c"
	if _, err := os.Stat(windowsMount); err != nil {
		return "", fmt.Errorf("未找到 Windows 挂载点 /mnt/c: %w", err)
	}

	usersDir := filepath.Join(windowsMount, "Users")
	files, err := os.ReadDir(usersDir)
	if err != nil {
		return "", fmt.Errorf("读取 Windows 用户目录失败: %w", err)
	}

	for _, file := range files {
		if file.IsDir() {
			name := file.Name()
			if name != "Default" && name != "Public" && name != "All Users" &&
				name != "Default User" && !strings.HasSuffix(name, ".bak") &&
				name != "desktop.ini" {
				userPath := filepath.Join(usersDir, name)
				if _, err := os.Stat(filepath.Join(userPath, "Desktop")); err == nil {
					return name, nil
				}
				if _, err := os.Stat(filepath.Join(userPath, "Documents")); err == nil {
					return name, nil
				}
			}
		}
	}

	return "", fmt.Errorf("未找到有效的 Windows 用户目录")
}

// isWSL2 检测是否运行在 WSL2 环境中
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

	if output, err := exec.Command("uname", "-a").Output(); err == nil {
		uname := strings.ToLower(string(output))
		if strings.Contains(uname, "microsoft") || strings.Contains(uname, "wsl") {
			return true
		}
	}

	return false
}

// Cookie 表示一个浏览器 Cookie
type Cookie struct {
	Name     string
	Value    string
	Domain   string
	Path     string
	Expires  int64
	Secure   bool
	HTTPOnly bool
}

// Store 定义 Cookie 存储接口
type Store interface {
	GetCookies(domain string) ([]Cookie, error)
	ListDomains() ([]string, error)
}

// ChromeStore 实现从 Chrome 读取 Cookie
//
// 注意: Chrome 对 Cookie 值加密存储，直接读取数据库只能获得加密值。
// 推荐使用 Cookie Bridge 扩展模式 (cookie-cli serve) 获取明文 Cookie。
// 数据库直读仅作为回退方案，加密的值会标记为 [ENCRYPTED]。
type ChromeStore struct {
	dbPath string
}

// NewChromeStore 创建新的 ChromeStore 实例
func NewChromeStore() (*ChromeStore, error) {
	dbPath, err := findChromeCookiePath()
	if err != nil {
		return nil, err
	}
	return &ChromeStore{dbPath: dbPath}, nil
}

// findChromeCookiePath 查找 Chrome Cookie 文件路径
func findChromeCookiePath() (string, error) {
	var basePath string

	if runtime.GOOS == "linux" {
		if _, err := os.Stat("/mnt/c"); err == nil {
			user, err := getWSL2WindowsUsername()
			if err != nil {
				log.Printf("警告: 无法获取 Windows 用户名，使用默认路径: %v", err)
				user = "thomas"
			}

			winPaths := []string{
				"/mnt/c/Users/%s/AppData/Local/Google/Chrome/User Data/Default/Network/Cookies",
				"/mnt/c/Users/%s/AppData/Local/Google/Chrome/User Data/Profile 1/Network/Cookies",
				"/mnt/c/Users/%s/AppData/Local/Google/Chrome/User Data/Profile 2/Network/Cookies",
				"/mnt/c/Users/%s/AppData/Local/Google/Chrome/User Data/Default/Cookies",
				"/mnt/c/Users/%s/AppData/Local/Google/Chrome/User Data/Profile 1/Cookies",
				"/mnt/c/Users/%s/AppData/Local/Google/Chrome/User Data/Profile 2/Cookies",
			}

			for _, pattern := range winPaths {
				path := fmt.Sprintf(pattern, user)
				if _, err := os.Stat(path); err == nil {
					log.Printf("找到 Chrome Cookie 文件: %s", path)
					return path, nil
				}
			}

			usersDir := "/mnt/c/Users"
			if files, err := os.ReadDir(usersDir); err == nil {
				for _, file := range files {
					if file.IsDir() {
						userName := file.Name()
						if userName != "Default" && userName != "Public" && userName != "All Users" &&
							userName != "Default User" && !strings.HasSuffix(userName, ".bak") &&
							userName != "desktop.ini" {
							for _, pattern := range winPaths {
								path := fmt.Sprintf(pattern, userName)
								if _, err := os.Stat(path); err == nil {
									log.Printf("找到 Chrome Cookie 文件（用户 %s）: %s", userName, path)
									return path, nil
								}
							}
						}
					}
				}
			}
		}
	}

	switch runtime.GOOS {
	case "windows":
		basePath = filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "User Data")
	case "linux":
		basePath = filepath.Join(os.Getenv("HOME"), ".config", "google-chrome")
	default:
		return "", fmt.Errorf("不支持的操作系统: %s（仅支持 Windows 和 Linux）", runtime.GOOS)
	}

	profiles := []string{
		filepath.Join(basePath, "Default", "Cookies"),
		filepath.Join(basePath, "Profile 1", "Cookies"),
		filepath.Join(basePath, "Profile 2", "Cookies"),
	}

	for _, profile := range profiles {
		if _, err := os.Stat(profile); err == nil {
			return profile, nil
		}
	}

	return "", fmt.Errorf("未找到 Chrome Cookie 文件，请确保 Chrome 已安装并至少运行过一次：%s", basePath)
}

// GetCookies 实现 Store 接口
//
// Chrome 的 Cookie 值存储在 encrypted_value 列中（加密），
// 直接读取数据库无法获得明文。推荐使用 Cookie Bridge 扩展模式。
func (s *ChromeStore) GetCookies(domain string) ([]Cookie, error) {
	db, cleanup, err := s.openDB()
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	defer cleanup()

	query := `
		SELECT host_key, name, value, path, expires_utc, is_secure, is_httponly
		FROM cookies
		WHERE host_key LIKE ?
		ORDER BY name
	`

	domainPattern := "%" + domain
	rows, err := db.Query(query, domainPattern)
	if err != nil {
		return nil, fmt.Errorf("查询数据库失败: %w", err)
	}
	defer rows.Close()

	var cookies []Cookie
	for rows.Next() {
		var hostKey, name, value, path string
		var expiresUTC, isSecure, isHTTPOnly int64

		if err := rows.Scan(&hostKey, &name, &value, &path, &expiresUTC, &isSecure, &isHTTPOnly); err != nil {
			return nil, fmt.Errorf("读取行失败: %w", err)
		}

		if value == "" {
			value = "[ENCRYPTED]"
		}

		cookies = append(cookies, Cookie{
			Name:     name,
			Value:    value,
			Domain:   hostKey,
			Path:     path,
			Expires:  expiresUTC,
			Secure:   isSecure == 1,
			HTTPOnly: isHTTPOnly == 1,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历结果失败: %w", err)
	}

	return cookies, nil
}

// ListDomains 列出所有域名
func (s *ChromeStore) ListDomains() ([]string, error) {
	db, cleanup, err := s.openDB()
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	defer cleanup()

	query := `
		SELECT DISTINCT host_key
		FROM cookies
		ORDER BY host_key
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("查询数据库失败: %w", err)
	}
	defer rows.Close()

	var domains []string
	for rows.Next() {
		var domain string
		if err := rows.Scan(&domain); err != nil {
			return nil, fmt.Errorf("读取行失败: %w", err)
		}
		domain = strings.TrimPrefix(domain, ".")
		domains = append(domains, domain)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历结果失败: %w", err)
	}

	return domains, nil
}

// copyToTemp 将源文件复制到临时文件并返回临时文件路径
func copyToTemp(src string) (string, error) {
	wsl2WinPath := isWSL2() && strings.HasPrefix(src, "/mnt/")

	if wsl2WinPath {
		tmpPath, err := copyToTempViaWindows(src)
		if err == nil {
			return tmpPath, nil
		}
		log.Printf("Windows 端复制失败: %v，回退到直接复制", err)
	}

	tmpPath, err := copyToTempDirect(src)
	if err != nil {
		if wsl2WinPath {
			return "", fmt.Errorf("浏览器可能正在独占锁定 Cookie 文件，请关闭浏览器后重试: %w", err)
		}
		return "", fmt.Errorf("打开源文件失败: %w", err)
	}
	return tmpPath, nil
}

func copyToTempDirect(src string) (string, error) {
	srcFile, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer srcFile.Close()

	tmpFile, err := os.CreateTemp("", "cookie-*.sqlite")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, srcFile); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("复制文件失败: %w", err)
	}

	return tmpFile.Name(), nil
}

// copyToTempViaWindows 通过 Windows 端命令复制被浏览器锁定的文件。
func copyToTempViaWindows(src string) (string, error) {
	winSrc := wslPathToWindows(src)
	pid := os.Getpid()
	winTmp := fmt.Sprintf(`C:\Windows\Temp\cookie_copy_%d.sqlite`, pid)
	wslTmp := fmt.Sprintf("/mnt/c/Windows/Temp/cookie_copy_%d.sqlite", pid)

	defer os.Remove(wslTmp)

	err := copyViaCreateFileW(winSrc, winTmp)
	if err != nil {
		log.Printf("CreateFileW 复制失败: %v，尝试 cmd.exe copy", err)
		err = copyViaCmdCopy(winSrc, winTmp)
	}
	if err != nil {
		return "", err
	}

	return moveFromWindows(wslTmp)
}

func copyViaCreateFileW(winSrc, winDst string) error {
	psScript := fmt.Sprintf(`
Add-Type -TypeDefinition @"
using System;
using System.IO;
using System.Runtime.InteropServices;
public class FC {
    [DllImport("kernel32.dll", SetLastError=true, CharSet=CharSet.Unicode)]
    static extern IntPtr CreateFileW(string f, uint a, uint s, IntPtr p, uint d, uint g, IntPtr t);
    [DllImport("kernel32.dll")] static extern bool ReadFile(IntPtr h, byte[] b, uint n, out uint r, IntPtr o);
    [DllImport("kernel32.dll")] static extern bool CloseHandle(IntPtr h);
    public static void Copy(string src, string dst) {
        IntPtr h = CreateFileW(src, 0x80000000, 7, IntPtr.Zero, 3, 0, IntPtr.Zero);
        if (h == new IntPtr(-1)) throw new Exception("CreateFileW error " + Marshal.GetLastWin32Error());
        try {
            using (var fs = new FileStream(dst, FileMode.Create)) {
                byte[] buf = new byte[65536]; uint r;
                while (ReadFile(h, buf, (uint)buf.Length, out r, IntPtr.Zero) && r > 0) fs.Write(buf, 0, (int)r);
            }
        } finally { CloseHandle(h); }
    }
}
"@
[FC]::Copy('%s','%s')
`, winSrc, winDst)

	cmd := exec.Command("/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe",
		"-NoProfile", "-Command", psScript)
	cmd.Dir = "/mnt/c/"
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("PowerShell CreateFileW 复制失败: %v\n%s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func copyViaCmdCopy(winSrc, winDst string) error {
	cmd := exec.Command("/mnt/c/Windows/System32/cmd.exe", "/c",
		"copy", "/y", winSrc, winDst)
	cmd.Dir = "/mnt/c/"
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cmd.exe copy 失败: %v\n%s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func moveFromWindows(wslTmp string) (string, error) {
	tmpFile, err := os.CreateTemp("", "cookie-*.sqlite")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpFile.Close()

	winCopied, err := os.Open(wslTmp)
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("打开 Windows 临时文件失败: %w", err)
	}
	defer winCopied.Close()

	dst, err := os.Create(tmpFile.Name())
	if err != nil {
		return "", fmt.Errorf("创建目标文件失败: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, winCopied); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("复制到 Linux 临时文件失败: %w", err)
	}

	return tmpFile.Name(), nil
}

// wslPathToWindows 将 WSL 路径转换为 Windows 路径
// /mnt/c/Users/foo -> C:\Users\foo
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

// openDB 打开数据库，必要时复制到临时文件
func (s *ChromeStore) openDB() (*sql.DB, func(), error) {
	db, err := sql.Open("sqlite3", s.dbPath+"?mode=ro&immutable=1&_timeout=5000")
	if err == nil {
		if err := db.Ping(); err == nil {
			return db, func() { db.Close() }, nil
		}
		db.Close()
	}

	tmpPath, err := copyToTemp(s.dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("复制到临时文件失败: %w", err)
	}

	db, err = sql.Open("sqlite3", tmpPath+"?mode=ro&_timeout=5000")
	if err != nil {
		os.Remove(tmpPath)
		return nil, nil, fmt.Errorf("打开临时数据库失败: %w", err)
	}

	cleanup := func() {
		db.Close()
		os.Remove(tmpPath)
	}

	return db, cleanup, nil
}
