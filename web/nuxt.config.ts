export default defineNuxtConfig({
  ssr: false,
  devtools: { enabled: false },
  modules: ['@nuxt/ui', '@pinia/nuxt', '@nuxtjs/i18n', '@nuxt/icon'],
  css: ['~/assets/css/main.css'],
  ui: {
    fonts: false
  },
  icon: {
    provider: 'none',
    fallbackToApi: false,
    serverBundle: 'local',
    clientBundle: { scan: true, sizeLimitKb: 256 }
  },
  i18n: {
    locales: [
      { code: 'zh-CN', language: 'zh-CN', file: 'zh-CN.json', name: '简体中文' },
      { code: 'en', language: 'en', file: 'en.json', name: 'English' }
    ],
    defaultLocale: 'zh-CN',
    strategy: 'no_prefix'
  },
  compatibilityDate: '2025-07-15'
})
