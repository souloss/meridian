<script setup lang="ts">
definePageMeta({ middleware: ['auth'], layout: false })
const { signOut } = useSession()

async function leave() {
  await signOut()
  await navigateTo('/login')
}
</script>

<template>
  <main class="flex min-h-screen items-center justify-center bg-[#f4f7f8] px-6">
    <section class="w-full max-w-md rounded-xl border border-slate-200 bg-white p-8 text-center shadow-sm">
      <div class="mx-auto flex size-11 items-center justify-center rounded-full bg-amber-50 text-amber-700">
        <UIcon name="i-lucide-building-2" class="size-5" aria-hidden="true" />
      </div>
      <h1 class="mt-5 text-xl font-semibold text-slate-950">暂无可访问租户</h1>
      <p class="mt-2 text-sm leading-6 text-slate-600">当前账户尚未加入任何租户，请联系平台管理员。</p>
      <UButton class="mt-6" color="neutral" variant="outline" label="退出登录" icon="i-lucide-log-out" @click="leave" />
    </section>
  </main>
</template>
