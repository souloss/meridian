import { createHash } from 'node:crypto'
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { spawnSync } from 'node:child_process'
import { assessGoTest, digestSourceFiles, writeSmokeReport } from './write-smoke-report.mjs'

const catalog = JSON.parse(readFileSync(new URL('./smoke-cases.json', import.meta.url), 'utf8'))
const mode = process.argv[2] || process.env.SMK
const milestone = process.env.MILESTONE || 'M5'
if (!/^M[0-5]$/.test(milestone)) throw new Error('MILESTONE must be M0 through M5')
const ids = mode === '--credentials' ? ['SMK-005', 'SMK-031', 'SMK-035']
  : mode === '--all' ? Object.keys(catalog).filter(id => catalog[id].milestone <= milestone)
  : catalog[mode] ? [mode] : []
if (ids.length === 0) {
  console.error('usage: make smoke SMK=SMK-005 | make smoke-all MILESTONE=M0 | make smoke-m0-credentials')
  process.exit(2)
}

const startedAt = new Date().toISOString()
const artifactRoot = process.env.SMOKE_ARTIFACT_DIR || 'artifacts/smoke'
mkdirSync(artifactRoot, { recursive: true })
const runDirectory = mkdtempSync(join(artifactRoot, startedAt.replace(/[:.]/g, '-')))
const hash = createHash('sha256')
for (const name of readdirSync('contracts').filter(name => name.endsWith('.yaml') && name !== 'work-items.yaml').sort()) {
  hash.update(`${name}\0`).update(readFileSync(join('contracts', name))).update('\0')
}
const contractDigest = `sha256:${hash.digest('hex')}`
function gitOutput(args) {
  const result = spawnSync('git', args, { encoding: 'utf8' })
  if (result.status !== 0) throw new Error(`git ${args.join(' ')} failed`)
  return result.stdout
}
const commit = gitOutput(['rev-parse', 'HEAD']).trim()
function sourceDigest() {
  const paths = gitOutput(['ls-files', '--cached', '--others', '--exclude-standard', '-z']).split('\0')
  return digestSourceFiles(paths.filter(path => path !== 'contracts/work-items.yaml'
    && (/^(cmd|internal|migrations|scripts|contracts|web)\//.test(path)
      || /^(Makefile|go\.(mod|sum)|docker-compose\.yml)$/.test(path))))
}
const testedSourceDigest = sourceDigest()
const available = ids.filter(id => catalog[id].test && existsSync(catalog[id].fixture))
let events = []
let execution = { status: 0 }
let command = null
let parseError = false
const logPath = join(runDirectory, 'commands.log')
const start = Date.now()
if (available.length) {
  // All implemented cases currently share one isolated PostgreSQL/HTTP fixture.
  const selector = `^TestM0CredentialSmoke$/^(${available.join('|')})$`
  const args = ['scripts/test-integration.sh', '-json', '-run', selector, './internal/handler']
  command = ['sh', ...args].map(arg => `'${arg.replaceAll("'", "'\\''")}'`).join(' ')
  execution = spawnSync('sh', args, { encoding: 'utf8', timeout: 900_000, maxBuffer: 64 * 1024 * 1024 })
  writeFileSync(logPath, `$ ${command}\n${execution.stdout || ''}\n${execution.stderr || ''}\n${execution.error || ''}`)
  try {
    events = (execution.stdout || '').split('\n').filter(line => line.startsWith('{')).map(line => JSON.parse(line))
  } catch {
    parseError = true
  }
}
const durationSeconds = (Date.now() - start) / 1000
const sourceChanged = testedSourceDigest !== sourceDigest()
let failed = false
for (const id of ids) {
  const assessment = sourceChanged ? { result: 'fail', failureKind: 'code_failure', reason: 'source changed during verification; rerun on a stable tree' }
    : parseError ? { result: 'fail', failureKind: 'tooling_gap', reason: 'fixture produced an invalid Go JSON test log' }
    : available.includes(id) ? assessGoTest(events, catalog[id].test, execution.status)
    : { result: 'fail', failureKind: 'tooling_gap', reason: 'implement the required fixture in the owning work item before verification' }
  const payload = {
    smokeId: id, ...assessment, assertions: [{ id, status: assessment.result === 'pass' ? 'passed' : 'failed' }],
    fixture: catalog[id].fixture || null, test: catalog[id].test || null,
    coverage: assessment.result === 'pass' ? 'required-fixture-executed' : 'incomplete',
    command: available.includes(id) ? command : null,
    exitCode: assessment.result === 'pass' ? 0 : execution.status || 125,
    startedAt, durationSeconds, commit, contractDigest, sourceDigest: testedSourceDigest,
    workingTreeDirty: gitOutput(['status', '--porcelain', '--untracked-files=normal']).trim() !== '',
    logPath: available.includes(id) ? logPath : null
  }
  const reportPath = join(runDirectory, `${id}.json`)
  writeSmokeReport(reportPath, payload)
  writeSmokeReport(join(artifactRoot, `${id}.json`), { ...payload, reportPath })
  console.log(`${id}: ${assessment.result}${assessment.failureKind ? ` (${assessment.failureKind})` : ''}; ${reportPath}`)
  if (assessment.reason) console.log(`  ${assessment.reason}`)
  failed ||= assessment.result !== 'pass'
}
process.exitCode = failed ? 1 : 0
