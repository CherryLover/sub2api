<template>
  <!--
    Bark 推送通知配置卡片。
    独立调 /admin/notifications/bark 三个接口，不进 SettingsView 的大 payload；
    因为它被挂在页面的大 <form> 里，输入框上的回车要拦下来，免得触发整页保存。
  -->
  <div class="card" data-testid="bark-notify-card" @keydown.enter="handleEnterKey">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2
        class="flex items-center gap-2 text-lg font-semibold text-gray-900 dark:text-white"
      >
        <Icon name="bell" size="sm" class="text-primary-600 dark:text-primary-400" />
        {{ t("admin.settings.notifications.bark.title") }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t("admin.settings.notifications.bark.description") }}
      </p>
    </div>

    <div v-if="loading" class="flex items-center justify-center py-10">
      <div
        class="h-6 w-6 animate-spin rounded-full border-b-2 border-primary-600"
      ></div>
    </div>

    <div v-else class="space-y-5 p-6">
      <!-- 启用开关 -->
      <div class="flex items-center justify-between gap-4">
        <div>
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t("admin.settings.notifications.bark.enabled") }}
          </label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.notifications.bark.enabledHint") }}
          </p>
        </div>
        <Toggle v-model="form.enabled" data-testid="bark-enabled" />
      </div>

      <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
        <!-- 服务器地址 -->
        <div>
          <label
            for="bark-server-url"
            class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
          >
            {{ t("admin.settings.notifications.bark.serverUrl") }}
          </label>
          <input
            id="bark-server-url"
            v-model="form.server_url"
            type="url"
            class="input w-full"
            data-testid="bark-server-url"
            autocomplete="off"
            :placeholder="t('admin.settings.notifications.bark.serverUrlPlaceholder')"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.notifications.bark.serverUrlHint") }}
          </p>
        </div>

        <!-- 设备 Key -->
        <div>
          <div class="mb-1 flex items-center justify-between gap-2">
            <label
              for="bark-device-key"
              class="block text-xs font-medium text-gray-600 dark:text-gray-400"
            >
              {{ t("admin.settings.notifications.bark.deviceKey") }}
            </label>
            <span
              v-if="hasDeviceKey"
              class="inline-flex items-center gap-1 rounded-full bg-green-50 px-2 py-0.5 text-xs font-medium text-green-700 dark:bg-green-900/30 dark:text-green-300"
              data-testid="bark-device-key-configured"
            >
              <Icon name="checkCircle" size="xs" />
              {{ t("admin.settings.notifications.bark.deviceKeyConfigured") }}
            </span>
          </div>
          <input
            id="bark-device-key"
            v-model="form.device_key"
            type="password"
            class="input w-full"
            data-testid="bark-device-key"
            autocomplete="new-password"
            :placeholder="
              hasDeviceKey
                ? t('admin.settings.notifications.bark.deviceKeyConfiguredPlaceholder')
                : t('admin.settings.notifications.bark.deviceKeyPlaceholder')
            "
          />
          <p
            v-if="deviceKeyMissing"
            class="mt-1 text-xs text-red-600 dark:text-red-400"
            data-testid="bark-device-key-warning"
          >
            {{ t("admin.settings.notifications.bark.deviceKeyRequired") }}
          </p>
          <p v-else class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.notifications.bark.deviceKeyHint") }}
          </p>
        </div>

        <!-- 通知分组 -->
        <div>
          <label
            for="bark-group"
            class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
          >
            {{ t("admin.settings.notifications.bark.group") }}
          </label>
          <input
            id="bark-group"
            v-model="form.group"
            type="text"
            class="input w-full"
            data-testid="bark-group"
            autocomplete="off"
            :placeholder="BARK_DEFAULT_GROUP"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.notifications.bark.groupHint") }}
          </p>
        </div>

        <!-- 打断级别 -->
        <div>
          <label
            for="bark-level"
            class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
          >
            {{ t("admin.settings.notifications.bark.level") }}
          </label>
          <Select
            id="bark-level"
            :model-value="form.level"
            :options="levelOptions"
            :searchable="false"
            data-testid="bark-level"
            @update:model-value="onLevelChange"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400" data-testid="bark-level-hint">
            {{ levelHint }}
          </p>
        </div>

        <!-- 提示音 -->
        <div>
          <label
            for="bark-sound"
            class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
          >
            {{ t("admin.settings.notifications.bark.sound") }}
          </label>
          <input
            id="bark-sound"
            v-model="form.sound"
            type="text"
            class="input w-full"
            data-testid="bark-sound"
            autocomplete="off"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.notifications.bark.soundHint") }}
          </p>
        </div>

        <!-- 点击跳转地址 -->
        <div>
          <label
            for="bark-click-url"
            class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400"
          >
            {{ t("admin.settings.notifications.bark.clickUrl") }}
          </label>
          <input
            id="bark-click-url"
            v-model="form.click_url"
            type="url"
            class="input w-full"
            data-testid="bark-click-url"
            autocomplete="off"
            placeholder="https://"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.notifications.bark.clickUrlHint") }}
          </p>
        </div>
      </div>

      <!-- 告警恢复时也通知 -->
      <div class="flex items-center justify-between gap-4">
        <div>
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t("admin.settings.notifications.bark.notifyOnResolve") }}
          </label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.notifications.bark.notifyOnResolveHint") }}
          </p>
        </div>
        <Toggle v-model="form.notify_on_resolve" data-testid="bark-notify-on-resolve" />
      </div>

      <!-- 最近一次测试的结果 -->
      <div
        v-if="lastTest"
        :class="[
          'flex items-start gap-2 rounded-lg border px-4 py-3 text-sm',
          lastTestSucceeded
            ? 'border-green-200 bg-green-50 text-green-800 dark:border-green-800 dark:bg-green-900/20 dark:text-green-200'
            : 'border-red-200 bg-red-50 text-red-800 dark:border-red-800 dark:bg-red-900/20 dark:text-red-200',
        ]"
        data-testid="bark-test-result"
      >
        <Icon
          :name="lastTestSucceeded ? 'checkCircle' : 'exclamationTriangle'"
          size="sm"
          class="mt-0.5 shrink-0"
        />
        <div class="min-w-0 space-y-0.5">
          <p class="font-medium">{{ lastTestHeadline }}</p>
          <p v-if="lastTestDetail" class="break-all text-xs opacity-80">
            {{ lastTestDetail }}
          </p>
        </div>
      </div>

      <!-- 操作区 -->
      <div
        class="flex flex-col gap-3 border-t border-gray-100 pt-4 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between"
      >
        <p
          v-if="lastSavedText"
          class="text-xs text-gray-500 dark:text-gray-400"
          data-testid="bark-last-saved"
        >
          {{ t("admin.settings.notifications.bark.lastSaved", { time: lastSavedText }) }}
        </p>
        <span v-else></span>
        <div class="flex flex-wrap justify-end gap-2">
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="busy || loadFailed"
            data-testid="bark-test-connection"
            @click="runTest('connection')"
          >
            {{
              testing === "connection"
                ? t("common.loading")
                : t("admin.settings.notifications.bark.testConnection")
            }}
          </button>
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="busy || loadFailed"
            data-testid="bark-send-test"
            @click="runTest('send')"
          >
            {{
              testing === "send"
                ? t("common.loading")
                : t("admin.settings.notifications.bark.sendTest")
            }}
          </button>
          <button
            type="button"
            class="btn btn-primary btn-sm"
            :disabled="busy || loadFailed"
            data-testid="bark-save"
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
  BarkLevel,
  BarkNotifyConfig,
  TestBarkNotifyRequest,
  TestBarkNotifyResponse,
  UpdateBarkNotifyConfigRequest,
} from "@/api/admin/notifications";
import Icon from "@/components/icons/Icon.vue";
import Select from "@/components/common/Select.vue";
import Toggle from "@/components/common/Toggle.vue";
import { extractApiErrorMessage } from "@/utils/apiError";
import { useAppStore } from "@/stores";

