<script setup lang="ts">
const route = useRoute()
const tenantSlug = computed(() => String(route.params.tenant ?? ''))
const menuOpen = ref(false)
const { me, refresh, signOut } = useSession()

if (!me.value) {
  await refresh()
}

const tenant = computed(() => me.value?.tenants.find((item) => item.slug === tenantSlug.value))
const navItems = computed(() => [
  { label: '概览', to: `/t/${tenantSlug.value}/dashboard`, icon: 'i-lucide-layout-dashboard' },
  { label: '仓库', to: `/t/${tenantSlug.value}/repos`, icon: 'i-lucide-git-branch' },
  { label: '凭据', to: `/t/${tenantSlug.value}/credentials`, icon: 'i-lucide-key-round' },
  { label: '任务', to: `/t/${tenantSlug.value}/jobs`, icon: 'i-lucide-list-checks' }
])

async function logoutAndRedirect() {
  await signOut()
  await navigateTo({ path: '/login', query: { returnTo: route.fullPath } })
}

function switchTenant(event: Event) {
  const value = (event.target as HTMLSelectElement).value
  if (value) {
    navigateTo(`/t/${value}/dashboard`)
  }
}
</script>

<template>
  <div class="min-h-screen bg-[#f4f7f8] text-slate-900">
    <div v-if="menuOpen" class="fixed inset-0 z-30 bg-slate-950/30 lg:hidden" @click="menuOpen = false" />
    <aside
      class="fixed inset-y-0 left-0 z-40 flex w-72 -translate-x-full flex-col border-r border-slate-200 bg-white transition-transform lg:translate-x-0"
      :class="{ 'translate-x-0': menuOpen }"
    >
      <div class="flex h-20 items-center gap-3 border-b border-slate-200 px-6">
        <div class="flex size-9 items-center justify-center rounded-lg bg-teal-600 text-sm font-bold text-white">M</div>
        <div>
          <p class="text-sm font-semibold tracking-wide text-slate-950">Meridian</p>
          <p class="text-xs text-slate-600">Asset control plane</p>
        </div>
      </div>
      <div class="border-b border-slate-200 p-4">
        <label class="sr-only" for="tenant-switcher">选择租户</label>
        <select
          id="tenant-switcher"
          class="w-full rounded-md border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-800 outline-none transition focus:border-teal-600 focus:ring-2 focus:ring-teal-100"
          :value="tenantSlug"
          @change="switchTenant"
        >
          <option v-for="item in me?.tenants ?? []" :key="item.id" :value="item.slug">{{ item.displayName }}</option>
        </select>
        <p class="mt-2 truncate px-1 text-xs text-slate-600">{{ tenant?.role ?? '成员' }} · {{ tenantSlug }}</p>
      </div>
      <nav class="flex-1 space-y-1 p-4" aria-label="主导航">
        <NuxtLink
          v-for="item in navItems"
          :key="item.to"
          :to="item.to"
          class="flex items-center gap-3 rounded-md px-3 py-2.5 text-sm font-medium text-slate-600 transition hover:bg-slate-100 hover:text-slate-950"
          active-class="bg-teal-50 text-teal-800 ring-1 ring-teal-100"
          @click="menuOpen = false"
        >
          <UIcon :name="item.icon" class="size-4" aria-hidden="true" />
          {{ item.label }}
        </NuxtLink>
      </nav>
      <div class="border-t border-slate-200 p-4">
        <div class="mb-3 flex items-center gap-3 px-2">
          <div class="flex size-8 items-center justify-center rounded-full bg-slate-200 text-xs font-semibold text-slate-700">
            {{ me?.user.displayName?.slice(0, 1).toUpperCase() }}
          </div>
          <div class="min-w-0">
            <p class="truncate text-sm font-medium text-slate-800">{{ me?.user.displayName }}</p>
            <p class="truncate text-xs text-slate-600">{{ me?.user.username }}</p>
          </div>
        </div>
        <UButton block color="neutral" variant="ghost" icon="i-lucide-log-out" label="退出登录" @click="logoutAndRedirect" />
      </div>
    </aside>

    <div class="lg:pl-72">
      <header class="sticky top-0 z-20 flex h-16 items-center justify-between border-b border-slate-200 bg-white/95 px-4 backdrop-blur lg:px-8">
        <div class="flex items-center gap-3">
          <UButton class="lg:hidden" color="neutral" variant="ghost" icon="i-lucide-menu" aria-label="打开导航" @click="menuOpen = true" />
          <div>
            <p class="text-xs font-medium uppercase tracking-[0.16em] text-teal-700">{{ tenantSlug }}</p>
            <h1 class="text-base font-semibold text-slate-950">{{ tenant?.displayName ?? '租户控制面' }}</h1>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <UBadge v-if="me?.isPlatformAdmin" color="warning" variant="subtle">平台管理员</UBadge>
          <UButton color="neutral" variant="ghost" icon="i-lucide-circle-help" aria-label="帮助" />
        </div>
      </header>
      <main class="mx-auto max-w-[1440px] px-4 py-6 sm:px-6 lg:px-8">
        <slot />
      </main>
    </div>
  </div>
</template>
