"""Curate survey hints into a review list; candidates are not attribution decisions."""
import collections
import hashlib
import json
import pathlib
import re
import sys

BASE = pathlib.Path(__file__).parent
SOURCE = pathlib.Path(sys.argv[1]) if len(sys.argv) > 1 else BASE / "collected.json"
D = json.loads(SOURCE.read_text(encoding="utf-8"))
if "curation" in D:
    raise SystemExit("Use fresh collector output, not the bounded curated evidence.json.")

# id, group, label, boundary-matched path fragments, question
CANDIDATES = [
    ("B01", "后端", "192.168.1.27 / micro-activity / battlepass", ["micro-activity/battlepass"], "按用户给出的结构，项目为 battlepass；是否用服务/项目作为唯一标识？"),
    ("B02", "后端", "micro-activity / condition", ["micro-activity/condition"], "condition 是否为独立项目？"),
    ("B03", "后端", "micro-activity / modules", ["micro-activity/modules"], "公共模块仓库是否作为独立项目统计？依赖缓存另列，不并入此行。"),
    ("B04", "后端", "micro-perm / modules", ["micro-perm/modules"], "是否单列公共权限模块？"),
    ("B05", "后端", "micro-perm / login-server", ["micro-perm/login-server"], "是否单列登录服务项目？"),
    ("B06", "后端", "micro-manager / perm-server；E:/micro/perm-server", ["micro-manager/perm-server", "e:/micro/perm-server"], "本地短路径是否与此远程仓库合并？"),
    ("B07", "后端", "microservice / gateway", ["microservice/gateway"], "microservice 是否也遵循服务/项目两层结构？"),
    ("B08", "后端", "micro-manager / activity-server", ["micro-manager/activity-server"], "是否独立活动服务项目？"),
    ("B09", "后端", "micro-manager / marketing-agency", ["micro-manager/marketing-agency"], "是否与下列客户端目录分开统计？"),
    ("B10", "后端", "micro-user / personal-center", ["micro-user/personal-center"], "正文提及路径，项目正式名是什么？"),
    ("B11", "后端", "micro-manager / version-release", ["micro-manager/version-release"], "当前已归因 version-release，是否保留服务层？"),
    ("B12", "后端", "micro-manager / version-deploy", ["micro-manager/version-deploy"], "当前已归因 version-deploy，是否保留服务层？"),
    ("B13", "后端", "micro-business / kol-oversea", ["micro-business/kol-oversea"], "当前已归因 kol-oversea，是否保留服务层？"),
    ("B14", "后端", "micro-activity / marketing-v3", ["micro-activity/marketing-v3"], "当前已归因 marketing-v3，是否保留服务层？"),
    ("B15", "后端", "micro-user / authtype-phone", ["micro-user/authtype-phone"], "低频正文提及，是否纳入正式项目字典？"),
    ("B16", "后端", "micro-user / internalserver", ["micro-user/internalserver"], "低频正文提及，是否纳入正式项目字典？"),
    ("O01", "其他目录", "E:/10.13；F:/police/dataset；F:/police/model", ["e:/10.13", "f:/police/dataset", "f:/police/model"], "这些工作区、数据集和模型目录的正式项目名及合并关系？"),
    ("O02", "其他目录", "PycharmProjects/WelcomeScreen（Windows / macOS）", ["pycharmprojects/welcomescreen"], "WelcomeScreen 是真实项目名还是 IDE 默认目录？两平台是否同项目？"),
    ("O03", "其他目录", "D:/AI_project/compass/desktop/framework", ["d:/ai_project/compass/desktop/framework"], "统计为 compass、desktop，还是 framework？src-tauri 是否并入？"),
    ("O04", "其他目录", "compass/component/process", ["compass/component/process"], "独立仓库是否单列，还是归入 compass？"),
    ("O05", "其他目录", "compass/component/screen-perception", ["compass/component/screen-perception"], "独立仓库是否单列，还是归入 compass？"),
    ("O06", "其他目录", "compass/runtime/llm-core", ["compass/runtime/llm-core"], "独立仓库是否单列，还是归入 compass？"),
    ("O07", "其他目录", "D:/AI_project/atlas/deploy", ["d:/ai_project/atlas/deploy"], "deploy 是否只是 atlas 的部署子仓库？"),
    ("O08", "其他目录", "D:/AI_project/atlas/manager-service", ["d:/ai_project/atlas/manager-service"], "按 atlas 汇总还是单列 manager-service？"),
    ("O09", "其他目录", "D:/AI_project/atlas/resource-service", ["d:/ai_project/atlas/resource-service"], "按 atlas 汇总还是单列 resource-service？"),
    ("O10", "其他目录", "atlas/service_sdk", ["atlas/service_sdk"], "公共 SDK 单列还是归入 atlas？"),
    ("O11", "其他目录", "D:/AI_project/workshop/interactive-map", ["d:/ai_project/workshop/interactive-map"], "editor_service、web、plugin 是一个项目还是多个？"),
    ("O12", "其他目录", "D:/AI_project/workshop/game-guide", ["d:/ai_project/workshop/game-guide"], "是否独立项目？是否属于 compass 产品？"),
    ("O13", "其他目录", "D:/AI_project/workshop/game-manager", ["d:/ai_project/workshop/game-manager"], "是否独立项目？是否属于 compass 产品？"),
    ("O14", "其他目录", "D:/AI_project/workshop/mod-manager", ["d:/ai_project/workshop/mod-manager"], "是否独立项目？是否属于 compass 产品？"),
    ("O15", "其他目录", "D:/AI_project/tools/repo-deck", ["d:/ai_project/tools/repo-deck"], "工具自身作为独立项目，还是只算被管理项目？"),
    ("O16", "其他目录", "/Users/<user>/code/python/gitlab_tool", ["code/python/gitlab_tool"], "单列 gitlab_tool，还是归属某个业务项目？"),
    ("O17", "其他目录", "xy-data-router/xunyou-datarouter", ["xy-data-router/xunyou-datarouter"], "src、cgo 与构建子目录是否统一归到 xunyou-datarouter？"),
    ("O18", "其他目录", "E:/allsite/xyai；E:/tmp/imagegen-review-fixes", ["e:/allsite/xyai", "e:/tmp/imagegen-review-fixes"], "临时审核副本是否并入 xyai？正文提及其他项目时如何取主项目？"),
    ("O19", "其他目录", ".codex/worktrees/3b88/client-web-page；orig-web/orig.client-web-page", [".codex/worktrees/3b88/client-web-page", "orig-web/orig.client-web-page"], "worktree 和 orig. 前缀是否统一为 client-web-page？"),
    ("O20", "其他目录", "orig-web/orig.xyweb-lib", ["orig-web/orig.xyweb-lib"], "公共组件库是否单列 xyweb-lib？"),
    ("O21", "其他目录", "C:/Work/Web/orig-lagz/orig.www.lagzapper.com", ["c:/work/web/orig-lagz/orig.www.lagzapper.com"], "是否统一为 www.lagzapper.com？"),
    ("O22", "其他目录", "orig-web/orig.cddada7d.xunyou.com；E:/orig-web/cddada7d.xunyou.com", ["orig-web/orig.cddada7d.xunyou.com", "e:/orig-web/cddada7d.xunyou.com"], "是否同一项目？正式统计名称是什么？"),
    ("O23", "其他目录", "D:/Go/src/marketing-client；marketing-client-component", ["d:/go/src/marketing-client", "d:/go/src/marketing-client-component"], "两个客户端是替代关系还是独立项目？是否与 marketing-agency 合并？"),
    ("O24", "其他目录", "192.168.1.23/beyond/g", ["192.168.1.23/beyond/g"], "当前样本主要为规范文档引用，是否作为规则背景而不参与归因？"),
    ("O25", "其他目录", "C:/Users/<user>/Documents/Codex/2026-10-15/wom", ["documents/codex/2026-10-15/wom"], "正式项目名是什么？日期父目录应忽略？"),
    ("O26", "其他目录", "Documents/海外；Documents/ChatGPT/阿卡贝拉Game；Documents/testcase", ["documents/海外", "documents/chatgpt/阿卡贝拉game", "documents/testcase"], "这些是否业务项目、一次性工作，或应继续未归因？"),
    ("O27", "其他目录", "e:/tmp", ["e:/tmp"], "临时工作区必须依赖哪个明确仓库证据，才能合并到真实项目？"),
]


