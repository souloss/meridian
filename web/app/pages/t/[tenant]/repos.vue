<script setup lang="ts">
import { deleteRepository, useListRepositories } from '~/api/generated/repository/repository'
import { useListCredentials } from '~/api/generated/tenant/tenant'
import type { CredentialPage } from '~/api/generated/models/credentialPage'
import type { Repository } from '~/api/generated/models/repository'
import type { RepositoryPage } from '~/api/generated/models/repositoryPage'
import { responseData } from '~/composables/useResponseData'
import { useQueryClient } from '@tanstack/vue-query'

definePageMeta({ layout: 'tenant', middleware: ['auth', 'tenant'] })

const route = useRoute()
const tenantSlug = computed(() => String(route.params.tenant))
const page = ref(1)
const params = computed(() => ({ page: page.value, pageSize: 20 }))
const query = useListRepositories(tenantSlug, params)
const credentialQuery = useListCredentials(tenantSlug, { page: 1, pageSize: 100 })
const queryClient = useQueryClient()
const createOpen = ref(false)
const editOpen = ref(false)
const editTarget = ref<Repository>()
const deleteOpen = ref(false)
const deleteTarget = ref<Repository>()
const deleteBusy = ref(false)
const deleteError = ref('')
const result = computed(() => responseData<RepositoryPage>(query.data.value))
const repositories = computed(() => result.value?.items ?? [])
const credentials = computed(() => responseData<CredentialPage>(credentialQuery.data.value)?.items ?? [])

function formatDate(value: string) {
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

async function refreshRepositories() {
  await queryClient.invalidateQueries({ queryKey: ['api', 'v1', 't', tenantSlug.value, 'repositories'] })
}

function editRepository(repository: Repository) {
  editTarget.value = repository
  editOpen.value = true
}

function requestDelete(repository: Repository) {
  deleteTarget.value = repository
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
    await deleteRepository(tenantSlug.value, deleteTarget.value.id, { headers: { 'If-Match': deleteTarget.value.etag } })
    await refreshRepositories()
    deleted = true
  } catch {
    deleteError.value = '仓库删除失败，资源可能已被更新，请刷新后重试。'
  } finally {
    deleteBusy.value = false
  }
  if (deleted) updateDeleteOpen(false)
}
</script>

<template>
  <div class="space-y-6">
    <section class="flex flex-col justify-between gap-4 border-b border-slate-200 pb-6 sm:flex-row sm:items-end">
      <div><p class="text-sm font-medium text-teal-700">资源目录</p><h2 class="mt-1 text-2xl font-semibold tracking-tight text-slate-950">仓库</h2><p class="mt-2 text-sm text-slate-600">管理 Git 连接、默认分支和同步健康状态。</p></div>
      <UButton color="primary" icon="i-lucide-plus" label="添加仓库" @click="createOpen = true" />
    </section>
    <UAlert v-if="query.error.value" color="error" variant="subtle" icon="i-lucide-circle-alert" title="仓库列表加载失败" description="请刷新页面后重试。" />
    <UCard :ui="{ body: 'p-0' }">
      <div v-if="query.isLoading.value" class="space-y-3 p-5"><USkeleton v-for="index in 5" :key="index" class="h-14 w-full" /></div>
      <div v-else-if="repositories.length === 0" class="p-12 text-center"><UIcon name="i-lucide-git-branch" class="mx-auto size-8 text-slate-300" /><p class="mt-3 text-sm font-medium text-slate-700">暂无仓库</p><p class="mt-1 text-sm text-slate-600">添加仓库后，它们会出现在这里。</p></div>
      <div v-else class="divide-y divide-slate-100">
        <div v-for="repository in repositories" :key="repository.id" class="flex flex-col gap-3 px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
          <div class="min-w-0"><p class="truncate text-sm font-semibold text-slate-800">{{ repository.url }}</p><p class="mt-1 flex flex-wrap items-center gap-2 text-xs text-slate-600"><span class="font-mono">{{ repository.defaultBranch }}</span><span class="text-slate-300">·</span><span>更新于 {{ formatDate(repository.updatedAt) }}</span></p></div>
          <div class="flex items-center gap-2"><UBadge :color="repository.health.failStreak === 0 ? 'success' : repository.health.lastSyncAt ? 'warning' : 'error'" variant="subtle">{{ repository.health.failStreak === 0 ? 'healthy' : repository.health.lastSyncAt ? 'stale' : 'invalid' }}</UBadge><template v-if="repository.capabilities.includes('repository:write')"><UButton color="neutral" variant="ghost" icon="i-lucide-pencil" aria-label="编辑仓库" @click="editRepository(repository)" /><UButton color="error" variant="ghost" icon="i-lucide-trash-2" aria-label="删除仓库" @click="requestDelete(repository)" /></template></div>
        </div>
      </div>
      <template v-if="result && result.total > result.pageSize" #footer><div class="flex items-center justify-between"><p class="text-xs text-slate-600">共 {{ result.total }} 个仓库</p><div class="flex gap-2"><UButton color="neutral" variant="outline" size="sm" label="上一页" :disabled="page <= 1" @click="page--" /><UButton color="neutral" variant="outline" size="sm" label="下一页" :disabled="page * result.pageSize >= result.total" @click="page++" /></div></div></template>
    </UCard>
    <RepositoryCreateModal v-model:open="createOpen" :tenant-slug="tenantSlug" :credentials="credentials" @created="refreshRepositories" />
    <RepositoryEditModal v-model:open="editOpen" :tenant-slug="tenantSlug" :repository="editTarget" :credentials="credentials" @updated="refreshRepositories" />
    <ActionConfirmModal :open="deleteOpen" title="删除仓库" :description="`确定删除 ${deleteTarget?.url ?? '此仓库'}？删除后不会再出现在租户列表中。`" :busy="deleteBusy" :error-message="deleteError" @update:open="updateDeleteOpen" @confirm="confirmDelete" />
  </div>
</template>
