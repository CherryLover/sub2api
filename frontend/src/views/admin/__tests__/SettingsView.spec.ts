import { beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h } from "vue";
import { flushPromises, mount } from "@vue/test-utils";

import enSettings from "@/i18n/locales/en/admin/settings";
import zhSettings from "@/i18n/locales/zh/admin/settings";
import SettingsView from "../SettingsView.vue";

const {
  getSettings,
  updateSettings,
  getWebSearchEmulationConfig,
  updateWebSearchEmulationConfig,
  getAdminApiKey,
  getOverloadCooldownSettings,
  getRateLimit429CooldownSettings,
  updateRateLimit429CooldownSettings,
  getPanelRateLimitSettings,
  updatePanelRateLimitSettings,
  getStreamTimeoutSettings,
  getRectifierSettings,
  getBetaPolicySettings,
  getUpstreamBillingProbeSettings,
  updateUpstreamBillingProbeSettings,
  getOllamaCloudUsageSettings,
  updateOllamaCloudUsageSettings,
  getGroups,
  listProxies,
  getProviders,
  updateProvider,
  createProvider,
  deleteProvider,
  fetchPublicSettings,
  adminSettingsFetch,
  showError,
  showSuccess,
} = vi.hoisted(() => ({
  getSettings: vi.fn(),
  updateSettings: vi.fn(),
  getWebSearchEmulationConfig: vi.fn(),
  updateWebSearchEmulationConfig: vi.fn(),
  getAdminApiKey: vi.fn(),
  getOverloadCooldownSettings: vi.fn(),
  getRateLimit429CooldownSettings: vi.fn(),
  updateRateLimit429CooldownSettings: vi.fn(),
  getPanelRateLimitSettings: vi.fn().mockResolvedValue({
    enabled: true,
    user_rpm: 240,
    heavy_rpm: 60,
    exempt_admin: true,
    public_ip_rpm: 300,
  }),
  updatePanelRateLimitSettings: vi.fn().mockImplementation(async (payload) => payload),
  getStreamTimeoutSettings: vi.fn(),
  getRectifierSettings: vi.fn(),
  getBetaPolicySettings: vi.fn(),
  getUpstreamBillingProbeSettings: vi.fn().mockResolvedValue({
    enabled: true,
    interval_minutes: 30,
  }),
  updateUpstreamBillingProbeSettings: vi.fn().mockImplementation(async (payload) => payload),
  getOllamaCloudUsageSettings: vi.fn().mockResolvedValue({
    enabled: false,
    interval_minutes: 60,
    debounce_minutes: 1,
  }),
  updateOllamaCloudUsageSettings: vi.fn().mockImplementation(async (payload) => payload),
  getGroups: vi.fn(),
  listProxies: vi.fn(),
  getProviders: vi.fn(),
  updateProvider: vi.fn(),
  createProvider: vi.fn(),
  deleteProvider: vi.fn(),
  fetchPublicSettings: vi.fn(),
  adminSettingsFetch: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}));

const localeRef = vi.hoisted(() => ({ value: "zh-CN" }));

vi.mock("@/api", () => ({
  adminAPI: {
    settings: {
      getSettings,
      updateSettings,
      getWebSearchEmulationConfig,
      updateWebSearchEmulationConfig,
      getAdminApiKey,
      getOverloadCooldownSettings,
      getRateLimit429CooldownSettings,
      updateRateLimit429CooldownSettings,
      getPanelRateLimitSettings,
      updatePanelRateLimitSettings,
      getStreamTimeoutSettings,
      getRectifierSettings,
      getBetaPolicySettings,
    },
    accounts: {
      getUpstreamBillingProbeSettings,
      updateUpstreamBillingProbeSettings,
      getOllamaCloudUsageSettings,
      updateOllamaCloudUsageSettings,
    },
    groups: {
      getAll: getGroups,
    },
    proxies: {
      list: listProxies,
    },
    payment: {
      getProviders,
      updateProvider,
      createProvider,
      deleteProvider,
    },
  },
}));

vi.mock("@/stores", () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showWarning: vi.fn(),
    showInfo: vi.fn(),
    fetchPublicSettings,
  }),
}));

vi.mock("@/stores/adminSettings", () => ({
  useAdminSettingsStore: () => ({
    fetch: adminSettingsFetch,
  }),
}));

vi.mock("@/composables/useClipboard", () => ({
  useClipboard: () => ({
    copyToClipboard: vi.fn(),
  }),
}));

vi.mock("@/utils/apiError", () => ({
  extractApiErrorMessage: () => "error",
}));

