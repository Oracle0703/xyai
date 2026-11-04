# 图片生成页设计

更新：2026-11-04。状态：**已合入 main（首版 `7fad5a525`，多图与状态持久化 `31453d3c0`，动态模型与 Key 记忆 `7d0f760fc`）。Codex 对最终修复的独立审核、浏览器验收（含正式站点 Chrome 密码自动填充 U1）、真实 Key 目录读取与真实生图均未完成（§17）。**

本文是图片生成页 `/image-gen` 的唯一文档，包含需求、规则、接口合同、代码位置、易错点和验证方式。审核原文与证据目录已移出仓库，最后存在于提交 `26f6363af`（可用 `git show 26f6363af:docs/features/image-gen-model-selection-design-cn.md` 查看；另有 `docs/features/image-gen-state-persistence-multi-image-design-cn.md`、`docs/superpowers/specs/2026-05-24-image-gen-tool-design.md`、`docs/superpowers/plans/2026-05-24-image-gen-tool.md`）。凡是对后续修改有约束的审核结论都已写入正文，主要在 §14。

阅读顺序建议：
- 改页面状态或生成流程：先读 §4、§10、§14；
- 改模型选择或 Key 记忆：读 §7–§9、§11、§14；
- 改上传限制或 CSP：读 §5、§6；
- 定位代码：读 §13；
- 跑测试或验收：读 §16、§17。

## 1. 需求与演进

| 阶段 | 需求 | 结果 |
| --- | --- | --- |
| 首版（2026-05-24） | 运营人员粘贴平台 API Key 即可生图，无需登录 | 公开整页工具 `/image-gen`，纯前端，调用现有网关 `POST /v1/images/generations`；鉴权、分组生图权限、审核、路由、计费、用量日志全部复用后端 |
| 图改图 | 上传/粘贴原图做编辑 | `POST /v1/images/edits` multipart；恢复历史时清除残留原图 |
| 登录嵌入 | 登录用户在站内侧栏中使用 | 登录用户套 `AppLayout`，未登录仍为独立整页（§3） |
| 状态持久化与多图（2026-06-10） | 切菜单丢失一切；生产环境原图预览不显示；历史单列浪费空间；只能传一张原图 | Pinia store（§4）、CSP 放行 `blob:`（§6）、多图最多 16 张与 n ≤ 10（§5）、历史两列 |
| 动态模型与 Key 记忆（2026-11-02） | 接口已升级到 2.5，页面仍写死 `gpt-image-2`；每次都要重新输入 Key | 按当前 Key 的 `/v1/models` 目录选模型，可手动输入；可选在本浏览器记住 Key（§7–§11） |

后端没有单独的 `gpt-image-2.5`：2.5 系列是 `gpt-image-2.5-flare`、`gpt-image-2.5-sunburst` 及其 `-2026-09-08` 快照。页面不把任何版本写死为默认值，以当前 Key 的模型目录和真实生成结果为准。

## 2. 范围与边界

| 包含 | 不包含 |
| --- | --- |
| 文生图、图改图（多图）、结果展示、下载/复制 Data URL、本地历史 | 服务端保存 Key、多设备同步、多 Key 管理、自动创建 Key、用登录态代替网关 Key |
| 当前 Key 的模型发现、按后端图片模型判定筛选、选择与手动输入 | 修改上游模型映射、计费、鉴权、调度或模型目录生成算法 |
| 显式勾选后在本浏览器记住 Key；始终记住上次模型（不含 Key） | 通过真实生图请求自动探测能力，或失败后自动换模型/参数重试 |
| 并发响应隔离、存储故障降级、错误码映射与脱敏 | 后端改动（本功能除 CSP 外不改后端） |

模型列表是网关发布的候选目录，不是逐模型探活或图片能力证明：列表有模型不代表余额、额度、上游健康或图改图能力一定满足；非 OpenAI/Grok 分组的目录也可能出现图片模型名，但其 `/v1/images/*` 返回 404。没有模型也不等于 Key 无效。若真实目录遗漏目标模型，先查账号映射、分组白名单与 pinned 目录原因，不能为页面便利扩大后端模型权限。

## 3. 路由与布局

- 路由 `/image-gen`（`frontend/src/router/index.ts`，`meta.requiresAuth: false`），并在 `BACKEND_MODE_ALLOWED_PATHS` 中。页面不能依赖必须登录的用户 API。
- 侧栏入口：`frontend/src/components/layout/AppSidebar.vue` 的 `nav.imageGeneration`。
- `ImageGenView.vue` 的 `embedded` 计算属性：`authStore.isAuthenticated && !(appStore.backendModeEnabled && !authStore.isAdmin)`。为真时外层用 `AppLayout`（保留侧栏），否则用视图内的轻量 `StandaloneShell` 整屏外壳并显示独立页头。
- 本项目没有嵌套布局路由：`App.vue` 只有一个裸 `RouterView`，各视图自行包 `AppLayout`，任何侧栏导航都会卸载整个视图。这是 §4 改用 store 的直接原因。

