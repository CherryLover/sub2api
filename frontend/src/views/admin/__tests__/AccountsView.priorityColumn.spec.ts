import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

const source = readFileSync(
  resolve(process.cwd(), 'src/views/admin/AccountsView.vue'),
  'utf8'
)

describe('AccountsView priority column default', () => {
  it('shows priority by default instead of hiding it with scheduler score', () => {
    const match = source.match(/const DEFAULT_HIDDEN_COLUMNS = (\[[^\]]*\])/)
    expect(match).toBeTruthy()
    expect(match![1]).not.toContain("'priority'")
    expect(match![1]).toContain("'scheduler_score'")
  })
})
