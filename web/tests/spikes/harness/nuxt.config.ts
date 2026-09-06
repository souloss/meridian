export default defineNuxtConfig({
  ssr: false,
  modules: ['@nuxt/ui'],
  css: ['~/assets/spike.css'],
  ui: { fonts: false },
  icon: { provider: 'none', fallbackToApi: false },
  devtools: { enabled: false },
  app: {
    head: {
      title: 'Meridian 组件能力验证',
      htmlAttrs: { lang: 'zh-CN' }
    }
  },
  compatibilityDate: '2025-07-15'
})
