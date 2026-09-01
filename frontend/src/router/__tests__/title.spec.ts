import { describe, expect, it } from 'vitest'
import { resolveDocumentTitle, resolveRouteDocumentTitle } from '@/router/title'

describe('resolveDocumentTitle', () => {
  it('路由存在标题时，使用“路由标题 - 站点名”格式', () => {
    expect(resolveDocumentTitle('Usage Records')).toBe('Usage Records - Sub2API')
  })

  it('路由无标题时，回退到站点名', () => {
    expect(resolveDocumentTitle(undefined)).toBe('Sub2API')
  })

  it('空白标题不产生只剩分隔符的标题', () => {
    expect(resolveDocumentTitle('   ')).toBe('Sub2API')
  })
})

describe('resolveRouteDocumentTitle', () => {
  it('使用路由静态标题拼接固定站点名', () => {
    const route = {
      name: 'Usage',
      params: {},
      meta: {
        title: 'Usage Records'
      }
    }

    expect(resolveRouteDocumentTitle(route)).toBe('Usage Records - Sub2API')
  })

  it('路由无标题时只显示站点名，绝不为空', () => {
    const route = { name: 'Unknown', params: {}, meta: {} }

    expect(resolveRouteDocumentTitle(route)).toBe('Sub2API')
  })
})
