import { describe, expect, it } from 'vitest'
import router from '@/router'

describe('topology route', () => {
  it('registers the admin topology page with admin metadata', () => {
    const route = router.getRoutes().find(item => item.name === 'AdminTopology')

    expect(route).toMatchObject({
      path: '/admin/topology',
      name: 'AdminTopology',
      meta: {
        requiresAuth: true,
        requiresAdmin: true,
        title: 'Routing Topology',
        titleKey: 'admin.topology.title',
        descriptionKey: 'admin.topology.description'
      }
    })
  })
})