## 4. 状态管理（Pinia store）

决策：**用 Pinia store，不用 KeepAlive**。依据：KeepAlive 会把整棵 `AppLayout` 子树缓存第二份（布局在视图内部）；全局 paste 监听器要手工管理停用态；`include` 白名单字符串耦合易静默失效。store 方案让生成任务与组件生命周期彻底解耦，还能跨页 toast 通知。

`frontend/src/stores/imageGen.ts`（`useImageGenStore`，setup 风格）持有：表单（`apiKey`/`prompt`/`size`/`count`/`generationMode`）、`sourceImages: {id, file, previewUrl}[]`、`results`、`historyItems`、`isGenerating`、`errorMessage`，以及模型发现与 Key 记忆状态（§7–§10）。视图是模板壳：`storeToRefs` 绑状态，action 直接解构；只保留 `showKey`、文件 input DOM ref、纯 UI helper 和 window paste 监听器（`onMounted` 注册、`onBeforeUnmount` 移除，委托给 `addSourceImages`）。

- 生成成功/失败都调 app store 全局 toast，用户在任何页面都能收到。
- 历史：localStorage `image-gen-history-v1`，最多 20 条（`MAX_HISTORY_ITEMS`），只在生成成功后写入；条目含 prompt、model、size、createdAt、mode、images（data URL），**不含 Key**。localStorage 不可用或超额时 `saveHistory` 已 try/catch，生成结果仍可用，只是历史不持久。
- 历史列表两列：`grid grid-cols-1 items-start gap-3 md:grid-cols-2`，窄屏回落单列。
- F5 刷新丢失全部内存态（含进行中的生成展示；请求本身已发出、计费照常），这是方案边界，不是回归。

## 5. 多图输入与上传限制

- dropzone：input `multiple`、`accept="image/png,image/jpeg,image/webp"`；选择、拖拽、粘贴统一走 `addSourceImages` 追加；缩略图网格、单张移除、全部移除、“已选 N/16 张 · 共 X MB”计数。
- 校验顺序：类型 → 单张大小 → 张数 → 总量；有拒绝时提示首个原因，全部通过清错误。
- 上限常量（`imageGen.ts`）：

| 常量 | 值 | 依据 |
| --- | --- | --- |
| `MAX_SOURCE_IMAGES` | 16 | 官方 `/v1/images/edits` 的 `image[]` 上限 |
| `MAX_SOURCE_IMAGE_SIZE` | 20MB | 历史依据见下方说明 |
| `MAX_TOTAL_SOURCE_SIZE` | 100MB | 网关 `gateway.max_body_size` 默认 256MB，留余量避免 413 |
| `MAX_IMAGE_COUNT` | 10 | 官方 n 取值 1–10；计费按上游实际返回张数，多要少给不会多扣 |

- 单张 20MB 的原始依据是：网关 `parseOpenAIImagesMultipartRequest` 曾对每个分片在 20MB 处静默截断，超限文件会变成损坏图片送往上游。**该依据已过期**：提交 `cde6ddfd8`（2026-10-26）把 `backend/internal/service/openai_images.go` 的 `openAIImageMaxUploadPartSize` 改为 50MB，并在超限时返回错误而不是截断。前端 20MB 上限和 `imageGen.ts` 中“静默截断、不能放宽”的注释尚未随之调整；是否放宽到 50MB 待决定（§17）。放宽前须确认总量 100MB 与 `gateway.max_body_size` 的关系仍成立。
- 网关侧无需改动：multipart 解析收集 `image` / `image[` 前缀全部字段，模型重写时逐 part 原样转发。
- FormData `append('image[]', file)` **不传 filename 参数**：File 自带 name，显式传参会按 WHATWG 规范重新包一层 File（浏览器与 jsdom 一致），破坏测试同一性断言且无意义。
- 尺寸选项保留了既有的 1792x1024/1024x1792（官方 gpt-image 仅支持 1024x1024/1536x1024/1024x1536/auto），未收敛；是否收敛由上游通道实际支持情况决定。

## 6. 预览 CSP

原图预览用 `URL.createObjectURL` 生成 `blob:` 地址。后端 CSP 默认 `img-src 'self' data: https:` 不放行 `blob:`，浏览器静默拦截；dev 模式 vite 不发 CSP 头，所以本地永远复现不了。修复（需部署后端生效）：

- `backend/internal/config/config.go` 的 `DefaultCSPPolicy`：`img-src 'self' data: blob: https:`；`deploy/config.example.yaml` 示例串同步。
- `backend/internal/server/middleware/security_headers.go` 的 `requiredCSPDirectiveValues` 追加 `{"img-src", "blob:"}`：这是“旧配置缺新指令”的运行时补丁机制（支付 SDK 域名同款），自定义了旧 policy 的存量部署无需手改生产 config.yaml。
- `blob:` 只允许同源脚本自己创建的对象 URL，不引入远程来源，安全面无扩大。
- 部署后 `curl -I` 任意非 API 页面，确认响应头 `img-src` 含 `blob:`。

