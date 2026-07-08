export default {
  tokenAnalysis: {
    title: 'Token 分析',
    description: '查看部门用量、缓存命中和疑似浪费请求',
    indexNow: '索引当前范围',
    riskMin: '风险分',
    includeUnmatched: '包含未匹配归档',
    indexStatus: '索引状态',
    processed: '已处理',
    failed: '失败',
    indexed: '已索引',
    indexStarted: '索引已在后台启动, 完成后自动刷新',
    indexRunning: '索引进行中…',
    indexDone: '索引完成',
    indexAlreadyRunning: '已有索引任务在运行, 正在跟进其进度',
    userRanking: '用户排行',
    projectRanking: '项目消耗排行(成员 × 项目)',
    project: '项目',
    projectFilterHint: '项目名(精确匹配) / unattributed',
    userEmail: '用户邮箱',
    userEmailHint: '邮箱模糊搜索(用户/项目排行、请求明细)',
    riskLegend: '风险说明',
    riskLegendHint: '下列为各风险标记的含义,鼠标移到「风险原因分布」或请求明细的标签上也会显示对应解释。',
    // 风险原因 code → 中文名称与解释; 标签悬停展示 desc, 让不熟悉英文的客户
    // 也能看懂每个风险标记的具体含义。新增风险类型时在此同步补充。
    riskCodes: {
      huge_input_tiny_output: {
        label: '大输入·小输出',
        desc: '输入 token 极高(≥10万)但输出极短(≤200)。通常是把超大上下文或文件一次性塞进请求,却几乎没有产出,白白付了输入费用。'
      },
      repeat_uncached_body: {
        label: '重复请求未命中缓存',
        desc: '短时间内重复发送几乎相同的大请求(≥3 次、输入≥2万 token),但缓存命中很低,相当于为相同内容反复付费。'
      },
      low_cache_hit_large_input: {
        label: '大输入·缓存命中低',
        desc: '单次输入很大(≥5万 token)但缓存命中极低,大部分内容没有复用缓存,输入成本偏高。'
      },
      rapid_similar_requests: {
        label: '高频相似大请求',
        desc: '短时间内连续发送多个(≥5 次)内容相似的大请求(输入≥1万 token),可能是循环或重试造成的重复消耗。'
      },
      oversized_system_prompt: {
        label: 'system 提示过大',
        desc: 'system(系统提示)内容明显偏大(≥2万字符,或超过用户输入)。它每次请求都会被重复携带,放大输入成本。'
      },
      tool_heavy_short_output: {
        label: '工具多·输出短',
        desc: '携带的工具(tools)定义较多(≥10 个)且输入大,但输出很短(≤500 token),工具定义占用大量输入 token 却没带来等价产出。'
      },
      large_tool_history: {
        label: '工具历史过大',
        desc: '对话历史里累积的工具(tool)返回结果体积过大(≥512KB、≥10 条),每轮都会被重复带入上下文,持续推高输入成本。'
      },
      giant_tool_output: {
        label: '单条工具输出过大',
        desc: '存在单条超大的工具(tool)返回结果(≥512KB),一次性占用大量 token,通常是工具返回了未裁剪的大段内容。'
      }
    },
    unattributed: '未归因',
    requests: '请求数',
    cacheTokens: '缓存 Token',
    inputOutput: '输入 / 输出',
    outputInputRatio: '输出 / 输入',
    outputInputRatioThreshold: '健康阈值',
    outputInputRatioUnavailable: '无输入 token, 无法计算输出 / 输入比例',
    lastActive: '最近活动',
    workdir: '工作目录',
    branch: '分支',
    attributionSource: '归因来源',
    duplicateCount: '重复',
    duplicateHint: '当前筛选范围内净输入相同的请求数; >1 表示同一输入在 agent 轮次或重试里重发',
    bodyTruncated: '归档截断',
    bodyTruncatedHint: '归档时请求体被截断, 提取的归因与全文可能不完整',
    contentHash: '内容哈希',
    contentHashHint: '对脱敏后净输入计算, 用于识别重复输入(非原文指纹)',
    chars: '字符',
    requestDetails: '请求明细',
    time: '时间',
    quality: '质量',
    qualityNotEvaluated: '未评估',
    noInputStored: '未留存输入',
    userRequest: 'User Request',
    userRequestHint: '从 <userRequest> 标签提取, 内容仍来自已脱敏/可能截断的留存输入',
    inputFull: '用户输入全文',
    inputTruncatedNote: '超长已截断, 完整内容见归档 JSONL',
    sortByTime: '按时间',
    sortByRisk: '按风险',
    user: '用户',
    tokens: 'Token',
    cost: '费用',
    risk: '风险',
    usage: '用量',
    preview: '请求预览',
    matchQuality: '匹配质量',
    matched: '已匹配',
    unmatched: '未匹配',
    unmatchedRate: '未匹配率',
    riskReasons: '风险原因分布',
    riskRate: '风险占比',
    file: '文件',
    offset: '偏移',
    updated: '更新时间',
    archiveFiles: '归档文件',
    archiveFilesHint: '仅列出当前生效归档目录(切换目录后旧目录文件需手动迁移)。「可删除」= 已全部入库, 可在服务器上 gzip/删除(删除为手动操作); 今日文件正在写入不可删; 「有失败行」删除前请先确认失败原因。',
    fileSize: '大小',
    indexProgress: '读取进度',
    fileStatus: '状态',
    fullyRead: '已读完',
    requestRowsTotal: '合计 {n} 请求行',
    archiveFileStatus: {
      writing: '写入中',
      indexing: '待索引',
      deletable: '可删除',
      attention: '有失败行',
      compressed: '已压缩'
    },
    archiveFileStatusHint: {
      writing: '今日文件写入端正持有句柄, 不可删除',
      indexing: '索引水位尚未追平文件大小, 等待自动索引或手动触发',
      deletable: '已全部读完入库且无失败行, 可在服务器上安全压缩/删除',
      attention: '文件已全部读完, 但有失败行未入库; 失败行不会自动重试, 删除文件即放弃重灌机会',
      compressed: '已压缩文件, 不参与索引'
    },
    summary: {
      totalRequests: '已归档请求数',
      billedRequests: '同期计费请求数',
      archiveCoverage: '归档覆盖率',
      totalTokens: '总 Token',
      inputTokens: '输入 Token',
      outputTokens: '输出 Token',
      totalCost: '总费用',
      cacheRead: '缓存命中',
      cacheHitRate: '缓存命中率',
      riskyRequests: '可疑请求',
      riskyCost: '可疑费用'
    }
  },


}
