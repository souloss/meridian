import { createHash } from 'node:crypto'
import { lstatSync, mkdirSync, readFileSync, readlinkSync, writeFileSync } from 'node:fs'
import { dirname } from 'node:path'

export function digestSourceFiles(paths) {
  const hash = createHash('sha256')
  for (const path of [...new Set(paths)].sort()) {
    const entry = lstatSync(path, { throwIfNoEntry: false })
    const content = !entry ? '<deleted>' : entry.isSymbolicLink() ? readlinkSync(path) : readFileSync(path)
    hash.update(`${path}\0`).update(content).update('\0')
  }
  return `sha256:${hash.digest('hex')}`
}

export function assessGoTest(events, fixture, exitCode) {
  const relevant = events.filter(event => event.Package === fixture.package
    && (event.Test === fixture.test || event.Test?.startsWith(`${fixture.test}/`)))
  if (exitCode !== 0) return { result: 'fail', failureKind: 'code_failure', reason: 'fixture command failed' }
  if (relevant.some(event => event.Action === 'skip')) {
    return { result: 'fail', failureKind: 'tooling_gap', reason: 'required test was skipped' }
  }
  if (relevant.some(event => event.Action === 'fail')) {
    return { result: 'fail', failureKind: 'code_failure', reason: 'required assertion failed' }
  }
  if (!relevant.some(event => event.Test === fixture.test && event.Action === 'pass')) {
    return { result: 'fail', failureKind: 'tooling_gap', reason: 'required test did not run' }
  }
  return { result: 'pass', failureKind: null, reason: null }
}

export function smokeFixtureStatus(fixture, fixtureExists) {
  if (!fixture || typeof fixture !== 'object' || !fixture.fixture || !fixture.test || !fixture.package) {
    return { result: 'fail', failureKind: 'tooling_gap', reason: 'catalog entry has no executable fixture, test, and package mapping' }
  }
  if (!fixtureExists) {
    return { result: 'fail', failureKind: 'tooling_gap', reason: `catalog fixture does not exist: ${fixture.fixture}` }
  }
  return null
}

export function goTestSelector(test) {
  const components = test.split('/')
  if (components.some(component => !component || !/^[A-Za-z0-9_-]+$/.test(component))) {
    throw new Error(`invalid catalog test name: ${test}`)
  }
  return components.map(component => `^${component.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}$`).join('/')
}

function escapeXML(value) {
  return String(value).replace(/[&<>"']/g, character => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&apos;' })[character])
}

export function writeSmokeReport(path, payload) {
  mkdirSync(dirname(path), { recursive: true })
  writeFileSync(path, `${JSON.stringify(payload, null, 2)}\n`)
  const failure = payload.result === 'pass' ? '' : `<failure message="${escapeXML(payload.reason)}"/>`
  writeFileSync(path.replace(/\.json$/, '.xml'), `<?xml version="1.0" encoding="UTF-8"?><testsuite name="meridian-smoke" tests="1" failures="${payload.result === 'pass' ? 0 : 1}"><testcase name="${escapeXML(payload.smokeId)}" time="${payload.durationSeconds}">${failure}</testcase></testsuite>\n`)
}
