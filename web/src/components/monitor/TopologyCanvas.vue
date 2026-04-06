<script setup lang="ts">
import { ref, onMounted, computed, reactive } from 'vue'
import NodeCard from './NodeCard.vue'
import type { ProbeNode, ProbeLink } from '@/stores/monitor'

const props = defineProps<{
  nodes: ProbeNode[]
  links: ProbeLink[]
}>()

const emit = defineEmits<{
  (e: 'update-position', nodeId: string, x: number, y: number): void
  (e: 'click-link', linkId: string): void
  (e: 'create-link', sourceId: string, targetId: string): void
  (e: 'delete-link', linkId: string): void
  (e: 'update-role', nodeId: string, role: string): void
}>()

const containerRef = ref<HTMLElement | null>(null)

// Canvas transform (pan + zoom)
const transform = reactive({ x: 0, y: 0, scale: 1 })

// Node positions
const nodePositions = ref<Map<string, { x: number; y: number }>>(new Map())

// Drag state
const dragging = ref<{ nodeId: string; offsetX: number; offsetY: number } | null>(null)
const panning = ref<{ startX: number; startY: number; origX: number; origY: number } | null>(null)

// Link creation drag
const linkDrag = ref<{ sourceId: string; mouseX: number; mouseY: number } | null>(null)

// Context menu
const contextMenu = ref<{ x: number; y: number; type: 'node' | 'link'; id: string } | null>(null)

onMounted(() => {
  let col = 0
  for (const node of props.nodes) {
    const x = node.canvas_x ?? 100 + (col % 4) * 220
    const y = node.canvas_y ?? 100 + Math.floor(col / 4) * 160
    nodePositions.value.set(node.id, { x, y })
    col++
  }
})

function getPos(nodeId: string) {
  return nodePositions.value.get(nodeId) || { x: 0, y: 0 }
}

// --- Mouse handlers ---

function onNodeMouseDown(nodeId: string, event: MouseEvent) {
  if (event.button === 2) return // right-click handled separately
  event.stopPropagation()

  if (event.shiftKey) {
    // Shift+drag = create link
    if (!containerRef.value) return
    const rect = containerRef.value.getBoundingClientRect()
    linkDrag.value = {
      sourceId: nodeId,
      mouseX: (event.clientX - rect.left - transform.x) / transform.scale,
      mouseY: (event.clientY - rect.top - transform.y) / transform.scale,
    }
    return
  }

  const pos = getPos(nodeId)
  dragging.value = {
    nodeId,
    offsetX: (event.clientX - transform.x) / transform.scale - pos.x,
    offsetY: (event.clientY - transform.y) / transform.scale - pos.y,
  }
}

function onCanvasMouseDown(event: MouseEvent) {
  if (event.button !== 0) return
  contextMenu.value = null
  panning.value = {
    startX: event.clientX,
    startY: event.clientY,
    origX: transform.x,
    origY: transform.y,
  }
}

function onMouseMove(event: MouseEvent) {
  const rect = containerRef.value?.getBoundingClientRect()
  if (!rect) return

  if (dragging.value) {
    const x = (event.clientX - transform.x) / transform.scale - dragging.value.offsetX
    const y = (event.clientY - transform.y) / transform.scale - dragging.value.offsetY
    nodePositions.value.set(dragging.value.nodeId, { x, y })
  } else if (panning.value) {
    transform.x = panning.value.origX + (event.clientX - panning.value.startX)
    transform.y = panning.value.origY + (event.clientY - panning.value.startY)
  } else if (linkDrag.value) {
    linkDrag.value.mouseX = (event.clientX - rect.left - transform.x) / transform.scale
    linkDrag.value.mouseY = (event.clientY - rect.top - transform.y) / transform.scale
  }
}

function onMouseUp(event: MouseEvent) {
  if (dragging.value) {
    const pos = getPos(dragging.value.nodeId)
    emit('update-position', dragging.value.nodeId, pos.x, pos.y)
    dragging.value = null
  }
  if (panning.value) {
    panning.value = null
  }
  if (linkDrag.value && containerRef.value) {
    // Check if mouse is over a node
    const rect = containerRef.value.getBoundingClientRect()
    const mx = (event.clientX - rect.left - transform.x) / transform.scale
    const my = (event.clientY - rect.top - transform.y) / transform.scale
    for (const node of props.nodes) {
      const pos = getPos(node.id)
      if (node.id !== linkDrag.value.sourceId &&
          mx >= pos.x && mx <= pos.x + 140 &&
          my >= pos.y && my <= pos.y + 40) {
        emit('create-link', linkDrag.value.sourceId, node.id)
        break
      }
    }
    linkDrag.value = null
  }
}

