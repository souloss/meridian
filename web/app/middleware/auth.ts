import { getMe } from '~/api/generated/auth/auth'
import { MeridianApiError } from '~/api/fetcher'

export default defineNuxtRouteMiddleware(async (to) => {
  if (to.path === '/login') {
    return
  }
  try {
    await getMe()
  } catch (error) {
    if (error instanceof MeridianApiError && error.status === 401) {
      return navigateTo({ path: '/login', query: { returnTo: to.fullPath } })
    }
    throw error
  }
})
