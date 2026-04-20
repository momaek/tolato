<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount, watch, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, Terminal as TerminalIcon, Folder, ChevronUp, Trash2, Download, Upload, FolderPlus, RefreshCcw } from 'lucide-vue-next'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'

import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Input } from '@/components/ui/input'

import { useAppStore } from '@/stores/app'
import { useTheme } from '@/composables/useTheme'
import { createTerminalWs } from '@/services/terminalWs'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const { theme } = useTheme()

const nodeId = route.params.nodeId as string

// --- Terminal state ---
const termContainer = ref<HTMLDivElement | null>(null)
let term: Terminal | null = null
let fitAddon: FitAddon | null = null
const ws = createTerminalWs()
const connState = ref(ws.state)
const exitInfo = ref<{ code: number; error?: string } | null>(null)

// --- Files state ---
const filesCwd = ref('/')
const filesEntries = ref<FileEntry[]>([])
const filesLoading = ref(false)
const filesError = ref('')
const pendingFileOps = new Map<string, (res: FileResult) => void>()

interface FileEntry {
  name: string
  size: number
  mode: number
  mod_time: number
  is_dir: boolean
}
interface FileResult {
  ok: boolean
  error?: string
  entries?: FileEntry[]
  data?: string
  stat?: FileEntry
  eof?: boolean
}

// ---------- Terminal helpers ----------

function b64Encode(data: string): string {
  // TextEncoder + btoa-safe path for multibyte input
  const bytes = new TextEncoder().encode(data)
  let bin = ''
  for (const b of bytes) bin += String.fromCharCode(b)
  return btoa(bin)
}

function terminalTheme() {
  const dark = theme.value === 'dark'
  return dark
    ? {
        background: '#0b0b0c',
        foreground: '#e6e6e6',
        cursor: '#e6e6e6',
        selectionBackground: '#3a3d41',
      }
    : {
        background: '#ffffff',
        foreground: '#1a1a1a',
        cursor: '#1a1a1a',
        selectionBackground: '#bfdbfe',
      }
}

function mountTerminal() {
  if (!termContainer.value) return
  term = new Terminal({
    cursorBlink: true,
    fontSize: 13,
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    theme: terminalTheme(),
    convertEol: true,
  })
  fitAddon = new FitAddon()
  term.loadAddon(fitAddon)
  term.loadAddon(new WebLinksAddon())
  term.open(termContainer.value)

  fitAddon.fit()

  term.onData((data) => {
    ws.send({ type: 'input', payload: { data: b64Encode(data) } })
  })

  term.onResize(({ cols, rows }) => {
    ws.send({ type: 'resize', payload: { cols, rows } })
  })
}

function openSession() {
  const token = appStore.token
  if (!token) {
    router.replace('/login')
    return
  }

  ws.on('ready', () => {
    term?.focus()
  })
  ws.on('output', (msg) => {
    const payload = msg.payload as { data: string }
    const bin = atob(payload.data)
    const bytes = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
    term?.write(bytes)
  })
  ws.on('exit', (msg) => {
    const p = (msg.payload as { exit_code: number; error?: string }) || { exit_code: 0 }
    exitInfo.value = { code: p.exit_code, error: p.error }
    term?.writeln(`\r\n\x1b[33m[session exited: code=${p.exit_code}${p.error ? `, error=${p.error}` : ''}]\x1b[0m`)
  })
  ws.on('error', (msg) => {
    const p = msg.payload as { message: string }
    term?.writeln(`\r\n\x1b[31m[server error: ${p.message}]\x1b[0m`)
  })
  ws.on('file_result', (msg) => {
    const p = msg.payload as { req_id: string; result: FileResult }
    const cb = pendingFileOps.get(p.req_id)
    if (cb) {
      cb(p.result)
      pendingFileOps.delete(p.req_id)
    }
  })
  ws.onStateChange((s) => {
    connState.value = s
    if (s === 'authenticated') {
      const cols = term?.cols ?? 80
      const rows = term?.rows ?? 24
      ws.send({ type: 'open', payload: { node_id: nodeId, cols, rows } })
    }
  })

  ws.connect(token)
}

function handleResize() {
  try {
    fitAddon?.fit()
  } catch {
    /* ignore */
  }
}

// ---------- Files helpers ----------

