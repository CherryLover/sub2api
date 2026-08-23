import { describe, expect, it, beforeEach, afterEach, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'

import KeyUsageView from '../KeyUsageView.vue'

const { showInfo, showSuccess, showError, fetchPublicSettings } = vi.hoisted(() => ({
  showInfo: vi.fn(),
  showSuccess: vi.fn(),
  showError: vi.fn(),
  fetchPublicSettings: vi.fn(),
}))

const routeState = vi.hoisted(() => ({
  path: '/key-usage',
  query: {} as Record<string, unknown>,
}))

const routerMock = vi.hoisted(() => ({
  replace: vi.fn(() => Promise.resolve()),
  push: vi.fn(() => Promise.resolve()),
}))

const messages: Record<string, string> = {
  'keyUsage.title': 'API Key Usage',
  'keyUsage.subtitle': 'Usage status',
  'keyUsage.placeholder': 'sk-test',
  'keyUsage.query': 'Query',
  'keyUsage.querying': 'Querying...',
  'keyUsage.privacyNote':
    'Your Key is sent to this site’s server only to verify it and exchange it for a read-only lookup token; the exchange does not save your Key. The token carries an irreversible fingerprint rather than the Key itself, and it can only read usage — it cannot call the API.',
  'keyUsage.dateRange': 'Date Range:',
  'keyUsage.dateRangeToday': 'Today',
  'keyUsage.dateRange7d': '7 Days',
  'keyUsage.dateRange30d': '30 Days',
  'keyUsage.dateRange90d': '90 Days',
  'keyUsage.dateRangeCustom': 'Custom',
  'keyUsage.apply': 'Apply',
  'keyUsage.used': 'Used',
  'keyUsage.detailInfo': 'Detail Information',
  'keyUsage.tokenStats': 'Token Statistics',
  'keyUsage.dailyDetail': 'Daily Detail',
  'keyUsage.modelStats': 'Model Usage Statistics',
  'keyUsage.date': 'Date',
  'keyUsage.model': 'Model',
  'keyUsage.requests': 'Requests',
  'keyUsage.inputTokens': 'Input Tokens',
  'keyUsage.outputTokens': 'Output Tokens',
  'keyUsage.cacheReadTokens': 'Cache Read',
  'keyUsage.cacheWriteTokens': 'Cache Write',
  'keyUsage.cacheCreationTokens': 'Cache Creation',
  'keyUsage.totalTokens': 'Total Tokens',
  'keyUsage.cost': 'Cost',
  'keyUsage.quotaMode': 'Key Quota Mode',
  'keyUsage.walletBalance': 'Wallet Balance',
  'keyUsage.totalQuota': 'Total Quota',
  'keyUsage.limit5h': '5-Hour Limit',
  'keyUsage.limitDaily': 'Daily Limit',
  'keyUsage.limit7d': '7-Day Limit',
  'keyUsage.limitWeekly': 'Weekly Limit',
  'keyUsage.limitMonthly': 'Monthly Limit',
  'keyUsage.remainingQuota': 'Remaining Quota',
  'keyUsage.usedQuota': 'Used Quota',
  'keyUsage.subscriptionType': 'Subscription Type',
  'keyUsage.todayRequests': 'Today Requests',
  'keyUsage.todayInputTokens': 'Today Input',
  'keyUsage.todayOutputTokens': 'Today Output',
  'keyUsage.todayTokens': 'Today Tokens',
  'keyUsage.todayCacheCreation': 'Today Cache Creation',
  'keyUsage.todayCacheRead': 'Today Cache Read',
  'keyUsage.todayCost': 'Today Cost',
  'keyUsage.rpmTpm': 'RPM / TPM',
  'keyUsage.totalRequests': 'Total Requests',
  'keyUsage.totalInputTokens': 'Total Input',
  'keyUsage.totalOutputTokens': 'Total Output',
  'keyUsage.totalTokensLabel': 'Total Tokens',
  'keyUsage.totalCacheCreation': 'Total Cache Creation',
  'keyUsage.totalCacheRead': 'Total Cache Read',
  'keyUsage.totalCost': 'Total Cost',
  'keyUsage.avgDuration': 'Avg Duration',
  'keyUsage.enterApiKey': 'Please enter an API Key',
  'keyUsage.querySuccess': 'Query successful',
  'keyUsage.queryFailed': 'Query failed',
  'keyUsage.queryFailedRetry': 'Query failed, please try again later',
  'keyUsage.noDailyUsage': 'No daily usage data',
  'keyUsage.keyInfo.name': 'Key Name',
  'keyUsage.keyInfo.createdAt': 'Created',
  'keyUsage.session.active': 'Signed in with a lookup link',
  'keyUsage.session.activeFor': 'Lookup link for {name}',
  'keyUsage.session.expiresAt': 'Link valid until {time}',
  'keyUsage.session.copyLink': 'Copy lookup link',
  'keyUsage.session.copied': 'Lookup link copied',
  'keyUsage.session.copyFailed': 'Copy failed',
  'keyUsage.session.shareWarning': 'Anyone holding this link can see this Key usage.',
  'keyUsage.session.exit': 'Sign out',
  'keyUsage.session.cleared': 'Lookup link cleared',
  'keyUsage.session.expired': 'This lookup link is no longer valid, please enter your Key again',
  'keyUsage.windows.today': 'Today',
  'keyUsage.windows.last7d': 'Last 7 Days',
  'keyUsage.windows.last30d': 'Last 30 Days',
  'keyUsage.windows.all': 'All Time',
  'keyUsage.explorer.title': 'Usage & Rankings',
  'keyUsage.explorer.window': 'Time range',
  'keyUsage.explorer.hint': 'Combine a time range, a leaderboard and a sort metric',
  'keyUsage.rankings.scope': 'Ranking scope',
  'keyUsage.windows.modelsTitle': 'Model Breakdown',
  'keyUsage.windows.empty': 'No usage in this window',
  'keyUsage.windows.noModels': 'No model usage in this window',
  'keyUsage.rankings.scopeAccount': 'Account',
  'keyUsage.rankings.scopeSite': 'Site-wide',
  'keyUsage.rankings.scopeAccountHint': 'Compared with account Keys',
  'keyUsage.rankings.scopeSiteHint': 'Compared with site Keys',
  'keyUsage.rankings.metric': 'Sort by',
  'keyUsage.rankings.metricCost': 'Cost (USD)',
  'keyUsage.rankings.metricTokens': 'Total Tokens',
  'keyUsage.rankings.metricRequests': 'Requests',
  'keyUsage.rankings.rank': 'Rank',
  'keyUsage.rankings.keyName': 'Key Name',
  'keyUsage.rankings.you': 'You',
  'keyUsage.rankings.selfRank': 'You rank #{rank} of {total} Keys',
  'keyUsage.rankings.selfUnranked': 'You are not ranked in this window',
  'keyUsage.rankings.unavailable': 'Rankings are temporarily unavailable, please try again later',
  'keyUsage.rankings.podiumTied': '{medal} (tied)',
  'keyUsage.usageUnavailable': 'Usage details could not be loaded right now.',
  'keyUsage.rankings.empty': 'No ranking data in this window',
  'keyUsage.rankings.refreshing': 'Updating rankings...',
  'keyUsage.rankings.podiumGold': 'Champion',
  'keyUsage.rankings.podiumSilver': 'Runner-up',
  'keyUsage.rankings.podiumBronze': 'Third place',
  'home.viewDocs': 'Docs',
  'home.switchToLight': 'Light',
  'home.switchToDark': 'Dark',
  'home.footer.allRightsReserved': 'All rights reserved.',
}

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => {
        const raw = messages[key] ?? key
        if (!params) return raw
        return raw.replace(/\{(\w+)\}/g, (_match, name: string) => String(params[name] ?? ''))
      },
      locale: { value: 'en' },
    }),
  }
})

