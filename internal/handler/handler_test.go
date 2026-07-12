package handler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

// setupTest 为每个测试创建 Fiber app 和临时上传目录。
// 返回 app、上传目录路径和清理函数。
func setupTest(t *testing.T) (*fiber.App, string) {
	t.Helper()

	// 创建临时目录作为上传目录
	tmpDir := t.TempDir()

	h := New(tmpDir)

	app := fiber.New(fiber.Config{
		AppName:   "GFile-Test",
		BodyLimit: 10 * 1024 * 1024, // 10 MB
	})

	// 注册路由（与服务一致）
	app.Get("/", h.Index)
	app.Post("/upload", h.Upload)
	app.Get("/view/:filename", h.View)
	app.Get("/download/:filename", h.Download)
	app.Get("/files", h.List)

	// Cleanup: 释放 fasthttp.FS 缓存的文件句柄，让 t.TempDir() 可以正常清理。
	// Fiber 的 SendFile/Download 使用 fasthttp.FS 缓存文件句柄（默认 10 秒过期）。
	// 在 Windows 上，被缓存的文件句柄会阻止 t.TempDir() 删除临时目录。
	// 释放 app 引用并强制 GC 触发 runtime.AddCleanup 来释放被缓存的文件句柄。
	t.Cleanup(func() {
		app.Shutdown()
		app = nil
		// 多次 GC 确保 runtime.AddCleanup 被触发
		runtime.GC()
		runtime.GC()
	})

	return app, tmpDir
}

