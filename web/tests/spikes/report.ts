import { createHash } from 'node:crypto'
import { mkdirSync, readFileSync, readdirSync, renameSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { execFileSync } from 'node:child_process'
import type { Browser, TestInfo } from '@playwright/test'

type Assertion = { id: string; status: 'passed' | 'failed'; actual?: unknown; threshold?: unknown }

function sourceDigest() {
  const files = execFileSync('git', [
    'ls-files', '-z', '--cached', '--others', '--exclude-standard', '--',
    'Makefile', 'web/package.json', 'web/pnpm-lock.yaml', 'web/scripts/run-spike.mjs',
    'web/scripts/serve-spike-harness.mjs', 'web/tests/spikes'
  ], { cwd: '..' }).toString().split('\0').filter(Boolean).sort()
  const hash = createHash('sha256')
  for (const file of files) hash.update(`${file}\0`).update(readFileSync(join('..', file))).update('\0')
  return `sha256:${hash.digest('hex')}`
}

export async function writeSpikeReport(testInfo: TestInfo, browser: Browser, payload: {
  measurements: Record<string, unknown>
  thresholds: Record<string, unknown>
  assertions: Assertion[]
}) {
  const path = process.env.SPIKE_REPORT_PATH
  if (!path) throw new Error('SPIKE_REPORT_PATH is required')
  const hash = createHash('sha256')
  for (const name of readdirSync('../contracts').filter(name => name.endsWith('.yaml') && name !== 'work-items.yaml').sort()) {
    hash.update(`${name}\0`).update(readFileSync(join('../contracts', name))).update('\0')
  }
  const failed = payload.assertions.some(assertion => assertion.status !== 'passed')
  let report = {
    id: process.env.SPIKE_ID,
    status: failed ? 'failed' : 'passed',
    commit: execFileSync('git', ['rev-parse', 'HEAD'], { cwd: '..', encoding: 'utf8' }).trim(),
    sourceDigest: sourceDigest(),
    contractDigest: `sha256:${hash.digest('hex')}`,
    browser: 'chromium',
    browserVersion: browser.version(),
    project: testInfo.project.name,
    viewport: testInfo.project.use.viewport,
    fixture: 'web/tests/spikes/harness',
    thresholds: payload.thresholds,
    measurements: payload.measurements,
    assertions: payload.assertions,
    tests: { run: 1, passed: failed ? 0 : 1, failed: failed ? 1 : 0, skipped: 0 }
  }
  if (process.env.SPIKE_ID === 'a11y-m0') {
    try {
      const previous = JSON.parse(readFileSync(path, 'utf8'))
      if (previous.status === 'passed') {
        report = {
          ...report,
          project: 'desktop+mobile',
          viewport: [previous.viewport, report.viewport] as any,
          measurements: { [previous.project]: previous.measurements, [testInfo.project.name]: report.measurements },
          assertions: [...previous.assertions, ...report.assertions],
          tests: { run: previous.tests.run + 1, passed: previous.tests.passed + (failed ? 0 : 1), failed: previous.tests.failed + (failed ? 1 : 0), skipped: 0 }
        }
      }
    } catch {
      // The runner's initial failure sentinel is intentionally replaced by the first real result.
    }
  }
  mkdirSync(dirname(path), { recursive: true })
  const temporary = `${path}.${process.pid}.tmp`
  writeFileSync(temporary, `${JSON.stringify(report, null, 2)}\n`)
  renameSync(temporary, path)
}