function sendFileOp(op: string, path: string, extra: Partial<{ data: string; mode: number; offset: number; length: number }> = {}): Promise<FileResult> {
  return new Promise((resolve) => {
    const reqId = `f_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 7)}`
    pendingFileOps.set(reqId, resolve)
    ws.send({
      type: 'file_op',
      payload: {
        req_id: reqId,
        node_id: nodeId,
        op,
        path,
        ...extra,
      },
    })
    // Safety timeout.
    setTimeout(() => {
      if (pendingFileOps.has(reqId)) {
        pendingFileOps.delete(reqId)
        resolve({ ok: false, error: 'timeout' })
      }
    }, 30_000)
  })
}

async function loadCwd() {
  filesLoading.value = true
  filesError.value = ''
  const res = await sendFileOp('list', filesCwd.value)
  filesLoading.value = false
  if (!res.ok) {
    filesError.value = res.error || 'failed'
    filesEntries.value = []
    return
  }
  filesEntries.value = (res.entries || []).sort((a, b) => {
    if (a.is_dir !== b.is_dir) return a.is_dir ? -1 : 1
    return a.name.localeCompare(b.name)
  })
}

function joinPath(parent: string, name: string) {
  if (parent.endsWith('/')) return parent + name
  return parent + '/' + name
}

function parentPath(p: string) {
  if (p === '/' || p === '') return '/'
  const trimmed = p.replace(/\/+$/, '')
  const idx = trimmed.lastIndexOf('/')
  if (idx <= 0) return '/'
  return trimmed.slice(0, idx)
}

async function enterEntry(e: FileEntry) {
  if (!e.is_dir) return
  filesCwd.value = joinPath(filesCwd.value, e.name)
  await loadCwd()
}

async function goUp() {
  filesCwd.value = parentPath(filesCwd.value)
  await loadCwd()
}

async function createDir() {
  const name = window.prompt('Directory name:')
  if (!name) return
  const res = await sendFileOp('mkdir', joinPath(filesCwd.value, name))
  if (!res.ok) alert(res.error || 'mkdir failed')
  await loadCwd()
}

async function deleteEntry(e: FileEntry) {
  if (!window.confirm(`Delete ${e.name}?`)) return
  const res = await sendFileOp('delete', joinPath(filesCwd.value, e.name))
  if (!res.ok) alert(res.error || 'delete failed')
  await loadCwd()
}

async function downloadEntry(e: FileEntry) {
  const path = joinPath(filesCwd.value, e.name)
  // Chunked read in 512 KiB blocks.
  const chunks: Uint8Array[] = []
  let offset = 0
  while (true) {
    const res = await sendFileOp('read', path, { offset, length: 512 * 1024 })
    if (!res.ok) {
      alert(res.error || 'read failed')
      return
    }
    const bin = atob(res.data || '')
    const bytes = new Uint8Array(bin.length)
    for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
    if (bytes.length > 0) chunks.push(bytes)
    offset += bytes.length
    if (res.eof || bytes.length === 0) break
  }
  const blob = new Blob(chunks as BlobPart[])
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = e.name
  a.click()
  URL.revokeObjectURL(url)
}

async function uploadFile(file: File) {
  const buf = new Uint8Array(await file.arrayBuffer())
  const chunkSize = 512 * 1024
  for (let offset = 0; offset < buf.length; offset += chunkSize) {
    const slice = buf.slice(offset, offset + chunkSize)
    let bin = ''
    for (const b of slice) bin += String.fromCharCode(b)
    const res = await sendFileOp('write', joinPath(filesCwd.value, file.name), {
      offset,
      data: btoa(bin),
    })
    if (!res.ok) {
      alert(res.error || 'upload failed')
      return
    }
  }
  await loadCwd()
}

function onUploadChange(ev: Event) {
  const input = ev.target as HTMLInputElement
  const f = input.files?.[0]
  if (f) uploadFile(f)
  input.value = ''
}

// ---------- Lifecycle ----------

onMounted(async () => {
  await nextTick()
  mountTerminal()
  openSession()
  window.addEventListener('resize', handleResize)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize)
  ws.send({ type: 'close' })
  ws.close()
  term?.dispose()
})

watch(theme, () => {
  if (term) term.options.theme = terminalTheme()
})