## 7. 页面交互（模型与 Key）

表单顺序：API Key → 记住选项 → 模型选择 → 生成方式 → 提示词/原图 → 尺寸和数量 → 生成按钮。

“当前 Key”指输入框中的值，加载模型与生成都使用它；“已保存 Key”指 `image-gen-key-v1` 中的值，两者可以不同。

| 操作 | 是否发请求 | 对已保存 Key / 页面状态的影响 |
| --- | --- | --- |
| 输入、密码管理器填入（input/change） | 不发 | 不改存储；清空旧候选并取消在途目录请求，列表模型标为未确认；勾选记住且已有保存值时提示“浏览器中保存的是之前的 Key，当前 Key 加载模型成功后替换” |
| 粘贴 | 不发 | 同输入 |
| 失焦 | 不发 | 无 |
| Key 框回车 | `GET /v1/models`（当前 Key）；忽略 `event.repeat` 与输入法组合 `isComposing`；同 Key 在途去重；`preventDefault`，不提交生图 | 成功且勾选记住：写入/覆盖为当前 Key；401：提示“Key 无效，未保存”，已保存 Key 不变；其他失败：保持待保存/待替换 |
| “加载模型/刷新模型”按钮 | 同回车；当前 Key 无目录时显示“加载模型”，有目录（含空目录）时显示“刷新模型”；加载中禁用 | 同回车；不清除提示词、原图、结果或历史 |
| 勾选记住 | 不发 | 当前 Key 已验证（§9.2）则立即写入；否则显示“待保存：当前 Key 加载模型成功后保存”；端点不同源停用发现时立即写入 |
| 取消勾选 | 不发 | 删除已保存 Key，内存 Key 保留；删除失败显示残留警告与“重试清除” |
| 清除已保存配置 | 不发；取消在途目录请求 | 删除 Key 与模型偏好，清空当前 Key、模型、候选和发现错误；提示词、结果、历史不删；Key 删除失败时保留警告、“重试清除”和本按钮 |
| 重试清除 | 不发 | 只删除 Key 记录；成功 toast“已清除浏览器中保存的 API Key”，失败 toast 提示检查存储设置 |
| 生成 | `POST` 生图端点，使用当前 Key 快照；不发目录请求 | 不保存、不替换已保存 Key；写模型偏好；错误按请求快照脱敏 |
| 打开/刷新页面 | 有显式记住的 Key 时 `GET /v1/models` 一次 | 恢复已保存 Key，不改存储；配置损坏时忽略，删除失败显示残留警告 |
| 其他标签页删除、覆盖或清空存储 | 不发 | 本页退出记住状态并提示；解除本页残留警告；存储中若仍是本页写入的值则一并删除；之后到达的成功响应不写回 |
| 端点不同源（发现停用） | 从不请求目录 | 勾选或回车时直接写入/覆盖当前 Key |

其他控件：

- 模型下拉显示 API 返回的完整 ID；下拉外提供“手动输入模型”入口（别名、列表遗漏），不限前缀，提示“是否支持图片生成以接口结果为准”；去首尾空白后须非空且不含控制字符；手动框回车/失焦确认（`confirmManualModel`，回车 `.prevent`）；从下拉选择即退出手动模式。
- 版本徽标显示当前模型及状态（目录已确认 / 未确认 / 手动，手动用蓝色区分）；无选择时显示“未选择模型”。
- 生成按钮：点击时校验并提示缺失项（Key、模型、提示词、图改图原图）；不要求先加载模型；目录加载中或失败不阻止使用当前（未确认）模型；生成中阻止重复提交。
- 首次访问（无模型偏好）直接点生成会提示“请选择或手动输入模型”；改动前会静默使用 `gpt-image-2`。这是“不写死默认模型”的预期行为，测试人员需先回车或点“加载模型”。
- `restoreHistory` 不修改当前模型（历史模型可能不在当前 Key 目录）；历史条目展示原模型。
- 模型列表不展示“已验证可用”标签，不按名称推断尺寸、图改图能力和价格；切换模型沿用现有尺寸与数量，参数不兼容由生成接口报错，不自动降级。

## 8. 模型发现合同

### 8.1 请求与端点

```http
GET /v1/models
Authorization: Bearer <当前输入的 API Key>
Accept: application/json
```