vi.mock("vue-i18n", async () => {
  const actual = await vi.importActual<typeof import("vue-i18n")>("vue-i18n");
  const translations: Record<string, string> = {
    "admin.settings.wechatConnect.title": "微信登录",
    "admin.settings.wechatConnect.description": "用于微信开放平台或公众号/小程序的第三方登录配置。",
    "admin.settings.wechatConnect.enabledLabel": "启用微信登录",
    "admin.settings.wechatConnect.enabledHint": "开启后可使用微信第三方登录回调与授权配置。",
    "admin.settings.wechatConnect.appIdLabel": "AppID",
    "admin.settings.wechatConnect.appIdPlaceholder": "微信开放平台 AppID",
    "admin.settings.wechatConnect.appSecretLabel": "AppSecret",
    "admin.settings.wechatConnect.appSecretConfiguredPlaceholder": "密钥已配置，留空以保留当前值。",
    "admin.settings.wechatConnect.appSecretPlaceholder": "微信开放平台 AppSecret",
    "admin.settings.wechatConnect.appSecretConfiguredHint": "密钥已配置，留空以保留当前值。",
    "admin.settings.wechatConnect.appSecretHint": "填写后会覆盖当前微信密钥。",
    "admin.settings.wechatConnect.modeLabel": "模式",
    "admin.settings.wechatConnect.openModeLabel": "非微信环境使用开放平台",
    "admin.settings.wechatConnect.openModeHint": "浏览器不在微信内时，自动走开放平台扫码授权。",
    "admin.settings.wechatConnect.mpModeLabel": "微信环境使用公众号",
    "admin.settings.wechatConnect.mpModeHint": "浏览器在微信内时，自动走公众号授权。",
    "admin.settings.wechatConnect.redirectUrlLabel": "回调地址",
    "admin.settings.wechatConnect.redirectUrlPlaceholder": "https://your-site.com/api/v1/auth/oauth/wechat/callback",
    "admin.settings.wechatConnect.generateAndCopy": "使用当前站点生成并复制",
    "admin.settings.wechatConnect.redirectUrlSetAndCopied": "已使用当前站点生成回调地址并复制到剪贴板",
    "admin.settings.wechatConnect.frontendRedirectUrlLabel": "前端回调地址",
    "admin.settings.wechatConnect.frontendRedirectUrlPlaceholder": "/auth/wechat/callback",
    "admin.settings.wechatConnect.frontendRedirectUrlHint": "通常用于前端路由回调地址，需与后端配置保持一致。",
    "admin.settings.authSourceDefaults.title": "认证来源默认值",
    "admin.settings.authSourceDefaults.description": "按注册来源配置新用户默认余额、并发、订阅与授权策略。",
    "admin.settings.authSourceDefaults.requireEmailLabel": "第三方注册强制补充邮箱",
    "admin.settings.authSourceDefaults.requireEmailHint": "启用后，Linux DO、OIDC、微信注册缺少邮箱时必须先补充邮箱地址。",
    "admin.settings.authSourceDefaults.enabledHint": "以下默认值会在该来源注册新用户时发放；首次绑定时授权仅作用于已有账号绑定该来源。",
    "admin.settings.authSourceDefaults.sources.email.title": "邮箱注册",
    "admin.settings.authSourceDefaults.sources.email.description": "适用于邮箱密码注册的新用户默认配额。",
    "admin.settings.authSourceDefaults.sources.linuxdo.title": "Linux DO 登录",
    "admin.settings.authSourceDefaults.sources.linuxdo.description": "适用于 Linux DO 第三方注册的新用户默认配额。",
    "admin.settings.authSourceDefaults.sources.oidc.title": "OIDC 登录",
    "admin.settings.authSourceDefaults.sources.oidc.description": "适用于 OIDC 第三方注册的新用户默认配额。",
    "admin.settings.authSourceDefaults.sources.wechat.title": "微信登录",
    "admin.settings.authSourceDefaults.sources.wechat.description": "适用于微信第三方注册的新用户默认配额。",
    "admin.settings.authSourceDefaults.grantOnFirstBindLabel": "首次绑定时授权",
    "admin.settings.authSourceDefaults.grantOnFirstBindHint": "已有账号首次绑定该来源时发放默认权益。",
    "admin.settings.authSourceDefaults.defaultSubscriptionsLabel": "默认订阅",
    "admin.settings.authSourceDefaults.defaultSubscriptionsHint": "仅对当前认证来源生效，未配置时不追加来源专属订阅。",
    "admin.settings.authSourceDefaults.noSourceSubscriptions": "当前来源未配置专属默认订阅。",
    "admin.settings.paymentVisibleMethods.methodLabel": "{title} 可见方式",
    "admin.settings.paymentVisibleMethods.methodHint": "控制前台结算页是否展示该方式，以及展示时使用的来源键。",
    "admin.settings.paymentVisibleMethods.sourceLabel": "支付来源",
    "admin.settings.paymentVisibleMethods.sourceHint": "启用后必须明确选择一个来源；未配置状态不会对外展示该支付方式。",
    "admin.settings.paymentVisibleMethods.sourceRequiredError": "{title} 已启用，请先选择支付来源。",
    "admin.settings.payment.configGuide": "查看支付配置说明",
    "admin.settings.payment.findProvider": "查看支持的支付方式",
    "admin.settings.openaiExperimentalScheduler.title": "OpenAI 实验调度策略",
    "admin.settings.openaiExperimentalScheduler.description": "默认关闭。开启后仅影响本网关在 OpenAI 账号间的实验性调度选择逻辑，不代表上游 OpenAI 官方能力。",
    "admin.settings.openaiExperimentalScheduler.lowRatePriorityTitle": "低倍率优先",
    "admin.settings.openaiExperimentalScheduler.lowRatePriorityDescription": "开启后优先选择计费倍率较低的账号；倍率相同时，再比较账号优先级和当前负载等。启用实验调度策略后，此开关不生效。",
    "admin.settings.openaiExperimentalScheduler.oauthRateTitle": "OAuth 调度参考倍率",
    "admin.settings.openaiExperimentalScheduler.oauthRatePriorityDescription": "同一分组同时包含 API Key 和 OAuth 账号时，OAuth 账号按此倍率与已探测的 API Key 计费倍率一起排序。",
    "admin.settings.openaiExperimentalScheduler.oauthRateWeightedDescription": "同一分组同时包含 API Key 和 OAuth 账号时，计算“计费倍率”得分时，OAuth 账号按此倍率参与计算。",
    "admin.settings.openaiExperimentalScheduler.stickyWeightedTitle": "粘性加权",
    "admin.settings.openaiExperimentalScheduler.stickyWeightedDescription": "开启后 previous_response_id 和 session_hash 粘性进入高级调度打分；关闭时仍按旧逻辑硬命中粘性账号。",
    "admin.settings.openaiExperimentalScheduler.subscriptionPriorityTitle": "订阅优先",
    "admin.settings.openaiExperimentalScheduler.subscriptionPriorityDescription": "开启后先在 ChatGPT 订阅账号池中按权值选取；订阅池拿不到席位时再回退到非订阅账号池。",
    "admin.settings.openaiExperimentalScheduler.weightsTitle": "调度权值覆盖",
    "admin.settings.openaiExperimentalScheduler.weightsDescription": "留空时使用配置/环境变量值；配置未设置时使用内置默认值。页面非空设置优先。",
    "admin.settings.openaiExperimentalScheduler.defaultPlaceholder": "配置/默认：{value}",
    "admin.settings.openaiExperimentalScheduler.topKLabel": "TopK",
    "admin.settings.openaiExperimentalScheduler.priorityWeight": "优先级",
    "admin.settings.openaiExperimentalScheduler.loadWeight": "负载",
    "admin.settings.openaiExperimentalScheduler.queueWeight": "排队",
    "admin.settings.openaiExperimentalScheduler.errorRateWeight": "错误率",
    "admin.settings.openaiExperimentalScheduler.ttftWeight": "首包延迟",
    "admin.settings.openaiExperimentalScheduler.resetWeight": "重置窗口",
    "admin.settings.openaiExperimentalScheduler.quotaHeadroomWeight": "额度余量",
    "admin.settings.openaiExperimentalScheduler.upstreamCostWeight": "计费倍率",
    "admin.settings.openaiExperimentalScheduler.previousResponseWeight": "previous_response 粘性",
    "admin.settings.openaiExperimentalScheduler.sessionStickyWeight": "session_hash 粘性",
    "admin.settings.upstreamBillingProbe.title": "上游倍率自动探测",
    "admin.settings.upstreamBillingProbe.description": "定期获取 OpenAI API Key 所连接上游 Sub2API 站点声明的计费倍率。",
    "admin.settings.upstreamBillingProbe.enabled": "启用全局自动探测",
    "admin.settings.upstreamBillingProbe.enabledHint": "开启后，仅对账号自身已启用自动检测的账号执行定时探测。",
    "admin.settings.upstreamBillingProbe.intervalMinutes": "探测周期（分钟）",
    "admin.settings.upstreamBillingProbe.intervalHint": "范围 5–1440 分钟。",
    "admin.settings.upstreamBillingProbe.saved": "上游倍率自动探测设置已保存",
    "admin.settings.upstreamBillingProbe.saveFailed": "保存上游倍率自动探测设置失败",
    "admin.settings.openaiFastPolicy.summaryTargetModels": "目标模型",
    "admin.settings.openaiFastPolicy.summaryAllModels": "全部模型",
    "admin.settings.openaiFastPolicy.summaryOtherModels": "其他模型",
    "admin.settings.openaiFastPolicy.summaryAction.filter": "过滤",
    "admin.settings.openaiFastPolicy.summaryAction.pass": "透传",
    "admin.settings.security.passkeyDeploymentHint":
      "请由服务器运维在部署配置中将 webauthn.enabled 设为 true，填写 webauthn.rp_id（仅域名）与 webauthn.rp_origins（完整 HTTPS 来源），然后重启服务。",
    "admin.settings.site.uploadImage": "上传图片",
    "admin.settings.site.remove": "移除",
    "admin.settings.platformQuota.platform": "平台",
    "admin.settings.platformQuota.daily": "日限额 (USD)",
    "admin.settings.platformQuota.weekly": "周限额 (USD)",
    "admin.settings.platformQuota.monthly": "月限额 (USD, 30天滚动)",
    "admin.settings.platformQuota.placeholder": "不限",
    "admin.settings.defaults.defaultPlatformQuotas": "默认平台限额（注册时分配）",
    "admin.settings.defaults.defaultPlatformQuotasHint": "新用户注册时自动写入平台限额记录；已有用户不受影响。留空 = 该平台该窗口不限制。",
    "admin.settings.defaults.platformQuotaNotice": "月限额为 30 天滚动窗口，非自然月",
    "admin.settings.authSourceDefaults.platformQuotasOverride": "平台限额覆盖",
    "admin.settings.authSourceDefaults.platformQuotasOverrideHint": "留空的字段继承「系统默认平台限额」；填 0 表示禁止该窗口使用。",
  };
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) =>
        (translations[key] ?? key).replace(/\{(\w+)\}/g, (_, token) => params?.[token] ?? `{${token}}`),
      locale: localeRef,
    }),
  };
});

