# 图片生成页动态模型选择与 API Key 记忆设计

| 项目 | 内容 |
| --- | --- |
| 日期 | 2026-11-02 |
| 状态 | 代码修复完成并经提交前复核（第 13 节），待 Codex 独立审核与浏览器验收；正式站点密码自动填充未验证 |
| 工作分支 | `feature/hy/10227_image_gen_upgrade` |
| 页面 | `/image-gen` |
| 目标 | 根据当前 API Key 发现并选择图片模型，减少重复输入 Key 和重复选模型；不再固定模型版本 |

## 1. 背景与现状

用户反馈接口已升级到 2.5，而页面仍固定使用 `gpt-image-2`。后端没有单独的 `gpt-image-2.5`：2.5 系列是 `gpt-image-2.5-flare`、`gpt-image-2.5-sunburst` 及其 `-2026-09-08` 快照。设计不把任何版本写死为默认值，以当前 Key 的模型目录和真实生成结果为依据。

| 源码位置 | 已核对的现状 |
| --- | --- |
| `frontend/src/stores/imageGen.ts` | `DEFAULT_MODEL = 'gpt-image-2'` 同时用于文生图 JSON、图改图 multipart 和生成历史；API Key 仅在 Pinia 内存中；`extractErrorMessage` 已兼容 `{ error: { message } }` 与 `{ message }` 两种错误体 |
| `frontend/src/views/ImageGenView.vue` | 徽标写死 `gpt-image-2`；Key 输入使用 `v-model.trim`，位于 `<form @submit.prevent="generate">` 内（表单无 submit 按钮且有多个文本类输入，按 HTML 规范回车不会隐式提交）；提示“页面不会保存 API Key” |
| `frontend/src/router/index.ts` | 公开路由，且在 `BACKEND_MODE_ALLOWED_PATHS` 中，不能依赖必须登录的用户 API |
| `backend/internal/server/routes/gateway.go` | `GET /v1/models` 走 API Key 鉴权，带 `client_version` 时转 Codex manifest；`/v1/images/*` 按分组平台分发，OpenAI 与 Grok 支持，其他平台返回 404，Composite 按解析出的目标平台分发 |
| `backend/internal/handler/gateway_handler.go` | `Models` 按分组平台、账号 model_mapping、分组白名单生成目录，无映射时回落默认目录；开启 Codex manifest 的 OpenAI 分组走 pinned 上游目录；普通目录有服务端缓存（`gateway.models_list_cache_ttl_seconds`，默认 15 秒） |
| `backend/internal/pkg/openai/constants.go` | OpenAI 默认目录含 `gpt-image-1`、`gpt-image-1.5`、`gpt-image-2`、`gpt-image-2.5-flare`、`gpt-image-2.5-sunburst` |
| `backend/internal/service/openai_images.go` | `validateCompatibleImagesModel` 是图片端点接受的模型名判定：`gpt-image-*`、`grok-imagine`、`grok-imagine-edit`、`grok-imagine-image*`、`gemini-*-image` 与 `gemini-*-image-*`，前缀判断不区分大小写 |
| `backend/internal/server/middleware/api_key_auth.go` | `/v1/models` 与生图一样经过计费前置检查：Key 过期、余额不足、无订阅、IP 拒绝返回 403；Key 额度耗尽、订阅限额返回 429。无效 Key 计入按 IP 的无效鉴权限流（默认 60 秒 120 次，封禁 60 秒，返回 429 `INVALID_AUTH_RATE_LIMITED`） |

模型列表是网关发布的候选目录，不是逐模型探活或图片能力证明。列表有模型不代表余额、额度、上游健康状态或图改图能力一定满足；非 OpenAI/Grok 分组的目录也可能出现图片模型名，但其 `/v1/images/*` 返回 404。没有模型也不能直接等同于 Key 无效。

## 2. 范围与边界

| 本次包含 | 本次不包含 |
| --- | --- |
| 当前 Key 的模型发现、按后端图片模型判定筛选、选择与手动输入 | 自动创建 API Key、登录态代替网关 Key |
| 显式选择在本浏览器记住 Key；记住上次使用的模型（不含 Key） | 服务端保存 Key、多设备同步、多 Key 管理 |
| 文生图、图改图及历史使用实际选择的模型 | 修改上游模型映射、计费、鉴权、调度或模型目录生成算法 |
| 并发响应隔离、存储故障降级、错误码映射、针对性验收 | 通过真实生图请求自动探测能力或自动换模型重试 |

模型目录的实际覆盖范围与各模型参数兼容性留到实施联调验证，不以文档推断代替结果。

## 3. 页面交互

表单顺序：API Key → 记住选项 → 模型选择 → 生成方式 → 提示词/原图 → 尺寸和数量 → 生成按钮。继续使用现有页面布局和组件风格。

| 控件/状态 | 设计行为 |
| --- | --- |
| API Key | 保持密码输入及显隐切换，沿用 `v-model.trim` |
| 加载模型（发送 Key） | 只有两种显式操作把当前 Key 发往目录端点：Key 框内回车（忽略按住连发与输入法组合）和“加载模型/刷新模型”按钮，两者总是请求，同一 Key 的在途请求去重。输入、粘贴、失焦、勾选记住都不发请求。唯一例外是打开页面时恢复显式记住的 Key 并加载一次 |
| 回车 | Key 输入框内回车只加载模型，手动模型输入框内回车只确认模型，两者都显式阻止默认行为，防止日后加入 submit 按钮或调整输入后误发付费生图 |
| 加载/刷新按钮 | 当前 Key 尚无目录时显示“加载模型”，已有目录（含空目录）时显示“刷新模型”，加载中禁用；不清除提示词、原图、结果或历史 |
| 模型下拉 | 显示 API 返回的完整模型 ID；候选筛选见 4.2 |
| 自定义模型 | 下拉外提供明确的“手动输入模型”入口，适用于别名及列表遗漏；不限前缀，提示“是否支持图片生成以接口结果为准”。去首尾空白后须非空且不含控制字符；从下拉选择即退出手动模式 |
| 默认选择 | 规则见 4.3；不按版本号或价格自动挑选 |
| 版本徽标 | 显示当前模型及状态（目录已确认 / 未确认 / 手动）；无选择时显示“未选择模型” |
| 生成按钮 | 沿用点击时校验并提示缺失项（Key、模型、提示词、图改图原图）；不要求先加载模型，直接使用输入框中的当前 Key（与改动前一致），但不保存、不替换已保存的 Key；目录加载中或失败不阻止使用当前（未确认）模型；正在生成时继续阻止重复提交 |
| 恢复历史 | `restoreHistory` 不修改当前模型（历史模型可能不在当前 Key 目录）；历史条目继续展示原模型 |

逐字输入不触发请求，是为了避免中间态 Key 产生 401 并计入按 IP 的无效鉴权限流；办公网络共享出口 IP 时，多人同时输入会互相影响。

粘贴、失焦也不触发请求：密码管理器可能把本站登录密码填入密码型 Key 框并触发 input/change，`autocomplete="off"`、`new-password` 与 `event.isTrusted` 都不能可靠区分自动填充与用户输入，因此不以这些事件作为发送 Key 或改动已保存 Key 的依据。Chrome 是否实际填充需浏览器验证。

模型列表不展示“已验证可用”标签，不根据名称推断尺寸、图改图能力和价格。切换模型后沿用现有尺寸与数量输入，参数不兼容由生成接口明确报错；不自动降级模型或参数。

