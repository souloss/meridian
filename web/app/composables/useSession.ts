import { computed } from 'vue'
import { useQueryClient } from '@tanstack/vue-query'
import {
  getGetMeQueryKey,
  getMe,
  login,
  logout,
  useGetMe
} from '~/api/generated/auth/auth'
import type { Me } from '~/api/generated/models/me'
import { MeridianApiError, setCsrfToken } from '~/api/fetcher'
import { responseData } from '~/composables/useResponseData'

export function useSession() {
  const queryClient = useQueryClient()
  const csrf = useState<string | undefined>('meridian-csrf-token', () => undefined)
  const meQuery = useGetMe({ query: { enabled: false, retry: false } })
  const me = computed(() => responseData<Me>(meQuery.data.value))

  const authenticated = computed(() => me.value !== undefined)

  async function refresh(): Promise<Me | undefined> {
    try {
      const response = await getMe()
      if (response.status !== 200) {
        return undefined
      }
      if (!isMe(response.data)) {
        return undefined
      }
      queryClient.setQueryData(getGetMeQueryKey(), response)
      return response.data
    } catch (error) {
      if (error instanceof MeridianApiError && error.status === 401) {
        queryClient.removeQueries({ queryKey: getGetMeQueryKey() })
        return undefined
      }
      throw error
    }
  }

  async function signIn(username: string, password: string): Promise<Me> {
    const response = await login({ username, password })
    if (response.status !== 200 || !response.data || typeof response.data !== 'object' || !('me' in response.data) || !isMe(response.data.me)) {
      throw new Error('login_failed')
    }
    csrf.value = response.data.csrfToken
    setCsrfToken(csrf.value)
    // 保持 get-me 缓存的数据形状为 Me；登录接口返回的是 LoginResult。
    queryClient.setQueryData(getGetMeQueryKey(), { ...response, data: response.data.me })
    return response.data.me
  }

  async function signOut(): Promise<void> {
    await logout()
    csrf.value = undefined
    setCsrfToken(undefined)
    queryClient.removeQueries({ queryKey: getGetMeQueryKey() })
  }

  return { authenticated, csrf, me, refresh, signIn, signOut }
}

function isMe(value: unknown): value is Me {
  if (!value || typeof value !== 'object' || !('user' in value) || !('tenants' in value) || !Array.isArray(value.tenants)) {
    return false
  }
  return typeof value.user === 'object' && value.user !== null
}
