import { createHash } from 'node:crypto'
import { mkdirSync, readFileSync, readdirSync, renameSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { spawnSync } from 'node:child_process'

const id = process.argv[2]
const file = process.argv[3]
const project = process.argv[4]
if (!id || !file || !project) throw new Error('usage: run-spike <id> <spec-file> <project>')
const artifact = join('..', 'artifacts', 'spikes', `${id}.json`)
mkdirSync(join('..', 'artifacts', 'spikes'), { recursive: true })
const hash = createHash('sha256')
for (const name of readdirSync('../contracts').filter(name => name.endsWith('.yaml') && name !== 'work-items.yaml').sort()) {
  hash.update(`${name}\0`).update(readFileSync(join('../contracts', name))).update('\0')
}
const initial = {
  id,
  status: 'failed',
  commit: git(['rev-parse', 'HEAD']).trim(),
  sourceDigest: sourceDigest(),
  contractDigest: `sha256:${hash.digest('hex')}`,
  browser: 'chromium',
  project,
  fixture: 'web/tests/spikes/harness',
  tests: { run: 0, passed: 0, failed: 1, skipped: 0 },
  assertions: [{ id: 'process-completed', status: 'failed' }],
  reason: 'Playwright process did not produce a passing report'
}
atomicWrite(artifact, initial)
const args = ['exec', 'playwright', 'test', file, '--config', 'tests/spikes/playwright.config.ts']
if (project !== 'all') args.push('--project', project)
const result = spawnSync('pnpm', args, {
  stdio: 'inherit',
  encoding: 'utf8',
  env: { ...process.env, SPIKE_ID: id, SPIKE_REPORT_PATH: artifact }
})
if (result.status !== 0) process.exit(result.status ?? 1)
const report = JSON.parse(readFileSync(artifact, 'utf8'))
const requiredRuns = project === 'all' ? 2 : 1
if (report.status !== 'passed' || report.tests.run !== requiredRuns || report.tests.failed !== 0 || report.tests.skipped !== 0 || report.assertions.some(item => item.status !== 'passed')) {
  throw new Error(`${id} report failed validation`)
}

function git(args) {
  const result = spawnSync('git', args, { cwd: '..', encoding: 'utf8' })
  if (result.status !== 0) throw new Error(`git ${args.join(' ')} failed`)
  return result.stdout
}

function sourceDigest() {
  const files = git([
    'ls-files', '-z', '--cached', '--others', '--exclude-standard', '--',
    'Makefile', 'web/package.json', 'web/pnpm-lock.yaml', 'web/scripts/run-spike.mjs',
    'web/scripts/serve-spike-harness.mjs', 'web/tests/spikes'
  ]).split('\0').filter(Boolean).sort()
  const sourceHash = createHash('sha256')
  for (const file of files) sourceHash.update(`${file}\0`).update(readFileSync(join('..', file))).update('\0')
  return `sha256:${sourceHash.digest('hex')}`
}

function atomicWrite(path, payload) {
  const temporary = `${path}.${process.pid}.tmp`
  writeFileSync(temporary, `${JSON.stringify(payload, null, 2)}\n`)
  renameSync(temporary, path)
}