## 4. 模型发现合同

### 4.1 请求与端点

```http
GET /v1/models
Authorization: Bearer <当前输入的 API Key>
Accept: application/json
```

- 使用独立 `fetch`，不使用注入登录 JWT 的管理 API client，不添加 `client_version`。
- 默认同源路径 `/v1/models`，Key 只放 Authorization 请求头，不进入 URL、日志、历史或 toast；服务端错误文案若回显 Key，展示前按请求 Key 快照脱敏（见第 7 节）。
- `cache: 'no-store'`，10 秒超时，可用 `AbortController` 取消；失败不后台循环重试。
- 新增可选构建变量 `VITE_IMAGE_MODELS_ENDPOINT`，默认 `/v1/models`。保留现有 `VITE_IMAGE_GENERATION_ENDPOINT`、`VITE_IMAGE_EDIT_ENDPOINT`。
- 三个端点解析后的 origin 必须一致，否则停用自动发现、提示部署配置问题，保留手动模型。典型场景：生图端点配置为外部网关但未配置模型端点，默认同源 `/v1/models` 会把外部 Key 发给本站后端。不通过猜测路径派生第三方 URL，不跟随重定向（`redirect: 'error'`）。
- 发现请求只发往生图请求本来就会发送 Key 的 origin，不新增 Key 的发送目标；外部服务的 HTTPS 与 CORS 由部署者负责（现行 CSP `connect-src` 已放行 `https:`）。
- 普通目录有服务端缓存，默认 15 秒，以 `gateway.models_list_cache_ttl_seconds` 配置为准（允许 10–30 秒）；刷新提示不承诺立即反映后台配置变更。
- 发现请求与其他网关请求一样会刷新 Key 的最近使用时间（`TouchLastUsed`），这是已知副作用，不另做规避。

### 4.2 解析与筛选

| 规则 | 约定 |
| --- | --- |
| 响应形状 | 接受 `{ data: [{ id: string }] }`；顶层缺少数组属于响应异常，合法空数组属于空目录 |
| 条目清洗 | 忽略非对象、非字符串或空 ID，按完整 ID 去重；拒绝含首尾空白或控制字符的 ID，不悄悄修改服务器模型标识 |
| 候选筛选 | 纯函数镜像后端 `validateCompatibleImagesModel`：小写后匹配 `gpt-image-` 前缀、`grok-imagine`、`grok-imagine-edit`、`grok-imagine-image` 前缀，以及 `gemini-` 前缀且以 `-image` 结尾或含 `-image-`。小写只用于判断，展示和提交使用原始 ID。函数旁注明后端来源，后端判定变化时同步。Composite 分组由后端按解析后的上游模型校验，目录中的公开别名可能不符合该判定而被隐藏，需手动输入。仅为候选筛选，不扩大后端权限 |
| 顺序 | 按 ID 确定性自然排序，数字段按数值比较；排序仅为展示，不承担版本推荐 |

### 4.3 选择与恢复

| 场景 | 约定 |
| --- | --- |
| 目录成功返回 | 手动来源保持手动，不受目录影响。列表来源的当前选择在目录中则标记已确认；不在目录中则清除并提示“原模型 X 不在当前 Key 的目录，请重新选择”，本次不自动改选其他模型。没有当前选择时，依次取：在目录中的模型偏好 → 唯一候选 → 提示“请选择模型” |
| 目录请求失败 | 任何失败都不清除当前选择，标记为“未确认”，允许提交，以生成接口结果为准。401 清空候选；其他失败在同 Key 时保留旧候选并标记“刷新失败，当前为旧目录” |
| 上次使用的模型 | 读取 `image-gen-model-v1` 作为未确认的初始选择，按上两行规则确认或清除；手动来源恢复为手动模式，不伪装成目录返回值 |
| Key 变化 | 立即清空旧候选、旧错误并取消旧请求；当前选择保留为未确认，由新 Key 的目录确认或清除 |

## 5. 记住模型与 API Key

### 5.1 模型偏好（始终保存，无开关，不含 Key）

- 存储键 `image-gen-model-v1`，内容仅 `version`、`model`、`modelSource`（`list`/`manual`）。
- 用户显式选择、确认手动模型以及生图请求实际发出时写入；本地校验未通过的提交不写入；目录清除选择时不改写偏好。只保留最后一个。
- 读写失败静默降级（与生成历史一致）；损坏或未知版本时忽略并尽力删除。

### 5.2 记住 API Key（默认关闭）

勾选项“在此浏览器记住 API Key”，说明：“仅保存在当前浏览器，其他使用此浏览器的人可能访问；公共设备请勿勾选。”原“页面不会保存 API Key”文案改为随状态准确说明。界面区分“已保存”“待保存”和“浏览器中保存的是之前的 Key”（编辑后、新 Key 加载成功前）。

| 行为 | 持久化与内存处理 |
| --- | --- |
| 勾选记住 | 空 Key 不写入。当前 Key 最近一次获得结构合法的成功目录响应（其后无失败、未改 Key）则立即写入；否则进入“待保存”，不发请求，待用户回车或点击“加载模型”获得成功目录后写入。自动发现停用时勾选即写入 |
| 刷新/重新打开 | 读取并恢复 Key，立即发起目录请求；不使用旧目录 |
| 修改 Key | 指用户编辑及密码管理器填入等输入事件；恢复、清除等程序赋值不触发本行。不发请求，不删除已保存的旧 Key，勾选状态保持。用户显式加载后，新 Key 获得结构合法的成功目录响应时覆盖旧 Key；401 提示“Key 无效，未保存”，旧 Key 不变；其他失败保持待替换，重试成功后覆盖。改回已保存的值即恢复“已保存”。自动发现停用时，新 Key 在回车时覆盖 |
| 取消记住 | 立即删除已保存 Key；当前内存 Key 和表单保留，刷新后不恢复 |
| 清除已保存配置 | 删除已保存 Key 与模型偏好，清空当前 Key、模型、候选和发现错误，取消发现请求；提示词、图片结果及历史不删除 |
| 存储不可用或超额 | 明确提示“无法保存，当前仅本次页面有效”，回到未记住状态，仍可手动生图；不得显示已保存。覆盖写入失败时同时删除旧 Key，退出记住状态后不留旧值 |
| 删除存储失败 | 内存仍清空；显示“未能清除浏览器中保存的 API Key，可能仍有残留”和“重试清除”，“清除已保存配置”入口保持可见；编辑 Key、勾选/取消记住、清空输入都不解除。删除成功、新 Key 覆盖写入成功，或其他标签页删除/覆盖该记录后才解除；不能宣称已删除 |
| 配置损坏/未知版本 | 不恢复 Key，不自动请求；尽力删除无效配置，删除失败按上述规则提示 |

存储键 `image-gen-key-v1`，内容仅 `version`、`apiKey`。只保留最后一个 Key，不以明文 Key 作为存储键，不写入 `image-gen-history-v1`。Key 与模型偏好分开存储，选择模型不会触发 Key 存储的跨标签页事件。

localStorage 不是加密保险箱，同源脚本（含 CSP 放行的第三方脚本）可读取该 Key；不采用前端硬编码密钥“加密”。页面是独立 API Key 工具，记住配置属于此浏览器，不绑定站内登录账号；站内退出登录不自动删除显式记住的工具配置，界面应说明并提供清除入口。跨标签页只监听 `image-gen-key-v1`（含整体清空存储时 `key` 为 null 的事件）：其他标签页修改或删除时，本页退出记住状态并提示，解除本页的删除失败状态，不自动导入新 Key，防止旧标签页把已清除的 Key 写回；若此时存储中仍是本页写入的值（本页迟到的写入先于对方事件），一并删除；模型偏好的跨页变化不影响当前选择。

