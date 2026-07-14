// Package handler 提供文件服务器的 HTTP 请求处理逻辑。
// 包含文件上传、下载、列表展示等核心业务功能。
package handler

import (
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	contribi18n "github.com/gofiber/contrib/v3/i18n"

	"gfile/internal/extractor"
	"gfile/internal/i18n"
	"gfile/internal/response"
	"gfile/internal/views"

	"github.com/gofiber/fiber/v3"
)

// Handler 持有文件处理所需的依赖。
// UploadDir 指定文件存放的根目录，Translator 用于多语言消息翻译。
type Handler struct {
	UploadDir  string
	Translator *contribi18n.I18n
}

// New 创建并返回一个新的 Handler 实例。
// uploadDir 是文件上传后存放的目录路径。
// translator 是 i18n 翻译器，用于多语言支持。
func New(uploadDir string, translator *contribi18n.I18n) *Handler {
	return &Handler{UploadDir: uploadDir, Translator: translator}
}

// t 是 i18n.T 的快捷方式，简化 handler 中的翻译调用。
func (h *Handler) t(c fiber.Ctx, msgID string, data ...map[string]string) string {
	return i18n.T(h.Translator, c, msgID, data...)
}

// buildTranslations 根据当前语言构建页面翻译数据。
func (h *Handler) buildTranslations(c fiber.Ctx) views.Translations {
	// 从请求中提取语言代码
	lang := strings.ToLower(h.requestLang(c))

	// 根据当前语言确定切换选项
	var altLang, altLabel string
	if strings.HasPrefix(lang, "en") {
		altLang = "zh-CN"
		altLabel = "中文"
	} else {
		altLang = "en-US"
		altLabel = "English"
	}

	return views.Translations{
		Title:         h.t(c, "pageTitle"),
		Heading:       h.t(c, "heading"),
		UploadBtn:     h.t(c, "uploadBtn"),
		FileListTitle: h.t(c, "fileListTitle"),
		FileName:      h.t(c, "fileName"),
		Size:          h.t(c, "size"),
		ModTime:       h.t(c, "modTime"),
		Actions:       h.t(c, "actions"),
		NoFiles:       h.t(c, "noFiles"),
		Download:      h.t(c, "download"),
		CurrentLang:   lang,
		AltLang:       altLang,
		AltLangLabel:  altLabel,
	}
}

// requestLang 从请求中提取语言代码，优先级：query > header > 默认 "zh"。
func (h *Handler) requestLang(c fiber.Ctx) string {
	if c == nil {
		return "zh"
	}
	if lang := c.Query("lang"); lang != "" {
		return lang
	}
	if lang := c.Get("Accept-Language"); lang != "" {
		// Accept-Language 可能为 "zh-CN,zh;q=0.9" 等格式，取第一个值
		parts := strings.Split(lang, ",")
		return strings.TrimSpace(parts[0])
	}
	return "zh"
}

// Index 渲染首页 HTML 页面。
// 页面包含文件上传表单和已上传文件列表，使用 templ 组件生成。
//
// @Summary		首页
// @Description	返回包含上传表单和文件列表的 HTML 页面
// @Tags			页面
// @Produce		html
// @Success		200	{string}	string	"HTML 页面"
// @Router		/ [get]
func (h *Handler) Index(c fiber.Ctx) error {
	c.Set("Content-Type", "text/html; charset=utf-8")

	// 获取已上传的文件列表
	files, _ := h.listFiles()
	fileInfos := make([]views.FileInfo, 0, len(files))
	for _, fi := range files {
		fileInfos = append(fileInfos, views.FileInfo{
			Name:    fi.Name(),
			Size:    fi.Size(),
			ModTime: fi.ModTime(),
		})
	}

	// 获取当前语言的翻译数据并渲染页面
	tr := h.buildTranslations(c)
	return views.IndexPage(fileInfos, tr).Render(c.Context(), c)
}

// Upload 处理 multipart/form-data 文件上传。
// 请求中必须包含名为 "file" 的文件字段。
// 上传成功返回 201，文件已存在返回 409，参数错误返回 400。
//
// @Summary		上传文件
// @Description	上传单个文件。支持普通文件以及 .zip / .7z 压缩包（自动解压）
// @Tags			文件操作
// @Accept		mpfd
// @Produce		json
// @Param		file	formData	file	true	"上传的文件"
// @Success		201	{object}	response.Response	"上传成功"
// @Failure		400	{object}	response.Response	"请求错误"
// @Failure		409	{object}	response.Response	"文件已存在"
// @Failure		500	{object}	response.Response	"服务器错误"
// @Router		/upload [post]
func (h *Handler) Upload(c fiber.Ctx) error {
	// 从 multipart 表单中读取上传文件
	file, err := c.FormFile("file")
	if err != nil {
		return response.BadRequest(c, h.t(c, "fileMissing", map[string]string{"Error": err.Error()}))
	}

	// 净化文件名，防止路径穿越攻击
	filename := filepath.Base(file.Filename)
	if filename == "." || filename == "" {
		return response.BadRequest(c, h.t(c, "invalidFilename"))
	}

	// 构造目标文件路径
	dst := filepath.Join(h.UploadDir, filename)

	// 检查文件是否已存在，避免覆盖
	if _, err := os.Stat(dst); err == nil {
		return response.Conflict(c, h.t(c, "fileExists", map[string]string{"Filename": filename}))
	}

	// 将上传的文件保存到磁盘
	if err := c.SaveFile(file, dst); err != nil {
		return response.InternalError(c, h.t(c, "saveFailed", map[string]string{"Error": err.Error()}))
	}

	// 构建响应数据
	data := fiber.Map{
		"filename": filename,
		"size":     file.Size,
	}

	// 检测是否为压缩包并自动解压
	if ext := extractor.ArchiveExt(filename); ext != "" {
		if result, err := extractor.Extract(dst, h.UploadDir); err != nil {
			// 解压失败时仅记录，不妨碍文件已上传的事实
			data["extract_error"] = err.Error()
		} else {
			data["extracted"] = result.Extracted
		}
	}

	// 返回统一格式的上传成功响应
	return response.Created(c, data, h.t(c, "uploadSuccess"))
}

