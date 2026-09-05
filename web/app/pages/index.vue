<script setup lang="ts">
definePageMeta({ layout: false })

const { refresh } = useSession()
const loading = ref(true)

onMounted(async () => {
  try {
    const me = await refresh()
    await navigateTo(me?.tenants[0] ? `/t/${me.tenants[0].slug}/dashboard` : '/no-tenant')
  } catch {
    await navigateTo('/login')
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <main class="flex min-h-screen items-center justify-center bg-[#f4f7f8] px-6">
    <div class="text-center">
      <div class="mx-auto flex size-12 items-center justify-center rounded-xl bg-teal-600 text-lg font-bold text-white">M</div>
      <p class="mt-5 text-sm font-medium text-slate-600">{{ loading ? '正在加载 Meridian…' : '正在跳转…' }}</p>
    </div>
  </main>
</template>