def matches(value, fragment):
    # Match a whole directory or repository segment, not a shared name prefix.
    return re.search(r"(?:^|/)" + re.escape(fragment.lower()) + r"(?:/|\.git(?:/|$)|$)", value.lower()) is not None


def is_dependency(path):
    return any(x in path.lower() for x in ("/pkg/mod/", "/node_modules/", "/site-packages/"))


def clean_extra(path):
    path = re.split(r"[：”“？]|/n@@|/n-", path)[0]
    path = path.removeprefix("file:")
    path = re.sub(r"(?i)([a-z]:/Users/)[^/]+$", r"\1<user>", path)
    return path


summaries = []
retained = collections.defaultdict(set)
candidate_ids = collections.defaultdict(list)
for row in D["requests"]:
    row["hints"] = [(kind, clean_extra(path), context) for kind, path, context in row.get("hints", [])]
for cid, group, label, fragments, question in CANDIDATES:
    patterns = [re.compile(r"(?:^|/)" + re.escape(f.lower()) + r"(?:/|\.git(?:/|$)|$)") for f in fragments]
    found = {}
    paths = collections.defaultdict(set)
    for row in D["requests"]:
        relevant = []
        for kind, path, context in row.get("hints", []):
            if is_dependency(path) or not any(pattern.search(path.lower()) for pattern in patterns):
                continue
            relevant.append((kind, path, context))
            retained[row["archive_id"]].add((kind, path, context))
            paths[path].add(row["archive_id"])
        if relevant:
            found[row["archive_id"]] = (row, relevant)
            candidate_ids[row["archive_id"]].append(cid)
    roots = [r for r in D["attributed_roots"] if r["client_workdir"] and any(matches(r["client_workdir"], f) for f in fragments)]
    summaries.append({
        "id": cid, "group": group, "label": label, "question": question,
        "unattributed_requests_with_hint": len(found),
        "users": len({row["user_id"] for row, _ in found.values()}),
        "cwd_marker_requests": sum(any(h[0] == "cwd_marker" for h in hints) for _, hints in found.values()),
        "tagged_file_requests": sum(any(h[0] == "tagged_files" for h in hints) for _, hints in found.values()),
        "repository_url_requests": sum(any(h[0] == "repository_url_mention" for h in hints) for _, hints in found.values()),
        "already_attributed_requests": sum(r["requests"] for r in roots),
        "example_paths": [p for p, ids in sorted(paths.items(), key=lambda x: (-len(x[1]), len(x[0])))[:5]],
        "archive_ids": sorted(found), "existing_roots": roots,
    })