const AppLayoutStub = { template: "<div><slot /></div>" };
const ToggleStub = defineComponent({
  props: {
    modelValue: {
      type: Boolean,
      default: false,
    },
  },
  emits: ["update:modelValue"],
  inheritAttrs: false,
  setup(props, { attrs, emit }) {
    return () =>
      h("input", {
        ...attrs,
        class: "toggle-stub",
        type: "checkbox",
        checked: props.modelValue,
        onChange: (event: Event) => {
          emit("update:modelValue", (event.target as HTMLInputElement).checked);
        },
      });
  },
});

const SelectStub = defineComponent({
  props: {
    modelValue: {
      type: [String, Number, Boolean, null],
      default: "",
    },
    options: {
      type: Array,
      default: () => [],
    },
    placeholder: {
      type: String,
      default: "",
    },
  },
  emits: ["update:modelValue", "change"],
  setup(props, { emit }) {
    const onChange = (event: Event) => {
      const target = event.target as HTMLSelectElement;
      emit("update:modelValue", target.value);
      const option =
        (props.options as Array<Record<string, unknown>>).find(
          (item) => String(item.value ?? "") === target.value,
        ) ?? null;
      emit("change", target.value, option);
    };

    return () =>
      h(
        "select",
        {
          class: "select-stub",
          value: props.modelValue ?? "",
          "data-placeholder": props.placeholder,
          onChange,
        },
        (props.options as Array<Record<string, unknown>>).map((option) =>
          h(
            "option",
            {
              key: `${String(option.value ?? "")}:${String(option.label ?? "")}`,
              value: option.value as string,
            },
            String(option.label ?? ""),
          ),
        ),
      );
  },
});

