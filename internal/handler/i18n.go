package handler

import (
	"sync"

	meridian "github.com/meridian-labs/meridian"
	"github.com/meridian-labs/meridian/internal/i18n"
)

// defaultI18n 是进程生命周期的缺省消息包，由首次使用时从嵌入文件加载。
// 消息文件内容固定，因此加载一次后对并发的 T 调用是只读且线程安全的。
var (
	defaultI18n     *i18n.Bundle
	defaultI18nOnce sync.Once
)

// i18nBundle 惰性加载嵌入的 locale 消息文件并返回进程级消息包。
// 加载失败时返回 nil，调用方回落到错误码本身作为消息。
func i18nBundle() *i18n.Bundle {
	defaultI18nOnce.Do(func() {
		bundle := i18n.New(meridian.I18nLocales, "internal/i18n/locales")
		if err := bundle.Load(); err != nil {
			return
		}
		defaultI18n = bundle
	})
	return defaultI18n
}

// localize 按请求语言渲染错误码对应的本地化消息；模板变量通过 data 传入。
// 语言协商失败或消息缺失时回落到错误码本身。
func localize(code string, lang string, data map[string]any) string {
	bundle := i18nBundle()
	if bundle == nil {
		return code
	}
	return bundle.T(lang, errorMsgID(code), data, nil)
}

// errorMsgID 返回错误码对应的 i18n 消息键（统一前缀 error.）。
func errorMsgID(code string) string {
	return "error." + code
}
