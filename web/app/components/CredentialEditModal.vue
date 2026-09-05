<script setup lang="ts">
import { updateCredential } from '~/api/generated/tenant/tenant'
import type { Credential } from '~/api/generated/models/credential'
import type { CredentialPatchRequest } from '~/api/generated/models/credentialPatchRequest'
import { MeridianApiError } from '~/api/fetcher'

const props = defineProps<{ open: boolean; tenantSlug: string; credential?: Credential }>()
const emit = defineEmits<{ 'update:open': [value: boolean]; updated: [] }>()

type SharedScope = 'private' | 'team' | 'tenant'
const form = reactive({ name: '', sharedScope: 'private' as SharedScope, teamIds: '' })
const busy = ref(false)
const errorMessage = ref('')
const scopeItems = [
  { label: '仅创建者', value: 'private' },
  { label: '指定团队', value: 'team' },
  { label: '整个租户', value: 'tenant' }
]

watch(() => props.credential, (credential) => {
  if (!credential) return
  form.name = credential.name
  form.sharedScope = credential.sharedScope
  form.teamIds = credential.teamIds.join('\n')
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

function isUUID(value: string): boolean {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value)
}

async function submit() {
  if (!props.credential) return
  errorMessage.value = ''
  const name = form.name.trim()
  const teamIds = form.teamIds.split(/[\s,]+/).map((value) => value.trim()).filter(Boolean)
  if (!name) {
    errorMessage.value = '凭据名称不能为空。'
    return
  }
  if (form.sharedScope === 'team' && (teamIds.length === 0 || teamIds.some((id) => !isUUID(id)))) {
    errorMessage.value = '团队共享需要填写有效的团队 UUID。'
    return
  }
  if (form.sharedScope !== 'team' && teamIds.length > 0) {
    errorMessage.value = '只有指定团队范围可以填写团队 UUID。'
    return
  }
  const body: CredentialPatchRequest = { name, sharedScope: form.sharedScope, teamIds }
  busy.value = true
  try {
    await updateCredential(props.tenantSlug, props.credential.id, body, { headers: { 'If-Match': props.credential.etag } })
    busy.value = false
    emit('updated')
    onOpenChange(false)
  } catch (error) {
    errorMessage.value = error instanceof MeridianApiError && error.status === 412 ? '凭据已被其他操作更新，请关闭后刷新。' : '凭据更新失败，请稍后重试。'
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <UModal :open="props.open" title="编辑凭据" description="仅更新名称和共享范围，凭据 secret 不会回显或被重新发送。" scrollable :ui="{ content: 'sm:max-w-xl' }" @update:open="onOpenChange">
    <template #body>
      <form id="credential-edit-form" class="space-y-5" @submit.prevent="submit">
        <UAlert v-if="errorMessage" color="error" variant="subtle" icon="i-lucide-circle-alert" :title="errorMessage" />
        <UFormField label="凭据名称" name="credential-edit-name" required>
          <UInput id="credential-edit-name" v-model="form.name" class="w-full" maxlength="64" required />
        </UFormField>
        <UFormField label="共享范围" name="credential-edit-scope" required>
          <USelect id="credential-edit-scope" v-model="form.sharedScope" :items="scopeItems" aria-label="共享范围" class="w-full" />
        </UFormField>
        <UFormField v-if="form.sharedScope === 'team'" label="团队 UUID" name="credential-edit-team-ids" hint="多个 UUID 用空格或换行分隔" required>
          <UTextarea id="credential-edit-team-ids" v-model="form.teamIds" class="w-full font-mono" :rows="3" required />
        </UFormField>
      </form>
    </template>
    <template #footer>
      <div class="flex justify-end gap-3">
        <UButton color="neutral" variant="outline" label="取消" :disabled="busy" @click="close" />
        <UButton form="credential-edit-form" type="submit" color="primary" label="保存修改" :loading="busy" />
      </div>
    </template>
  </UModal>
</template>
