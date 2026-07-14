// Package i18n 提供多语言支持的封装，基于 gofiber/contrib/i18n。
// 支持中文和英文，通过 Accept-Language 请求头或 lang 查询参数切换语言。
package i18n

import (
	"strings"

	contribi18n "github.com/gofiber/contrib/v3/i18n"
	"github.com/gofiber/fiber/v3"
	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

// New 创建并返回一个配置好的 i18n 翻译器。
// rootPath 指向包含翻译 YAML 文件的目录。
func New(rootPath string) *contribi18n.I18n {
	return contribi18n.New(&contribi18n.Config{
		RootPath:        rootPath,
		AcceptLanguages: []language.Tag{language.Chinese, language.English},
		DefaultLanguage: language.Chinese,
		LangHandler:     langHandler,
	})
}

// langHandler 自定义语言处理器，将请求中的语言代码转为标准格式。
// 例如 "en-US" → "en", "zh-CN" → "zh"，确保与翻译文件名匹配。
func langHandler(c fiber.Ctx, defaultLang string) string {
	if c == nil || c.Request() == nil {
		return defaultLang
	}

	// 优先使用查询参数 ?lang=en 或 ?lang=zh-CN
	lang := c.Query("lang")
	if lang == "" {
		lang = c.Get("Accept-Language")
	}
	if lang == "" {
		return defaultLang
	}

	// 只取语言标签的第一部分（如 "en-US,en;q=0.9" → "en-US"）
	parts := strings.Split(lang, ",")
	code := strings.TrimSpace(parts[0])

	// 将完整区域标签（如 zh-CN, en-US）转为标准语言代码
	if strings.HasPrefix(code, "zh") {
		return "zh"
	}
	if strings.HasPrefix(code, "en") {
		return "en"
	}

	return defaultLang
}

// Localize 是 Localize 的便捷包装，返回本地化字符串。
// params 可以是消息 ID 字符串或 *goi18n.LocalizeConfig。
func Localize(translator *contribi18n.I18n, c fiber.Ctx, params interface{}) (string, error) {
	return translator.Localize(c, params)
}

// MustLocalize 是 MustLocalize 的便捷包装，失败时返回空字符串。
func MustLocalize(translator *contribi18n.I18n, c fiber.Ctx, params interface{}) string {
	s, err := translator.Localize(c, params)
	if err != nil {
		return ""
	}
	return s
}

// T 是 MustLocalize 的短别名，用于模板和 handler 中快速获取翻译。
func T(translator *contribi18n.I18n, c fiber.Ctx, messageID string, templateData ...map[string]string) string {
	config := &goi18n.LocalizeConfig{
		MessageID: messageID,
	}
	if len(templateData) > 0 && templateData[0] != nil {
		config.TemplateData = templateData[0]
	}
	return MustLocalize(translator, c, config)
}