- 独立 `fetch`，不用注入登录 JWT 的管理 API client，不加 `client_version`（带它时后端会转 Codex manifest）。
- Key 只放 Authorization 头，不进 URL、日志、历史或 toast。
- `cache: 'no-store'`、`redirect: 'error'`、10 秒超时（`MODEL_DISCOVERY_TIMEOUT_MS`，含读取响应体）、`AbortController` 可取消；失败不后台循环重试。
- 端点为构建期变量：`VITE_IMAGE_MODELS_ENDPOINT`（默认 `/v1/models`）、`VITE_IMAGE_GENERATION_ENDPOINT`（默认 `/v1/images/generations`）、`VITE_IMAGE_EDIT_ENDPOINT`（默认 `/v1/images/edits`）。三者解析后的 origin 必须一致（`endpointsShareOrigin`），否则停用自动发现、提示部署配置问题、只允许手动模型。典型场景：生图端点配置为外部网关但未配置模型端点，默认同源 `/v1/models` 会把外部 Key 发给本站后端。不猜测路径派生第三方 URL，不跟随重定向。外部服务的 HTTPS 与 CORS 由部署者负责（CSP `connect-src` 已放行 `https:`）。
- 后端：`backend/internal/server/routes/gateway.go` 的 `GET /v1/models` 走 API Key 鉴权；`backend/internal/handler/gateway_handler.go` 的 `Models` 按分组平台、账号 model_mapping、分组白名单生成目录，无映射时回落默认目录（`backend/internal/pkg/openai/constants.go`，含 `gpt-image-1`、`gpt-image-1.5`、`gpt-image-2`、`gpt-image-2.5-flare`、`gpt-image-2.5-sunburst`）；开启 Codex manifest 的 OpenAI 分组走 pinned 上游目录。普通目录有服务端缓存，默认 15 秒，以 `gateway.models_list_cache_ttl_seconds` 为准（校验范围 10–30 秒）；“刷新”不承诺立即反映后台配置变更。
- `/v1/images/*` 按分组平台分发：OpenAI 与 Grok 支持，其他平台 404，Composite 按解析出的目标平台分发。
- 发现请求与其他网关请求一样会刷新 Key 的 `TouchLastUsed`，已知副作用，不规避。

### 8.2 解析与筛选

| 规则 | 约定 |
| --- | --- |
| 响应形状 | 接受 `{ data: [{ id: string }] }`；顶层缺数组属于响应异常，合法空数组属于空目录 |
| 条目清洗 | 忽略非对象、非字符串或空 ID，按完整 ID 去重；拒绝含首尾空白或控制字符（C0/DEL/C1）的 ID，不悄悄修改服务器模型标识 |
| 候选筛选 | `isImageModelCandidate` 镜像后端 `validateCompatibleImagesModel`：小写后匹配 `gpt-image-` 前缀、`grok-imagine`、`grok-imagine-edit`、`grok-imagine-image` 前缀、`gemini-` 前缀且以 `-image` 结尾或含 `-image-`。小写只用于判断，展示与提交用原始 ID。仅为候选筛选，不扩大后端权限 |
| Composite | 后端按解析后的上游模型校验，目录中的公开别名可能不满足判定而被隐藏，需手动输入 |
| 顺序 | `compareModelIds` 确定性自然排序（数字段按数值），仅为展示，不承担版本推荐 |

### 8.3 选择与恢复

| 场景 | 约定 |
| --- | --- |
| 目录成功 | 手动来源保持手动。列表来源的当前选择在目录中则标记已确认；不在则清除并提示“原模型 X 不在当前 Key 的目录，请重新选择”，本次不自动改选。无当前选择时依次取：在目录中的模型偏好 → 唯一候选 → 提示“请选择模型”（多候选不自动选） |
| 目录失败 | 不清除当前选择，标为未确认，允许提交。401 清空候选；其他失败在同 Key 时保留旧候选并标记“刷新失败，当前为旧目录”（`discoveryStale`） |
| 上次模型 | 读取 `image-gen-model-v1` 作为未确认的初始选择，按上两行确认或清除；手动来源恢复为手动模式，不伪装成目录返回值 |
| Key 变化 | 立即清空旧候选、旧错误并取消旧请求；当前选择保留为未确认 |

## 9. 记住模型与 API Key

### 9.1 模型偏好（始终保存，无开关）

- 键 `image-gen-model-v1`，内容仅 `version`、`model`、`modelSource`（`list`/`manual`）。
- 显式选择、确认手动模型、生图请求实际发出时写入；本地校验未通过的提交不写；目录清除选择时不改写偏好；只保留最后一个。
- 读写失败静默降级；损坏或未知版本时忽略并尽力删除。“清除已保存配置”按钮的显示以实际写入结果（`preferenceStored`）判断。

### 9.2 记住 API Key（默认关闭）

勾选项“在此浏览器记住 API Key”，说明：“仅保存在当前浏览器，其他使用此浏览器的人可能访问；公共设备请勿勾选。”界面区分“已保存”“待保存”和“浏览器中保存的是之前的 Key”。

