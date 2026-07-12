// GFile — 基于 Fiber v3 的轻量级文件上传与下载服务器。
// 提供文件上传、下载、列表查看功能，支持中文界面。
//
// @title			GFile API
// @version			1.0.0
// @description		轻量级文件上传与下载服务器，支持 zip/7z 自动解压
// @host			localhost:8080
// @BasePath		/
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gfile/internal/handler"
	"gfile/internal/response"

	"github.com/gofiber/contrib/v3/swaggo"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/recover"

	// 导入 swag 生成的文档，确保 swaggo 能获取到 API 定义
	_ "gfile/docs"
)

func main() {
	// 解析命令行参数
	port := flag.Int("port", 8080, "服务监听端口")
	uploadDir := flag.String("dir", "./uploads", "文件上传目录")
	flag.Parse()

	// 将上传目录解析为绝对路径
	absDir, err := filepath.Abs(*uploadDir)
	if err != nil {
		log.Fatalf("无法解析上传目录路径: %v", err)
	}

	// 确保上传目录存在，不存在则自动创建
	if err := os.MkdirAll(absDir, 0755); err != nil {
		log.Fatalf("无法创建上传目录 %s: %v", absDir, err)
	}

	// 创建处理器实例，注入上传目录路径
	h := handler.New(absDir)

	// 初始化 Fiber 应用
	app := fiber.New(fiber.Config{
		AppName:      "GFile",
		BodyLimit:    100 * 1024 * 1024, // 请求体上限：100 MB
		ErrorHandler: customErrorHandler, // 自定义错误处理
	})

	// 注册全局中间件
	app.Use(recover.New())                   // 防止 panic 导致进程崩溃
	app.Use(logger.New(logger.Config{        // 请求日志
		Format: "[${ip}] ${method} ${path} ${status} ${latency}\n",
	}))

	// 注册路由
	app.Get("/", h.Index)                          // 首页（HTML 上传页面）
	app.Post("/upload", h.Upload)                  // 文件上传
	app.Get("/view/:filename", h.View)             // 文件预览（内联静态文件）
	app.Get("/download/:filename", h.Download)     // 文件下载
	app.Get("/files", h.List)                      // 文件列表（JSON）

	// Swagger 文档
	app.Get("/swagger/*", swaggo.HandlerDefault)

	// 启动服务
	addr := fmt.Sprintf(":%d", *port)
	fmt.Printf("🚀 GFile 文件服务器启动于 http://localhost%s\n", addr)
	fmt.Printf("📁 上传目录: %s\n", absDir)
	fmt.Printf("📖 Swagger 文档: http://localhost%s/swagger/\n", addr)

	if err := app.Listen(addr); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}

// customErrorHandler 是 Fiber 的自定义错误处理器。
// 对 JSON 接口请求返回 JSON 错误，其他请求返回纯文本。
func customErrorHandler(c fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError

	// 提取 Fiber 错误码
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
	}

	// API 路径或 Accept 为 JSON 时返回统一格式的 JSON 错误
	if c.Get("Accept") == "application/json" ||
		c.Path() == "/files" || c.Path() == "/upload" {
		return response.Error(c, code, err.Error())
	}

	return c.Status(code).SendString(err.Error())
}
