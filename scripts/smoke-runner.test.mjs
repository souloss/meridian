import assert from 'node:assert/strict'
import { mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'
import { assessGoTest, digestSourceFiles, goTestSelector, smokeFixtureStatus, writeSmokeReport } from './write-smoke-report.mjs'

const name = 'TestM0CredentialSmoke/SMK-005'
const fixture = { package: 'github.com/meridian-labs/meridian/internal/handler', test: name }
const event = (Action, Test = name) => ({ Package: 'github.com/meridian-labs/meridian/internal/handler', Test, Action })
for (const scenario of [
  { name: 'named test passes', events: [event('run'), event('pass')], exitCode: 0, result: 'pass' },
  { name: 'zero tests cannot pass', events: [], exitCode: 0, result: 'fail' },
  { name: 'parent pass is insufficient', events: [event('pass', 'TestM0CredentialSmoke')], exitCode: 0, result: 'fail' },
  { name: 'skip cannot pass', events: [event('skip')], exitCode: 0, result: 'fail' },
  { name: 'skipped nested assertion cannot pass', events: [event('pass'), event('skip', `${name}/assertion`)], exitCode: 0, result: 'fail' },
  { name: 'failing nested assertion cannot pass', events: [event('pass'), event('fail', `${name}/assertion`)], exitCode: 0, result: 'fail' },
  { name: 'command failure overrides test pass', events: [event('pass')], exitCode: 1, result: 'fail' },
  { name: 'signal or timeout cannot pass', events: [event('pass')], exitCode: null, result: 'fail' },
  { name: 'wrong package cannot pass', events: [{ ...event('pass'), Package: 'other' }], exitCode: 0, result: 'fail' }
]) {
  test(scenario.name, () => assert.equal(assessGoTest(scenario.events, fixture, scenario.exitCode).result, scenario.result))
}

test('catalog package is part of the fixture identity', () => {
  assert.equal(assessGoTest([event('pass')], { ...fixture, package: 'other' }, 0).result, 'fail')
})

test('future catalog entries remain tooling gaps until all execution fields exist', () => {
  assert.deepEqual(smokeFixtureStatus({ milestone: 'M5' }, false), {
    result: 'fail', failureKind: 'tooling_gap', reason: 'catalog entry has no executable fixture, test, and package mapping'
  })
  assert.deepEqual(smokeFixtureStatus({ ...fixture, fixture: 'missing.go' }, false), {
    result: 'fail', failureKind: 'tooling_gap', reason: 'catalog fixture does not exist: missing.go'
  })
  assert.equal(smokeFixtureStatus({ ...fixture, fixture: 'fixture.go' }, true), null)
})

test('Go test selector targets every test path component exactly', () => {
  assert.equal(goTestSelector('TestM0Smoke/SMK-003'), '^TestM0Smoke$/^SMK-003$')
  assert.throws(() => goTestSelector('TestM0Smoke/bad fixture'), /invalid catalog test name/)
})

test('report preserves evidence and escapes JUnit failure messages', t => {
  const directory = mkdtempSync(join(tmpdir(), 'meridian-smoke-report-'))
  t.after(() => rmSync(directory, { recursive: true, force: true }))
  const path = join(directory, 'SMK-005.json')
  const payload = { smokeId: 'SMK-005', result: 'fail', reason: 'missing "fixture" <required> & test', durationSeconds: 0 }
  writeSmokeReport(path, payload)
  assert.deepEqual(JSON.parse(readFileSync(path, 'utf8')), payload)
  assert.match(readFileSync(path.replace('.json', '.xml'), 'utf8'), /failures="1"/)
  assert.match(readFileSync(path.replace('.json', '.xml'), 'utf8'), /&quot;fixture&quot; &lt;required&gt; &amp; test/)
})

test('source digest handles directory symlinks, changed and deleted files', t => {
  const directory = mkdtempSync(join(tmpdir(), 'meridian-source-digest-'))
  t.after(() => rmSync(directory, { recursive: true, force: true }))
  const source = join(directory, 'source')
  const link = join(directory, 'dist')
  const missing = join(directory, 'missing')
  writeFileSync(source, 'before')
  symlinkSync(directory, link)
  const digest = digestSourceFiles([source, link, missing])
  assert.equal(digest, digestSourceFiles([missing, link, source, source]))
  writeFileSync(source, 'after')
  assert.notEqual(digest, digestSourceFiles([source, link, missing]))
})
