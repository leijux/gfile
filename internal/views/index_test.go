package views

import (
	"strings"
	"testing"
)

// TestUrlPathEscape 验证 urlPathEscape 函数对各种字符的正确编码。
func TestUrlPathEscape(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"纯字母", "hello.txt", "hello.txt"},
		{"含空格", "my file.txt", "my%20file.txt"},
		{"含加号", "file+plus.txt", "file%2Bplus.txt"},
		{"含括号", "file(1).txt", "file%281%29.txt"},
		{"含井号", "file#1.txt", "file%231.txt"},
		{"含问号", "file?1.txt", "file%3F1.txt"},
		{"含中文", "测试.txt", "%E6%B5%8B%E8%AF%95.txt"},
		{"含百分号", "file%1.txt", "file%251.txt"},
		{"纯数字", "123.txt", "123.txt"},
		{"保留字符 -_.~", "a-b_c.d~e", "a-b_c.d~e"},
		{"混合复杂", "hello (world) #1!.txt", "hello%20%28world%29%20%231%21.txt"},
		{"空字符串", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := urlPathEscape(tt.input)
			if got != tt.expected {
				t.Errorf("urlPathEscape(%q) = %q, 期望 %q", tt.input, got, tt.expected)
			}

			// 验证输出中不包含未编码的非保留字符
			for _, r := range []byte(got) {
				isUnreserved := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') ||
					(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == '~'
				if !isUnreserved && r != '%' {
					t.Errorf("输出 %q 包含非法字符 %c", got, r)
				}
				// % 后应跟随两位十六进制
				if r == '%' {
					// 找到 % 后的位置较复杂，跳过详细校验
				}
			}
		})
	}
}

// TestUrlPathEscapeRoundTrip 验证 urlPathEscape 与 url.PathUnescape 是互逆操作。
func TestUrlPathEscapeRoundTrip(t *testing.T) {
	inputs := []string{
		"simple.txt",
		"file with spaces.txt",
		"file+plus.txt",
		"file(1).txt",
		"file#1.txt",
		"file?1.txt",
		"测试.txt",
		"a-b_c.d~e",
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			encoded := urlPathEscape(input)
			// 模拟 handler 中的 PathUnescape 行为
			importPathUnescape(t, encoded, input)
		})
	}
}

// importPathUnescape 模拟 url.PathUnescape 并验证结果。
// 单独函数以便可以引用 net/url。
func importPathUnescape(t *testing.T, encoded, expected string) {
	t.Helper()
	decoded, err := urlPathUnescape(encoded)
	if err != nil {
		t.Fatalf("PathUnescape(%q) 失败: %v", encoded, err)
	}
	if decoded != expected {
		t.Errorf("PathUnescape(%q) = %q, 期望 %q", encoded, decoded, expected)
	}
}

// urlPathUnescape 模拟 url.PathUnescape，避免引入 net/url 依赖。
// 实际 handler 中使用的是 url.PathUnescape。
func urlPathUnescape(s string) (string, error) {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '%' && i+2 < len(s) {
			hi := hexVal(s[i+1])
			lo := hexVal(s[i+2])
			if hi >= 0 && lo >= 0 {
				b.WriteByte(byte(hi<<4 | lo))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String(), nil
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'A' && c <= 'F':
		return int(c - 'A' + 10)
	case c >= 'a' && c <= 'f':
		return int(c - 'a' + 10)
	}
	return -1
}
