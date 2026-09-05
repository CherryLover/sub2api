<template>
  <!--
    账号用量提醒规则卡片。
    复用运维告警的规则 CRUD 接口（/admin/ops/alert-rules），只管四个账号用量指标；
    作用范围（平台 / 分组 / 账号 / 窗口）放在 filters 里，触发后由上面的 Bark 卡片推送。
    卡片挂在 SettingsView 的大 <form> 里，按钮一律 type="button"，输入框上的回车要拦下来。
  -->
  <div class="card" data-testid="account-usage-rules-card" @keydown.enter="handleEnterKey">
    <div
      class="flex flex-col gap-3 border-b border-gray-100 px-6 py-4 dark:border-dark-700 sm:flex-row sm:items-start sm:justify-between"
    >
      <div>
        <h2 class="flex items-center gap-2 text-lg font-semibold text-gray-900 dark:text-white">
          <Icon name="chartBar" size="sm" class="text-primary-600 dark:text-primary-400" />
          {{ t("admin.settings.notifications.accountUsageRules.title") }}
        </h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t("admin.settings.notifications.accountUsageRules.description") }}
        </p>
      </div>
      <div v-if="!opsDisabled" class="flex flex-wrap gap-2 sm:justify-end">
        <button
          type="button"
          class="btn btn-primary btn-sm"
          :disabled="loading"
          data-testid="usage-rules-create"
          @click="openCreate"
        >
          {{ t("admin.settings.notifications.accountUsageRules.create") }}
        </button>
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          :disabled="loading"
          data-testid="usage-rules-create-tiers"
          @click="openTiers"
        >
          {{ t("admin.settings.notifications.accountUsageRules.createTiers") }}
        </button>
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          :disabled="loading"
          data-testid="usage-rules-refresh"
          @click="load"
        >
          {{ t("common.refresh") }}
        </button>
      </div>
    </div>

    <div v-if="loading" class="flex items-center justify-center py-10">
      <div class="h-6 w-6 animate-spin rounded-full border-b-2 border-primary-600"></div>
    </div>

    <!-- 运维监控总开关关着：规则接口一律 OPS_DISABLED，这里给提示而不是空表 -->
    <div
      v-else-if="opsDisabled"
      class="m-6 flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-200"
      data-testid="usage-rules-ops-disabled"
    >
      <Icon name="exclamationTriangle" size="sm" class="mt-0.5 shrink-0" />
      <p>{{ t("admin.settings.notifications.accountUsageRules.opsDisabled") }}</p>
    </div>

    <div v-else class="p-6">
      <div
        v-if="rules.length === 0"
        class="rounded-xl border border-dashed border-gray-200 p-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400"
        data-testid="usage-rules-empty"
      >
        {{ t("admin.settings.notifications.accountUsageRules.empty") }}
      </div>

      <template v-else>
        <!-- 桌面：表格 -->
        <div class="hidden overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-700 md:block">
          <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700" data-testid="usage-rules-table">
            <thead class="bg-gray-50 dark:bg-dark-900">
              <tr>
                <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.notifications.accountUsageRules.table.name") }}
                </th>
                <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.notifications.accountUsageRules.table.metric") }}
                </th>
                <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.notifications.accountUsageRules.table.scope") }}
                </th>
                <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.notifications.accountUsageRules.table.condition") }}
                </th>
                <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.notifications.accountUsageRules.table.severity") }}
                </th>
                <th class="px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.notifications.accountUsageRules.table.enabled") }}
                </th>
                <th class="px-4 py-3 text-right text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                  {{ t("admin.settings.notifications.accountUsageRules.table.actions") }}
                </th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-800">
              <tr
                v-for="row in rules"
                :key="row.id"
                class="hover:bg-gray-50 dark:hover:bg-dark-700/50"
                :data-testid="`usage-rule-row-${row.id}`"
              >
                <td class="px-4 py-3">
                  <div class="text-xs font-bold text-gray-900 dark:text-white">{{ row.name }}</div>
                  <div v-if="row.description" class="mt-0.5 line-clamp-2 text-[11px] text-gray-500 dark:text-gray-400">
                    {{ row.description }}
                  </div>
                </td>
                <td class="whitespace-nowrap px-4 py-3 text-xs text-gray-700 dark:text-gray-200" data-testid="usage-rule-row-metric">
                  {{ metricLabel(row.metric_type) }}
                </td>
                <td class="px-4 py-3 text-xs text-gray-700 dark:text-gray-200" data-testid="usage-rule-scope">
                  {{ scopeSummary(row) }}
                </td>
                <td class="whitespace-nowrap px-4 py-3 font-mono text-xs text-gray-700 dark:text-gray-200" data-testid="usage-rule-condition">
                  {{ conditionText(row) }}
                </td>
                <td class="whitespace-nowrap px-4 py-3 text-xs font-bold text-gray-700 dark:text-gray-200">
                  {{ row.severity }}
                </td>
                <td class="whitespace-nowrap px-4 py-3">
                  <Toggle
                    :model-value="row.enabled"
                    :data-testid="`usage-rule-toggle-${row.id}`"
                    @update:model-value="toggleEnabled(row)"
                  />
                </td>
                <td class="whitespace-nowrap px-4 py-3 text-right text-xs">
                  <div class="flex justify-end gap-2">
                    <button type="button" class="btn btn-sm btn-secondary" :data-testid="`usage-rule-edit-${row.id}`" @click="openEdit(row)">
                      {{ t("common.edit") }}
                    </button>
                    <button type="button" class="btn btn-sm btn-secondary" :data-testid="`usage-rule-evaluate-${row.id}`" @click="openEvaluate(row)">
                      {{ t("admin.settings.notifications.accountUsageRules.evaluate.action") }}
                    </button>
                    <button type="button" class="btn btn-sm btn-danger" :data-testid="`usage-rule-delete-${row.id}`" @click="requestDelete(row)">
                      {{ t("common.delete") }}
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- 移动端：卡片 -->
        <div class="divide-y divide-gray-100 rounded-xl border border-gray-200 dark:divide-dark-700 dark:border-dark-700 md:hidden">
          <div v-for="row in rules" :key="row.id" class="space-y-2 p-4">
            <div class="flex items-start justify-between gap-2">
              <div class="min-w-0">
                <div class="text-xs font-bold text-gray-900 dark:text-white">{{ row.name }}</div>
                <div class="mt-0.5 text-[11px] text-gray-500 dark:text-gray-400">{{ scopeSummary(row) }}</div>
              </div>
              <span class="shrink-0 text-xs font-bold text-gray-700 dark:text-gray-200">{{ row.severity }}</span>
            </div>
            <div class="text-xs text-gray-700 dark:text-gray-200">
              {{ metricLabel(row.metric_type) }}
              <span class="ml-1 font-mono">{{ conditionText(row) }}</span>
            </div>
            <div class="flex items-center justify-between gap-2">
              <Toggle :model-value="row.enabled" @update:model-value="toggleEnabled(row)" />
              <div class="flex flex-wrap justify-end gap-2">
                <button type="button" class="btn btn-sm btn-secondary" @click="openEdit(row)">{{ t("common.edit") }}</button>
                <button type="button" class="btn btn-sm btn-secondary" @click="openEvaluate(row)">
                  {{ t("admin.settings.notifications.accountUsageRules.evaluate.action") }}
                </button>
                <button type="button" class="btn btn-sm btn-danger" @click="requestDelete(row)">{{ t("common.delete") }}</button>
              </div>
            </div>
          </div>
        </div>
      </template>
    </div>

    <!-- 新建 / 编辑 / 一键三档 共用一个编辑器，mode 决定显示哪些字段 -->
    <BaseDialog :show="showEditor" :title="editorTitle" width="wide" @close="closeEditor">
      <div v-if="draft" class="space-y-4" data-testid="usage-rule-editor">
        <p v-if="editorMode === 'tiers'" class="text-xs text-gray-500 dark:text-gray-400">
          {{ t("admin.settings.notifications.accountUsageRules.tiers.description") }}
        </p>

        <div
          v-if="editorErrors.length > 0"
          class="rounded-xl bg-red-50 p-4 text-xs text-red-700 dark:bg-red-900/30 dark:text-red-300"
          data-testid="usage-rule-editor-errors"
        >
          <ul class="list-disc pl-5">
            <li v-for="e in editorErrors" :key="e">{{ e }}</li>
          </ul>
        </div>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div class="md:col-span-2">
            <label class="input-label" for="usage-rule-name">
              {{
                editorMode === "tiers"
                  ? t("admin.settings.notifications.accountUsageRules.tiers.baseName")
                  : t("admin.settings.notifications.accountUsageRules.form.name")
              }}
            </label>
            <input
              id="usage-rule-name"
              v-model="draft.name"
              type="text"
              class="input"
              autocomplete="off"
              data-testid="usage-rule-name"
              :placeholder="
                editorMode === 'tiers'
                  ? t('admin.settings.notifications.accountUsageRules.tiers.baseNamePlaceholder')
                  : t('admin.settings.notifications.accountUsageRules.form.namePlaceholder')
              "
            />
            <p v-if="editorMode === 'tiers'" class="mt-1 text-xs text-gray-500 dark:text-gray-400" data-testid="usage-rule-tiers-preview">
              {{ t("admin.settings.notifications.accountUsageRules.tiers.preview", { names: tierNames.join(" / ") }) }}
            </p>
          </div>

          <div>
            <label class="input-label">{{ t("admin.settings.notifications.accountUsageRules.form.metric") }}</label>
            <Select
              :model-value="draft.metric_type"
              :options="metricOptions"
              :searchable="false"
              data-testid="usage-rule-metric"
              @update:model-value="onMetricChange"
            />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ metricHint(draft.metric_type) }}</p>
          </div>

          <!-- 随指标切换的附加字段 -->
          <div v-if="draft.metric_type === 'account_window_used_percent'">
            <label class="input-label">{{ t("admin.settings.notifications.accountUsageRules.form.window") }}</label>
            <Select
              :model-value="draft.window"
              :options="windowOptions"
              :searchable="false"
              data-testid="usage-rule-window"
              @update:model-value="(v) => (draft!.window = v === '7d' ? '7d' : '5h')"
            />
          </div>
          <div v-else-if="draft.metric_type === 'account_quota_used_percent'">
            <label class="input-label">{{ t("admin.settings.notifications.accountUsageRules.form.dimension") }}</label>
            <Select
              :model-value="draft.dimension"
              :options="dimensionOptions"
              :searchable="false"
              data-testid="usage-rule-dimension"
              @update:model-value="(v) => (draft!.dimension = isDimension(v) ? v : 'daily')"
            />
          </div>
          <div v-else-if="draft.metric_type === 'account_balance'">
            <label class="input-label">{{ t("admin.settings.notifications.accountUsageRules.form.provider") }}</label>
            <Select
              :model-value="draft.provider"
              :options="providerOptions"
              :searchable="false"
              data-testid="usage-rule-provider"
              @update:model-value="(v) => (draft!.provider = isProvider(v) ? v : '')"
            />
          </div>
          <div v-else></div>

          <div>
            <label class="input-label">{{ t("admin.settings.notifications.accountUsageRules.form.platform") }}</label>
            <Select
              :model-value="draft.platform"
              :options="platformOptions"
              :searchable="false"
              data-testid="usage-rule-platform"
              @update:model-value="onPlatformChange"
            />
          </div>

          <div>
            <label class="input-label">{{ t("admin.settings.notifications.accountUsageRules.form.group") }}</label>
            <Select
              :model-value="draft.group_id"
              :options="groupOptions"
              searchable
              data-testid="usage-rule-group"
              @update:model-value="(v) => (draft!.group_id = parsePositiveInt(v))"
            />
          </div>

          <div class="md:col-span-2">
            <label class="input-label">{{ t("admin.settings.notifications.accountUsageRules.form.accounts") }}</label>
            <Select
              :model-value="null"
              :options="accountPickerOptions"
              searchable
              :disabled="accountsLoading"
              :placeholder="
                accountsLoading
                  ? t('admin.settings.notifications.accountUsageRules.form.accountsLoading')
                  : accountPickerOptions.length === 0
                    ? t('admin.settings.notifications.accountUsageRules.form.accountsEmpty')
                    : t('admin.settings.notifications.accountUsageRules.form.accountsPlaceholder')
              "
              data-testid="usage-rule-account-picker"
              @update:model-value="addAccount"
            />
            <div v-if="draft.account_ids.length > 0" class="mt-2 flex flex-wrap gap-2" data-testid="usage-rule-account-chips">
              <span
                v-for="id in draft.account_ids"
                :key="id"
                class="inline-flex items-center gap-1 rounded-full bg-primary-50 px-2.5 py-1 text-xs text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
              >
                {{ accountLabel(id) }}
                <button
                  type="button"
                  class="rounded-full p-0.5 hover:bg-primary-100 dark:hover:bg-primary-800/40"
                  :aria-label="t('common.remove')"
                  :data-testid="`usage-rule-account-remove-${id}`"
                  @click="removeAccount(id)"
                >
                  <Icon name="x" size="xs" />
                </button>
              </span>
            </div>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.notifications.accountUsageRules.form.accountsHint") }}
            </p>
          </div>

          <template v-if="editorMode !== 'tiers'">
            <div>
              <label class="input-label">{{ t("admin.settings.notifications.accountUsageRules.form.operator") }}</label>
              <Select
                :model-value="draft.operator"
                :options="operatorOptions"
                :searchable="false"
                data-testid="usage-rule-operator"
                @update:model-value="(v) => (draft!.operator = isOperator(v) ? v : '>=')"
              />
            </div>
            <div>
              <label class="input-label" for="usage-rule-threshold">
                {{ t("admin.settings.notifications.accountUsageRules.form.threshold") }}
                <span v-if="metricUnit(draft.metric_type)" class="ml-1 text-gray-400">({{ metricUnit(draft.metric_type) }})</span>
              </label>
              <input
                id="usage-rule-threshold"
                v-model.number="draft.threshold"
                type="number"
                step="any"
                min="0"
                class="input"
                data-testid="usage-rule-threshold"
              />
            </div>
          </template>

          <div>
            <label class="input-label">{{ t("admin.settings.notifications.accountUsageRules.form.severity") }}</label>
            <Select
              :model-value="draft.severity"
              :options="severityOptions"
              :searchable="false"
              data-testid="usage-rule-severity"
              @update:model-value="(v) => (draft!.severity = typeof v === 'string' ? v : 'P2')"
            />
          </div>
          <div>
            <label class="input-label" for="usage-rule-cooldown">
              {{ t("admin.settings.notifications.accountUsageRules.form.cooldown") }}
            </label>
            <input
              id="usage-rule-cooldown"
              v-model.number="draft.cooldown_minutes"
              type="number"
              min="0"
              max="1440"
              class="input"
              data-testid="usage-rule-cooldown"
            />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t("admin.settings.notifications.accountUsageRules.form.cooldownHint") }}
            </p>
          </div>

          <div class="flex items-center justify-between rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-800/50 md:col-span-2">
            <span class="text-xs font-bold text-gray-700 dark:text-gray-200">
              {{ t("admin.settings.notifications.accountUsageRules.form.enabled") }}
            </span>
            <Toggle v-model="draft.enabled" data-testid="usage-rule-enabled" />
          </div>
        </div>

        <!-- 一键三档：逐条结果 -->
        <ul v-if="tierResults.length > 0" class="space-y-1 text-xs" data-testid="usage-rule-tier-results">
          <li
            v-for="item in tierResults"
            :key="item.name"
            class="flex items-start gap-2 rounded-lg px-3 py-2"
            :class="TIER_STATUS_CLASSES[item.status]"
            :data-status="item.status"
          >
            <span class="font-medium">{{ item.name }}</span>
            <span class="opacity-80">{{ tierStatusText(item) }}</span>
          </li>
        </ul>
      </div>

      <template #footer>
        <div class="flex items-center justify-end gap-2">
          <button type="button" class="btn btn-secondary" :disabled="saving" @click="closeEditor">
            {{ t("common.cancel") }}
          </button>
          <button type="button" class="btn btn-primary" :disabled="saving" data-testid="usage-rule-save" @click="submitEditor">
            {{ editorSubmitText }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- 立即试发 -->
    <BaseDialog
      :show="showEvaluate"
      :title="t('admin.settings.notifications.accountUsageRules.evaluate.title', { name: evaluateRule?.name ?? '' })"
      width="wide"
      @close="closeEvaluate"
    >
      <div class="space-y-4" data-testid="usage-rule-evaluate-dialog">
        <p class="text-xs text-gray-500 dark:text-gray-400">
          {{ t("admin.settings.notifications.accountUsageRules.evaluate.description") }}
        </p>

        <div v-if="evaluating" class="py-6 text-center text-sm text-gray-500 dark:text-gray-400" data-testid="usage-rule-evaluate-running">
          {{ t("admin.settings.notifications.accountUsageRules.evaluate.running") }}
        </div>

        <div
          v-else-if="evaluateError"
          class="flex items-start gap-2 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800 dark:border-red-800 dark:bg-red-900/20 dark:text-red-200"
          data-testid="usage-rule-evaluate-error"
        >
          <Icon name="xCircle" size="sm" class="mt-0.5 shrink-0" />
          <p>{{ evaluateError }}</p>
        </div>

        <template v-else-if="evaluation">
          <div class="grid grid-cols-2 gap-3 text-sm md:grid-cols-4">
            <div class="rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-800/50">
              <div class="text-[11px] uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.notifications.accountUsageRules.evaluate.aggregate") }}
              </div>
              <div class="mt-1 font-mono text-base font-bold text-gray-900 dark:text-white" data-testid="usage-rule-evaluate-value">
                {{ evaluation.has_data && evaluation.value != null ? formatValue(evaluation.metric_type, evaluation.value, evaluationCurrency) : "—" }}
              </div>
            </div>
            <div class="rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-800/50">
              <div class="text-[11px] uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.notifications.accountUsageRules.evaluate.threshold") }}
              </div>
              <div class="mt-1 font-mono text-base font-bold text-gray-900 dark:text-white">
                {{ evaluation.operator }} {{ formatValue(evaluation.metric_type, evaluation.threshold, evaluationCurrency) }}
              </div>
            </div>
            <div class="rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-800/50">
              <div class="text-[11px] uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.notifications.accountUsageRules.evaluate.columns.breached") }}
              </div>
              <div class="mt-1 text-base font-bold" :class="evaluation.breached ? 'text-red-600 dark:text-red-400' : 'text-green-600 dark:text-green-400'" data-testid="usage-rule-evaluate-breached">
                {{ evaluation.breached ? t("admin.settings.notifications.accountUsageRules.evaluate.breached") : t("admin.settings.notifications.accountUsageRules.evaluate.notBreached") }}
              </div>
            </div>
            <div class="rounded-xl bg-gray-50 px-4 py-3 dark:bg-dark-800/50">
              <div class="text-[11px] uppercase tracking-wider text-gray-500 dark:text-gray-400">
                {{ t("admin.settings.notifications.accountUsageRules.evaluate.evaluatedAt") }}
              </div>
              <div class="mt-1 text-xs text-gray-700 dark:text-gray-200">{{ formatTime(evaluation.evaluated_at) }}</div>
            </div>
          </div>

          <p v-if="!evaluation.has_data" class="text-xs text-amber-700 dark:text-amber-300" data-testid="usage-rule-evaluate-nodata">
            {{ t("admin.settings.notifications.accountUsageRules.evaluate.noData") }}
          </p>

          <div>
            <div class="mb-2 text-xs font-bold text-gray-700 dark:text-gray-200">
              {{ t("admin.settings.notifications.accountUsageRules.evaluate.accountsTitle") }}
            </div>
            <div v-if="evaluation.accounts.length === 0" class="rounded-xl border border-dashed border-gray-200 p-4 text-center text-xs text-gray-500 dark:border-dark-700 dark:text-gray-400">
              {{ t("admin.settings.notifications.accountUsageRules.evaluate.accountsEmpty") }}
            </div>
            <div v-else class="max-h-72 overflow-auto rounded-xl border border-gray-200 dark:border-dark-700">
              <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700" data-testid="usage-rule-evaluate-accounts">
                <thead class="sticky top-0 bg-gray-50 dark:bg-dark-900">
                  <tr>
                    <th class="px-3 py-2 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      {{ t("admin.settings.notifications.accountUsageRules.evaluate.columns.account") }}
                    </th>
                    <th class="px-3 py-2 text-left text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      {{ t("admin.settings.notifications.accountUsageRules.evaluate.columns.platform") }}
                    </th>
                    <th class="px-3 py-2 text-right text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      {{ t("admin.settings.notifications.accountUsageRules.evaluate.columns.value") }}
                    </th>
                    <th class="px-3 py-2 text-right text-[11px] font-bold uppercase tracking-wider text-gray-500 dark:text-gray-400">
                      {{ t("admin.settings.notifications.accountUsageRules.evaluate.columns.breached") }}
                    </th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-800">
                  <tr v-for="acc in evaluation.accounts" :key="acc.account_id" :data-breached="acc.breached ? 'true' : 'false'">
                    <td class="px-3 py-2 text-xs text-gray-900 dark:text-white">{{ acc.account_name || `#${acc.account_id}` }}</td>
                    <td class="px-3 py-2 text-xs text-gray-700 dark:text-gray-200">{{ platformLabel(acc.platform) }}</td>
                    <td class="px-3 py-2 text-right font-mono text-xs text-gray-700 dark:text-gray-200">
                      {{ formatValue(evaluation.metric_type, acc.value, acc.currency) }}
                    </td>
                    <td class="px-3 py-2 text-right text-xs font-bold" :class="acc.breached ? 'text-red-600 dark:text-red-400' : 'text-green-600 dark:text-green-400'">
                      {{ acc.breached ? t("admin.settings.notifications.accountUsageRules.evaluate.breached") : t("admin.settings.notifications.accountUsageRules.evaluate.notBreached") }}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div
            v-if="evaluateSendRequested"
            class="flex items-start gap-2 rounded-lg border px-4 py-3 text-sm"
            :class="evaluation.sent ? SEND_TONE_CLASSES.success : SEND_TONE_CLASSES.error"
            :data-sent="evaluation.sent ? 'true' : 'false'"
            data-testid="usage-rule-evaluate-send-result"
          >
            <Icon :name="evaluation.sent ? 'checkCircle' : 'xCircle'" size="sm" class="mt-0.5 shrink-0" />
            <p>
              {{
                evaluation.sent
                  ? t("admin.settings.notifications.accountUsageRules.evaluate.sent")
                  : evaluation.send_error
                    ? t("admin.settings.notifications.accountUsageRules.evaluate.sendError", { error: evaluation.send_error })
                    : t("admin.settings.notifications.accountUsageRules.evaluate.notSent")
              }}
            </p>
          </div>
        </template>
      </div>

      <template #footer>
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <input v-model="evaluateSend" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" data-testid="usage-rule-evaluate-send" />
            {{ t("admin.settings.notifications.accountUsageRules.evaluate.sendToBark") }}
          </label>
          <div class="flex items-center justify-end gap-2">
            <button type="button" class="btn btn-secondary" :disabled="evaluating" @click="closeEvaluate">
              {{ t("common.close") }}
            </button>
            <button type="button" class="btn btn-primary" :disabled="evaluating" data-testid="usage-rule-evaluate-run" @click="runEvaluate">
              {{ evaluating ? t("admin.settings.notifications.accountUsageRules.evaluate.running") : t("admin.settings.notifications.accountUsageRules.evaluate.run") }}
            </button>
          </div>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showDeleteConfirm"
      :title="t('admin.settings.notifications.accountUsageRules.deleteConfirmTitle')"
      :message="t('admin.settings.notifications.accountUsageRules.deleteConfirmMessage')"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmDelete"
      @cancel="cancelDelete"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { adminAPI } from "@/api";
