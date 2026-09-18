// Package i18n 提供面向 API 响应错误消息的本地化支持。
// 它封装 go-i18n/v2 的消息加载与查找，从 YAML 消息文件按请求语言渲染
// 带变量与单复数形式的错误消息，缺省语言为简体中文（与前端一致）。
package i18n

import (
	"strconv"
	"strings"
)

const (
	// LangZHCN 是简体中文语言标签，同时是缺省语言。
	LangZHCN = "zh-CN"
	// LangEN 是英语语言标签。
	LangEN = "en"
	// HeaderAcceptLanguage 是 HTTP 请求携带语言偏好的标准头。
	HeaderAcceptLanguage = "Accept-Language"
	// headerLangSeparator 分隔 Accept-Language 头中的多个语言范围。
	headerLangSeparator = ","
	// headerQSeparator 分隔语言范围与其质量因子（q 值）。
	headerQSeparator = ";"
)

// Negotiate 解析 Accept-Language 头并返回白名单内优先级最高的语言标签。
// 白名单仅含 zh-CN 与 en；缺省或无法匹配时回落到 zh-CN。
func Negotiate(acceptLanguage string) string {
	bestLang := LangZHCN
	bestQuality := -1.0
	for _, rawRange := range strings.Split(acceptLanguage, headerLangSeparator) {
		part := strings.TrimSpace(rawRange)
		if part == "" {
			continue
		}
		lang := part
		quality := 1.0
		if index := strings.Index(part, headerQSeparator); index >= 0 {
			lang = strings.TrimSpace(part[:index])
			for _, param := range strings.Split(part[index+1:], headerQSeparator) {
				param = strings.TrimSpace(param)
				if strings.HasPrefix(param, "q=") {
					if parsed, err := strconv.ParseFloat(strings.TrimPrefix(param, "q="), 64); err == nil {
						quality = parsed
					}
				}
			}
		}
		if !supported(lang) {
			continue
		}
		if quality > bestQuality {
			bestQuality = quality
			bestLang = canonical(lang)
		}
	}
	return bestLang
}

// supported 判断语言范围是否落在白名单内（忽略区域大小写）。
func supported(lang string) bool {
	switch canonical(lang) {
	case LangZHCN, LangEN:
		return true
	default:
		return false
	}
}

// canonical 归一化语言标签为白名单内的规范写法。
func canonical(lang string) string {
	switch {
	case strings.HasPrefix(lang, "zh"):
		return LangZHCN
	case strings.HasPrefix(lang, "en"):
		return LangEN
	default:
		return lang
	}
}