dependency_requests = set()
for row in D["requests"]:
    for kind, path, context in row.get("hints", []):
        if "/pkg/mod/192.168.1.27/" in path.lower():
            dependency_requests.add(row["archive_id"])
            # Preserve only the module identity; no dependency source code.
            module = re.split(r"(/pkg/mod/192\.168\.1\.27/[^/]+/[^/]+(?:/v\d+@[^/]+)?)", path, flags=re.I)
            if len(module) > 1:
                retained[row["archive_id"]].add(("dependency_cache", module[0] + module[1], context))

D["curation"] = {
    "candidate_rows": len(summaries),
    "requests_with_curated_candidates": sum(bool(retained[r["archive_id"]]) for r in D["requests"]),
    "requests_with_internal_go_module_cache": len(dependency_requests),
    "notes": "Candidate mentions can overlap. Counts do not establish attribution. Broad hint counts include template and runtime noise. Raw prompt text was not exported.",
}
for row in D["requests"]:
    hints = retained[row["archive_id"]]
    row["candidate_ids"] = candidate_ids[row["archive_id"]]
    row["curated_hint_count"] = len(hints)
    # Keep bounded witnesses, not every repeated file path in conversation history.
    row["hints"] = [h for kind in ("cwd_marker", "tagged_files", "repository_url_mention", "path_mention", "dependency_cache")
                    for h in sorted((h for h in hints if h[0] == kind), key=lambda h: (len(h[1]), h[1], h[2]))[:4]]
(BASE / "evidence.json").write_text(json.dumps(D, ensure_ascii=True), encoding="utf-8")
(BASE / "candidates.json").write_text(json.dumps(summaries, ensure_ascii=False, indent=2), encoding="utf-8")