import { opsAPI } from "@/api/admin/ops";
import type { AlertRule, AlertRuleEvaluation, MetricType, Operator } from "@/api/admin/ops";
import type { Account, AccountPlatform, AdminGroup } from "@/types";
import BaseDialog from "@/components/common/BaseDialog.vue";
import ConfirmDialog from "@/components/common/ConfirmDialog.vue";
import Icon from "@/components/icons/Icon.vue";
import Select, { type SelectOption } from "@/components/common/Select.vue";
import Toggle from "@/components/common/Toggle.vue";
import { extractApiErrorCode, extractApiErrorMessage } from "@/utils/apiError";
import { useAppStore } from "@/stores";

const { t } = useI18n();
const appStore = useAppStore();

const I18N = "admin.settings.notifications.accountUsageRules";

// ---------- 契约常量 ----------
type AccountMetricType =
  | "account_window_used_percent"
  | "account_quota_used_percent"
  | "account_balance"
  | "account_today_cost";
type WindowKey = "5h" | "7d";
type DimensionKey = "daily" | "weekly" | "total";
type ProviderKey = "" | "kimi" | "deepseek";

const ACCOUNT_METRIC_TYPES: readonly AccountMetricType[] = [
  "account_window_used_percent",
  "account_quota_used_percent",
  "account_balance",
  "account_today_cost",
];
const PERCENT_METRIC_TYPES = new Set<MetricType>([
  "account_window_used_percent",
  "account_quota_used_percent",
]);
const METRIC_I18N_KEY: Record<AccountMetricType, string> = {
  account_window_used_percent: "windowUsedPercent",
  account_quota_used_percent: "quotaUsedPercent",
  account_balance: "balance",
  account_today_cost: "todayCost",
};
const WINDOW_I18N_KEY: Record<WindowKey, string> = { "5h": "h5", "7d": "d7" };
const DIMENSIONS: readonly DimensionKey[] = ["daily", "weekly", "total"];
const PROVIDERS: readonly ProviderKey[] = ["kimi", "deepseek"];
const PLATFORMS: readonly AccountPlatform[] = [
  "anthropic",
  "openai",
  "gemini",
  "antigravity",
  "grok",
  "kimi",
  "zhipu",
  "deepseek",
];
const OPERATORS: readonly Operator[] = [">", ">=", "<", "<=", "==", "!="];
const SEVERITIES = ["P0", "P1", "P2", "P3"] as const;
// 一键三档：名称后缀与阈值一一对应，运算符固定 >=
const TIER_PERCENTS = [40, 60, 80] as const;
// 账号指标不看统计窗口，后端要求必须是 1 / 5 / 60 之一，固定发 1
const ACCOUNT_RULE_WINDOW_MINUTES = 1;

