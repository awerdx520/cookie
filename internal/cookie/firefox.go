package cookie

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// FirefoxStore 实现从 Firefox 读取 Cookie
type FirefoxStore struct {
	dbPath string
}

// NewFirefoxStore 创建新的 FirefoxStore 实例
func NewFirefoxStore() (*FirefoxStore, error) {
	dbPath, err := findFirefoxCookiePath()
	if err != nil {
		return nil, err
	}
	return &FirefoxStore{dbPath: dbPath}, nil
}

// findFirefoxCookiePath 查找 Firefox cookies.sqlite 文件路径
func findFirefoxCookiePath() (string, error) {
	profileDir, err := findFirefoxProfileDir()
	if err != nil {
		return "", err
	}

	cookiePath := filepath.Join(profileDir, "cookies.sqlite")
	if _, err := os.Stat(cookiePath); err != nil {
		return "", fmt.Errorf("未找到 Firefox Cookie 文件: %s", cookiePath)
	}

	log.Printf("找到 Firefox Cookie 文件: %s", cookiePath)
	return cookiePath, nil
}

// findFirefoxProfileDir 查找 Firefox 默认 profile 目录
func findFirefoxProfileDir() (string, error) {
	var profilesRoot string

	if runtime.GOOS == "linux" && isWSL2() {
		home, err := getWSL2WindowsHome()
		if err == nil {
			winProfilesRoot := filepath.Join(home, "AppData", "Roaming", "Mozilla", "Firefox", "Profiles")
			if dir, err := findDefaultProfile(winProfilesRoot); err == nil {
				return dir, nil
			}
		} else {
			log.Printf("警告: 无法获取 Windows 家目录: %v", err)
		}
	}

	switch runtime.GOOS {
	case "windows":
		profilesRoot = filepath.Join(os.Getenv("APPDATA"), "Mozilla", "Firefox", "Profiles")
	case "linux":
		profilesRoot = filepath.Join(os.Getenv("HOME"), ".mozilla", "firefox")
	default:
		return "", fmt.Errorf("不支持的操作系统: %s（仅支持 Windows 和 Linux）", runtime.GOOS)
	}

	return findDefaultProfile(profilesRoot)
}

// findDefaultProfile 在 profiles 根目录下找到默认 profile
//
// Firefox profile 目录名格式通常为 "<random>.default-release" 或 "<random>.default"。
// 优先选择 .default-release（Firefox 67+ 的默认 profile）。
func findDefaultProfile(profilesRoot string) (string, error) {
	entries, err := os.ReadDir(profilesRoot)
	if err != nil {
		return "", fmt.Errorf("无法读取 Firefox Profiles 目录 %s: %w", profilesRoot, err)
	}

	var defaultRelease, defaultProfile, anyProfile string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		path := filepath.Join(profilesRoot, name)

		if strings.HasSuffix(name, ".default-release") {
			defaultRelease = path
		} else if strings.HasSuffix(name, ".default") {
			defaultProfile = path
		} else if anyProfile == "" {
			// 记录第一个找到的 profile 作为兜底
			if _, err := os.Stat(filepath.Join(path, "cookies.sqlite")); err == nil {
				anyProfile = path
			}
		}
	}

	if defaultRelease != "" {
		return defaultRelease, nil
	}
	if defaultProfile != "" {
		return defaultProfile, nil
	}
	if anyProfile != "" {
		log.Printf("警告: 未找到默认 Firefox profile，使用: %s", anyProfile)
		return anyProfile, nil
	}

	return "", fmt.Errorf("未找到 Firefox profile 目录: %s", profilesRoot)
}

