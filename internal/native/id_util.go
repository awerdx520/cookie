package native

import (
	"crypto/sha256"
	"fmt"
	"unicode/utf16"
)

// hexToID 将 hex 字符映射到 Chrome 扩展 ID 字母表。
// Chromium crx_file::id_util 约定：0-9 -> a-j, a-f -> k-p。
var hexToID = map[rune]rune{
	'0': 'a', '1': 'b', '2': 'c', '3': 'd', '4': 'e', '5': 'f',
	'6': 'g', '7': 'h', '8': 'i', '9': 'j',
	'a': 'k', 'b': 'l', 'c': 'm', 'd': 'n', 'e': 'o', 'f': 'p',
}

// GenerateIDForPath 按 Chromium GenerateIdForPath 算法计算未打包扩展 ID。
// Windows 路径（含盘符如 C:\...）用 UTF-16LE 编码，Linux 路径用 UTF-8。
// 自动检测 Windows 路径：len(path) >= 2 且 path[1] == ':' 且首字符为字母。
// 等价于 MaybeNormalizePath：Windows 路径盘符先转大写再编码。
func GenerateIDForPath(path string) string {
	utf16le := len(path) >= 2 && path[1] == ':' &&
		((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z'))
	if utf16le {
		// MaybeNormalizePath：盘符转大写（如 c: -> C:）
		if path[0] >= 'a' && path[0] <= 'z' {
			path = string(path[0]-'a'+'A') + path[1:]
		}
	}
	return generateID(path, utf16le)
}

// generateID 实现 Chromium crx_file::id_util::GenerateIdForPath 核心算法：
// SHA-256(input) 前 16 字节 -> hex 编码（高 nibble 先）-> 每个 hex 字符映射到
// 扩展 ID 字母表，输出 32 字符 ID。
// utf16le 为 true 时输入按 UTF-16LE 编码（Windows FilePath::value() 原始字节），
// 否则按 UTF-8（Linux FilePath::value()）。
func generateID(path string, utf16le bool) string {
	var input []byte
	if utf16le {
		u16 := utf16.Encode([]rune(path))
		for _, u := range u16 {
			input = append(input, byte(u&0xFF), byte(u>>8))
		}
	} else {
		input = []byte(path)
	}

	sum := sha256.Sum256(input)
	out := make([]rune, 0, 32)
	for i := 0; i < 16; i++ {
		hex := fmt.Sprintf("%02x", sum[i])
		out = append(out, hexToID[rune(hex[0])], hexToID[rune(hex[1])])
	}
	return string(out)
}
