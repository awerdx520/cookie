package native

import "testing"

// TestGenerateIDForPathWindows 验证 Windows 路径（UTF-16LE + 盘符大写）的 ID 计算。
// 期望值与 Chromium GenerateIdForPath 真实输出对照（实测 ID: nenjeagkplpblkkoncfapcjnjmiiilbo，
// 输入路径 C:\Users\thomas\cookie-bridge-extension）。
func TestGenerateIDForPathWindows(t *testing.T) {
	got := GenerateIDForPath(`C:\Users\thomas\cookie-bridge-extension`)
	want := "nenjeagkplpblkkoncfapcjnjmiiilbo"
	if got != want {
		t.Fatalf("GenerateIDForPath() = %q, want %q", got, want)
	}
}

// TestGenerateIDForPathWindowsLowerDrive 验证 MaybeNormalizePath 等价行为：
// 小写盘符路径（c:\...）与盘符转大写后应得到相同 ID。
func TestGenerateIDForPathWindowsLowerDrive(t *testing.T) {
	lower := GenerateIDForPath(`c:\Users\thomas\cookie-bridge-extension`)
	upper := GenerateIDForPath(`C:\Users\thomas\cookie-bridge-extension`)
	if lower != upper {
		t.Fatalf("小写盘符路径 ID = %q, 大写盘符路径 ID = %q, 应一致", lower, upper)
	}
}

// TestGenerateIDForPathLinux 验证 Linux 路径（UTF-8）的 ID 计算：
// 输出为 32 字符且全部落在扩展 ID 字母表 a-p。
func TestGenerateIDForPathLinux(t *testing.T) {
	got := GenerateIDForPath("/home/thomas/cookie-bridge-extension")
	if len(got) != 32 {
		t.Fatalf("GenerateIDForPath() 长度 = %d, want 32", len(got))
	}
	for _, r := range got {
		if r < 'a' || r > 'p' {
			t.Fatalf("GenerateIDForPath() 包含非法字符 %q", r)
		}
	}
}