// Load file list lazily the first time the tab is clicked.
const activeTab = ref<'terminal' | 'files'>('terminal')
let filesLoaded = false
watch(activeTab, (tab) => {
  if (tab === 'files' && !filesLoaded) {
    filesLoaded = true
    loadCwd()
  }
  if (tab === 'terminal') {
    nextTick(() => {
      handleResize()
      term?.focus()
    })
  }
})

function formatSize(n: number) {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GB`
}
</script>

<template>
  <div class="flex h-full flex-col" style="background-color: var(--background)">
    <!-- Header -->
    <div class="flex items-center gap-3 border-b px-4 py-3" style="border-color: var(--border)">
      <Button variant="ghost" size="icon" @click="router.push(`/nodes/${nodeId}`)">
        <ArrowLeft class="h-4 w-4" />
      </Button>
      <TerminalIcon class="h-4 w-4" style="color: var(--primary)" />
      <h1 class="text-sm font-semibold">{{ nodeId }}</h1>
      <span class="text-xs" style="color: var(--muted-foreground)">
        {{ connState === 'ready' ? 'connected' : connState }}
      </span>
    </div>

    <Tabs v-model="activeTab" class="flex-1 overflow-hidden flex flex-col">
      <TabsList class="mx-4 mt-2 self-start">
        <TabsTrigger value="terminal" class="gap-1">
          <TerminalIcon class="h-3 w-3" /> Terminal
        </TabsTrigger>
        <TabsTrigger value="files" class="gap-1">
          <Folder class="h-3 w-3" /> Files
        </TabsTrigger>
      </TabsList>

      <!-- Terminal tab -->
      <TabsContent value="terminal" class="flex-1 overflow-hidden p-0 m-0">
        <div class="h-full w-full p-2" :style="{ backgroundColor: terminalTheme().background }">
          <div ref="termContainer" class="h-full w-full" />
        </div>
      </TabsContent>

      <!-- Files tab -->
      <TabsContent value="files" class="flex-1 overflow-auto p-4 m-0">
        <div class="flex items-center gap-2 mb-3">
          <Button variant="outline" size="sm" @click="goUp" :disabled="filesCwd === '/'">
            <ChevronUp class="h-3 w-3" />
          </Button>
          <Input
            v-model="filesCwd"
            class="font-mono text-xs"
            @keydown.enter="loadCwd"
          />
          <Button variant="outline" size="sm" @click="loadCwd">
            <RefreshCcw class="h-3 w-3" />
          </Button>
          <Button variant="outline" size="sm" @click="createDir">
            <FolderPlus class="h-3 w-3" />
          </Button>
          <label class="inline-flex items-center gap-1 border rounded-md px-2 py-1 text-xs cursor-pointer" style="border-color: var(--border)">
            <Upload class="h-3 w-3" />
            <span>Upload</span>
            <input type="file" class="hidden" @change="onUploadChange" />
          </label>
        </div>

        <div v-if="filesError" class="mb-2 text-xs" style="color: var(--destructive)">
          {{ filesError }}
        </div>

        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead>Size</TableHead>
              <TableHead>Modified</TableHead>
              <TableHead class="w-[100px]"></TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow
              v-for="e in filesEntries"
              :key="e.name"
              class="cursor-pointer"
              @dblclick="enterEntry(e)"
            >
              <TableCell class="font-mono text-xs">
                <span v-if="e.is_dir">📁 {{ e.name }}</span>
                <span v-else>📄 {{ e.name }}</span>
              </TableCell>
              <TableCell class="text-xs">{{ e.is_dir ? '-' : formatSize(e.size) }}</TableCell>
              <TableCell class="text-xs">{{ new Date(e.mod_time * 1000).toLocaleString() }}</TableCell>
              <TableCell class="text-right">
                <Button v-if="!e.is_dir" variant="ghost" size="icon" @click.stop="downloadEntry(e)">
                  <Download class="h-3 w-3" />
                </Button>
                <Button variant="ghost" size="icon" @click.stop="deleteEntry(e)">
                  <Trash2 class="h-3 w-3" />
                </Button>
              </TableCell>
            </TableRow>
            <TableRow v-if="filesEntries.length === 0 && !filesLoading">
              <TableCell :colspan="4" class="text-center text-xs" style="color: var(--muted-foreground)">
                (empty)
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </TabsContent>
    </Tabs>
  </div>
</template>

<style>
/* xterm requires the viewport to have explicit height. */
.xterm {
  height: 100%;
}
.xterm-viewport {
  overflow-y: auto;
}
</style>
