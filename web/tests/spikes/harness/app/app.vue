<script setup lang="ts">
import { basicSetup, EditorView } from 'codemirror'
import { EditorState } from '@codemirror/state'
import { json } from '@codemirror/lang-json'
import cytoscape, { type Core } from 'cytoscape'
import { computed, h, nextTick, onBeforeUnmount, onMounted, ref, resolveComponent } from 'vue'
import type { TableColumn } from '@nuxt/ui'

type TableRow = { id: number; service: string; kind: string; owner: string }
type SpikeState = Record<string, any>

const mode = ref('table-graph')
const graphHost = ref<HTMLElement | null>(null)
const smallEditorHost = ref<HTMLElement | null>(null)
const mediumEditorHost = ref<HTMLElement | null>(null)
const largeEditorHost = ref<HTMLElement | null>(null)
const actionStatus = ref('')
const preview = ref('')
const tableRows = Array.from({ length: 10_000 }, (_, index): TableRow => ({
  id: index + 1,
  service: `service-${String(index + 1).padStart(5, '0')}`,
  kind: index % 2 === 0 ? 'openapi' : 'asyncapi',
  owner: `team-${String(index % 20).padStart(2, '0')}`
}))
const UButton = resolveComponent('UButton')
const tableColumns: TableColumn<TableRow>[] = [
  { accessorKey: 'id', header: 'ID' },
  { accessorKey: 'service', header: '服务' },
  { accessorKey: 'kind', header: '类型' },
  { accessorKey: 'owner', header: '负责人' },
  {
    id: 'action',
    header: '操作',
    cell: ({ row }) => h(UButton, {
      color: 'neutral',
      variant: 'outline',
      size: 'xs',
      'aria-label': `打开 ${row.original.service}`,
      onClick: () => { actionStatus.value = `已打开 ${row.original.service}` }
    }, () => '打开')
  }
]
const splitterItems = [
  { id: 'source', slot: 'source', defaultSize: 50, minSize: 20 },
  { id: 'preview', slot: 'preview', defaultSize: 50, minSize: 20 }
]
const showTableGraph = computed(() => mode.value === 'table-graph' || mode.value === 'a11y')
const showEditors = computed(() => mode.value === 'editor' || mode.value === 'a11y')
const showA11y = computed(() => mode.value === 'a11y')
const editorViews: EditorView[] = []
let graph: Core | undefined
let previewTimer: ReturnType<typeof setTimeout> | undefined
let longTaskObserver: PerformanceObserver | undefined

function exactJSON(bytes: number) {
  const prefix = '{"data":"'
  const suffix = '"}'
  return prefix + 'x'.repeat(bytes - prefix.length - suffix.length) + suffix
}

function installEditor(host: HTMLElement, bytes: number, editable: boolean, label: string) {
  const extensions = editable
    ? [basicSetup, json(), EditorView.lineWrapping, EditorView.contentAttributes.of({ 'aria-label': label }), EditorView.updateListener.of((update) => {
        if (!update.docChanged) return
        clearTimeout(previewTimer)
        previewTimer = setTimeout(() => {
          preview.value = update.state.doc.sliceString(0, 80)
          window.__spike.editor.previewCount += 1
        }, 800)
      })]
    : [EditorState.readOnly.of(true), EditorView.editable.of(false), EditorView.lineWrapping, EditorView.contentAttributes.of({ 'aria-label': label })]
  const view = new EditorView({ state: EditorState.create({ doc: exactJSON(bytes), extensions }), parent: host })
  view.scrollDOM.tabIndex = 0
  view.scrollDOM.setAttribute('aria-label', `${label}滚动区域`)
  editorViews.push(view)
  return view
}

async function installGraph() {
  if (!graphHost.value) return
  const elements = [
    ...Array.from({ length: 500 }, (_, index) => ({ data: { id: `n${index}`, label: `Service ${index + 1}` } })),
    ...Array.from({ length: 5000 }, (_, index) => {
      const source = index % 500
      const target = (source + 1 + Math.floor(index / 500) * 37) % 500
      return { data: { id: `e${index}`, source: `n${source}`, target: `n${target}` } }
    })
  ]
  const started = performance.now()
  graph = cytoscape({
    container: graphHost.value,
    elements,
    pixelRatio: 1,
    hideEdgesOnViewport: true,
    textureOnViewport: true,
    style: [
      { selector: 'node', style: { width: 14, height: 14, 'background-color': '#006b5f', label: '' } },
      { selector: 'edge', style: { width: 1, 'line-color': '#9aa6b2', opacity: 0.35, label: '' } },
      { selector: ':selected', style: { 'background-color': '#b42318', 'line-color': '#b42318' } }
    ]
  })
  await new Promise<void>((resolve) => {
    graph!.one('layoutstop', () => requestAnimationFrame(() => requestAnimationFrame(() => resolve())))
    graph!.layout({ name: 'breadthfirst', directed: true, animate: false, fit: true, padding: 24 }).run()
  })
  const first = graph.$id('n0')
  first.select()
  window.__spike.graph = {
    ready: true,
    nodes: graph.nodes().length,
    edges: graph.edges().length,
    firstPaintMs: performance.now() - started,
    selected: first.selected(),
    neighborhood: first.neighborhood().length,
    destroyed: false
  }
}