function isAccountMetricType(value: unknown): value is AccountMetricType {
  return typeof value === "string" && (ACCOUNT_METRIC_TYPES as readonly string[]).includes(value);
}
function isDimension(value: unknown): value is DimensionKey {
  return typeof value === "string" && (DIMENSIONS as readonly string[]).includes(value);
}
function isProvider(value: unknown): value is ProviderKey {
  return typeof value === "string" && (PROVIDERS as readonly string[]).includes(value);
}
function isOperator(value: unknown): value is Operator {
  return typeof value === "string" && (OPERATORS as readonly string[]).includes(value);
}
function parsePositiveInt(value: unknown): number | null {
  if (value == null || typeof value === "boolean") return null;
  const n = typeof value === "number" ? value : Number.parseInt(String(value), 10);
  return Number.isFinite(n) && n > 0 ? n : null;
}
function parseIdList(value: unknown): number[] {
  if (!Array.isArray(value)) return [];
  const out: number[] = [];
  for (const item of value) {
    const id = parsePositiveInt(item);
    if (id != null && !out.includes(id)) out.push(id);
  }
  return out;
}

// ---------- 列表 ----------
const loading = ref(true);
const opsDisabled = ref(false);
// 全量规则：唯一索引是全表的，一键三档 / 新建前用它查重
const allRules = ref<AlertRule[]>([]);

