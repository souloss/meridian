import { getMe } from '~/api/generated/auth/auth'
import { MeridianApiError } from '~/api/fetcher'

export default defineNuxtRouteMiddleware(async (to) => {
  try {
    const response = await getMe()
    if (response.status !== 200) {
      return navigateTo({ path: '/login', query: { returnTo: to.fullPath } })
    }
    if (response.data.isPlatformAdmin) return
    const fallback = response.data.tenants[0]
    return navigateTo(fallback ? `/t/${fallback.slug}/dashboard` : '/no-tenant')
  } catch (error) {
    if (error instanceof MeridianApiError && error.status === 401) {
      return navigateTo({ path: '/login', query: { returnTo: to.fullPath } })
    }
    throw error
  }
})
