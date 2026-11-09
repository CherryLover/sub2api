export default {
  topology: {
    title: '调度拓扑',
    description: '谁的请求走到了哪个账号，中间过了哪些关卡',
    live: {
      open: '实时',
      connecting: '连接中',
      reconnecting: '重连中',
      closed: '未连接'
    },
    range: {
      label: '时间范围',
      now: '此刻',
      m5: '近 5 分钟',
      h1: '近 1 小时',
      soon: '即将支持'
    },
    totals: {
      inFlight: '正在请求',
      queued: '排队',
      requests5m: '近 5 分钟',
      times: '次',
      failed: '失败',
      available: '可用账号'
    },
    columns: {
      keys: 'API 密钥',
      accounts: '上游账号'
    },
    key: {
      inFlight: '正在请求',
      queued: '排队',
      idle: '空闲',
      infoLabel: '这个数字是什么意思',
      foldedMore: '其余 {n} 个密钥 · 近 5 分钟没有请求',
      collapse: '收起不活跃的密钥',
      noGroup: '未分组',
      none: '暂无 API 密钥',
      tipTitleActive: '{name} · 正在请求 {n}',
      tipTitleQueued: '{name} · 排队 {n}',
      tipTitleIdle: '{name} · 空闲',
      tipBodyActive: '这个密钥此刻有 {n} 个请求已经发出、还没返回，比如同时开着 {n} 个窗口在用。',
      tipBodyQueued: '这个密钥有 {n} 个请求在等空位：能接它的账号此刻都满了。',
      tipBodyIdle: '这个密钥此刻没有正在跑的请求。',
      tipInFlight: '正在请求：已经发出、还没返回的请求数。',
      tipQueued: '排队：账号满了，正在等空位的请求数（有才显示）。',
      tipIdle: '空闲：此刻一个请求都没有。',
      closeTip: '关闭说明'
    },
    platform: {
      title: '调度中心',
      subtitle: '每个请求从上到下过一遍这些关卡',
      passed: '放行',
      blocked: '拦下',
      window5m: '近 5 分钟',
      retries: '重试',
      retrySame: '原地',
      retrySwitch: '换号',
      failedAfterRetry: '重试后仍失败',
      timesSuffix: '次',
      topBlock: '拦下最多的关卡：',
      topBlockNone: '近 5 分钟没有被拦下的请求',
      blockedChip: '拦下：{reason}'
    },
    gates: {
      auth: { title: '校验密钥', desc: '密钥有效、没停用、余额够' },
      group: { title: '找到分组', desc: '按密钥绑定的分组，拿到这一组的账号' },
      sticky: { title: '优先沿用上次账号', desc: '同一个会话尽量回到上次那个账号，能命中缓存' },
      platform: { title: '平台对得上', desc: 'OpenAI 的请求只给 OpenAI 账号' },
      status: { title: '账号状态正常', desc: '没被限流、没在冷却、授权没失效' },
      quota: { title: '额度还够', desc: '5 小时和每周额度都没用完' },
      model: { title: '支持这个模型', desc: '账号的模型列表里有请求的模型' },
      capacity: { title: '还有空位', desc: '同时在跑的没满；满了就排队等' },
      dispatch: { title: '挑最闲的发出去', desc: '负载最低的接单；上游报错先原地重试，不行就换下一个（最多 10 次）' }
    },
    reason: {
      unknown: '原因未知',
      invalid_api_key: '密钥无效',
      api_key_disabled: '密钥已停用',
      api_key_expired: '密钥已过期',
      insufficient_balance: '余额不足',
      group_not_found: '找不到分组',
      no_group: '密钥没有绑定分组',
      group_disabled: '分组已停用',
      platform_mismatch: '平台对不上',
      platform_filtered: '平台对不上',
      channel_restricted: '渠道限制',
      channel_upstream_restricted: '渠道上游限制',
      transport_incompatible: '传输方式不兼容',
      not_schedulable: '账号暂不可调度',
      unschedulable: '账号暂不可调度',
      runtime_blocked: '账号运行中被临时屏蔽',
      excluded: '已试过的账号被排除',
      shadow_parent_unhealthy: '上级账号不健康',
      privacy_not_set: '账号未设置隐私模式',
      ineligible: '账号不符合条件',
      model_rate_limited: '模型被限流',
      quota_exhausted: '额度用完',
      quota: '额度用完',
      quota_unknown: '额度读不到',
      model_not_supported: '没有支持这个模型的账号',
      model_unsupported: '没有支持这个模型的账号',
      compact_unsupported: '不支持压缩请求',
      capacity: '账号都满了',
      concurrency_full: '账号都满了',
      queue_full: '排队已满',
      full: '账号都满了',
      no_available_accounts: '没有可用账号'
    },
    tag: {
      success: '成功',
      successNth: '第 {n} 次成功',
      upstream: '上游 {code}',
      upstreamRateLimited: '上游 429 · 被限流',
      upstreamNoResponse: '上游无响应',
      retrySame: '原账号重试 · 第 {n} 次',
      switchNext: '换下一个 · 第 {n} 次',
      giveUp: '能换的都试过了',
      returned: '返回 {code}',
      cooldown: '冷却中 · 还剩 {n}s'
    },
    account: {
      rateLimited: '限流中',
      cooldown: '冷却中',
      error: '授权失效',
      disabled: '已停用',
      recoverAt: '{time} 恢复',
      reauth: '去重新授权',
      payg: '按量付费，没有 5 小时 / 7 天限制',
      quotaLabel: '额度',
      quotaNone: '读不到',
      none: '暂无上游账号',
      concurrency: '同时在跑 / 上限'
    },
    quota: {
      w5h: '5 小时',
      w7d: '7 天',
      used: '{percent}%',
      usedReset: '{percent}% · {when}重置',
      exhausted: '用完',
      exhaustedUntil: '用完 · {time} 恢复'
    },
    time: {
      minutesLater: '{n} 分钟后',
      hoursLater: '{n} 小时后',
      daysLater: '{n} 天后'
    },
    strategy: {
      load_balance: '负载均衡',
      sticky: '会话粘滞',
      priority: '按优先级',
      round_robin: '轮流',
      mixed: '多种策略'
    },
    legend: {
      active: '有请求在走',
      cooldown: '账号冷却中，暂不接新请求',
      request: '一个请求',
      error: '上游报错，往回退',
      unavailable: '账号不可用'
    },
    state: {
      loading: '正在读取调度数据…',
      loadFailed: '读取调度数据失败',
      refreshFailed: '刷新失败，显示的是上一次的数据',
      retry: '重试',
      empty: '还没有 API 密钥和上游账号，添加后这里会显示调度关系。'
    }
  }
}
