import { AxeBuilder } from '@axe-core/playwright'
import { expect, test, type Page } from '@playwright/test'

const tenant = {
  id: '01999c55-8d0c-7c1e-8c2d-000000000001',
  slug: 'acme',
  displayName: 'Acme Platform',
  role: 'tenant_admin'
}

const me = {
  user: {
    id: '01999c55-8d0c-7c1e-8c2d-000000000010',
    etag: 'v1-user',
    username: 'operator',
    displayName: 'Platform Operator',
    email: 'operator@example.com',
    status: 'active',
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z'
  },
  tenants: [tenant],
  isPlatformAdmin: true
}

const job = {
  id: '01999c55-8d0c-7c1e-8c2d-000000000020',
  tenantSlug: 'acme',
  retryOfJobId: null,
  type: 'repo.sync',
  trigger: 'api',
  status: 'running',
  stage: 'discover',
  scopeType: 'repository',
  scopeId: '01999c55-8d0c-7c1e-8c2d-000000000030',
  refType: 'branch',
  ref: 'main',
  result: null,
  progress: 20,
  dirty: false,
  attempt: 1,
  maxAttempts: 3,
  nextAttemptAt: null,
  attempts: [{ stage: 'discover', attempt: 1, status: 'running', startedAt: '2026-01-01T00:00:00Z', finishedAt: null, error: null }],
  error: null,
  createdAt: '2026-01-01T00:00:00Z',
  startedAt: '2026-01-01T00:00:02Z',
  finishedAt: null,
  capabilities: ['job:read', 'job:run']
}

const repository = {
  id: '01999c55-8d0c-7c1e-8c2d-000000000030',
  etag: 'v1-repository',
  url: 'https://git.example.com/team/service.git',
  credentialId: null,
  defaultBranch: 'main',
  branchPolicy: { branchPatterns: ['main'], tagPatterns: [] },
  fetchConfig: { shallow: false, depth: 50, submodules: false, proxy: null, pathAllow: [], pathIgnore: [], knownHostPolicy: 'strict' },
  syncCron: null,
  note: null,
  health: { lastSyncAt: null, lastCommit: null, lastError: null, failStreak: 0, durationMs: null },
  capabilities: ['repository:read', 'repository:write'],
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z'
}

const credential = {
  id: '01999c55-8d0c-7c1e-8c2d-000000000040',
  etag: 'v1-credential',
  name: 'deploy-key',
  kind: 'ssh_key',
  fingerprint: 'SHA256:e2e',
  sharedScope: 'private',
  teamIds: [],
  isGlobal: false,
  createdBy: me.user.id,
  lastUsedAt: null,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z'
}

async function mockTenantApi(page: Page) {
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(me) }))
  await page.route(/\/api\/v1\/t\/acme\/jobs(?:\?.*)?$/, async (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [job], page: 1, pageSize: 20, total: 1 }) }))
  await page.route('**/api/v1/t/acme/repositories**', async (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [], page: 1, pageSize: 5, total: 0 }) }))
  await page.route('**/api/v1/t/acme/credentials**', async (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [], page: 1, pageSize: 5, total: 0 }) }))
  await page.route('**/api/v1/t/acme/jobs/*', async (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(job) }))
  await page.route('**/api/v1/t/acme/jobs/*/logs', async (route) => route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'event: state\ndata: {"event":"state","id":"0","status":"running","progress":20,"at":"2026-01-01T00:00:00Z"}\n\n' }))
}

async function mockWriteApi(page: Page, requests: { repository?: unknown; credential?: unknown }) {
  let repositoryCreated = false
  let credentialCreated = false
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(me) }))
  await page.route('**/api/v1/t/acme/repositories**', async (route) => {
    if (route.request().method() === 'POST') {
      requests.repository = route.request().postDataJSON()
      repositoryCreated = true
      return route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify(repository) })
    }
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: repositoryCreated ? [repository] : [], page: 1, pageSize: 20, total: repositoryCreated ? 1 : 0 }) })
  })
  await page.route('**/api/v1/t/acme/credentials**', async (route) => {
    if (route.request().method() === 'POST') {
      requests.credential = route.request().postDataJSON()
      credentialCreated = true
      return route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify(credential) })
    }
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: credentialCreated ? [credential] : [], page: 1, pageSize: 20, total: credentialCreated ? 1 : 0 }) })
  })
}

