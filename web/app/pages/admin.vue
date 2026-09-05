<script setup lang="ts">
import { useListGlobalCredentials, useListPlatformAuditLogs, useListPlatformJobs } from '~/api/generated/platform/platform'
import { useListTenants, useListUsers } from '~/api/generated/platform/platform'
import type { AuditLogPage } from '~/api/generated/models/auditLogPage'
import type { GlobalCredentialPage } from '~/api/generated/models/globalCredentialPage'
import type { PlatformJobPage } from '~/api/generated/models/platformJobPage'
import type { TenantPage } from '~/api/generated/models/tenantPage'
import type { UserPage } from '~/api/generated/models/userPage'
import { responseData } from '~/composables/useResponseData'

definePageMeta({ layout: 'platform', middleware: ['auth', 'platform'] })

const params = { page: 1, pageSize: 20 }
const userQuery = useListUsers({ page: 1, pageSize: 20 })
const tenantQuery = useListTenants(params)
const credentialQuery = useListGlobalCredentials(params)
const jobQuery = useListPlatformJobs(params)
const auditQuery = useListPlatformAuditLogs(params)

const users = computed(() => responseData<UserPage>(userQuery.data.value)?.items ?? [])
const tenants = computed(() => responseData<TenantPage>(tenantQuery.data.value)?.items ?? [])
const credentials = computed(() => responseData<GlobalCredentialPage>(credentialQuery.data.value)?.items ?? [])
const jobs = computed(() => responseData<PlatformJobPage>(jobQuery.data.value)?.items ?? [])
const audits = computed(() => responseData<AuditLogPage>(auditQuery.data.value)?.items ?? [])
const loading = computed(() => userQuery.isLoading.value || tenantQuery.isLoading.value || credentialQuery.isLoading.value || jobQuery.isLoading.value || auditQuery.isLoading.value)

const statusLabels: Record<string, string> = {
  active: '启用', disabled: '停用', pending: '排队中', running: '执行中', succeeded: '已完成',
  succeeded_with_warnings: '完成但有警告', failed: '失败', outcome_unknown: '结果未知', cancelled: '已取消'
}

function statusLabel(value: string) {
  return statusLabels[value] ?? value
}

function statusColor(value: string) {
  if (value === 'failed' || value === 'outcome_unknown' || value === 'disabled') return 'error'
  if (value === 'running' || value === 'active') return 'primary'
  if (value === 'succeeded' || value === 'succeeded_with_warnings') return 'success'
  return 'neutral'
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}
</script>