- 键 `image-gen-key-v1`，内容仅 `version`、`apiKey`；只保留最后一个，不以明文 Key 作存储键，不写入 `image-gen-history-v1`。Key 与模型偏好分开存储，选择模型不会触发 Key 存储的跨标签页事件。
- **保存条件**：当前 Key 最近一次获得**结构合法的成功目录响应**（实现为 `response.ok` 且目录结构合法，其后无失败、未改 Key）。200 但 HTML/错误结构不算，始终待保存。
- 修改 Key（用户编辑、密码管理器填入；恢复、清除等程序赋值不算）：不发请求、不删除已保存的旧 Key，勾选保持；显式加载成功后覆盖；改回已保存的值即恢复“已保存”。
- 覆盖写入失败时同时删除旧 Key，退出记住状态，不留旧值；提示“无法保存，当前仅本次页面有效”。
- 删除失败：内存仍清空；响应式 `keyRemovalFailed` 保留“未能清除浏览器中保存的 API Key，可能仍有残留”、“重试清除”和“清除已保存配置”入口（`hasSavedConfig` 含该状态）；编辑 Key、勾选/取消记住、清空输入都不解除；删除成功、新 Key 覆盖写入成功，或其他标签页删除/覆盖该记录后才解除。
- 配置损坏/未知版本：不恢复 Key、不自动请求；尽力删除，删除失败按上一条提示。
- 跨标签页只监听 `image-gen-key-v1`（含整体清空存储时 `key === null` 的事件），规则见 §7 表最后两行；不自动导入新 Key；模型偏好的跨页变化不影响当前选择。
- 安全边界：localStorage 不是加密保险箱，同源脚本（含 CSP 放行的第三方脚本）可读取；不用前端硬编码密钥“加密”。记住配置属于此浏览器，不绑定站内登录账号；站内退出登录不自动删除，界面说明并提供清除入口。

## 10. 并发、竞态与生成快照

| 发现状态 `ModelDiscoveryStatus` | 表现 |
| --- | --- |
| `idle` | 无 Key 或尚未加载（等待回车或点击“加载模型”）；不显示上一个 Key 的候选 |
| `loading` | 显示加载中；不阻止使用当前选择 |
| `ready` | 展示候选与选择；同 Key 刷新时保留现有候选并提示正在刷新 |
| `empty` | “当前 Key 的目录中没有图片模型”，提供重试和手动入口 |
| `error` | 显示 §11 错误；当前选择保留为未确认；提供重试和手动入口 |
| `disabled` | 端点 origin 不一致，仅手动模型 |

- 每次 Key 变化或发现请求替换都增加请求序号并取消旧请求。成功、失败、超时及 finally 都检查序号与 Key 快照；由目录结果引发的确认/清除选择和“成功后写入 Key”同样受此检查，写入前还要读取最新勾选状态。旧请求的 finally 不得释放新请求的在途标记。
- 生成：`generate()` 在第一个 await 前快照 `key/model/mode/prompt/size/count/files`，异步尾部不读活 ref；文生图 JSON、图改图 FormData、成功历史都用同一 `requestModel`（`activeRequestModel` 在当前任务区域标注本次模型）。
- 生成中切换 Key 或模型不修改已发出请求，也不自动重发；清除配置不取消已提交任务，不承诺撤销计费。
- 视图卸载不清空正在生成的任务；store 的 scope dispose 不等同视图卸载。
- objectURL 只在 remove/clear（及 `restoreHistory` → clear）时 revoke，组件卸载不得 revoke。
- store 单例消除了旧实现“卸载后旧实例整体覆写 localStorage、冲掉新实例增删”的竞态。

## 11. 错误处理

- `extractErrorMessage` 依次取 `error.message`、`message`，缺失时用状态码兜底；只展示 message，不展示原始响应正文；`extractErrorCode` 识别 `{ code, message }` 中的 code。
- **message 不是脱敏边界**：目录与生图两条路径在写入页面和 toast 前，用请求发起时的 Key 快照经 `redactCredential` 把回显替换为 `***`（`REDACTED_CREDENTIAL`），不能用当前输入值。本站鉴权中间件当前返回固定文案（如 `"Invalid API key"`），审计日志对凭据掩码；此规则防范外部网关或上游回显。
- `/v1/models` 与生图经过同一套前置检查（`backend/internal/server/middleware/api_key_auth.go`，未被 skipBilling 豁免），403/429 不能解释为“只是不能看目录”。

| 场景 | 典型 code | 处理 |
| --- | --- | --- |
| 401 | `INVALID_API_KEY`、`API_KEY_DISABLED`、`USER_NOT_FOUND`、`USER_INACTIVE` | 显示服务端信息；清空候选；不删除已保存 Key |
| 403 | `API_KEY_EXPIRED`、`INSUFFICIENT_BALANCE`、`SUBSCRIPTION_NOT_FOUND`、`SUBSCRIPTION_INVALID`、`ACCESS_DENIED`、`GROUP_DELETED`、`GROUP_DISABLED`、`GROUP_NOT_ALLOWED` | 显示服务端信息；生图同样会被拒；保留旧候选但未确认 |
| 429 | `API_KEY_QUOTA_EXHAUSTED`、`USAGE_LIMIT_EXCEEDED`、`INVALID_AUTH_RATE_LIMITED` | 显示服务端信息，不自动重试；`INVALID_AUTH_RATE_LIMITED` 按 `Retry-After` 提示等待秒数（跨域读不到时只提示稍后重试） |
| 其他 4xx | 端点配置错误导致 404/405 | 显示服务端信息或状态码，提示检查部署配置 |
| 超时、网络/CORS、5xx | 如 503 `API_KEY_AUTH_OVERLOADED` | “模型列表加载失败”附状态码，可重试或手动输入；不展示 5xx 正文 |
| 200 但非 JSON 或结构错误 | SPA 回落返回 HTML | “模型列表响应格式异常”，不能当作空列表；读完响应体后先判断超时，避免把超时误报为格式异常 |
| 生图失败 | — | 保留输入，显示服务端信息（如不支持 Images API 的平台返回 404），不自动换模型重试 |

