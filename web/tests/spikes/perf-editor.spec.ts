import { expect, test } from '@playwright/test'
import { writeSpikeReport } from './report'

test('CodeMirror enforces editable and read-only large document modes', async ({ page, browser }, testInfo) => {
  await page.goto('/?mode=editor')
  await page.waitForFunction(() => window.__spike?.editor?.ready)
  await page.evaluate(() => window.__spike.editor.interact())
  await expect(page.getByRole('status')).toContainText('预览已更新', { timeout: 3000 })
  const measurements = await page.evaluate(() => ({
    sizes: window.__spike.editor.sizes,
    editable: window.__spike.editor.editable,
    previewCount: window.__spike.editor.previewCount,
    longTasks: window.__spike.editor.longTasks,
    repeatedLongTasks: window.__spike.editor.longTasks.filter((duration: number) => duration > 50).length
  }))
  const assertions = [
    { id: 'fixture-byte-sizes-exact', status: JSON.stringify(measurements.sizes) === JSON.stringify([1048576, 5242880, 10485760]) ? 'passed' : 'failed', actual: measurements.sizes },
    { id: 'one-mib-editable-five-ten-readonly', status: JSON.stringify(measurements.editable) === JSON.stringify([true, false, false]) ? 'passed' : 'failed', actual: measurements.editable },
    { id: 'preview-is-debounced-once', status: measurements.previewCount === 1 ? 'passed' : 'failed', actual: measurements.previewCount, threshold: 1 },
    { id: 'no-repeated-long-task-over-50ms', status: measurements.repeatedLongTasks <= 1 ? 'passed' : 'failed', actual: measurements.repeatedLongTasks, threshold: '<=1' }
  ] as const
  await writeSpikeReport(testInfo, browser, { measurements, thresholds: { maxRepeatedLongTasksOver50Ms: 1 }, assertions: [...assertions] })
  expect(assertions.every(item => item.status === 'passed')).toBeTruthy()
})

declare global { interface Window { __spike: any } }