const rules = computed(() =>
  allRules.value
    .filter((r) => isAccountMetricType(r.metric_type))
    .sort((a, b) => (b.id || 0) - (a.id || 0)),
);

function isOpsDisabledError(err: unknown): boolean {
  if (!err || typeof err !== "object") return false;
  const e = err as { code?: unknown; reason?: unknown };
  return e.code === "OPS_DISABLED" || e.reason === "OPS_DISABLED";
}

async function load(): Promise<void> {
  loading.value = true;
  try {
    allRules.value = await opsAPI.listAlertRules();
    opsDisabled.value = false;
  } catch (error) {
    if (isOpsDisabledError(error)) {
      opsDisabled.value = true;
      allRules.value = [];
    } else {
      appStore.showError(extractApiErrorMessage(error, t(`${I18N}.loadFailed`)));
    }
  } finally {
    loading.value = false;
  }
}

// ---------- 分组 / 账号候选 ----------
const groups = ref<AdminGroup[]>([]);
async function loadGroups(): Promise<void> {
  try {
    groups.value = await adminAPI.groups.getAll();
  } catch (error) {
    console.error("[AccountUsageAlertRulesCard] Failed to load groups", error);
    groups.value = [];
  }
}

const groupNameById = computed(() => {
  const map = new Map<number, string>();
  for (const g of groups.value) map.set(g.id, g.name);
  return map;
});