// GetCookies 实现 Store 接口
func (s *FirefoxStore) GetCookies(domain string) ([]Cookie, error) {
	db, cleanup, err := s.openDB()
	if err != nil {
		return nil, fmt.Errorf("打开 Firefox 数据库失败: %w", err)
	}
	defer cleanup()

	// Firefox moz_cookies 表：host, name, value, path, expiry, isSecure, isHttpOnly
	// Firefox 不加密 Cookie 值
	query := `
		SELECT host, name, value, path, expiry, isSecure, isHttpOnly
		FROM moz_cookies
		WHERE host LIKE ?
		ORDER BY name
	`

	domainPattern := "%" + domain
	rows, err := db.Query(query, domainPattern)
	if err != nil {
		return nil, fmt.Errorf("查询 Firefox 数据库失败: %w", err)
	}
	defer rows.Close()

	var cookies []Cookie
	for rows.Next() {
		var host, name, value, path string
		var expiry, isSecure, isHTTPOnly int64

		if err := rows.Scan(&host, &name, &value, &path, &expiry, &isSecure, &isHTTPOnly); err != nil {
			return nil, fmt.Errorf("读取行失败: %w", err)
		}

		cookies = append(cookies, Cookie{
			Name:     name,
			Value:    value,
			Domain:   host,
			Path:     path,
			Expires:  expiry,
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
func (s *FirefoxStore) ListDomains() ([]string, error) {
	db, cleanup, err := s.openDB()
	if err != nil {
		return nil, fmt.Errorf("打开 Firefox 数据库失败: %w", err)
	}
	defer cleanup()

	query := `
		SELECT DISTINCT host
		FROM moz_cookies
		ORDER BY host
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("查询 Firefox 数据库失败: %w", err)
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

// copyFileTo 复制单个文件到指定目标路径。
// 源路径以 /mnt/ 开头（WSL2 访问 Windows 文件）时，Firefox 可能独占锁定，
// 需走 Windows 端复制（copyViaCreateFileW → copyViaCmdCopy），再用 moveFromWindows 搬回 Linux 侧。
func copyFileTo(dst, src string) error {
	if isWSL2() && strings.HasPrefix(src, "/mnt/") {
		winSrc := wslPathToWindows(src)
		// 生成唯一 Windows 临时路径（参考 store.go copyToTempViaWindows 的做法，但需唯一避免三件套冲突）
		pid := os.Getpid()
		// 用文件名后缀区分三件套，避免覆盖
		base := filepath.Base(src) // cookies.sqlite / cookies.sqlite-wal / cookies.sqlite-shm
		winTmp := fmt.Sprintf(`C:\Windows\Temp\cookie_%d_%s`, pid, base)
		wslTmp := fmt.Sprintf("/mnt/c/Windows/Temp/cookie_%d_%s", pid, base)
		defer os.Remove(wslTmp)

		if err := copyViaCreateFileW(winSrc, winTmp); err != nil {
			if err2 := copyViaCmdCopy(winSrc, winTmp); err2 != nil {
				return fmt.Errorf("Windows 端复制 %s 失败: %v（CreateFileW）/%v（cmd copy）", base, err, err2)
			}
		}
		linuxTmp, err := moveFromWindows(wslTmp)
		if err != nil {
			return fmt.Errorf("搬回 Linux 侧失败: %w", err)
		}
		defer os.Remove(linuxTmp)
		return os.Rename(linuxTmp, dst)
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// copyFirefoxDB 将 Firefox Cookie 数据库三件套（主文件 + WAL + SHM）复制到独立临时目录，
// 保持 cookies.sqlite 原名，使 SQLite 打开时能自动应用 WAL 日志。返回临时目录路径。
func copyFirefoxDB(dbPath string) (string, error) {
	dir, err := os.MkdirTemp("", "cookie-firefox-*")
	if err != nil {
		return "", fmt.Errorf("创建临时目录失败: %w", err)
	}

	// 主文件必须存在
	mainDst := filepath.Join(dir, "cookies.sqlite")
	if err := copyFileTo(mainDst, dbPath); err != nil {
		os.RemoveAll(dir)
		return "", fmt.Errorf("复制主文件失败: %w", err)
	}

	// -wal 与 -shm 可能不存在（Firefox 关闭并 checkpoint 后），忽略不存在错误
	for _, suffix := range []string{"-wal", "-shm"} {
		src := dbPath + suffix
		if _, err := os.Stat(src); err != nil {
			continue // 不存在则跳过
		}
		if err := copyFileTo(filepath.Join(dir, "cookies.sqlite"+suffix), src); err != nil {
			os.RemoveAll(dir)
			return "", fmt.Errorf("复制 %s 失败: %w", suffix, err)
		}
	}
	return dir, nil
}

// openDB 打开 Firefox Cookie 数据库。
// Firefox 使用 WAL 模式且运行时会锁定数据库，因此复制三件套到临时目录后用 mode=ro 打开，
// 让 SQLite 自动应用 WAL 日志，避免丢失尚未 checkpoint 的最新 Cookie。
func (s *FirefoxStore) openDB() (*sql.DB, func(), error) {
	dir, err := copyFirefoxDB(s.dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("复制 Firefox Cookie 数据库失败: %w", err)
	}

	mainPath := filepath.Join(dir, "cookies.sqlite")
	db, err := sql.Open("sqlite3", mainPath+"?mode=ro")
	if err != nil {
		os.RemoveAll(dir)
		return nil, nil, fmt.Errorf("打开临时数据库失败: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		os.RemoveAll(dir)
		return nil, nil, fmt.Errorf("连接临时数据库失败: %w", err)
	}

	cleanup := func() {
		db.Close()
		os.RemoveAll(dir)
	}
	return db, cleanup, nil
}
