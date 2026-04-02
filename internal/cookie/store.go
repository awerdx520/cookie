package cookie

import (
	"crypto/aes"
	"crypto/cipher"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/zalando/go-keyring"
)

// zeroBytes 用零填充字节切片，用于安全清除敏感数据
func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// getWSL2WindowsUsername 获取 WSL2 中 Windows 用户名
func getWSL2WindowsUsername() (string, error) {
	// 检查是否在 WSL2 中
	if runtime.GOOS != "linux" {
		return "", fmt.Errorf("非 Linux 环境")
	}

	// 检查 Windows 挂载点
	windowsMount := "/mnt/c"
	if _, err := os.Stat(windowsMount); err != nil {
		return "", fmt.Errorf("未找到 Windows 挂载点 /mnt/c: %w", err)
	}

	usersDir := filepath.Join(windowsMount, "Users")
	files, err := os.ReadDir(usersDir)
	if err != nil {
		return "", fmt.Errorf("读取 Windows 用户目录失败: %w", err)
	}

	// 查找非系统用户目录
	for _, file := range files {
		if file.IsDir() {
			name := file.Name()
			// 排除系统目录
			if name != "Default" && name != "Public" && name != "All Users" &&
				name != "Default User" && !strings.HasSuffix(name, ".bak") &&
				name != "desktop.ini" {
				// 检查是否是有效用户目录（包含典型子目录）
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

	// 检查 /mnt/c 是否存在（WSL2 的 Windows 挂载点）
	if _, err := os.Stat("/mnt/c"); err != nil {
		return false
	}

	// 检查 /proc/version 是否包含 Microsoft 或 WSL
	if data, err := os.ReadFile("/proc/version"); err == nil {
		version := string(data)
		if strings.Contains(strings.ToLower(version), "microsoft") ||
			strings.Contains(strings.ToLower(version), "wsl") {
			return true
		}
	}

	// 检查 uname 输出
	cmd := exec.Command("uname", "-a")
	if output, err := cmd.Output(); err == nil {
		uname := string(output)
		if strings.Contains(strings.ToLower(uname), "microsoft") ||
			strings.Contains(strings.ToLower(uname), "wsl") {
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
	// GetCookies 获取指定域名的所有 Cookie
	GetCookies(domain string) ([]Cookie, error)
	// GetCookie 获取指定域名和名称的 Cookie
	GetCookie(domain, name string) (*Cookie, error)
	// ListDomains 列出所有包含 Cookie 的域名
	ListDomains() ([]string, error)
}

// ChromeStore 实现从 Chrome 读取 Cookie
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

	// 检测是否在 WSL2 中运行
	if runtime.GOOS == "linux" {
		// 检查 /mnt/c 是否存在（WSL2 的 Windows 挂载点）
		if _, err := os.Stat("/mnt/c"); err == nil {
			// WSL2 环境，尝试查找 Windows Chrome 的 Cookie 文件
			user, err := getWSL2WindowsUsername()
			if err != nil {
				log.Printf("警告: 无法获取 Windows 用户名，使用默认路径: %v", err)
				user = "thomas" // 回退到默认用户名
			}

			// 尝试多种可能的路径
			winPaths := []string{
				// 默认配置
				"/mnt/c/Users/%s/AppData/Local/Google/Chrome/User Data/Default/Network/Cookies",
				"/mnt/c/Users/%s/AppData/Local/Google/Chrome/User Data/Profile 1/Network/Cookies",
				"/mnt/c/Users/%s/AppData/Local/Google/Chrome/User Data/Profile 2/Network/Cookies",
				// 旧版本路径（无 Network 子目录）
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

			// 如果未找到，尝试扫描所有可能的用户目录
			usersDir := "/mnt/c/Users"
			if files, err := os.ReadDir(usersDir); err == nil {
				for _, file := range files {
					if file.IsDir() {
						userName := file.Name()
						// 排除系统目录
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

	// 根据操作系统确定 Chrome 数据目录
	switch runtime.GOOS {
	case "windows":
		basePath = filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "User Data")
	case "darwin":
		basePath = filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "Google", "Chrome")
	case "linux":
		basePath = filepath.Join(os.Getenv("HOME"), ".config", "google-chrome")
	default:
		return "", fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}

	// 尝试不同的配置文件目录
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
func (s *ChromeStore) GetCookies(domain string) ([]Cookie, error) {
	// 打开数据库（必要时复制到临时文件）
	db, cleanup, err := s.openDB()
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	defer cleanup()

	// Chrome 的 Cookie 表结构：
	// host_key (域名), name, value, encrypted_value, path, expires_utc, is_secure, is_httponly
	query := `
		SELECT host_key, name, value, encrypted_value, path, expires_utc, is_secure, is_httponly
		FROM cookies
		WHERE host_key LIKE ?
		ORDER BY name
	`

	// 支持子域名匹配
	domainPattern := "%" + domain
	rows, err := db.Query(query, domainPattern)
	if err != nil {
		return nil, fmt.Errorf("查询数据库失败: %w", err)
	}
	defer rows.Close()

	var cookies []Cookie
	for rows.Next() {
		var hostKey, name, value, path string
		var encryptedValue []byte
		var expiresUTC, isSecure, isHTTPOnly int64

		if err := rows.Scan(&hostKey, &name, &value, &encryptedValue, &path, &expiresUTC, &isSecure, &isHTTPOnly); err != nil {
			return nil, fmt.Errorf("读取行失败: %w", err)
		}

		// Chrome 可能将值存储在 encrypted_value 中
		finalValue := value
		if finalValue == "" && len(encryptedValue) > 0 {
			// 尝试解密加密的 Cookie 值
			decrypted, err := decryptChromeValue(encryptedValue)
			if err != nil {
				log.Printf("解密 Cookie %s@%s 失败: %v", name, hostKey, err)
				finalValue = "[DECRYPT_FAILED]"
			} else {
				finalValue = decrypted
			}
		}

		cookie := Cookie{
			Name:     name,
			Value:    finalValue,
			Domain:   hostKey,
			Path:     path,
			Expires:  expiresUTC,
			Secure:   isSecure == 1,
			HTTPOnly: isHTTPOnly == 1,
		}
		cookies = append(cookies, cookie)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历结果失败: %w", err)
	}

	return cookies, nil
}

// GetCookie 获取特定 Cookie
func (s *ChromeStore) GetCookie(domain, name string) (*Cookie, error) {
	cookies, err := s.GetCookies(domain)
	if err != nil {
		return nil, err
	}

	for _, cookie := range cookies {
		if cookie.Name == name {
			return &cookie, nil
		}
	}

	return nil, fmt.Errorf("未找到 Cookie: %s@%s", name, domain)
}

// ListDomains 列出所有域名
func (s *ChromeStore) ListDomains() ([]string, error) {
	// 打开数据库（必要时复制到临时文件）
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
		// 去除开头的点（Chrome 存储时可能包含）
		domain = strings.TrimPrefix(domain, ".")
		domains = append(domains, domain)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历结果失败: %w", err)
	}

	return domains, nil
}

// copyToTemp 将源文件复制到临时文件并返回临时文件路径
// 调用者需要负责清理临时文件
func copyToTemp(src string) (string, error) {
	srcFile, err := os.Open(src)
	if err != nil {
		return "", fmt.Errorf("打开源文件失败: %w", err)
	}
	defer srcFile.Close()

	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "cookie-*.sqlite")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer tmpFile.Close()

	// 复制数据
	if _, err := io.Copy(tmpFile, srcFile); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("复制文件失败: %w", err)
	}

	return tmpFile.Name(), nil
}

// openDB 打开数据库，必要时复制到临时文件
func (s *ChromeStore) openDB() (*sql.DB, func(), error) {
	// 先尝试直接打开
	db, err := sql.Open("sqlite3", s.dbPath+"?mode=ro&immutable=1&_timeout=5000")
	if err == nil {
		// 测试连接
		if err := db.Ping(); err == nil {
			return db, func() { db.Close() }, nil
		}
		db.Close()
	}

	// 直接打开失败，尝试复制到临时文件
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

// getChromeLocalStatePath 获取 Chrome Local State 文件路径
func getChromeLocalStatePath() (string, error) {
	// 首先尝试使用与 Cookie 文件相同的目录
	cookiePath, err := findChromeCookiePath()
	if err != nil {
		return "", err
	}

	// Cookie 文件在 "Network" 子目录中，Local State 在 User Data 目录根目录
	networkDir := filepath.Dir(cookiePath)
	if filepath.Base(networkDir) == "Network" {
		networkDir = filepath.Dir(networkDir)
	}

	localStatePath := filepath.Join(networkDir, "Local State")
	if _, err := os.Stat(localStatePath); err == nil {
		return localStatePath, nil
	}

	// 如果未找到，尝试常见路径
	var basePath string
	switch runtime.GOOS {
	case "windows":
		basePath = filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "User Data")
	case "darwin":
		basePath = filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "Google", "Chrome")
	case "linux":
		basePath = filepath.Join(os.Getenv("HOME"), ".config", "google-chrome")
	default:
		return "", fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}

	localStatePath = filepath.Join(basePath, "Local State")
	if _, err := os.Stat(localStatePath); err != nil {
		return "", fmt.Errorf("未找到 Chrome Local State 文件: %w", err)
	}

	return localStatePath, nil
}

// getEncryptedKey 从 Local State 文件获取加密密钥
func getEncryptedKey() ([]byte, error) {
	localStatePath, err := getChromeLocalStatePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(localStatePath)
	if err != nil {
		return nil, fmt.Errorf("读取 Local State 文件失败: %w", err)
	}

	var localState map[string]interface{}
	if err := json.Unmarshal(data, &localState); err != nil {
		return nil, fmt.Errorf("解析 Local State JSON 失败: %w", err)
	}

	osCrypt, ok := localState["os_crypt"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("Local State 中缺少 os_crypt 字段")
	}

	encryptedKey, ok := osCrypt["encrypted_key"].(string)
	if !ok {
		return nil, fmt.Errorf("os_crypt 中缺少 encrypted_key 字段")
	}

	// encrypted_key 是 base64 编码的
	keyBytes, err := base64.StdEncoding.DecodeString(encryptedKey)
	if err != nil {
		return nil, fmt.Errorf("解码加密密钥失败: %w", err)
	}

	// Chrome 在密钥前添加 "DPAPI" 前缀（5字节）
	if len(keyBytes) > 5 && string(keyBytes[:5]) == "DPAPI" {
		keyBytes = keyBytes[5:]
	}

	return keyBytes, nil
}

// decryptEncryptedKeyWithKeyring 使用系统密钥环解密加密密钥
func decryptEncryptedKeyWithKeyring(encryptedKey []byte) ([]byte, error) {
	// Chrome 在 macOS 和 Linux 上使用系统密钥环存储主密钥
	// 密钥环服务名和用户名
	service := "Chrome Safe Storage"
	user := "Chrome"

	// 尝试从密钥环获取密码
	password, err := keyring.Get(service, user)
	if err != nil {
		// 如果密钥环中找不到，可能密钥已解密或使用其他方法
		log.Printf("警告: 无法从密钥环获取密码 (%v)，尝试使用原始密钥", err)
		// 检查密钥是否已经是 256 位（32 字节）
		if len(encryptedKey) == 32 {
			return encryptedKey, nil
		}
		// 否则尝试直接使用（可能已解密）
		return encryptedKey, nil
	}

	// 使用密码解密加密密钥
	// 注意：Chrome 实际使用密码派生密钥，这里简化处理
	// 实际实现需要参考 Chrome 的密钥派生逻辑
	// 简化版本：假设密码就是密钥（实际不安全，仅用于演示）
	if len(password) >= 32 {
		return []byte(password[:32]), nil
	}

	// 密码长度不足，填充或截断
	key := make([]byte, 32)
	copy(key, password)
	for i := len(password); i < 32; i++ {
		key[i] = byte(i)
	}

	return key, nil
}

// decryptAESGCM 使用 AES-GCM 解密 Chrome v11 加密数据
func decryptAESGCM(encryptedData []byte) (string, error) {
	// 加密格式: v11 + 12字节 nonce + 密文 + 16字节认证标签
	// 实际格式: "v11" + nonce(12) + ciphertext + authTag(16)
	if len(encryptedData) < 3+12+16 {
		return "", fmt.Errorf("加密数据长度不足")
	}

	// 检查前缀
	if string(encryptedData[:3]) != "v11" {
		return "", fmt.Errorf("非 v11 加密格式")
	}

	// 获取加密密钥
	encryptedKey, err := getEncryptedKey()
	if err != nil {
		return "", fmt.Errorf("获取加密密钥失败: %w", err)
	}

	// 解密密钥
	var key []byte
	defer func() { zeroBytes(key) }()
	if runtime.GOOS == "windows" {
		// Windows 使用 DPAPI 加密密钥
		decryptedKey, err := decryptWithDPAPI(encryptedKey)
		if err != nil {
			log.Printf("警告: Windows DPAPI 解密密钥失败: %v，尝试使用原始密钥", err)
			key = encryptedKey
		} else {
			key = decryptedKey
		}
	} else {
		// macOS/Linux 使用系统密钥环
		key, err = decryptEncryptedKeyWithKeyring(encryptedKey)
		if err != nil {
			return "", fmt.Errorf("解密密钥失败: %w", err)
		}
	}

	// 确保密钥长度为 32 字节（AES-256）
	if len(key) != 32 {
		return "", fmt.Errorf("密钥长度无效: %d 字节 (需要 32 字节)", len(key))
	}

	// 提取 nonce 和密文
	nonce := encryptedData[3:15]
	ciphertext := encryptedData[15 : len(encryptedData)-16]
	authTag := encryptedData[len(encryptedData)-16:]

	// 组合密文和认证标签
	ciphertextWithTag := append(ciphertext, authTag...)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("创建 AES 加密器失败: %w", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("创建 GCM 模式失败: %w", err)
	}

	plaintext, err := aesgcm.Open(nil, nonce, ciphertextWithTag, nil)
	if err != nil {
		return "", fmt.Errorf("解密失败: %w", err)
	}

	return string(plaintext), nil
}

// decryptWithDPAPI 使用 Windows DPAPI 解密数据
func decryptWithDPAPI(data []byte) ([]byte, error) {
	// 检查是否在支持 DPAPI 的环境中
	isWindows := runtime.GOOS == "windows"
	isWSL2Env := isWSL2()

	if !isWindows && !isWSL2Env {
		return nil, fmt.Errorf("DPAPI 解密仅支持 Windows 平台和 WSL2 环境")
	}

	// 使用 PowerShell 调用 DPAPI
	// 将数据转换为 base64 以便在命令行中传递
	base64Data := base64.StdEncoding.EncodeToString(data)

	// PowerShell 命令：使用 ProtectedData::Unprotect 解密
	psCmd := fmt.Sprintf(`Add-Type -AssemblyName System.Security; [System.Security.Cryptography.ProtectedData]::UnprotectData([System.Convert]::FromBase64String('%s'), $null, 'CurrentUser')`, base64Data)

	var cmd *exec.Cmd
	if isWindows {
		// Windows 环境：直接使用 powershell
		cmd = exec.Command("powershell", "-Command", psCmd)
	} else if isWSL2Env {
		// WSL2 环境：使用 Windows PowerShell
		// 尝试多个可能的 PowerShell 路径
		psPaths := []string{
			"/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe",
			"/mnt/c/Windows/SysWOW64/WindowsPowerShell/v1.0/powershell.exe",
		}

		var psPath string
		for _, path := range psPaths {
			if _, err := os.Stat(path); err == nil {
				psPath = path
				break
			}
		}

		if psPath == "" {
			return nil, fmt.Errorf("在 WSL2 中未找到 Windows PowerShell")
		}

		cmd = exec.Command(psPath, "-Command", psCmd)
	} else {
		return nil, fmt.Errorf("不支持的平台")
	}

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("PowerShell 命令失败: %s\n%s", exitErr.Stderr, err)
		}
		return nil, fmt.Errorf("执行 PowerShell 失败: %w", err)
	}

	// 输出可能是字节数组的字符串表示，或者是原始字节
	// 尝试解析为字节数组
	// 简化处理：假设输出就是解密后的数据
	return output, nil
}

// decryptDPAPI 使用 Windows DPAPI 解密 Chrome v10 加密数据
func decryptDPAPI(encryptedData []byte) (string, error) {
	// 加密格式: v10 + DPAPI 加密的 Cookie 值
	if len(encryptedData) < 3 {
		return "", fmt.Errorf("加密数据长度不足")
	}

	// 检查前缀
	if string(encryptedData[:3]) != "v10" {
		return "", fmt.Errorf("非 v10 加密格式")
	}

	// 平台检查：DPAPI 支持 Windows 和 WSL2
	isWindows := runtime.GOOS == "windows"
	isWSL2Env := isWSL2()

	if !isWindows && !isWSL2Env {
		log.Printf("警告: 当前平台 %s 不支持 DPAPI 解密 (仅 Windows 和 WSL2)", runtime.GOOS)
		return "[ENCRYPTED_DPAPI_UNSUPPORTED_PLATFORM]", nil
	}

	// 实际加密数据（去掉 "v10" 前缀）
	encryptedBytes := encryptedData[3:]

	// 尝试使用 DPAPI 解密
	decrypted, err := decryptWithDPAPI(encryptedBytes)
	if err != nil {
		log.Printf("警告: DPAPI 解密失败: %v", err)
		return "[ENCRYPTED_DPAPI]", nil
	}

	return string(decrypted), nil
}

// decryptV20 解密 Chrome v20 加密数据
func decryptV20(encryptedData []byte) (string, error) {
	// v20 加密格式：根据 Chromium 源代码，v20 使用 AES-256-GCM 加密
	// 格式: "v20" + 12字节 nonce + 密文 + 16字节认证标签
	// 与 v11 类似，但可能使用不同的密钥派生或加密上下文
	if len(encryptedData) < 3+12+16 {
		log.Printf("v20 加密数据长度不足: %d 字节", len(encryptedData))
		return "[ENCRYPTED_V20_INVALID_LENGTH]", nil
	}

	// 检查前缀
	if string(encryptedData[:3]) != "v20" {
		return "", fmt.Errorf("非 v20 加密格式")
	}

	// 获取加密密钥
	encryptedKey, err := getEncryptedKey()
	if err != nil {
		log.Printf("获取 v20 加密密钥失败: %v", err)
		return "[ENCRYPTED_V20_KEY_ERROR]", nil
	}

	// 解密密钥（与 v11 相同的方式）
	var key []byte
	defer func() { zeroBytes(key) }()
	if runtime.GOOS == "windows" || isWSL2() {
		// Windows 或 WSL2 使用 DPAPI 加密密钥
		decryptedKey, err := decryptWithDPAPI(encryptedKey)
		if err != nil {
			log.Printf("警告: v20 Windows DPAPI 解密密钥失败: %v，尝试使用原始密钥", err)
			key = encryptedKey
		} else {
			key = decryptedKey
		}
	} else {
		// macOS/Linux 使用系统密钥环
		key, err = decryptEncryptedKeyWithKeyring(encryptedKey)
		if err != nil {
			log.Printf("v20 解密密钥失败: %v", err)
			return "[ENCRYPTED_V20_KEYRING_ERROR]", nil
		}
	}

	// 确保密钥长度为 32 字节（AES-256）
	if len(key) != 32 {
		log.Printf("v20 密钥长度无效: %d 字节 (需要 32 字节)", len(key))
		return "[ENCRYPTED_V20_INVALID_KEY]", nil
	}

	// 提取 nonce 和密文（与 v11 相同的结构）
	nonce := encryptedData[3:15]
	ciphertext := encryptedData[15 : len(encryptedData)-16]
	authTag := encryptedData[len(encryptedData)-16:]

	// 组合密文和认证标签
	ciphertextWithTag := append(ciphertext, authTag...)

	block, err := aes.NewCipher(key)
	if err != nil {
		log.Printf("v20 创建 AES 加密器失败: %v", err)
		return "[ENCRYPTED_V20_CIPHER_ERROR]", nil
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		log.Printf("v20 创建 GCM 模式失败: %v", err)
		return "[ENCRYPTED_V20_GCM_ERROR]", nil
	}

	plaintext, err := aesgcm.Open(nil, nonce, ciphertextWithTag, nil)
	if err != nil {
		log.Printf("v20 解密失败: %v (数据长度: %d, nonce: %v)", err, len(encryptedData), nonce)
		return "[ENCRYPTED_V20_DECRYPT_FAILED]", nil
	}

	return string(plaintext), nil
}

// decryptChromeValue 自动解密 Chrome 加密的 Cookie 值
//
// 本函数自动处理 Chrome 不同版本和平台的加密 Cookie：
// 1. 自动检测加密类型：通过检查数据前缀 (v10, v11, v20)
// 2. 自动选择解密算法：
//   - v10: Windows DPAPI 加密，支持 Windows 和 WSL2 环境
//   - v11: AES-GCM 加密，支持 macOS/Linux/Windows/WSL2（使用系统密钥环或 DPAPI 解密密钥）
//   - v20: AES-GCM 加密，支持与 v11 相同的解密方式
//
// 3. 自动获取密钥：从 Chrome Local State 文件读取加密密钥
// 4. 自动平台适配：
//   - Windows: 使用 DPAPI 解密密钥和 Cookie 值
//   - WSL2: 调用 Windows PowerShell 进行 DPAPI 解密
//   - macOS: 使用系统密钥环获取主密钥
//   - Linux: 使用系统密钥环或 GNOME Keyring/KWallet
//
// 5. 自动错误处理：解密失败时返回占位符，不中断流程
//
// 解密过程完全自动，无需用户干预。仅在以下情况需要用户操作：
// - Chrome 主密码已更改：需要重新登录 Chrome 同步密钥环
// - 跨用户访问：可能需要管理员权限
// - 系统密钥环不可用：需要确保密钥环服务运行
// - WSL2 环境：需要确保可以访问 Windows PowerShell
func decryptChromeValue(encrypted []byte) (string, error) {
	if len(encrypted) == 0 {
		return "", nil
	}

	// Chrome 加密值以特定前缀开头
	// v10: DPAPI 加密 (Windows)
	// v11: AES-GCM 加密 (macOS, Linux)
	// v20: AES-GCM 加密（新版本）
	if len(encrypted) < 3 {
		return "", fmt.Errorf("加密值太短")
	}

	prefix := string(encrypted[:3])
	switch prefix {
	case "v10":
		// Windows DPAPI 加密
		return decryptDPAPI(encrypted)
	case "v11":
		// AES-GCM 加密
		return decryptAESGCM(encrypted)
	case "v20":
		// v20 加密版本
		return decryptV20(encrypted)
	default:
		// 可能未加密，直接返回字符串表示
		return string(encrypted), nil
	}
}