// 账号按平台缓存（'' = 全部平台），编辑器打开 / 切平台时按需拉
const accountsByPlatform = ref<Record<string, Account[]>>({});
const accountsLoading = ref(false);
async function ensureAccounts(platform: string): Promise<void> {
  if (accountsByPlatform.value[platform]) return;
  accountsLoading.value = true;
  try {
    const page = await adminAPI.accounts.list(1, 500, {
      ...(platform ? { platform } : {}),
      lite: "1",
    });
    accountsByPlatform.value = { ...accountsByPlatform.value, [platform]: page.items ?? [] };
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t(`${I18N}.form.accountsLoadFailed`)));
  } finally {
    accountsLoading.value = false;
  }
}

const accountById = computed(() => {
  const map = new Map<number, Account>();
  for (const list of Object.values(accountsByPlatform.value)) {
    for (const acc of list) map.set(acc.id, acc);
  }
  return map;
});

function platformLabel(platform: string): string {
  if (!platform) return "";
  const key = `admin.accounts.platforms.${platform}`;
  const translated = t(key);
  return translated === key ? platform : translated;
}

function accountLabel(id: number): string {
  const acc = accountById.value.get(id);
  if (!acc) return `#${id}`;
  return draft.value?.platform ? acc.name : `${acc.name} · ${platformLabel(acc.platform)}`;
}

// ---------- 展示辅助 ----------
function metricLabel(metricType: MetricType): string {
  if (!isAccountMetricType(metricType)) return metricType;
  return t(`${I18N}.metrics.${METRIC_I18N_KEY[metricType]}`);
}
function metricHint(metricType: AccountMetricType): string {
  return t(`${I18N}.metricHints.${METRIC_I18N_KEY[metricType]}`);
}
function metricUnit(metricType: MetricType): string {
  if (PERCENT_METRIC_TYPES.has(metricType)) return "%";
  if (metricType === "account_today_cost") return "$";
  return "";
}

function formatNumber(value: number): string {
  if (!Number.isFinite(value)) return String(value);
  return Number.isInteger(value) ? String(value) : value.toFixed(2).replace(/\.?0+$/, "");
}

function formatValue(metricType: MetricType, value: number, currency?: string): string {
  const text = formatNumber(value);
  if (PERCENT_METRIC_TYPES.has(metricType)) return `${text}%`;
  if (metricType === "account_today_cost") return `$${text}`;
  if (metricType === "account_balance") return currency ? `${text} ${currency}` : text;
  return text;
}

