# llm-wiki schema pointer

本文件供 `/understand-knowledge` 识别 Karpathy-pattern wiki 的 schema 入口。

**权威规则在仓库根目录 `AGENTS.md`。** 日常 AI 协作请始终以根目录 `AGENTS.md` 与 `llm-wiki/wiki/*.md` 为准；不要在本文件复制第二份完整规则，避免漂移。

## 知识分层

- `wiki/`: 稳定结论（开发前必读）
- `raw/`: 原始材料（非日常入口）
- `index.md`: 分类目录（wikilink）
- 根目录 `AGENTS.md`: 强制读写纪律

## 与知识图谱的关系

- Wiki 正文是权威知识；图谱是导航/检索层。
- 代码图谱：仓库根 `.understand-anything/knowledge-graph.json`（本机生成，默认不入库）
- Wiki 图谱：`llm-wiki/.understand-anything/knowledge-graph.json`（体积小，可入库共享）
