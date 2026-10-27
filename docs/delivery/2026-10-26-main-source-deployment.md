# 118 main 源码部署记录

日期：2026-10-26，时区 Asia/Shanghai。目标：`118.25.112.30`，项目目录 `/opt/sub2api`。按用户之前给定的流程执行，仅部署远端 main。

| 项目 | 结果 |
| --- | --- |
| 部署前提交 | `777960134bcb94173ddcf7c6a62fd5874ed09901` |
| 部署后提交 | `25bedc620d9783e065ec6efae45d56f984b6b01c`，main 与 origin/main 一致 |
| 更新方式 | fetch origin main、checkout main、pull --ff-only origin main |
| 本次增量 | 3 个提交，子管理员账号操作员能力、额度展示文案与相关文档；无 migration 或依赖锁文件变更 |
| 工作区 | 部署前、构建后、重启后均干净 |
| 前端 | frozen-lockfile 安装、i18n 3 项、vue-tsc、Vite production build 通过（1179 modules） |
| 后端 | CGO_ENABLED=0，embed，按指定 ldflags/trimpath 构建；Version=0.2.17 |
| 二进制 | sub2api:sub2api，0750，127127712 bytes |
| 二进制 SHA-256 | `6b2c01f1b0cad22b009b23c29c79e144f17c9f1021648d460cfb7257508b1753` |
| 应用重启 | 仅一次 systemctl restart sub2api；09:55:27 启动，09:55:28 开始监听 52997 |
| 运行状态 | active/running，PID 3522741，NRestarts=0；运行中二进制与磁盘 SHA-256 相同 |
| 健康检查 | 本机及公网 /health 返回 HTTP 200 / status=ok |
| 页面入口 | /admin/token-analysis、/admin/accounts、/admin/gpt-quota 均返回 HTTP 200 |
| 资源一致性 | TokenAnalysisView-Yeygr-MV.js、AccountsView-DEfrXc1j.js、GPTQuotaDisplayView-FHUol6It.js 的线上内容与本次构建逐字节一致 |
| 启动日志 | 至 09:55:51 检查当前进程日志，未发现 ERROR/FATAL/panic；有既有 trusted_proxies/CORS 配置警告 |

## 服务与权限边界

| 服务 | 部署前 PID | 部署后 PID |
| --- | --- | --- |
| sub2api | 2760022 | 3522741 |
| Redis | 33882 | 33882 |
| PostgreSQL 15 main | 35193 | 35193 |
| PostgreSQL 15 portal | 4002336 | 4002336 |
| Nginx | 989556 | 989556 |

除 sub2api 外未重启其他服务；上述基础服务启动时间也保持不变，Docker 服务未安装。未手工执行 SQL，迁移检查仅由应用启动流程处理。未更改运行配置、分配权限、创建生产测试账号或触发历史重索引。

本次验证为构建、启动、健康与静态资源一致性检查，不替代新角色权限的真实业务验收。未用生产账号执行新增、删除、导入或其它业务写操作。

执行脚本以标准输入传给 bash，过滤 Windows CR；首页前五行仅限制输出长度，完整响应仍被读取。前端的 vue-demi 安装脚本提示、Browserslist 和大 chunk 提示未导致构建失败；未升级工具或改变依赖批准配置。

本机原始执行日志：`E:/tmp/origin-audit/deployment-2026-10-26-preflight.log`、`deployment-2026-10-26-build.log`、`deployment-2026-10-26-verify.log`、`deployment-2026-10-26-final-check.log`。本记录和 wiki 更新仅在本地，未另行提交或推送。