async function mockResourceApi(page: Page, requests: Record<string, unknown>) {
  let repositoryPresent = true
  let credentialPresent = true
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(me) }))
  await page.route('**/api/v1/t/acme/repositories**', async (route) => {
    const method = route.request().method()
    if (method === 'PATCH') {
      requests.repositoryPatch = route.request().postDataJSON()
      requests.repositoryIfMatch = route.request().headers()['if-match']
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(repository) })
    }
    if (method === 'DELETE') {
      requests.repositoryDeleteIfMatch = route.request().headers()['if-match']
      repositoryPresent = false
      return route.fulfill({ status: 204 })
    }
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: repositoryPresent ? [repository] : [], page: 1, pageSize: 20, total: repositoryPresent ? 1 : 0 }) })
  })
  await page.route('**/api/v1/t/acme/credentials**', async (route) => {
    const method = route.request().method()
    if (method === 'PATCH') {
      requests.credentialPatch = route.request().postDataJSON()
      requests.credentialIfMatch = route.request().headers()['if-match']
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(credential) })
    }
    if (method === 'DELETE') {
      requests.credentialDeleteIfMatch = route.request().headers()['if-match']
      credentialPresent = false
      return route.fulfill({ status: 204 })
    }
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: credentialPresent ? [credential] : [], page: 1, pageSize: 20, total: credentialPresent ? 1 : 0 }) })
  })
}

async function mockPlatformApi(page: Page) {
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(me) }))
  await page.route('**/api/v1/admin/users**', async (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [me.user], page: 1, pageSize: 20, total: 1 }) }))
  await page.route('**/api/v1/admin/tenants**', async (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [{ id: tenant.id, etag: 'v1-tenant', slug: tenant.slug, displayName: tenant.displayName, status: 'active', quota: { maxRepositories: 10, maxServices: 20, maxStorageBytes: 1000000, maxCollectConcurrency: 2 }, createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' }], page: 1, pageSize: 20, total: 1 }) }))
  await page.route('**/api/v1/admin/global-credentials**', async (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [{ id: credential.id, etag: credential.etag, name: 'platform-deploy', kind: credential.kind, fingerprint: credential.fingerprint, createdBy: me.user.id, lastUsedAt: null, revision: 1, createdAt: '2026-01-01T00:00:00Z', updatedAt: '2026-01-01T00:00:00Z' }], page: 1, pageSize: 20, total: 1 }) }))
  await page.route('**/api/v1/admin/jobs**', async (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [{ id: job.id, tenantSlug: job.tenantSlug, type: job.type, trigger: job.trigger, status: job.status, stage: job.stage, scopeType: job.scopeType, scopeId: job.scopeId, createdAt: job.createdAt, startedAt: job.startedAt, finishedAt: job.finishedAt }], page: 1, pageSize: 20, total: 1 }) }))
  await page.route('**/api/v1/admin/audit-logs**', async (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [{ id: '01999c55-8d0c-7c1e-8c2d-000000000050', tenantSlug: tenant.slug, actorId: me.user.id, action: 'repository.created', resourceType: 'repository', resourceId: repository.id, requestId: 'e2e-request', metadata: { source: 'e2e' }, createdAt: '2026-01-01T00:00:00Z' }], page: 1, pageSize: 20, total: 1 }) }))
}

test('login screen is keyboard reachable and accessible', async ({ page }) => {
  test.setTimeout(60_000)
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ code: 'unauthenticated', message: 'sign in', requestId: 'e2e' }) }))
  await page.goto('/login')
  await expect(page.getByRole('heading', { name: '登录控制面' })).toBeVisible({ timeout: 15_000 })
  await expect(page.getByRole('textbox', { name: '用户名' })).toBeFocused()
  const results = await new AxeBuilder({ page }).analyze()
  expect(results.violations).toEqual([])
})