onMounted(async () => {
  mode.value = new URLSearchParams(window.location.search).get('mode') || 'table-graph'
  window.__spike = {
    mode: mode.value,
    table: { ready: false, rows: tableRows.length, estimateSize: 44, overscan: 12 },
    graph: { ready: false },
    editor: { ready: false, previewCount: 0, longTasks: [] as number[] }
  }
  await nextTick()
  if (showTableGraph.value) {
    window.__spike.table.ready = true
    await installGraph()
  }
  if (showEditors.value && smallEditorHost.value && mediumEditorHost.value && largeEditorHost.value) {
    const sizes = mode.value === 'editor'
      ? [1024 * 1024, 5 * 1024 * 1024, 10 * 1024 * 1024] as const
      : [4096, 8192, 16384] as const
    const smallView = installEditor(smallEditorHost.value, sizes[0], true, '可编辑 JSON 文档')
    const mediumView = installEditor(mediumEditorHost.value, sizes[1], false, '只读中型文档')
    const largeView = installEditor(largeEditorHost.value, sizes[2], false, '只读大型文档')
    const views = [smallView, mediumView, largeView]
    longTaskObserver = new PerformanceObserver((list) => {
      window.__spike.editor.longTasks.push(...list.getEntries().map(entry => entry.duration))
    })
    longTaskObserver.observe({ type: 'longtask', buffered: false })
    window.__spike.editor = {
      ...window.__spike.editor,
      ready: true,
      sizes: views.map(view => new TextEncoder().encode(view.state.doc.toString()).byteLength),
      editable: views.map(view => view.state.facet(EditorView.editable)),
      interact: () => {
        window.__spike.editor.longTasks.length = 0
        smallView.dispatch({ changes: { from: 9, to: 10, insert: 'y' } })
        mediumView.scrollDOM.scrollTop = mediumView.scrollDOM.scrollHeight
        largeView.scrollDOM.scrollTop = largeView.scrollDOM.scrollHeight
      }
    }
  }
})

onBeforeUnmount(() => {
  clearTimeout(previewTimer)
  longTaskObserver?.disconnect()
  editorViews.forEach(view => view.destroy())
  graph?.destroy()
  if (window.__spike?.graph) window.__spike.graph.destroyed = true
})

declare global {
  interface Window { __spike: SpikeState }
}
</script>

<template>
  <main class="shell">
    <section v-if="showA11y" class="band login" aria-labelledby="login-heading">
      <h1 id="login-heading">Meridian 登录</h1>
      <label>用户名 <input name="username" autocomplete="username" autofocus></label>
      <label>密码 <input name="password" type="password" autocomplete="current-password"></label>
      <UButton type="button" color="neutral">登录</UButton>
    </section>

    <section v-if="showA11y" class="band" aria-labelledby="viewer-heading">
      <h2 id="viewer-heading">文档查看器</h2>
      <USplitter id="viewer-splitter" :items="splitterItems" class="border border-default">
        <template #source><article class="split-panel"><h3>源文档</h3><p>固定版本 source 内容。</p></article></template>
        <template #preview><article class="split-panel"><h3>预览</h3><p>固定版本渲染结果。</p></article></template>
      </USplitter>
    </section>

    <section v-if="showTableGraph" class="band" aria-labelledby="table-heading">
      <h1 v-if="!showA11y" id="table-heading">大规模目录与依赖图</h1>
      <h2 v-else id="table-heading">服务目录</h2>
      <p class="muted">10,000 条确定性记录</p>
      <div class="table-frame">
        <UTable
          data-testid="table-scroll"
          class="table-scroll"
          :data="tableRows"
          :columns="tableColumns"
          :virtualize="{ estimateSize: 44, overscan: 12 }"
          :watch-options="{ deep: false }"
          sticky="header"
          caption="服务目录，包含明确的逐行操作"
        />
      </div>
      <p class="status" role="status">{{ actionStatus }}</p>
    </section>

    <section v-if="showTableGraph" class="band" aria-labelledby="graph-heading">
      <h2 id="graph-heading">服务依赖图</h2>
      <div ref="graphHost" class="graph" role="img" aria-label="500 个服务节点和 5000 条依赖边的交互图" tabindex="0" />
      <details class="graph-summary">
        <summary>依赖图文本摘要</summary>
        <ul><li v-for="index in 20" :key="index">Service {{ index }}，可从完整图中选择</li></ul>
      </details>
    </section>

    <section v-if="showEditors" class="band" aria-labelledby="editor-heading">
      <h1 v-if="!showA11y" id="editor-heading">大文档编辑策略</h1>
      <h2 v-else id="editor-heading">文档编辑器</h2>
      <div class="editor-grid">
        <article><h3>1 MiB 可编辑 JSON</h3><div ref="smallEditorHost" class="editor-host" data-testid="editor-1m" /></article>
        <article><h3>5 MiB 只读文档</h3><div ref="mediumEditorHost" class="editor-host" data-testid="editor-5m" /></article>
        <article><h3>10 MiB 只读文档</h3><div ref="largeEditorHost" class="editor-host" data-testid="editor-10m" /></article>
      </div>
      <p class="status" role="status">{{ preview ? '预览已更新' : '等待编辑' }}</p>
    </section>
  </main>
</template>
