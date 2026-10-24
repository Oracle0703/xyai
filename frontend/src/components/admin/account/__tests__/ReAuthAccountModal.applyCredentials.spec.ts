import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const source = readFileSync(
  resolve(process.cwd(), 'src/components/admin/account/ReAuthAccountModal.vue'),
  'utf8'
)

describe('ReAuthAccountModal credential persistence', () => {
  it('persists every platform through apply-oauth-credentials instead of the full account update', () => {
    // 受限账号运维子管理员只被授权 apply-oauth-credentials；通用 PUT /accounts/:id 与 clear-error 会被拒绝。
    expect(source).not.toContain('adminAPI.accounts.update(')
    expect(source).not.toContain('adminAPI.accounts.clearError(')
    expect(source.match(/adminAPI\.accounts\.applyOAuthCredentials\(/g)?.length ?? 0).toBeGreaterThanOrEqual(8)
  })
})
