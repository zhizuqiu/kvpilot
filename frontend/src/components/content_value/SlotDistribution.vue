<script setup>
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NDropdown, NIcon, NTag, useThemeVars } from 'naive-ui'
import Refresh from '@/components/icons/Refresh.vue'
import { computeSlotLayout, slotAtPoint, slotsInRect, SLOT_COUNT } from '@/utils/slot_layout.js'
import { buildHotspotScale, hotspotAlpha, hotspotRatio } from '@/utils/slot_color.js'
import { GetSlotDistribution, ScanSlotDistribution } from 'wailsjs/go/services/browserService.js'

const MAP_HEIGHT = 300

const props = defineProps({
    server: String,
    pause: Boolean,
})

const emit = defineEmits(['migrate'])

const i18n = useI18n()
const themeVars = useThemeVars()

const data = reactive({
    slots: new Array(SLOT_COUNT).fill(0),
    nodes: [],
    scannedAt: 0,
    scanning: false,
    unavailable: [],
})

// macOS 习惯用 Command 框选，其他平台用 Alt。
const isMac = /mac/i.test(`${navigator.platform || ''} ${navigator.userAgent || ''}`)
const modifierKey = isMac ? '⌘ Command' : 'Alt'
const isSelectionModifier = (event) => (isMac ? event.metaKey : event.altKey)

const selection = ref([])
const drag = reactive({ active: false, x1: 0, y1: 0, x2: 0, y2: 0 })
const loading = ref(false)
const canvasEl = ref(null)
const wrapEl = ref(null)
const tooltip = reactive({ show: false, x: 0, y: 0, slot: 0 })

let ownerIndex = new Int32Array(SLOT_COUNT)
let layout = null
let suppressClick = false
let drawFrame = null
let pollTimer = null
let resizeObserver = null

// 颜色表达相对中位数的热点程度：均匀集群整体浅色，只有热点变深。
const hotScale = computed(() => buildHotspotScale(data.slots))

const scannedTime = computed(() => {
    if (!data.scannedAt) return ''
    const date = new Date(data.scannedAt)
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
})

const selected = computed(() => (selection.value.length === 1 ? selection.value[0] : null))

const selectedInfo = computed(() => {
    if (selected.value === null) return null
    const slot = selected.value
    const node = ownerNode(slot)
    return {
        slot,
        addr: node?.addr || '',
        keys: data.slots[slot] || 0,
    }
})

const selectionInfo = computed(() => {
    if (selection.value.length === 0) return null
    const keys = selection.value.reduce((sum, slot) => sum + (data.slots[slot] || 0), 0)
    return { count: selection.value.length, keys }
})

const ownerNode = (slot) => {
    const owner = ownerIndex[slot] || 0
    return owner > 0 ? data.nodes[owner - 1] : null
}

const buildOwnerIndex = () => {
    const index = new Int32Array(SLOT_COUNT)
    data.nodes.forEach((node, nodeIndex) => {
        for (const range of node.slotRanges || []) {
            const start = Math.max(0, range.start)
            const end = Math.min(SLOT_COUNT - 1, range.end)
            for (let slot = start; slot <= end; slot++) {
                index[slot] = nodeIndex + 1
            }
        }
    })
    return index
}

const applyData = (payload) => {
    if (!payload) return
    data.slots = Array.isArray(payload.slots) && payload.slots.length === SLOT_COUNT
        ? payload.slots
        : new Array(SLOT_COUNT).fill(0)
    data.nodes = payload.nodes || []
    data.scannedAt = payload.scannedAt || 0
    data.scanning = payload.scanning === true
    data.unavailable = payload.unavailable || []
    ownerIndex = buildOwnerIndex()
    if (selection.value.length > 0) {
        selection.value = selection.value.filter((slot) => ownerNode(slot))
    }
    nextTick(() => {
        resizeCanvas()
        draw()
    })
}

const isDarkBody = () => {
    const color = themeVars.value.bodyColor || '#ffffff'
    const match = color.replace('#', '')
    if (match.length !== 6) return false
    const r = parseInt(match.slice(0, 2), 16)
    const g = parseInt(match.slice(2, 4), 16)
    const b = parseInt(match.slice(4, 6), 16)
    return (r * 299 + g * 587 + b * 114) / 1000 < 128
}

