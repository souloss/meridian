import { AxeBuilder } from '@axe-core/playwright'
import { expect, test } from '@playwright/test'
import { writeSpikeReport } from './report'

test('M0 spike surfaces are keyboard accessible without viewport overflow', async ({ page, browser }, testInfo) => {
  await page.goto('/?mode=a11y')
  await page.waitForFunction(() => window.__spike?.table?.ready && window.__spike?.graph?.ready && window.__spike?.editor?.ready)
  const axe = await new AxeBuilder({ page }).analyze()
  const measurements = await page.evaluate(() => {
    const viewportWidth = document.documentElement.clientWidth
    const overflow = document.documentElement.scrollWidth - viewportWidth
    const outOfBounds = [...document.querySelectorAll('button,input,[tabindex="0"]')].filter(element => {
      if (element.closest('[data-testid="table-scroll"]')) return false
      const box = element.getBoundingClientRect()
      return box.left < 0 || box.right > viewportWidth
    }).length
    return { axeViolations: axePlaceholder(), axeRules: [] as string[], axeFindings: [] as unknown[], overflow, outOfBounds }
    function axePlaceholder() { return 0 }
  })
  measurements.axeViolations = axe.violations.length
  measurements.axeRules = axe.violations.map(violation => violation.id)
  measurements.axeFindings = axe.violations.map(violation => ({
    id: violation.id,
    targets: violation.nodes.map(node => node.target)
  }))
  await page.getByRole('textbox', { name: '用户名' }).focus()
  await page.keyboard.press('Tab')
  await expect(page.getByLabel('密码')).toBeFocused()
  const separator = page.getByRole('separator')
  await separator.focus()
  await page.keyboard.press('ArrowRight')
  const graph = page.getByRole('img', { name: /500 个服务节点/ })
  await graph.focus()
  await expect(graph).toBeFocused()
  const assertions = [
    { id: 'axe-has-no-violations', status: measurements.axeViolations === 0 ? 'passed' : 'failed', actual: measurements.axeViolations, threshold: 0 },
    { id: 'page-has-no-horizontal-overflow', status: measurements.overflow <= 0 ? 'passed' : 'failed', actual: measurements.overflow, threshold: '<=0' },
    { id: 'controls-stay-inside-viewport', status: measurements.outOfBounds === 0 ? 'passed' : 'failed', actual: measurements.outOfBounds, threshold: 0 },
    { id: 'keyboard-path-completes', status: 'passed' }
  ] as const
  await writeSpikeReport(testInfo, browser, { measurements, thresholds: { axeViolations: 0, horizontalOverflowPx: 0 }, assertions: [...assertions] })
  expect(assertions.every(item => item.status === 'passed')).toBeTruthy()
})

declare global { interface Window { __spike: any } }
