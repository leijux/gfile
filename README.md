# GFile — 轻量级文件服务器

基于 **Go 1.26** + **Fiber v3** + **a-h/templ** 构建的简单文件上传与下载服务器。

## 功能特性

- 📤 **文件上传** — 通过网页表单或 `curl` 上传文件
- 📥 **文件下载** — 直接下载已上传的文件
- 📋 **文件列表** — 以 JSON 格式或网页表格查看已上传文件
- 📦 **压缩包自动解压** — 上传 `.zip` 或 `.7z` 文件时自动解压到上传目录
- 🛡️ **多重安全防护** — 路径穿越防护、Zip Slip 防护、文件名净化
- 🧩 **类型安全模板** — 使用 `a-h/templ` 编译生成类型安全的 HTML

## 快速开始

### 前置要求

- Go 1.26+
- `templ` 命令行工具（用于编译模板）

### 安装与运行

```bash
# 1. 安装 templ 命令行工具
go install github.com/a-h/templ/cmd/templ@latest

# 2. 克隆或进入项目目录
cd gfile

# 3. 编译模板并构建
templ generate && go build -o gfile .

# 4. 启动服务
./gfile

# 或使用自定义端口和上传目录
./gfile --port 9090 --dir /data/files
```

服务默认启动在 `http://localhost:8080`。

### 开发模式运行

```bash
templ generate && go run .
```

## 命令行参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `--port` | `8080` | 服务监听端口 |
| `--dir` | `./uploads` | 文件上传存放目录 |

## API 接口

### `GET /` — 首页

返回包含上传表单和文件列表的 HTML 页面。

### `POST /upload` — 上传文件

上传一个文件到服务器。支持普通文件以及 `.zip` / `.7z` 压缩包。

```bash
# 上传普通文件
curl -X POST http://localhost:8080/upload \
  -F "file=@/path/to/your/file.pdf"

# 上传压缩包（自动解压）
curl -X POST http://localhost:8080/upload \
  -F "file=@/path/to/your/archive.zip"
```

**普通文件上传成功（201）：**
```json
{
  "message": "上传成功",
  "filename": "file.pdf",
  "size": 12345
}
```

**压缩包上传成功（201）：**
```json
{
  "message": "上传成功",
  "filename": "archive.zip",
  "size": 456,
  "extracted": [
    "readme.txt",
    "src/main.go",
    "src/utils.go"
  ]
}
```

> 压缩包上传后，原始压缩包文件保留，同时所有文件被解压到上传目录。`extracted` 字段列出解压出的所有文件路径。

**文件已存在（409）：**
```json
{
  "error": "文件已存在: file.pdf"
}
```

### `GET /download/:filename` — 下载文件

```bash
curl -O http://localhost:8080/download/file.pdf
```

文件以附件形式下载。

### `GET /files` — 文件列表

```bash
curl http://localhost:8080/files
```

**响应示例：**
```json
{
  "files": [
    {
      "name": "file.pdf",
      "size": 12345,
      "mod_time": "2026-07-11T17:00:38+08:00"
    }
  ],
  "count": 1
}
```

## 项目结构

```
gfile/
├── main.go                       # 入口：CLI 参数、Fiber 配置、路由注册
├── internal/
│   ├── extractor/
│   │   ├── extractor.go          # 压缩包解压引擎（zip + 7z）
│   │   └── extractor_test.go     # 解压引擎测试
│   ├── handler/
│   │   ├── handler.go            # HTTP 处理器：上传、下载、列表
│   │   └── handler_test.go       # HTTP 处理器测试
│   └── views/
│       ├── index.templ           # templ 模板源码
│       └── index_templ.go        # templ 生成的 Go 代码（勿手动编辑）
├── uploads/                      # 默认上传目录
├── .gitignore
├── go.mod
├── go.sum
└── README.md
```

## 技术栈

| 技术 | 用途 |
|------|------|
| [Go](https://go.dev/) 1.26 | 编程语言 |
| [Fiber v3](https://github.com/gofiber/fiber) | HTTP Web 框架 |
| [a-h/templ](https://github.com/a-h/templ) | 类型安全 HTML 模板引擎 |
| [bodgit/sevenzip](https://github.com/bodgit/sevenzip) | 7z 压缩包解压 |

## 安全说明

- **文件名净化** — 上传文件通过 `filepath.Base` 净化文件名
- **路径穿越防护** — 下载接口对文件名进行 URL 解码后检查是否包含 `..`、`/` 和路径分隔符
- **Zip Slip 防护** — 解压时验证每个条目的绝对路径是否以上传目录为前缀，阻止恶意压缩包逃脱目录
- **文件冲突保护** — 同名文件上传返回 409 错误，不会覆盖已有文件
- **请求体限制** — 请求体大小限制为 100 MB
- **进程稳定性** — 使用 `recover` 中间件防止 panic 导致服务中断

## 支持的压缩格式

| 格式 | 实现 | 说明 |
|------|------|------|
| `.zip` | Go 标准库 `archive/zip` | 内置支持，无需外部依赖 |
| `.7z` | `github.com/bodgit/sevenzip` | 纯 Go 实现，自动识别解压 |