test('tenant dashboard renders on desktop with isolated tenant navigation', async ({ page }) => {
  test.setTimeout(60_000)
  await mockTenantApi(page)
  await page.goto('/t/acme/dashboard')
  await expect(page.getByRole('heading', { name: 'acme', exact: true })).toBeVisible({ timeout: 15_000 })
  await expect(page.getByRole('navigation', { name: '主导航' })).toContainText('仓库')
  await expect(page.getByText('进行中任务')).toBeVisible()
  const results = await new AxeBuilder({ page }).analyze()
  expect(results.violations).toEqual([])
})

test('platform admin dashboard renders redacted operations projections', async ({ page }) => {
  test.setTimeout(60_000)
  await mockPlatformApi(page)
  await page.goto('/admin')
  await expect(page.getByRole('heading', { name: '平台概览' })).toBeVisible({ timeout: 15_000 })
  await expect(page.getByRole('navigation', { name: '平台导航' })).toContainText('全局凭据')
  await expect(page.getByRole('main').getByText('Platform Operator')).toBeVisible()
  await expect(page.getByText('platform-deploy')).toBeVisible()
  await expect(page.getByText('repository.created')).toBeVisible()
  await expect(page.getByText('secret-material')).toHaveCount(0)
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([])
})

test('tenant member is redirected away from platform operations', async ({ page }) => {
  test.setTimeout(60_000)
  const tenantMember = { ...me, isPlatformAdmin: false }
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(tenantMember) }))
  await page.route('**/api/v1/t/acme/jobs**', async (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [], page: 1, pageSize: 20, total: 0 }) }))
  await page.goto('/admin')
  await expect(page).toHaveURL(/\/t\/acme\/dashboard$/)
  await expect(page.getByRole('heading', { name: 'Acme Platform' })).toBeVisible({ timeout: 15_000 })
})

test('mobile navigation opens without covering the page controls', async ({ page }) => {
  test.skip((page.viewportSize()?.width ?? 0) >= 600, 'navigation drawer is covered by the permanent desktop sidebar')
  test.setTimeout(60_000)
  await mockTenantApi(page)
  await page.goto('/t/acme/jobs')
  await expect(page.getByRole('heading', { name: '任务' })).toBeVisible({ timeout: 15_000 })
  await page.getByRole('button', { name: '打开导航' }).click()
  await expect(page.getByRole('navigation', { name: '主导航' })).toBeVisible()
  await expect(page.getByRole('link', { name: '凭据' })).toBeVisible()
  const results = await new AxeBuilder({ page }).analyze()
  expect(results.violations).toEqual([])
})

test('repository creation follows the contract and refreshes the tenant list', async ({ page }) => {
  test.setTimeout(60_000)
  const requests: { repository?: unknown } = {}
  await mockWriteApi(page, requests)
  await page.goto('/t/acme/repos')
  await expect(page.getByRole('heading', { name: '仓库' })).toBeVisible({ timeout: 15_000 })
  await page.getByRole('button', { name: '添加仓库' }).click()
  await expect(page.getByRole('heading', { name: '添加仓库' })).toBeVisible()
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([])
  await page.getByRole('textbox', { name: 'Git 远程地址' }).fill('https://git.example.com/team/service.git')
  await page.getByRole('textbox', { name: '默认分支' }).fill('main')
  await page.getByRole('button', { name: '创建仓库' }).click()
  await expect(page.getByRole('heading', { name: '添加仓库' })).toBeHidden()
  expect(requests.repository).toMatchObject({ url: 'https://git.example.com/team/service.git', defaultBranch: 'main', credentialId: null, note: null })
})