const fitText = (ctx, text, maxWidth) => {
    if (maxWidth <= 0) return ''
    if (ctx.measureText(text).width <= maxWidth) return text
    let low = 0
    let high = text.length
    while (low < high) {
        const mid = Math.ceil((low + high) / 2)
        if (ctx.measureText(`${text.slice(0, mid)}…`).width <= maxWidth) {
            low = mid
        } else {
            high = mid - 1
        }
    }
    return `${text.slice(0, low)}…`
}

const cellRect = (layoutData, slot) => {
    for (const block of layoutData.blocks) {
        const index = block.slots.indexOf(slot)
        if (index < 0) continue
        const col = index % block.cols
        const row = Math.floor(index / block.cols)
        return {
            x: block.x + col * layoutData.cell,
            y: layoutData.labelHeight + row * layoutData.cell,
            w: layoutData.cell,
            h: layoutData.cell,
        }
    }
    return null
}

const draw = () => {
    const canvas = canvasEl.value
    if (!canvas) return
    let ctx = null
    try {
        ctx = canvas.getContext('2d')
    } catch (e) {
        return
    }
    if (!ctx) return

    const width = wrapEl.value?.clientWidth || 0
    ctx.clearRect(0, 0, width, MAP_HEIGHT)
    layout = computeSlotLayout(data.nodes, width, MAP_HEIGHT)
    if (!layout) return

    const primary = themeVars.value.primaryColor || '#D33A31'
    // 空槽用中性色铺底，红色只表示有数据的槽。
    const empty = themeVars.value.textColor3 || '#999999'
    const labelColor = themeVars.value.textColor3 || '#999999'
    const lineColor = isDarkBody() ? 'rgba(255,255,255,0.3)' : 'rgba(0,0,0,0.25)'
    const emptyAlpha = isDarkBody() ? 0.1 : 0.07
    const cell = layout.cell

    ctx.textBaseline = 'middle'
    ctx.font = '11px ui-monospace, SFMono-Regular, Menlo, monospace'

    for (const block of layout.blocks) {
        ctx.globalAlpha = 1
        ctx.fillStyle = labelColor
        const addr = data.nodes[block.index]?.addr || ''
        ctx.fillText(fitText(ctx, addr, block.width - 4), block.x, layout.labelHeight / 2 + 1)

        block.slots.forEach((slot, index) => {
            const col = index % block.cols
            const row = Math.floor(index / block.cols)
            const count = data.slots[slot] || 0
            let alpha
            if (count === 0) {
                alpha = emptyAlpha
            } else {
                alpha = hotspotAlpha(hotspotRatio(count, hotScale.value))
            }
            ctx.globalAlpha = alpha
            ctx.fillStyle = count > 0 ? primary : empty
            // 单元格边界取整并向外取整，避免小数单元格之间出现条纹缝隙。
            const left = Math.floor(block.x + col * cell)
            const top = Math.floor(layout.labelHeight + row * cell)
            const right = Math.ceil(block.x + (col + 1) * cell)
            const bottom = Math.ceil(layout.labelHeight + (row + 1) * cell)
            ctx.fillRect(left, top, right - left, bottom - top)
        })

        ctx.globalAlpha = 1
        ctx.strokeStyle = lineColor
        ctx.lineWidth = 1
        ctx.strokeRect(
            Math.round(block.x) + 0.5,
            Math.round(layout.labelHeight) + 0.5,
            Math.round(block.width) - 1,
            Math.round(MAP_HEIGHT - layout.labelHeight) - 1
        )
    }

    if (selection.value.length > 0) {
        if (selection.value.length === 1) {
            const rect = cellRect(layout, selection.value[0])
            if (rect) {
                ctx.globalAlpha = 1
                ctx.strokeStyle = isDarkBody() ? '#ffffff' : '#000000'
                ctx.lineWidth = 2
                ctx.strokeRect(rect.x + 1, rect.y + 1, Math.max(1, rect.w - 2), Math.max(1, rect.h - 2))
            }
        } else {
            // 多选统一用半透明遮罩，避免相邻单元格的描边连成一条黑带。
            ctx.globalAlpha = 1
            ctx.fillStyle = isDarkBody() ? 'rgba(255,255,255,0.35)' : 'rgba(0,0,0,0.32)'
            for (const slot of selection.value) {
                const rect = cellRect(layout, slot)
                if (rect) ctx.fillRect(rect.x, rect.y, rect.w, rect.h)
            }
        }
    }

    if (drag.active && layout) {
        const left = Math.min(drag.x1, drag.x2)
        const top = Math.max(layout.labelHeight, Math.min(drag.y1, drag.y2))
        const width = Math.abs(drag.x2 - drag.x1)
        const height = Math.abs(drag.y2 - drag.y1)
        ctx.globalAlpha = 0.15
        ctx.fillStyle = primary
        ctx.fillRect(left, top, width, height)
        ctx.globalAlpha = 1
        ctx.strokeStyle = primary
        ctx.lineWidth = 1
        ctx.setLineDash([3, 2])
        ctx.strokeRect(left + 0.5, top + 0.5, width, height)
        ctx.setLineDash([])
    }
    ctx.globalAlpha = 1
}

