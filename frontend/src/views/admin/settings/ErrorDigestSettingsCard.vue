<template>
  <!--
    定时报错汇总配置卡片。
    和 Bark 卡片一样独立调 /admin/notifications/error-digest 三个接口，不进 SettingsView 的大 payload；
    因为它被挂在页面的大 <form> 里，输入框上的回车要拦下来，免得触发整页保存。
  -->
  <div class="card" data-testid="error-digest-card" @keydown.enter="handleEnterKey">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="flex items-center gap-2 text-lg font-semibold text-gray-900 dark:text-white">
        <Icon name="bell" size="sm" class="text-primary-600 dark:text-primary-400" />
        {{ t("admin.settings.notifications.errorDigest.title") }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t("admin.settings.notifications.errorDigest.description") }}
      </p>
    </div>

    <div v-if="loading" class="flex items-center justify-center py-10">
      <div class="h-6 w-6 animate-spin rounded-full border-b-2 border-primary-600"></div>
    </div>

    <div v-else class="space-y-5 p-6">
      <!-- 启用开关 -->
      <div class="flex items-center justify-between gap-4">
        <div>
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t("admin.settings.notifications.errorDigest.enabled") }}
          </label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.notifications.errorDigest.enabledHint") }}
          </p>
        </div>
        <Toggle v-model="form.enabled" data-testid="error-digest-enabled" />
      </div>

      <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
        <!-- 推送时间 -->
        <div>
          <label
            for="error-digest-schedule"
            class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
          >
            {{ t("admin.settings.notifications.errorDigest.schedule") }}
          </label>
          <input
            id="error-digest-schedule"
            v-model="form.schedule"
            type="text"
            class="input w-full"
            data-testid="error-digest-schedule"
            autocomplete="off"
            :placeholder="DEFAULT_SCHEDULE"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.notifications.errorDigest.scheduleHint") }}
          </p>
        </div>

        <!-- 展开几个密钥 -->
        <div>
          <label
            for="error-digest-top-keys"
            class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
          >
            {{ t("admin.settings.notifications.errorDigest.topKeys") }}
          </label>
          <input
            id="error-digest-top-keys"
            v-model.number="form.top_keys"
            type="number"
            min="1"
            max="50"
            class="input w-full"
            data-testid="error-digest-top-keys"
            autocomplete="off"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.notifications.errorDigest.topKeysHint") }}
          </p>
        </div>
      </div>

      <!-- 没报错时跳过 -->
      <div class="flex items-center justify-between gap-4">
        <div>
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t("admin.settings.notifications.errorDigest.skipWhenEmpty") }}
          </label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.notifications.errorDigest.skipWhenEmptyHint") }}
          </p>
        </div>
        <Toggle v-model="form.skip_when_empty" data-testid="error-digest-skip-when-empty" />
      </div>

      <!-- 最近一次试推的结果：推成功绿、只算出内容没推出去琥珀、接口失败红 -->
      <div
        v-if="lastTest"
        :class="[
          'space-y-2 rounded-lg border px-4 py-3 text-sm',
          RESULT_TONE_CLASSES[lastTestTone],
        ]"
        :data-tone="lastTestTone"
        data-testid="error-digest-result"
      >
        <div class="flex items-start gap-2">
          <Icon :name="RESULT_TONE_ICONS[lastTestTone]" size="sm" class="mt-0.5 shrink-0" />
          <p class="min-w-0 break-all font-medium">{{ lastTestHeadline }}</p>
        </div>
        <!-- 把后端算出来的正文原样贴出来，站长看到的就是手机上会收到的那条 -->
        <pre
          v-if="lastTest.result"
          class="overflow-x-auto whitespace-pre-wrap break-words rounded bg-black/5 p-3 text-xs leading-relaxed dark:bg-white/10"
          data-testid="error-digest-preview"
          >{{ lastTest.result.body }}</pre
        >
      </div>

      <!-- 操作区 -->
      <div
        class="flex flex-col gap-3 border-t border-gray-100 pt-4 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between"
      >
        <p
          v-if="lastSavedText"
          class="text-xs text-gray-500 dark:text-gray-400"
          data-testid="error-digest-last-saved"
        >
          {{ t("admin.settings.notifications.errorDigest.lastSaved", { time: lastSavedText }) }}
        </p>
        <span v-else></span>
        <div class="flex flex-wrap justify-end gap-2">
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="busy || loadFailed"
            data-testid="error-digest-test"
            @click="runTest"
          >
            {{
              testing
                ? t("common.loading")
                : t("admin.settings.notifications.errorDigest.test")
            }}
          </button>
          <button
            type="button"
            class="btn btn-primary btn-sm"
            :disabled="busy || loadFailed"
            data-testid="error-digest-save"
            @click="save"
          >
            {{ saving ? t("common.saving") : t("common.save") }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useI18n } from "vue-i18n";
import { adminAPI } from "@/api";
import type {
  ErrorDigestConfig,
  TestErrorDigestResponse,
  UpdateErrorDigestConfigRequest,
} from "@/api/admin/notifications";
import Icon from "@/components/icons/Icon.vue";
import Toggle from "@/components/common/Toggle.vue";
import { extractApiErrorMessage } from "@/utils/apiError";
import { useAppStore } from "@/stores";

const { t } = useI18n();
const appStore = useAppStore();

// 与后端默认值保持一致：每天 11:30 与 17:30 各一条
const DEFAULT_SCHEDULE = "30 11,17 * * *";
const DEFAULT_TOP_KEYS = 8;
const MIN_TOP_KEYS = 1;
const MAX_TOP_KEYS = 50;