vi.mock('vue-router', () => ({
  useRoute: () => routeState,
  useRouter: () => routerMock,
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    cachedPublicSettings: null,
    siteName: 'Sub2API',
    siteLogo: '',
    docUrl: '',
    publicSettingsLoaded: true,
    fetchPublicSettings,
    showInfo,
    showSuccess,
    showError,
  }),
}))

// ==================== Fixtures ====================

const usagePayload = {
  mode: 'quota_limited',
  isValid: true,
  status: 'active',
  quota: { limit: 10, used: 1, remaining: 9, unit: 'USD' },
  usage: {
    today: {
      requests: 1,
      input_tokens: 10,
      output_tokens: 20,
      cache_creation_tokens: 0,
      cache_read_tokens: 0,
      total_tokens: 30,
      actual_cost: 0.01,
    },
    total: {
      requests: 12,
      input_tokens: 100,
      output_tokens: 200,
      cache_creation_tokens: 10,
      cache_read_tokens: 30,
      total_tokens: 340,
      actual_cost: 0.12,
    },
    rpm: 0,
    tpm: 0,
  },
  daily_usage: [
    {
      date: '2026-05-19',
      requests: 12,
      input_tokens: 100,
      output_tokens: 200,
      cache_read_tokens: 30,
      cache_write_tokens: 10,
      total_tokens: 340,
      cost: 0.15,
      actual_cost: 0.12,
    },
  ],
}

/**
 * One fixture per time window. The backend computes exactly one window per request, so the
 * fixtures are keyed the same way — a test that switches the time tab must see different data
 * come back, otherwise it proves nothing about the tab actually driving the request.
 */