test('credential creation keeps secret fields write-only in the UI', async ({ page }) => {
  test.setTimeout(60_000)
  const requests: { credential?: unknown } = {}
  await mockWriteApi(page, requests)
  await page.goto('/t/acme/credentials')
  await expect(page.getByRole('heading', { name: '凭据' })).toBeVisible({ timeout: 15_000 })
  await page.getByRole('button', { name: '创建凭据' }).click()
  await expect(page.getByRole('heading', { name: '创建凭据' })).toBeVisible()
  expect((await new AxeBuilder({ page }).analyze()).violations).toEqual([])
  await page.getByRole('textbox', { name: '凭据名称' }).fill('deploy-key')
  await page.getByRole('textbox', { name: 'SSH 私钥' }).fill('-----BEGIN OPENSSH PRIVATE KEY-----\ne2e-key-material-that-is-long-enough\n-----END OPENSSH PRIVATE KEY-----')
  await page.getByRole('button', { name: '创建凭据' }).click()
  await expect(page.getByRole('heading', { name: '创建凭据' })).toBeHidden()
  expect(requests.credential).toMatchObject({ name: 'deploy-key', kind: 'ssh_key', sharedScope: 'private' })
  expect((requests.credential as { sshKey: { privateKeyPem: string } }).sshKey.privateKeyPem).toContain('BEGIN OPENSSH')
  await expect(page.getByText('deploy-key')).toBeVisible()
  await expect(page.getByText('BEGIN OPENSSH')).toHaveCount(0)
})

test('resource controls use ETag guarded edit and delete requests', async ({ page }) => {
  test.setTimeout(60_000)
  const requests: Record<string, unknown> = {}
  await mockResourceApi(page, requests)
  await page.goto('/t/acme/repos')
  await expect(page.getByText(repository.url)).toBeVisible({ timeout: 15_000 })
  await page.getByRole('button', { name: '编辑仓库' }).click()
  await expect(page.getByRole('heading', { name: '编辑仓库' })).toBeVisible()
  await page.getByRole('textbox', { name: '默认分支' }).fill('develop')
  await page.getByRole('button', { name: '保存修改' }).click()
  await expect(page.getByRole('heading', { name: '编辑仓库' })).toBeHidden()
  expect(requests.repositoryPatch).toMatchObject({ defaultBranch: 'develop', credentialId: null, note: null })
  expect(requests.repositoryIfMatch).toBe(repository.etag)
  await page.getByRole('button', { name: '删除仓库' }).click()
  await expect(page.getByRole('heading', { name: '删除仓库' })).toBeVisible()
  await page.getByRole('button', { name: '确认删除' }).click()
  await expect(page.getByText(repository.url)).toBeHidden()
  expect(requests.repositoryDeleteIfMatch).toBe(repository.etag)
})

test('credential edits stay secret-free and deletion preserves If-Match', async ({ page }) => {
  test.setTimeout(60_000)
  const requests: Record<string, unknown> = {}
  await mockResourceApi(page, requests)
  await page.goto('/t/acme/credentials')
  await expect(page.getByText(credential.name)).toBeVisible({ timeout: 15_000 })
  await page.getByRole('button', { name: '编辑凭据' }).click()
  await expect(page.getByRole('heading', { name: '编辑凭据' })).toBeVisible()
  await page.getByRole('textbox', { name: '凭据名称' }).fill('deploy-key-renamed')
  await page.getByRole('button', { name: '保存修改' }).click()
  await expect(page.getByRole('heading', { name: '编辑凭据' })).toBeHidden()
  expect(requests.credentialPatch).toEqual({ name: 'deploy-key-renamed', sharedScope: 'private', teamIds: [] })
  expect(JSON.stringify(requests.credentialPatch)).not.toContain('privateKey')
  expect(requests.credentialIfMatch).toBe(credential.etag)
  await page.getByRole('button', { name: '删除凭据' }).click()
  await expect(page.getByRole('heading', { name: '删除凭据' })).toBeVisible()
  await page.getByRole('button', { name: '确认删除' }).click()
  await expect(page.getByText(credential.name, { exact: true })).toBeHidden()
  expect(requests.credentialDeleteIfMatch).toBe(credential.etag)
})
