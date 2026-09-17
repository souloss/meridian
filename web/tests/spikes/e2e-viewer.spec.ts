import { expect, test } from '@playwright/test'
import { writeSpikeReport } from './report'

test('viewer renders a resolved OpenAPI document', async ({ page, browser }, testInfo) => {
  await page.goto('/?mode=viewer')
  await page.waitForFunction(() => window.__spike?.viewer?.ready)
  await expect(page.getByTestId('viewer-status')).toContainText('已加载')
  const documentText = await page.getByTestId('viewer-document').textContent()
  const renderedBytes = (await page.getByTestId('viewer-document').textContent())?.length ?? 0
  const measurements = await page.evaluate((bytes) => ({
    ...window.__spike.viewer,
    renderedBytes: bytes
  }), renderedBytes)
  const assertions = [
    { id: 'viewer-ready', status: measurements.ready ? 'passed' : 'failed', actual: measurements.ready, threshold: true },
    { id: 'document-rendered', status: measurements.rendered ? 'passed' : 'failed', actual: measurements.rendered, threshold: true },
    { id: 'document-non-empty', status: measurements.renderedBytes > 0 ? 'passed' : 'failed', actual: measurements.renderedBytes, threshold: '>0' },
    { id: 'document-is-openapi', status: (documentText ?? '').includes('openapi: 3.1.0') ? 'passed' : 'failed', actual: 'contains openapi header', threshold: 'openapi: 3.1.0' }
  ] as const
  await writeSpikeReport(testInfo, browser, {
    measurements,
    thresholds: { minRenderedBytes: 1 },
    assertions: [...assertions]
  })
  expect(assertions.every(item => item.status === 'passed')).toBeTruthy()
})

declare global { interface Window { __spike: any } }
