# 项目归因线索待确认清单（2026-11-03）

本轮只读采集，不修改线上归因结果。用户给出的后端结构：`192.168.1.27 / 服务 / 项目 / 文件`；例如 `micro-activity / battlepass / README.md`。其他映射等待用户定义。

## 范围与验证

| 项目 | 结果 |
| --- | --- |
| 时间窗口 | 2026-11-02 00:00–2026-11-03 00:00，Asia/Shanghai |
| 原始文件 | 231,243,156,719 bytes（215.36 GiB）；扫描前后 size/mtime 不变 |
| 已索引请求 | 68,019；未归因 2,690（3.95%），涉及 46 个用户 |
| 读取范围 | 全部 2,690 条未归因请求，逐条校验 archive_id 与 request 事件；不是全文件全量审计 |
| 读取成本 | 按行末 source_offset 倒读；读取约 4.12 GiB，目标请求行合计约 2.60 GiB，135 秒 |
| 原文形态 | 289 条含 tagged_files；32 条正文不是可解析 JSON，采用原始文本搜索；归档 body_truncated 标记均为 false |
| 路径初筛 | 2,556 条出现宽泛路径/URL 线索，包含示例、工具与运行时路径，不能解释为 2,556 条可准确归因 |
| 候选整理 | 43 组候选；内部 Go 模块缓存出现于 282 条请求，单独标注 |
| 验证 | 倒读的首行、多块大行、末行 3 项本地边界检查通过；线上与本地 project_attribution.go SHA-256 相同 |

每行的“未归因线索请求”按 archive_id 去重；同一请求可提及多个候选，各行不能相加。对话历史会重复携带路径，出现次数不等于独立会话、人工操作数、项目用量或可回填条数。已归因列只核对当天现有工作目录记录。

“目录标记”可能来自当前环境、历史环境或引用；“附件”也可能是依赖文件。必须结合位置、角色、主工作区与多仓库冲突规则后才能自动归因。

## 后端

| 编号 | 发现的路径/仓库线索 | 未归因线索请求 | 目录标记 / 附件 / 仓库地址 | 当前已归因 | 待确认规则 |
| --- | --- | ---: | ---: | ---: | --- |
| B01 | `192.168.1.27 / micro-activity / battlepass` | 217 | 212 / 183 / 0 | 0 | 按用户给出的结构，项目为 battlepass；是否用服务/项目作为唯一标识？ |
| B02 | `micro-activity / condition` | 10 | 8 / 8 / 0 | 388 | condition 是否为独立项目？ |
| B03 | `micro-activity / modules` | 16 | 15 / 0 / 0 | 0 | 公共模块仓库是否作为独立项目统计？依赖缓存另列，不并入此行。 |
| B04 | `micro-perm / modules` | 202 | 0 / 0 / 117 | 0 | 是否单列公共权限模块？ |
| B05 | `micro-perm / login-server` | 161 | 0 / 0 / 0 | 0 | 是否单列登录服务项目？ |
| B06 | `micro-manager / perm-server；E:/micro/perm-server` | 207 | 207 / 0 / 74 | 0 | 本地短路径是否与此远程仓库合并？ |
| B07 | `microservice / gateway` | 201 | 0 / 0 / 75 | 0 | microservice 是否也遵循服务/项目两层结构？ |
| B08 | `micro-manager / activity-server` | 68 | 0 / 0 / 11 | 0 | 是否独立活动服务项目？ |
| B09 | `micro-manager / marketing-agency` | 68 | 0 / 0 / 0 | 41 | 是否与下列客户端目录分开统计？ |
| B10 | `micro-user / personal-center` | 15 | 0 / 0 / 0 | 0 | 正文提及路径，项目正式名是什么？ |
| B11 | `micro-manager / version-release` | 0 | 0 / 0 / 0 | 167 | 当前已归因 version-release，是否保留服务层？ |
| B12 | `micro-manager / version-deploy` | 0 | 0 / 0 / 0 | 62 | 当前已归因 version-deploy，是否保留服务层？ |
| B13 | `micro-business / kol-oversea` | 0 | 0 / 0 / 0 | 77 | 当前已归因 kol-oversea，是否保留服务层？ |
| B14 | `micro-activity / marketing-v3` | 0 | 0 / 0 / 0 | 60 | 当前已归因 marketing-v3，是否保留服务层？ |
| B15 | `micro-user / authtype-phone` | 1 | 0 / 0 / 0 | 0 | 低频正文提及，是否纳入正式项目字典？ |
| B16 | `micro-user / internalserver` | 1 | 0 / 0 / 0 | 0 | 低频正文提及，是否纳入正式项目字典？ |

## 其他目录