## 6. 状态、并发与生成快照

| 状态 | 表现 |
| --- | --- |
| `idle` | 无 Key 或尚未加载（等待回车或点击“加载模型”）；不显示上一个 Key 的候选 |
| `loading` | 显示加载状态；不阻止使用当前选择 |
| `ready` | 展示候选与选择；同 Key 刷新时保留现有候选并提示正在刷新 |
| `empty` | “当前 Key 的目录中没有图片模型”，提供重试和手动入口 |
| `error` | 显示第 7 节的错误信息；当前选择保留为未确认；提供重试和手动入口 |
| `disabled` | 端点 origin 不一致，自动发现停用，仅手动模型 |

- Store 管理发现状态、当前模型和持久偏好；候选判定、目录解析和存储读写抽成纯函数，避免 store 膨胀。视图卸载不清空正在生成的任务，保持现有切页行为。
- 每次 Key 变化或发现请求替换，增加请求序号并取消旧请求。成功、失败、超时及 finally 均检查序号与 Key 快照，取消不能只依赖网络层；由目录结果引发的确认/清除选择和“成功目录后写入 Key”同样受此检查，写入前还要读取最新勾选状态，迟到响应不得把旧 Key 或已被其他标签页清除的 Key 写入存储。
- Key 相关状态分开维护：当前 Key（输入框，加载与生成都使用）、已保存 Key（store 内部 `savedKey`，对外只暴露 `keySaved` / `keySavedIsCurrent`）、删除失败状态 `keyRemovalFailed`。
- 开始生成前快照 `key/model/mode/prompt/size/count/files`；文生图 JSON、图改图 FormData、成功历史均使用同一 `requestModel`。
- 生成过程中切换 Key 或模型不修改已发出请求，也不自动重发。当前任务区域标注本次请求模型；完成后历史记录原模型，避免与新表单混淆。
- 清除配置不取消已经提交的生图任务，不承诺撤销计费；保留现有任务完成提示。

## 7. 错误处理

错误信息复用 `extractErrorMessage`：依次取 `error.message`、`message`，缺失时用状态码兜底；只展示 message 字段，不展示原始响应正文。message 字段本身不是脱敏边界：目录与生图两条路径在写入页面和 toast 前，都用请求发起时的 Key 快照把回显的 Key 替换为 `***`，不能用当前输入值（请求发出后可能已改 Key）。本站鉴权中间件当前返回固定文案，此规则用于防范外部网关或上游回显。鉴权中间件的错误体为 `{ code, message }`，需要区分处理的场景按 `code` 识别。`/v1/models` 的 403/429 与生图请求经过同一套检查，不能解释为“只是不能查看目录”。

| 场景 | 典型 code | 用户提示/处理 |
| --- | --- | --- |
| 401 | `INVALID_API_KEY`、`API_KEY_DISABLED`、`USER_NOT_FOUND`、`USER_INACTIVE` | 显示服务端信息；清空候选；不删除已保存的 Key，允许编辑或主动清除 |
| 403 | `API_KEY_EXPIRED`、`INSUFFICIENT_BALANCE`、`SUBSCRIPTION_NOT_FOUND`、`SUBSCRIPTION_INVALID`、`ACCESS_DENIED`、`GROUP_DELETED`、`GROUP_DISABLED`、`GROUP_NOT_ALLOWED` | 显示服务端信息；这些情况生图同样会被拒绝 |
| 429 | `API_KEY_QUOTA_EXHAUSTED`、`USAGE_LIMIT_EXCEEDED`、`INVALID_AUTH_RATE_LIMITED` | 显示服务端信息，不自动重试；`INVALID_AUTH_RATE_LIMITED` 时按 `Retry-After` 提示等待秒数（跨域读不到时只提示稍后重试） |
| 其他 4xx | 如端点配置错误导致 404/405 | 显示服务端信息或状态码，提示检查部署配置 |
| 超时、网络/CORS、5xx | 如 503 `API_KEY_AUTH_OVERLOADED` | “模型列表加载失败”并附状态码，支持重试或手动输入；不展示 5xx 正文 |
| 200 但非 JSON 或结构错误 | 如 SPA 回落返回 HTML | 模型列表响应格式异常；不能当作合法空列表 |
| 生图失败 | — | 保留用户输入，显示服务端信息（如不支持 Images API 的平台返回 404），不自动换模型重试；继续走原有服务端鉴权、审核和计费 |

## 8. 文件影响与实施顺序

| 顺序 | 文件/职责 |
| --- | --- |
| 1 | 新增纯函数模块（如 `frontend/src/utils/imageGenModels.ts`）：候选判定、目录解析与排序、偏好与 Key 存储读写 |
| 2 | `frontend/src/stores/imageGen.ts`：发现状态、触发与去重、并发隔离、选择规则、模型快照 |
| 3 | `frontend/src/views/ImageGenView.vue`：模型下拉/手动输入、徽标状态、记住与清除交互、触发点绑定 |
| 4 | `frontend/src/views/__tests__/ImageGenView.spec.ts` 与纯函数测试：更新旧固定模型及不持久化断言 |
| 5 | `llm-wiki/wiki/ops.md`：记录 `VITE_IMAGE_MODELS_ENDPOINT` 与 origin 约束（仓库无前端环境变量示例文件，不新建） |
| 6 | `llm-wiki/wiki/frontend.md`：把 `imageGen.ts` “Key 禁止持久化”的现行合同改为“用户显式选择保存”，记录选择规则与并发约束；`security-and-reliability.md`：新增 Key 存于 localStorage 的风险边界；`backend.md` 图片段：注明前端候选判定镜像 `validateCompatibleImagesModel`，后端修改时须同步 |

预计不修改后端。若真实目录遗漏目标模型，先明确账号映射、分组白名单与 pinned 目录原因，单独记录，不能为页面便利扩大后端模型权限。

## 9. 验收矩阵

| 范围 | 必须覆盖 |
| --- | --- |
| 候选解析 | `gpt-image-1/1.5/2/2.5-flare/2.5-sunburst` 及日期快照、大小写变体、`grok-imagine`、`grok-imagine-edit`、`grok-imagine-image*`、`gemini-*-image` 入选；`grok-imagine-video*`、`gpt-imagex`、文本模型排除；重复项、非法 ID、空目录及异常响应结构 |
| 选择策略 | 当前选择保留/移除（移除时不自动改选）、无选择时偏好命中/未命中、单候选、多候选、手动别名及非法手动输入、目录失败保留未确认选择、`restoreHistory` 不改模型 |
| 触发 | 输入、change、粘贴、失焦、勾选记住均不请求；回车/加载按钮总是请求（忽略按住连发）；恢复已保存 Key 请求一次；同 Key 去重 |
| 回车 | Key 框与手动模型框回车不提交表单、不发生图请求 |
| 竞态 | A Key 慢响应晚于 B；旧失败/旧 finally 不覆盖新状态；清空 Key、快速刷新、超时取消 |
| 凭据边界 | 请求不含登录 JWT/client_version；Key 不进 URL/错误/历史；服务端错误回显 Key 时页面与 toast 按请求快照脱敏（两种错误结构）；origin 不一致时停用发现；重定向不发送到新目标 |
| 持久化 | 默认不保存 Key；勾选后已验证则写入，否则待保存且不请求；改 Key 不删旧值，显式加载成功后覆盖，401 不覆盖；覆盖写入失败删除旧值；删除失败保留警告与重试入口直至删除或覆盖成功；恢复已保存 Key 不改存储；迟到响应与跨标签页清除后不写回；发现停用时按回车写入；取消记住；清除同时删 Key 与模型偏好；模型偏好独立读写且校验失败不写；损坏配置；跨标签页只响应 Key 存储变化 |
| 错误映射 | 401/403/429 各 code 显示服务端信息；`INVALID_AUTH_RATE_LIMITED` 按 code 识别；其他 4xx；200 返回 HTML；5xx/网络使用通用文案 |
| 生成一致性 | 两种请求都携带选中模型；提交后改模型/Key，历史仍记录请求快照；不自动重试付费请求 |
| 既有能力 | 多图上传限制、提示词、尺寸、数量、结果展示、历史、路由切换任务继续执行不回归 |
| 浏览器 | 独立访问及登录布局，桌面/窄屏，刷新恢复与错误状态；Chrome 密码管理器自动填充专项（正式同源、具备已保存登录项、请求隔离）；先 iab，再已连接本地 Chrome，不启动隔离/无头实例 |