// Download 根据文件名提供文件下载。
// 包含路径穿越防护，确保只能下载上传目录内的文件。
// 文件不存在返回 404。
//
// @Summary		下载文件
// @Description	以附件形式下载指定文件
// @Tags			文件操作
// @Produce		application/octet-stream
// @Param		filename	path	string	true	"文件名"
// @Success		200	{file}		binary	"文件内容"
// @Failure		400	{object}	response.Response	"请求错误"
// @Failure		404	{object}	response.Response	"文件不存在"
// @Router		/download/{filename} [get]
func (h *Handler) Download(c fiber.Ctx) error {
	filename := c.Params("filename")
	if filename == "" {
		return response.BadRequest(c, h.t(c, "filenameMissing"))
	}

	// URL 解码，防止 %2e%2e 编码绕过路径穿越检查
	if decoded, err := url.PathUnescape(filename); err == nil {
		filename = decoded
	}

	// 路径穿越防护：禁止包含 ".." 或路径分隔符
	if strings.Contains(filename, "..") ||
		strings.Contains(filename, "/") ||
		strings.Contains(filename, string(os.PathSeparator)) {
		return response.BadRequest(c, h.t(c, "invalidPath"))
	}

	// 使用 filepath.Base 进一步确保安全性
	safeName := filepath.Base(filename)
	path := filepath.Join(h.UploadDir, safeName)

	// 检查文件是否存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return response.NotFound(c, h.t(c, "fileNotFound", map[string]string{"Filename": safeName}))
	}

	// 以附件形式提供下载
	return c.Download(path, safeName)
}

// View 在浏览器中内联预览文件（静态文件服务）。
// 与 Download 不同，View 不会弹出下载对话框，而是直接展示文件内容。
// 包含路径穿越防护，确保只能预览上传目录内的文件。
// 文件不存在返回 404。
//
// @Summary		预览文件
// @Description	在浏览器中内联展示文件内容（图片、文本等支持直接预览）
// @Tags			文件操作
// @Produce		application/octet-stream
// @Param		filename	path	string	true	"文件名"
// @Success		200	{file}		binary	"文件内容"
// @Failure		400	{object}	response.Response	"请求错误"
// @Failure		404	{object}	response.Response	"文件不存在"
// @Router		/view/{filename} [get]
func (h *Handler) View(c fiber.Ctx) error {
	filename := c.Params("filename")
	if filename == "" {
		return response.BadRequest(c, h.t(c, "filenameMissing"))
	}

	// URL 解码，防止 %2e%2e 编码绕过路径穿越检查
	if decoded, err := url.PathUnescape(filename); err == nil {
		filename = decoded
	}

	// 路径穿越防护：禁止包含 ".." 或路径分隔符
	if strings.Contains(filename, "..") ||
		strings.Contains(filename, "/") ||
		strings.Contains(filename, string(os.PathSeparator)) {
		return response.BadRequest(c, h.t(c, "invalidPath"))
	}

	// 使用 filepath.Base 进一步确保安全性
	safeName := filepath.Base(filename)
	path := filepath.Join(h.UploadDir, safeName)

	// 检查文件是否存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return response.NotFound(c, h.t(c, "fileNotFound", map[string]string{"Filename": safeName}))
	}

	// 使用 SendFile 内联发送文件，浏览器会根据 Content-Type 自动预览
	return c.SendFile(path)
}

// List 以 JSON 格式返回已上传文件的列表。
// 返回每个文件的名称、大小和最后修改时间。
//
// @Summary		文件列表
// @Description	以 JSON 格式返回所有已上传文件的信息
// @Tags			文件操作
// @Produce		json
// @Success		200	{object}	object	"文件列表"
// @Failure		500	{object}	response.Response	"服务器错误"
// @Router		/files [get]
func (h *Handler) List(c fiber.Ctx) error {
	files, err := h.listFiles()
	if err != nil {
		return response.InternalError(c, h.t(c, "listFailed", map[string]string{"Error": err.Error()}))
	}

	// 内部结构体，用于 JSON 序列化
	type fileInfo struct {
		Name    string `json:"name"`
		Size    int64  `json:"size"`
		ModTime string `json:"mod_time"`
	}

	infos := make([]fileInfo, 0, len(files))
	for _, fi := range files {
		infos = append(infos, fileInfo{
			Name:    fi.Name(),
			Size:    fi.Size(),
			ModTime: fi.ModTime().Format(time.RFC3339),
		})
	}

	return response.Ok(c, fiber.Map{
		"files": infos,
		"count": len(infos),
	})
}

// listFiles 扫描上传目录，返回按修改时间降序排列的普通文件列表。
// 如果目录不存在则自动创建。
func (h *Handler) listFiles() ([]os.FileInfo, error) {
	entries, err := os.ReadDir(h.UploadDir)
	if err != nil {
		// 目录不存在时自动创建
		if os.IsNotExist(err) {
			os.MkdirAll(h.UploadDir, 0755)
			return nil, nil
		}
		return nil, err
	}

	// 过滤出普通文件，跳过子目录
	files := make([]os.FileInfo, 0, len(entries))
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, info)
	}

	// 按修改时间降序排列（最新的在前）
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime().After(files[j].ModTime())
	})

	return files, nil
}