无效 Key 计入按 IP 的无效鉴权限流（默认 60 秒 120 次，封禁 60 秒，429 `INVALID_AUTH_RATE_LIMITED`）。办公网络共享出口 IP 时，多人逐字输入产生的 401 会互相影响，这是“逐字输入不发请求”的依据之一。

## 12. 配置

| 项 | 位置 | 说明 |
| --- | --- | --- |
| `VITE_IMAGE_GENERATION_ENDPOINT` / `VITE_IMAGE_EDIT_ENDPOINT` / `VITE_IMAGE_MODELS_ENDPOINT` | 前端构建期变量 | 默认同源 `/v1/images/generations`、`/v1/images/edits`、`/v1/models`；三者 origin 必须一致。仓库无前端环境变量示例文件 |
| `gateway.models_list_cache_ttl_seconds` | 后端配置 | 目录缓存，默认 15，范围 10–30 |
| `gateway.max_body_size` | 后端配置 | 默认 256MB，约束前端总量上限 |
| CSP `img-src` | `DefaultCSPPolicy` / `requiredCSPDirectiveValues` | 须含 `blob:`（§6） |

localStorage 键：`image-gen-history-v1`（历史）、`image-gen-model-v1`（模型偏好）、`image-gen-key-v1`（显式记住的 Key）。

## 13. 代码地图

| 位置 | 内容 |
| --- | --- |
| `frontend/src/views/ImageGenView.vue` | 模板壳；`embedded`、`wrapperComponent`、`StandaloneShell`；`handleApiKeyEnter`（拦截 repeat/isComposing）、`handleRememberChange`、`confirmManualModel`、`handleSourceImageChange`、paste 监听 |
| `frontend/src/stores/imageGen.ts` | `useImageGenStore`；生成 `generate`/`createImages`/`editImages`；Key 记忆 `updateApiKey`/`setRememberKey`/`persistApiKey`/`removeSavedKey`/`retrySavedKeyRemoval`/`clearSavedConfig`/`handleStorageEvent`；模型 `selectModel`/`setManualModel`/`saveModelPreference`/`restorePreferences`；发现 `triggerModelDiscovery`/`loadModels`/`cancelDiscovery`/`resetDiscoveryForKeyChange`/`applyCatalog`/`applyDiscoveryFailure`/`applyDiscoveryHttpError`；原图 `addSourceImages`/`removeSourceImage`/`clearSourceImages`；历史 `loadHistory`/`saveHistory`/`deleteHistoryItem`/`clearHistory`/`restoreHistory`；`extractErrorMessage`/`extractErrorCode`/`normalizeCount`；常量 `MAX_SOURCE_IMAGES` 等 |
| `frontend/src/utils/imageGenModels.ts` | 纯函数：`isImageModelCandidate`、`isValidModelId`、`compareModelIds`、`parseImageModelCatalog`、`endpointsShareOrigin`、`redactCredential`、`read/write/removeModelPreference`、`read/write/removeSavedApiKey`；键常量 `MODEL_PREFERENCE_STORAGE_KEY`、`SAVED_API_KEY_STORAGE_KEY` |
| `frontend/src/router/index.ts` | `/image-gen` 路由与 `BACKEND_MODE_ALLOWED_PATHS` |
| `backend/internal/service/openai_images.go` | `validateCompatibleImagesModel`（前端候选判定的来源）、`parseOpenAIImagesMultipartRequest`、`openAIImageMaxUploadPartSize` |
| `backend/internal/server/middleware/security_headers.go`、`backend/internal/config/config.go` | `requiredCSPDirectiveValues`、`DefaultCSPPolicy` |
| `backend/internal/handler/gateway_handler.go`、`backend/internal/server/routes/gateway.go` | `/v1/models` 目录与 `/v1/images/*` 分发 |
| 测试 | `frontend/src/views/__tests__/ImageGenView.spec.ts`、`frontend/src/utils/__tests__/imageGenModels.spec.ts`、`frontend/src/stores/__tests__/imageGen.discovery.spec.ts`、`backend/internal/server/middleware/security_headers_test.go` |

## 14. 关键约束与易错点

