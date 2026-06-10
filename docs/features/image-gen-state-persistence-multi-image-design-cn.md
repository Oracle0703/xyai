# 图片生成页:状态持久化(Pinia)+ 预览 CSP 修复 + 多图输入设计

> 2026-06-10。关联首版设计: `docs/superpowers/specs/2026-05-24-image-gen-tool-design.md`(公开整页工具)与后续"登录用户嵌入 AppLayout"改造。

## 背景

`/image-gen` 页面(`frontend/src/views/ImageGenView.vue`)实际使用中暴露四个问题:

1. **切菜单丢一切**。本项目没有嵌套布局路由: `App.vue` 只有一个裸 `RouterView`, 38 个视图各自内部包一份 `AppLayout`, 任何侧边栏导航都会把整个视图卸载重建。而 API Key、提示词、原图、生成结果、"生成中"标志全部是组件内 `ref`, 卸载即清零。生成请求(原生 `fetch`)虽然不会被卸载打断, 但完成结果写进的是旧实例的闭包, 用户切回来挂载的是新实例, 看不到结果; 历史记录要等"再切走再切回"触发第二次 remount 才能从 localStorage 读到。
2. **图改图原图预览生产环境不显示**。预览用 `URL.createObjectURL` 生成 `blob:` 地址, 而后端 CSP 默认策略 `img-src 'self' data: https:` 没放行 `blob:`, 浏览器静默拦截。dev 模式 vite server 不发 CSP 头, 所以本地永远复现不了。
3. **历史生图单列展示浪费空间**。
4. **不支持多图输入**。官方 gpt-image `/v1/images/edits` 支持 `image[]` 数组(最多 16 张, png/webp/jpg)基于多张原图改图, `n` 支持 1~10; 页面只能传一张, 数量上限 4。

## 产品决策

| 决策点 | 结论 |
| --- | --- |
| 状态保持方案 | **Pinia store 而非 KeepAlive**。KeepAlive 会把整棵 AppLayout 子树缓存第二份(本项目布局在视图内部), 全局 paste 监听器需手工管理停用态, include 白名单字符串耦合易静默失效; store 方案生成任务与组件生命周期彻底解耦, 还能跨页 toast 通知 |
| API Key 持久化 | **绝不落盘**(页面明确承诺"不会保存 API Key"), 只在 store 内存里; F5 刷新丢失是接受的边界 |
| 多图上限 | 16 张(官方上限); 单张 ≤20MB(**保命限制**, 见下); 总量 ≤100MB(网关 `gateway.max_body_size` 默认 256MB, 留余量避免 413) |
| 生成数量 n | 上限 4 → 10(官方区间 1~10; 计费按上游实际返回张数, 多要少给不会多扣) |
| CSP 修复方式 | 默认串 + `requiredCSPDirectiveValues` retrofit 双修, 自定义了旧 policy 的存量部署运行时自动补上, 不需要手改生产 config.yaml |
| 生成完成提醒 | store 成功/失败都调 app store 全局 toast, 用户切到任何页面都能收到 |

## 方案

### 1. 状态与生成逻辑搬入 `frontend/src/stores/imageGen.ts`

setup 风格 store(与 `app.ts` 同款), 持有: 表单字段(apiKey/prompt/size/count/generationMode)、`sourceImages: {id, file, previewUrl}[]`、`results`、`historyItems`、`isGenerating`、`errorMessage`。视图瘦身为模板壳: `storeToRefs` 绑状态, action 直接解构; 只保留 `showKey`、文件 input DOM ref、纯 UI helper 和 window paste 监听器(onMounted 注册/onBeforeUnmount 移除, 委托给 store 的 `addSourceImages`)。

关键实现约束:

