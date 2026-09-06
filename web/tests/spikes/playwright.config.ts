import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: '.',
  testMatch: /.*\.spec\.ts/,
  fullyParallel: false,
  workers: 1,
  retries: 0,
  forbidOnly: true,
  reporter: [['line']],
  use: {
    baseURL: 'http://127.0.0.1:4182',
    trace: 'retain-on-failure'
  },
  webServer: {
    command: 'node ../../scripts/serve-spike-harness.mjs',
    url: 'http://127.0.0.1:4182/?mode=table-graph',
    reuseExistingServer: false,
    timeout: 30_000
  },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'], viewport: { width: 1440, height: 960 } } },
    { name: 'mobile', use: { ...devices['Pixel 5'] } }
  ]
})
