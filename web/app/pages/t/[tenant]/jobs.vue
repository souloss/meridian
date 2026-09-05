<script setup lang="ts">
import { cancelJob, getJob, listJobs, retryJob } from '~/api/generated/job/job'
import type { Job } from '~/api/generated/models/job'
import type { JobAccepted } from '~/api/generated/models/jobAccepted'
import type { JobPage } from '~/api/generated/models/jobPage'
import { MeridianApiError } from '~/api/fetcher'
import { responseData } from '~/composables/useResponseData'

definePageMeta({ layout: 'tenant', middleware: ['auth', 'tenant'] })

const route = useRoute()
const tenantSlug = computed(() => String(route.params.tenant))
const page = ref(1)
const loading = ref(true)
const actionBusy = ref(false)
const errorMessage = ref('')
const jobs = ref<Job[]>([])
const total = ref(0)
const selected = ref<Job | undefined>()
const selectedLoading = ref(false)
const streamConnected = ref(false)
let eventSource: EventSource | undefined
const statusLabels: Record<string, string> = { pending: '排队中', running: '执行中', succeeded: '已完成', succeeded_with_warnings: '完成但有警告', failed: '失败', outcome_unknown: '结果未知', cancelled: '已取消' }

const filters = ref<{ statuses?: Array<'pending' | 'running' | 'succeeded' | 'succeeded_with_warnings' | 'failed' | 'outcome_unknown' | 'cancelled'> }>({})

function statusLabel(status: string) { return statusLabels[status] ?? status }
function statusColor(status: string) { return status === 'failed' || status === 'outcome_unknown' ? 'error' : status === 'running' ? 'primary' : status === 'succeeded' || status === 'succeeded_with_warnings' ? 'success' : 'neutral' }
function formatDate(value: string) { return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) }

async function loadJobs() {
  loading.value = true
  errorMessage.value = ''
  try {
    const response = await listJobs(tenantSlug.value, { page: page.value, pageSize: 20, filter: filters.value })
    const data = responseData<JobPage>(response)
    jobs.value = data?.items ?? []
    total.value = data?.total ?? 0
    const selectedID = typeof route.query.job === 'string' ? route.query.job : ''
    const current = jobs.value.find((job) => job.id === selectedID)
    if (current) {
      selected.value = current
    } else if (selected.value && !jobs.value.some((job) => job.id === selected.value?.id)) {
      selected.value = undefined
    }
  } catch {
    errorMessage.value = '任务列表加载失败，请刷新页面后重试。'
  } finally {
    loading.value = false
  }
}

async function selectJob(job: Job) {
  selected.value = job
  await navigateTo({ query: { ...route.query, job: job.id } })
  selectedLoading.value = true
  try {
    const response = await getJob(tenantSlug.value, job.id)
    selected.value = responseData<Job>(response)
  } finally {
    selectedLoading.value = false
  }
  connectSelectedJob()
}

function connectSelectedJob() {
  eventSource?.close()
  streamConnected.value = false
  if (!selected.value) return
  eventSource = new EventSource(`/api/v1/t/${encodeURIComponent(tenantSlug.value)}/jobs/${encodeURIComponent(selected.value.id)}/logs`)
  eventSource.addEventListener('open', () => { streamConnected.value = true })
  eventSource.addEventListener('state', () => { void refreshSelectedJob() })
  eventSource.addEventListener('error', () => { streamConnected.value = false })
}

async function refreshSelectedJob() {
  if (!selected.value) return
  try {
    const response = await getJob(tenantSlug.value, selected.value.id)
    const fresh = responseData<Job>(response)
    if (fresh) {
      selected.value = fresh
      if (['succeeded', 'succeeded_with_warnings', 'failed', 'outcome_unknown', 'cancelled'].includes(fresh.status)) {
        eventSource?.close()
        streamConnected.value = false
      }
    }
  } catch {
    // The list query remains the durable fallback if a stream reconnect races a deletion.
  }
}

