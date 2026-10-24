# 118 Token Analysis 关联修复部署记录

日期：2026-10-25，时区 Asia/Shanghai。目标：`118.25.112.30`，项目目录 `/opt/sub2api`。按用户提供的源码部署流程执行，只部署远端 main。

| 项目 | 实际结果 |
| --- | --- |
| 部署前提交 | `98151012b1ce107fbbbbc67d93aafd08aa40bbf6` |
| 部署后提交 | `777960134bcb94173ddcf7c6a62fd5874ed09901`，main 与 origin/main 一致 |
| 更新方式 | `git fetch origin main`、`git checkout main`、`git pull --ff-only origin main` |
| 工作树 | 部署前、构建后及重启后均干净 |
| 审核基线 | 8 份关键源码的 SHA-256 与第四次独立复核快照一致；本机与服务器各核验一次 |
| 前端 | frozen-lockfile 安装、i18n 3 项、vue-tsc、Vite production build 成功 |
| 后端 | `CGO_ENABLED=0 go build -tags embed`，带用户指定 ldflags/trimpath，Version=0.2.17 |
| 二进制 | `sub2api:sub2api`，0750，127041696 bytes |
| 二进制 SHA-256 | `be1e5e61233ce6fc1b4022bd680e83d41ac5dfb0e1606979d12de21c80a52fee` |
| 应用重启 | 只执行一次 `systemctl restart sub2api`；01:40:06 新进程启动，01:40:42 开始监听 |
| 运行状态 | active/running，PID 2760022，NRestarts=0 |
| 运行文件一致性 | `/proc/2760022/exe` 与磁盘 `backend/bin/server` SHA-256 相同 |
| 健康检查 | 服务器本机及公网 `/health` 均返回 HTTP 200 / `{"status":"ok"}` |
| 页面入口 | `/` 与 `/admin/token-analysis` 均返回 HTTP 200 |
| 页面资源 | `TokenAnalysisView-DYvmtAgR.js`，75039 bytes，与本次构建逐字节一致，包含 usage_link_status |
| 页面资源 SHA-256 | `14a91bed5dbdb3798703a63f13b07c86982204e474901756840f2f857ffaf803` |
| 日志 | 核验窗口至 01:41:35，未发现 ERROR/FATAL；启动时有既有 trusted_proxies/CORS 配置警告 |

## 服务边界

| 服务 | 部署前 PID | 部署后 PID | 结论 |
| --- | --- | --- | --- |
| sub2api | 2483694 | 2760022 | 按授权重启 |
| Redis | 33882 | 33882 | 未重启 |
| PostgreSQL 15 main | 35193 | 35193 | 未重启 |
| PostgreSQL 15 portal | 4002336 | 4002336 | 未重启 |
| Nginx | 989556 | 989556 | 未重启 |
| Docker | 未安装服务 | 未安装服务 | 未操作 |

未手工执行任何 SQL；migration 仅随应用启动流程处理。未改变配置、未重置索引水位、未手动触发历史重索引，也未发送人工构造的模型请求。

正常自动索引由应用既有流程运行。本次验证确认服务和新页面资源已上线，没有将旧进程退出时的 `TokenAnalysisAutoIndex succeeded` 清理日志当作新版本索引完成的证据；具体历史请求关联是否已恢复仍需独立核对。

## 执行与证据说明

构建脚本已完成前后端构建、权限设置、二进制校验及工作树检查，随后因 Windows 管道附加的空白 CR 行返回 127；不是构建器失败。后续脚本入口过滤 CR，重新校验工作树/分支/审核源码后才重启应用，重启及完整验证脚本返回 0，没有重复构建或重复重启。

就绪验证使用带重试的健康检查，等待应用完成启动；首页输出用读取完整响应的 sed 展示前五行，避免 pipefail 下 head 提前关闭管道造成误报。其余更新、构建、权限和服务操作沿用户给定流程。

本机执行日志保存在 `E:/tmp/origin-audit/`：

- `deployment-2026-10-25-preflight.log`
- `deployment-2026-10-25-build.log`
- `deployment-2026-10-25-verify.log`
- `deployment-2026-10-25-final-check.log`

以上日志不上传生产、不提交原始请求或凭据。当前部署记录及 wiki 更新仅在本地工作区，未额外提交或推送。

## 新测试消息的线上验证

用户发送“测试一下是否修改成功,这是一条测试的信息”后，复用右侧已登录浏览器并只读核对归档。首发请求 `d5a496e050ad7291aa0c2f4f8f40eece`，开始 01:43:07、完成 01:43:13、状态 200，归档尾项为 user/message。

- 页面“本次调用来源”实际显示“人工”，判断依据为本轮末尾包含新用户输入。
- 预览和用户输入全文只保留测试正文，已去除前置浏览器上下文包装。
- “排行含未关联请求”未勾选、勾选两种状态均可见同一条人工请求；检查后已恢复未勾选，并将详情留在右侧供用户查看。
- 同轮后续工具续调用显示 AI，预览仍是相同用户正文，符合本次调用来源的合同。
- 该请求在 01:50:43 水位推进后出现在列表；观察期间自动索引仍运行，详情仍为“未关联计费”，Token/费用显示未知而非零。本次只能确认来源、预览和可见性通过，计费关联及原历史样本恢复尚未验收。

归档只读查找有 64 MiB 尾部窗口上限（文件快照 1,058,404,686 bytes）；命中请求偏移 1,042,723,437、行长度 1,200,953、SHA-256 `12a707379d3f121dc3fe6b77b6acadd5eb79ee254e8a67c71d0ae7c59fae5dde`。本机最小证据：`E:/tmp/origin-audit/production-test-2026-10-25-archive.json`；截图：`E:/tmp/origin-audit/production-test-2026-10-25-human.jpg`。未执行 SQL、未手动触发索引、未更改服务。