const { t } = useI18n();
const appStore = useAppStore();

// 与后端契约一致的四个打断级别；顺序就是下拉里的顺序
const BARK_LEVELS: readonly BarkLevel[] = [
  "active",
  "timeSensitive",
  "passive",
  "critical",
];
const BARK_DEFAULT_SERVER_URL = "https://api.day.app";
const BARK_DEFAULT_GROUP = "sub2api";

type TestMode = "connection" | "send";
interface TestOutcome {
  mode: TestMode;
  result?: TestBarkNotifyResponse;
  error?: string;
}

const loading = ref(true);
const loadFailed = ref(false);
const saving = ref(false);
const testing = ref<TestMode | null>(null);
const busy = computed(() => saving.value || testing.value !== null);

// 后端永远不回显设备 Key，只告诉我们「有没有」；输入框里只放这次要写入的新值
const hasDeviceKey = ref(false);
const updatedAt = ref("");
const lastTest = ref<TestOutcome | null>(null);

const form = reactive<UpdateBarkNotifyConfigRequest>({
  enabled: false,
  server_url: BARK_DEFAULT_SERVER_URL,
  device_key: "",
  group: BARK_DEFAULT_GROUP,
  level: "active",
  sound: "",
  click_url: "",
  notify_on_resolve: true,
});

