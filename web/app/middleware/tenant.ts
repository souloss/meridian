import { getMe } from '~/api/generated/auth/auth'
import { MeridianApiError } from '~/api/fetcher'

export default defineNuxtRouteMiddleware(async (to) => {
  const tenantSlug = String(to.params.tenant ?? '')
  if (!tenantSlug) {
    return navigateTo('/login')
  }
  try {
    const response = await getMe()
    if (response.status !== 200) {
      return navigateTo('/login')
    }
    const membership = response.data.tenants.find((tenant) => tenant.slug === tenantSlug)
    if (membership) {
      return
    }
    const fallback = response.data.tenants[0]
    return navigateTo(fallback ? `/t/${fallback.slug}/dashboard` : '/no-tenant')
  } catch (error) {
    if (error instanceof MeridianApiError && error.status === 401) {
      return navigateTo({ path: '/login', query: { returnTo: to.fullPath } })
    }
    throw error
  }
})
