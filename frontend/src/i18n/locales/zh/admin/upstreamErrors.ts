export default {
  upstreamErrors: {
    title: '上游错误',
    description: '集中查看上游 502 / 503 错误，包括重试后恢复与最终失败的请求，按最新时间排列。',
    time: '发生时间', key: '触发 API Key', keyId: 'API Key ID', accountId: '失败账号 ID',
    failedAccount: '失败账号', finalAccount: '最终成功账号', events: '上游错误事件', outcome: '最终结果',
    details: '查看链路', range: '时间范围', status: '上游状态码', hours: '最近 {n} 小时',
    allOutcomes: '全部结果', recovered: '已恢复', failed: '最终失败', notRecorded: '未记录', noSuccess: '未成功',
    retentionHint: '复用运维错误采集与保留设置。仅展示已采集的数据；关闭监控、跳过记录、队列丢弃或清理会影响可见记录。',
    eventsHint: '按采集顺序展示失败、重试及切换账号事件。同一次尝试可能生成多个事件，事件数不等于重试次数。',
    eventCount: '{n} 个事件', loadFailed: '加载失败，请确认运维监控已开启后重试。',
    empty: '当前条件下暂无已采集的 502 / 503 错误', requestId: '客户端请求 ID',
    upstreamRequestId: '上游请求 ID', responseBody: '已脱敏的上游响应正文'
  }
}