const deviceKeyAvailable = computed(
  () => form.device_key.trim() !== "" || hasDeviceKey.value,
);
// 开了推送却没有任何可用的 Key：保存前就在界面上提示，并在点保存时拦下
const deviceKeyMissing = computed(
  () => form.enabled && !deviceKeyAvailable.value,
);

const levelOptions = computed(() =>
  BARK_LEVELS.map((value) => ({
    value,
    label: `${t(`admin.settings.notifications.bark.levels.${value}`)} (${value})`,
  })),
);
const levelHint = computed(() =>
  t(`admin.settings.notifications.bark.levelHints.${form.level}`),
);

const lastSavedText = computed(() => {
  if (!updatedAt.value) return "";
  const date = new Date(updatedAt.value);
  return Number.isNaN(date.getTime()) ? updatedAt.value : date.toLocaleString();
});

const lastTestSucceeded = computed(() => {
  const outcome = lastTest.value;
  if (!outcome || outcome.error || !outcome.result) return false;
  return outcome.mode === "connection"
    ? outcome.result.ping_ok
    : outcome.result.ok;
});

const lastTestHeadline = computed(() => {
  const outcome = lastTest.value;
  if (!outcome) return "";
  if (outcome.error) {
    return outcome.mode === "connection"
      ? t("admin.settings.notifications.bark.connectionFailed")
      : t("admin.settings.notifications.bark.sendFailed");
  }
  const result = outcome.result;
  if (!result) return "";
  if (outcome.mode === "connection") {
    return result.ping_ok
      ? t("admin.settings.notifications.bark.resultPingOk")
      : t("admin.settings.notifications.bark.resultPingFailed");
  }
  return result.ok
    ? t("admin.settings.notifications.bark.resultSent")
    : t("admin.settings.notifications.bark.resultNotSent");
});

const lastTestDetail = computed(() => {
  const outcome = lastTest.value;
  if (!outcome) return "";
  if (outcome.error) return outcome.error;
  const result = outcome.result;
  if (!result) return "";
  const parts: string[] = [];
  if (typeof result.latency_ms === "number") {
    parts.push(
      t("admin.settings.notifications.bark.resultLatency", {
        latency: result.latency_ms,
      }),
    );
  }
  if (result.status_code) {
    parts.push(
      t("admin.settings.notifications.bark.resultStatus", {
        status: result.status_code,
      }),
    );
  }
  if (result.message) parts.push(result.message);
  return parts.join(" · ");
});

function isBarkLevel(value: unknown): value is BarkLevel {
  return typeof value === "string" && (BARK_LEVELS as readonly string[]).includes(value);
}

function onLevelChange(value: string | number | boolean | null): void {
  if (isBarkLevel(value)) {
    form.level = value;
  }
}

