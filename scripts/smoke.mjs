import { createHash } from 'node:crypto'
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { spawnSync } from 'node:child_process'
import { assessGoTest, digestSourceFiles, goTestSelector, smokeFixtureStatus, writeSmokeReport } from './write-smoke-report.mjs'

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
let failed = false
for (const id of ids) {
  const fixture = catalog[id]
  const fixtureFailure = smokeFixtureStatus(fixture, Boolean(fixture?.fixture && existsSync(fixture.fixture)))
  let events = []
  let execution = { status: 125 }
  let command = null
  let parseError = false
  let logPath = null
  const caseStart = Date.now()
  if (!fixtureFailure) {
    const args = ['scripts/test-integration.sh', '-json', '-run', goTestSelector(fixture.test), fixture.package]
    command = ['sh', ...args].map(arg => `'${arg.replaceAll("'", "'\\''")}'`).join(' ')
    execution = spawnSync('sh', args, { encoding: 'utf8', timeout: 900_000, maxBuffer: 64 * 1024 * 1024 })
    logPath = join(runDirectory, `${id}.log`)
    writeFileSync(logPath, `$ ${command}\n${execution.stdout || ''}\n${execution.stderr || ''}\n${execution.error || ''}`)
    try {
      events = (execution.stdout || '').split('\n').filter(line => line.startsWith('{')).map(line => JSON.parse(line))
    } catch {
      parseError = true
    }
  }
  const durationSeconds = (Date.now() - caseStart) / 1000
  const sourceChanged = testedSourceDigest !== sourceDigest()
  const assessment = sourceChanged ? { result: 'fail', failureKind: 'code_failure', reason: 'source changed during verification; rerun on a stable tree' }
    : fixtureFailure
      ?? (parseError ? { result: 'fail', failureKind: 'tooling_gap', reason: 'fixture produced an invalid Go JSON test log' }
        : assessGoTest(events, fixture, execution.status))
  const payload = {
    smokeId: id, ...assessment, assertions: [{ id, status: assessment.result === 'pass' ? 'passed' : 'failed' }],
    fixture: fixture.fixture || null, test: fixture.test || null, package: fixture.package || null,
    coverage: assessment.result === 'pass' ? 'required-fixture-executed' : 'incomplete',
    command,
    exitCode: assessment.result === 'pass' ? 0 : execution.status ?? 125,
    startedAt, durationSeconds, commit, contractDigest, sourceDigest: testedSourceDigest,
    workingTreeDirty: gitOutput(['status', '--porcelain', '--untracked-files=normal']).trim() !== '',
    logPath
  }
  const reportPath = join(runDirectory, `${id}.json`)
  writeSmokeReport(reportPath, payload)
  writeSmokeReport(join(artifactRoot, `${id}.json`), { ...payload, reportPath })
  console.log(`${id}: ${assessment.result}${assessment.failureKind ? ` (${assessment.failureKind})` : ''}; ${reportPath}`)
  if (assessment.reason) console.log(`  ${assessment.reason}`)
  failed ||= assessment.result !== 'pass'
}
process.exitCode = failed ? 1 : 0