const resizeCanvas = () => {
    const canvas = canvasEl.value
    const wrap = wrapEl.value
    if (!canvas || !wrap) return
    const width = Math.max(1, wrap.clientWidth || 0)
    const dpr = window.devicePixelRatio || 1
    canvas.width = Math.round(width * dpr)
    canvas.height = Math.round(MAP_HEIGHT * dpr)
    canvas.style.width = `${width}px`
    canvas.style.height = `${MAP_HEIGHT}px`
    try {
        const ctx = canvas.getContext('2d')
        ctx?.setTransform(dpr, 0, 0, dpr, 0, 0)
    } catch (e) {
        return
    }
    draw()
}

const scheduleDraw = () => {
    if (drawFrame) return
    drawFrame = requestAnimationFrame(() => {
        drawFrame = null
        draw()
    })
}

const onMouseDown = (event) => {
    if (!isSelectionModifier(event) || event.button !== 0) return
    event.preventDefault()
    const y = Math.max(event.offsetY, layout?.labelHeight || 0)
    drag.active = true
    drag.x1 = event.offsetX
    drag.y1 = y
    drag.x2 = event.offsetX
    drag.y2 = y
    tooltip.show = false
}

const finishDrag = () => {
    if (!drag.active) return
    drag.active = false
    selection.value = slotsInRect(layout, drag.x1, drag.y1, drag.x2, drag.y2)
    suppressClick = true
    setTimeout(() => {
        suppressClick = false
    }, 0)
    draw()
}

const onMove = (event) => {
    if (drag.active) {
        drag.x2 = event.offsetX
        drag.y2 = Math.max(event.offsetY, layout?.labelHeight || 0)
        scheduleDraw()
        return
    }
    const slot = slotAtPoint(layout, event.offsetX, event.offsetY)
    if (slot === null) {
        tooltip.show = false
        return
    }
    tooltip.slot = slot
    tooltip.show = true
    const wrap = wrapEl.value
    const width = wrap?.clientWidth || 0
    tooltip.x = Math.min(event.offsetX + 14, Math.max(0, width - 210))
    tooltip.y = Math.min(event.offsetY + 14, Math.max(0, MAP_HEIGHT - 56))
}

const onLeave = () => {
    tooltip.show = false
    finishDrag()
}

const onClick = (event) => {
    if (suppressClick) return
    const slot = slotAtPoint(layout, event.offsetX, event.offsetY)
    if (slot === null) {
        selection.value = []
        draw()
        return
    }
    const current = selection.value
    selection.value = current.length === 1 && current[0] === slot ? [] : [slot]
    draw()
}

const targetOptions = computed(() => {
    if (selected.value === null) return []
    const owner = ownerNode(selected.value)
    return data.nodes
        .filter((node) => node.id !== owner?.id)
        .map((node) => ({ label: node.addr, key: node.id }))
})

const selectTarget = (targetId) => {
    const info = selectedInfo.value
    if (!info) return
    const source = ownerNode(info.slot)
    const target = data.nodes.find((node) => node.id === targetId)
    if (!source || !target) return
    emit('migrate', {
        slot: info.slot,
        nodeId: source.id,
        targetId: target.id,
        targetAddr: target.addr,
        keys: info.keys,
    })
}

const allTargetOptions = computed(() => {
    // 多选可能跨多个源节点，目标节点必须排除所有被选槽位的归属节点。
    const owners = new Set(selection.value.map((slot) => ownerNode(slot)?.id).filter(Boolean))
    return data.nodes
        .filter((node) => !owners.has(node.id))
        .map((node) => ({ label: node.addr, key: node.id }))
})