function applyConfig(cfg: BarkNotifyConfig): void {
  form.enabled = Boolean(cfg.enabled);
  form.server_url = cfg.server_url || BARK_DEFAULT_SERVER_URL;
  form.device_key = "";
  form.group = cfg.group || BARK_DEFAULT_GROUP;
  form.level = isBarkLevel(cfg.level) ? cfg.level : "active";
  form.sound = cfg.sound || "";
  form.click_url = cfg.click_url || "";
  form.notify_on_resolve = cfg.notify_on_resolve !== false;
  hasDeviceKey.value = Boolean(cfg.has_device_key);
  updatedAt.value = cfg.updated_at || "";
}

function buildPayload(): UpdateBarkNotifyConfigRequest {
  return {
    enabled: form.enabled,
    server_url: form.server_url.trim(),
    device_key: form.device_key.trim(),
    group: form.group.trim(),
    level: form.level,
    sound: form.sound.trim(),
    click_url: form.click_url.trim(),
    notify_on_resolve: form.notify_on_resolve,
  };
}

async function load(): Promise<void> {
  loading.value = true;
  loadFailed.value = false;
  try {
    applyConfig(await adminAPI.notifications.getBarkConfig());
  } catch (error) {
    loadFailed.value = true;
    appStore.showError(
      extractApiErrorMessage(
        error,
        t("admin.settings.notifications.bark.loadFailed"),
      ),
    );
  } finally {
    loading.value = false;
  }
}

async function save(): Promise<void> {
  if (busy.value || loadFailed.value) return;
  if (deviceKeyMissing.value) {
    appStore.showError(t("admin.settings.notifications.bark.deviceKeyRequired"));
    return;
  }
  saving.value = true;
  try {
    const saved = await adminAPI.notifications.updateBarkConfig(buildPayload());
    applyConfig(saved);
    appStore.showSuccess(t("admin.settings.notifications.bark.saved"));
  } catch (error) {
    appStore.showError(
      extractApiErrorMessage(
        error,
        t("admin.settings.notifications.bark.saveFailed"),
      ),
    );
  } finally {
    saving.value = false;
  }
}

// 两个测试按钮共用同一个接口，都用表单当前值（不要求先保存）；
// 「测试连接」只看 ping_ok 与延迟，「发送测试通知」看 ok 并带上标题正文
async function runTest(mode: TestMode): Promise<void> {
  if (busy.value || loadFailed.value) return;
  if (form.server_url.trim() === "") {
    appStore.showError(t("admin.settings.notifications.bark.serverUrlRequired"));
    return;
  }
  if (mode === "send" && !deviceKeyAvailable.value) {
    appStore.showError(
      t("admin.settings.notifications.bark.deviceKeyRequiredForTest"),
    );
    return;
  }

  const payload: TestBarkNotifyRequest = buildPayload();
  if (mode === "send") {
    payload.title = t("admin.settings.notifications.bark.testTitle");
    payload.body = t("admin.settings.notifications.bark.testBody");
  }

  testing.value = mode;
  lastTest.value = null;
  try {
    const result = await adminAPI.notifications.testBark(payload);
    lastTest.value = { mode, result };
    if (mode === "connection") {
      if (result.ping_ok) {
        appStore.showSuccess(
          t("admin.settings.notifications.bark.connectionOk", {
            latency: result.latency_ms,
          }),
        );
      } else {
        appStore.showError(
          result.message ||
            t("admin.settings.notifications.bark.connectionFailed"),
        );
      }
    } else if (result.ok) {
      appStore.showSuccess(t("admin.settings.notifications.bark.sent"));
    } else {
      appStore.showError(
        result.message || t("admin.settings.notifications.bark.sendFailed"),
      );
    }
  } catch (error) {
    const message = extractApiErrorMessage(
      error,
      t("admin.settings.notifications.bark.testFailed"),
    );
    lastTest.value = { mode, error: message };
    appStore.showError(message);
  } finally {
    testing.value = null;
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
