export default {
  organizationUsage: {
    title: 'Organization Usage Report',
    description: 'Review organization and user usage over a selected reporting period.',
    filters: {
      title: 'Report filters',
      monthly: 'Monthly',
      weekly: 'Weekly',
      custom: 'Custom',
      month: 'Month',
      weekAnchor: 'Week containing',
      startDate: 'Start date',
      endDate: 'End date',
      organization: 'Organization',
      email: 'Email',
      emailPlaceholder: 'Search user email'
    },
    actions: {
      query: 'Query',
      reset: 'Reset',
      retry: 'Retry',
      export: 'Export XLSX',
      exporting: 'Exporting...'
    },
    overview: { title: 'Overview' },
    trend: {
      title: 'Usage trend',
      granularity: 'Trend granularity',
      day: 'Day',
      week: 'Week',
      month: 'Month',
      loadFailed: 'Failed to load the usage trend.',
      retry: 'Retry',
      asOf: 'Data as of {value}',
      partialHint: 'Partial period'
    },
    organizationSummary: { title: 'Organization summary' },
    people: {
      title: 'People summary',
      total: '{count} users',
      pageSize: 'Rows'
    },
    organizations: {
      all: 'All organizations',
      other: 'Other'
    },
    metrics: {
      activeUsers: 'Registered users',
      usedUsers: 'Active users',
      requests: 'Requests',
      inputTokens: 'Input tokens',
      outputTokens: 'Output tokens',
      cacheCreationTokens: 'Cache creation tokens',
      cacheReadTokens: 'Cache read tokens',
      cacheTokens: 'Cache tokens',
      totalTokens: 'Total tokens',
      actualCost: 'Actual cost',
      tokenShare: 'Token share'
    },
    columns: {
      organization: 'Organization',
      email: 'Email',
      peakDay: 'Peak day',
      peakWeek: 'Peak week',
      peakMonth: 'Peak month'
    },
    champions: {
      day: 'Team day champion',
      week: 'Team week champion',
      month: 'Team month champion'
    },
    common: {
      noData: 'No data',
      partial: 'Partial',
      tokens: 'tokens'
    },
    validation: {
      required: 'Select both a start and end date.',
      invalidDate: 'Enter a valid date range.',
      startAfterEnd: 'Start date cannot be after end date.',
      rangeTooLong: 'Custom date range cannot exceed 366 days.'
    },
    feedback: {
      loadFailed: 'Failed to load the organization usage report.',
      exportPreparing: 'Preparing report data...',
      generatingWorkbook: 'Generating workbook...',
      exportSuccess: 'Organization usage report exported.',
      exportCanceled: 'Export canceled.',
      exportTooLarge: 'The report exceeds the 100,000-row client export limit.',
      exportFailed: 'Failed to export the organization usage report.'
    }
  }
}
