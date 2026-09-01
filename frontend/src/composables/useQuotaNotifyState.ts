import { reactive, ref } from 'vue'
import { QUOTA_THRESHOLD_TYPE_FIXED, type QuotaThresholdType } from '@/constants/account'

export const QUOTA_NOTIFY_DIMS = ['daily', 'weekly', 'total'] as const
export type QuotaNotifyDim = (typeof QUOTA_NOTIFY_DIMS)[number]

interface DimState {
  enabled: boolean | null
  threshold: number | null
  thresholdType: QuotaThresholdType | null
}

export function useQuotaNotifyState() {
  // 邮件体系已整体移除，账号额度提醒没有任何推送通道，全局开关恒为关闭；
  // QuotaDimensionRow 据此把每个维度的"提醒阈值"控件整体隐藏。
  const globalEnabled = ref(false)
  const state = reactive<Record<QuotaNotifyDim, DimState>>({
    daily: { enabled: null, threshold: null, thresholdType: null },
    weekly: { enabled: null, threshold: null, thresholdType: null },
    total: { enabled: null, threshold: null, thresholdType: null },
  })

  function loadFromExtra(extra: Record<string, unknown> | null | undefined) {
    for (const d of QUOTA_NOTIFY_DIMS) {
      state[d].enabled = (extra?.[`quota_notify_${d}_enabled`] as boolean) ?? null
      state[d].threshold = (extra?.[`quota_notify_${d}_threshold`] as number) ?? null
      state[d].thresholdType = (extra?.[`quota_notify_${d}_threshold_type`] as QuotaThresholdType) ?? null
    }
  }

  function writeToExtra(extra: Record<string, unknown>, mode: 'create' | 'update') {
    for (const d of QUOTA_NOTIFY_DIMS) {
      const s = state[d]
      if (s.enabled) {
        extra[`quota_notify_${d}_enabled`] = true
        if (s.threshold != null) {
          extra[`quota_notify_${d}_threshold`] = s.threshold
        } else if (mode === 'update') {
          delete extra[`quota_notify_${d}_threshold`]
        }
        extra[`quota_notify_${d}_threshold_type`] = s.thresholdType || QUOTA_THRESHOLD_TYPE_FIXED
      } else if (mode === 'update') {
        delete extra[`quota_notify_${d}_enabled`]
        delete extra[`quota_notify_${d}_threshold`]
        delete extra[`quota_notify_${d}_threshold_type`]
      }
    }
  }

  function reset() {
    for (const d of QUOTA_NOTIFY_DIMS) {
      state[d].enabled = null
      state[d].threshold = null
      state[d].thresholdType = null
    }
  }

  return { globalEnabled, state, loadFromExtra, writeToExtra, reset }
}