function formatTime(value: string): string {
  if (!value) return "";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function conditionText(rule: AlertRule): string {
  const unit = metricUnit(rule.metric_type);
  return `${rule.operator} ${formatNumber(rule.threshold)}${unit}`;
}

// 作用域摘要：平台 · 分组 · N 个账号 · 窗口/维度/供应商；三项都没填就是「全部账号」
function scopeSummary(rule: AlertRule): string {
  const f = rule.filters ?? {};
  const parts: string[] = [];
  const platform = typeof f.platform === "string" ? f.platform : "";
  const groupId = parsePositiveInt(f.group_id);
  const accountIds = parseIdList(f.account_ids);

  if (platform) parts.push(platformLabel(platform));
  if (groupId != null) {
    parts.push(t(`${I18N}.scope.group`, { name: groupNameById.value.get(groupId) ?? `#${groupId}` }));
  }
  if (accountIds.length > 0) parts.push(t(`${I18N}.scope.accountCount`, { count: accountIds.length }));
  if (parts.length === 0) parts.push(t(`${I18N}.scope.allAccounts`));

  if (rule.metric_type === "account_window_used_percent") {
    const window: WindowKey = f.window === "7d" ? "7d" : "5h";
    parts.push(t(`${I18N}.windows.${WINDOW_I18N_KEY[window]}`));
  } else if (rule.metric_type === "account_quota_used_percent") {
    parts.push(t(`${I18N}.dimensions.${isDimension(f.dimension) ? f.dimension : "daily"}`));
  } else if (rule.metric_type === "account_balance" && isProvider(f.provider) && f.provider) {
    parts.push(t(`${I18N}.providers.${f.provider}`));
  }
  return parts.join(" · ");
}

// ---------- 编辑器（新建 / 编辑 / 一键三档） ----------
type EditorMode = "create" | "edit" | "tiers";

interface RuleDraft {
  name: string;
  metric_type: AccountMetricType;
  window: WindowKey;
  dimension: DimensionKey;
  provider: ProviderKey;
  platform: string;
  group_id: number | null;
  account_ids: number[];
  operator: Operator;
  threshold: number;
  severity: string;
  cooldown_minutes: number;
  enabled: boolean;
}

const showEditor = ref(false);
const editorMode = ref<EditorMode>("create");
const saving = ref(false);
const draft = ref<RuleDraft | null>(null);
// 编辑时保留原规则里这张表单不管的字段（description / sustained / notify_email）
const editingRule = ref<AlertRule | null>(null);

function newDraft(): RuleDraft {
  return {
    name: "",
    metric_type: "account_window_used_percent",
    window: "5h",
    dimension: "daily",
    provider: "",
    platform: "",
    group_id: null,
    account_ids: [],
    operator: ">=",
    threshold: 80,
    severity: "P2",
    cooldown_minutes: 60,
    enabled: true,
  };
}

function draftFromRule(rule: AlertRule): RuleDraft {
  const f = rule.filters ?? {};
  const base = newDraft();
  return {
    ...base,
    name: rule.name,
    metric_type: isAccountMetricType(rule.metric_type) ? rule.metric_type : base.metric_type,
    window: f.window === "7d" ? "7d" : "5h",
    dimension: isDimension(f.dimension) ? f.dimension : "daily",
    provider: isProvider(f.provider) ? f.provider : "",
    platform: typeof f.platform === "string" ? f.platform : "",
    group_id: parsePositiveInt(f.group_id),
    account_ids: parseIdList(f.account_ids),
    operator: isOperator(rule.operator) ? rule.operator : ">=",
    threshold: rule.threshold,
    severity: rule.severity || "P2",
    cooldown_minutes: rule.cooldown_minutes ?? 0,
    enabled: rule.enabled !== false,
  };
}

const editorTitle = computed(() => {
  if (editorMode.value === "tiers") return t(`${I18N}.tiers.title`);
  return editorMode.value === "edit" ? t(`${I18N}.form.editTitle`) : t(`${I18N}.form.createTitle`);
});

const editorSubmitText = computed(() => {
  if (editorMode.value === "tiers") {
    return saving.value ? t(`${I18N}.tiers.running`) : t(`${I18N}.tiers.run`);
  }
  return saving.value ? t("common.saving") : t("common.save");
});

const metricOptions = computed<SelectOption[]>(() =>
  ACCOUNT_METRIC_TYPES
    // 三档是 40% / 60% / 80%，只对百分比指标有意义
    .filter((m) => editorMode.value !== "tiers" || PERCENT_METRIC_TYPES.has(m))
    .map((m) => ({ value: m, label: metricLabel(m) })),
);
const windowOptions = computed<SelectOption[]>(() =>
  (["5h", "7d"] as WindowKey[]).map((w) => ({ value: w, label: t(`${I18N}.windows.${WINDOW_I18N_KEY[w]}`) })),
);
const dimensionOptions = computed<SelectOption[]>(() =>
  DIMENSIONS.map((d) => ({ value: d, label: t(`${I18N}.dimensions.${d}`) })),
);
const providerOptions = computed<SelectOption[]>(() => [
  { value: "", label: t(`${I18N}.providers.all`) },
  ...PROVIDERS.map((p) => ({ value: p, label: t(`${I18N}.providers.${p}`) })),
]);
const platformOptions = computed<SelectOption[]>(() => [
  { value: "", label: t(`${I18N}.form.platformAll`) },
  ...PLATFORMS.map((p) => ({ value: p, label: platformLabel(p) })),
]);
const groupOptions = computed<SelectOption[]>(() => {
  const platform = draft.value?.platform ?? "";
  const selected = draft.value?.group_id ?? null;
  const list = groups.value.filter((g) => !platform || g.platform === platform || g.id === selected);
  return [
    { value: null, label: t(`${I18N}.form.groupAll`) },
    ...list.map((g) => ({ value: g.id, label: platform ? g.name : `${g.name} · ${platformLabel(g.platform)}` })),
  ];
});
const operatorOptions = computed<SelectOption[]>(() => OPERATORS.map((o) => ({ value: o, label: o })));
const severityOptions = computed<SelectOption[]>(() => SEVERITIES.map((s) => ({ value: s, label: s })));

const accountPickerOptions = computed<SelectOption[]>(() => {
  const d = draft.value;
  if (!d) return [];
  const list = accountsByPlatform.value[d.platform] ?? [];
  return list
    .filter((acc) => !d.account_ids.includes(acc.id))
    .map((acc) => ({
      value: acc.id,
      label: d.platform ? acc.name : `${acc.name} · ${platformLabel(acc.platform)}`,
    }));
});

const tierNames = computed(() => {
  const base = (draft.value?.name ?? "").trim();
  return TIER_PERCENTS.map((p) => `${base} ${p}%`);
});

function onMetricChange(value: string | number | boolean | null): void {
  if (!draft.value || !isAccountMetricType(value)) return;
  const previous = draft.value.metric_type;
  draft.value.metric_type = value;
  // 阈值随指标切换到一个合理的默认值（编辑已有规则时不改）
  if (editorMode.value !== "edit" && previous !== value) {
    if (value === "account_balance") {
      draft.value.operator = "<";
      draft.value.threshold = 10;
    } else if (value === "account_today_cost") {
      draft.value.operator = ">=";
      draft.value.threshold = 10;
    } else {
      draft.value.operator = ">=";
      draft.value.threshold = 80;
    }
  }
}

function onPlatformChange(value: string | number | boolean | null): void {
  if (!draft.value) return;
  const platform = typeof value === "string" ? value : "";
  if (platform === draft.value.platform) return;
  draft.value.platform = platform;
  // 换了平台，之前选的账号和分组大概率不属于新平台，一并清掉
  draft.value.account_ids = [];
  if (draft.value.group_id != null) {
    const g = groups.value.find((item) => item.id === draft.value?.group_id);
    if (platform && g && g.platform !== platform) draft.value.group_id = null;
  }
  void ensureAccounts(platform);
}

function addAccount(value: string | number | boolean | null): void {
  const id = parsePositiveInt(value);
  if (!draft.value || id == null || draft.value.account_ids.includes(id)) return;
  draft.value.account_ids = [...draft.value.account_ids, id];
}

function removeAccount(id: number): void {
  if (!draft.value) return;
  draft.value.account_ids = draft.value.account_ids.filter((item) => item !== id);
}

function openCreate(): void {
  editorMode.value = "create";
  editingRule.value = null;
  draft.value = newDraft();
  tierResults.value = [];
  showEditor.value = true;
  void ensureAccounts("");
}

function openEdit(rule: AlertRule): void {
  editorMode.value = "edit";
  editingRule.value = rule;
  draft.value = draftFromRule(rule);
  tierResults.value = [];
  showEditor.value = true;
  void ensureAccounts(draft.value.platform);
}

function openTiers(): void {
  editorMode.value = "tiers";
  editingRule.value = null;
  draft.value = newDraft();
  tierResults.value = [];
  showEditor.value = true;
  void ensureAccounts("");
}

function closeEditor(): void {
  if (saving.value) return;
  showEditor.value = false;
  draft.value = null;
  editingRule.value = null;
  tierResults.value = [];
}

function nameExists(name: string, exceptId?: number | null): boolean {
  const target = name.trim();
  return allRules.value.some((r) => r.name.trim() === target && (exceptId == null || r.id !== exceptId));
}

const editorErrors = computed<string[]>(() => {
  const d = draft.value;
  if (!d) return [];
  const errors: string[] = [];
  const name = d.name.trim();
  if (editorMode.value === "tiers") {
    if (!name) errors.push(t(`${I18N}.tiers.baseNameRequired`));
  } else {
    if (!name) errors.push(t(`${I18N}.validation.nameRequired`));
    else if (nameExists(name, editingRule.value?.id)) errors.push(t(`${I18N}.duplicateName`));
    if (typeof d.threshold !== "number" || !Number.isFinite(d.threshold)) {
      errors.push(t(`${I18N}.validation.thresholdRequired`));
    } else if (PERCENT_METRIC_TYPES.has(d.metric_type) && (d.threshold < 0 || d.threshold > 100)) {
      errors.push(t(`${I18N}.validation.thresholdPercentRange`));
    } else if (d.threshold < 0) {
      errors.push(t(`${I18N}.validation.thresholdNonNegative`));
    }
  }
  const cooldown = d.cooldown_minutes;
  if (typeof cooldown !== "number" || !Number.isFinite(cooldown) || cooldown < 0 || cooldown > 1440) {
    errors.push(t(`${I18N}.validation.cooldownRange`));
  }
  return errors;
});

function buildFilters(d: RuleDraft): Record<string, unknown> {
  const filters: Record<string, unknown> = {};
  if (d.metric_type === "account_window_used_percent") filters.window = d.window;
  if (d.metric_type === "account_quota_used_percent") filters.dimension = d.dimension;
  if (d.metric_type === "account_balance" && d.provider) filters.provider = d.provider;
  if (d.platform) filters.platform = d.platform;
  if (d.group_id != null) filters.group_id = d.group_id;
  if (d.account_ids.length > 0) filters.account_ids = [...d.account_ids];
  return filters;
}

function buildRulePayload(
  d: RuleDraft,
  overrides: Partial<Pick<RuleDraft, "name" | "operator" | "threshold">> = {},
): AlertRule {
  const original = editingRule.value;
  return {
    name: (overrides.name ?? d.name).trim(),
    description: original?.description ?? "",
    enabled: d.enabled,
    metric_type: d.metric_type,
    operator: overrides.operator ?? d.operator,
    threshold: overrides.threshold ?? d.threshold,
    window_minutes: ACCOUNT_RULE_WINDOW_MINUTES,
    sustained_minutes: original?.sustained_minutes ?? 1,
    severity: d.severity,
    cooldown_minutes: d.cooldown_minutes,
    notify_email: original?.notify_email ?? false,
    filters: buildFilters(d),
  };
}

// 后端可能用 reason（infraerrors）或直接把码写在 message 里（response.BadRequest），两种都认
function isBarkNotEnabledError(err: unknown): boolean {
  if (extractApiErrorCode(err) === "BARK_NOT_ENABLED") return true;
  return /BARK_NOT_ENABLED/.test(extractApiErrorMessage(err, ""));
}

function isDuplicateNameError(err: unknown): boolean {
  if (!err || typeof err !== "object") return false;
  const e = err as { status?: unknown; message?: unknown; reason?: unknown };
  if (e.status === 409) return true;
  const text = `${typeof e.reason === "string" ? e.reason : ""} ${typeof e.message === "string" ? e.message : ""}`;
  return /duplicate|unique|already exists|已存在|重名/i.test(text);
}

function saveErrorMessage(err: unknown, fallback: string): string {
  if (isDuplicateNameError(err)) return t(`${I18N}.duplicateName`);
  return extractApiErrorMessage(err, fallback);
}

async function submitEditor(): Promise<void> {
  if (!draft.value || saving.value) return;
  if (editorErrors.value.length > 0) {
    appStore.showError(editorErrors.value[0]);
    return;
  }
  if (editorMode.value === "tiers") {
    await createTiers();
    return;
  }
  saving.value = true;
  try {
    const payload = buildRulePayload(draft.value);
    if (editorMode.value === "edit" && editingRule.value?.id) {
      await opsAPI.updateAlertRule(editingRule.value.id, payload);
    } else {
      await opsAPI.createAlertRule(payload);
    }
    appStore.showSuccess(t(`${I18N}.saveSuccess`));
    saving.value = false;
    closeEditor();
    await load();
  } catch (error) {
    appStore.showError(saveErrorMessage(error, t(`${I18N}.saveFailed`)));
  } finally {
    saving.value = false;
  }
}

// ---------- 一键三档 ----------
type TierStatus = "pending" | "ok" | "failed" | "skipped";
interface TierResult {
  name: string;
  status: TierStatus;
  message?: string;
}
const tierResults = ref<TierResult[]>([]);
const TIER_STATUS_CLASSES: Record<TierStatus, string> = {
  pending: "bg-gray-50 text-gray-600 dark:bg-dark-800/50 dark:text-gray-300",
  ok: "bg-green-50 text-green-800 dark:bg-green-900/20 dark:text-green-200",
  failed: "bg-red-50 text-red-800 dark:bg-red-900/20 dark:text-red-200",
  skipped: "bg-amber-50 text-amber-800 dark:bg-amber-900/20 dark:text-amber-200",
};

function tierStatusText(item: TierResult): string {
  switch (item.status) {
    case "ok":
      return t(`${I18N}.tiers.created`);
    case "failed":
      return t(`${I18N}.tiers.failed`, { error: item.message ?? "" });
    case "skipped":
      return t(`${I18N}.tiers.duplicate`);
    default:
      return t(`${I18N}.tiers.running`);
  }
}

// 串行创建三条：名称后缀 40% / 60% / 80%，运算符 >=；重名的（本地已知或后端报错）单独标出来
async function createTiers(): Promise<void> {
  const d = draft.value;
  if (!d) return;
  saving.value = true;
  const results: TierResult[] = tierNames.value.map((name) => ({ name, status: "pending" }));
  tierResults.value = results;
  let ok = 0;
  let failed = 0;
  for (let i = 0; i < TIER_PERCENTS.length; i += 1) {
    const item = results[i];
    if (nameExists(item.name)) {
      item.status = "skipped";
      failed += 1;
      continue;
    }
    try {
      const created = await opsAPI.createAlertRule(
        buildRulePayload(d, { name: item.name, operator: ">=", threshold: TIER_PERCENTS[i] }),
      );
      // 立刻记进全量列表，后一条如果同名就能在本地拦下
      allRules.value = [...allRules.value, created];
      item.status = "ok";
      ok += 1;
    } catch (error) {
      item.status = isDuplicateNameError(error) ? "skipped" : "failed";
      item.message = extractApiErrorMessage(error, t(`${I18N}.saveFailed`));
      failed += 1;
    }
    tierResults.value = [...results];
  }
  saving.value = false;
  const summary = t(`${I18N}.tiers.summary`, { ok, failed });
  if (failed === 0) appStore.showSuccess(summary);
  else if (ok === 0) appStore.showError(summary);
  else appStore.showWarning(summary);
  await load();
}

// ---------- 启停 / 删除 ----------
async function toggleEnabled(rule: AlertRule): Promise<void> {
  if (!rule.id) return;
  const next = !rule.enabled;
  try {
    const updated = await opsAPI.updateAlertRule(rule.id, { ...rule, enabled: next });
    allRules.value = allRules.value.map((r) => (r.id === rule.id ? { ...r, ...updated, enabled: next } : r));
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t(`${I18N}.toggleFailed`)));
  }
}