1. **只有回车和“加载/刷新模型”按钮会把 Key 发往目录端点**（打开页面恢复已记住的 Key 时加载一次是唯一例外）。依据：密码管理器可能把本站登录密码填入密码型 Key 框并触发 input/change；`autocomplete="off"`、`new-password`、`event.isTrusted` 都不能可靠区分自动填充与用户输入；早期版本“失焦自动加载 + 编辑即删旧 Key”会把登录密码作为 Bearer 发出并删掉已保存 Key（审核 U1）。
2. **编辑 Key 不删除已保存的 Key**；新 Key 显式加载成功后才覆盖；覆盖写入失败必须同时删除旧值。依据：编辑即删会被自动填充误触发；不删旧值又引入“写失败残留旧 Key”路径。
3. **保存条件是“最近一次结构合法的成功目录响应”**，任何失败或改 Key 清除已验证标记。曾复现：某 Key 先 200、后刷新 401，再勾选记住会保存这个已被拒绝的 Key。
4. **删除失败必须保留残留警告与清除入口**，编辑/勾选/清空都不解除。曾复现：只记住 Key、未选模型时 Key 删除抛错、偏好删除成功 → 清除按钮消失但存储仍有 Key，刷新还会恢复。
5. **跨标签页退出记住时，若存储中仍是本页写入的值要一并删除**。曾复现：B 页删除已保存 Key 后，A 页迟到的目录成功响应在 B 的 storage 事件到达前写入；A 随后退出记住，两页都不记住但存储留有 Key 且无警告。写入前再读一次存储无法闭合此窗口（Chrome 中存储值与事件同时到达其他标签页）。
6. **错误展示前用请求快照 Key 脱敏**，两种错误结构都要覆盖。曾复现：mock `/v1/models` 401 的 message 回显 Key → 页面完整显示凭据。
7. **回车必须 `preventDefault`**（Key 框与手动模型框），测试断言 `defaultPrevented`，不能依赖“表单无 submit 按钮所以回车不提交”。依据：日后加入 submit 按钮或调整输入会误发付费生图；jsdom 不模拟隐式提交。
8. **候选判定镜像后端**：改 `validateCompatibleImagesModel`（及其依赖 `IsGPTImageGenerationModel`、`isGrokImageGenerationModel`、`isGeminiCompatibleImageModel`）必须同步 `isImageModelCandidate`；固定样例测试不会随 Go 判定变化自动失败。
9. **生成快照在第一个 await 前完成**，历史记录请求模型而非当前表单模型。
10. **objectURL 只在 remove/clear 时 revoke**，组件卸载不 revoke，否则切回页面缩略图全部失效。
11. **store 从文件模块 `./app` 导入 app store**，不走 `@/stores` index，否则 index → imageGen → index 循环依赖；测试因此只需 mock `@/stores/app`（解析到同一 module ID）。
12. **测试必须 `setActivePinia(createPinia())` 并显式释放 store**。依据：新建 Pinia 不等于 dispose 旧 store，窗口 storage listener 与悬挂定时器会跨用例存活。
13. 读完响应体后先判断超时再判格式。曾复现：超时发生在读取响应体期间被误报为“格式异常”。
14. 自动化中的 input/change/paste/blur 都是模拟 DOM 事件，只证明处理逻辑；Chrome 是否自动填充、触发哪些事件必须在浏览器验证。
15. 浏览器验收的 mock 必须隔离模型发现与生成请求，避免触发真实生图收费；真实 Key 目录读取和真实付费生图分开记录；“列表出现模型”不能写成“该模型所有图片能力已验证”。

已否决 / 未采纳：
- 已否决：KeepAlive 保持页面状态，原因见 §4。
- 已否决：生成前强制确认 Key（U1 的建议之一）。原因：改动前已是同样的密码框加 `autocomplete="off"`，点击生成直接用框内内容；生成是用户显式点击，不属于本功能新增的自动发送路径；强制确认给“填 Key → 写提示词 → 生成”多加一步。生成仍不保存、不替换已保存 Key。
- 已否决：无效鉴权限流后“暂停自动触发”。原因：已不存在自动触发，429 只提示 `Retry-After`。
- 未采纳：编辑 Key 时保留一次性提示（“无法保存…”“其他标签页修改了…”“Key 无效，未保存”）。原因：这些提示针对上一个 Key 或上一次操作，开始编辑后清掉符合预期；需要持续可见的残留风险由 `keyRemovalFailed` 承担。
- 已否决：按版本号或价格自动挑选默认模型，或在 API 不可用时回落写死模型。

## 15. 合同中有意保留的“未确认”

- `autocomplete="off"` 的密码框在正式站点（有本站已保存登录项的同源 Chrome）是否被自动填充，未验证（U1）。若确认会填充，再评估 `new-password` 或其他措施；代码层已保证填充不会发请求、不会删旧 Key。
- Grok、Gemini 模型对当前尺寸、数量、`b64_json` 参数的兼容性未验证；未验证前不得报告为已支持。

## 16. 验证

### 16.1 命令（仓库根目录）

