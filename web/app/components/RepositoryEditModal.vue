<script setup lang="ts">
import { updateRepository } from '~/api/generated/repository/repository'
import type { Credential } from '~/api/generated/models/credential'
import type { Repository } from '~/api/generated/models/repository'
import type { RepositoryPatchRequest } from '~/api/generated/models/repositoryPatchRequest'
import { MeridianApiError } from '~/api/fetcher'

const props = defineProps<{
  open: boolean
  tenantSlug: string
  repository?: Repository
  credentials: Credential[]
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  updated: []
}>()

const form = reactive({ defaultBranch: '', credentialId: '__public__', note: '' })
const busy = ref(false)
const errorMessage = ref('')
const credentialItems = computed(() => [
  { label: '公开仓库（无需凭据）', value: '__public__' },
  ...props.credentials.map((credential) => ({ label: `${credential.name} · ${credential.kind === 'ssh_key' ? 'SSH' : 'HTTP token'}`, value: credential.id }))
])

watch(() => props.repository, (repository) => {
  if (!repository) return
  form.defaultBranch = repository.defaultBranch
  form.credentialId = repository.credentialId ?? '__public__'
  form.note = repository.note ?? ''
  errorMessage.value = ''
}, { immediate: true })

function close() {
  if (!busy.value) emit('update:open', false)
}

function onOpenChange(value: boolean) {
  if (!value && busy.value) return
  if (!value) errorMessage.value = ''
  emit('update:open', value)
}

async function submit() {
  if (!props.repository) return
  errorMessage.value = ''
  const defaultBranch = form.defaultBranch.trim()
  if (!defaultBranch) {
    errorMessage.value = '默认分支不能为空。'
    return
  }
  if (form.note.length > 500) {
    errorMessage.value = '备注不能超过 500 个字符。'
    return
  }
  const body: RepositoryPatchRequest = {
    defaultBranch,
    credentialId: form.credentialId === '__public__' ? null : form.credentialId,
    note: form.note.trim() || null
  }
  busy.value = true
  try {
    await updateRepository(props.tenantSlug, props.repository.id, body, { headers: { 'If-Match': props.repository.etag } })
    busy.value = false
    emit('updated')
    onOpenChange(false)
  } catch (error) {
    errorMessage.value = error instanceof MeridianApiError && error.status === 412 ? '仓库已被其他操作更新，请关闭后刷新。' : '仓库更新失败，请稍后重试。'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <UModal :open="props.open" title="编辑仓库" description="更新仓库的默认分支、访问凭据和备注。" scrollable :ui="{ content: 'sm:max-w-xl' }" @update:open="onOpenChange">
    <template #body>
      <form id="repository-edit-form" class="space-y-5" @submit.prevent="submit">
        <UAlert v-if="errorMessage" color="error" variant="subtle" icon="i-lucide-circle-alert" :title="errorMessage" />
        <UFormField label="Git 远程地址" name="repository-edit-url">
          <UInput id="repository-edit-url" :model-value="props.repository?.url" class="w-full" disabled />
        </UFormField>
        <div class="grid gap-5 sm:grid-cols-2">
          <UFormField label="默认分支" name="repository-edit-default-branch" required>
            <UInput id="repository-edit-default-branch" v-model="form.defaultBranch" class="w-full" required />
          </UFormField>
          <UFormField label="访问凭据" name="repository-edit-credential">
            <USelect id="repository-edit-credential" v-model="form.credentialId" :items="credentialItems" aria-label="访问凭据" class="w-full" />
          </UFormField>
        </div>
        <UFormField label="备注" name="repository-edit-note" hint="最多 500 个字符">
          <UTextarea id="repository-edit-note" v-model="form.note" class="w-full" :rows="3" maxlength="500" />
        </UFormField>
      </form>
    </template>
    <template #footer>
      <div class="flex justify-end gap-3">
        <UButton color="neutral" variant="outline" label="取消" :disabled="busy" @click="close" />
        <UButton form="repository-edit-form" type="submit" color="primary" label="保存修改" :loading="busy" />
      </div>
    </template>
  </UModal>
</template>
