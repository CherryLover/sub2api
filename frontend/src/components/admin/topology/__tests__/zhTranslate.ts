import zh from '@/i18n/locales/zh'

/** 测试用翻译：查真实中文文案表并做 {name} 插值；key 不存在时直接抛错，暴露拼写错误。 */
export function zhT(key: string, params?: Record<string, unknown>): string {
  const value = key.split('.').reduce<unknown>((acc, part) => {
    if (acc && typeof acc === 'object') return (acc as Record<string, unknown>)[part]
    return undefined
  }, zh as unknown)
  if (typeof value !== 'string') throw new Error(`missing locale key: ${key}`)
  return value.replace(/\{(\w+)\}/g, (_, name: string) =>
    params && name in params ? String(params[name]) : `{${name}}`
  )
}
