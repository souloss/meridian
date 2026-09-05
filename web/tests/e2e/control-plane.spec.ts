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

async function mockTenantApi(page: Page) {
  await page.route('**/api/v1/auth/me', async (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(me) }))
  await page.route(/\/api\/v1\/t\/acme\/jobs(?:\?.*)?$/, async (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [job], page: 1, pageSize: 20, total: 1 }) }))
  await page.route('**/api/v1/t/acme/repositories**', async (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [], page: 1, pageSize: 5, total: 0 }) }))
  await page.route('**/api/v1/t/acme/credentials**', async (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ items: [], page: 1, pageSize: 5, total: 0 }) }))
  await page.route('**/api/v1/t/acme/jobs/*', async (route) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(job) }))
  await page.route('**/api/v1/t/acme/jobs/*/logs', async (route) => route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'event: state\ndata: {"event":"state","id":"0","status":"running","progress":20,"at":"2026-01-01T00:00:00Z"}\n\n' }))
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
