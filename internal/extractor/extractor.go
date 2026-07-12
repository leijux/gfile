// Package extractor 提供压缩包解压功能，支持 zip 和 7z 格式。
// 自动检测文件扩展名并选择对应的解压方式，内置 Zip Slip 防护。
package extractor

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bodgit/sevenzip"
)

// Result 包含解压结果信息。
type Result struct {
	Extracted []string // 解压出的文件列表（相对于目标目录）
	Archive   string   // 原始压缩包文件名
}

// ArchiveExt 判断文件扩展名是否为支持的压缩包格式。
func ArchiveExt(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".zip", ".7z":
		return ext
	default:
		return ""
	}
}

// Extract 将压缩包解压到目标目录。
// 自动根据文件扩展名选择解压方式。
// 返回解压出的文件列表，或错误信息。
func Extract(archivePath, destDir string) (*Result, error) {
	ext := ArchiveExt(archivePath)
	if ext == "" {
		return nil, fmt.Errorf("不支持的压缩格式: %s", filepath.Ext(archivePath))
	}

	switch ext {
	case ".zip":
		return extractZip(archivePath, destDir)
	case ".7z":
		return extract7z(archivePath, destDir)
	default:
		return nil, fmt.Errorf("不支持的压缩格式: %s", ext)
	}
}

// extractZip 解压 zip 文件。
func extractZip(archivePath, destDir string) (*Result, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("打开 zip 文件失败: %w", err)
	}
	defer r.Close()

	return extractZipFiles(r.File, destDir)
}

// extractZipFiles 从 zip 的 File 列表中提取文件。
func extractZipFiles(files []*zip.File, destDir string) (*Result, error) {
	result := &Result{
		Extracted: make([]string, 0),
	}

	for _, f := range files {
		// Zip Slip 防护：验证解压路径在目标目录内
		dstPath, err := safeFilePath(destDir, f.Name)
		if err != nil {
			return nil, fmt.Errorf("文件 %q 路径不安全: %w", f.Name, err)
		}

		if err := extractZipEntry(f, dstPath); err != nil {
			return nil, fmt.Errorf("解压 %q 失败: %w", f.Name, err)
		}

		// 只有普通文件才计入结果
		if !f.FileInfo().IsDir() {
			relPath, _ := filepath.Rel(destDir, dstPath)
			result.Extracted = append(result.Extracted, filepath.ToSlash(relPath))
		}
	}

	return result, nil
}
// extractZipEntry 解压 zip 中的单个条目。
func extractZipEntry(f *zip.File, dstPath string) error {
	// 创建父目录
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err
	}

	// 如果是目录，创建即可
	if f.FileInfo().IsDir() {
		return os.MkdirAll(dstPath, 0755)
	}

	// 打开压缩包内的文件
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("打开条目失败: %w", err)
	}
	defer rc.Close()

	// 创建目标文件
	outFile, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer outFile.Close()

	// 写入内容
	if _, err := io.Copy(outFile, rc); err != nil {
		return fmt.Errorf("写入文件失败: %w", err)
	}

	return nil
}

// extract7z 解压 7z 文件。
func extract7z(archivePath, destDir string) (*Result, error) {
	sz, err := sevenzip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("打开 7z 文件失败: %w", err)
	}
	defer sz.Close()

	result := &Result{
		Extracted: make([]string, 0),
	}

	for _, f := range sz.File {
		// Zip Slip 防护
		dstPath, err := safeFilePath(destDir, f.Name)
		if err != nil {
			return nil, fmt.Errorf("文件 %q 路径不安全: %w", f.Name, err)
		}

		// 创建父目录
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return nil, fmt.Errorf("创建目录失败: %w", err)
		}

		// 如果是目录，创建即可
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dstPath, 0755); err != nil {
				return nil, fmt.Errorf("创建目录 %q 失败: %w", f.Name, err)
			}
			continue
		}

		// 打开压缩包内的文件
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("打开 7z 条目 %q 失败: %w", f.Name, err)
		}

		// 创建目标文件
		outFile, err := os.Create(dstPath)
		if err != nil {
			rc.Close()
			return nil, fmt.Errorf("创建文件 %q 失败: %w", dstPath, err)
		}

		// 写入内容
		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return nil, fmt.Errorf("写入文件 %q 失败: %w", dstPath, err)
		}

		relPath, _ := filepath.Rel(destDir, dstPath)
		result.Extracted = append(result.Extracted, filepath.ToSlash(relPath))
	}

	return result, nil
}

// safeFilePath 验证解压路径是否在目标目录内，防止 Zip Slip 攻击。
// 将文件名中的路径分隔符统一处理，确保最终绝对路径以目标目录为前缀。
func safeFilePath(destDir, entryName string) (string, error) {
	// 清理路径分隔符并取 Base 兜底，但这里我们保留目录结构
	// 使用 filepath.Clean 规范化路径
	cleanName := filepath.Clean(entryName)
	if cleanName == "." || cleanName == "" {
		return "", fmt.Errorf("空文件名")
	}

	// 将路径 join 到目标目录
	fullPath := filepath.Join(destDir, cleanName)

	// 获取目标目录的绝对路径
	absDest, err := filepath.Abs(destDir)
	if err != nil {
		return "", err
	}

	// 获取解压路径的绝对路径
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}

	// 验证解压路径以目标目录为前缀
	if !strings.HasPrefix(absPath, absDest+string(filepath.Separator)) &&
		absPath != absDest {
		return "", fmt.Errorf("路径穿越尝试: %s", entryName)
	}

	return absPath, nil
}