const WINDOW_FIXTURES = {
  today: {
    stat: {
      requests: 12,
      tokens: 3400,
      cost_usd: 1.25,
      models: [{ model: 'claude-opus-5', requests: 10, tokens: 3000, cost_usd: 1 }],
    },
    ranking: {
      total_keys: 12,
      self_rank: 3,
      top: [
        { rank: 1, key_name: 'Alpha Key', requests: 100, tokens: 50000, cost_usd: 9.9, is_self: false },
        { rank: 2, key_name: 'Bravo Key', requests: 80, tokens: 40000, cost_usd: 7.7, is_self: false },
        { rank: 3, key_name: 'My Key', requests: 12, tokens: 3400, cost_usd: 1.25, is_self: true },
      ],
      self: { rank: 3, key_name: 'My Key', requests: 12, tokens: 3400, cost_usd: 1.25, is_self: true },
    },
  },
  last_7d: {
    stat: { requests: 90, tokens: 22000, cost_usd: 9.5, models: [] },
    ranking: {
      total_keys: 12,
      self_rank: 5,
      top: [{ rank: 1, key_name: 'Alpha Key', requests: 700, tokens: 350000, cost_usd: 69.9, is_self: false }],
      self: { rank: 5, key_name: 'My Key', requests: 90, tokens: 22000, cost_usd: 9.5, is_self: true },
    },
  },
  // Deliberately empty: exercises the empty-state branch.
  last_30d: {
    stat: { requests: 0, tokens: 0, cost_usd: 0, models: [] },
    ranking: { total_keys: 12, self_rank: 0, top: [], self: null },
  },
  // Big enough to exercise the M/B abbreviation everywhere on the page.
  all: {
    stat: {
      requests: 1_234_567,
      tokens: 2_500_000_000,
      cost_usd: 4321.5,
      models: [{ model: 'claude-opus-5', requests: 1_234_567, tokens: 2_500_000_000, cost_usd: 4321.5 }],
    },
    ranking: {
      total_keys: 40,
      self_rank: 1,
      top: [
        { rank: 1, key_name: 'My Key', requests: 1_234_567, tokens: 2_500_000_000, cost_usd: 4321.5, is_self: true },
      ],
      self: { rank: 1, key_name: 'My Key', requests: 1_234_567, tokens: 2_500_000_000, cost_usd: 4321.5, is_self: true },
    },
  },
} as const

type WindowKey = keyof typeof WINDOW_FIXTURES

function makeReport(metric = 'cost', window: WindowKey = 'today') {
  const fixture = WINDOW_FIXTURES[window] ?? WINDOW_FIXTURES.today
  return {
    key: { name: 'My Key', created_at: '2026-01-02T03:04:05Z', status: 'active' },
    usage: usagePayload,
    window,
    window_stat: fixture.stat,
    rankings: {
      account: fixture.ranking,
      site: fixture.ranking,
    },
    metric,
    generated_at: '2026-05-19T10:00:00Z',
  }
}

// ==================== Fetch routing ====================

interface FetchOverrides {
  session?: () => unknown
  report?: (url: string) => unknown
}

let overrides: FetchOverrides = {}

function jsonResponse(body: unknown, ok = true, status = 200) {
  return { ok, status, json: async () => body }
}

function installFetch() {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: unknown) => {
      const url = String(input)
      if (url.includes('/key-usage/session')) {
        return overrides.session ? overrides.session() : jsonResponse({ token: 'tok-1', expires_at: '2026-05-20T10:00:00Z' })
      }
      if (url.includes('/key-usage/report')) {
        if (overrides.report) return overrides.report(url)
        const params = new URL(url, 'http://localhost').searchParams
        const metric = params.get('metric') || 'cost'
        const window = (params.get('window') || 'today') as WindowKey
        return jsonResponse(makeReport(metric, window))
      }
      throw new Error(`unexpected fetch: ${url}`)
    })
  )
}

function mountView() {
  return mount(KeyUsageView, {
    global: {
      stubs: {
        RouterLink: { template: '<a><slot /></a>' },
        LocaleSwitcher: true,
        Icon: true,
      },
    },
  })
}

function fetchCalls(): string[] {
  return vi.mocked(fetch).mock.calls.map(call => String(call[0]))
}

function reportCalls(): string[] {
  return fetchCalls().filter(url => url.includes('/key-usage/report'))
}

async function settle() {
  await flushPromises()
  await nextTick()
  await flushPromises()
}

