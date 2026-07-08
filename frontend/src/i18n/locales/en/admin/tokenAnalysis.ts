export default {
  tokenAnalysis: {
    title: 'Token Analysis',
    description: 'View department usage, cache hits, and suspicious token waste requests',
    indexNow: 'Index Range',
    riskMin: 'Risk',
    includeUnmatched: 'Include unmatched',
    indexStatus: 'Index status',
    processed: 'Processed',
    failed: 'Failed',
    indexed: 'Indexed',
    indexStarted: 'Indexing started in the background; the page refreshes when it finishes',
    indexRunning: 'Indexing…',
    indexDone: 'Indexing finished',
    indexAlreadyRunning: 'An indexing run is already in progress; following its progress',
    userRanking: 'User Ranking',
    projectRanking: 'Project Usage (Member x Project)',
    project: 'Project',
    projectFilterHint: 'exact project name / unattributed',
    userEmail: 'User Email',
    userEmailHint: 'fuzzy email search (user/project ranking, requests)',
    riskLegend: 'Risk reference',
    riskLegendHint: 'What each risk tag means. Hovering a tag in the distribution or request list also shows its explanation.',
    // Risk code -> human label and explanation; the full desc is shown on hover.
    riskCodes: {
      huge_input_tiny_output: {
        label: 'Huge input, tiny output',
        desc: 'Input tokens are extremely high (>=100k) but output is tiny (<=200). Usually a huge context/file is stuffed into the request with almost no result, wasting input cost.'
      },
      repeat_uncached_body: {
        label: 'Repeated, uncached request',
        desc: 'Nearly identical large requests are repeated in a short window (>=3 times, input >=20k tokens) with low cache hits — paying again for the same content.'
      },
      low_cache_hit_large_input: {
        label: 'Large input, low cache hit',
        desc: 'A single input is very large (>=50k tokens) but cache hit is very low; most content is not reused from cache, so input cost is high.'
      },
      rapid_similar_requests: {
        label: 'Rapid similar requests',
        desc: 'Several similar large requests (>=5, input >=10k tokens) sent in a short window — likely repeated spend from a loop or retries.'
      },
      oversized_system_prompt: {
        label: 'Oversized system prompt',
        desc: 'The system prompt is clearly oversized (>=20k chars, or larger than user input). It is re-sent with every request, amplifying input cost.'
      },
      tool_heavy_short_output: {
        label: 'Tool-heavy, short output',
        desc: 'Many tool definitions (>=10) with a large input but very short output (<=500 tokens); tool defs consume lots of input tokens without matching value.'
      },
      large_tool_history: {
        label: 'Oversized tool history',
        desc: 'Accumulated tool outputs in the conversation are oversized (>=512KB, >=10 messages), re-injected into context each turn and steadily driving up input cost.'
      },
      giant_tool_output: {
        label: 'Giant tool output',
        desc: 'A single tool output is huge (>=512KB), consuming many tokens at once — usually an untrimmed large blob returned by a tool.'
      }
    },
    unattributed: 'Unattributed',
    requests: 'Requests',
    cacheTokens: 'Cache Tokens',
    inputOutput: 'Input / Output',
    outputInputRatio: 'Output / Input',
    outputInputRatioThreshold: 'Healthy threshold',
    outputInputRatioUnavailable: 'No input tokens; output / input ratio is unavailable',
    lastActive: 'Last Active',
    workdir: 'Workdir',
    branch: 'Branch',
    attributionSource: 'Attribution Source',
    duplicateCount: 'Repeats',
    duplicateHint: 'Requests sharing the same net input in the current filter; >1 means the same input was resent across agent turns or retries',
    bodyTruncated: 'Body truncated',
    bodyTruncatedHint: 'Request body was truncated when archived; attribution and full text may be incomplete',
    contentHash: 'Content hash',
    contentHashHint: 'Computed over the redacted net input; used to dedupe inputs (not a raw-text fingerprint)',
    chars: 'chars',
    requestDetails: 'Request Details',
    time: 'Time',
    quality: 'Quality',
    qualityNotEvaluated: 'Not evaluated',
    noInputStored: 'Input not stored',
    userRequest: 'User Request',
    userRequestHint: 'Extracted from the <userRequest> tag; content is still from redacted and possibly truncated stored input',
    inputFull: 'Full User Input',
    inputTruncatedNote: 'Truncated; full text in archive JSONL',
    sortByTime: 'By time',
    sortByRisk: 'By risk',
    user: 'User',
    tokens: 'Tokens',
    cost: 'Cost',
    risk: 'Risk',
    usage: 'Usage',
    preview: 'Request Preview',
    matchQuality: 'Match Quality',
    matched: 'Matched',
    unmatched: 'Unmatched',
    unmatchedRate: 'Unmatched Rate',
    riskReasons: 'Risk Reasons',
    riskRate: 'Risk Rate',
    file: 'File',
    offset: 'Offset',
    updated: 'Updated',
    archiveFiles: 'Archive Files',
    archiveFilesHint: 'Only the effective archive dir is listed (move legacy-dir files manually after switching). "Deletable" = fully indexed into DB; safe to gzip/delete on the server (deletion is manual). Today\'s file is being written. Check failures before deleting "Has failures" files.',
    fileSize: 'Size',
    indexProgress: 'Read Progress',
    fileStatus: 'Status',
    fullyRead: 'fully read',
    requestRowsTotal: '{n} request rows total',
    archiveFileStatus: {
      writing: 'Writing',
      indexing: 'Pending index',
      deletable: 'Deletable',
      attention: 'Has failures',
      compressed: 'Compressed'
    },
    archiveFileStatusHint: {
      writing: "Today's file is still being written; do not delete",
      indexing: 'Index watermark has not caught up with the file size yet; wait for auto index or trigger manually',
      deletable: 'Fully read and indexed with no failed rows; safe to compress/delete on the server',
      attention: 'File fully read, but some rows failed to index; failed rows are not retried automatically — deleting the file forfeits re-indexing',
      compressed: 'Compressed file; not indexed'
    },
    summary: {
      totalRequests: 'Archived Requests',
      billedRequests: 'Billed Requests (same period)',
      archiveCoverage: 'Archive Coverage',
      totalTokens: 'Total Tokens',
      inputTokens: 'Input Tokens',
      outputTokens: 'Output Tokens',
      totalCost: 'Total Cost',
      cacheRead: 'Cache Read',
      cacheHitRate: 'Cache Hit Rate',
      riskyRequests: 'Risky Requests',
      riskyCost: 'Risky Cost'
    }
  },


}
