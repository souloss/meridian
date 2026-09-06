import { expect, test } from '@playwright/test'
import { writeSpikeReport } from './report'

test('virtual table and dependency graph meet capacity and interaction gates', async ({ page, browser }, testInfo) => {
  await page.goto('/?mode=table-graph')
  await page.waitForFunction(() => window.__spike?.table?.ready && window.__spike?.graph?.ready)
  const measurements = await page.evaluate(() => {
    const frame = document.querySelector('[data-testid="table-scroll"]')!
    const rows = frame.querySelectorAll('tbody tr').length
    let paintedPixels = 0
    for (const canvas of document.querySelectorAll<HTMLCanvasElement>('.graph canvas')) {
      const context = canvas.getContext('2d', { willReadFrequently: true })
      if (!context) continue
      const data = context.getImageData(0, 0, canvas.width, canvas.height).data
      for (let index = 3; index < data.length; index += 4) {
        if (data[index] > 0 && (data[index - 1] < 245 || data[index - 2] < 245 || data[index - 3] < 245)) paintedPixels += 1
      }
    }
    return { ...window.__spike, renderedTableRows: rows, paintedPixels }
  })
  const frame = page.getByTestId('table-scroll')
  await frame.evaluate(element => { element.scrollTop = element.scrollHeight })
  await expect(page.getByText('service-10000')).toBeVisible()
  const action = page.getByRole('button', { name: /打开 service-10000/ })
  await action.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByRole('status')).toContainText('service-10000')
  const assertions = [
    { id: 'table-model-has-10000-rows', status: measurements.table.rows === 10_000 ? 'passed' : 'failed', actual: measurements.table.rows, threshold: 10_000 },
    { id: 'table-dom-is-virtualized', status: measurements.renderedTableRows <= 50 ? 'passed' : 'failed', actual: measurements.renderedTableRows, threshold: '<=50' },
    { id: 'graph-has-500-nodes', status: measurements.graph.nodes === 500 ? 'passed' : 'failed', actual: measurements.graph.nodes, threshold: 500 },
    { id: 'graph-has-5000-edges', status: measurements.graph.edges === 5000 ? 'passed' : 'failed', actual: measurements.graph.edges, threshold: 5000 },
    { id: 'graph-first-paint-under-2s', status: measurements.graph.firstPaintMs < 2000 ? 'passed' : 'failed', actual: measurements.graph.firstPaintMs, threshold: '<2000ms' },
    { id: 'graph-canvas-is-painted', status: measurements.paintedPixels > 100 ? 'passed' : 'failed', actual: measurements.paintedPixels, threshold: '>100' },
    { id: 'graph-selection-and-neighborhood-work', status: measurements.graph.selected && measurements.graph.neighborhood > 0 ? 'passed' : 'failed', actual: { selected: measurements.graph.selected, neighborhood: measurements.graph.neighborhood } }
  ] as const
  await writeSpikeReport(testInfo, browser, { measurements, thresholds: { graphFirstPaintMs: 2000, maxRenderedRows: 50 }, assertions: [...assertions] })
  expect(assertions.every(item => item.status === 'passed')).toBeTruthy()
})

declare global { interface Window { __spike: any } }
