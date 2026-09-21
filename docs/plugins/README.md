# Meridian 插件运行模型

Meridian 的插件模型分为两层：统一的运行时边界，以及按领域定义的能力协议。

## 运行时边界

`internal/plugin` 定义了与传输无关的 `Manifest`、`Factory`、`Endpoint`、`Call` 和 `Result`。

- `Manifest` 声明插件 ID、版本、协议版本、能力和依赖。
- `Factory.Open` 返回一个端点；端点只接收字节请求封套并返回字节响应封套。
- `plugin.Host` 负责安装、依赖检查、能力冲突检查、调用路由和逆序关闭。
- `InstallWithRegistration` 返回安装句柄；句柄是带代际校验的 disposer，旧句柄不能误删同 ID 的新安装。
- `Endpoint` 是 Host 路由代理而不是底层对象。插件移除或 Host 关闭后，长期持有的 endpoint 会返回稳定错误，不会调用已关闭的本地对象。
- 移除会先撤销新调用、等待活跃调用排空；普通移除在 context 超时后回滚为 active，Host 关闭则保留 closing 状态并返回诊断错误。
- `Snapshots` 提供状态、活跃调用数和最后关闭错误；`Subscribe` 提供有界的安装、关闭、移除和失败事件流。
- 插件不能从接口获得数据库连接、Go service container 或隐式全局状态。

当前内置插件使用 `InProcess` 实现，但接口本身不依赖进程边界。未来的 RPC/process runtime 只需要实现同一个 `Factory`/`Endpoint`，不需要修改 Kind、AI 或 Host 调用方。

组合层提供 `Runtime.RegisterKind`、`Runtime.RegisterAIProvider` 和 `Runtime.RegisterAIEndpoint`，因此外部 endpoint 可以直接绑定到现有 registry；不需要为 RPC 另造一套 Kind/AI API。

## 能力协议

Kind 与 AI 共享运行时模型，但保留不同的领域协议：

- Kind：`Validate`、`Normalize`、`Extract`、`Diff`、`CompileOverlay`。
- AI：`Generate`，包含 Kind、名称、引用、配置、内容、完成清单和稳定阶段/错误码。

`internal/kinds.EndpointPlugin` 和 `internal/ai.EndpointProvider` 是字节端点到领域强类型接口的适配器，因此远程插件可以作为本地代理被业务代码使用。

## 当前组合

`internal/plugins.Runtime` 在一个 Meridian 进程中持有统一 Host，以及 Kind/AI registry。OpenAPI、dbschema 和 dependency 已经通过 Host 安装，再以 endpoint 绑定到 Kind registry；pipeline、layer merge/index 和 push validation 通过 endpoint 调用 Kind 能力，而不是直接调用解析函数。

AI 工作流已经通过 `ai.Provider` registry 调用内置 command producer。持久化、租户权限、审核、幂等和 River 任务仍由 Host/application service 负责；provider 只拥有生成边界。

## 外部运行时约束

外部插件协议应保持以下语义：

1. 请求携带 capability、method、content type、payload 和显式 metadata。
2. 调用遵守 context cancellation/deadline。
3. 版本、依赖和 capability 冲突在安装时检查。
4. Endpoint 关闭必须幂等或至少可安全重复调用。
5. 任何跨进程协议都不得把 Go 内存对象、数据库句柄或 Host 内部类型作为 wire contract。
