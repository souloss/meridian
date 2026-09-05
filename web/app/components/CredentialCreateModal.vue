<script setup lang="ts">
import { createCredential } from '~/api/generated/tenant/tenant'
import type { CredentialCreateRequest } from '~/api/generated/models/credentialCreateRequest'
import { MeridianApiError } from '~/api/fetcher'

const props = defineProps<{
  open: boolean
  tenantSlug: string
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  created: []
}>()

type CredentialKind = 'ssh_key' | 'http_token'
type SharedScope = 'private' | 'team' | 'tenant'

const form = reactive({
  name: '',
  kind: 'ssh_key' as CredentialKind,
  privateKeyPem: '',
  passphrase: '',
  username: '',
  token: '',
  sharedScope: 'private' as SharedScope,
  teamIds: ''
})
const busy = ref(false)
const errorMessage = ref('')

const kindItems = [
  { label: 'SSH 私钥', value: 'ssh_key' },
  { label: 'HTTP Token', value: 'http_token' }
]
const scopeItems = [
  { label: '仅创建者', value: 'private' },
  { label: '指定团队', value: 'team' },
  { label: '整个租户', value: 'tenant' }
]

function reset() {
  form.name = ''
  form.kind = 'ssh_key'
  form.privateKeyPem = ''
  form.passphrase = ''
  form.username = ''
  form.token = ''
  form.sharedScope = 'private'
  form.teamIds = ''
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

function parsedTeamIds(): string[] {
  return form.teamIds.split(/[\s,]+/).map((value) => value.trim()).filter(Boolean)
}

function isUUID(value: string): boolean {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value)
}

async function submit() {
  errorMessage.value = ''
  const name = form.name.trim()
  if (!name) {
    errorMessage.value = '请填写凭据名称。'
    return
  }
  const teamIds = parsedTeamIds()
  if (form.sharedScope === 'team' && (teamIds.length === 0 || teamIds.some((id) => !isUUID(id)))) {
    errorMessage.value = '团队共享需要填写有效的团队 UUID。'
    return
  }
  if (form.sharedScope !== 'team' && teamIds.length > 0) {
    errorMessage.value = '只有指定团队范围可以填写团队 UUID。'
    return
  }

  let body: CredentialCreateRequest
  if (form.kind === 'ssh_key') {
    if (form.privateKeyPem.trim().length < 32) {
      errorMessage.value = 'SSH 私钥至少需要 32 个字符。'
      return
    }
    body = {
      name,
      kind: 'ssh_key',
      sshKey: { privateKeyPem: form.privateKeyPem, passphrase: form.passphrase || null },
      sharedScope: form.sharedScope,
      ...(teamIds.length > 0 ? { teamIds } : {})
    }
  } else {
    if (!form.username.trim() || form.token.length < 8) {
      errorMessage.value = 'HTTP Token 需要用户名和至少 8 个字符的 Token。'
      return
    }
    body = {
      name,
      kind: 'http_token',
      httpToken: { username: form.username.trim(), token: form.token },
      sharedScope: form.sharedScope,
      ...(teamIds.length > 0 ? { teamIds } : {})
    }
  }

  busy.value = true
  try {
    await createCredential(props.tenantSlug, body, { headers: { 'Idempotency-Key': crypto.randomUUID() } })
    emit('created')
    busy.value = false
    onOpenChange(false)
  } catch (error) {
    if (error instanceof MeridianApiError && error.status === 422) {
      errorMessage.value = '凭据内容或共享范围未通过校验。'
    } else {
      errorMessage.value = '凭据创建失败，请稍后重试。'
    }
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <UModal
    :open="props.open"
    title="创建凭据"
    description="秘密只在创建请求中发送，控制面永远不回显原文。"
    scrollable
    :ui="{ content: 'sm:max-w-xl' }"
    @update:open="onOpenChange"
  >
    <template #body>
      <form id="credential-create-form" class="space-y-5" @submit.prevent="submit">
        <UAlert v-if="errorMessage" color="error" variant="subtle" icon="i-lucide-circle-alert" :title="errorMessage" />
        <div class="grid gap-5 sm:grid-cols-2">
          <UFormField label="凭据名称" name="credential-name" required>
            <UInput id="credential-name" v-model="form.name" class="w-full" maxlength="64" autocomplete="off" required />
          </UFormField>
          <UFormField label="凭据类型" name="credential-kind" required>
            <USelect id="credential-kind" v-model="form.kind" :items="kindItems" aria-label="凭据类型" class="w-full" />
          </UFormField>
        </div>
        <template v-if="form.kind === 'ssh_key'">
          <UFormField label="SSH 私钥" name="credential-private-key" hint="至少 32 个字符" required>
            <UTextarea id="credential-private-key" v-model="form.privateKeyPem" class="w-full font-mono" :rows="7" autocomplete="off" required />
          </UFormField>
          <UFormField label="私钥口令" name="credential-passphrase" hint="可选">
            <UInput id="credential-passphrase" v-model="form.passphrase" class="w-full" type="password" autocomplete="new-password" />
          </UFormField>
        </template>
        <template v-else>
          <div class="grid gap-5 sm:grid-cols-2">
            <UFormField label="用户名" name="credential-username" required>
              <UInput id="credential-username" v-model="form.username" class="w-full" autocomplete="username" required />
            </UFormField>
            <UFormField label="Token" name="credential-token" required>
              <UInput id="credential-token" v-model="form.token" class="w-full" type="password" minlength="8" autocomplete="new-password" required />
            </UFormField>
          </div>
        </template>
        <UFormField label="共享范围" name="credential-scope" required>
          <USelect id="credential-scope" v-model="form.sharedScope" :items="scopeItems" aria-label="共享范围" class="w-full" />
        </UFormField>
        <UFormField v-if="form.sharedScope === 'team'" label="团队 UUID" name="credential-team-ids" hint="多个 UUID 用空格或换行分隔" required>
          <UTextarea id="credential-team-ids" v-model="form.teamIds" class="w-full font-mono" :rows="3" autocomplete="off" required />
        </UFormField>
      </form>
    </template>
    <template #footer>
      <div class="flex justify-end gap-3">
        <UButton color="neutral" variant="outline" label="取消" :disabled="busy" @click="close" />
        <UButton form="credential-create-form" type="submit" color="primary" label="创建凭据" :loading="busy" />
      </div>
    </template>
  </UModal>
</template>