const ImageUploadStub = defineComponent({
  props: {
    modelValue: {
      type: String,
      default: "",
    },
    uploadLabel: {
      type: String,
      default: "",
    },
    removeLabel: {
      type: String,
      default: "",
    },
    placeholder: {
      type: String,
      default: "",
    },
  },
  setup(props) {
    return () =>
      h("div", {
        class: "image-upload-stub",
        "data-model-value": props.modelValue,
        "data-upload-label": props.uploadLabel,
        "data-remove-label": props.removeLabel,
        "data-placeholder": props.placeholder,
      });
  },
});

const baseSettingsResponse = {
  registration_enabled: true,
  registration_email_suffix_whitelist: [],
  promo_code_enabled: true,
  invitation_code_enabled: false,
  totp_enabled: false,
  totp_encryption_key_configured: false,
  passkey_enabled: true,
  passkey_configured: true,
  passkey_rp_id: "sub3.nebula-spaces.com",
  passkey_rp_origins: ["https://sub3.nebula-spaces.com"],
  default_concurrency: 1,
  doc_url: "",
  backend_mode_enabled: false,
  custom_endpoints: [],
  turnstile_enabled: false,
  turnstile_site_key: "",
  turnstile_secret_key_configured: false,
  tencent_captcha_enabled: false,
  tencent_captcha_app_id: "",
  tencent_captcha_app_secret_key_configured: false,
  tencent_captcha_cloud_secret_id_configured: false,
  tencent_captcha_cloud_secret_key_configured: false,
  api_key_acl_trust_forwarded_ip: true,
  forwarded_client_ip_headers: [],
  linuxdo_connect_enabled: false,
  linuxdo_connect_client_id: "",
  linuxdo_connect_client_secret_configured: false,
  linuxdo_connect_redirect_url: "",
  wechat_connect_enabled: true,
  wechat_connect_app_id: "wx-app-id-123",
  wechat_connect_app_secret_configured: true,
  wechat_connect_open_enabled: false,
  wechat_connect_mp_enabled: true,
  wechat_connect_mode: "mp",
  wechat_connect_scopes: "",
  wechat_connect_redirect_url:
    "https://admin.example.com/api/v1/auth/oauth/wechat/callback",
  wechat_connect_frontend_redirect_url: "/auth/wechat/callback",
  oidc_connect_enabled: false,
  oidc_connect_provider_name: "OIDC",
  oidc_connect_client_id: "",
  oidc_connect_client_secret_configured: false,
  oidc_connect_issuer_url: "",
  oidc_connect_discovery_url: "",
  oidc_connect_authorize_url: "",
  oidc_connect_token_url: "",
  oidc_connect_userinfo_url: "",
  oidc_connect_jwks_url: "",
  oidc_connect_scopes: "openid email profile",
  oidc_connect_redirect_url: "",
  oidc_connect_frontend_redirect_url: "/auth/oidc/callback",
  oidc_connect_token_auth_method: "client_secret_post",
  oidc_connect_use_pkce: true,
  oidc_connect_validate_id_token: true,
  oidc_connect_allowed_signing_algs: "RS256,ES256,PS256",
  oidc_connect_clock_skew_seconds: 120,
  oidc_connect_require_email_verified: false,
  oidc_connect_userinfo_email_path: "",
  oidc_connect_userinfo_id_path: "",
  oidc_connect_userinfo_username_path: "",
  enable_model_fallback: false,
  fallback_model_anthropic: "",
  fallback_model_openai: "",
  fallback_model_gemini: "",
  fallback_model_antigravity: "",
  grok_default_text_model: "grok-4.5",
  grok_cross_client_model_map_enabled: false,
  enable_identity_patch: false,
  identity_patch_prompt: "",
  ops_monitoring_enabled: false,
  ops_realtime_monitoring_enabled: false,
  ops_query_mode_default: "auto",
  ops_metrics_interval_seconds: 60,
  min_claude_code_version: "",
  max_claude_code_version: "",
  allow_ungrouped_key_scheduling: false,
  enable_fingerprint_unification: true,
  enable_metadata_passthrough: false,
  enable_cch_signing: false,
  enable_claude_oauth_system_prompt_injection: true,
  claude_oauth_system_prompt: "",
  claude_oauth_system_prompt_blocks: "",
  enable_anthropic_cache_ttl_1h_injection: false,
  rewrite_message_cache_control: false,
  enable_client_dateline_normalization: true,
  antigravity_user_agent_version: "",
  openai_codex_user_agent: "",
  openai_low_upstream_rate_priority_enabled: false,
  openai_oauth_scheduling_rate_multiplier: 1,
  openai_advanced_scheduler_enabled: false,
  openai_advanced_scheduler_sticky_weighted_enabled: false,
  openai_advanced_scheduler_subscription_priority_enabled: false,
  openai_advanced_scheduler_lb_top_k: "",
  openai_advanced_scheduler_weight_priority: "",
  openai_advanced_scheduler_weight_load: "",
  openai_advanced_scheduler_weight_queue: "",
  openai_advanced_scheduler_weight_error_rate: "",
  openai_advanced_scheduler_weight_ttft: "",
  openai_advanced_scheduler_weight_reset: "",
  openai_advanced_scheduler_weight_quota_headroom: "",
  openai_advanced_scheduler_weight_upstream_cost: "",
  openai_advanced_scheduler_weight_previous_response: "",
  openai_advanced_scheduler_weight_session_sticky: "",
  openai_advanced_scheduler_effective_lb_top_k: "7",
  openai_advanced_scheduler_effective_weight_priority: "1",
  openai_advanced_scheduler_effective_weight_load: "1",
  openai_advanced_scheduler_effective_weight_queue: "0.7",
  openai_advanced_scheduler_effective_weight_error_rate: "0.8",
  openai_advanced_scheduler_effective_weight_ttft: "0.5",
  openai_advanced_scheduler_effective_weight_reset: "0",
  openai_advanced_scheduler_effective_weight_quota_headroom: "0",
  openai_advanced_scheduler_effective_weight_upstream_cost: "0",
  openai_advanced_scheduler_effective_weight_previous_response: "5",
  openai_advanced_scheduler_effective_weight_session_sticky: "3",
  // 平台限额嵌套字段（新后端契约）
  default_platform_quotas: {
    anthropic:   { daily: null, weekly: null, monthly: null },
    openai:      { daily: null, weekly: 12.5, monthly: null },
    gemini:      { daily: null, weekly: null, monthly: 200 },
    antigravity: { daily: null, weekly: null, monthly: null },
  },
};