const selectTargetForSelection = (targetId) => {
    if (!selection.value.length) return
    const target = data.nodes.find((node) => node.id === targetId)
    if (!target) return
    const items = selection.value
        .map((slot) => ({
            slot,
            nodeId: ownerNode(slot)?.id || '',
            keys: data.slots[slot] || 0,
        }))
        .filter((item) => item.nodeId)
    emit('migrate', { items, targetId: target.id, targetAddr: target.addr })
}

const tooltipAddr = computed(() => {
    if (!tooltip.show) return ''
    return ownerNode(tooltip.slot)?.addr || ''
})

const loadCached = async () => {
    const { success, data: payload } = await GetSlotDistribution(props.server)
    if (!success) return
    applyData(payload)
    if (payload?.scanning) {
        schedulePoll()
    }
}

const scan = async () => {
    if (loading.value || data.scanning) return
    loading.value = true
    data.scanning = true
    try {
        const { success, data: payload, msg } = (await ScanSlotDistribution(props.server)) || {}
        if (!success) {
            $message.warning(msg || i18n.t('slot.dist_scan_fail'))
            return
        }
        applyData(payload)
    } finally {
        loading.value = false
        data.scanning = false
        draw()
    }
}

const schedulePoll = () => {
    clearPoll()
    pollTimer = setTimeout(async () => {
        if (props.pause) return
        const { success, data: payload } = await GetSlotDistribution(props.server)
        if (success && payload) {
            applyData(payload)
            if (payload.scanning) {
                schedulePoll()
            }
        }
    }, 1500)
}

const clearPoll = () => {
    if (pollTimer) {
        clearTimeout(pollTimer)
        pollTimer = null
    }
}

watch(selection, () => draw())
watch(() => themeVars.value.primaryColor, () => draw())
watch(() => themeVars.value.bodyColor, () => draw())
watch(
    () => props.pause,
    (paused) => {
        if (paused) {
            clearPoll()
        } else if (data.scanning) {
            schedulePoll()
        }
    }
)

defineExpose({ refresh: scan })

onMounted(() => {
    loadCached()
    resizeCanvas()
    if (typeof ResizeObserver !== 'undefined' && wrapEl.value) {
        resizeObserver = new ResizeObserver(() => resizeCanvas())
        resizeObserver.observe(wrapEl.value)
    }
})

onUnmounted(() => {
    clearPoll()
    resizeObserver?.disconnect()
})
</script>