async function cancelSelected() {
  if (!selected.value) return
  actionBusy.value = true
  errorMessage.value = ''
  try {
    await cancelJob(tenantSlug.value, selected.value.id)
    await loadJobs()
    if (selected.value) await selectJob(selected.value)
  } catch (error) {
    errorMessage.value = error instanceof MeridianApiError && error.code === 'job_not_cancellable' ? '该任务已结束，无法取消。' : '取消任务失败，请稍后重试。'
  } finally {
    actionBusy.value = false
  }
}

async function retrySelected() {
  if (!selected.value) return
  actionBusy.value = true
  errorMessage.value = ''
  try {
    const response = await retryJob(tenantSlug.value, selected.value.id, { headers: { 'Idempotency-Key': crypto.randomUUID() } })
    const accepted = responseData<JobAccepted>(response)
    await loadJobs()
    if (accepted) {
      const retried = jobs.value.find((job) => job.id === accepted.jobId)
      if (retried) await selectJob(retried)
    }
  } catch (error) {
    errorMessage.value = error instanceof MeridianApiError && error.code === 'invalid_state' ? '当前已有等价任务在执行，暂时不能重试。' : '重试任务失败，请稍后重试。'
  } finally {
    actionBusy.value = false
  }
}

watch([tenantSlug, page, filters], loadJobs, { immediate: true, deep: true })
onUnmounted(() => eventSource?.close())
</script>

