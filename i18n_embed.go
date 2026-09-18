package meridian

import "embed"

// I18nLocales 以嵌入方式打包内部 i18n 消息 YAML 文件，
// 使生产二进制在启动时不依赖源码检出目录。
//
//go:embed internal/i18n/locales
var I18nLocales embed.FS