function mountView() {
  return mount(SettingsView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        Select: SelectStub,
        Toggle: ToggleStub,
        Icon: true,
        ConfirmDialog: true,
        PaymentProviderList: true,
        PaymentProviderDialog: true,
        GroupBadge: true,
        GroupOptionItem: true,
        ProxySelector: true,
        ImageUpload: ImageUploadStub,
        BackupSettings: true,
      },
    },
  });
}

async function openSecurityTab(wrapper: ReturnType<typeof mountView>) {
  const securityTabButton = wrapper
    .findAll("button")
    .find((node) => node.text().includes("admin.settings.tabs.security"));

  expect(securityTabButton).toBeDefined();
  await securityTabButton?.trigger("click");
  await flushPromises();
}

async function openUsersTab(wrapper: ReturnType<typeof mountView>) {
  const usersTabButton = wrapper
    .findAll("button")
    .find((node) => node.text().includes("admin.settings.tabs.users"));

  expect(usersTabButton).toBeDefined();
  await usersTabButton?.trigger("click");
  await flushPromises();
}

describe("admin SettingsView email suffix whitelist copy", () => {
  it("documents the empty-whitelist behavior in both locales", () => {
    // 白名单 hint 描述严格默认语义（非白名单域名限量注册开关已随注册体系删除）。
    const zhWhitelistHint = zhSettings.settings.registration.emailSuffixWhitelistHint;
    const enWhitelistHint = enSettings.settings.registration.emailSuffixWhitelistHint;
    expect(zhWhitelistHint).toContain("留空则不限制");
    expect(enWhitelistHint).toContain("leave empty for no restriction");
  });
});