实施阶段在仓库根目录执行：

```powershell
pnpm --dir frontend run typecheck
pnpm --dir frontend exec vitest run src/utils/__tests__/imageGenModels.spec.ts src/stores/__tests__/imageGen.discovery.spec.ts src/views/__tests__/ImageGenView.spec.ts
pnpm --dir frontend run lint:check
```

浏览器 mock 必须隔离模型发现与生成请求，避免测试流量触发真实生图收费。真实 Key 的目录读取和真实付费生图验收分开记录；未验证的模型/模式不能报告为已支持。

## 10. 本轮交付与后续门槛

设计阶段只交付设计及 wiki 导航；实施结果见下方“实施状态”，实现复核见第 11 节，Codex 交叉复核见第 12 节，修复记录见第 13 节。

实施完成的门槛是自动化验证通过、受控浏览器交互验收完成，并明确记录实际目标模型 ID、目录来源及真实生图是否验证。不得把“列表出现模型”写成“该模型所有图片能力均已验证”。

### 实施状态（2026-11-02）

| 项目 | 结果 |
| --- | --- |
| 代码 | `frontend/src/utils/imageGenModels.ts`（新增）、`frontend/src/stores/imageGen.ts`、`frontend/src/views/ImageGenView.vue`；未改后端；未提交 |
| 自动化 | 最新结果见第 13 节 |
| wiki | `frontend.md`、`security-and-reliability.md`、`backend.md`、`ops.md` 已更新 |
| 未完成 | Codex 对第 13 节修复的独立审核；正式同源 Chrome 密码自动填充验证；第 9 节浏览器验收（iab / 已连接本地 Chrome）；真实 Key 目录读取与真实生图验证（含 Grok、Gemini 参数兼容性），未验证前不得报告为已支持 |

## 11. 实现复核结论（2026-11-02）

结论：**通过，无阻塞问题；复核发现的问题已修复并补测试。** 复核对象为本分支未提交的工作区改动。复核方式：由独立审查代理只拿到代码与本设计文档，不提供实现方结论；实现方逐条对照源码核实后修复，并自行重跑全部命令。审查代理与实现方同为 Claude，不等同于跨团队复核；如需 Codex 交叉复核，以本节为输入另行记录。

| 项 | 结果 |
| --- | --- |
| 本功能测试 | `imageGenModels.spec.ts` 10、`ImageGenView.spec.ts` 37、`stores/__tests__/imageGen.discovery.spec.ts` 3，共 50 项通过；连续重跑 3 次结果一致 |
| 变异检查 | 依次注入 5 个已修复缺陷（失败后仍视 Key 已验证、回车未拦截、恢复走用户编辑路径、读响应体超时误报、勾选框不同步），对应用例均失败；首轮未发现“恢复走用户编辑路径”，已补“恢复时目录失败仍保留 Key”用例 |
| 前端全量 | `vitest run` 423 个文件 / 3441 项通过；与 typecheck、lint 并行的一次运行出现 1 项失败，日志未保留、无法定位，单独重跑全量通过 |
| 静态检查 | `typecheck`、`lint:check` 通过 |

复核发现与处理：

| 级别 | 问题 | 处理 |
| --- | --- | --- |
| 应修 | 某 Key 曾返回 200、之后刷新得到 401，再勾选记住会直接保存这个已被拒绝的 Key | 改为“最近一次目录响应为 200”才视为已验证，任何失败或改 Key 都清除；5.2 措辞同步 |
| 应修 | 测试证明力不足：回车拦截、恢复不删 Key、请求头完整性、同步保存失败路径、超时、旧目录、401 清空候选等无有效断言 | 回车改为断言 `defaultPrevented`，请求头改为精确比较，补齐对应用例；端点不同源与超时在 store 层用重置模块与假计时器验证 |
| 次要 | 限流暂停后换 Key，提示仍说“失焦即可加载”，实际被静默忽略 | 暂停状态改为响应式，idle 提示显示已暂停及恢复方式 |
| 次要 | 超时发生在读取响应体期间时误报“格式异常” | 读完响应体后先判断超时 |
| 次要 | 按住回车连发会在每次响应后重复请求 | 忽略 `event.repeat` |
| 次要 | 删除旧 Key 失败的警告在下一次按键时消失 | 删除失败时保留警告，直至写入或删除成功 |
| 细节 | 偏好写入失败时仍显示“清除已保存配置” | 以实际写入结果判断；手动模型徽标改用蓝色，与“已确认”区分；模型 ID 同时拒绝 C1 控制字符；wiki 段落空行 |
| 设计 | Composite 别名、5.2“已有 200”歧义、3 节触发措辞、7 节 403/503 码表、10 节过期段落、跨标签页 `key=null` | 已在对应章节修订 |

未修复、转浏览器验收：密码型 Key 输入框使用 `autocomplete="off"`，Chrome 可能仍为同源站点自动填充登录密码。若发生，失焦会把密码作为 Bearer 发往 `/v1/models`，并且会被视为用户改 Key 而删除已保存的 Key。该问题无法在 jsdom 中确认，需在浏览器验收时检查；如确认，再评估改用 `autocomplete="new-password"` 等方案。

未覆盖：第 9 节浏览器交互验收；真实 Key 的目录读取与付费生图；Grok、Gemini 模型对当前尺寸、数量、`b64_json` 参数的兼容性；生产构建产物。

## 12. Codex 交叉复核结论（2026-11-02）

### 12.1 结论、范围与独立性

**不建议按“全部验收通过”收口：2 项应修已复现，2 项文档细节需澄清，Chrome 正式站点密码自动填充仍未确认。** 未发现已确认的永久卡死或越权绕过；已执行的类型检查、lint、专项和全量测试均通过，但这些结果不能覆盖浏览器密码管理器或真实模型能力。

