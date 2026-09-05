<script setup lang="ts">
import { MeridianApiError } from '~/api/fetcher'

definePageMeta({ layout: false })

const route = useRoute()
const { refresh, signIn } = useSession()
const username = ref('')
const password = ref('')
const busy = ref(false)
const errorMessage = ref('')

onMounted(async () => {
  try {
    const me = await refresh()
    if (me?.tenants[0]) {
      await navigateTo(String(route.query.returnTo || `/t/${me.tenants[0].slug}/dashboard`))
    }
  } catch {
    // An unauthenticated or unavailable session leaves the login form usable.
  }
})

async function submit() {
  errorMessage.value = ''
  busy.value = true
  try {
    const me = await signIn(username.value.trim(), password.value)
    const returnTo = typeof route.query.returnTo === 'string' ? route.query.returnTo : ''
    await navigateTo(returnTo.startsWith('/') ? returnTo : `/t/${me.tenants[0]?.slug ?? ''}/dashboard`)
  } catch (error) {
    if (error instanceof MeridianApiError && error.status === 401) {
      errorMessage.value = '用户名或密码不正确。'
    } else {
      errorMessage.value = '登录失败，请稍后重试。'
    }
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <main class="grid min-h-screen bg-[#f4f7f8] lg:grid-cols-[1.05fr_0.95fr]">
    <section class="hidden flex-col justify-between bg-slate-950 p-12 text-white lg:flex">
      <div class="flex items-center gap-3">
        <div class="flex size-9 items-center justify-center rounded-lg bg-teal-500 text-sm font-bold">M</div>
        <span class="text-sm font-semibold tracking-wide">Meridian</span>
      </div>
      <div class="max-w-lg">
        <p class="text-sm font-medium uppercase tracking-[0.2em] text-teal-300">Control plane</p>
        <h1 class="mt-5 text-4xl font-semibold leading-tight tracking-tight">让资产变更可见、可审计、可回溯。</h1>
        <p class="mt-5 max-w-md text-base leading-7 text-slate-300">从仓库接入到异步任务，每个租户都有清晰的边界和状态。</p>
      </div>
      <p class="text-xs text-slate-400">Meridian · M0 foundation</p>
    </section>
    <section class="flex items-center justify-center px-6 py-12 sm:px-10">
      <div class="w-full max-w-md">
        <div class="mb-10 lg:hidden">
          <div class="flex size-10 items-center justify-center rounded-lg bg-teal-600 text-sm font-bold text-white">M</div>
          <p class="mt-4 text-xl font-semibold text-slate-950">Meridian</p>
        </div>
        <div class="mb-8">
          <p class="text-sm font-medium text-teal-700">欢迎回来</p>
          <h1 class="mt-2 text-3xl font-semibold tracking-tight text-slate-950">登录控制面</h1>
          <p class="mt-3 text-sm leading-6 text-slate-600">使用本地账户进入你的租户工作区。</p>
        </div>
        <UAlert v-if="errorMessage" color="error" variant="subtle" icon="i-lucide-circle-alert" :title="errorMessage" class="mb-5" />
        <form class="space-y-5" @submit.prevent="submit">
          <UFormField label="用户名" name="username">
            <UInput v-model="username" autocomplete="username" size="lg" class="w-full" required autofocus />
          </UFormField>
          <UFormField label="密码" name="password">
            <UInput v-model="password" type="password" autocomplete="current-password" size="lg" class="w-full" required />
          </UFormField>
          <UButton type="submit" block size="lg" color="primary" :loading="busy" label="登录" trailing-icon="i-lucide-arrow-right" />
        </form>
      </div>
    </section>
  </main>
</template>
