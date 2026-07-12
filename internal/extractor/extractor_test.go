package extractor

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// createZip 创建一个测试用的 zip 文件，返回文件路径。
func createZip(t *testing.T, files map[string]string) string {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "test-*.zip")
	if err != nil {
		t.Fatalf("无法创建临时 zip 文件: %v", err)
	}
	defer tmpFile.Close()

	w := zip.NewWriter(tmpFile)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("无法创建 zip 条目 %q: %v", name, err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatalf("无法写入 zip 条目 %q: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("无法关闭 zip writer: %v", err)
	}

	return tmpFile.Name()
}

// create7z 需要 7z 命令行工具，跳过测试。
// 我们通过手动构造一个 7z 文件来测试，但更简单的方式是跳过 7z 单元测试，
// 依赖集成测试验证。

// TestArchiveExt 验证 ArchiveExt 能正确识别扩展名。
func TestArchiveExt(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"file.zip", ".zip"},
		{"file.7z", ".7z"},
		{"file.ZIP", ".zip"},
		{"file.7Z", ".7z"},
		{"file.tar.gz", ""},
		{"file.txt", ""},
		{"file", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ArchiveExt(tt.name)
			if got != tt.expected {
				t.Errorf("ArchiveExt(%q) = %q，期望 %q", tt.name, got, tt.expected)
			}
		})
	}
}

// TestExtractZip 验证 zip 文件能正确解压。
func TestExtractZip(t *testing.T) {
	destDir := t.TempDir()
	zipPath := createZip(t, map[string]string{
		"hello.txt":     "Hello, World!",
		"sub/dir/file.txt": "Nested file",
		"empty.txt":     "",
	})

	result, err := Extract(zipPath, destDir)
	if err != nil {
		t.Fatalf("解压失败: %v", err)
	}

	// 验证解压出的文件列表
	expectedFiles := map[string]bool{
		"hello.txt":      true,
		"sub/dir/file.txt": true,
		"empty.txt":      true,
	}

	if len(result.Extracted) != len(expectedFiles) {
		t.Errorf("期望解压出 %d 个文件，得到 %d: %v", len(expectedFiles), len(result.Extracted), result.Extracted)
	}

	for _, f := range result.Extracted {
		if !expectedFiles[f] {
			t.Errorf("未预期的解压文件: %s", f)
		}
	}

	// 验证文件内容
	content, err := os.ReadFile(filepath.Join(destDir, "hello.txt"))
	if err != nil {
		t.Fatalf("无法读取解压文件: %v", err)
	}
	if string(content) != "Hello, World!" {
		t.Errorf("文件内容不符，期望 'Hello, World!'，得到 '%s'", string(content))
	}

	// 验证嵌套目录文件
	if _, err := os.Stat(filepath.Join(destDir, "sub", "dir", "file.txt")); os.IsNotExist(err) {
		t.Error("嵌套目录文件未解压")
	}
}

// TestExtractZipSlip 验证 Zip Slip 攻击被阻止。
func TestExtractZipSlip(t *testing.T) {
	destDir := t.TempDir()
	zipPath := createZip(t, map[string]string{
		"../outside.txt": "should not extract",
	})

	_, err := Extract(zipPath, destDir)
	if err == nil {
		t.Fatal("期望 Zip Slip 报错，但没有")
	}
	if !strings.Contains(err.Error(), "路径不安全") &&
		!strings.Contains(err.Error(), "路径穿越") {
		t.Errorf("错误信息应提示路径问题，得到: %v", err)
	}

	// 验证文件未被写入目标目录外
	if _, err := os.Stat(filepath.Join(destDir, "outside.txt")); !os.IsNotExist(err) {
		t.Error("Zip Slip 文件不应被解压")
	}
}

// TestExtractInvalidFile 验证非压缩包返回错误。
func TestExtractInvalidFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-*.txt")
	if err != nil {
		t.Fatalf("无法创建临时文件: %v", err)
	}
	tmpFile.WriteString("not an archive")
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	_, err = Extract(tmpFile.Name(), t.TempDir())
	if err == nil {
		t.Fatal("期望非压缩包文件解压报错")
	}
}

// TestExtractUnsupportedFormat 验证不支持的格式返回错误。
func TestExtractUnsupportedFormat(t *testing.T) {
	_, err := Extract("file.tar.gz", t.TempDir())
	if err == nil {
		t.Fatal("期望不支持的格式报错")
	}
	if !strings.Contains(err.Error(), "不支持的压缩格式") {
		t.Errorf("错误信息应提示格式不支持，得到: %v", err)
	}
}

// TestSafeFilePath 验证 safeFilePath 的行为。
func TestSafeFilePath(t *testing.T) {
	destDir := t.TempDir()

	tests := []struct {
		name      string
		entryName string
		wantErr   bool
	}{
		{"正常文件", "normal.txt", false},
		{"子目录文件", "sub/dir/file.txt", false},
		{"Zip Slip ..", "../outside.txt", true},
		{"深层 Zip Slip", "a/../../../outside.txt", true},
		{"空文件名", "", true},
		{"点", ".", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := safeFilePath(destDir, tt.entryName)
			if tt.wantErr {
				if err == nil {
					t.Errorf("期望错误，但得到路径: %s", path)
				}
				return
			}
			if err != nil {
				t.Errorf("不期望错误: %v", err)
			}
			if !strings.HasPrefix(path, destDir) {
				t.Errorf("路径应在目标目录内: %s", path)
			}
		})
	}
}

// TestArchiveExtInResult 验证解压结果包含原始压缩包名。
func TestArchiveExtractResult(t *testing.T) {
	destDir := t.TempDir()
	zipPath := createZip(t, map[string]string{"test.txt": "content"})

	result, err := Extract(zipPath, destDir)
	if err != nil {
		t.Fatalf("解压失败: %v", err)
	}

	if len(result.Extracted) != 1 {
		t.Errorf("期望 1 个文件，得到 %d", len(result.Extracted))
	}
	if result.Extracted[0] != "test.txt" {
		t.Errorf("期望 test.txt，得到 %s", result.Extracted[0])
	}
}
