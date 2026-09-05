<script setup lang="ts">
const props = defineProps<{
  open: boolean
  title: string
  description: string
  confirmLabel?: string
  busy?: boolean
  errorMessage?: string
}>()

const emit = defineEmits<{
  'update:open': [value: boolean]
  confirm: []
}>()
</script>

<template>
  <UModal
    :open="props.open"
    :title="props.title"
    :description="props.description"
    :dismissible="!props.busy"
    scrollable
    :ui="{ content: 'sm:max-w-md' }"
    @update:open="emit('update:open', $event)"
  >
    <template #body>
      <UAlert v-if="props.errorMessage" color="error" variant="subtle" icon="i-lucide-circle-alert" :title="props.errorMessage" />
    </template>
    <template #footer>
      <div class="flex justify-end gap-3">
        <UButton color="neutral" variant="outline" label="取消" :disabled="props.busy" @click="emit('update:open', false)" />
        <UButton color="error" :label="props.confirmLabel ?? '确认删除'" :loading="props.busy" @click="emit('confirm')" />
      </div>
    </template>
  </UModal>
</template>
