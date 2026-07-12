// Package response 提供统一的 API 响应封装格式。
// 所有 JSON 接口均通过此包返回，确保客户端解析一致。
//
// 成功响应格式：{"code": 0, "message": "ok", "data": {...}}
// 错误响应格式：{"code": <http_status>, "message": "<错误描述>"}
package response

import "github.com/gofiber/fiber/v3"

// Response 是统一的 API 响应结构体。
type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Ok 返回 200 成功响应。
// data 为需要返回的业务数据，可以为 nil。
func Ok(c fiber.Ctx, data any) error {
	return c.Status(fiber.StatusOK).JSON(Response{
		Code:    0,
		Message: "ok",
		Data:    data,
	})
}

// Created 返回 201 创建成功响应。
func Created(c fiber.Ctx, data any, message string) error {
	return c.Status(fiber.StatusCreated).JSON(Response{
		Code:    0,
		Message: message,
		Data:    data,
	})
}

// Error 返回指定状态码的错误响应。
func Error(c fiber.Ctx, status int, message string) error {
	return c.Status(status).JSON(Response{
		Code:    status,
		Message: message,
	})
}

// BadRequest 返回 400 错误。
func BadRequest(c fiber.Ctx, message string) error {
	return Error(c, fiber.StatusBadRequest, message)
}

// NotFound 返回 404 错误。
func NotFound(c fiber.Ctx, message string) error {
	return Error(c, fiber.StatusNotFound, message)
}

// Conflict 返回 409 错误。
func Conflict(c fiber.Ctx, message string) error {
	return Error(c, fiber.StatusConflict, message)
}

// InternalError 返回 500 错误。
func InternalError(c fiber.Ctx, message string) error {
	return Error(c, fiber.StatusInternalServerError, message)
}