| 项目 | 本轮证据与边界 |
| --- | --- |
| 审查对象 | `feature/hy/10227_image_gen_upgrade` 未提交工作区；HEAD 为 `70e90d7064bef67ba4feadd61b3fdeafd7efd0cd`，不是本功能提交 |
| 独立性 | 先读取第 1–10 节、3 个实现文件、3 个测试文件、wiki 差异及指定后端；形成独立发现并进行首轮浏览器复现后，才读取第 11 节 |
| 后端核对 | `openai_images.go` 的模型判定；`api_key_auth.go`、`ingress_reject.go` 与错误响应 helper；`gateway.go` 的 models/images 分派；`gateway_handler.go` 的普通、Composite、pinned 目录分支；目录缓存 TTL 配置 |
| 修改范围 | 本轮不修改代码或测试；仅追加本节、更新文档状态行与 wiki README 对应条目。第 11 节保留为历史结论 |
| 浏览器隔离 | 先 iab，随后用户已连接的 Chrome；临时本地 Vite 服务监听 `127.0.0.1:15287`，直接加载当前源码；关闭真实代理，`/api/*`、`/v1/*` 全部由 mock 接管，不接入数据库或上游 |
| 数据与权限 | 仅使用合成测试输入与 mock 登录；未读取真实 Key 目录、未发起真实生图、未读取或导出浏览器保存的密码；未启动隔离/无头浏览器 |

下列行号对应本轮工作区版本；后续修改后需重新定位。

### 12.2 按严重程度排序的发现

| 编号 | 级别 | 文件:行 | 一句话缺陷与具体复现场景 | 建议 |
| --- | --- | --- | --- | --- |
| R1 | 应修，机制已确认 | `frontend/src/stores/imageGen.ts:579`、`:839`、`:193`；本设计 `:143` | 服务端 `message` 未脱敏即展示，违反 Key 不进入错误文案/toast 的合同。输入合成测试 Key → mock `/v1/models` 返回 401，message 中包含该请求凭据 → 页面完整显示凭据；iab 已复现。生图失败同样经 `extractErrorMessage` 后进入 `errorMessage` 与 toast；其原样显示行为在基线已存在，本功能新增发现路径复用了它。没有证据表明当前生产中间件会回显 Key，不把条件性暴露写成生产泄漏事件。 | 使用请求快照 Key 做错误展示脱敏，并对发现/生图两条路径覆盖当前 Key 已改变后的错误；或对鉴权错误采用受控 code→文案映射。§7 的“只展示 message”不是脱敏边界，需同步澄清。 |
| R2 | 应修，已确认 | `frontend/src/stores/imageGen.ts:146`、`:337`、`:348`；`frontend/src/views/ImageGenView.vue:83` | 删除 Key 失败时把 `keySaved` 清为 false，使唯一直接清除入口消失，但敏感记录仍在。先只记住 Key、尚未选择模型 → 让该存储项的 `removeItem` 抛错而模型偏好删除成功 → 点清除 → 页面显示失败，实测存储仍有 Key、清除按钮已消失；恢复存储能力后没有直接重试按钮，刷新还会恢复旧 Key。Chrome 隔离页已复现。另 `setRememberKey` 的首行会清空失败提示，空 Key 再勾选时旧记录仍在。 | 将“可能残留待清除记录”单独计入清除入口显示条件，保留删除失败状态直到删除/覆盖成功，提供重试。此项可通过刷新或再次切换记住状态恢复，不声称永久不可恢复。 |
| U1 | 应修风险，未确认 | `frontend/src/views/ImageGenView.vue:48`、`:51`、`:54`；`frontend/src/stores/imageGen.ts:276` | 密码管理器若把本站登录密码填入 Key 框并触发 input/change，会被当作用户编辑删除旧 Key，失焦随后作为 Bearer 发往目录。Chrome 隔离地址打开、聚焦、失焦时实测输入为空且 `:-webkit-autofill=false`；但这个 origin 没有已确认的本站保存密码，不能外推到正式站点。 | 必须在具有对应保存登录项、且请求完全隔离的同源环境补验；评估 `new-password`、显式确认凭据/加载交互等措施，不能单靠 `autocomplete=off` 宣称安全。本轮没有读取真实密码或向正式接口发送它。 |
| R3 | 次要，设计与实现口径不一致 | 本设计 `:111`、`:113`；`frontend/src/stores/imageGen.ts:469`、`:473`、`:479` | §5.2 把“200”写成保存条件，实际要求 HTTP 成功且目录 JSON 可解析、结构合法；200 HTML/错误结构始终待保存，刷新不会改变。反过来 `response.ok` 允许其他成功状态而非只认 200。§7 又要求识别 200 格式异常，合同没有明确这种情况能否保存。 | 保留当前对格式异常的保守处理，统一改为“当前 Key 最近一次获得结构合法的成功目录响应”，同步 wiki；不要为机械满足 200 措辞而放宽保存。 |
| R4 | 细节，已确认 | 本设计 `:76`；`backend/internal/service/gateway_service.go:523` | §4.1 写成固定 15 秒缓存，与 §1 的“默认 15 秒”及可配置实现不一致；配置为 30 秒时刷新后的陈旧窗口可能超过 15 秒。 | 写为“默认 15 秒，以 `gateway.models_list_cache_ttl_seconds` 为准”；当前配置校验范围为 10–30 秒。 |

R1、R2 和 U1 均未在本轮修改代码处理。不能把自动化通过解释为这些问题已经修复。

### 12.3 已核对无误的合同项

| 合同 | 已核对范围 |
| --- | --- |
| §1–2 目录与能力边界 | 普通目录由分组/账号/白名单决定，pinned 分支单独存在；目录不是实时能力探测。图片路由仅分发到 OpenAI/Grok（Composite 经中间件目标改写），不能据 Gemini 候选名称宣称其分组 Images API 可用。 |
| §3 触发 | 可写 computed 将用户编辑交给 `updateApiKey`；逐字输入不请求；粘贴延后读取输入，失焦自动一次；回车/刷新显式触发；同 Key 在途去重；回车 repeat/composition 分支有保护。真实粘贴及失焦/回车已做隔离浏览器验证。 |
| §3 回车安全 | Key 框及手动框均有 `.prevent`；测试真正断言 `defaultPrevented`，不是仅靠 jsdom 不发生默认提交。浏览器手动确认/加载阶段 mock 生图计数为 0。 |
| §4.1 发现请求 | 三个端点按 URL 解析后的 origin 比较；不一致停用发现；独立 fetch 只设置 Bearer/Accept，不注入站内 JWT 或 client_version，使用 no-store、redirect:error 和 AbortSignal。真正 HTTP 重定向未做浏览器网络级复现。 |
| §4.2 候选 | 对合法 ID 的 GPT/Grok/Gemini 判定与当前后端一致；展示/提交保留原始大小写，拒绝首尾空白/C0/DEL/C1，去重并自然排序。默认列表不包含公开别名时仍可手动输入。 |
| §4.3 选择 | 当前列表模型仍在目录则确认，缺失则清除且本次不改选；无选择走偏好/唯一候选；手动模型不被目录覆盖；失败保留未确认模型；Key 变化清候选但保留未确认选择。 |
| §5 模型与 Key 分离 | 模型偏好和 Key 是不同存储项，默认不存 Key，生成历史只记录请求模型等字段而不主动加入 Key。自定义模型/提示词本身由用户填写，不构成凭据保管位置。 |
| §5 正常保存/取消/恢复 | 成功目录后按记住状态写入；恢复直接给 ref 赋值，不走编辑删除；编辑删旧 Key；取消记住保留内存值；正常清除同时删除 Key/偏好而保留历史；写失败退回未记住并同步 checkbox。删除失败例外见 R2。 |
| §5 跨标签页 | Key 项和 `key=null` 事件使本页退出记住状态，模型偏好事件忽略；迟到目录完成时检查最新 remember 状态，正常顺序下不会把已清除 Key 写回。两个真实 iab 标签页清除/通知已验证，不把事件模拟当作多进程原子性证明。 |
| §6 发现竞态 | seq+Key 快照隔离旧成功/失败，旧 finally 不清新 controller；取消立即释放 in-flight Key；10 秒包括响应体读取。浏览器 mock 超时后可换 Key 正常重载，未发现永久 loading。 |
| §6 生成快照 | Key/model/mode/prompt/size/count/files 在首个 await 前快照；JSON、FormData 和历史使用 requestModel；生成中修改表单或清除配置不会改写已提交任务，无自动付费重试。 |
| §6 Vue/Pinia 生命周期 | `storeToRefs` 用于状态，动作直接解构；store 的 scope dispose 不等同视图卸载，切页继续生成；checkbox 同步写失败由事件处理器显式回写，异步失败由响应式更新。 |
| §7 错误映射 | 后端 code/message 及 OpenAI error.message 均解析；401 清候选，403 保留旧候选但未确认；429 按 INVALID_AUTH_RATE_LIMITED + Retry-After 暂停 auto；5xx 隐去正文；200 HTML 判格式错误。原样 message 的安全边界见 R1。 |
| §7 后端状态 | 模型请求未被 skipBilling 豁免，与生图共享 Key/用户/分组/余额/订阅前置鉴权；无效鉴权限流按 IP，Retry-After 由服务端秒数向上取整；成功可更新 TouchLastUsed。 |