<template>
  <div class="space-y-6">
    <section class="flex flex-col justify-between gap-4 border-b border-slate-200 pb-6 sm:flex-row sm:items-end">
      <div><p class="text-sm font-medium text-teal-700">运行状态</p><h2 class="mt-1 text-2xl font-semibold tracking-tight text-slate-950">任务</h2><p class="mt-2 text-sm text-slate-600">查看阶段进度、尝试记录和可执行的控制操作。</p></div>
      <div class="flex items-center gap-2"><USelect v-model="filters.statuses" :items="Object.keys(statusLabels).map((value) => ({ label: statusLabel(value), value }))" multiple placeholder="全部状态" aria-label="按状态筛选" class="w-44" /><UButton color="neutral" variant="outline" icon="i-lucide-refresh-cw" aria-label="刷新任务" :loading="loading" @click="loadJobs" /></div>
    </section>
    <UAlert v-if="errorMessage" color="error" variant="subtle" icon="i-lucide-circle-alert" :title="errorMessage" />
    <div class="grid gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(320px,0.72fr)]">
      <UCard :ui="{ body: 'p-0' }">
        <div v-if="loading" class="space-y-3 p-5"><USkeleton v-for="index in 6" :key="index" class="h-16 w-full" /></div>
        <div v-else-if="jobs.length === 0" class="p-12 text-center"><UIcon name="i-lucide-list-checks" class="mx-auto size-8 text-slate-300" /><p class="mt-3 text-sm font-medium text-slate-700">暂无任务</p><p class="mt-1 text-sm text-slate-600">当仓库同步或其他异步操作启动后，会在这里显示。</p></div>
        <div v-else class="divide-y divide-slate-100">
          <button v-for="job in jobs" :key="job.id" type="button" class="flex w-full items-center justify-between gap-4 px-5 py-4 text-left transition hover:bg-slate-50" :class="selected?.id === job.id ? 'bg-teal-50/60' : ''" @click="selectJob(job)">
            <div class="min-w-0"><div class="flex items-center gap-2"><span class="truncate text-sm font-semibold text-slate-800">{{ job.type }}</span><UBadge :color="statusColor(job.status)" variant="subtle" size="sm">{{ statusLabel(job.status) }}</UBadge></div><p class="mt-1 truncate font-mono text-xs text-slate-600">{{ job.id }}</p></div>
            <div class="flex shrink-0 items-center gap-3"><div class="hidden w-24 sm:block"><div class="mb-1 flex justify-between text-[11px] text-slate-600"><span>进度</span><span>{{ job.progress }}%</span></div><div class="h-1.5 overflow-hidden rounded-full bg-slate-100"><div class="h-full rounded-full bg-teal-500 transition-all" :style="{ width: `${job.progress}%` }" /></div></div><UIcon name="i-lucide-chevron-right" class="size-4 text-slate-600" /></div>
          </button>
        </div>
        <template v-if="total > 20" #footer><div class="flex items-center justify-between"><p class="text-xs text-slate-600">共 {{ total }} 个任务</p><div class="flex gap-2"><UButton color="neutral" variant="outline" size="sm" label="上一页" :disabled="page <= 1" @click="page--" /><UButton color="neutral" variant="outline" size="sm" label="下一页" :disabled="page * 20 >= total" @click="page++" /></div></div></template>
      </UCard>

      <UCard v-if="selected" :ui="{ body: 'p-0' }">
        <template #header><div class="flex items-start justify-between gap-3"><div><div class="flex items-center gap-2"><p class="text-xs font-medium uppercase tracking-wide text-teal-700">任务详情</p><UBadge v-if="streamConnected" color="success" variant="subtle" size="sm">实时</UBadge></div><h3 class="mt-1 font-semibold text-slate-950">{{ selected.type }}</h3></div><UBadge :color="statusColor(selected.status)" variant="subtle">{{ statusLabel(selected.status) }}</UBadge></div></template>
        <div v-if="selectedLoading" class="space-y-3 p-5"><USkeleton class="h-5 w-2/3" /><USkeleton class="h-5 w-full" /><USkeleton class="h-20 w-full" /></div>
        <div v-else class="space-y-5 p-5">
          <div><p class="text-xs text-slate-600">任务 ID</p><p class="mt-1 break-all font-mono text-xs text-slate-700">{{ selected.id }}</p></div>
          <div class="grid grid-cols-2 gap-4"><div><p class="text-xs text-slate-600">进度</p><p class="mt-1 text-lg font-semibold text-slate-950">{{ selected.progress }}%</p></div><div><p class="text-xs text-slate-600">当前阶段</p><p class="mt-1 text-sm font-medium text-slate-800">{{ selected.stage ?? '等待执行' }}</p></div><div><p class="text-xs text-slate-600">尝试次数</p><p class="mt-1 text-sm font-medium text-slate-800">{{ selected.attempt }} / {{ selected.maxAttempts }}</p></div><div><p class="text-xs text-slate-600">触发方式</p><p class="mt-1 text-sm font-medium text-slate-800">{{ selected.trigger }}</p></div></div>
          <div><p class="text-xs text-slate-600">创建时间</p><p class="mt-1 text-sm text-slate-700">{{ formatDate(selected.createdAt) }}</p></div>
          <UAlert v-if="selected.error" color="error" variant="subtle" icon="i-lucide-circle-alert" :title="selected.error.message" :description="selected.error.code" />
          <div v-if="selected.attempts.length" class="space-y-2"><p class="text-xs font-medium text-slate-600">阶段尝试</p><div v-for="attempt in selected.attempts" :key="`${attempt.attempt}-${attempt.stage}`" class="flex items-center justify-between rounded-md bg-slate-50 px-3 py-2 text-xs"><span class="font-medium text-slate-700">{{ attempt.stage }} · attempt {{ attempt.attempt }}</span><UBadge :color="statusColor(attempt.status)" variant="subtle" size="sm">{{ statusLabel(attempt.status) }}</UBadge></div></div>
          <div class="flex flex-wrap gap-2 border-t border-slate-100 pt-4"><UButton v-if="selected.capabilities.includes('job:run') && ['pending', 'running'].includes(selected.status)" color="neutral" variant="outline" icon="i-lucide-square" label="取消任务" :loading="actionBusy" @click="cancelSelected" /><UButton v-if="selected.capabilities.includes('job:run') && ['failed', 'cancelled'].includes(selected.status)" color="primary" icon="i-lucide-rotate-ccw" label="重试任务" :loading="actionBusy" @click="retrySelected" /></div>
        </div>
      </UCard>
      <UCard v-else><div class="flex min-h-80 flex-col items-center justify-center text-center"><UIcon name="i-lucide-mouse-pointer-click" class="size-8 text-slate-300" /><p class="mt-3 text-sm font-medium text-slate-700">选择一个任务</p><p class="mt-1 max-w-xs text-sm text-slate-600">从列表选择任务，查看阶段历史和可用操作。</p></div></UCard>
    </div>
  </div>
</template>