type ResultTone = "success" | "warning" | "error";
interface TestOutcome {
  result?: TestErrorDigestResponse;
  error?: string;
}

const RESULT_TONE_CLASSES: Record<ResultTone, string> = {
  success:
    "border-green-200 bg-green-50 text-green-800 dark:border-green-800 dark:bg-green-900/20 dark:text-green-200",
  warning:
    "border-amber-200 bg-amber-50 text-amber-800 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-200",
  error:
    "border-red-200 bg-red-50 text-red-800 dark:border-red-800 dark:bg-red-900/20 dark:text-red-200",
};
const RESULT_TONE_ICONS = {
  success: "checkCircle",
  warning: "exclamationTriangle",
  error: "xCircle",
} as const;

const loading = ref(true);
const loadFailed = ref(false);
const saving = ref(false);
const testing = ref(false);
const busy = computed(() => saving.value || testing.value);

const updatedAt = ref("");
const lastTest = ref<TestOutcome | null>(null);

const form = reactive<UpdateErrorDigestConfigRequest>({
  enabled: false,
  schedule: DEFAULT_SCHEDULE,
  skip_when_empty: true,
  top_keys: DEFAULT_TOP_KEYS,
});

const lastSavedText = computed(() => {
  if (!updatedAt.value) return "";
  const date = new Date(updatedAt.value);
  return Number.isNaN(date.getTime()) ? updatedAt.value : date.toLocaleString();
});

// 「算出来了但没推出去」不该显示成一片绿：Bark 没启用时站长手机上什么都收不到
const lastTestTone = computed<ResultTone>(() => {
  const outcome = lastTest.value;
  if (!outcome || outcome.error || !outcome.result) return "error";
  return outcome.result.pushed ? "success" : "warning";
});

const lastTestHeadline = computed(() => {
  const outcome = lastTest.value;
  if (!outcome) return "";
  if (outcome.error) return outcome.error;
  return outcome.result?.pushed
    ? t("admin.settings.notifications.errorDigest.resultPushed")
    : t("admin.settings.notifications.errorDigest.resultNotPushed");
});

function applyConfig(cfg: ErrorDigestConfig): void {
  form.enabled = Boolean(cfg.enabled);
  form.schedule = cfg.schedule || DEFAULT_SCHEDULE;
  form.skip_when_empty = cfg.skip_when_empty !== false;
  form.top_keys = Number(cfg.top_keys) || DEFAULT_TOP_KEYS;
  updatedAt.value = cfg.updated_at || "";
}

// 数字框被清空时 v-model.number 给的是空串或 NaN，这里统一夹回合法区间再提交，
// 免得后端收到个 NaN（JSON 序列化成 null）而回落成默认值，站长却以为自己填的生效了。
function normalizedTopKeys(): number {
  const raw = Number(form.top_keys);
  if (!Number.isFinite(raw) || raw <= 0) return DEFAULT_TOP_KEYS;
  return Math.min(MAX_TOP_KEYS, Math.max(MIN_TOP_KEYS, Math.trunc(raw)));
}

function buildPayload(): UpdateErrorDigestConfigRequest {
  return {
    enabled: form.enabled,
    schedule: form.schedule.trim(),
    skip_when_empty: form.skip_when_empty,
    top_keys: normalizedTopKeys(),
  };
}

async function load(): Promise<void> {
  loading.value = true;
  loadFailed.value = false;
  try {
    applyConfig(await adminAPI.notifications.getErrorDigestConfig());
  } catch (error) {
    loadFailed.value = true;
    appStore.showError(
      extractApiErrorMessage(
        error,
        t("admin.settings.notifications.errorDigest.loadFailed"),
      ),
    );
  } finally {
    loading.value = false;
  }
}

async function save(): Promise<void> {
  if (busy.value || loadFailed.value) return;
  saving.value = true;
  try {
    const saved = await adminAPI.notifications.updateErrorDigestConfig(buildPayload());
    applyConfig(saved);
    appStore.showSuccess(t("admin.settings.notifications.errorDigest.saved"));
  } catch (error) {
    appStore.showError(
      extractApiErrorMessage(
        error,
        t("admin.settings.notifications.errorDigest.saveFailed"),
      ),
    );
  } finally {
    saving.value = false;
  }
}

// 试推走的是后端的最近 24 小时，跟这里表单上还没保存的值无关，所以不带任何请求体；
// 站长要验的是「通知长什么样 + Bark 通不通」，不是「这套配置对不对」。
async function runTest(): Promise<void> {
  if (busy.value || loadFailed.value) return;
  testing.value = true;
  lastTest.value = null;
  try {
    const result = await adminAPI.notifications.testErrorDigest();
    lastTest.value = { result };
    if (result.pushed) {
      appStore.showSuccess(t("admin.settings.notifications.errorDigest.pushed"));
    } else {
      appStore.showWarning(t("admin.settings.notifications.errorDigest.notPushed"));
    }
  } catch (error) {
    const message = extractApiErrorMessage(
      error,
      t("admin.settings.notifications.errorDigest.testFailed"),
    );
    lastTest.value = { error: message };
    appStore.showError(message);
  } finally {
    testing.value = false;
  }
}

// 卡片挂在 SettingsView 的大 <form> 里：输入框里回车默认会提交整页，这里拦下并改为保存本卡片
function handleEnterKey(event: KeyboardEvent): void {
  const target = event.target as HTMLElement | null;
  if (target?.tagName !== "INPUT") return;
  event.preventDefault();
  void save();
}

onMounted(load);
</script>