describe('KeyUsageView', () => {
  beforeEach(() => {
    showInfo.mockReset()
    showSuccess.mockReset()
    showError.mockReset()
    fetchPublicSettings.mockReset()
    routerMock.replace.mockClear()
    routerMock.push.mockClear()
    routeState.query = {}
    overrides = {}
    localStorage.clear()

    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: vi.fn().mockReturnValue({ matches: false }),
    })
    vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => window.setTimeout(() => cb(0), 0))
    installFetch()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  // ==================== Usage payload availability ====================

  describe('usage payload availability', () => {
    it('warns when the backend could not assemble the usage payload', async () => {
      overrides.report = () => jsonResponse({ ...makeReport(), usage: null, usage_available: false })

      const wrapper = mountView()
      await wrapper.find('input').setValue('sk-test-key')
      await wrapper.find('input').trigger('keydown.enter')
      await settle()

      // Silently degrading to an empty object + HTTP 200 makes "the backend broke"
      // indistinguishable from "this Key has never been used".
      expect(wrapper.find('[data-testid="usage-unavailable"]').exists()).toBe(true)
      expect(wrapper.text()).toContain('Usage details could not be loaded right now.')
      // The parts that did load are still rendered.
      expect(wrapper.find('[data-testid="windows-section"]').exists()).toBe(true)

      wrapper.unmount()
    })

    it('does not warn when the usage payload is available', async () => {
      const wrapper = mountView()
      await wrapper.find('input').setValue('sk-test-key')
      await wrapper.find('input').trigger('keydown.enter')
      await settle()

      expect(wrapper.find('[data-testid="usage-unavailable"]').exists()).toBe(false)

      wrapper.unmount()
    })
  })

  // ==================== Legacy usage panels ====================

  describe('daily detail', () => {
    it('renders daily usage detail rows after a successful query', async () => {
      const wrapper = mountView()

      await wrapper.find('input').setValue('sk-test-key')
      await wrapper.find('input').trigger('keydown.enter')
      await settle()

      const fetchMock = vi.mocked(fetch)
      // The raw key is exchanged for a lookup token; it never travels in the URL.
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/key-usage/session',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ key: 'sk-test-key' }),
        })
      )
      expect(reportCalls()).toHaveLength(1)
      expect(reportCalls()[0]).toContain('/api/v1/key-usage/report?')
      expect(reportCalls()[0]).toContain('token=tok-1')
      expect(reportCalls()[0]).toContain('days=30')

      const text = wrapper.text()
      expect(text).toContain('Daily Detail')
      expect(text).toContain('Date')
      expect(text).toContain('Cache Read')
      expect(text).toContain('Cache Write')
      expect(text).toContain('2026-05-19')
      expect(text).toContain('12')
      expect(text).toContain('100')
      expect(text).toContain('200')
      expect(text).toContain('30')
      expect(text).toContain('10')
      expect(text).toContain('$0.12')

      wrapper.unmount()
    })

    it('queries the current local calendar date near midnight', async () => {
      vi.useFakeTimers()
      vi.setSystemTime(new Date(2026, 6, 13, 0, 30))

      const wrapper = mountView()

      await wrapper.find('input').setValue('sk-test-key')
      await wrapper.find('input').trigger('keydown.enter')
      await flushPromises()

      const requestUrl = reportCalls()[0]
      expect(requestUrl).toContain('start_date=2026-07-13')
      expect(requestUrl).toContain('end_date=2026-07-13')

      wrapper.unmount()
    })
  })

  // ==================== URL token persistence ====================

  describe('lookup token in the URL', () => {
    it('replaces (never pushes) the URL with the issued token after a successful query', async () => {
      const wrapper = mountView()

      await wrapper.find('input').setValue('sk-test-key')
      await wrapper.find('input').trigger('keydown.enter')
      await settle()

      // Defaults are omitted from the URL, so a fresh lookup link stays clean.
      expect(routerMock.replace).toHaveBeenCalledWith({ query: { t: 'tok-1' } })
      expect(routerMock.push).not.toHaveBeenCalled()
      // The raw key must never be written into the URL.
      const replaced = JSON.stringify(routerMock.replace.mock.calls)
      expect(replaced).not.toContain('sk-test-key')

      wrapper.unmount()
    })

    it('auto-queries from ?t= on mount and hides the key input', async () => {
      routeState.query = { t: 'tok-from-url' }

      const wrapper = mountView()
      await settle()

      expect(fetchCalls().some(url => url.includes('/key-usage/session'))).toBe(false)
      expect(reportCalls()[0]).toContain('token=tok-from-url')

      expect(wrapper.find('input[type="password"]').exists()).toBe(false)
      expect(wrapper.find('[data-testid="session-bar"]').exists()).toBe(true)
      expect(wrapper.text()).toContain('Lookup link for My Key')
      expect(wrapper.text()).toContain('Anyone holding this link can see this Key usage.')
      expect(wrapper.text()).toContain('Daily Detail')

      wrapper.unmount()
    })

    it('mirrors the three selectors into the URL with replace, never push', async () => {
      routeState.query = { t: 'tok-from-url' }
      const wrapper = mountView()
      await settle()
      routerMock.replace.mockClear()

      await wrapper.find('[data-testid="window-tab"][data-window="all"]').trigger('click')
      await settle()
      expect(routerMock.replace).toHaveBeenLastCalledWith({ query: { t: 'tok-from-url', w: 'all' } })

      await wrapper.find('[data-testid="scope-tab"][data-scope="site"]').trigger('click')
      await settle()
      expect(routerMock.replace).toHaveBeenLastCalledWith({ query: { t: 'tok-from-url', w: 'all', s: 'site' } })

      await wrapper.find('[data-testid="metric-tab"][data-metric="tokens"]').trigger('click')
      await settle()
      expect(routerMock.replace).toHaveBeenLastCalledWith({
        query: { t: 'tok-from-url', w: 'all', s: 'site', m: 'tokens' },
      })

      // Flipping a filter tab must never grow the browser back stack.
      expect(routerMock.push).not.toHaveBeenCalled()

      wrapper.unmount()
    })

    it('omits selectors that are still on their default from the URL', async () => {
      routeState.query = { t: 'tok-from-url', w: 'all' }
      const wrapper = mountView()
      await settle()
      routerMock.replace.mockClear()

      await wrapper.find('[data-testid="window-tab"][data-window="today"]').trigger('click')
      await settle()

      expect(routerMock.replace).toHaveBeenLastCalledWith({ query: { t: 'tok-from-url' } })

      wrapper.unmount()
    })

    it('restores a shared view (window / scope / metric) from the URL on mount', async () => {
      routeState.query = { t: 'tok-from-url', w: 'all', s: 'site', m: 'tokens' }

      const wrapper = mountView()
      await settle()

      // The very first request already carries the shared view — no default flash first.
      expect(reportCalls()).toHaveLength(1)
      expect(reportCalls()[0]).toContain('window=all')
      expect(reportCalls()[0]).toContain('metric=tokens')

      expect(wrapper.find('[data-testid="window-tab"][data-window="all"]').attributes('aria-pressed')).toBe('true')
      expect(wrapper.find('[data-testid="scope-tab"][data-scope="site"]').attributes('aria-pressed')).toBe('true')
      expect(wrapper.find('[data-testid="metric-tab"][data-metric="tokens"]').attributes('aria-pressed')).toBe('true')
      expect(wrapper.text()).toContain('Compared with site Keys')

      wrapper.unmount()
    })

    it('ignores unrecognised selector values in the URL', async () => {
      routeState.query = { t: 'tok-from-url', w: 'last_millennium', s: 'galaxy', m: 'vibes' }

      const wrapper = mountView()
      await settle()

      expect(reportCalls()[0]).toContain('window=today')
      expect(reportCalls()[0]).toContain('metric=cost')
      expect(wrapper.find('[data-testid="window-tab"][data-window="today"]').attributes('aria-pressed')).toBe('true')
      expect(wrapper.find('[data-testid="scope-tab"][data-scope="account"]').attributes('aria-pressed')).toBe('true')

      wrapper.unmount()
    })

    it('drops the token and returns to the input box when the report answers 401', async () => {
      routeState.query = { t: 'expired-token' }
      overrides.report = () => jsonResponse({ error: { message: 'token expired' } }, false, 401)

      const wrapper = mountView()
      await settle()

      expect(routerMock.replace).toHaveBeenCalledWith({ query: {} })
      expect(showError).toHaveBeenCalledWith('This lookup link is no longer valid, please enter your Key again')
      expect(wrapper.find('[data-testid="session-bar"]').exists()).toBe(false)
      expect(wrapper.find('input[type="password"]').exists()).toBe(true)
      expect(wrapper.find('[data-testid="rankings-section"]').exists()).toBe(false)

      wrapper.unmount()
    })

    it('clears the URL parameter and local state when signing out', async () => {
      routeState.query = { t: 'tok-from-url' }

      const wrapper = mountView()
      await settle()
      routerMock.replace.mockClear()

      await wrapper.find('[data-testid="exit-session"]').trigger('click')
      await settle()

      expect(routerMock.replace).toHaveBeenCalledWith({ query: {} })
      expect(wrapper.find('[data-testid="session-bar"]').exists()).toBe(false)
      expect(wrapper.find('input[type="password"]').exists()).toBe(true)
      expect(wrapper.find('[data-testid="rankings-section"]').exists()).toBe(false)

      wrapper.unmount()
    })

    it('copies a share link that carries the token', async () => {
      routeState.query = { t: 'tok-from-url' }
      const writeText = vi.fn().mockResolvedValue(undefined)
      Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })

      const wrapper = mountView()
      await settle()

      await wrapper.find('[data-testid="copy-share-link"]').trigger('click')
      await settle()

      expect(writeText).toHaveBeenCalledTimes(1)
      expect(String(writeText.mock.calls[0][0])).toContain('t=tok-from-url')
      expect(showSuccess).toHaveBeenCalledWith('Lookup link copied')

      wrapper.unmount()
    })
  })

  // ==================== Podium & rankings ====================

  describe('rankings', () => {
    async function mountWithToken() {
      routeState.query = { t: 'tok-from-url' }
      const wrapper = mountView()
      await settle()
      return wrapper
    }

    it('lays the podium out as silver (left), gold (center), bronze (right)', async () => {
      const wrapper = await mountWithToken()

      const todayWindow = wrapper.find('[data-testid="ranking-window"][data-window="today"]')
      expect(todayWindow.exists()).toBe(true)

      const slots = todayWindow.findAll('[data-testid="podium-slot"]')
      expect(slots).toHaveLength(3)
      expect(slots.map(slot => slot.attributes('data-rank'))).toEqual(['2', '1', '3'])
      expect(slots.map(slot => slot.attributes('data-medal'))).toEqual(['silver', 'gold', 'bronze'])
      expect(slots.map(slot => slot.text())).toEqual([
        expect.stringContaining('Bravo Key'),
        expect.stringContaining('Alpha Key'),
        expect.stringContaining('My Key'),
      ])

      wrapper.unmount()
    })

    it('keeps a readable 1-2-3 order on narrow screens via order utilities', async () => {
      const wrapper = await mountWithToken()

      const slots = wrapper
        .find('[data-testid="ranking-window"][data-window="today"]')
        .findAll('[data-testid="podium-slot"]')

      // DOM order is 2/1/3 for the desktop podium; the mobile column reorders to 1/2/3.
      expect(slots[0].classes()).toContain('order-2')
      expect(slots[1].classes()).toContain('order-1')
      expect(slots[2].classes()).toContain('order-3')
      slots.forEach(slot => expect(slot.classes()).toContain('sm:order-none'))

      wrapper.unmount()
    })

    it('highlights the querying key via is_self', async () => {
      const wrapper = await mountWithToken()

      const todayWindow = wrapper.find('[data-testid="ranking-window"][data-window="today"]')

      const selfSlot = todayWindow.find('[data-testid="podium-slot"][data-rank="3"]')
      expect(selfSlot.find('[data-testid="podium-self-badge"]').exists()).toBe(true)
      expect(selfSlot.html()).toContain('ring-primary-500')

      const rows = todayWindow.findAll('[data-testid="ranking-row"]')
      const selfRows = rows.filter(row => row.attributes('data-self') === 'true')
      expect(selfRows).toHaveLength(1)
      expect(selfRows[0].text()).toContain('My Key')
      expect(selfRows[0].find('[data-testid="ranking-self-badge"]').exists()).toBe(true)

      expect(todayWindow.text()).toContain('You rank #3 of 12 Keys')

      wrapper.unmount()
    })

    it('renders exactly one window at a time, switched by the time tab', async () => {
      const wrapper = await mountWithToken()

      // One ranking block on screen, never a vertical stack of three.
      expect(wrapper.findAll('[data-testid="ranking-window"]')).toHaveLength(1)
      expect(wrapper.find('[data-testid="ranking-window"]').attributes('data-window')).toBe('today')

      // A single-entry window still renders, with just the gold slot.
      await wrapper.find('[data-testid="window-tab"][data-window="last_7d"]').trigger('click')
      await settle()

      const week = wrapper.find('[data-testid="ranking-window"]')
      expect(week.attributes('data-window')).toBe('last_7d')
      const weekSlots = week.findAll('[data-testid="podium-slot"]')
      expect(weekSlots).toHaveLength(1)
      expect(weekSlots[0].attributes('data-medal')).toBe('gold')

      wrapper.unmount()
    })

    it('renders the empty ranking state for a window without data', async () => {
      const wrapper = await mountWithToken()

      await wrapper.find('[data-testid="window-tab"][data-window="last_30d"]').trigger('click')
      await settle()

      const empty = wrapper.find('[data-testid="ranking-window"]')
      expect(empty.attributes('data-window')).toBe('last_30d')
      expect(empty.find('[data-testid="ranking-window-empty"]').exists()).toBe(true)
      expect(empty.findAll('[data-testid="podium-slot"]')).toHaveLength(0)
      expect(empty.text()).toContain('No ranking data in this window')

      wrapper.unmount()
    })

    it('switches the ranking scope between account and site', async () => {
      const wrapper = await mountWithToken()
      const callsBefore = reportCalls().length

      await wrapper.find('[data-testid="scope-tab"][data-scope="site"]').trigger('click')
      await nextTick()

      expect(wrapper.text()).toContain('Compared with site Keys')
      // Scope is a pure client-side pivot on data already fetched.
      expect(reportCalls()).toHaveLength(callsBefore)

      wrapper.unmount()
    })

    it('re-requests with the selected metric and re-renders without a skeleton', async () => {
      const wrapper = await mountWithToken()
      expect(reportCalls()[0]).toContain('metric=cost')

      await wrapper.find('[data-testid="metric-tab"][data-metric="tokens"]').trigger('click')
      await settle()

      const calls = reportCalls()
      expect(calls).toHaveLength(2)
      expect(calls[1]).toContain('metric=tokens')
      expect(calls[1]).toContain('token=tok-from-url')

      // Data stays on screen through the switch: no loading skeleton is mounted.
      expect(wrapper.find('.skeleton').exists()).toBe(false)
      expect(wrapper.find('[data-testid="rankings-section"]').exists()).toBe(true)

      const tokensTab = wrapper.find('[data-testid="metric-tab"][data-metric="tokens"]')
      expect(tokensTab.attributes('aria-pressed')).toBe('true')
      // Total-tokens column becomes the emphasised one.
      const firstRow = wrapper.find('[data-testid="ranking-row"]')
      expect(firstRow.text()).toContain('50,000')

      wrapper.unmount()
    })

    it('re-requests with the selected time window and re-renders without a skeleton', async () => {
      const wrapper = await mountWithToken()
      expect(reportCalls()[0]).toContain('window=today')

      await wrapper.find('[data-testid="window-tab"][data-window="all"]').trigger('click')
      await settle()

      const calls = reportCalls()
      expect(calls).toHaveLength(2)
      expect(calls[1]).toContain('window=all')
      expect(calls[1]).toContain('token=tok-from-url')
      // The window tab must not disturb the other two dimensions.
      expect(calls[1]).toContain('metric=cost')

      // Data stays on screen through the switch: no loading skeleton is mounted.
      expect(wrapper.find('.skeleton').exists()).toBe(false)
      expect(wrapper.find('[data-testid="rankings-section"]').exists()).toBe(true)
      expect(wrapper.find('[data-testid="window-tab"][data-window="all"]').attributes('aria-pressed')).toBe('true')
      expect(wrapper.find('[data-testid="window-summary"]').attributes('data-window')).toBe('all')

      wrapper.unmount()
    })

    it('offers all four time windows and renders each one it is switched to', async () => {
      const wrapper = await mountWithToken()

      const tabs = wrapper.findAll('[data-testid="window-tab"]')
      expect(tabs.map(tab => tab.attributes('data-window'))).toEqual(['today', 'last_7d', 'last_30d', 'all'])
      expect(tabs.map(tab => tab.text())).toEqual(['Today', 'Last 7 Days', 'Last 30 Days', 'All Time'])

      for (const window of ['last_7d', 'last_30d', 'all', 'today']) {
        await wrapper.find(`[data-testid="window-tab"][data-window="${window}"]`).trigger('click')
        await settle()
        expect(wrapper.find('[data-testid="window-summary"]').attributes('data-window')).toBe(window)
        expect(wrapper.find('[data-testid="ranking-window"]').attributes('data-window')).toBe(window)
      }

      wrapper.unmount()
    })

    it('keeps the current report on screen when a window switch fails', async () => {
      const wrapper = await mountWithToken()

      overrides.report = () => jsonResponse({ error: { message: 'window boom' } }, false, 500)
      await wrapper.find('[data-testid="window-tab"][data-window="all"]').trigger('click')
      await settle()

      expect(showError).toHaveBeenCalledWith('window boom')
      expect(wrapper.find('[data-testid="rankings-section"]').exists()).toBe(true)
      // The selection rolls back to the window that is actually rendered.
      expect(wrapper.find('[data-testid="window-tab"][data-window="today"]').attributes('aria-pressed')).toBe('true')
      expect(wrapper.find('[data-testid="window-summary"]').attributes('data-window')).toBe('today')

      wrapper.unmount()
    })

    it('trusts the window the backend echoes back over the one that was requested', async () => {
      const wrapper = await mountWithToken()

      // The backend silently falls back for an unrecognised window; the page must follow it
      // rather than labelling today-sized numbers "All Time".
      overrides.report = () => jsonResponse(makeReport('cost', 'today'))
      await wrapper.find('[data-testid="window-tab"][data-window="all"]').trigger('click')
      await settle()

      expect(wrapper.find('[data-testid="window-summary"]').attributes('data-window')).toBe('today')
      expect(wrapper.find('[data-testid="window-tab"][data-window="today"]').attributes('aria-pressed')).toBe('true')

      wrapper.unmount()
    })

    it('groups the three selectors together in one filter panel', async () => {
      const wrapper = await mountWithToken()

      const filters = wrapper.find('[data-testid="usage-filters"]')
      expect(filters.exists()).toBe(true)
      // All three dimensions live in the same panel, not scattered down the page.
      expect(filters.findAll('[data-testid="window-tab"]')).toHaveLength(4)
      expect(filters.findAll('[data-testid="scope-tab"]')).toHaveLength(2)
      expect(filters.findAll('[data-testid="metric-tab"]')).toHaveLength(3)
      expect(filters.text()).toContain('Time range')
      expect(filters.text()).toContain('Ranking scope')
      expect(filters.text()).toContain('Sort by')

      wrapper.unmount()
    })

    it('abbreviates large token counts and keeps the exact value in the title', async () => {
      const wrapper = await mountWithToken()

      await wrapper.find('[data-testid="window-tab"][data-window="all"]').trigger('click')
      await settle()

      const summary = wrapper.find('[data-testid="window-summary"]')
      expect(summary.text()).toContain('2.50B')
      expect(summary.text()).toContain('1.23M')
      expect(summary.text()).not.toContain('2,500,000,000')

      const titles = summary.findAll('dd').map(dd => dd.attributes('title'))
      expect(titles).toContain('2,500,000,000')
      expect(titles).toContain('1,234,567')

      // Cost keeps its exact $x.xx form — abbreviation is for counts only.
      expect(summary.text()).toContain('$4321.50')

      // Same treatment in the ranking table and the model breakdown.
      const rankCells = wrapper.findAll('[data-testid="ranking-row"] td')
      expect(rankCells.some(td => td.attributes('title') === '2,500,000,000')).toBe(true)
      const modelCells = wrapper.findAll('[data-testid="window-model-row"] td')
      expect(modelCells.some(td => td.attributes('title') === '2,500,000,000')).toBe(true)
      expect(wrapper.find('[data-testid="window-model-table"]').text()).toContain('2.50B')

      wrapper.unmount()
    })

    it('keeps the current report on screen when a metric switch fails', async () => {
      const wrapper = await mountWithToken()

      overrides.report = () => jsonResponse({ error: { message: 'boom' } }, false, 500)
      await wrapper.find('[data-testid="metric-tab"][data-metric="requests"]').trigger('click')
      await settle()

      expect(showError).toHaveBeenCalledWith('boom')
      expect(wrapper.find('[data-testid="rankings-section"]').exists()).toBe(true)
      // The metric selection rolls back to what is actually rendered.
      expect(wrapper.find('[data-testid="metric-tab"][data-metric="cost"]').attributes('aria-pressed')).toBe('true')

      wrapper.unmount()
    })
  })

  // ==================== Key handling disclosure ====================

  describe('key handling disclosure', () => {
    it('states that the key is sent to the server, before the visitor pastes one', async () => {
      const wrapper = mountView()
      await nextTick()

      const note = wrapper.find('[data-testid="privacy-note"]')
      expect(note.exists()).toBe(true)
      expect(note.text()).toContain('sent to this site’s server')
      expect(note.text()).toContain('cannot call the API')
      // Guards against re-introducing the old, now-false "never leaves the browser" claim.
      expect(note.text()).not.toMatch(/processed locally|never leaves|stays in your browser/i)

      wrapper.unmount()
    })

    it('keeps the disclosure out of the way once a lookup link is in use', async () => {
      routeState.query = { t: 'tok-from-url' }
      const wrapper = mountView()
      await settle()

      // No key is entered in this mode; the share warning is the relevant notice instead.
      expect(wrapper.find('[data-testid="privacy-note"]').exists()).toBe(false)
      expect(wrapper.text()).toContain('Anyone holding this link can see this Key usage.')

      wrapper.unmount()
    })
  })

  // ==================== Per-window statistics ====================

  describe('window statistics', () => {
    it('renders requests / tokens / cost and the model breakdown for the selected window', async () => {
      routeState.query = { t: 'tok-from-url' }
      const wrapper = mountView()
      await settle()

      // One summary card, for the window currently selected — not three stacked cards.
      const summaries = wrapper.findAll('[data-testid="window-summary"]')
      expect(summaries).toHaveLength(1)
      expect(summaries[0].attributes('data-window')).toBe('today')
      expect(summaries[0].text()).toContain('3,400')
      expect(summaries[0].text()).toContain('$1.25')

      // Today's models are shown by default.
      expect(wrapper.find('[data-testid="window-model-table"]').exists()).toBe(true)
      expect(wrapper.text()).toContain('claude-opus-5')

      wrapper.unmount()
    })

    it('renders an empty state for a window with no data instead of blank space', async () => {
      routeState.query = { t: 'tok-from-url' }
      const wrapper = mountView()
      await settle()

      await wrapper.find('[data-testid="window-tab"][data-window="last_30d"]').trigger('click')
      await settle()

      const empty = wrapper.find('[data-testid="window-summary"]')
      expect(empty.attributes('data-window')).toBe('last_30d')
      expect(empty.find('[data-testid="window-summary-empty"]').exists()).toBe(true)
      expect(empty.text()).toContain('No usage in this window')

      expect(wrapper.find('[data-testid="window-model-table"]').exists()).toBe(false)
      expect(wrapper.find('[data-testid="window-model-empty"]').exists()).toBe(true)
      expect(wrapper.text()).toContain('No model usage in this window')

      wrapper.unmount()
    })

    it('degrades gracefully when the backend omits windows and rankings', async () => {
      routeState.query = { t: 'tok-from-url' }
      overrides.report = () => jsonResponse({ key: null, usage: usagePayload, metric: 'cost', window: 'today' })

      const wrapper = mountView()
      await settle()

      expect(wrapper.find('[data-testid="windows-section"]').exists()).toBe(false)
      expect(wrapper.find('[data-testid="rankings-section"]').exists()).toBe(false)
      expect(wrapper.find('[data-testid="usage-filters"]').exists()).toBe(false)
      // The legacy usage panels still render.
      expect(wrapper.text()).toContain('Daily Detail')

      wrapper.unmount()
    })
  })
})
