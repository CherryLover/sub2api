import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'

import AccountUsageAlertRulesCard from '../AccountUsageAlertRulesCard.vue'

const {
  listAlertRules,
  createAlertRule,
  updateAlertRule,
  deleteAlertRule,
  evaluateAlertRule,
  listAccounts,
  getAllGroups,
  showError,
  showSuccess,
  showInfo,
  showWarning,
} = vi.hoisted(() => ({
  listAlertRules: vi.fn(),
  createAlertRule: vi.fn(),
  updateAlertRule: vi.fn(),
  deleteAlertRule: vi.fn(),
  evaluateAlertRule: vi.fn(),
  listAccounts: vi.fn(),
  getAllGroups: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  showInfo: vi.fn(),
  showWarning: vi.fn(),
}))

vi.mock('@/api', () => ({
  adminAPI: {
    accounts: { list: listAccounts },
    groups: { getAll: getAllGroups },
  },
}))

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    listAlertRules,
    createAlertRule,
    updateAlertRule,
    deleteAlertRule,
    evaluateAlertRule,
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showWarning,
    showInfo,
  }),
}))

// t() 原样返回键名；带参数时把参数值拼在后面，方便断言「成功 2 条，失败 1 条」这类插值
vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params ? `${key}:${Object.values(params).join(',')}` : key,
  }),
}))

const ToggleStub = defineComponent({
  props: { modelValue: { type: Boolean, default: false } },
  emits: ['update:modelValue'],
  inheritAttrs: false,
  setup(props, { attrs, emit }) {
    return () =>
      h('input', {
        ...attrs,
        type: 'checkbox',
        checked: props.modelValue,
        onChange: (event: Event) => {
          emit('update:modelValue', (event.target as HTMLInputElement).checked)
        },
      })
  },
})

// 原生 <select> 只能吐字符串；组件里对 group_id / account_id 会自己 parseInt，所以这里不用转
const SelectStub = defineComponent({
  props: {
    modelValue: { type: [String, Number, Boolean, null], default: '' },
    options: { type: Array, default: () => [] },
    disabled: { type: Boolean, default: false },
  },
  emits: ['update:modelValue'],
  inheritAttrs: false,
  setup(props, { attrs, emit }) {
    return () =>
      h(
        'select',
        {
          ...attrs,
          disabled: props.disabled,
          value: props.modelValue ?? '',
          onChange: (event: Event) => {
            emit('update:modelValue', (event.target as HTMLSelectElement).value)
          },
        },
        (props.options as Array<{ value: string | number | null; label: string }>).map((option) =>
          h('option', { key: String(option.value), value: option.value ?? '' }, option.label),
        ),
      )
  },
})

// BaseDialog 真身会 Teleport 到 body，测试里换成就地渲染的壳
const BaseDialogStub = defineComponent({
  props: { show: { type: Boolean, default: false }, title: { type: String, default: '' } },
  emits: ['close'],
  setup(props, { slots }) {
    return () =>
      props.show
        ? h('div', { 'data-testid': 'dialog', 'data-title': props.title }, [
            slots.default?.(),
            slots.footer?.(),
          ])
        : null
  },
})

const ConfirmDialogStub = defineComponent({
  props: {
    show: { type: Boolean, default: false },
    title: { type: String, default: '' },
    message: { type: String, default: '' },
  },
  emits: ['confirm', 'cancel'],
  setup(props, { emit }) {
    return () =>
      props.show
        ? h('button', { 'data-testid': 'confirm-delete', onClick: () => emit('confirm') }, 'confirm')
        : null
  },
})

const systemRule = () => ({
  id: 1,
  name: 'Error rate',
  description: '',
  enabled: true,
  metric_type: 'error_rate',
  operator: '>',
  threshold: 1,
  window_minutes: 5,
  sustained_minutes: 2,
  severity: 'P1',
  cooldown_minutes: 10,
  notify_email: false,
})

const windowRule = () => ({
  id: 2,
  name: 'Codex 7d',
  description: 'keep me',
  enabled: true,
  metric_type: 'account_window_used_percent',
  operator: '>=',
  threshold: 80,
  window_minutes: 1,
  sustained_minutes: 3,
  severity: 'P2',
  cooldown_minutes: 60,
  notify_email: true,
  filters: { window: '7d', platform: 'openai', account_ids: [11, 12] },
})