// createUploadRequest 创建一个包含文件上传字段的 HTTP 请求。
func createUploadRequest(t *testing.T, fieldName, fileName, content string) *http.Request {
	t.Helper()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile(fieldName, fileName)
	if err != nil {
		t.Fatalf("无法创建表单文件: %v", err)
	}

	if _, err := io.Copy(part, strings.NewReader(content)); err != nil {
		t.Fatalf("无法写入表单内容: %v", err)
	}
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

// --- 测试用例 ---

// TestIndex 验证首页返回 HTML 且状态码为 200。
func TestIndex(t *testing.T) {
	app, _ := setupTest(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("期望状态码 %d，得到 %d", fiber.StatusOK, resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/html") {
		t.Errorf("期望 Content-Type 为 text/html，得到 %s", contentType)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "GFile") {
		t.Error("响应体中应包含 'GFile'")
	}
}

// TestUploadSuccess 验证文件上传成功返回 201 和正确信息。
func TestUploadSuccess(t *testing.T) {
	app, tmpDir := setupTest(t)

	req := createUploadRequest(t, "file", "hello.txt", "Hello, GFile!")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}

	if resp.StatusCode != fiber.StatusCreated {
		t.Errorf("期望状态码 %d，得到 %d", fiber.StatusCreated, resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("无法解析 JSON 响应: %v", err)
	}

	if result["message"] != "上传成功" {
		t.Errorf("期望 message 为 '上传成功'，得到 %v", result["message"])
	}
	// 使用 data 辅助函数获取嵌套数据
	data, _ := result["data"].(map[string]any)
	if data["filename"] != "hello.txt" {
		t.Errorf("期望 filename 为 'hello.txt'，得到 %v", data["filename"])
	}

	// 验证文件实际写入磁盘
	content, err := os.ReadFile(filepath.Join(tmpDir, "hello.txt"))
	if err != nil {
		t.Fatalf("文件未写入磁盘: %v", err)
	}
	if string(content) != "Hello, GFile!" {
		t.Errorf("文件内容不符，期望 'Hello, GFile!'，得到 '%s'", string(content))
	}
}

// TestUploadNoFile 验证没有上传文件时返回 400。
func TestUploadNoFile(t *testing.T) {
	app, _ := setupTest(t)

	req := httptest.NewRequest(http.MethodPost, "/upload", nil)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("期望状态码 %d，得到 %d", fiber.StatusBadRequest, resp.StatusCode)
	}
}

// TestUploadDuplicate 验证重复上传同一文件返回 409。
func TestUploadDuplicate(t *testing.T) {
	app, _ := setupTest(t)

	// 第一次上传
	req1 := createUploadRequest(t, "file", "duplicate.txt", "content")
	resp1, err := app.Test(req1)
	if err != nil {
		t.Fatalf("第一次请求失败: %v", err)
	}
	if resp1.StatusCode != fiber.StatusCreated {
		t.Fatalf("第一次上传失败: %d", resp1.StatusCode)
	}

	// 第二次上传同一文件
	req2 := createUploadRequest(t, "file", "duplicate.txt", "content again")
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("第二次请求失败: %v", err)
	}

	if resp2.StatusCode != fiber.StatusConflict {
		t.Errorf("期望状态码 %d，得到 %d", fiber.StatusConflict, resp2.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp2.Body).Decode(&result)
	if !strings.Contains(result["message"].(string), "文件已存在") {
		t.Errorf("期望错误包含 '文件已存在'，得到 %v", result["message"])
	}
}

// TestDownloadSuccess 验证文件下载成功。
func TestDownloadSuccess(t *testing.T) {
	app, _ := setupTest(t)

	// 先上传一个文件
	uploadReq := createUploadRequest(t, "file", "download-me.txt", "download content")
	uploadResp, err := app.Test(uploadReq)
	if err != nil || uploadResp.StatusCode != fiber.StatusCreated {
		t.Fatalf("上传前置条件失败: %v", err)
	}

	// 下载该文件
	req := httptest.NewRequest(http.MethodGet, "/download/download-me.txt", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("下载请求失败: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("期望状态码 %d，得到 %d", fiber.StatusOK, resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "download content" {
		t.Errorf("文件内容不符，期望 'download content'，得到 '%s'", string(body))
	}

	// 验证 Content-Disposition 为附件
	cd := resp.Header.Get("Content-Disposition")
	if !strings.HasPrefix(cd, "attachment") {
		t.Errorf("期望 Content-Disposition 以 'attachment' 开头，得到 %s", cd)
	}
}

// TestDownloadNotFound 验证下载不存在的文件返回 404。
func TestDownloadNotFound(t *testing.T) {
	app, _ := setupTest(t)

	req := httptest.NewRequest(http.MethodGet, "/download/nope.txt", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}

	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("期望状态码 %d，得到 %d", fiber.StatusNotFound, resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if !strings.Contains(result["message"].(string), "文件不存在") {
		t.Errorf("期望错误包含 '文件不存在'，得到 %v", result["message"])
	}
}

// TestDownloadPathTraversal 验证各种路径穿越攻击均被拦截。
func TestDownloadPathTraversal(t *testing.T) {
	app, _ := setupTest(t)

	tests := []struct {
		name string
		path string
	}{
		{"原始 .. 路径", "/download/.."},
		{"URL 编码 %2e%2e", "/download/%2e%2e%2fetc%2fpasswd"},
		{"包含反斜杠 %5c", "/download/test%5c..%5cetc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("请求失败: %v", err)
			}

			if resp.StatusCode != fiber.StatusBadRequest {
				t.Errorf("期望状态码 %d，得到 %d", fiber.StatusBadRequest, resp.StatusCode)
			}
		})
	}
}

// TestListEmpty 验证空目录下文件列表返回空。
func TestListEmpty(t *testing.T) {
	app, _ := setupTest(t)

	req := httptest.NewRequest(http.MethodGet, "/files", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("期望状态码 %d，得到 %d", fiber.StatusOK, resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	data, _ := result["data"].(map[string]any)
	if data["count"].(float64) != 0 {
		t.Errorf("期望 count 为 0，得到 %v", data["count"])
	}
}

// TestListWithFiles 验证上传文件后列表能正确返回。
func TestListWithFiles(t *testing.T) {
	app, _ := setupTest(t)

	// 上传两个文件
	for _, f := range []struct{ name, content string }{
		{"alpha.txt", "alpha content"},
		{"beta.txt", "beta content"},
	} {
		req := createUploadRequest(t, "file", f.name, f.content)
		resp, err := app.Test(req)
		if err != nil || resp.StatusCode != fiber.StatusCreated {
			t.Fatalf("上传 %s 失败: %v", f.name, err)
		}
	}

	// 获取文件列表
	req := httptest.NewRequest(http.MethodGet, "/files", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("期望状态码 %d，得到 %d", fiber.StatusOK, resp.StatusCode)
	}

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Files []struct {
				Name    string  `json:"name"`
				Size    float64 `json:"size"`
				ModTime string  `json:"mod_time"`
			} `json:"files"`
			Count int `json:"count"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("无法解析 JSON: %v", err)
	}

	if result.Data.Count != 2 {
		t.Errorf("期望 count 为 2，得到 %d", result.Data.Count)
	}

	// 验证文件名包含
	names := make(map[string]bool)
	for _, f := range result.Data.Files {
		names[f.Name] = true
	}
	if !names["alpha.txt"] {
		t.Error("列表中缺少 alpha.txt")
	}
	if !names["beta.txt"] {
		t.Error("列表中缺少 beta.txt")
	}
}

// TestUploadWritesToDisk 验证上传的文件确实写入到磁盘的 uploadDir 中。
func TestUploadWritesToDisk(t *testing.T) {
	app, tmpDir := setupTest(t)

	req := createUploadRequest(t, "file", "disk-test.txt", "disk content")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("上传失败: %d", resp.StatusCode)
	}

	// 验证文件已写入临时目录
	content, err := os.ReadFile(filepath.Join(tmpDir, "disk-test.txt"))
	if err != nil {
		t.Fatalf("文件未写入磁盘: %v", err)
	}
	if string(content) != "disk content" {
		t.Errorf("文件内容不符，期望 'disk content'，得到 '%s'", string(content))
	}
}

// createZipInMemory 在内存中创建一个 zip 文件，返回 byte 数据。
func createZipInMemory(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("无法创建 zip 条目: %v", err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatalf("无法写入 zip 条目: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("无法关闭 zip writer: %v", err)
	}
	return buf.Bytes()
}

// TestUploadZip 验证上传 zip 文件后自动解压。
func TestUploadZip(t *testing.T) {
	app, tmpDir := setupTest(t)

	zipData := createZipInMemory(t, map[string]string{
		"hello.txt":       "Hello from zip",
		"nested/deep.txt": "Deep file",
	})

	// 构造 multipart 上传请求
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "archive.zip")
	if err != nil {
		t.Fatalf("无法创建表单文件: %v", err)
	}
	if _, err := part.Write(zipData); err != nil {
		t.Fatalf("无法写入表单内容: %v", err)
	}
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("上传请求失败: %v", err)
	}

	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("期望状态码 %d，得到 %d", fiber.StatusCreated, resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("无法解析 JSON: %v", err)
	}

	// 验证响应包含提取信息
	data, _ := result["data"].(map[string]any)
	extracted, ok := data["extracted"].([]any)
	if !ok {
		t.Fatalf("响应中应包含 extracted 数组，得到: %v", data["extracted"])
	}

	if len(extracted) != 2 {
		t.Errorf("期望解压 2 个文件，得到 %d: %v", len(extracted), extracted)
	}

	// 验证压缩包本身保留了
	if _, err := os.Stat(filepath.Join(tmpDir, "archive.zip")); os.IsNotExist(err) {
		t.Error("原始压缩包文件应保留")
	}

	// 验证解压出的文件
	content, err := os.ReadFile(filepath.Join(tmpDir, "hello.txt"))
	if err != nil {
		t.Fatalf("解压文件不存在: %v", err)
	}
	if string(content) != "Hello from zip" {
		t.Errorf("文件内容不符，期望 'Hello from zip'，得到 '%s'", string(content))
	}

	// 验证嵌套目录
	nestedContent, err := os.ReadFile(filepath.Join(tmpDir, "nested", "deep.txt"))
	if err != nil {
		t.Fatalf("嵌套解压文件不存在: %v", err)
	}
	if string(nestedContent) != "Deep file" {
		t.Errorf("嵌套文件内容不符，期望 'Deep file'，得到 '%s'", string(nestedContent))
	}
}

// TestUploadPlainFile 验证普通文件上传不会触发解压。
func TestUploadPlainFile(t *testing.T) {
	app, _ := setupTest(t)

	req := createUploadRequest(t, "file", "readme.txt", "plain text")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	data, _ := result["data"].(map[string]any)
	// 普通文件不应包含 extracted 字段
	if _, exists := data["extracted"]; exists {
		t.Error("普通文件上传响应中不应包含 extracted 字段")
	}
}

// TestViewSuccess 验证文件预览返回文件内容且 Content-Type 正确。
func TestViewSuccess(t *testing.T) {
	app, _ := setupTest(t)

	// 先上传一个文本文件
	uploadReq := createUploadRequest(t, "file", "preview.txt", "preview content")
	uploadResp, err := app.Test(uploadReq)
	if err != nil || uploadResp.StatusCode != fiber.StatusCreated {
		t.Fatalf("上传前置条件失败: %v", err)
	}

	// 预览该文件
	req := httptest.NewRequest(http.MethodGet, "/view/preview.txt", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("预览请求失败: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("期望状态码 %d，得到 %d", fiber.StatusOK, resp.StatusCode)
	}

	// 验证 Content-Type 为文本类型
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("期望 Content-Type 以 text/plain 开头，得到 %s", ct)
	}

	// 验证 Content-Disposition 不应为 attachment（与下载区分）
	cd := resp.Header.Get("Content-Disposition")
	if cd != "" {
		t.Errorf("预览响应不应包含 Content-Disposition，得到 %s", cd)
	}

	// 验证文件内容
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "preview content" {
		t.Errorf("文件内容不符，期望 'preview content'，得到 '%s'", string(body))
	}
}

// TestViewImage 验证图片文件预览返回正确的 Content-Type。
func TestViewImage(t *testing.T) {
	app, tmpDir := setupTest(t)

	// 直接写入一个图片文件到上传目录
	// 使用 1x1 像素 PNG
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, // 1x1 pixel
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41, // IDAT chunk
		0x54, 0x08, 0xD7, 0x63, 0x60, 0x60, 0x00, 0x00,
		0x00, 0x02, 0x00, 0x01, 0xE5, 0x27, 0xDE, 0xFC,
		0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, // IEND chunk
		0xAE, 0x42, 0x60, 0x82,
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "image.png"), pngData, 0644); err != nil {
		t.Fatalf("无法写入测试图片: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/view/image.png", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("预览请求失败: %v", err)
	}

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("期望状态码 %d，得到 %d", fiber.StatusOK, resp.StatusCode)
	}

	// 验证 Content-Type 为图片类型
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/png") {
		t.Errorf("期望 Content-Type 以 image/png 开头，得到 %s", ct)
	}

	// 验证文件内容完整
	body, _ := io.ReadAll(resp.Body)
	if len(body) != len(pngData) {
		t.Errorf("文件大小不符，期望 %d，得到 %d", len(pngData), len(body))
	}
}

// TestViewNotFound 验证预览不存在的文件返回 404。
func TestViewNotFound(t *testing.T) {
	app, _ := setupTest(t)

	req := httptest.NewRequest(http.MethodGet, "/view/missing.txt", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}

	if resp.StatusCode != fiber.StatusNotFound {
		t.Errorf("期望状态码 %d，得到 %d", fiber.StatusNotFound, resp.StatusCode)
	}
}

// TestViewPathTraversal 验证预览接口的路径穿越防护。
func TestViewPathTraversal(t *testing.T) {
	app, _ := setupTest(t)

	req := httptest.NewRequest(http.MethodGet, "/view/..", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Errorf("期望状态码 %d，得到 %d", fiber.StatusBadRequest, resp.StatusCode)
	}
}

// TestIndexLinksEncoded 验证首页 HTML 中的文件链接已正确 URL 编码。
func TestIndexLinksEncoded(t *testing.T) {
	app, _ := setupTest(t)

	// 上传含空格的文件
	req := createUploadRequest(t, "file", "my important file.txt", "content")
	resp, err := app.Test(req)
	if err != nil || resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("上传失败: %v", err)
	}

	// 获取首页 HTML
	indexReq := httptest.NewRequest(http.MethodGet, "/", nil)
	indexResp, err := app.Test(indexReq)
	if err != nil {
		t.Fatalf("获取首页失败: %v", err)
	}

	body, _ := io.ReadAll(indexResp.Body)
	html := string(body)

	// /view/ 链接应编码空格
	if !strings.Contains(html, "/view/my%20important%20file.txt") {
		t.Errorf("view 链接应包含编码后的空格")
	}

	// /download/ 链接应编码空格
	if !strings.Contains(html, "/download/my%20important%20file.txt") {
		t.Errorf("download 链接应包含编码后的空格")
	}

	// 文件名原文应保留空格（显示文本不编码）
	if !strings.Contains(html, "my important file.txt") {
		t.Errorf("页面应显示原始文件名（含空格）")
	}

	// 不应出现未编码的模板表达式
	if strings.Contains(html, "urlPathEscape") {
		t.Error("页面中不应出现未编译的模板表达式")
	}
	if strings.Contains(html, "{ urlPathEscape") {
		t.Error("页面中不应出现未编译的大括号表达式")
	}
}

// TestIndexLinksEncodedChinese 验证中文字符在链接中已被 URL 编码。
func TestIndexLinksEncodedChinese(t *testing.T) {
	chineseDir := t.TempDir()
	err := os.WriteFile(filepath.Join(chineseDir, "测试报告.txt"), []byte("content"), 0644)
	if err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}

	h := New(chineseDir)
	app := fiber.New()
	app.Get("/", h.Index)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, _ := app.Test(req)
	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	// 中文字符应被 URL 编码
	if strings.Contains(html, "/view/测试报告.txt") {
		t.Error("中文字符在链接中应被 URL 编码，不应原文出现")
	}

	// 应该包含编码后的链接（不区分大小写）
	if !strings.Contains(html, "/view/%E6%B5%8B%E8%AF%95%E6%8A%A5%E5%91%8A.txt") &&
		!strings.Contains(html, "/view/%e6%b5%8b%e8%af%95%e6%8a%a5%e5%91%8a.txt") {
		t.Errorf("链接中应包含 URL 编码的中文")
	}

	// 文件名原文在表格中显示
	if !strings.Contains(html, "测试报告.txt") {
		t.Errorf("页面应显示原始中文文件名")
	}
}

// TestViewWithSpecialChars 验证 View 能正确处理含特殊字符的文件名。
func TestViewWithSpecialChars(t *testing.T) {
	app, tmpDir := setupTest(t)

	tests := []struct {
		name    string
		content string
		urlPath string
	}{
		{"simple.txt", "simple", "/view/simple.txt"},
		{"file with spaces.txt", "spaces", "/view/file%20with%20spaces.txt"},
		{"file+plus.txt", "plus", "/view/file%2Bplus.txt"},
		{"file(1).txt", "parentheses", "/view/file%281%29.txt"},
	}

	for _, tt := range tests {
		// 直接写入文件到上传目录
		if err := os.WriteFile(filepath.Join(tmpDir, tt.name), []byte(tt.content), 0644); err != nil {
			t.Fatalf("写入文件 %q 失败: %v", tt.name, err)
		}

		req := httptest.NewRequest(http.MethodGet, tt.urlPath, nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("请求 %s 失败: %v", tt.urlPath, err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Errorf("GET %s: 期望状态码 %d，得到 %d", tt.urlPath, fiber.StatusOK, resp.StatusCode)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != tt.content {
			t.Errorf("GET %s: 期望内容 %q，得到 %q", tt.urlPath, tt.content, string(body))
		}
	}
}