### 12.4 测试证明力与缺口

| 类别 | 本轮判断 |
| --- | --- |
| 有效测试 | 50 项专项跑真实 imageGen store；目录与生图按 URL 分流；请求头精确比较；历史不含测试 Key；旧成功响应和迟到保存有 deferred promise；超时 mock 监听 abort；回车断言 defaultPrevented；同步/异步保存失败检查 checkbox。 |
| 候选测试边界 | 固定样例证明当前列举模型，不会自动随 Go 判定变化而失败；“镜像后端”仍需人工合同复核，本轮已核对源码。 |
| 布局测试边界 | AppLayout 被桩替换，登录/独立测试只证明 wrapper 分支，不能证明实际侧栏与窄屏；本轮浏览器加载了真实布局，但 mock 登录不代表真实认证验收。 |
| 粘贴测试边界 | 原单测先 setValue 再派发 paste，不能证明原生 paste→input 的时序；本轮在 Chrome 使用剪贴板粘贴补验，成功加载；剪贴板恢复为空。 |
| 跨标签页测试边界 | 原单测只 dispatch StorageEvent 并断言取消勾选，不实际删除另一页存储，也不验证迟到请求后不写回；本轮补验实际两标签页清除，但“跨页删除与在途响应交错”仍应加自动化。 |
| 测试隔离细节 | 新建 Pinia 不等于 dispose 旧 store；两个测试文件未统一 dispose store，窗口 storage listener/悬挂定时器可跨用例存活。当前测试通过不能证明不存在这种测试资源残留，建议显式释放 Pinia/store。 |

仍缺少重要自动化：R1 回显脱敏、R2 删除失败及恢复；密码管理器真实自动填充；旧请求 reject/finally 与新请求在途交错；同 Key 在途去重的直接断言；超时后原 Key 显式重试；无偏好多候选不自动选择；非法手动草稿的 UI 流程；Retry-After 缺失/跨域不可见；`storage key=null` 与迟到完成组合；图改图时同时更改 Key/model 的请求及历史快照；真实 HTTP 重定向拒绝。现有保存写失败测试不能替代删除失败测试。

### 12.5 本轮重跑命令

所有命令从仓库根目录执行，退出码均为 0；全量 Vitest 在 typecheck、专项、lint 完成后单独运行，未与其他重型测试/构建并行。

| 命令 | 实际结果 |
| --- | --- |
| `pnpm --dir frontend run typecheck` | 通过 |
| `pnpm --dir frontend exec vitest run src/utils/__tests__/imageGenModels.spec.ts src/stores/__tests__/imageGen.discovery.spec.ts src/views/__tests__/ImageGenView.spec.ts` | 3 文件 / 50 用例通过：10 + 3 + 37 |
| `pnpm --dir frontend run lint:check` | 通过 |
| `pnpm --dir frontend exec vitest run` | 423 文件 / 3441 用例通过，119.88 秒；本轮未出现失败 |
| `git diff --check` | 通过；本节写入后再次检查 |

### 12.6 浏览器验收记录

| 场景 | 结果与限制 |
| --- | --- |
| 独立访问 | iab 当前源码页面正常显示独立页，无站内侧栏；桌面可操作。 |
| 登录布局 | Chrome 合成站内身份 + mock `/auth/me` 下显示实际 AppLayout/侧栏；不是使用真实账号完成登录。 |
| 窄屏 | 独立页及登录页均设置 390px；页面报告 `innerWidth=390`、文档 scrollWidth=382，登录表单宽约 309px，控件可操作，无观察到的横向溢出。截图通道缩放/一次 CDP 截图超时限制了精细视觉核对，不宣称所有断点像素级验收完成；临时 viewport 已恢复。 |
| 粘贴/失焦/回车 | Chrome 真实粘贴触发；iab 失焦加载、回车加载及手动确认可用，未误发生图；手动点击生成前 mock images 计数为 0。 |
| 记住与恢复 | iab 保存、刷新恢复 Key 与模型并重新加载目录通过；Key 改动清旧配置，正常清除和跨标签页退出记住通过；删除故障见 R2。 |
| 模型 | 下拉选择 2.5 样例、手动别名、状态徽标可用；候选名称均来自 mock，不证明任何真实模型可用。 |
| 异常 | mock 401、403、无效鉴权 429 + 30 秒提示、503、200 HTML、空目录、10 秒超时均显示预期状态；限流换 Key 后仍提示暂停，手动刷新恢复；超时后换 Key 可重载。网络断开/CORS 的浏览器级场景未单独模拟。 |
| 生图隔离 | Chrome 手动点击一次生成，服务端 mock 返回受控失败，页面及 toast 显示失败；全程没有上游转发，未消耗真实额度。未做浏览器图改图成功及下载链路验收。 |
| Chrome 自动填充专项 | 已连接用户 Chrome，在隔离地址打开、聚焦、失焦并检查 `type=password` / `autocomplete=off`、是否非空及 `:-webkit-autofill`；未见填充。未确认该隔离 origin 存在匹配保存密码，故正式站点风险仍为 U1，不能标为“不发生”。 |
| 未验证 | 真实 Key 目录、真实文生图/图改图、Grok/Gemini 参数兼容、生产构建/CSP、带正式保存登录密码的同源自动填充。真实请求需另行授权，不能替代 mock 验收。 |

### 12.7 对第 11 节逐条核实