const balanceRule = () => ({
  id: 3,
  name: 'Kimi balance',
  description: '',
  enabled: true,
  metric_type: 'account_balance',
  operator: '<',
  threshold: 10,
  window_minutes: 1,
  sustained_minutes: 1,
  severity: 'P1',
  cooldown_minutes: 120,
  notify_email: false,
  filters: { provider: 'kimi', group_id: 5 },
})

const accountsPage = (items: unknown[]) => ({
  items,
  total: items.length,
  page: 1,
  page_size: 500,
  pages: 1,
})

const openaiAccounts = () => [
  { id: 11, name: 'codex-a', platform: 'openai', type: 'oauth', status: 'active' },
  { id: 12, name: 'codex-b', platform: 'openai', type: 'oauth', status: 'active' },
]
const kimiAccounts = () => [{ id: 21, name: 'kimi-a', platform: 'kimi', type: 'apikey', status: 'active' }]

async function mountCard() {
  const wrapper = mount(AccountUsageAlertRulesCard, {
    global: {
      stubs: {
        Toggle: ToggleStub,
        Select: SelectStub,
        BaseDialog: BaseDialogStub,
        ConfirmDialog: ConfirmDialogStub,
        Icon: true,
      },
    },
  })
  await flushPromises()
  return wrapper
}

const field = (wrapper: Awaited<ReturnType<typeof mountCard>>, id: string) =>
  wrapper.find(`[data-testid="${id}"]`)