function onWheel(event: WheelEvent) {
  event.preventDefault()
  if (!containerRef.value) return
  const delta = event.deltaY > 0 ? 0.9 : 1.1
  const newScale = Math.max(0.3, Math.min(3, transform.scale * delta))

  const rect = containerRef.value.getBoundingClientRect()
  const mx = event.clientX - rect.left
  const my = event.clientY - rect.top

  transform.x = mx - (mx - transform.x) * (newScale / transform.scale)
  transform.y = my - (my - transform.y) * (newScale / transform.scale)
  transform.scale = newScale
}

// --- Context menu ---

function onNodeContextMenu(nodeId: string, event: MouseEvent) {
  event.preventDefault()
  event.stopPropagation()
  contextMenu.value = { x: event.clientX, y: event.clientY, type: 'node', id: nodeId }
}

function onLinkContextMenu(linkId: string, event: MouseEvent) {
  event.preventDefault()
  event.stopPropagation()
  contextMenu.value = { x: event.clientX, y: event.clientY, type: 'link', id: linkId }
}

function setRole(role: string) {
  if (contextMenu.value?.type === 'node') {
    emit('update-role', contextMenu.value.id, role)
  }
  contextMenu.value = null
}

function deleteLink() {
  if (contextMenu.value?.type === 'link') {
    emit('delete-link', contextMenu.value.id)
  }
  contextMenu.value = null
}

function viewLinkDetail() {
  if (contextMenu.value?.type === 'link') {
    emit('click-link', contextMenu.value.id)
  }
  contextMenu.value = null
}

// --- Computed ---

function linkColor(link: ProbeLink) {
  const m = link.latest_metric
  if (!m) return '#6b7280'
  if (m.packet_loss && m.packet_loss > 5) return '#ef4444'
  if (m.latency_avg && m.latency_avg > 200) return '#f59e0b'
  return '#4ade80'
}

function linkLabel(link: ProbeLink) {
  const m = link.latest_metric
  if (!m) return 'no data'
  const parts: string[] = []
  if (m.latency_avg != null) parts.push(`${m.latency_avg.toFixed(0)}ms`)
  if (m.packet_loss != null) parts.push(`${m.packet_loss.toFixed(1)}%`)
  return parts.join(' | ') || 'no data'
}

function linkTooltip(link: ProbeLink) {
  const m = link.latest_metric
  if (!m) return 'No data'
  const lines: string[] = []
  if (m.latency_avg != null) lines.push(`Latency: ${m.latency_min?.toFixed(1)}/${m.latency_avg.toFixed(1)}/${m.latency_max?.toFixed(1)} ms`)
  if (m.packet_loss != null) lines.push(`Packet Loss: ${m.packet_loss.toFixed(1)}%`)
  if (m.tcp_connect_time != null) lines.push(`TCP: ${m.tcp_connect_time.toFixed(1)} ms`)
  if (m.bandwidth_mbps != null) lines.push(`Bandwidth: ${m.bandwidth_mbps.toFixed(1)} Mbps`)
  return lines.join('\n')
}

const svgLines = computed(() => {
  return props.links.map((link) => {
    const from = getPos(link.source_id)
    const to = getPos(link.target_id)
    return {
      link,
      x1: from.x + 70, y1: from.y + 20,
      x2: to.x + 70, y2: to.y + 20,
      mx: (from.x + to.x) / 2 + 70,
      my: (from.y + to.y) / 2 + 20,
    }
  })
})
</script>