<template>
    <div class="slot-distribution">
        <div class="slot-dist-head flex-box-h">
            <span class="slot-dist-title">{{ $t('slot.dist_title') }}</span>
            <span v-if="data.scannedAt" class="slot-dist-hint">
                {{ $t('slot.dist_scanned_at', { time: scannedTime }) }}
            </span>
            <span v-else class="slot-dist-hint">{{ $t('slot.dist_never') }}</span>
            <n-tag v-if="data.unavailable.length" size="small" type="warning" :bordered="false">
                {{ $t('slot.dist_partial', { n: data.unavailable.length }) }}
            </n-tag>
            <div class="flex-item-expand"></div>
            <n-button size="small" secondary :loading="loading" :disabled="data.scanning" @click="scan">
                <template #icon>
                    <n-icon :component="Refresh" />
                </template>
                {{ data.scannedAt ? $t('slot.dist_rescan') : $t('slot.dist_scan') }}
            </n-button>
        </div>

        <div class="slot-dist-hint-line">
            {{ $t('slot.dist_hint', { key: modifierKey }) }}
            <template v-if="hotScale.median > 0">
                · {{ $t('slot.dist_hotspot', { n: Math.round(hotScale.median) }) }}
            </template>
        </div>

        <div v-if="!data.scannedAt" class="slot-dist-placeholder">
            {{ data.scanning ? $t('slot.dist_scanning') : $t('slot.dist_empty') }}
        </div>

        <div v-show="data.scannedAt" ref="wrapEl" class="slot-dist-canvas-wrap">
            <canvas
                ref="canvasEl"
                class="slot-dist-canvas"
                @mousedown="onMouseDown"
                @mousemove="onMove"
                @mouseup="finishDrag"
                @mouseleave="onLeave"
                @click="onClick"></canvas>
            <div v-if="data.scanning" class="slot-dist-scanning">
                {{ $t('slot.dist_scanning') }}
            </div>
            <div
                v-if="tooltip.show"
                class="slot-dist-tooltip"
                :style="{ left: tooltip.x + 'px', top: tooltip.y + 'px' }">
                <div class="slot-dist-tooltip-slot">{{ $t('slot.dist_slot', { slot: tooltip.slot }) }}</div>
                <div class="slot-dist-tooltip-line">{{ tooltipAddr || $t('slot.dist_unassigned') }}</div>
                <div class="slot-dist-tooltip-line">
                    {{ $t('slot.dist_keys', { n: data.slots[tooltip.slot] || 0 }) }}
                </div>
            </div>
        </div>

        <div v-if="selectionInfo && selectedInfo" class="slot-dist-detail flex-box-h">
            <span class="slot-dist-detail-text">
                {{ $t('slot.dist_detail', { slot: selectedInfo.slot, addr: selectedInfo.addr, n: selectedInfo.keys }) }}
            </span>
            <n-dropdown
                v-if="selectedInfo.addr && targetOptions.length"
                trigger="click"
                placement="bottom-end"
                :options="targetOptions"
                @select="selectTarget">
                <n-button size="tiny" secondary>{{ $t('slot.dist_migrate') }}</n-button>
            </n-dropdown>
            <n-button v-else size="tiny" secondary disabled>{{ $t('slot.dist_migrate') }}</n-button>
            <div class="flex-item-expand"></div>
            <n-button size="tiny" quaternary @click="selection = []">{{ $t('slot.dist_deselect') }}</n-button>
        </div>

        <div v-else-if="selectionInfo" class="slot-dist-detail flex-box-h">
            <span class="slot-dist-detail-text">
                {{ $t('slot.dist_multi', { n: selectionInfo.count, keys: selectionInfo.keys }) }}
            </span>
            <n-dropdown
                v-if="allTargetOptions.length"
                trigger="click"
                placement="bottom-end"
                :options="allTargetOptions"
                @select="selectTargetForSelection">
                <n-button size="tiny" secondary>
                    {{ $t('slot.dist_migrate_multi', { n: selectionInfo.count }) }}
                </n-button>
            </n-dropdown>
            <n-button v-else size="tiny" secondary disabled>
                {{ $t('slot.dist_migrate_multi', { n: selectionInfo.count }) }}
            </n-button>
            <div class="flex-item-expand"></div>
            <n-button size="tiny" quaternary @click="selection = []">{{ $t('slot.dist_deselect') }}</n-button>
        </div>
    </div>
</template>

<style scoped lang="scss">
.slot-distribution {
    margin-top: 12px;
    padding: 12px;
    border: 1px solid v-bind('themeVars.borderColor');
    border-radius: 6px;
}

.slot-dist-head {
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
}

.slot-dist-title {
    font-weight: 600;
}

.slot-dist-hint {
    font-size: 12px;
    color: v-bind('themeVars.textColor3');
}

.slot-dist-hint-line {
    margin-top: 4px;
    font-size: 12px;
    color: v-bind('themeVars.textColor3');
}

.slot-dist-canvas-wrap {
    position: relative;
    margin-top: 10px;
    max-width: 100%;
    overflow: hidden;
}

.slot-dist-canvas {
    display: block;
    cursor: crosshair;
}

.slot-dist-placeholder {
    margin-top: 10px;
    padding: 22px 16px;
    border: 1px dashed v-bind('themeVars.borderColor');
    border-radius: 4px;
    color: v-bind('themeVars.textColor3');
    font-size: 12px;
    text-align: center;
}

.slot-dist-scanning {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    color: v-bind('themeVars.textColor2');
    font-size: 12px;
    background: v-bind('themeVars.bodyColor');
    opacity: 0.75;
    pointer-events: none;
}

.slot-dist-tooltip {
    position: absolute;
    z-index: 10;
    padding: 6px 8px;
    border: 1px solid v-bind('themeVars.borderColor');
    border-radius: 4px;
    background: v-bind('themeVars.cardColor');
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.18);
    font-size: 12px;
    line-height: 1.5;
    pointer-events: none;
    white-space: nowrap;
}

.slot-dist-tooltip-slot {
    font-weight: 600;
}

.slot-dist-tooltip-line {
    color: v-bind('themeVars.textColor2');
}

.slot-dist-detail {
    align-items: center;
    gap: 8px;
    margin-top: 8px;
    padding-top: 8px;
    border-top: 1px dashed v-bind('themeVars.borderColor');
}

.slot-dist-detail-text {
    font-size: 12px;
    color: v-bind('themeVars.textColor2');
}
</style>