| 编号 | 发现的路径/仓库线索 | 未归因线索请求 | 目录标记 / 附件 / 仓库地址 | 当前已归因 | 待确认规则 |
| --- | --- | ---: | ---: | ---: | --- |
| O01 | `E:/10.13；F:/police/dataset；F:/police/model` | 417 | 417 / 0 / 0 | 0 | 这些工作区、数据集和模型目录的正式项目名及合并关系？ |
| O02 | `PycharmProjects/WelcomeScreen（Windows / macOS）` | 228 | 227 / 0 / 0 | 0 | WelcomeScreen 是真实项目名还是 IDE 默认目录？两平台是否同项目？ |
| O03 | `D:/AI_project/compass/desktop/framework` | 260 | 215 / 0 / 0 | 0 | 统计为 compass、desktop，还是 framework？src-tauri 是否并入？ |
| O04 | `compass/component/process` | 315 | 0 / 0 / 187 | 0 | 独立仓库是否单列，还是归入 compass？ |
| O05 | `compass/component/screen-perception` | 187 | 0 / 0 / 187 | 0 | 独立仓库是否单列，还是归入 compass？ |
| O06 | `compass/runtime/llm-core` | 325 | 0 / 0 / 187 | 0 | 独立仓库是否单列，还是归入 compass？ |
| O07 | `D:/AI_project/atlas/deploy` | 401 | 215 / 0 / 0 | 0 | deploy 是否只是 atlas 的部署子仓库？ |
| O08 | `D:/AI_project/atlas/manager-service` | 260 | 0 / 0 / 0 | 0 | 按 atlas 汇总还是单列 manager-service？ |
| O09 | `D:/AI_project/atlas/resource-service` | 101 | 0 / 0 / 0 | 0 | 按 atlas 汇总还是单列 resource-service？ |
| O10 | `atlas/service_sdk` | 138 | 0 / 0 / 34 | 0 | 公共 SDK 单列还是归入 atlas？ |
| O11 | `D:/AI_project/workshop/interactive-map` | 468 | 155 / 0 / 0 | 0 | editor_service、web、plugin 是一个项目还是多个？ |
| O12 | `D:/AI_project/workshop/game-guide` | 313 | 0 / 0 / 0 | 0 | 是否独立项目？是否属于 compass 产品？ |
| O13 | `D:/AI_project/workshop/game-manager` | 313 | 0 / 0 / 0 | 0 | 是否独立项目？是否属于 compass 产品？ |
| O14 | `D:/AI_project/workshop/mod-manager` | 186 | 0 / 0 / 0 | 0 | 是否独立项目？是否属于 compass 产品？ |
| O15 | `D:/AI_project/tools/repo-deck` | 239 | 0 / 0 / 0 | 0 | 工具自身作为独立项目，还是只算被管理项目？ |
| O16 | `/Users/<user>/code/python/gitlab_tool` | 70 | 69 / 69 / 0 | 0 | 单列 gitlab_tool，还是归属某个业务项目？ |
| O17 | `xy-data-router/xunyou-datarouter` | 86 | 86 / 0 / 0 | 571 | src、cgo 与构建子目录是否统一归到 xunyou-datarouter？ |
| O18 | `E:/allsite/xyai；E:/tmp/imagegen-review-fixes` | 198 | 197 / 0 / 0 | 138 | 临时审核副本是否并入 xyai？正文提及其他项目时如何取主项目？ |
| O19 | `.codex/worktrees/3b88/client-web-page；orig-web/orig.client-web-page` | 36 | 36 / 0 / 36 | 0 | worktree 和 orig. 前缀是否统一为 client-web-page？ |
| O20 | `orig-web/orig.xyweb-lib` | 36 | 0 / 0 / 36 | 0 | 公共组件库是否单列 xyweb-lib？ |
| O21 | `C:/Work/Web/orig-lagz/orig.www.lagzapper.com` | 35 | 35 / 0 / 0 | 232 | 是否统一为 www.lagzapper.com？ |
| O22 | `orig-web/orig.cddada7d.xunyou.com；E:/orig-web/cddada7d.xunyou.com` | 207 | 0 / 0 / 40 | 0 | 是否同一项目？正式统计名称是什么？ |
| O23 | `D:/Go/src/marketing-client；marketing-client-component` | 68 | 0 / 0 / 0 | 0 | 两个客户端是替代关系还是独立项目？是否与 marketing-agency 合并？ |
| O24 | `192.168.1.23/beyond/g` | 207 | 0 / 0 / 207 | 0 | 当前样本主要为规范文档引用，是否作为规则背景而不参与归因？ |
| O25 | `C:/Users/<user>/Documents/Codex/2026-10-15/wom` | 8 | 8 / 0 / 0 | 0 | 正式项目名是什么？日期父目录应忽略？ |
| O26 | `Documents/海外；Documents/ChatGPT/阿卡贝拉Game；Documents/testcase` | 8 | 7 / 0 / 0 | 164 | 这些是否业务项目、一次性工作，或应继续未归因？ |
| O27 | `e:/tmp` | 198 | 197 / 0 / 0 | 0 | 临时工作区必须依赖哪个明确仓库证据，才能合并到真实项目？ |

