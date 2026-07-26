export default {
  organizationUsage: {
    title: '组织用量报表',
    description: '按报表周期查看组织与人员维度的请求、Token 和实际成本。',
    filters: {
      title: '报表筛选',
      monthly: '月报',
      weekly: '周报',
      custom: '自定义',
      month: '月份',
      weekAnchor: '所在周',
      startDate: '开始日期',
      endDate: '结束日期',
      organization: '组织',
      email: '邮箱',
      emailPlaceholder: '搜索用户邮箱'
    },
    actions: {
      query: '查询',
      reset: '重置',
      retry: '重试',
      export: '导出 XLSX',
      exporting: '导出中...'
    },
    overview: { title: '用量概览' },
    organizationSummary: { title: '组织汇总' },
    people: {
      title: '人员汇总',
      total: '共 {count} 人',
      pageSize: '每页'
    },
    organizations: {
      all: '全部组织',
      other: '其他'
    },
    metrics: {
      activeUsers: '活跃人数',
      usedUsers: '有用量人数',
      requests: '请求数',
      inputTokens: '输入 Token',
      outputTokens: '输出 Token',
      cacheCreationTokens: '缓存创建 Token',
      cacheReadTokens: '缓存读取 Token',
      cacheTokens: '缓存 Token',
      totalTokens: '总 Token',
      actualCost: '实际成本',
      tokenShare: 'Token 占比'
    },
    columns: {
      organization: '组织',
      email: '邮箱',
      peakDay: '个人日峰值',
      peakWeek: '个人周峰值',
      peakMonth: '个人月峰值'
    },
    champions: {
      day: '团队日 Champion',
      week: '团队周 Champion',
      month: '团队月 Champion'
    },
    common: {
      noData: '无数据',
      partial: '不完整周期',
      tokens: 'Token'
    },
    validation: {
      required: '请选择开始和结束日期。',
      invalidDate: '请输入有效的日期范围。',
      startAfterEnd: '开始日期不能晚于结束日期。',
      rangeTooLong: '自定义日期范围不能超过 366 天。'
    },
    feedback: {
      loadFailed: '组织用量报表加载失败。',
      exportPreparing: '正在准备报表数据...',
      generatingWorkbook: '正在生成工作簿...',
      exportSuccess: '组织用量报表已导出。',
      exportCanceled: '已取消导出。',
      exportTooLarge: '报表超过客户端 100,000 行导出上限。',
      exportFailed: '组织用量报表导出失败。'
    }
  }
}
