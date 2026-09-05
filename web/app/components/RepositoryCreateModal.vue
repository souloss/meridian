<script setup lang="ts">
import { createRepository } from '~/api/generated/repository/repository'
import type { Credential } from '~/api/generated/models/credential'
import type { RepositoryCreateRequest } from '~/api/generated/models/repositoryCreateRequest'
import { MeridianApiError } from '~/api/fetcher'

const props = defineProps<{
  open: boolean
  tenantSlug: string
  credentials: Credential[]
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  created: []
}>()

const form = reactive({
  url: '',
  defaultBranch: 'main',
  credentialId: '__public__',
  note: ''
})
const busy = ref(false)
const errorMessage = ref('')

const credentialItems = computed(() => [
  { label: '公开仓库（无需凭据）', value: '__public__' },
  ...props.credentials.map((credential) => ({
    label: `${credential.name} · ${credential.kind === 'ssh_key' ? 'SSH' : 'HTTP token'}`,
    value: credential.id
  }))
])

function reset() {
  form.url = ''
  form.defaultBranch = 'main'
  form.credentialId = '__public__'
  form.note = ''
  errorMessage.value = ''
}

function close() {
  if (!busy.value) emit('update:open', false)
}

function onOpenChange(value: boolean) {
  if (!value && busy.value) return
  if (!value) reset()
  emit('update:open', value)
}

async function submit() {
  errorMessage.value = ''
  const url = form.url.trim()
  const defaultBranch = form.defaultBranch.trim()
  if (!url || !defaultBranch) {
    errorMessage.value = '请填写 Git 地址和默认分支。'
    return
  }
  if (form.note.length > 500) {
    errorMessage.value = '备注不能超过 500 个字符。'
    return
  }

  const body: RepositoryCreateRequest = {
    url,
    defaultBranch,
    credentialId: form.credentialId === '__public__' ? null : form.credentialId,
    note: form.note.trim() || null
  }
  busy.value = true
  try {
    await createRepository(props.tenantSlug, body, { headers: { 'Idempotency-Key': crypto.randomUUID() } })
    emit('created')
    busy.value = false
    onOpenChange(false)
  } catch (error) {
    if (error instanceof MeridianApiError && error.status === 409) {
      errorMessage.value = '当前租户已达到仓库配额。'
    } else if (error instanceof MeridianApiError && error.status === 422) {
      errorMessage.value = '仓库配置未通过校验，请检查地址和分支。'
    } else {
      errorMessage.value = '仓库创建失败，请稍后重试。'
    }
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <UModal
    :open="props.open"
    title="添加仓库"
    description="填写 Git 连接信息，创建后可在仓库列表中查看同步健康状态。"
    scrollable
    :ui="{ content: 'sm:max-w-xl' }"
    @update:open="onOpenChange"
  >
    <template #body>
      <form id="repository-create-form" class="space-y-5" @submit.prevent="submit">
        <UAlert v-if="errorMessage" color="error" variant="subtle" icon="i-lucide-circle-alert" :title="errorMessage" />
        <UFormField label="Git 远程地址" name="repository-url" required>
          <UInput id="repository-url" v-model="form.url" class="w-full" placeholder="https://git.example.com/team/service.git" autocomplete="url" required />
        </UFormField>
        <div class="grid gap-5 sm:grid-cols-2">
          <UFormField label="默认分支" name="repository-default-branch" required>
            <UInput id="repository-default-branch" v-model="form.defaultBranch" class="w-full" placeholder="main" required />
          </UFormField>
          <UFormField label="访问凭据" name="repository-credential">
            <USelect id="repository-credential" v-model="form.credentialId" :items="credentialItems" aria-label="访问凭据" class="w-full" />
          </UFormField>
        </div>
        <UFormField label="备注" name="repository-note" hint="最多 500 个字符">
          <UTextarea id="repository-note" v-model="form.note" class="w-full" :rows="3" maxlength="500" placeholder="可选，记录连接用途或维护说明。" />
        </UFormField>
      </form>
    </template>
    <template #footer>
      <div class="flex justify-end gap-3">
        <UButton color="neutral" variant="outline" label="取消" :disabled="busy" @click="close" />
        <UButton form="repository-create-form" type="submit" color="primary" label="创建仓库" :loading="busy" />
      </div>
    </template>
  </UModal>
</template>