<template>
  <div class="space-y-8">
    <section class="flex flex-col justify-between gap-4 border-b border-slate-200 pb-6 sm:flex-row sm:items-end">
      <div>
        <p class="text-sm font-medium text-slate-600">全局控制面</p>
        <h2 class="mt-1 text-2xl font-semibold tracking-tight text-slate-950">平台概览</h2>
        <p class="mt-2 max-w-2xl text-sm text-slate-600">集中查看身份、租户、全局凭据、异步任务和审计事实。业务正文仍严格留在租户边界内。</p>
      </div>
      <UBadge color="warning" variant="subtle" icon="i-lucide-shield-check">平台管理员</UBadge>
    </section>

    <section class="grid gap-4 sm:grid-cols-2 xl:grid-cols-5" aria-label="平台统计">
      <UCard v-for="item in [
        { label: '用户', value: users.length, icon: 'i-lucide-users', tone: 'text-slate-950' },
        { label: '租户', value: tenants.length, icon: 'i-lucide-building-2', tone: 'text-slate-950' },
        { label: '全局凭据', value: credentials.length, icon: 'i-lucide-key-round', tone: 'text-slate-950' },
        { label: '运行中任务', value: jobs.filter((job) => job.status === 'pending' || job.status === 'running').length, icon: 'i-lucide-loader-circle', tone: 'text-teal-700' },
        { label: '审计事件', value: audits.length, icon: 'i-lucide-scroll-text', tone: 'text-amber-700' }
      ]" :key="item.label" :ui="{ body: 'p-5' }">
        <div class="flex items-center justify-between"><p class="text-xs font-medium uppercase tracking-wide text-slate-600">{{ item.label }}</p><UIcon :name="item.icon" class="size-4 text-slate-400" /></div>
        <p class="mt-3 text-3xl font-semibold" :class="item.tone">{{ loading ? '—' : item.value }}</p>
        <p class="mt-1 text-xs text-slate-600">当前查询页</p>
      </UCard>
    </section>

    <section class="grid gap-6 xl:grid-cols-2">
      <UCard>
        <template #header><div><h3 class="font-semibold text-slate-950">身份目录</h3><p class="mt-1 text-xs text-slate-600">仅展示用户元数据，不读取密码哈希或会话字段。</p></div></template>
        <div v-if="userQuery.isError.value" class="p-5 text-sm text-red-700">用户目录暂时不可用，请稍后重试。</div>
        <div v-else-if="users.length === 0" class="p-5 text-sm text-slate-600">暂无用户。</div>
        <div v-else class="divide-y divide-slate-100">
          <div v-for="user in users" :key="user.id" class="flex items-center justify-between gap-4 py-3 first:pt-0 last:pb-0">
            <div class="min-w-0"><p class="truncate text-sm font-semibold text-slate-800">{{ user.displayName }}</p><p class="mt-1 truncate text-xs text-slate-600">{{ user.username }} · {{ user.email ?? '未设置邮箱' }}</p></div>
            <UBadge :color="statusColor(user.status)" variant="subtle">{{ statusLabel(user.status) }}</UBadge>
          </div>
        </div>
      </UCard>

      <UCard>
        <template #header><div><h3 class="font-semibold text-slate-950">租户目录</h3><p class="mt-1 text-xs text-slate-600">覆盖 active/disabled 生命周期状态和冻结配额摘要。</p></div></template>
        <div v-if="tenantQuery.isError.value" class="p-5 text-sm text-red-700">租户目录暂时不可用，请稍后重试。</div>
        <div v-else-if="tenants.length === 0" class="p-5 text-sm text-slate-600">暂无租户。</div>
        <div v-else class="divide-y divide-slate-100">
          <div v-for="tenant in tenants" :key="tenant.id" class="flex items-center justify-between gap-4 py-3 first:pt-0 last:pb-0">
            <div class="min-w-0"><p class="truncate text-sm font-semibold text-slate-800">{{ tenant.displayName }}</p><p class="mt-1 truncate font-mono text-xs text-slate-600">{{ tenant.slug }} · {{ tenant.quota.maxRepositories }} repos / {{ tenant.quota.maxServices }} services</p></div>
            <UBadge :color="statusColor(tenant.status)" variant="subtle">{{ statusLabel(tenant.status) }}</UBadge>
          </div>
        </div>
      </UCard>
    </section>

    <section id="credentials">
      <UCard>
        <template #header><div class="flex items-start justify-between gap-4"><div><h3 class="font-semibold text-slate-950">全局凭据</h3><p class="mt-1 text-xs text-slate-600">平台拥有、租户可选择；secret 只在创建或轮换请求中出现。</p></div><UBadge color="neutral" variant="subtle">{{ credentials.length }} 条</UBadge></div></template>
        <div v-if="credentialQuery.isError.value" class="p-5 text-sm text-red-700">全局凭据目录暂时不可用，请稍后重试。</div>
        <div v-else-if="credentials.length === 0" class="p-5 text-sm text-slate-600">暂无全局凭据。</div>
        <div v-else class="divide-y divide-slate-100">
          <div v-for="credential in credentials" :key="credential.id" class="flex flex-col gap-2 py-3 sm:flex-row sm:items-center sm:justify-between">
            <div class="flex min-w-0 items-center gap-3"><div class="flex size-9 shrink-0 items-center justify-center rounded-md bg-slate-100 text-slate-600"><UIcon :name="credential.kind === 'ssh_key' ? 'i-lucide-key-round' : 'i-lucide-shield-check'" class="size-4" /></div><div class="min-w-0"><p class="truncate text-sm font-semibold text-slate-800">{{ credential.name }}</p><p class="mt-1 truncate font-mono text-xs text-slate-600">{{ credential.fingerprint }}</p></div></div>
            <span class="text-xs text-slate-600 sm:text-right">更新于 {{ formatDate(credential.updatedAt) }}</span>
          </div>
        </div>
      </UCard>
    </section>

    <section id="jobs" class="grid gap-6 xl:grid-cols-[1.1fr_0.9fr]">
      <UCard>
        <template #header><div><h3 class="font-semibold text-slate-950">平台任务</h3><p class="mt-1 text-xs text-slate-600">跨租户任务仅显示脱敏投影，不包含 input、result、error 或阶段日志。</p></div></template>
        <div v-if="jobQuery.isError.value" class="p-5 text-sm text-red-700">平台任务暂时不可用，请稍后重试。</div>
        <div v-else-if="jobs.length === 0" class="p-5 text-sm text-slate-600">暂无平台任务。</div>
        <div v-else class="divide-y divide-slate-100">
          <div v-for="job in jobs.slice(0, 8)" :key="job.id" class="flex items-center justify-between gap-4 py-3 first:pt-0 last:pb-0"><div class="min-w-0"><p class="truncate text-sm font-semibold text-slate-800">{{ job.type }} · {{ job.tenantSlug }}</p><p class="mt-1 truncate font-mono text-xs text-slate-600">{{ job.id }}</p></div><UBadge :color="statusColor(job.status)" variant="subtle">{{ statusLabel(job.status) }}</UBadge></div>
        </div>
      </UCard>

      <UCard id="audit">
        <template #header><div><h3 class="font-semibold text-slate-950">最近审计</h3><p class="mt-1 text-xs text-slate-600">只展示受控元数据和 tenant slug。</p></div></template>
        <div v-if="auditQuery.isError.value" class="p-5 text-sm text-red-700">审计目录暂时不可用，请稍后重试。</div>
        <div v-else-if="audits.length === 0" class="p-5 text-sm text-slate-600">暂无审计事件。</div>
        <div v-else class="divide-y divide-slate-100">
          <div v-for="entry in audits.slice(0, 8)" :key="entry.id" class="py-3 first:pt-0 last:pb-0"><div class="flex items-center justify-between gap-3"><p class="truncate text-sm font-semibold text-slate-800">{{ entry.action }}</p><span class="shrink-0 text-xs text-slate-600">{{ formatDate(entry.createdAt) }}</span></div><p class="mt-1 truncate text-xs text-slate-600">{{ entry.tenantSlug ?? 'platform' }} · {{ entry.resourceType }}{{ entry.resourceId ? ` · ${entry.resourceId}` : '' }}</p></div>
        </div>
      </UCard>
    </section>
  </div>
</template>