- **`generate()` 在第一个 await 前快照全部请求参数**(files/mode/size/n), 异步尾部不读活 ref——生成期间用户切页或继续改表单不会污染请求与历史记录。
- **objectURL 只在 remove/clear(及 restoreHistory→clear)时 revoke, 组件卸载不得 revoke**, 否则切回页面缩略图全部失效。
- store 内部 `import { useAppStore } from './app'`(文件模块, 不走 `@/stores` index), 避免 index→imageGen→index 循环依赖, 同时让测试只 mock `@/stores/app` 一个模块即可同时覆盖视图与 store(解析到同一 module ID)。
- FormData `append('image[]', file)` **不传 filename 参数**: File 自带 name, 显式传参会按 WHATWG 规范重新包一层 File 对象(浏览器与 jsdom 行为一致), 破坏测试同一性断言也无意义。
- 历史仍是 localStorage key `image-gen-history-v1`, 最多 20 条, 只在生成成功后写入; store 单例消除了旧实现"卸载后旧实例整体覆写 localStorage、冲掉新实例增删"的竞态。

### 2. CSP `img-src` 放行 `blob:`(后端, 需部署生效)

- `backend/internal/config/config.go` `DefaultCSPPolicy`: `img-src 'self' data: https:` → `img-src 'self' data: blob: https:`。
- `backend/internal/server/middleware/security_headers.go` `requiredCSPDirectiveValues` 追加 `{"img-src", "blob:"}`——该列表本来就是"旧配置缺新指令"的运行时补丁机制(支付 SDK 域名同款), 自定义 policy 的部署也会被补上。
- `deploy/config.example.yaml` 示例串同步。

`blob:` 仅允许同源脚本自己创建的对象 URL, 不引入远程来源, 安全面无扩大。

### 3. 多图输入(图改图)

- dropzone: input 加 `multiple` + `accept="image/png,image/jpeg,image/webp"`, 选择/拖拽/粘贴统一走 `addSourceImages` 追加; 缩略图网格 + 单张移除 + 全部移除 + "已选 N/16 张 · 共 X MB"计数。
- 校验顺序: 类型 → 单张 20MB → 16 张 → 100MB 总量, 有拒绝给首个原因, 全通过清错误。
- **20MB 单张上限不可放宽**: 网关 `parseOpenAIImagesMultipartRequest` 对每个 multipart 分片用 `io.LimitReader` 在 `openAIImageMaxUploadPartSize`(20MB)处**静默截断**, 超限文件会变成损坏图片送往上游而不是报错。
- 网关侧零改动: multipart 解析已收集 `image` / `image[` 前缀全部字段, model 重写时逐 part 原样复制转发。

### 4. 历史生图两列

列表容器 `space-y-3` → `grid grid-cols-1 items-start gap-3 md:grid-cols-2`, 窄屏回落单列, 卡片内部结构不变。

## 测试

`frontend/src/views/__tests__/ImageGenView.spec.ts` 从 5 个用例扩到 11 个。mock 策略从"整体 mock `@/stores`"换成"文件模块 mock `@/stores/app` + `@/stores/auth`, imageGen store 跑真实实现"(先例: `ApiKeyCreate.spec.ts`); `beforeEach` 必须 `setActivePinia(createPinia())`, 否则 store 单例跨用例漏状态。新增覆盖:

- 卸载重挂后表单值/结果/历史保留;
- 生成中卸载、fetch 之后 resolve → toast 恰一次、localStorage 历史恰一条、重挂可见结果且按钮恢复可用;
- 多图 `image[]` 提交顺序与单张移除(含 revokeObjectURL 断言);
- 数量 12 钳制为 n=10;
- 25MB 单张拒绝(提示 20MB)与 6×18MB 触发 100MB 总量护栏(5 张通过)。

后端: `security_headers_test.go` 补两个 enhance 子测试(旧策略补出 `img-src blob:`、已含不重复)。

## 边界与运维注意

- **CSP 修复必须部署后端才生效**; 部署后 `curl -I` 任意非 API 页面确认响应头 `img-src` 含 `blob:`。
- F5 刷新浏览器仍丢全部内存态(含进行中的生成展示, 请求本身已发出、计费照常), 这是方案边界, 不是回归。
- 单条历史最多 10 张 data URL, localStorage 配额超限时 `saveHistory` 已 try/catch, 生成结果仍可用只是历史不持久。
- 尺寸选项保留了既有的 1792x1024/1024x1792(官方 gpt-image 仅支持 1024x1024/1536x1024/1024x1536/auto), 本次未动, 是否收敛由上游通道实际支持情况决定。
