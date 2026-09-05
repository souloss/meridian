import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'

export default defineNuxtPlugin((nuxtApp) => {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        refetchOnWindowFocus: false,
        retry: 1,
        staleTime: 30_000
      },
      mutations: {
        retry: false
      }
    }
  })

  nuxtApp.vueApp.use(VueQueryPlugin, { queryClient })
})
