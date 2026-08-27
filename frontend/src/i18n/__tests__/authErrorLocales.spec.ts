import { describe, expect, it } from 'vitest'
import en from '@/i18n/locales/en'
import zh from '@/i18n/locales/zh'
import { extractI18nErrorMessage } from '@/utils/apiError'

// 登录/认证链路的高频后端错误码，缺翻译时前端会把英文原文（如 "invalid email or password"）
// 直接弹给用户。
const REQUIRED_AUTH_ERROR_CODES = [
  'INVALID_CREDENTIALS',
  'USER_NOT_ACTIVE',
  'BACKEND_MODE_ADMIN_ONLY',
  'EMAIL_VERIFY_REQUIRED',
  'SERVICE_UNAVAILABLE',
  'TOTP_INVALID_CODE',
  'TOTP_NOT_SETUP',
  'TOTP_TOO_MANY_ATTEMPTS'
]

function lookup(messages: Record<string, any>, key: string): unknown {
  return key.split('.').reduce<any>((acc, part) => (acc == null ? acc : acc[part]), messages)
}

function makeTranslate(messages: Record<string, any>) {
  // 模拟 vue-i18n：缺失的键原样返回。
  return (key: string) => {
    const value = lookup(messages, key)
    return typeof value === 'string' ? value : key
  }
}

describe('auth.errors locale completeness', () => {
  for (const code of REQUIRED_AUTH_ERROR_CODES) {
    it(`zh locale has auth.errors.${code}`, () => {
      const value = lookup(zh, `auth.errors.${code}`)
      expect(typeof value).toBe('string')
      expect(value).not.toBe('')
      // 中文文案必须含中日韩字符，避免误把英文原文抄进 zh 文件。
      expect(String(value)).toMatch(/[一-龥]/)
    })

    it(`en locale has auth.errors.${code}`, () => {
      const value = lookup(en, `auth.errors.${code}`)
      expect(typeof value).toBe('string')
      expect(value).not.toBe('')
    })
  }
})

describe('extractI18nErrorMessage maps login failures to locale text', () => {
  it('translates INVALID_CREDENTIALS instead of showing the backend message', () => {
    const error = {
      status: 401,
      reason: 'INVALID_CREDENTIALS',
      message: 'invalid email or password'
    }

    const zhMessage = extractI18nErrorMessage(error, makeTranslate(zh), 'auth.errors', 'fallback')
    expect(zhMessage).toBe(lookup(zh, 'auth.errors.INVALID_CREDENTIALS'))
    expect(zhMessage).not.toBe('invalid email or password')

    const enMessage = extractI18nErrorMessage(error, makeTranslate(en), 'auth.errors', 'fallback')
    expect(enMessage).toBe(lookup(en, 'auth.errors.INVALID_CREDENTIALS'))
  })

  it('falls back to the backend message for unmapped codes', () => {
    const error = { status: 500, reason: 'SOME_UNMAPPED_CODE', message: 'boom' }
    expect(extractI18nErrorMessage(error, makeTranslate(zh), 'auth.errors', 'fallback')).toBe('boom')
  })
})
