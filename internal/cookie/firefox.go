package cookie

import (
	"database/sql"
	"fmt"
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

// openDB 打开 Firefox Cookie 数据库
// Firefox 运行时会锁定数据库，所以需要复制到临时文件
func (s *FirefoxStore) openDB() (*sql.DB, func(), error) {
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