<template>
  <div
    ref="containerRef"
    class="relative h-full w-full overflow-hidden select-none"
    style="background-color: var(--background)"
    @mousedown="onCanvasMouseDown"
    @mousemove="onMouseMove"
    @mouseup="onMouseUp"
    @mouseleave="onMouseUp"
    @wheel.prevent="onWheel"
    @click="contextMenu = null"
  >
    <!-- Transform container -->
    <div :style="{ transform: `translate(${transform.x}px, ${transform.y}px) scale(${transform.scale})`, transformOrigin: '0 0' }">
      <!-- SVG layer for links -->
      <svg class="absolute inset-0 pointer-events-none" style="width: 4000px; height: 4000px; overflow: visible">
        <!-- Link lines -->
        <g v-for="line in svgLines" :key="line.link.id">
          <line
            :x1="line.x1" :y1="line.y1"
            :x2="line.x2" :y2="line.y2"
            :stroke="linkColor(line.link)"
            stroke-width="2"
            marker-end="url(#arrow)"
          />
          <!-- Clickable label -->
          <text
            :x="line.mx" :y="line.my - 8"
            text-anchor="middle"
            fill="var(--muted-foreground)"
            font-size="11"
            class="pointer-events-auto cursor-pointer"
            @click.stop="emit('click-link', line.link.id)"
            @contextmenu="onLinkContextMenu(line.link.id, $event)"
          >
            <title>{{ linkTooltip(line.link) }}</title>
            {{ linkLabel(line.link) }}
          </text>
        </g>

        <!-- Link creation drag line -->
        <line
          v-if="linkDrag"
          :x1="getPos(linkDrag.sourceId).x + 70"
          :y1="getPos(linkDrag.sourceId).y + 20"
          :x2="linkDrag.mouseX"
          :y2="linkDrag.mouseY"
          stroke="var(--primary)"
          stroke-width="2"
          stroke-dasharray="6,4"
        />

        <!-- Arrow marker -->
        <defs>
          <marker id="arrow" markerWidth="8" markerHeight="6" refX="8" refY="3" orient="auto">
            <path d="M0,0 L8,3 L0,6" fill="var(--muted-foreground)" />
          </marker>
        </defs>
      </svg>

      <!-- Node cards -->
      <div
        v-for="node in nodes"
        :key="node.id"
        class="absolute"
        :style="{ left: getPos(node.id).x + 'px', top: getPos(node.id).y + 'px' }"
        @mousedown="onNodeMouseDown(node.id, $event)"
        @contextmenu="onNodeContextMenu(node.id, $event)"
      >
        <NodeCard :node="node" />
      </div>
    </div>

    <!-- Zoom indicator -->
    <div
      class="absolute bottom-3 left-3 rounded px-2 py-1 text-xs"
      style="background-color: var(--card); color: var(--muted-foreground)"
    >
      {{ (transform.scale * 100).toFixed(0) }}%
    </div>

    <!-- Help hint -->
    <div
      class="absolute bottom-3 right-3 rounded px-2 py-1 text-xs"
      style="background-color: var(--card); color: var(--muted-foreground)"
    >
      Drag: move node | Shift+Drag: create link | Scroll: zoom | Right-click: menu
    </div>

    <!-- Context menu -->
    <div
      v-if="contextMenu"
      class="fixed z-50 rounded-lg border py-1 shadow-lg"
      :style="{ left: contextMenu.x + 'px', top: contextMenu.y + 'px', backgroundColor: 'var(--card)', borderColor: 'var(--border)' }"
    >
      <template v-if="contextMenu.type === 'node'">
        <button
          class="block w-full px-4 py-1.5 text-left text-sm hover:bg-[var(--secondary)]"
          @click="setRole('entry')"
        >Set Role: Entry</button>
        <button
          class="block w-full px-4 py-1.5 text-left text-sm hover:bg-[var(--secondary)]"
          @click="setRole('relay')"
        >Set Role: Relay</button>
        <button
          class="block w-full px-4 py-1.5 text-left text-sm hover:bg-[var(--secondary)]"
          @click="setRole('landing')"
        >Set Role: Landing</button>
      </template>
      <template v-if="contextMenu.type === 'link'">
        <button
          class="block w-full px-4 py-1.5 text-left text-sm hover:bg-[var(--secondary)]"
          @click="viewLinkDetail"
        >View Details</button>
        <button
          class="block w-full px-4 py-1.5 text-left text-sm hover:bg-[var(--secondary)]"
          style="color: var(--color-error-foreground)"
          @click="deleteLink"
        >Delete Link</button>
      </template>
    </div>
  </div>
</template>