describe('AccountUsageAlertRulesCard', () => {
  beforeEach(() => {
    listAlertRules.mockReset()
    createAlertRule.mockReset()
    updateAlertRule.mockReset()
    deleteAlertRule.mockReset()
    evaluateAlertRule.mockReset()
    listAccounts.mockReset()
    getAllGroups.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    showInfo.mockReset()
    showWarning.mockReset()

    listAlertRules.mockResolvedValue([systemRule(), windowRule(), balanceRule()])
    getAllGroups.mockResolvedValue([
      { id: 5, name: 'kimi-pool', platform: 'kimi' },
      { id: 6, name: 'codex-pool', platform: 'openai' },
    ])
    listAccounts.mockImplementation((_page: number, _size: number, filters?: { platform?: string }) => {
      if (filters?.platform === 'openai') return Promise.resolve(accountsPage(openaiAccounts()))
      if (filters?.platform === 'kimi') return Promise.resolve(accountsPage(kimiAccounts()))
      return Promise.resolve(accountsPage([...openaiAccounts(), ...kimiAccounts()]))
    })
  })

  it('loads all rules but only lists the four account usage metrics, with a readable scope and condition', async () => {
    const wrapper = await mountCard()

    expect(listAlertRules).toHaveBeenCalledTimes(1)
    expect(field(wrapper, 'usage-rule-row-1').exists()).toBe(false)
    expect(field(wrapper, 'usage-rule-row-2').exists()).toBe(true)
    expect(field(wrapper, 'usage-rule-row-3').exists()).toBe(true)
    expect(field(wrapper, 'usage-rules-empty').exists()).toBe(false)
    expect(field(wrapper, 'usage-rules-ops-disabled').exists()).toBe(false)

    const windowRow = field(wrapper, 'usage-rule-row-2')
    expect(windowRow.find('[data-testid="usage-rule-row-metric"]').text()).toBe(
      'admin.settings.notifications.accountUsageRules.metrics.windowUsedPercent',
    )
    const windowScope = windowRow.find('[data-testid="usage-rule-scope"]').text()
    expect(windowScope).toContain('openai')
    expect(windowScope).toContain('admin.settings.notifications.accountUsageRules.scope.accountCount:2')
    expect(windowScope).toContain('admin.settings.notifications.accountUsageRules.windows.d7')
    expect(windowRow.find('[data-testid="usage-rule-condition"]').text()).toBe('>= 80%')

    const balanceRow = field(wrapper, 'usage-rule-row-3')
    const balanceScope = balanceRow.find('[data-testid="usage-rule-scope"]').text()
    expect(balanceScope).toContain('admin.settings.notifications.accountUsageRules.scope.group:kimi-pool')
    expect(balanceScope).toContain('admin.settings.notifications.accountUsageRules.providers.kimi')
    expect(balanceRow.find('[data-testid="usage-rule-condition"]').text()).toBe('< 10')
    expect(showError).not.toHaveBeenCalled()
  })

  it('shows the ops-disabled hint instead of an empty table when monitoring is off', async () => {
    listAlertRules.mockRejectedValue({
      status: 404,
      code: 'OPS_DISABLED',
      message: 'Ops monitoring is disabled',
    })

    const wrapper = await mountCard()

    expect(field(wrapper, 'usage-rules-ops-disabled').exists()).toBe(true)
    expect(field(wrapper, 'usage-rules-ops-disabled').text()).toContain(
      'admin.settings.notifications.accountUsageRules.opsDisabled',
    )
    expect(field(wrapper, 'usage-rules-empty').exists()).toBe(false)
    expect(field(wrapper, 'usage-rules-table').exists()).toBe(false)
    expect(field(wrapper, 'usage-rules-create').exists()).toBe(false)
    expect(showError).not.toHaveBeenCalled()
  })

  it('creates a rule with filters, window_minutes=1 and the platform-scoped account list', async () => {
    createAlertRule.mockResolvedValue({ ...windowRule(), id: 9, name: 'Codex 5h' })

    const wrapper = await mountCard()
    await field(wrapper, 'usage-rules-create').trigger('click')
    await flushPromises()

    expect(field(wrapper, 'usage-rule-editor').exists()).toBe(true)
    // 打开编辑器时先拉一次「全部平台」的账号
    expect(listAccounts).toHaveBeenCalledWith(1, 500, expect.objectContaining({ lite: '1' }))

    await field(wrapper, 'usage-rule-name').setValue('  Codex 5h  ')
    await field(wrapper, 'usage-rule-window').setValue('7d')
    await field(wrapper, 'usage-rule-platform').setValue('openai')
    await flushPromises()

    expect(listAccounts).toHaveBeenLastCalledWith(1, 500, { platform: 'openai', lite: '1' })
    const picker = field(wrapper, 'usage-rule-account-picker')
    expect(picker.findAll('option').map((o) => o.text())).toEqual(['codex-a', 'codex-b'])

    await picker.setValue('11')
    await flushPromises()
    expect(field(wrapper, 'usage-rule-account-chips').text()).toContain('codex-a')
    expect(field(wrapper, 'usage-rule-account-remove-11').exists()).toBe(true)
    // 已选的账号从候选里拿掉
    expect(field(wrapper, 'usage-rule-account-picker').findAll('option').map((o) => o.text())).toEqual(['codex-b'])

    await field(wrapper, 'usage-rule-group').setValue('6')
    await field(wrapper, 'usage-rule-operator').setValue('>=')
    await field(wrapper, 'usage-rule-threshold').setValue('85')
    await field(wrapper, 'usage-rule-severity').setValue('P1')
    await field(wrapper, 'usage-rule-cooldown').setValue('30')
    await field(wrapper, 'usage-rule-save').trigger('click')
    await flushPromises()

    expect(createAlertRule).toHaveBeenCalledTimes(1)
    expect(createAlertRule).toHaveBeenCalledWith({
      name: 'Codex 5h',
      description: '',
      enabled: true,
      metric_type: 'account_window_used_percent',
      operator: '>=',
      threshold: 85,
      window_minutes: 1,
      sustained_minutes: 1,
      severity: 'P1',
      cooldown_minutes: 30,
      notify_email: false,
      filters: { window: '7d', platform: 'openai', group_id: 6, account_ids: [11] },
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.settings.notifications.accountUsageRules.saveSuccess')
    expect(showError).not.toHaveBeenCalled()
    expect(listAlertRules).toHaveBeenCalledTimes(2)
    expect(field(wrapper, 'usage-rule-editor').exists()).toBe(false)
  })

  it('refuses to create a rule whose name already exists, without calling the API', async () => {
    const wrapper = await mountCard()
    await field(wrapper, 'usage-rules-create').trigger('click')
    await flushPromises()

    await field(wrapper, 'usage-rule-name').setValue('Kimi balance')
    expect(field(wrapper, 'usage-rule-editor-errors').text()).toContain(
      'admin.settings.notifications.accountUsageRules.duplicateName',
    )
    await field(wrapper, 'usage-rule-save').trigger('click')
    await flushPromises()

    expect(createAlertRule).not.toHaveBeenCalled()
    expect(showError).toHaveBeenCalledWith('admin.settings.notifications.accountUsageRules.duplicateName')
  })

  it('edits a rule: prefills scope from filters and keeps description / sustained / notify_email from the original', async () => {
    updateAlertRule.mockResolvedValue({ ...windowRule(), threshold: 90 })

    const wrapper = await mountCard()
    await field(wrapper, 'usage-rule-edit-2').trigger('click')
    await flushPromises()

    expect((field(wrapper, 'usage-rule-name').element as HTMLInputElement).value).toBe('Codex 7d')
    expect((field(wrapper, 'usage-rule-window').element as HTMLSelectElement).value).toBe('7d')
    expect((field(wrapper, 'usage-rule-platform').element as HTMLSelectElement).value).toBe('openai')
    expect(listAccounts).toHaveBeenCalledWith(1, 500, { platform: 'openai', lite: '1' })
    expect(field(wrapper, 'usage-rule-account-chips').text()).toContain('codex-a')
    expect(field(wrapper, 'usage-rule-account-chips').text()).toContain('codex-b')

    await field(wrapper, 'usage-rule-account-remove-12').trigger('click')
    await field(wrapper, 'usage-rule-threshold').setValue('90')
    await field(wrapper, 'usage-rule-save').trigger('click')
    await flushPromises()

    expect(updateAlertRule).toHaveBeenCalledWith(2, {
      name: 'Codex 7d',
      description: 'keep me',
      enabled: true,
      metric_type: 'account_window_used_percent',
      operator: '>=',
      threshold: 90,
      window_minutes: 1,
      sustained_minutes: 3,
      severity: 'P2',
      cooldown_minutes: 60,
      notify_email: true,
      filters: { window: '7d', platform: 'openai', account_ids: [11] },
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.settings.notifications.accountUsageRules.saveSuccess')
  })

  it('evaluates on open without sending, lists the accounts, then sends when the Bark box is ticked', async () => {
    evaluateAlertRule.mockResolvedValue({
      rule_id: 2,
      rule_name: 'Codex 7d',
      metric_type: 'account_window_used_percent',
      operator: '>=',
      threshold: 80,
      evaluated_at: '2026-09-05T10:00:00Z',
      has_data: true,
      value: 91.5,
      breached: true,
      accounts: [
        { account_id: 11, account_name: 'codex-a', platform: 'openai', value: 91.5, breached: true },
        { account_id: 12, account_name: 'codex-b', platform: 'openai', value: 40, breached: false },
      ],
      sent: false,
    })

    const wrapper = await mountCard()
    await field(wrapper, 'usage-rule-evaluate-2').trigger('click')
    await flushPromises()

    expect(evaluateAlertRule).toHaveBeenCalledTimes(1)
    expect(evaluateAlertRule).toHaveBeenCalledWith(2, false)
    expect(field(wrapper, 'usage-rule-evaluate-value').text()).toBe('91.5%')
    expect(field(wrapper, 'usage-rule-evaluate-breached').text()).toBe(
      'admin.settings.notifications.accountUsageRules.evaluate.breached',
    )
    const rows = field(wrapper, 'usage-rule-evaluate-accounts').findAll('tbody tr')
    expect(rows).toHaveLength(2)
    expect(rows[0].text()).toContain('codex-a')
    expect(rows[0].text()).toContain('91.5%')
    expect(rows[0].attributes('data-breached')).toBe('true')
    expect(rows[1].text()).toContain('codex-b')
    expect(rows[1].text()).toContain('40%')
    expect(rows[1].attributes('data-breached')).toBe('false')
    // 干跑不显示推送结果条
    expect(field(wrapper, 'usage-rule-evaluate-send-result').exists()).toBe(false)

    evaluateAlertRule.mockResolvedValueOnce({
      rule_id: 2,
      rule_name: 'Codex 7d',
      metric_type: 'account_window_used_percent',
      operator: '>=',
      threshold: 80,
      evaluated_at: '2026-09-05T10:01:00Z',
      has_data: true,
      value: 91.5,
      breached: true,
      accounts: [{ account_id: 11, account_name: 'codex-a', platform: 'openai', value: 91.5, breached: true }],
      sent: true,
    })
    await field(wrapper, 'usage-rule-evaluate-send').setValue(true)
    await field(wrapper, 'usage-rule-evaluate-run').trigger('click')
    await flushPromises()

    expect(evaluateAlertRule).toHaveBeenCalledTimes(2)
    expect(evaluateAlertRule).toHaveBeenLastCalledWith(2, true)
    const sendResult = field(wrapper, 'usage-rule-evaluate-send-result')
    expect(sendResult.exists()).toBe(true)
    expect(sendResult.attributes('data-sent')).toBe('true')
    expect(sendResult.text()).toContain('admin.settings.notifications.accountUsageRules.evaluate.sent')
  })

  it('shows send_error when the push failed, and a Bark hint when Bark is not enabled', async () => {
    evaluateAlertRule.mockResolvedValueOnce({
      rule_id: 3,
      rule_name: 'Kimi balance',
      metric_type: 'account_balance',
      operator: '<',
      threshold: 10,
      evaluated_at: '2026-09-05T10:00:00Z',
      has_data: true,
      value: 3.5,
      breached: true,
      accounts: [{ account_id: 21, account_name: 'kimi-a', platform: 'kimi', value: 3.5, breached: true, currency: 'CNY' }],
      sent: false,
      send_error: 'dial tcp: i/o timeout',
    })

    const wrapper = await mountCard()
    await field(wrapper, 'usage-rule-evaluate-3').trigger('click')
    await flushPromises()
    // 干跑一次后勾选推送再跑一次，后端说推送失败
    await field(wrapper, 'usage-rule-evaluate-send').setValue(true)
    evaluateAlertRule.mockResolvedValueOnce({
      rule_id: 3,
      rule_name: 'Kimi balance',
      metric_type: 'account_balance',
      operator: '<',
      threshold: 10,
      evaluated_at: '2026-09-05T10:00:30Z',
      has_data: true,
      value: 3.5,
      breached: true,
      accounts: [{ account_id: 21, account_name: 'kimi-a', platform: 'kimi', value: 3.5, breached: true, currency: 'CNY' }],
      sent: false,
      send_error: 'dial tcp: i/o timeout',
    })
    await field(wrapper, 'usage-rule-evaluate-run').trigger('click')
    await flushPromises()

    expect(evaluateAlertRule).toHaveBeenLastCalledWith(3, true)
    expect(field(wrapper, 'usage-rule-evaluate-value').text()).toBe('3.5 CNY')
    const sendResult = field(wrapper, 'usage-rule-evaluate-send-result')
    expect(sendResult.attributes('data-sent')).toBe('false')
    expect(sendResult.text()).toContain(
      'admin.settings.notifications.accountUsageRules.evaluate.sendError:dial tcp: i/o timeout',
    )

    evaluateAlertRule.mockRejectedValueOnce({
      status: 400,
      code: 400,
      reason: 'BARK_NOT_ENABLED',
      message: 'bark push is not enabled',
    })
    await field(wrapper, 'usage-rule-evaluate-run').trigger('click')
    await flushPromises()

    expect(field(wrapper, 'usage-rule-evaluate-error').text()).toBe(
      'admin.settings.notifications.accountUsageRules.evaluate.barkNotEnabled',
    )
    expect(field(wrapper, 'usage-rule-evaluate-accounts').exists()).toBe(false)
  })

  it('creates three tiers in order with 40% / 60% / 80% suffixes and thresholds', async () => {
    createAlertRule.mockImplementation((rule: { name: string }) => Promise.resolve({ ...rule, id: Math.random() }))

    const wrapper = await mountCard()
    await field(wrapper, 'usage-rules-create-tiers').trigger('click')
    await flushPromises()

    // 三档只对百分比指标有意义，余额 / 费用不出现在下拉里
    expect(
      field(wrapper, 'usage-rule-metric')
        .findAll('option')
        .map((o) => (o.element as HTMLOptionElement).value),
    ).toEqual([
      'account_window_used_percent',
      'account_quota_used_percent',
    ])
    expect(field(wrapper, 'usage-rule-operator').exists()).toBe(false)
    expect(field(wrapper, 'usage-rule-threshold').exists()).toBe(false)

    await field(wrapper, 'usage-rule-name').setValue('Codex 5h')
    expect(field(wrapper, 'usage-rule-tiers-preview').text()).toContain('Codex 5h 40% / Codex 5h 60% / Codex 5h 80%')
    await field(wrapper, 'usage-rule-platform').setValue('openai')
    await flushPromises()
    await field(wrapper, 'usage-rule-severity').setValue('P1')
    await field(wrapper, 'usage-rule-save').trigger('click')
    await flushPromises()

    expect(createAlertRule).toHaveBeenCalledTimes(3)
    const payloads = createAlertRule.mock.calls.map((call) => call[0])
    expect(payloads.map((p) => p.name)).toEqual(['Codex 5h 40%', 'Codex 5h 60%', 'Codex 5h 80%'])
    expect(payloads.map((p) => p.threshold)).toEqual([40, 60, 80])
    for (const p of payloads) {
      expect(p).toMatchObject({
        metric_type: 'account_window_used_percent',
        operator: '>=',
        window_minutes: 1,
        severity: 'P1',
        enabled: true,
        filters: { window: '5h', platform: 'openai' },
      })
    }
    const results = field(wrapper, 'usage-rule-tier-results').findAll('li')
    expect(results.map((li) => li.attributes('data-status'))).toEqual(['ok', 'ok', 'ok'])
    expect(showSuccess).toHaveBeenCalledWith('admin.settings.notifications.accountUsageRules.tiers.summary:3,0')
    expect(listAlertRules).toHaveBeenCalledTimes(2)
  })

  it('reports duplicate tier names one by one: known names are skipped locally, backend duplicates read as duplicates', async () => {
    listAlertRules.mockResolvedValue([
      ...[systemRule(), windowRule(), balanceRule()],
      { ...windowRule(), id: 7, name: 'Codex 5h 40%' },
    ])
    createAlertRule.mockImplementation((rule: { name: string }) => {
      if (rule.name === 'Codex 5h 80%') {
        return Promise.reject({
          status: 500,
          message: 'pq: duplicate key value violates unique constraint "idx_ops_alert_rules_name_unique"',
        })
      }
      return Promise.resolve({ ...rule, id: 8 })
    })

    const wrapper = await mountCard()
    await field(wrapper, 'usage-rules-create-tiers').trigger('click')
    await flushPromises()
    await field(wrapper, 'usage-rule-name').setValue('Codex 5h')
    await field(wrapper, 'usage-rule-save').trigger('click')
    await flushPromises()

    // 40% 本地已知重名不发请求，60% 成功，80% 后端报唯一约束
    expect(createAlertRule).toHaveBeenCalledTimes(2)
    expect(createAlertRule.mock.calls.map((call) => call[0].name)).toEqual(['Codex 5h 60%', 'Codex 5h 80%'])
    const results = field(wrapper, 'usage-rule-tier-results').findAll('li')
    expect(results.map((li) => li.attributes('data-status'))).toEqual(['skipped', 'ok', 'skipped'])
    expect(results[0].text()).toContain('admin.settings.notifications.accountUsageRules.tiers.duplicate')
    expect(results[2].text()).toContain('admin.settings.notifications.accountUsageRules.tiers.duplicate')
    expect(showWarning).toHaveBeenCalledWith('admin.settings.notifications.accountUsageRules.tiers.summary:1,2')
  })

  it('toggles a rule by PUTting the whole rule with enabled flipped', async () => {
    updateAlertRule.mockResolvedValue({ ...balanceRule(), enabled: false })

    const wrapper = await mountCard()
    await field(wrapper, 'usage-rule-toggle-3').setValue(false)
    await flushPromises()

    expect(updateAlertRule).toHaveBeenCalledTimes(1)
    expect(updateAlertRule).toHaveBeenCalledWith(3, { ...balanceRule(), enabled: false })
    expect((field(wrapper, 'usage-rule-toggle-3').element as HTMLInputElement).checked).toBe(false)
    expect(showError).not.toHaveBeenCalled()
  })

  it('deletes a rule after confirmation and reloads the list', async () => {
    deleteAlertRule.mockResolvedValue(undefined)

    const wrapper = await mountCard()
    await field(wrapper, 'usage-rule-delete-3').trigger('click')
    await flushPromises()
    expect(deleteAlertRule).not.toHaveBeenCalled()

    listAlertRules.mockResolvedValue([systemRule(), windowRule()])
    await field(wrapper, 'confirm-delete').trigger('click')
    await flushPromises()

    expect(deleteAlertRule).toHaveBeenCalledWith(3)
    expect(showSuccess).toHaveBeenCalledWith('admin.settings.notifications.accountUsageRules.deleteSuccess')
    expect(listAlertRules).toHaveBeenCalledTimes(2)
    expect(field(wrapper, 'usage-rule-row-3').exists()).toBe(false)
    expect(field(wrapper, 'usage-rule-row-2').exists()).toBe(true)
  })
})