```powershell
pnpm --dir frontend run typecheck
pnpm --dir frontend exec vitest run src/utils/__tests__/imageGenModels.spec.ts src/stores/__tests__/imageGen.discovery.spec.ts src/views/__tests__/ImageGenView.spec.ts
pnpm --dir frontend run lint:check
pnpm --dir frontend exec vitest run   # 全量，单独运行，不与 typecheck/lint/构建并行
pnpm --dir frontend run build
cd backend; go test ./internal/server/middleware/ -run SecurityHeaders
```

### 16.2 测试清单

| 要求 | 用例（`ImageGenView.spec.ts`，除非注明） |
| --- | --- |
| 两种错误结构回显 Key 不进页面/toast | `redacts the request key from catalog errors in both error shapes`；`imageGenModels.spec.ts` 的 `redactCredential` |
| 请求后切换 Key，旧请求失败仍按快照脱敏 | `redacts with the request key snapshot after the key changed mid-generation`；`ignores a late failure for a previous key while the next key is loading` |
| 仅保存 Key 时删除失败可重试 | `keeps a retry entry when clearing a key-only config fails to remove the key` |
| 删除失败警告不被编辑、重新勾选、空输入清掉 | `keeps the removal warning through edits, re-checking and empty input until an overwrite succeeds` |
| 模拟事件不发送草稿、不删旧 Key | `treats input, change, paste and blur as a draft: nothing is sent and the saved key is kept` |
| 显式确认才切换 Key；生成不保存 Key | `replaces the saved key only after an explicit load; generating uses the typed key without saving it`；store 测试 `disables discovery and saves the key only on explicit confirmation ...`（`imageGen.discovery.spec.ts`） |
| 跨标签页清除与迟到响应不写回 | `does not write the key back when another tab removes the saved key / clears all storage during an in-flight load`（含 `key=null`） |
| 状态持久化与多图 | 卸载重挂后表单/结果/历史保留；生成中卸载后 resolve → toast 恰一次、历史恰一条；`image[]` 顺序与单张移除（含 revoke 断言）；数量 12 钳制为 10；25MB 单张拒绝、6×18MB 触发总量护栏 |
| CSP | `security_headers_test.go` 的 `adds_blob_to_img_src_for_legacy_policies`、`does_not_duplicate_blob_in_img_src` |

测试 mock 策略：文件模块 mock `@/stores/app` + `@/stores/auth`，imageGen store 跑真实实现；`AppLayout` 被桩替换，只证明 wrapper 分支，不能证明真实侧栏与窄屏。

变异检查（2026-11-02/03）：先后注入 17 个缺陷（生图/目录不脱敏、用当前输入值脱敏、清除入口不含删除失败状态、编辑即删 Key、失焦触发加载、勾选立即请求、勾选清删除失败状态、迟到成功不查记住状态、覆盖失败不删旧 Key、生成后保存 Key，以及跨页解除警告、旧 finally 释放新在途标记、生成时写偏好、损坏配置删除失败提示、未勾选不提示“Key 无效”、跨页残留删除），每个都有用例失败，源文件逐字节恢复。

### 16.3 最近结果

2026-11-03（提交前，仓库根目录依次执行）：typecheck、lint:check 通过；三个专项 63 用例通过（11 + 3 + 49）；前端全量 423 文件 / 3454 用例通过；build 通过（仅既有分块提示，产物在已忽略的 `backend/internal/web/dist`）。未调用真实模型或生图接口。本次文档合并未重跑。

受控浏览器验收（2026-11-02，iab 与已连接本地 Chrome，`/api/*`、`/v1/*` 全部 mock，未消耗真实额度）：独立页与登录布局、390px 窄屏、粘贴/回车/手动确认、记住与刷新恢复、跨标签页清除、401/403/429+Retry-After/503/200 HTML/空目录/10 秒超时均符合当时规则。这轮验收早于最终修复（失焦已不再触发、删除失败警告），需按 §7 复验。

## 17. 状态与待办

- [ ] Codex 对最终修复的独立审核，按 §7 表逐项走查。
- [ ] 浏览器复验：回车/按钮加载、勾选不请求、编辑后“之前的 Key”提示、删除失败警告与重试（需模拟存储故障）。
- [ ] U1：在具备本站已保存登录项的正式同源 Chrome 中验证，请求必须隔离。
- [ ] 真实 Key 目录读取、真实文生图/图改图，以及 Grok、Gemini 参数兼容性（需另行授权）。
- [ ] 未补的自动化：`Retry-After` 缺失/跨域不可见、图改图中途同时改 Key 与模型的快照、非法手动草稿的 UI 流程、真实 HTTP 重定向拒绝。
- [ ] 待决定：后端单分片上限已是 50MB 且超限报错（§5），前端 `MAX_SOURCE_IMAGE_SIZE` 是否从 20MB 放宽，以及同步更新 `imageGen.ts` 中的过期注释。
- [ ] 尺寸选项 1792x1024/1024x1792 是否收敛（§5）。
