# Model Plaza Components

本目录负责模型广场的筛选、分组和价格展示。数据只来自 `modelPlazaAPI.get()`；组件不自行推断分组可见性或登录权限，最终边界由后端 `/api/v1/model-plaza` 决定。

## 主要文件

| 文件 | 职责 |
|---|---|
| `ModelPlazaContent.vue` | 组合描述、搜索、平台/分组/倍率筛选和结果区 |
| `PlazaFilterBar.vue` | 输出筛选条件，不修改原始响应 |
| `PlazaGroupSection.vue` | 展示分组信息、高峰倍率说明与模型价格表 |
| `PlazaModelPricingTable.vue` | 展示 token、缓存、图片、视频和按次价格，并区分官方价格与用户有效倍率 |
| `PlazaNavBar.vue` | 展示站点品牌、登录状态和返回入口 |

`user_rate_multiplier` 存在时优先于公开 `rate_multiplier`，但不得据此推断授权；匿名用户只应收到后端允许公开的分组。Markdown 描述必须经过既有 sanitizer 后渲染。

## Cache write 展示

- `PlazaModelPricingTable.vue` 在实际价和官方价两个视图中都支持 `cache_write_1h_price`，包括分时 interval。
- 只有 1h 值非 `null/undefined` 时才在常规 cache-write 后标注 `(1h ...)`；显式 0 仍是有效价格，缺失值不额外渲染。

## 验证

```powershell
cmd.exe /c pnpm --dir frontend exec vitest run src/components/modelPlaza
cmd.exe /c pnpm --dir frontend run typecheck
```
