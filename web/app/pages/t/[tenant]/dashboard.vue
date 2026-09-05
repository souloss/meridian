<script setup lang="ts">
import { useListJobs } from '~/api/generated/job/job'
import { useListRepositories } from '~/api/generated/repository/repository'
import { useListCredentials } from '~/api/generated/tenant/tenant'
import type { CredentialPage } from '~/api/generated/models/credentialPage'
import type { JobPage } from '~/api/generated/models/jobPage'
import type { RepositoryPage } from '~/api/generated/models/repositoryPage'
import { responseData } from '~/composables/useResponseData'

definePageMeta({ layout: 'tenant', middleware: ['auth', 'tenant'] })

const route = useRoute()
const tenantSlug = computed(() => String(route.params.tenant))
const params = computed(() => ({ page: 1, pageSize: 5 }))
const jobsQuery = useListJobs(tenantSlug, params)
const repositoriesQuery = useListRepositories(tenantSlug, params)
const credentialsQuery = useListCredentials(tenantSlug, params)
const jobs = computed(() => responseData<JobPage>(jobsQuery.data.value)?.items ?? [])
const repositories = computed(() => responseData<RepositoryPage>(repositoriesQuery.data.value)?.items ?? [])
const credentials = computed(() => responseData<CredentialPage>(credentialsQuery.data.value)?.items ?? [])
const activeJobs = computed(() => jobs.value.filter((job) => job.status === 'pending' || job.status === 'running').length)
const failedJobs = computed(() => jobs.value.filter((job) => job.status === 'failed' || job.status === 'outcome_unknown').length)

const statusLabels: Record<string, string> = {
  pending: '排队中',
  running: '执行中',
  succeeded: '已完成',
  succeeded_with_warnings: '完成但有警告',
  failed: '失败',
  outcome_unknown: '结果未知',
  cancelled: '已取消'
}

function statusLabel(status: string) {
  return statusLabels[status] ?? status
}
</script>