| 第 11 节说法 | 核实结果 |
| --- | --- |
| “通过，无阻塞；问题已修复并补测试” | 未发现已确认的全局阻塞，但不足以认定完整验收通过；遗漏 R1/R2，且 U1 仍未闭环。以本节附条件结论为准。 |
| 50 项专项通过、连续 3 次一致 | 本轮独立复跑 50 项通过；历史连续 3 次未取得日志，次数与历史稳定性未独立确认。 |
| 5 项变异检查均杀死，恢复用例曾补强 | 本轮按只读要求未注入变异；源码有对应关键断言，但不能据此证明历史变异运行过程或变异覆盖完整性。历史说法标未独立确认。 |
| 全量 423/3441 通过，曾有一次并行失败 | 本轮单独重跑确为 423/3441 全通过；历史失败无日志，不能定位、归因或认定为已修复/一定偶发。 |
| typecheck、lint 通过 | 本轮独立重跑均通过。 |
| 失败后 keyVerified 修复 | 已核实，`applyDiscoveryFailure` 和 Key 变化均置 false，相关专项断言有效；“200”仍应按 R3 澄清。 |
| 回车、恢复、请求头、checkbox、超时、旧目录断言补强 | 已核实存在且本轮通过；jsdom 的默认行为限制已用 defaultPrevented 弥补。端点不同源测试证明不调用 fetch，不证明真实重定向行为。 |
| 限流暂停提示修复 | 已核实，换 Key 保留暂停标记，idle 文案响应式；iab 已复现暂停与显式恢复。 |
| 响应体超时修复 | 已核实，json 失败后优先检查 timedOut，专项用 abort 驱动响应体 reject；浏览器另验证整体超时及恢复。 |
| 按住回车 repeat 修复 | 已核实代码与直接派发 repeat 事件断言；真实长按不是本轮浏览器已验项。 |
| 删除失败警告保留至写入/删除成功 | 对继续编辑 Key 的路径成立；表述过宽，`setRememberKey` 可提前清提示，且 R2 的清除入口消失未覆盖。 |
| 偏好保存标记、蓝色手动徽标、C1 校验与空行 | 已核实代码/测试及 wiki 差异；不能据此推断 Key 删除失败的标记同样正确。 |
| Composite 别名、403/503、跨页 key=null 等设计修订 | 当前文档已包含相关说明；源码有 key=null 分支，自动化没有该事件与迟到保存组合覆盖；缓存 TTL 和成功响应口径仍有 R3/R4。 |
| 自动填充风险转浏览器验收 | 风险判断合理；本轮 Chrome 隔离页未出现填充，正式同源条件仍不足，未确认而非已排除。 |
| Grok/Gemini、真实请求和生产产物未覆盖 | 属实，本轮仍未验证这些能力；本轮补充的只有上述受控浏览器范围。 |

后续收口顺序：修复 R1/R2 并补回归 → 澄清 R3/R4 → 在安全隔离且具备同源保存登录项的 Chrome 环境核实 U1 → 补齐必要浏览器场景。未经过验证的模型和模式继续标为未验证。

## 13. 修复与自动化验证记录（2026-11-02）

结论：**第 12 节 R1–R4 已修复；U1 已做代码层处理（部分采纳，见 13.1）。代码修复完成，待 Codex 独立审核与浏览器验收。** 本节为实现方记录，不代表审核或浏览器验收通过；正式同源 Chrome 密码自动填充仍未验证，不标记为通过。第 12 节保留为 Codex 原始结论。

### 13.1 第 12 节逐项处理

| 编号 | 处理 | 修改文件 | 最终行为 |
| --- | --- | --- | --- |
| R1 | 采纳。级别补充：本站鉴权中间件返回固定文案（`backend/internal/server/middleware/api_key_auth.go` 中 `"Invalid API key"` 等），审计日志对凭据掩码（`audit_log.go` `MaskedRequestCredential`），同源部署下未见回显路径；外部网关或上游可能回显，按合同修复 | `frontend/src/utils/imageGenModels.ts`（`redactCredential`）、`frontend/src/stores/imageGen.ts` | 目录与生图两条路径在写入页面状态和 toast 前，用请求发起时的 Key 快照把回显替换为 `***`；兼容顶层 `message` 与 `error.message`，其余业务信息保留 |
| R2 | 采纳 | `imageGen.ts`、`frontend/src/views/ImageGenView.vue` | 删除失败改为响应式 `keyRemovalFailed`，独立显示“未能清除浏览器中保存的 API Key，可能仍有残留”及“重试清除”按钮，并计入“清除已保存配置”的显示条件；编辑、勾选/取消记住、清空输入不解除；删除成功、覆盖写入成功或其他标签页删除/覆盖该记录后才解除 |
| R3 | 采纳 | 本设计 §5.2、wiki | 保存条件统一为“当前 Key 最近一次获得结构合法的成功目录响应”（实现为 `response.ok` 且目录结构合法），不放宽对 200 HTML/错误结构的保守处理 |
| R4 | 采纳 | 本设计 §4.1 | 写为“默认 15 秒，以 `gateway.models_list_cache_ttl_seconds` 配置为准（允许 10–30 秒）” |
| U1 | 部分采纳（经用户确认） | `imageGen.ts`、`ImageGenView.vue`、本设计 §3/§5/§6/§7/§9、wiki | 采纳：输入/change/粘贴/失焦/勾选记住都不发请求，只有回车和“加载模型/刷新模型”按钮发送 Key；编辑 Key 不删除已保存的 Key，新 Key 加载成功后才覆盖。未采纳：生成前强制确认 Key。理由：改动前版本（`git show HEAD:frontend/src/views/ImageGenView.vue` 第 46–48 行）已是同样的密码框加 `autocomplete="off"`，点击生成直接使用框内内容；生成是用户显式点击，不属于本功能新增的自动发送路径，强制确认会给“填 Key → 写提示词 → 生成”多加一步。生成仍不保存、不替换已保存的 Key |

连带处理：取消了“无效鉴权限流后暂停自动触发”（不再有自动触发，429 只提示 `Retry-After` 秒数）；覆盖写入失败时同时删除旧 Key（编辑不再删旧值后新出现的残留路径）；测试结束与模拟刷新时显式释放 store（第 12.4 节测试隔离建议）。

### 13.2 最终交互规则（供浏览器验收逐项核对）

“当前 Key”指输入框中的值，加载模型与生成都使用它；“已保存 Key”指 `image-gen-key-v1` 中的值，两者可以不同。

| 操作 | 是否发请求 | 对已保存 Key / 页面状态的影响 |
| --- | --- | --- |
| 输入、密码管理器填入（input/change） | 不发 | 不改存储；清空旧候选并取消在途目录请求，列表模型标为未确认；勾选记住且已有保存值时提示“浏览器中保存的是之前的 Key，当前 Key 加载模型成功后替换” |
| 粘贴 | 不发 | 同输入 |
| 失焦 | 不发 | 无 |
| Key 框回车 | `GET /v1/models`（当前 Key）；忽略按住连发与输入法组合；同 Key 在途去重；`preventDefault`，不提交生图 | 成功且勾选记住：写入/覆盖为当前 Key；401：提示“Key 无效，未保存”，已保存 Key 不变；其他失败：保持待保存/待替换 |
| “加载模型/刷新模型”按钮 | 同回车；无目录时显示“加载模型”，有目录（含空目录）时显示“刷新模型” | 同回车 |
| 勾选记住 | 不发 | 当前 Key 已验证则立即写入；否则显示“待保存：当前 Key 加载模型成功后保存”；端点不同源停用发现时立即写入 |
| 取消勾选 | 不发 | 删除已保存 Key，内存 Key 保留；删除失败显示残留警告与“重试清除” |
| 清除已保存配置 | 不发；取消在途目录请求 | 删除 Key 与模型偏好，清空当前 Key、模型、候选；Key 删除失败时保留警告、“重试清除”和本按钮 |
| 重试清除 | 不发 | 只删除 Key 记录；成功 toast“已清除浏览器中保存的 API Key”，失败 toast 提示检查存储设置 |
| 生成 | `POST` 生图端点，使用当前 Key 快照；不发目录请求 | 不保存、不替换已保存 Key；写模型偏好；错误按请求快照脱敏 |
| 打开/刷新页面 | 有显式记住的 Key 时 `GET /v1/models` 一次 | 恢复已保存 Key，不改存储；配置损坏时忽略，删除失败显示残留警告 |
| 其他标签页删除、覆盖或清空存储 | 不发 | 本页退出记住状态并提示；解除本页残留警告；存储中若仍是本页写入的值则删除；之后到达的成功响应不写回 |
| 端点不同源（发现停用） | 从不请求目录 | 勾选或回车时直接写入/覆盖当前 Key |