const showDeleteConfirm = ref(false);
const pendingDelete = ref<AlertRule | null>(null);

function requestDelete(rule: AlertRule): void {
  pendingDelete.value = rule;
  showDeleteConfirm.value = true;
}

function cancelDelete(): void {
  showDeleteConfirm.value = false;
  pendingDelete.value = null;
}

async function confirmDelete(): Promise<void> {
  const rule = pendingDelete.value;
  if (!rule?.id) return;
  try {
    await opsAPI.deleteAlertRule(rule.id);
    showDeleteConfirm.value = false;
    pendingDelete.value = null;
    appStore.showSuccess(t(`${I18N}.deleteSuccess`));
    await load();
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t(`${I18N}.deleteFailed`)));
  }
}

// ---------- 立即试发 ----------
const showEvaluate = ref(false);
const evaluateRule = ref<AlertRule | null>(null);
const evaluateSend = ref(false);
// 结果条只在这次真的要求推送时显示，免得干跑也出现「未推送」
const evaluateSendRequested = ref(false);
const evaluating = ref(false);
const evaluation = ref<AlertRuleEvaluation | null>(null);
const evaluateError = ref("");

const SEND_TONE_CLASSES = {
  success:
    "border-green-200 bg-green-50 text-green-800 dark:border-green-800 dark:bg-green-900/20 dark:text-green-200",
  error: "border-red-200 bg-red-50 text-red-800 dark:border-red-800 dark:bg-red-900/20 dark:text-red-200",
} as const;

