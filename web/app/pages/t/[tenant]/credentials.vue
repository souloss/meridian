<script setup lang="ts">
import { useListCredentials } from '~/api/generated/tenant/tenant'
import type { CredentialPage } from '~/api/generated/models/credentialPage'
import { responseData } from '~/composables/useResponseData'

definePageMeta({ layout: 'tenant', middleware: ['auth', 'tenant'] })

const route = useRoute()
const tenantSlug = computed(() => String(route.params.tenant))
const page = ref(1)
const params = computed(() => ({ page: page.value, pageSize: 20 }))
const query = useListCredentials(tenantSlug, params)
const result = computed(() => responseData<CredentialPage>(query.data.value))
const credentials = computed(() => result.value?.items ?? [])

function formatDate(value: string) {
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}
</script>

<template>
  <div class="space-y-6">
    <section class="flex flex-col justify-between gap-4 border-b border-slate-200 pb-6 sm:flex-row sm:items-end">
      <div><p class="text-sm font-medium text-teal-700">安全与连接</p><h2 class="mt-1 text-2xl font-semibold tracking-tight text-slate-950">凭据</h2><p class="mt-2 text-sm text-slate-600">查看已授权凭据的元数据和共享范围，秘密永不回显。</p></div>
      <UButton color="primary" icon="i-lucide-plus" label="创建凭据" disabled />
    </section>
    <UAlert v-if="query.error.value" color="error" variant="subtle" icon="i-lucide-circle-alert" title="凭据列表加载失败" description="请刷新页面后重试。" />
    <UCard :ui="{ body: 'p-0' }">
      <div v-if="query.isLoading.value" class="space-y-3 p-5"><USkeleton v-for="index in 5" :key="index" class="h-14 w-full" /></div>
      <div v-else-if="credentials.length === 0" class="p-12 text-center"><UIcon name="i-lucide-key-round" class="mx-auto size-8 text-slate-300" /><p class="mt-3 text-sm font-medium text-slate-700">暂无凭据</p><p class="mt-1 text-sm text-slate-600">创建后可绑定到租户仓库。</p></div>
      <div v-else class="divide-y divide-slate-100">
        <div v-for="credential in credentials" :key="credential.id" class="flex flex-col gap-3 px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
          <div class="flex min-w-0 items-center gap-3"><div class="flex size-9 shrink-0 items-center justify-center rounded-md bg-slate-100 text-slate-600"><UIcon :name="credential.kind === 'ssh_key' ? 'i-lucide-key-round' : 'i-lucide-shield-check'" class="size-4" /></div><div class="min-w-0"><p class="truncate text-sm font-semibold text-slate-800">{{ credential.name }}</p><p class="mt-1 truncate font-mono text-xs text-slate-600">{{ credential.fingerprint }}</p></div></div>
          <div class="flex items-center gap-3 sm:pl-4"><UBadge v-if="credential.isGlobal" color="primary" variant="subtle">全局</UBadge><UBadge color="neutral" variant="subtle">{{ credential.sharedScope }}</UBadge><span class="hidden text-xs text-slate-600 lg:inline">{{ formatDate(credential.updatedAt) }}</span></div>
        </div>
      </div>
      <template v-if="result && result.total > result.pageSize" #footer><div class="flex items-center justify-between"><p class="text-xs text-slate-600">共 {{ result.total }} 条凭据</p><div class="flex gap-2"><UButton color="neutral" variant="outline" size="sm" label="上一页" :disabled="page <= 1" @click="page--" /><UButton color="neutral" variant="outline" size="sm" label="下一页" :disabled="page * result.pageSize >= result.total" @click="page++" /></div></div></template>
    </UCard>
  </div>
</template>