### 13.3 自动化测试

自动化测试中的 input/change/paste/blur 都是模拟 DOM 事件，只证明事件处理逻辑；Chrome 是否会自动填充、填充时触发哪些事件，必须在浏览器中验证。

| 要求 | 用例（`ImageGenView.spec.ts`，除非注明） |
| --- | --- |
| 1 两种错误结构回显 Key 不进页面/toast | `redacts the request key from catalog errors in both error shapes`；`redacts with the request key snapshot after the key changed mid-generation`；`imageGenModels.spec.ts` `redactCredential` |
| 2 请求后切换 Key，旧请求失败仍正确脱敏 | `redacts with the request key snapshot after the key changed mid-generation`（生图）；`ignores a late failure for a previous key while the next key is loading`（目录旧失败被丢弃，不显示） |
| 3 仅保存 Key 时删除失败可重试，恢复存储后清除 | `keeps a retry entry when clearing a key-only config fails to remove the key` |
| 4 删除失败警告不被编辑、重新勾选、空输入清掉 | `keeps the removal warning through edits, re-checking and empty input until an overwrite succeeds` |
| 5 模拟事件不发送草稿、不删除旧 Key | `treats input, change, paste and blur as a draft: nothing is sent and the saved key is kept`；`loads image models on Enter ...` 开头的负向断言 |
| 6 显式确认才切换 Key；生成不保存 Key | `replaces the saved key only after an explicit load; generating uses the typed key without saving it`；`keeps the saved key on edit and does not replace it with an invalid key`；store `disables discovery and saves the key only on explicit confirmation ...` |
| 7 跨标签页清除与迟到响应不写回 | `does not write the key back when another tab removes the saved key / clears all storage during an in-flight load`（含 `key=null`） |
| 8 既有行为 | 请求快照、切页继续生成、模型选择、超时等原用例保留；另补同 Key 在途去重、超时后同 Key 重试（store）、无偏好多候选不自动选择、覆盖写入失败删除旧 Key |

变异检查：依次注入 11 个缺陷，每个都有用例失败，注入后源文件逐字节恢复——生图不脱敏、生图用当前输入值脱敏、目录错误不脱敏、清除入口不含删除失败状态、编辑即删除已保存 Key、失焦触发加载、勾选记住立即请求、勾选清掉删除失败状态、迟到成功不检查记住状态、覆盖失败不删除旧 Key、生成后保存当前 Key。

未补的第 12.4 节缺口：`Retry-After` 缺失/跨域不可见、图改图中途同时改 Key 与模型的快照、非法手动草稿的 UI 流程、真实 HTTP 重定向拒绝。

### 13.4 验证命令

均从仓库根目录依次执行；全量 Vitest 单独运行，未与其他重任务并行。

| 命令 | 结果 |
| --- | --- |
| `pnpm --dir frontend run typecheck` | 通过 |
| `pnpm --dir frontend exec vitest run src/utils/__tests__/imageGenModels.spec.ts src/stores/__tests__/imageGen.discovery.spec.ts src/views/__tests__/ImageGenView.spec.ts` | 3 文件 / 60 用例通过（11 + 3 + 46；修复前为 50） |
| `pnpm --dir frontend run lint:check` | 通过 |
| `pnpm --dir frontend exec vitest run` | 423 文件 / 3451 用例通过，135.70 秒 |
| `pnpm --dir frontend run build` | 通过；仅有既有的动态导入/分块体积提示；产物输出到已忽略的 `backend/internal/web/dist`，未部署 |
| `git diff --check` | 通过；新增未跟踪文件另以 `git diff --no-index --check` 检查无空白问题，均为 LF |

未调用真实模型或生图接口；所有请求由测试 mock 接管。

### 13.5 待 Codex 审核与浏览器验收

- 独立审核本节修改，按 13.2 逐项走查。
- U1：在具备本站已保存登录项的正式同源 Chrome 环境中验证，请求必须隔离：密码管理器是否填入 Key 框；填入后失焦、粘贴是否仍不发请求；已保存 Key 是否保持。是否达到上线标准由 Codex 判断。
- 第 9 节浏览器场景按新规则复验：回车/按钮加载、勾选不请求、编辑后“之前的 Key”提示、删除失败警告与重试（需模拟存储故障）。
- 真实 Key 目录读取、真实生图，以及 Grok、Gemini 对尺寸、数量、`b64_json` 的参数兼容性，仍未验证，需另行授权。

### 13.6 提交前复核（2026-11-03）

提交测试前，由独立审查代理（与实现方同为 Claude，不替代 Codex 审核）只读复核本功能全部改动，并对被改坏的 store 副本运行测试来检验测试的证明力。结论：无阻塞问题；1 项应修已修复，5 项测试缺口已补齐。

| 级别 | 问题 | 处理 |
| --- | --- | --- |
| 应修 | 跨标签页竞态：B 页删除已保存 Key 后，A 页迟到的目录成功响应在 B 的 storage 事件到达前写入新 Key；A 随后退出记住状态。结果是两页都不记住，存储中却留有 Key，且没有任何警告，下次打开页面会被恢复 | A 页因 storage 事件退出记住时，若存储中仍是本页写入的值，则一并删除（删除失败按残留警告处理）。写入前再读一次存储无法闭合此窗口：Chrome 中存储值与事件同时到达其他标签页 |
| 次要 | 以下规则没有测试，删掉对应代码测试仍然全过：跨页事件解除残留警告；旧请求的 finally 不得释放新请求的在途标记；生成时写模型偏好；损坏配置删除失败时提示残留；未勾选记住时不提示“Key 无效，未保存” | 各补断言或用例 |
| 次要，不改 | 编辑 Key 时会清掉一次性提示（“无法保存…”“其他标签页修改了…”“Key 无效，未保存”） | 这些提示都针对上一个 Key 或上一次操作，用户开始编辑后清掉符合预期；需要持续可见的残留风险由独立的 `keyRemovalFailed` 警告承担，编辑不清除 |
| 验收提示 | 首次访问（无模型偏好）时直接点生成会提示“请选择或手动输入模型”，改动前会静默使用 `gpt-image-2` | 这是 §4.3“不写死默认模型”的预期行为，已补首次访问用例；测试人员需按回车或点击“加载模型”后再生成 |

另将原跨标签页用例改为真实时序：先由另一页改写存储再派发事件，并断言不删除对方写入的 Key。变异检查：对上述 6 处依次注入缺陷，每个都有用例失败，注入后源文件逐字节恢复。

验证（2026-11-03，仓库根目录依次执行，全量单独运行）：`typecheck`、`lint:check` 通过；三个专项文件 63 用例通过（11 + 3 + 49）；前端全量 423 文件 / 3454 用例通过（128.13 秒）；`build` 通过（仅既有分块提示，产物在已忽略的 `backend/internal/web/dist`，未部署）；`git diff --check` 通过。未调用真实模型或生图接口。