<template>
  <div class="space-y-8">
    <section class="flex flex-col justify-between gap-4 border-b border-slate-200 pb-6 sm:flex-row sm:items-end">
      <div>
        <p class="text-sm font-medium text-teal-700">工作区概览</p>
        <h2 class="mt-1 text-2xl font-semibold tracking-tight text-slate-950">{{ tenantSlug }}</h2>
        <p class="mt-2 text-sm text-slate-600">查看资源连接和最近异步任务的状态。</p>
      </div>
      <UButton to="jobs" color="primary" icon="i-lucide-list-checks" label="查看全部任务" />
    </section>

    <section class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4" aria-label="工作区统计">
      <UCard :ui="{ body: 'p-5' }">
        <p class="text-xs font-medium uppercase tracking-wide text-slate-600">仓库</p>
        <p class="mt-3 text-3xl font-semibold text-slate-950">{{ repositoriesQuery.isLoading.value ? '—' : repositories.length }}</p>
        <p class="mt-1 text-xs text-slate-600">当前页面记录</p>
      </UCard>
      <UCard :ui="{ body: 'p-5' }">
        <p class="text-xs font-medium uppercase tracking-wide text-slate-600">凭据</p>
        <p class="mt-3 text-3xl font-semibold text-slate-950">{{ credentialsQuery.isLoading.value ? '—' : credentials.length }}</p>
        <p class="mt-1 text-xs text-slate-600">可用凭据</p>
      </UCard>
      <UCard :ui="{ body: 'p-5' }">
        <p class="text-xs font-medium uppercase tracking-wide text-slate-600">进行中任务</p>
        <p class="mt-3 text-3xl font-semibold text-teal-700">{{ activeJobs }}</p>
        <p class="mt-1 text-xs text-slate-600">排队或执行中</p>
      </UCard>
      <UCard :ui="{ body: 'p-5' }">
        <p class="text-xs font-medium uppercase tracking-wide text-slate-600">需要关注</p>
        <p class="mt-3 text-3xl font-semibold text-amber-700">{{ failedJobs }}</p>
        <p class="mt-1 text-xs text-slate-600">失败或结果未知</p>
      </UCard>
    </section>

    <section class="grid gap-6 xl:grid-cols-[1.4fr_1fr]">
      <UCard :ui="{ body: 'p-0' }">
        <template #header>
          <div class="flex items-center justify-between">
            <div>
              <h3 class="font-semibold text-slate-950">最近任务</h3>
              <p class="mt-1 text-xs text-slate-600">按创建时间倒序</p>
            </div>
            <UButton :to="`/t/${tenantSlug}/jobs`" color="neutral" variant="ghost" size="sm" label="全部" trailing-icon="i-lucide-arrow-up-right" />
          </div>
        </template>
        <div v-if="jobsQuery.isLoading.value" class="space-y-3 p-5">
          <USkeleton v-for="index in 3" :key="index" class="h-12 w-full" />
        </div>
        <div v-else-if="jobs.length === 0" class="p-8 text-center text-sm text-slate-600">暂无任务记录</div>
        <div v-else class="divide-y divide-slate-100">
          <NuxtLink v-for="job in jobs" :key="job.id" :to="`/t/${tenantSlug}/jobs?job=${job.id}`" class="flex items-center justify-between gap-4 px-5 py-4 transition hover:bg-slate-50">
            <div class="min-w-0">
              <p class="truncate text-sm font-medium text-slate-800">{{ job.type }}</p>
              <p class="mt-1 truncate font-mono text-xs text-slate-600">{{ job.id }}</p>
            </div>
            <div class="flex shrink-0 items-center gap-3">
              <span class="hidden text-xs text-slate-600 sm:inline">{{ job.progress }}%</span>
              <UBadge :color="job.status === 'failed' || job.status === 'outcome_unknown' ? 'error' : job.status === 'running' ? 'primary' : 'neutral'" variant="subtle">{{ statusLabel(job.status) }}</UBadge>
            </div>
          </NuxtLink>
        </div>
      </UCard>

      <UCard>
        <template #header>
          <div>
            <h3 class="font-semibold text-slate-950">工作区入口</h3>
            <p class="mt-1 text-xs text-slate-600">进入高频管理页面</p>
          </div>
        </template>
        <div class="space-y-2">
          <NuxtLink :to="`/t/${tenantSlug}/repos`" class="flex items-center justify-between rounded-md border border-slate-200 px-4 py-3 text-sm font-medium text-slate-700 transition hover:border-teal-300 hover:bg-teal-50 hover:text-teal-800">
            <span class="flex items-center gap-3"><UIcon name="i-lucide-git-branch" class="size-4" />仓库连接</span><UIcon name="i-lucide-chevron-right" class="size-4" />
          </NuxtLink>
          <NuxtLink :to="`/t/${tenantSlug}/credentials`" class="flex items-center justify-between rounded-md border border-slate-200 px-4 py-3 text-sm font-medium text-slate-700 transition hover:border-teal-300 hover:bg-teal-50 hover:text-teal-800">
            <span class="flex items-center gap-3"><UIcon name="i-lucide-key-round" class="size-4" />凭据管理</span><UIcon name="i-lucide-chevron-right" class="size-4" />
          </NuxtLink>
          <NuxtLink :to="`/t/${tenantSlug}/jobs`" class="flex items-center justify-between rounded-md border border-slate-200 px-4 py-3 text-sm font-medium text-slate-700 transition hover:border-teal-300 hover:bg-teal-50 hover:text-teal-800">
            <span class="flex items-center gap-3"><UIcon name="i-lucide-activity" class="size-4" />任务监控</span><UIcon name="i-lucide-chevron-right" class="size-4" />
          </NuxtLink>
        </div>
      </UCard>
    </section>
  </div>
</template>