describe("admin SettingsView platform quota matrix", () => {
  beforeEach(() => {
    getSettings.mockReset();
    updateSettings.mockReset();
    getWebSearchEmulationConfig.mockReset();
    updateWebSearchEmulationConfig.mockReset();
    getAdminApiKey.mockReset();
    getOverloadCooldownSettings.mockReset();
    getRateLimit429CooldownSettings.mockReset();
    updateRateLimit429CooldownSettings.mockReset();
    getStreamTimeoutSettings.mockReset();
    getRectifierSettings.mockReset();
    getBetaPolicySettings.mockReset();
    getGroups.mockReset();
    listProxies.mockReset();
    getProviders.mockReset();
    updateProvider.mockReset();
    createProvider.mockReset();
    deleteProvider.mockReset();
    fetchPublicSettings.mockReset();
    adminSettingsFetch.mockReset();
    showError.mockReset();
    showSuccess.mockReset();
    localeRef.value = "zh-CN";

    getSettings.mockResolvedValue({ ...baseSettingsResponse });
    updateSettings.mockImplementation(async (payload) => ({
      ...baseSettingsResponse,
      ...payload,
    }));
    getWebSearchEmulationConfig.mockResolvedValue({ enabled: false, providers: [] });
    updateWebSearchEmulationConfig.mockResolvedValue({ enabled: false, providers: [] });
    getAdminApiKey.mockResolvedValue({ exists: false, masked_key: "" });
    getOverloadCooldownSettings.mockResolvedValue({});
    getRateLimit429CooldownSettings.mockResolvedValue({});
    updateRateLimit429CooldownSettings.mockResolvedValue({});
    getStreamTimeoutSettings.mockResolvedValue({});
    getRectifierSettings.mockResolvedValue({});
    getBetaPolicySettings.mockResolvedValue({});
    getGroups.mockResolvedValue([]);
    listProxies.mockResolvedValue({ items: [] });
    getProviders.mockResolvedValue({ data: [] });
  });

  it("从 baseSettings 加载默认平台配额数据并在 Users tab 渲染 5 平台行", async () => {
    const wrapper = mountView();
    await flushPromises();
    await openUsersTab(wrapper);

    expect(getSettings).toHaveBeenCalled();

    const html = wrapper.html();
    // 表格行的平台字段：font-mono 渲染纯英文 platform key
    expect(html).toContain("anthropic");
    expect(html).toContain("openai");
    expect(html).toContain("gemini");
    expect(html).toContain("antigravity");
  });

  it("保存时 updateSettings payload 应包含嵌套 default_platform_quotas 对象（含全 5 平台）", async () => {
    const wrapper = mountView();
    await flushPromises();
    await openUsersTab(wrapper);

    await wrapper.find("form").trigger("submit.prevent");
    await flushPromises();

    expect(updateSettings).toHaveBeenCalled();
    const lastCallArgs = updateSettings.mock.calls.at(-1);
    expect(lastCallArgs).toBeDefined();
    const payload = lastCallArgs![0] as Record<string, unknown>;

    // 应携带嵌套对象，而非扁平字段
    expect(payload).toHaveProperty("default_platform_quotas");
    const quotas = payload["default_platform_quotas"] as Record<string, unknown>;
    const platforms = ["anthropic", "openai", "gemini", "antigravity", "grok"];
    for (const p of platforms) {
      expect(quotas).toHaveProperty(p);
      const pq = quotas[p] as Record<string, unknown>;
      expect(pq).toHaveProperty("daily");
      expect(pq).toHaveProperty("weekly");
      expect(pq).toHaveProperty("monthly");
    }

    // 不应存在旧扁平字段
    expect(payload).not.toHaveProperty("default_platform_quota_anthropic_daily");
    expect(payload).not.toHaveProperty("default_platform_quota_openai_weekly");
  });

  it("加载后 form.default_platform_quotas 含全 5 平台，从嵌套 JSON 正确读取数值", async () => {
    getSettings.mockResolvedValueOnce({
      ...baseSettingsResponse,
      default_platform_quotas: {
        anthropic: { daily: 5, weekly: null, monthly: null },
        openai:    { daily: null, weekly: 12.5, monthly: null },
        // gemini / antigravity 缺失 → 应被归一化为全 null
      },
    });

    const wrapper = mountView();
    await flushPromises();
    await openUsersTab(wrapper);

    await wrapper.find("form").trigger("submit.prevent");
    await flushPromises();

    const payload = updateSettings.mock.calls.at(-1)![0] as Record<string, unknown>;
    const quotas = payload["default_platform_quotas"] as Record<string, Record<string, unknown>>;

    expect(quotas["anthropic"]?.["daily"]).toBe(5);
    expect(quotas["openai"]?.["weekly"]).toBe(12.5);
    // 缺失平台应补全为 null
    expect(quotas["gemini"]).toEqual({ daily: null, weekly: null, monthly: null });
    expect(quotas["antigravity"]).toEqual({ daily: null, weekly: null, monthly: null });
  });

  it("空输入（v-model.number 产出 \"\"）在提交时清洗为 null 而非空字符串", async () => {
    // 模拟后端返回带有 anthropic daily 值的配额
    getSettings.mockResolvedValueOnce({
      ...baseSettingsResponse,
      default_platform_quotas: {
        anthropic: { daily: 10, weekly: null, monthly: null },
        openai:    { daily: null, weekly: null, monthly: null },
        gemini:    { daily: null, weekly: null, monthly: null },
        antigravity: { daily: null, weekly: null, monthly: null },
      },
    });

    const wrapper = mountView();
    await flushPromises();
    await openUsersTab(wrapper);

    // 找到 anthropic daily 输入框并清空（模拟用户删除值）
    const inputs = wrapper.findAll('input[type="number"]');
    const anthropicDailyInput = inputs.find((i) => {
      const parent = i.element.closest("tr");
      return parent?.textContent?.includes("anthropic");
    });

    if (anthropicDailyInput) {
      // 设置为空字符串，模拟 v-model.number 在清空时产出 ""
      await anthropicDailyInput.setValue("");
    }

    await wrapper.find("form").trigger("submit.prevent");
    await flushPromises();

    const payload = updateSettings.mock.calls.at(-1)![0] as Record<string, unknown>;
    const quotas = payload["default_platform_quotas"] as Record<string, Record<string, unknown>>;
    // 不管输入是什么，提交值应为 null（而非 "" 或 NaN）
    expect(quotas["anthropic"]?.["daily"]).toBe(null);
  });
});