lines = [
    "# 项目归因线索待确认清单（2026-11-03）", "",
    "本轮只读采集，不修改线上归因结果。用户给出的后端结构：`192.168.1.27 / 服务 / 项目 / 文件`；例如 `micro-activity / battlepass / README.md`。其他映射等待用户定义。", "",
    "## 范围与验证", "",
    "| 项目 | 结果 |", "| --- | --- |",
    "| 时间窗口 | 2026-11-02 00:00–2026-11-03 00:00，Asia/Shanghai |",
    "| 原始文件 | 231,243,156,719 bytes（215.36 GiB）；扫描前后 size/mtime 不变 |",
    "| 已索引请求 | 68,019；未归因 2,690（3.95%），涉及 46 个用户 |",
    "| 读取范围 | 全部 2,690 条未归因请求，逐条校验 archive_id 与 request 事件；不是全文件全量审计 |",
    "| 读取成本 | 按行末 source_offset 倒读；读取约 4.12 GiB，目标请求行合计约 2.60 GiB，135 秒 |",
    "| 原文形态 | 289 条含 tagged_files；32 条正文不是可解析 JSON，采用原始文本搜索；归档 body_truncated 标记均为 false |",
    "| 路径初筛 | 2,556 条出现宽泛路径/URL 线索，包含示例、工具与运行时路径，不能解释为 2,556 条可准确归因 |",
    f"| 候选整理 | {len(summaries)} 组候选；内部 Go 模块缓存出现于 {len(dependency_requests)} 条请求，单独标注 |",
    "| 验证 | 倒读的首行、多块大行、末行 3 项本地边界检查通过；线上与本地 project_attribution.go SHA-256 相同 |", "",
    "每行的“未归因线索请求”按 archive_id 去重；同一请求可提及多个候选，各行不能相加。对话历史会重复携带路径，出现次数不等于独立会话、人工操作数、项目用量或可回填条数。已归因列只核对当天现有工作目录记录。", "",
    "“目录标记”可能来自当前环境、历史环境或引用；“附件”也可能是依赖文件。必须结合位置、角色、主工作区与多仓库冲突规则后才能自动归因。", "",
]
for group in ["后端", "其他目录"]:
    lines += ["## " + group, "", "| 编号 | 发现的路径/仓库线索 | 未归因线索请求 | 目录标记 / 附件 / 仓库地址 | 当前已归因 | 待确认规则 |", "| --- | --- | ---: | ---: | ---: | --- |"]
    for c in summaries:
        if c["group"] == group:
            lines.append(f'| {c["id"]} | `{c["label"]}` | {c["unattributed_requests_with_hint"]} | {c["cwd_marker_requests"]} / {c["tagged_file_requests"]} / {c["repository_url_requests"]} | {c["already_attributed_requests"]} | {c["question"]} |')
    lines.append("")
lines += [
    "## 已识别的误归因风险", "",
    "| 形态 | 本轮证据 | 需要的规则 |", "| --- | --- | --- |",
    "| Go 依赖缓存 | `.../go/pkg/mod/192.168.1.27/micro-activity/modules/v2@v2.44.0/...` 与 battlepass 同时出现 | 保留为依赖证据；不能仅因路径出现就把主项目改成 modules。修改依赖源码时是否另计需定义。 |",
    "| 同会话多仓库 | compass / atlas / workshop，以及 perm-server / modules / gateway 同时出现 | 定义单主项目、拆分或多项目；不能默认取第一条或最高频路径。 |",
    "| worktree / 临时副本 | `.codex/worktrees/.../client-web-page`、`E:/tmp/imagegen-review-fixes` | 用可验证的仓库映射合并；避免把哈希、tmp 或临时目录名作为正式项目。 |",
    "| 项目内子目录 | framework/src-tauri、editor_service/web、xunyou-datarouter/src | 回到确认的项目根；不能直接以当前 basename 作项目名。 |",
    "| 通用规范/基础设施 | beyond/g 文档、包仓库、登录页、API/部署地址 | 本轮不把这些普通提及当作当前项目。 |", "",
    "## 现有提取器的实证限制", "",
    "- 没有专门解析 tagged_files 的分支；Copilot 的 POSIX 路径兜底主要依赖 filePath 属性，普通 macOS 附件路径存在缺口。",
    "- Codex 仅扫描请求体前 300 KiB：两个未归因见证的首个 cwd 分别位于 393,193 和 532,258 字节，超出窗口（shape-check.json）。",
    "- Claude 两个未归因见证的正文携带 Primary working directory，但顶层 system 文本块未含此标记；不能把后续实现简化为只扩展顶层 system 正则（system-shape-check.json）。",
    "- 已归因项目直接取 cwd basename，因此日期、任务名、构建子目录也可能成为项目名；识别命中不等于业务归因正确。", "",
    "## 复查文件与后续", "",
    "- `candidates.json`：每组候选、计数、代表路径、archive_id 清单与现有已归因目录。",
    "- `evidence.json`：请求编号、行末偏移、哈希、经筛选的路径与协议位置；个人 home 目录用户名被遮蔽；不含原始 prompt/正文。",
    "- `collect.py`：在服务器通过 stdin 执行的只读采集器，数据库使用 BEGIN READ ONLY，低 CPU/IO 优先级读取归档；仅向 stdout 输出提取结果。",
    "- `summarize.py`：本地候选整理器；其分组只是待确认问题，不是线上规则。",
    "- 待用户按编号确认项目名、层级、别名与多仓库策略，再设计提取/归一化规则和受控历史回填。",
    "- 本轮没有部署、重索引、生产数据库写入、业务代码修改或提交；没有浏览器验证。wiki 仅补充当前解析能力边界。", "",
]
(BASE / "README.md").write_text("\n".join(lines), encoding="utf-8", newline="\n")
print(json.dumps({"curation": D["curation"], "candidates": [{k: c[k] for k in ("id", "label", "unattributed_requests_with_hint", "cwd_marker_requests", "tagged_file_requests", "already_attributed_requests")} for c in summaries]}, ensure_ascii=False, indent=2))
