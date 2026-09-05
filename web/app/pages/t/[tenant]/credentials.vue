<script setup lang="ts">
import { deleteCredential, useListCredentials } from '~/api/generated/tenant/tenant'
import type { CredentialPage } from '~/api/generated/models/credentialPage'
import type { Credential } from '~/api/generated/models/credential'
import { responseData } from '~/composables/useResponseData'
import { MeridianApiError } from '~/api/fetcher'
import { useQueryClient } from '@tanstack/vue-query'

definePageMeta({ layout: 'tenant', middleware: ['auth', 'tenant'] })

const route = useRoute()
const tenantSlug = computed(() => String(route.params.tenant))
const page = ref(1)
const params = computed(() => ({ page: page.value, pageSize: 20 }))
const query = useListCredentials(tenantSlug, params)
const queryClient = useQueryClient()
const createOpen = ref(false)
const editOpen = ref(false)
const editTarget = ref<Credential>()
const deleteOpen = ref(false)
const deleteTarget = ref<Credential>()
const deleteBusy = ref(false)
const deleteError = ref('')
const result = computed(() => responseData<CredentialPage>(query.data.value))
const credentials = computed(() => result.value?.items ?? [])

function formatDate(value: string) {
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

async function refreshCredentials() {
  await queryClient.invalidateQueries({ queryKey: ['api', 'v1', 't', tenantSlug.value, 'credentials'] })
}

function editCredential(credential: Credential) {
  editTarget.value = credential
  editOpen.value = true
}

function requestDelete(credential: Credential) {
  deleteTarget.value = credential
  deleteError.value = ''
  deleteOpen.value = true
}

function updateDeleteOpen(value: boolean) {
  if (deleteBusy.value) return
  deleteOpen.value = value
  if (!value) deleteTarget.value = undefined
}

async function confirmDelete() {
  if (!deleteTarget.value) return
  deleteBusy.value = true
  deleteError.value = ''
  let deleted = false
  try {
    await deleteCredential(tenantSlug.value, deleteTarget.value.id, undefined, { headers: { 'If-Match': deleteTarget.value.etag } })
    await refreshCredentials()
    deleted = true
  } catch (error) {
    deleteError.value = error instanceof MeridianApiError && error.status === 409 ? '凭据正在被仓库使用，请先解绑后再删除。' : '凭据删除失败，请刷新后重试。'
  } finally {
    deleteBusy.value = false
  }
  if (deleted) updateDeleteOpen(false)
}
</script>

<template>
  <div class="space-y-6">
    <section class="flex flex-col justify-between gap-4 border-b border-slate-200 pb-6 sm:flex-row sm:items-end">
      <div><p class="text-sm font-medium text-teal-700">安全与连接</p><h2 class="mt-1 text-2xl font-semibold tracking-tight text-slate-950">凭据</h2><p class="mt-2 text-sm text-slate-600">查看已授权凭据的元数据和共享范围，秘密永不回显。</p></div>
      <UButton color="primary" icon="i-lucide-plus" label="创建凭据" @click="createOpen = true" />
    </section>
    <UAlert v-if="query.error.value" color="error" variant="subtle" icon="i-lucide-circle-alert" title="凭据列表加载失败" description="请刷新页面后重试。" />
    <UCard :ui="{ body: 'p-0' }">
      <div v-if="query.isLoading.value" class="space-y-3 p-5"><USkeleton v-for="index in 5" :key="index" class="h-14 w-full" /></div>
      <div v-else-if="credentials.length === 0" class="p-12 text-center"><UIcon name="i-lucide-key-round" class="mx-auto size-8 text-slate-300" /><p class="mt-3 text-sm font-medium text-slate-700">暂无凭据</p><p class="mt-1 text-sm text-slate-600">创建后可绑定到租户仓库。</p></div>
      <div v-else class="divide-y divide-slate-100">
        <div v-for="credential in credentials" :key="credential.id" class="flex flex-col gap-3 px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
          <div class="flex min-w-0 items-center gap-3"><div class="flex size-9 shrink-0 items-center justify-center rounded-md bg-slate-100 text-slate-600"><UIcon :name="credential.kind === 'ssh_key' ? 'i-lucide-key-round' : 'i-lucide-shield-check'" class="size-4" /></div><div class="min-w-0"><p class="truncate text-sm font-semibold text-slate-800">{{ credential.name }}</p><p class="mt-1 truncate font-mono text-xs text-slate-600">{{ credential.fingerprint }}</p></div></div>
          <div class="flex items-center gap-2 sm:pl-4"><UBadge v-if="credential.isGlobal" color="primary" variant="subtle">全局</UBadge><UBadge color="neutral" variant="subtle">{{ credential.sharedScope }}</UBadge><span class="hidden text-xs text-slate-600 lg:inline">{{ formatDate(credential.updatedAt) }}</span><UButton color="neutral" variant="ghost" icon="i-lucide-pencil" aria-label="编辑凭据" @click="editCredential(credential)" /><UButton color="error" variant="ghost" icon="i-lucide-trash-2" aria-label="删除凭据" @click="requestDelete(credential)" /></div>
        </div>
      </div>
      <template v-if="result && result.total > result.pageSize" #footer><div class="flex items-center justify-between"><p class="text-xs text-slate-600">共 {{ result.total }} 条凭据</p><div class="flex gap-2"><UButton color="neutral" variant="outline" size="sm" label="上一页" :disabled="page <= 1" @click="page--" /><UButton color="neutral" variant="outline" size="sm" label="下一页" :disabled="page * result.pageSize >= result.total" @click="page++" /></div></div></template>
    </UCard>
    <CredentialCreateModal v-model:open="createOpen" :tenant-slug="tenantSlug" @created="refreshCredentials" />
    <CredentialEditModal v-model:open="editOpen" :tenant-slug="tenantSlug" :credential="editTarget" @updated="refreshCredentials" />
    <ActionConfirmModal :open="deleteOpen" title="删除凭据" :description="`确定删除 ${deleteTarget?.name ?? '此凭据'}？如果仍被仓库引用，服务端会拒绝删除。`" :busy="deleteBusy" :error-message="deleteError" @update:open="updateDeleteOpen" @confirm="confirmDelete" />
  </div>
</template>