const evaluationCurrency = computed(() => {
  const accounts = evaluation.value?.accounts ?? [];
  const currencies = new Set(accounts.map((a) => a.currency).filter((c): c is string => !!c));
  return currencies.size === 1 ? [...currencies][0] : undefined;
});

// 打开就先干跑一次（不推送）；勾上「同时推送到 Bark」再点「再试一次」才会真的推
function openEvaluate(rule: AlertRule): void {
  evaluateRule.value = rule;
  evaluateSend.value = false;
  evaluateSendRequested.value = false;
  evaluation.value = null;
  evaluateError.value = "";
  showEvaluate.value = true;
  void runEvaluate();
}

function closeEvaluate(): void {
  if (evaluating.value) return;
  showEvaluate.value = false;
  evaluateRule.value = null;
  evaluation.value = null;
  evaluateError.value = "";
}

async function runEvaluate(): Promise<void> {
  const rule = evaluateRule.value;
  if (!rule?.id || evaluating.value) return;
  const send = evaluateSend.value;
  evaluating.value = true;
  evaluateError.value = "";
  evaluateSendRequested.value = send;
  try {
    evaluation.value = await opsAPI.evaluateAlertRule(rule.id, send);
  } catch (error) {
    evaluation.value = null;
    // Bark 没启用：后端 400 BARK_NOT_ENABLED，提示去上面的卡片开
    evaluateError.value = isBarkNotEnabledError(error)
      ? t(`${I18N}.evaluate.barkNotEnabled`)
      : extractApiErrorMessage(error, t(`${I18N}.evaluate.failed`));
  } finally {
    evaluating.value = false;
  }
}

// 卡片挂在 SettingsView 的大 <form> 里：输入框里回车默认会提交整页，这里拦下
function handleEnterKey(event: KeyboardEvent): void {
  const target = event.target as HTMLElement | null;
  if (target?.tagName !== "INPUT") return;
  event.preventDefault();
}

onMounted(() => {
  void load();
  void loadGroups();
});
</script>
