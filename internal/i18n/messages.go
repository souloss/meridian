package i18n

import (
	"io/fs"
	"sync"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"go.yaml.in/yaml/v3"
	"golang.org/x/text/language"
)

// Bundle 封装 go-i18n 的消息包与本地化器工厂，供整个进程生命周期复用。
// 它只暴露两个稳定入口：Load 与 T，调用方无需接触 go-i18n 内部类型。
type Bundle struct {
	bundle     *i18n.Bundle
	localesFS  fs.FS
	localesDir string

	localizerMu sync.Mutex
	// localizers 缓存按语言创建的本地化器。go-i18n 的 NewLocalizer 每次都会
	// 解析语言标签并触发 bundle 标签匹配，缓存后 T 热路径免去重复构造。
	localizers map[string]*i18n.Localizer
}

// New 构造一个缺省语言为简体中文的消息包。localesDir 是消息 YAML 文件
// 所在的目录（形如 "internal/i18n/locales"）。
func New(localesFS fs.FS, localesDir string) *Bundle {
	bundle := i18n.NewBundle(language.Make(LangZHCN))
	bundle.RegisterUnmarshalFunc("yaml", yaml.Unmarshal)
	return &Bundle{bundle: bundle, localesFS: localesFS, localesDir: localesDir, localizers: make(map[string]*i18n.Localizer)}
}

// Load 递归加载 localesDir 下的全部 YAML 消息文件（文件名约定为
// "<lang>.yaml"，如 zh-CN.yaml / en.yaml），返回加载失败的文件错误。
func (messages *Bundle) Load() error {
	entries, err := fs.ReadDir(messages.localesFS, messages.localesDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) <= len(".yaml") || name[len(name)-len(".yaml"):] != ".yaml" {
			continue
		}
		path := messages.localesDir + "/" + name
		if _, err := messages.bundle.LoadMessageFileFS(messages.localesFS, path); err != nil {
			return err
		}
	}
	return nil
}

// T 按语言标签渲染指定消息，data 提供模板变量，pluralCount 决定单复数。
// 消息缺失时回落到缺省语言，再缺失时返回消息 ID 本身。
func (messages *Bundle) T(lang, messageID string, data map[string]any, pluralCount any) string {
	localizer := messages.localizer(lang)
	config := &i18n.LocalizeConfig{MessageID: messageID}
	if data != nil {
		config.TemplateData = data
	}
	if pluralCount != nil {
		config.PluralCount = pluralCount
	}
	text, err := localizer.Localize(config)
	if err != nil {
		return messageID
	}
	return text
}

// localizer 返回按语言缓存的本地化器，缺省回落 zh-CN。
func (messages *Bundle) localizer(lang string) *i18n.Localizer {
	messages.localizerMu.Lock()
	defer messages.localizerMu.Unlock()
	if localizer, ok := messages.localizers[lang]; ok {
		return localizer
	}
	localizer := i18n.NewLocalizer(messages.bundle, lang, LangZHCN)
	messages.localizers[lang] = localizer
	return localizer
}
