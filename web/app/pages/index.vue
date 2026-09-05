<script setup lang="ts">
const status = ref<'loading' | 'ready'>('loading')

onMounted(async () => {
  try {
    await $fetch('/healthz')
  } finally {
    status.value = 'ready'
  }
})
</script>

<template>
  <main class="min-h-screen bg-neutral-950 px-6 py-16 text-neutral-100">
    <section class="mx-auto max-w-5xl">
      <div class="mb-12 flex items-center justify-between border-b border-neutral-800 pb-6">
        <div>
          <p class="text-sm font-medium uppercase tracking-[0.18em] text-cyan-400">Meridian</p>
          <h1 class="mt-3 text-4xl font-semibold">Asset control plane</h1>
        </div>
        <UBadge color="primary" variant="subtle">M0 foundation</UBadge>
      </div>

      <div class="grid gap-5 md:grid-cols-3">
        <UCard>
          <template #header><h2 class="font-semibold">Static SPA</h2></template>
          <p class="text-sm text-neutral-400">Nuxt 4 output is ready to be embedded by the Go binary.</p>
        </UCard>
        <UCard>
          <template #header><h2 class="font-semibold">Contract first</h2></template>
          <p class="text-sm text-neutral-400">The API contract is served at <code>/api/v1/openapi.yaml</code>.</p>
        </UCard>
        <UCard>
          <template #header><h2 class="font-semibold">Runtime</h2></template>
          <p class="text-sm text-neutral-400">Backend health: <span class="font-medium text-cyan-300">{{ status }}</span></p>
        </UCard>
      </div>
    </section>
  </main>
</template>
