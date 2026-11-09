export default {
  topology: {
    title: 'Routing Topology',
    description: 'Which key’s requests went to which account, and which gates they passed',
    live: {
      open: 'Live',
      connecting: 'Connecting',
      reconnecting: 'Reconnecting',
      closed: 'Disconnected'
    },
    range: {
      label: 'Time range',
      now: 'Now',
      m5: 'Last 5 min',
      h1: 'Last hour',
      soon: 'Coming soon'
    },
    totals: {
      inFlight: 'In flight',
      queued: 'Queued',
      requests5m: 'Last 5 min',
      times: 'requests',
      failed: 'Failed',
      available: 'Available accounts'
    },
    columns: {
      keys: 'API keys',
      accounts: 'Upstream accounts'
    },
    key: {
      inFlight: 'In flight',
      queued: 'Queued',
      idle: 'Idle',
      infoLabel: 'What does this number mean',
      foldedMore: '{n} more keys · no requests in the last 5 min',
      collapse: 'Hide inactive keys',
      noGroup: 'No group',
      none: 'No API keys yet',
      tipTitleActive: '{name} · {n} in flight',
      tipTitleQueued: '{name} · {n} queued',
      tipTitleIdle: '{name} · idle',
      tipBodyActive: 'This key has {n} requests sent and not yet answered, e.g. {n} windows in use at once.',
      tipBodyQueued: 'This key has {n} requests waiting for a free slot: every account that can take it is full.',
      tipBodyIdle: 'This key has no running requests right now.',
      tipInFlight: 'In flight: requests sent and not yet answered.',
      tipQueued: 'Queued: requests waiting for a free account slot (shown only when non-zero).',
      tipIdle: 'Idle: no requests at all right now.',
      closeTip: 'Close'
    },
    platform: {
      title: 'Scheduler',
      subtitle: 'Every request passes these gates from top to bottom',
      passed: 'Passed',
      blocked: 'Blocked',
      window5m: 'Last 5 min',
      retries: 'Retries',
      retrySame: 'Same',
      retrySwitch: 'Switched',
      failedAfterRetry: 'Failed after retries',
      timesSuffix: 'times',
      topBlock: 'Most blocking gate: ',
      topBlockNone: 'No blocked requests in the last 5 min',
      blockedChip: 'Blocked: {reason}'
    },
    gates: {
      auth: { title: 'Check key', desc: 'Key is valid, enabled, and has balance' },
      group: { title: 'Find group', desc: 'Use the key’s group to get its accounts' },
      sticky: { title: 'Prefer last account', desc: 'Same session goes back to the same account to hit the cache' },
      platform: { title: 'Platform matches', desc: 'OpenAI requests only go to OpenAI accounts' },
      status: { title: 'Account healthy', desc: 'Not rate limited, not cooling down, auth still valid' },
      quota: { title: 'Quota left', desc: '5-hour and weekly quotas are not used up' },
      model: { title: 'Supports the model', desc: 'The account’s model list includes the requested model' },
      capacity: { title: 'Free slot', desc: 'Concurrency not full; otherwise wait in queue' },
      dispatch: { title: 'Send to the least busy', desc: 'Lowest load takes it; on upstream errors retry in place, then switch (up to 10 times)' }
    },
    reason: {
      unknown: 'Unknown reason',
      invalid_api_key: 'Invalid key',
      api_key_disabled: 'Key disabled',
      api_key_expired: 'Key expired',
      insufficient_balance: 'Insufficient balance',
      group_not_found: 'Group not found',
      no_group: 'Key has no group',
      group_disabled: 'Group disabled',
      platform_mismatch: 'Platform mismatch',
      platform_filtered: 'Platform mismatch',
      channel_restricted: 'Channel restricted',
      channel_upstream_restricted: 'Channel upstream restricted',
      transport_incompatible: 'Transport incompatible',
      not_schedulable: 'Account not schedulable',
      unschedulable: 'Account not schedulable',
      runtime_blocked: 'Account temporarily blocked',
      excluded: 'Tried accounts excluded',
      shadow_parent_unhealthy: 'Parent account unhealthy',
      privacy_not_set: 'Account privacy mode not set',
      ineligible: 'Account not eligible',
      model_rate_limited: 'Model rate limited',
      quota_exhausted: 'Quota used up',
      quota: 'Quota used up',
      quota_unknown: 'Quota unknown',
      model_not_supported: 'No account supports this model',
      model_unsupported: 'No account supports this model',
      compact_unsupported: 'Compact requests not supported',
      capacity: 'All accounts are full',
      concurrency_full: 'All accounts are full',
      queue_full: 'Queue is full',
      full: 'All accounts are full',
      no_available_accounts: 'No available accounts'
    },
    tag: {
      success: 'Success',
      successNth: 'Success on try {n}',
      upstream: 'Upstream {code}',
      upstreamRateLimited: 'Upstream 429 · rate limited',
      upstreamNoResponse: 'No upstream response',
      retrySame: 'Retry same account · try {n}',
      switchNext: 'Switch account · try {n}',
      giveUp: 'Tried every account',
      returned: 'Returned {code}',
      cooldown: 'Cooling down · {n}s left'
    },
    account: {
      rateLimited: 'Rate limited',
      cooldown: 'Cooling down',
      error: 'Auth expired',
      disabled: 'Disabled',
      recoverAt: 'recovers {time}',
      reauth: 'Re-authorize',
      payg: 'Pay as you go, no 5-hour / 7-day limits',
      quotaLabel: 'Quota',
      quotaNone: 'Unavailable',
      none: 'No upstream accounts yet',
      concurrency: 'Running / limit'
    },
    quota: {
      w5h: '5h',
      w7d: '7d',
      used: '{percent}%',
      usedReset: '{percent}% · resets {when}',
      exhausted: 'Used up',
      exhaustedUntil: 'Used up · recovers {time}'
    },
    time: {
      minutesLater: 'in {n} min',
      hoursLater: 'in {n} h',
      daysLater: 'in {n} d'
    },
    strategy: {
      load_balance: 'Load balance',
      sticky: 'Sticky session',
      priority: 'Priority',
      round_robin: 'Round robin',
      mixed: 'Mixed strategies'
    },
    legend: {
      active: 'Requests flowing',
      cooldown: 'Account cooling down, not taking new requests',
      request: 'A request',
      error: 'Upstream error, going back',
      unavailable: 'Account unavailable'
    },
    state: {
      loading: 'Loading routing data…',
      loadFailed: 'Failed to load routing data',
      refreshFailed: 'Refresh failed, showing the last data',
      retry: 'Retry',
      empty: 'No API keys or upstream accounts yet. Add some and the routing will show here.'
    }
  }
}