// 登录入口 / 默认首页从"只能改配置文件"挪到了后台，随之而来的最大风险是
// 管理员点几下就把登录页关掉、自己再也进不来。这一组用例守的就是那几道闸：
// 非法值拒绝、二次确认带完整地址、配置文件锁定时界面只读且不下发这几项。
describe("admin SettingsView login entry controls", () => {
  beforeEach(() => {
    getSettings.mockReset();
    updateSettings.mockReset();
    getWebSearchEmulationConfig.mockReset();
    updateWebSearchEmulationConfig.mockReset();
    getAdminApiKey.mockReset();
    getOverloadCooldownSettings.mockReset();
    getRateLimit429CooldownSettings.mockReset();
    updateRateLimit429CooldownSettings.mockReset();
    getStreamTimeoutSettings.mockReset();
    getRectifierSettings.mockReset();
    getBetaPolicySettings.mockReset();
    getGroups.mockReset();
    listProxies.mockReset();
    getProviders.mockReset();
    updateProvider.mockReset();
    createProvider.mockReset();
    deleteProvider.mockReset();
    fetchPublicSettings.mockReset();
    adminSettingsFetch.mockReset();
    showError.mockReset();
    showSuccess.mockReset();

    getSettings.mockResolvedValue({ ...baseSettingsResponse });
    updateSettings.mockImplementation(async (payload) => ({
      ...baseSettingsResponse,
      ...payload,
    }));
    getWebSearchEmulationConfig.mockResolvedValue({
      enabled: false,
      providers: [],
    });
    updateWebSearchEmulationConfig.mockResolvedValue({
      enabled: false,
      providers: [],
    });
    getAdminApiKey.mockResolvedValue({
      exists: false,
      masked_key: "",
    });
    getOverloadCooldownSettings.mockResolvedValue({
      enabled: true,
      cooldown_minutes: 10,
    });
    getRateLimit429CooldownSettings.mockResolvedValue({
      enabled: true,
      cooldown_seconds: 5,
    });
    updateRateLimit429CooldownSettings.mockImplementation(async (payload) => payload);
    getStreamTimeoutSettings.mockResolvedValue({
      enabled: true,
      action: "temp_unsched",
      temp_unsched_minutes: 5,
      threshold_count: 3,
      threshold_window_minutes: 10,
    });
    getRectifierSettings.mockResolvedValue({
      enabled: true,
      thinking_signature_enabled: true,
      thinking_budget_enabled: true,
      apikey_signature_enabled: false,
      apikey_signature_patterns: [],
    });
    getBetaPolicySettings.mockResolvedValue({
      rules: [],
    });
    getGroups.mockResolvedValue([]);
    listProxies.mockResolvedValue({
      items: [],
    });
    getProviders.mockResolvedValue({
      data: [],
    });
    fetchPublicSettings.mockResolvedValue(undefined);
    adminSettingsFetch.mockResolvedValue(undefined);
  });

  it("loads the effective login entry and shows the resulting login URL", async () => {
    getSettings.mockResolvedValue({
      ...baseSettingsResponse,
      login_entry_public: false,
      login_entry_path: "/j7q2m9x4vk3p",
      default_home_path: "/home",
      login_entry_locked_by_config: false,
      default_home_path_locked_by_config: false,
    });

    const wrapper = mountView();
    await flushPromises();
    await openSecurityTab(wrapper);

    const toggle = wrapper.get('[data-testid="login-entry-hidden-toggle"]');
    expect((toggle.element as HTMLInputElement).checked).toBe(true);
    expect(wrapper.get('[data-testid="login-entry-url"]').text()).toBe(
      `${window.location.origin}/j7q2m9x4vk3p`,
    );
    expect(wrapper.find('[data-testid="login-entry-locked-banner"]').exists()).toBe(
      false,
    );
  });

  it("shows a lock banner, disables the controls and omits the pinned fields from the payload", async () => {
    getSettings.mockResolvedValue({
      ...baseSettingsResponse,
      login_entry_public: true,
      login_entry_path: "",
      default_home_path: "/home",
      login_entry_locked_by_config: true,
      default_home_path_locked_by_config: true,
    });

    const wrapper = mountView();
    await flushPromises();
    await openSecurityTab(wrapper);

    expect(wrapper.find('[data-testid="login-entry-locked-banner"]').exists()).toBe(
      true,
    );
    const toggle = wrapper.get('[data-testid="login-entry-hidden-toggle"]');
    expect((toggle.element as HTMLInputElement).disabled).toBe(true);

    await wrapper.find("form").trigger("submit.prevent");
    await flushPromises();

    expect(updateSettings).toHaveBeenCalledTimes(1);
    const payload = updateSettings.mock.calls[0][0];
    expect(payload).not.toHaveProperty("login_entry_public");
    expect(payload).not.toHaveProperty("login_entry_path");
    expect(payload).not.toHaveProperty("default_home_path");
  });

  it("refuses to save a hidden login entry without a usable path", async () => {
    getSettings.mockResolvedValue({
      ...baseSettingsResponse,
      login_entry_public: true,
      login_entry_path: "",
      default_home_path: "/key-usage",
      login_entry_locked_by_config: false,
      default_home_path_locked_by_config: false,
    });

    const wrapper = mountView();
    await flushPromises();
    await openSecurityTab(wrapper);

    await wrapper
      .get('[data-testid="login-entry-hidden-toggle"]')
      .setValue(true);
    await wrapper.find("form").trigger("submit.prevent");
    await flushPromises();

    expect(updateSettings).not.toHaveBeenCalled();
    expect(showError).toHaveBeenCalled();
  });

  it("refuses to save a login path that collides with a backend prefix", async () => {
    getSettings.mockResolvedValue({
      ...baseSettingsResponse,
      login_entry_public: true,
      login_entry_path: "",
      default_home_path: "/key-usage",
      login_entry_locked_by_config: false,
      default_home_path_locked_by_config: false,
    });

    const wrapper = mountView();
    await flushPromises();
    await openSecurityTab(wrapper);

    await wrapper
      .get('[data-testid="login-entry-hidden-toggle"]')
      .setValue(true);
    await wrapper.get("#login-entry-path").setValue("/api/secret-gate");
    expect(wrapper.get('[data-testid="login-entry-path-error"]').exists()).toBe(
      true,
    );

    await wrapper.find("form").trigger("submit.prevent");
    await flushPromises();
    expect(updateSettings).not.toHaveBeenCalled();
  });

  it("asks for confirmation with the full login URL before hiding the entry", async () => {
    getSettings.mockResolvedValue({
      ...baseSettingsResponse,
      login_entry_public: true,
      login_entry_path: "",
      default_home_path: "/key-usage",
      login_entry_locked_by_config: false,
      default_home_path_locked_by_config: false,
    });

    const wrapper = mountView();
    await flushPromises();
    await openSecurityTab(wrapper);

    await wrapper
      .get('[data-testid="login-entry-hidden-toggle"]')
      .setValue(true);
    await wrapper.get("#login-entry-path").setValue("/j7q2m9x4vk3p");

    // 第一次提交只弹确认框，不保存。
    await wrapper.find("form").trigger("submit.prevent");
    await flushPromises();
    expect(updateSettings).not.toHaveBeenCalled();

    const dialog = wrapper
      .findAllComponents({ name: "ConfirmDialog" })
      .find((node) => node.props("show") === true);
    expect(dialog).toBeDefined();
    expect(dialog?.props("title")).toBe(
      "admin.settings.loginEntry.confirmTitle",
    );

    // 确认之后才真的下发，且 payload 带上归一化后的路径。
    dialog?.vm.$emit("confirm");
    await flushPromises();

    expect(updateSettings).toHaveBeenCalledTimes(1);
    expect(updateSettings.mock.calls[0][0]).toMatchObject({
      login_entry_public: false,
      login_entry_path: "/j7q2m9x4vk3p",
      default_home_path: "/key-usage",
    });
  });

  it("saves without confirmation when the login entry is untouched", async () => {
    getSettings.mockResolvedValue({
      ...baseSettingsResponse,
      login_entry_public: true,
      login_entry_path: "",
      default_home_path: "/key-usage",
      login_entry_locked_by_config: false,
      default_home_path_locked_by_config: false,
    });

    const wrapper = mountView();
    await flushPromises();

    await wrapper.find("form").trigger("submit.prevent");
    await flushPromises();

    expect(updateSettings).toHaveBeenCalledTimes(1);
    expect(updateSettings.mock.calls[0][0]).toMatchObject({
      login_entry_public: true,
      default_home_path: "/key-usage",
    });
  });

  it("drops /login as a landing page when the entry is hidden", async () => {
    getSettings.mockResolvedValue({
      ...baseSettingsResponse,
      login_entry_public: true,
      login_entry_path: "",
      default_home_path: "/login",
      login_entry_locked_by_config: false,
      default_home_path_locked_by_config: false,
    });

    const wrapper = mountView();
    await flushPromises();
    await openSecurityTab(wrapper);

    await wrapper
      .get('[data-testid="login-entry-hidden-toggle"]')
      .setValue(true);
    await wrapper.get("#login-entry-path").setValue("/j7q2m9x4vk3p");

    const select = wrapper.get("#default-home-path").element as HTMLSelectElement;
    expect(select.value).toBe("/key-usage");
    expect(
      Array.from(select.options).map((option) => option.value),
    ).not.toContain("/login");
  });
});

// 后台的这几句文案是要给站长看的，别把"隐藏登录入口"写成"保证安全"。
describe("admin SettingsView login entry copy", () => {
  it("describes hidden login as an exposure reduction, not a security boundary", () => {
    for (const copy of [
      zhSettings.settings.loginEntry,
      enSettings.settings.loginEntry,
    ]) {
      expect(copy.notASecurityBoundary).toBeTruthy();
    }
    expect(zhSettings.settings.loginEntry.notASecurityBoundary).toContain(
      "暴露面",
    );
    expect(zhSettings.settings.loginEntry.notASecurityBoundary).toContain(
      "2FA",
    );
    expect(enSettings.settings.loginEntry.notASecurityBoundary).toContain(
      "does not stop",
    );
    expect(enSettings.settings.loginEntry.notASecurityBoundary).toContain(
      "2FA",
    );
  });

  it("keeps the break-glass instructions in both locales", () => {
    expect(zhSettings.settings.loginEntry.confirmBreakGlass).toContain(
      "配置文件",
    );
    expect(enSettings.settings.loginEntry.confirmBreakGlass).toContain(
      "config file",
    );
  });
});