## 实际出现的仓库地址

以下地址来自请求中的仓库线索；出现并不证明它就是本次请求的主项目。SSH 用户部分已省略。

| 关联编号 | 地址 |
| --- | --- |
| B04 | `http://192.168.1.27/micro-perm/modules.git` |
| B06 | `http://192.168.1.27/micro-manager/perm-server.git` |
| B07 | `http://192.168.1.27/microservice/gateway.git` |
| O19 | `http://192.168.1.23/orig-web/orig.client-web-page.git` |
| O20 | `http://192.168.1.23/orig-web/orig.xyweb-lib.git` |
| O22 | `http://192.168.1.23/orig-web/orig.cddada7d.xunyou.com.git` |
| O04 | `ssh://192.168.8.6:30102/compass/component/process.git` |
| O05 | `ssh://192.168.8.6:30102/compass/component/screen-perception.git` |
| O06 | `ssh://192.168.8.6:30102/compass/runtime/llm-core.git` |
| O10 | `ssh://192.168.8.6:30102/atlas/service_sdk.git` |

## 已识别的误归因风险

| 形态 | 本轮证据 | 需要的规则 |
| --- | --- | --- |
| Go 依赖缓存 | `.../go/pkg/mod/192.168.1.27/micro-activity/modules/v2@v2.44.0/...` 与 battlepass 同时出现 | 保留为依赖证据；不能仅因路径出现就把主项目改成 modules。修改依赖源码时是否另计需定义。 |
| 同会话多仓库 | compass / atlas / workshop，以及 perm-server / modules / gateway 同时出现 | 定义单主项目、拆分或多项目；不能默认取第一条或最高频路径。 |
| worktree / 临时副本 | `.codex/worktrees/.../client-web-page`、`E:/tmp/imagegen-review-fixes` | 用可验证的仓库映射合并；避免把哈希、tmp 或临时目录名作为正式项目。 |
| 项目内子目录 | framework/src-tauri、editor_service/web、xunyou-datarouter/src | 回到确认的项目根；不能直接以当前 basename 作项目名。 |
| 通用规范/基础设施 | beyond/g 文档、包仓库、登录页、API/部署地址 | 本轮不把这些普通提及当作当前项目。 |

## 现有提取器的实证限制

- 没有专门解析 tagged_files 的分支；Copilot 的 POSIX 路径兜底主要依赖 filePath 属性，普通 macOS 附件路径存在缺口。
- Codex 仅扫描请求体前 300 KiB：两个未归因见证的首个 cwd 分别位于 393,193 和 532,258 字节，超出窗口（shape-check.json）。
- Claude 两个未归因见证的正文携带 Primary working directory，但顶层 system 文本块未含此标记；不能把后续实现简化为只扩展顶层 system 正则（system-shape-check.json）。
- 已归因项目直接取 cwd basename，因此日期、任务名、构建子目录也可能成为项目名；识别命中不等于业务归因正确。

## 复查文件与后续

- `candidates.json`：每组候选、计数、代表路径、archive_id 清单与现有已归因目录。
- `evidence.json`：请求编号、行末偏移、哈希、候选编号与每种证据最多 4 条路径见证；个人 home 目录用户名被遮蔽；不含原始 prompt/正文。
- `collect.py`：在服务器通过 stdin 执行的只读采集器，数据库使用 BEGIN READ ONLY，低 CPU/IO 优先级读取归档；仅向 stdout 输出提取结果。
- `summarize.py`：本地候选整理器；其分组只是待确认问题，不是线上规则。
- 待用户按编号确认项目名、层级、别名与多仓库策略，再设计提取/归一化规则和受控历史回填。
- 本轮没有部署、重索引、生产数据库写入、业务代码修改或提交；没有浏览器验证。wiki 仅补充当前解析能力边界。

采集器原始输出应先写入本地临时 `collected.json`，再运行 `python summarize.py <collected.json>`。整理后的 `evidence.json` 已限制见证数量，不可当完整采集输入重跑聚合。临时宽泛路径结果不保留。`gitlab-tool-recheck.json` 是修正本地分组前缀后，对涉及用户的 319 条请求定向复核所得 70 条路径见证；没有线上写入。
