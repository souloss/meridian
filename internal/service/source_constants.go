package service

// 本文件集中放置源配置（sourceSpec）域的领域常量。
// 值来源：contracts/domain.yaml 的 sourceMode / layerOrigin / sourceCompatibility 段。

// 源配置模式（sourceMode 枚举）。
const (
	// sourceModeBuiltin 是内置从仓库路径扫描源文件的模式。
	sourceModeBuiltin = "builtin"
	// sourceModeCommand 是通过生产者命令采集的模式。
	sourceModeCommand = "command"
	// sourceModePush 是第三方推送内容的模式。
	sourceModePush = "push"
	// sourceModeManual 是手动绑定目标资产的模式。
	sourceModeManual = "manual"
	// 注意：sourceModeAI（"ai"）已在 ai_constants.go 中定义为 aiMode，复用即可。
)

// 层来源（layerOrigin 枚举）。
const (
	// layerOriginRepo 表示来自仓库的内容。
	layerOriginRepo = "repo"
	// layerOriginThirdParty 表示来自第三方推送的内容。
	layerOriginThirdParty = "third_party"
	// 注意：layerOriginAIGenerated（"ai_generated"）已在 ai_constants.go 中定义为 aiOrigin；
	// layerOriginManual（"manual"）已在 layer_constants.go 中定义，复用即可。
)

// 源配置来源（configOrigin：API 显式配置或仓库配置文件导入）。
const (
	// sourceConfigOriginAPI 表示通过 API 显式创建的源配置。
	sourceConfigOriginAPI = "api"
	// sourceConfigOriginRepository 表示由仓库配置文件导入的源配置。
	sourceConfigOriginRepository = "repository"
)

// sourceBranchPatternAll 是匹配全部分支的通配模式。
const sourceBranchPatternAll = "**"
