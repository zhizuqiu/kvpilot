<script setup>
import { computed, h, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NEmpty, NIcon, NInput, NInputNumber, NProgress, NSelect, NTag, NTooltip, useThemeVars } from 'naive-ui'
import Refresh from '@/components/icons/Refresh.vue'
import CopyIcon from '@/components/icons/Copy.vue'
import { ClipboardSetText } from 'wailsjs/runtime/runtime.js'
import ContentDeployTopology from '@/components/content_value/ContentDeployTopology.vue'
import SlotDistribution from '@/components/content_value/SlotDistribution.vue'
import { formatBytes } from '@/utils/byte_convert.js'
import { formatSlotRanges, parseSlotList } from '@/utils/slot_text.js'
import {
    CancelSlotMigration,
    GetSlotMigration,
    PauseSlotMigration,
    ResumeSlotMigration,
    SlotOverview,
    SlotRebalancePlan,
    StartSlotRebalance,
} from 'wailsjs/go/services/browserService.js'

const props = defineProps({
    server: String,
    pause: Boolean,
})

const i18n = useI18n()
const themeVars = useThemeVars()

const OPTIONS_KEY = 'kvpilot.slotMigration'
const DEFAULT_OPTIONS = {
    batchKeys: 100,
    batchBytesMB: 1,
    migrateTimeoutMs: 5000,
    batchIntervalMs: 10,
    maxSlots: 100,
    strategy: 'slots',
    tolerancePercent: 10,
}

const loadSavedOptions = () => {
    try {
        return { ...DEFAULT_OPTIONS, ...JSON.parse(localStorage.getItem(OPTIONS_KEY) || '{}') }
    } catch (e) {
        return { ...DEFAULT_OPTIONS }
    }
}

const loading = ref(false)
const overview = reactive({
    mode: '',
    state: '',
    slotsAssigned: 0,
    knownNodes: 0,
    masters: [],
    migratingSlots: [],
    unavailable: [],
})

// 批量迁移列表：每条记录各自选择目标节点。
const batch = reactive({ items: [], sourceId: '', targetId: '', slotsText: '' })
const params = reactive(loadSavedOptions())
const advancedOpen = ref(false)
const section = ref('batch')
const topologyToken = ref(0)
const distRef = ref(null)
const plan = reactive({ loading: false, data: null })

const task = reactive({
    state: 'idle',
    step: '',
    slot: 0,
    source: {},
    target: {},
    totalKeys: 0,
    movedKeys: 0,
    movedBytes: 0,
    elapsedMs: 0,
    error: '',
    recovery: [],
    batch: false,
    totalSlots: 0,
    doneSlots: 0,
    currentSlot: 0,
})

let timer = null

const masterOptions = computed(() =>
    overview.masters.map((master) => ({ label: master.addr, value: master.id }))
)

const batchTotalKeys = computed(() => batch.items.reduce((sum, item) => sum + (item.keys || 0), 0))

// 列表按「源节点 + 目标节点」合并展示，避免框选后出现成百上千行。
const batchGroups = computed(() => {
    const groups = new Map()
    for (const item of batch.items) {
        const key = `${item.sourceId}|${item.targetId}`
        if (!groups.has(key)) {
            groups.set(key, {
                key,
                sourceId: item.sourceId,
                sourceAddr: item.sourceAddr,
                targetId: item.targetId,
                targetAddr: item.targetAddr,
                slots: [],
                keys: 0,
            })
        }
        const group = groups.get(key)
        group.slots.push(item.slot)
        group.keys += item.keys || 0
    }
    return [...groups.values()]
        .map((group) => ({ ...group, slots: group.slots.sort((a, b) => a - b) }))
        .sort((a, b) => a.slots[0] - b.slots[0])
})

const masterOwnsSlot = (master, slot) =>
    (master?.slotRanges || []).some((range) => slot >= range.start && slot <= range.end)

const slotRangeList = (master) => {
    if (!master || !master.slotRanges) return []
    return master.slotRanges.map((range) => (range.start === range.end ? `${range.start}` : `${range.start}-${range.end}`))
}

const copyRanges = async (master) => {
    const list = slotRangeList(master)
    if (list.length === 0) return
    await ClipboardSetText(list.join(','))
    $message.success(i18n.t('slot.copied'))
}

const copyAllRanges = async () => {
    const lines = overview.masters
        .filter((master) => slotRangeList(master).length > 0)
        .map((master) => `${master.addr}\t${slotRangeList(master).join(',')}`)
    if (lines.length === 0) return
    await ClipboardSetText(lines.join('\n'))
    $message.success(i18n.t('slot.copied'))
}

const slotRangesText = (master) => {
    const list = slotRangeList(master)
    if (list.length === 0) return '-'
    if (list.length <= 6) return list.join(', ')
    return `${list.slice(0, 6).join(', ')} +${list.length - 6}`
}

const taskActive = computed(() => task.state === 'running' || task.state === 'paused')
const progressPercent = computed(() => {
    if (task.state === 'completed') return 100
    if (!task.totalKeys) return 0
    return Math.min(100, Math.round((task.movedKeys / task.totalKeys) * 100))
})

const stateTagType = computed(() => {
    switch (task.state) {
        case 'running':
            return 'info'
        case 'paused':
            return 'warning'
        case 'completed':
            return 'success'
        case 'failed':
            return 'error'
        default:
            return 'default'
    }
})

const stepLabel = computed(() => (task.step ? i18n.t(`slot.step.${task.step}`) : ''))

const warningText = (warning) => i18n.t(`slot.warning.${warning.code}`, warning.values || {})
const warningType = (warning) => (warning.level === 'error' ? 'error' : warning.level === 'warn' ? 'warning' : 'info')

const clearTimer = () => {
    if (timer) {
        clearTimeout(timer)
        timer = null
    }
}

const applyTask = (data) => {
    if (!data) return
    task.state = data.state || 'idle'
    task.step = data.step || ''
    task.slot = data.slot || 0
    task.source = data.source || {}
    task.target = data.target || {}
    task.totalKeys = data.totalKeys || 0
    task.movedKeys = data.movedKeys || 0
    task.movedBytes = data.movedBytes || 0
    task.elapsedMs = data.elapsedMs || 0
    task.error = data.error || ''
    task.recovery = data.recovery || []
    task.batch = data.batch === true
    task.totalSlots = data.totalSlots || 0
    task.doneSlots = data.doneSlots || 0
    task.currentSlot = data.currentSlot || 0
}

const loadOverview = async () => {
    if (loading.value) return
    loading.value = true
    try {
        const { success, data, msg } = await SlotOverview(props.server)
        if (!success) {
            $message.warning(msg || i18n.t('slot.load_fail'))
            return
        }
        Object.assign(overview, {
            mode: data?.mode || '',
            state: data?.state || '',
            slotsAssigned: data?.slotsAssigned || 0,
            knownNodes: data?.knownNodes || 0,
            masters: data?.masters || [],
            migratingSlots: data?.migratingSlots || [],
            unavailable: data?.unavailable || [],
        })
        if (overview.masters.length > 0) {
            if (!batch.sourceId) {
                batch.sourceId = overview.masters[0].id
            }
            if (!batch.targetId && overview.masters.length > 1) {
                batch.targetId = overview.masters[1].id
            }
        }
    } finally {
        loading.value = false
    }
}

const refreshAll = () => {
    loadOverview()
    topologyToken.value++
}

// 分布图里点击「迁移此槽」并选好目标节点后，统一加入批量列表。
const migrateFromDistribution = (payload) => {
    if (Array.isArray(payload.items)) {
        addSlotsToBatch(payload.items, payload.targetId)
        return
    }
    const { slot, nodeId, targetId, targetAddr, keys = 0 } = payload
    const source = overview.masters.find((master) => master.id === nodeId)
    if (!source) return
    const target = overview.masters.find((master) => master.id === targetId)
    if (!target) {
        $message.warning(i18n.t('slot.batch_no_target'))
        return
    }
    const item = { slot, sourceId: source.id, sourceAddr: source.addr, targetId: target.id, targetAddr: target.addr, keys }
    if (pushBatchItem(item)) {
        section.value = 'batch'
        $message.success(i18n.t('slot.batch_added', { slot }))
    }
}

// 框选多个槽位：能迁的加入列表，不可迁的汇总提示，不逐条刷屏。
const addSlotsToBatch = (entries, targetId) => {
    const target = overview.masters.find((master) => master.id === targetId)
    if (!target) {
        $message.warning(i18n.t('slot.batch_no_target'))
        return
    }
    if (section.value !== 'batch') {
        section.value = 'batch'
    }
    let added = 0
    let skipped = 0
    for (const entry of entries) {
        const source = overview.masters.find((master) => master.id === entry.nodeId) ||
            overview.masters.find((master) => masterOwnsSlot(master, entry.slot))
        if (!source) {
            skipped++
            continue
        }
        const item = {
            slot: entry.slot,
            sourceId: source.id,
            sourceAddr: source.addr,
            targetId: target.id,
            targetAddr: target.addr,
            keys: entry.keys || 0,
        }
        if (validateBatchItem(item)) {
            skipped++
            continue
        }
        batch.items.push(item)
        added++
    }
    if (added > 0) {
        $message.success(i18n.t('slot.batch_added_many', { n: added }))
    }
    if (skipped > 0) {
        $message.warning(i18n.t('slot.batch_skipped_many', { n: skipped }))
    }
    return { added, skipped }
}

// 加入列表前先做可迁移性校验，避免把必然失败的槽位攒到最后才发现。
const validateBatchItem = (item) => {
    const { slot } = item
    if (batch.items.some((existing) => existing.slot === slot)) {
        return { code: 'batch_duplicate', values: { slot }, level: 'info' }
    }
    if (batch.items.length >= 1000) {
        return { code: 'batch_limit', level: 'warning' }
    }
    if (overview.state && overview.state !== 'ok') {
        return { code: 'batch_cluster_state', values: { state: overview.state }, level: 'warning' }
    }
    const source = overview.masters.find((master) => master.id === item.sourceId)
    if (!source || !masterOwnsSlot(source, slot)) {
        return { code: 'batch_not_owner', values: { slot }, level: 'warning' }
    }
    if (overview.unavailable.includes(source.addr)) {
        return { code: 'batch_node_unavailable', values: { addr: source.addr }, level: 'warning' }
    }
    if (overview.migratingSlots.includes(slot) || (source.migrating || []).includes(slot)) {
        return { code: 'batch_slot_migrating', values: { slot }, level: 'warning' }
    }
    const target = overview.masters.find((master) => master.id === item.targetId)
    if (!target) {
        return { code: 'batch_no_target', level: 'warning' }
    }
    if (target.id === source.id) {
        return { code: 'batch_target_same', level: 'warning' }
    }
    if (overview.unavailable.includes(target.addr)) {
        return { code: 'batch_node_unavailable', values: { addr: target.addr }, level: 'warning' }
    }
    return null
}

// 启动前按最新拓扑再校验一次，避免列表停留期间集群状态发生变化。
const validateBatchForStart = () => {
    if (overview.state && overview.state !== 'ok') {
        return { code: 'batch_cluster_state', values: { state: overview.state }, level: 'warning' }
    }
    for (const item of batch.items) {
        const source = overview.masters.find((master) => master.id === item.sourceId)
        if (!source || !masterOwnsSlot(source, item.slot)) {
            return { code: 'batch_not_owner', values: { slot: item.slot }, level: 'warning' }
        }
        if (overview.migratingSlots.includes(item.slot)) {
            return { code: 'batch_slot_migrating', values: { slot: item.slot }, level: 'warning' }
        }
        const target = overview.masters.find((master) => master.id === item.targetId)
        if (!target) {
            return { code: 'batch_no_target', level: 'warning' }
        }
        if (target.id === source.id) {
            return { code: 'batch_target_same', level: 'warning' }
        }
        if (overview.unavailable.includes(target.addr)) {
            return { code: 'batch_node_unavailable', values: { addr: target.addr }, level: 'warning' }
        }
    }
    return null
}

const pushBatchItem = (item) => {
    const reason = validateBatchItem(item)
    if (reason) {
        const message = i18n.t(`slot.${reason.code}`, reason.values || {})
        $message[reason.level || 'warning'](message)
        return false
    }
    batch.items.push(item)
    return true
}

const addBatchFromForm = () => {
    const source = overview.masters.find((master) => master.id === batch.sourceId)
    const target = overview.masters.find((master) => master.id === batch.targetId)
    if (!source || !target) return
    const parsed = parseSlotList(batch.slotsText)
    if (parsed.error) {
        $message.warning(i18n.t('slot.batch_parse_fail', { text: parsed.error }))
        return
    }
    if (parsed.slots.length === 0) return
    const entries = parsed.slots.map((slot) => ({ slot, nodeId: source.id, keys: 0 }))
    const { added } = addSlotsToBatch(entries, target.id)
    if (added > 0) {
        batch.slotsText = ''
    }
}

const removeBatchGroup = (group) => {
    batch.items = batch.items.filter(
        (item) => !(item.sourceId === group.sourceId && item.targetId === group.targetId)
    )
}

const clearBatch = () => {
    batch.items = []
}

const startBatch = () => {
    if (!batch.items.length || taskActive.value) return
    const reason = validateBatchForStart()
    if (reason) {
        $message[reason.level || 'warning'](i18n.t(`slot.${reason.code}`, reason.values || {}))
        return
    }

    // 相同源节点 + 相同目标节点的槽位合并成一个 move，后端按 move 顺序逐槽迁移。
    const groups = new Map()
    for (const item of batch.items) {
        const key = `${item.sourceId}|${item.targetId}`
        if (!groups.has(key)) {
            groups.set(key, {
                source: { id: item.sourceId, addr: item.sourceAddr },
                target: { id: item.targetId, addr: item.targetAddr },
                slots: [],
                approxKeys: 0,
            })
        }
        const move = groups.get(key)
        move.slots.push(item.slot)
        move.approxKeys += item.keys || 0
    }
    const moves = [...groups.values()]
    const totalSlots = batch.items.length

    $dialog.warning(i18n.t('slot.batch_confirm', { n: totalSlots }), async () => {
        const options = {
            batchKeys: params.batchKeys,
            batchBytes: Math.round(params.batchBytesMB * 1024 * 1024),
            migrateTimeoutMs: params.migrateTimeoutMs,
            batchIntervalMs: params.batchIntervalMs,
        }
        const { success, data, msg } = await StartSlotRebalance(props.server, moves, options)
        if (!success) {
            $message.error(msg || i18n.t('slot.batch_start_fail'))
            return
        }
        applyTask(data)
        batch.items = []
        clearTimer()
        timer = setTimeout(poll, 700)
    })
}

const renderSlotRanges = (slots) => {
    const ranges = formatSlotRanges(slots)
    if (ranges.length === 0) return '-'
    const visible = ranges.slice(0, 5)
    const hidden = ranges.length - visible.length
    const text = h('span', { class: 'slot-range-text' }, visible.join(', '))
    if (hidden <= 0) {
        return h('div', { class: 'slot-range-cell' }, [text])
    }
    const more = h(
        NTooltip,
        {
            trigger: 'hover',
            scrollable: true,
            style: { maxWidth: '60vw', maxHeight: '50vh' },
        },
        {
            trigger: () => h('span', { class: 'slot-range-more' }, `+${hidden}`),
            default: () =>
                h('div', {
                    class: 'slot-range-tooltip',
                    style: { maxHeight: '220px', overflowY: 'auto', paddingRight: '6px' },
                }, ranges.map((range) => h('div', { class: 'slot-range-tooltip-line' }, range))),
        }
    )
    return h('div', { class: 'slot-range-cell' }, [text, more])
}

const batchColumns = computed(() => [
    {
        title: () => i18n.t('slot.source'),
        key: 'sourceAddr',
        minWidth: 170,
    },
    {
        title: () => i18n.t('slot.slot_number'),
        key: 'slots',
        minWidth: 220,
        render: (group) => renderSlotRanges(group.slots),
    },
    {
        title: () => i18n.t('slot.batch_target'),
        key: 'targetAddr',
        minWidth: 170,
    },
    {
        title: () => i18n.t('keyspace.col_keys'),
        key: 'keys',
        width: 90,
        align: 'right',
    },
    {
        title: () => i18n.t('config.actions'),
        key: 'action',
        width: 80,
        align: 'center',
        render: (group) =>
            h(
                NButton,
                {
                    size: 'tiny',
                    quaternary: true,
                    disabled: taskActive.value,
                    onClick: () => removeBatchGroup(group),
                },
                { default: () => i18n.t('slot.batch_remove') }
            ),
    },
])

const loadTask = async () => {
    const { success, data } = await GetSlotMigration(props.server)
    if (!success) return
    applyTask(data)
    if (data?.state === 'running') {
        clearTimer()
        timer = setTimeout(poll, 700)
    }
}

const poll = async () => {
    const { success, data } = await GetSlotMigration(props.server)
    if (!success) return
    applyTask(data)
    if (data?.state === 'running') {
        timer = setTimeout(poll, 700)
    } else if (data?.state === 'completed') {
        plan.data = null
        refreshAll()
        // 迁移完成后槽位归属已变化，重新扫描分布，避免地图停留在旧快照。
        distRef.value?.refresh?.()
    }
}

const loadPlan = async () => {
    plan.loading = true
    plan.data = null
    try {
        const { success, data, msg } = await SlotRebalancePlan(
            props.server,
            params.strategy,
            params.maxSlots,
            params.tolerancePercent
        )
        if (!success) {
            $message.warning(msg || i18n.t('slot.plan_fail'))
            return
        }
        plan.data = data
    } finally {
        plan.loading = false
    }
}

const clearPlan = () => {
    plan.data = null
}

const startRebalance = () => {
    if (!plan.data?.moves?.length || taskActive.value) return
    $dialog.warning(i18n.t('slot.confirm_plan', { n: plan.data.totalSlots }), async () => {
        const options = {
            batchKeys: params.batchKeys,
            batchBytes: Math.round(params.batchBytesMB * 1024 * 1024),
            migrateTimeoutMs: params.migrateTimeoutMs,
            batchIntervalMs: params.batchIntervalMs,
        }
        const { success, data, msg } = await StartSlotRebalance(props.server, plan.data.moves, options)
        if (!success) {
            $message.error(msg || i18n.t('slot.start_plan_fail'))
            return
        }
        applyTask(data)
        clearTimer()
        timer = setTimeout(poll, 700)
    })
}

const strategyOptions = computed(() => [
    { label: i18n.t('slot.strategy_slots'), value: 'slots' },
    { label: i18n.t('slot.strategy_keys'), value: 'keys' },
])

const planColumns = computed(() => [
    {
        title: () => i18n.t('slot.source'),
        key: 'source',
        minWidth: 170,
        render: (move) => move.source.addr,
    },
    {
        title: () => i18n.t('slot.target'),
        key: 'target',
        minWidth: 170,
        render: (move) => move.target.addr,
    },
    {
        title: () => i18n.t('slot.plan_slots'),
        key: 'slots',
        width: 100,
        align: 'right',
        render: (move) => move.slots.length,
    },
    {
        title: () => i18n.t('slot.plan_keys'),
        key: 'approxKeys',
        width: 120,
        align: 'right',
        render: (move) => move.approxKeys,
    },
])

const clearResult = () => {
    task.state = 'idle'
    task.step = ''
    task.error = ''
    task.recovery = []
    task.batch = false
    task.totalSlots = 0
    task.doneSlots = 0
    task.currentSlot = 0
}

const pause = async () => {
    const { success, data, msg } = await PauseSlotMigration(props.server)
    if (!success) {
        $message.error(msg)
        return
    }
    applyTask(data)
}

const resume = async () => {
    const { success, data, msg } = await ResumeSlotMigration(props.server)
    if (!success) {
        $message.error(msg)
        return
    }
    applyTask(data)
    clearTimer()
    timer = setTimeout(poll, 700)
}

const cancel = async () => {
    const { success, data, msg } = await CancelSlotMigration(props.server)
    if (!success) {
        $message.error(`${i18n.t('slot.cancel_refused')} ${msg || ''}`.trim())
        return
    }
    applyTask(data)
    clearTimer()
    timer = setTimeout(poll, 400)
}

const columns = computed(() => [
    {
        title: () => i18n.t('session.col_node'),
        key: 'addr',
        minWidth: 190,
        render: (master) => h('div', { class: 'slot-node' }, [
            h('div', master.addr),
            h('div', { class: 'slot-node-id' }, master.id),
        ]),
    },
    {
        title: () => i18n.t('slot.col_ranges'),
        key: 'ranges',
        minWidth: 200,
        render: (master) => {
            const list = slotRangeList(master)
            if (list.length === 0) return '-'
            const visible = list.slice(0, 6)
            const hidden = list.length - visible.length
            const text = h('span', { class: 'slot-range-text' }, visible.join(', '))
            const copy = h(
                NButton,
                {
                    size: 'tiny',
                    quaternary: true,
                    class: 'slot-range-copy',
                    title: i18n.t('slot.copy_ranges'),
                    onClick: () => copyRanges(master),
                },
                { icon: () => h(NIcon, { size: 13 }, () => h(CopyIcon)) }
            )
            if (hidden <= 0) {
                return h('div', { class: 'slot-range-cell' }, [text, copy])
            }
            // 只有 +N 这个小胶囊触发详情，普通区间文本不响应悬停
            const more = h(
                NTooltip,
                {
                    trigger: 'hover',
                    scrollable: true,
                    style: { maxWidth: '60vw', maxHeight: '50vh' },
                },
                {
                    trigger: () => h('span', { class: 'slot-range-more' }, `+${hidden}`),
                    default: () =>
                        h('div', {
                            class: 'slot-range-tooltip',
                            style: { maxHeight: '220px', overflowY: 'auto', paddingRight: '6px' },
                        }, [
                            h('div', { class: 'slot-range-tooltip-title' },
                                `${i18n.t('slot.col_ranges')} · ${list.length}`),
                            ...list.map((range) => h('div', { class: 'slot-range-tooltip-line' }, range)),
                        ]),
                }
            )
            return h('div', { class: 'slot-range-cell' }, [text, more, copy])
        },
    },
    {
        title: () => i18n.t('slot.col_slots'),
        key: 'slotCount',
        width: 110,
        sorter: (a, b) => a.slotCount - b.slotCount,
    },
    {
        title: () => i18n.t('keyspace.col_keys'),
        key: 'keys',
        width: 110,
        sorter: (a, b) => a.keys - b.keys,
    },
    {
        title: () => i18n.t('config.category.memory'),
        key: 'usedMemory',
        width: 120,
        render: (master) => formatBytes(master.usedMemory || 0, 1),
    },
    {
        title: () => i18n.t('slot.col_migrating'),
        key: 'migrating',
        width: 110,
        render: (master) =>
            master.migrating?.length
                ? h(NTag, { size: 'tiny', type: 'warning', bordered: false }, { default: () => master.migrating.length })
                : '-',
    },
    {
        title: () => i18n.t('config.actions'),
        key: 'action',
        width: 110,
        align: 'center',
        render: (master) =>
            h(
                NButton,
                {
                    size: 'tiny',
                    secondary: true,
                    disabled: taskActive.value,
                    onClick: () => {
                        batch.sourceId = master.id
                        batch.slotsText = String(master.slotRanges?.[0]?.start ?? '')
                        section.value = 'batch'
                    },
                },
                { default: () => i18n.t('slot.select_source') }
            ),
    },
])

watch(params, () => localStorage.setItem(OPTIONS_KEY, JSON.stringify({ ...params })), { deep: true })

onMounted(() => {
    loadOverview()
    loadTask()
})

onUnmounted(() => clearTimer())

watch(
    () => props.pause,
    (paused) => {
        if (paused) {
            clearTimer()
        } else {
            loadOverview()
            loadTask()
        }
    }
)
</script>

<template>
    <div class="slot-pane flex-box-v">
        <div class="slot-header flex-box-h">
            <span class="slot-title">{{ $t('slot.title') }}</span>
            <n-tag v-if="overview.state" size="small" :bordered="false" :type="overview.state === 'ok' ? 'success' : 'error'">
                {{ overview.state }}
            </n-tag>
            <n-tag v-if="overview.masters.length" size="small" :bordered="false">
                {{ $t('slot.masters', { n: overview.masters.length }) }}
            </n-tag>
            <n-tag v-if="overview.slotsAssigned" size="small" :bordered="false">
                {{ $t('slot.slots_assigned', { n: overview.slotsAssigned }) }}
            </n-tag>
            <n-tag v-if="overview.migratingSlots.length" size="small" type="warning" :bordered="false">
                {{ $t('slot.migrating', { n: overview.migratingSlots.length }) }}
            </n-tag>
            <div class="flex-item-expand"></div>
            <n-button size="small" secondary @click="copyAllRanges">
                <template #icon>
                    <n-icon :component="CopyIcon" />
                </template>
                {{ $t('slot.copy_all_ranges') }}
            </n-button>
            <n-button size="small" secondary :loading="loading" @click="refreshAll">
                <template #icon>
                    <n-icon :component="Refresh" />
                </template>
                {{ $t('config.refresh') }}
            </n-button>
        </div>

        <n-empty v-if="overview.mode !== 'cluster'" :description="$t('slot.not_cluster')" class="slot-empty" />

        <template v-else>
            <div class="slot-topology">
                <content-deploy-topology
                    :pause="props.pause"
                    :refresh-token="topologyToken"
                    :server="props.server" />
            </div>

            <div class="slot-masters">
                <n-data-table
                    :columns="columns"
                    :data="overview.masters"
                    :loading="loading"
                    :pagination="false"
                    size="small"
                    striped />
            </div>

            <slot-distribution
                ref="distRef"
                :pause="props.pause"
                :server="props.server"
                @migrate="migrateFromDistribution" />

            <div class="slot-migration-panel">
                <div class="slot-panel-header">
                    <span class="slot-panel-title">{{ $t('slot.migration_panel') }}</span>
                    <span class="slot-panel-hint">{{ $t('slot.migration_panel_hint') }}</span>
                </div>
                <div class="slot-panel-advanced">
                    <n-button text size="small" @click="advancedOpen = !advancedOpen">
                        {{ $t('slot.advanced') }}
                    </n-button>
                    <div v-if="advancedOpen" class="slot-advanced flex-box-h">
                        <span class="slot-label">{{ $t('slot.batch_keys') }}</span>
                        <n-input-number v-model:value="params.batchKeys" :max="1000" :min="1" size="small" class="slot-number" />
                        <span class="slot-label">{{ $t('slot.batch_bytes') }}</span>
                        <n-input-number v-model:value="params.batchBytesMB" :max="16" :min="0.0625" :step="0.5" size="small" class="slot-number" />
                        <span class="slot-label">{{ $t('slot.timeout') }}</span>
                        <n-input-number v-model:value="params.migrateTimeoutMs" :max="60000" :min="1000" :step="500" size="small" class="slot-number" />
                        <span class="slot-label">{{ $t('slot.interval') }}</span>
                        <n-input-number v-model:value="params.batchIntervalMs" :max="1000" :min="0" :step="10" size="small" class="slot-number" />
                    </div>
                </div>
                <n-tabs v-model:value="section" size="small" type="line">
                    <n-tab-pane :tab="$t('slot.batch_migration')" display-directive="show" name="batch">
            <div class="slot-migrate">
                <div class="slot-form flex-box-h">
                    <span class="slot-label">{{ $t('slot.source') }}</span>
                    <n-select
                        v-model:value="batch.sourceId"
                        :options="masterOptions"
                        :disabled="taskActive"
                        class="slot-select"
                        size="small" />
                    <span class="slot-label">{{ $t('slot.slot_number') }}</span>
                    <n-input
                        v-model:value="batch.slotsText"
                        :disabled="taskActive"
                        :placeholder="$t('slot.batch_slots_placeholder')"
                        class="slot-slots-input"
                        size="small" />
                    <n-button size="small" secondary :disabled="taskActive" @click="addBatchFromForm">
                        {{ $t('slot.batch_add') }}
                    </n-button>
                    <span class="slot-label">{{ $t('slot.batch_target') }}</span>
                    <n-select
                        v-model:value="batch.targetId"
                        :options="masterOptions"
                        :disabled="taskActive"
                        class="slot-select"
                        size="small" />
                </div>

                <div class="slot-batch-hint-line">{{ $t('slot.batch_hint') }}</div>

                <n-data-table
                    v-if="batchGroups.length"
                    :columns="batchColumns"
                    :data="batchGroups"
                    :row-key="(group) => group.key"
                    :max-height="200"
                    :pagination="false"
                    class="slot-plan-table"
                    size="small"
                    striped />
                <div v-else class="slot-no-warnings">{{ $t('slot.batch_empty') }}</div>

                <div class="slot-batch-footer flex-box-h">
                    <span class="slot-batch-summary">
                        {{ $t('slot.batch_count', { n: batch.items.length, keys: batchTotalKeys }) }}
                    </span>
                    <div class="flex-item-expand"></div>
                    <n-button size="small" quaternary :disabled="!batch.items.length || taskActive" @click="clearBatch">
                        {{ $t('slot.batch_clear') }}
                    </n-button>
                    <n-button size="small" type="primary" :disabled="!batch.items.length || taskActive" @click="startBatch">
                        {{ $t('slot.batch_start') }}
                    </n-button>
                </div>
            </div>
                    </n-tab-pane>
                    <n-tab-pane :tab="$t('slot.rebalance')" display-directive="show" name="rebalance">
            <div class="slot-migrate">
                <div class="slot-form flex-box-h">
                    <span class="slot-label">{{ $t('slot.strategy') }}</span>
                    <n-select
                        v-model:value="params.strategy"
                        :disabled="taskActive"
                        :options="strategyOptions"
                        class="slot-strategy"
                        size="small" />
                    <span class="slot-label">{{ $t('slot.max_slots') }}</span>
                    <n-input-number
                        v-model:value="params.maxSlots"
                        :disabled="taskActive"
                        :max="1000"
                        :min="1"
                        class="slot-number"
                        size="small" />
                    <span class="slot-label">{{ $t('slot.tolerance') }}</span>
                    <n-input-number
                        v-model:value="params.tolerancePercent"
                        :disabled="taskActive"
                        :max="50"
                        :min="1"
                        :step="1"
                        class="slot-number"
                        size="small" />
                    <n-button size="small" secondary :disabled="taskActive" :loading="plan.loading" @click="loadPlan">
                        {{ $t('slot.plan') }}
                    </n-button>
                    <n-button v-if="plan.data" size="small" quaternary :disabled="taskActive" @click="clearPlan">
                        {{ $t('slot.clear_plan') }}
                    </n-button>
                </div>

                <template v-if="plan.data">
                    <div class="slot-metrics">
                        {{
                            plan.data.strategy === 'keys'
                                ? $t('slot.plan_summary_keys', { target: plan.data.balanceTarget, s: plan.data.totalSlots })
                                : $t('slot.plan_summary', { target: plan.data.balanceTarget, s: plan.data.totalSlots })
                        }}
                    </div>
                    <div v-if="plan.data.warnings?.length" class="slot-warnings">
                        <n-tag
                            v-for="(warning, index) in plan.data.warnings"
                            :key="index"
                            size="small"
                            :bordered="false"
                            :type="warningType(warning)">
                            {{ warningText(warning) }}
                        </n-tag>
                    </div>
                    <n-data-table
                        v-if="plan.data.moves.length"
                        :columns="planColumns"
                        :data="plan.data.moves"
                        :max-height="220"
                        :pagination="false"
                        class="slot-plan-table"
                        size="small"
                        striped />
                    <div v-else class="slot-no-warnings">{{ $t('slot.plan_empty') }}</div>
                    <div class="slot-actions">
                        <n-button
                            type="primary"
                            size="small"
                            :disabled="!plan.data.moves.length || taskActive"
                            @click="startRebalance">
                            {{ $t('slot.start_plan') }}
                        </n-button>
                    </div>
                </template>
            </div>
                    </n-tab-pane>
                </n-tabs>

            <div v-if="task.state !== 'idle'" class="slot-task">
                <div class="slot-task-header flex-box-h">
                    <n-tag size="small" :bordered="false" :type="stateTagType">
                        {{ $t(`slot.task_state.${task.state}`) }}
                    </n-tag>
                    <span v-if="!task.batch && task.slot !== undefined" class="slot-task-slot">
                        Slot {{ task.slot }} · {{ task.source.addr }} → {{ task.target.addr }}
                    </span>
                    <span v-if="stepLabel" class="slot-task-step">{{ stepLabel }}</span>
                    <div class="flex-item-expand"></div>
                    <span v-if="task.elapsedMs" class="slot-task-elapsed">
                        {{ $t('slot.elapsed', { s: (task.elapsedMs / 1000).toFixed(1) }) }}
                    </span>
                    <n-button size="tiny" quaternary @click="clearResult">
                        {{ $t('slot.clear_result') }}
                    </n-button>
                </div>

                <n-progress
                    type="line"
                    :percentage="progressPercent"
                    :status="task.state === 'failed' ? 'error' : task.state === 'completed' ? 'success' : 'default'"
                    :show-indicator="false" />
                <div class="slot-task-progress">
                    <template v-if="task.batch">
                        {{ $t('slot.batch_progress', { done: task.doneSlots, total: task.totalSlots }) }}
                        · {{ $t('slot.current_slot', { slot: task.currentSlot }) }} ·
                    </template>
                    {{ $t('slot.moved', { moved: task.movedKeys, total: task.totalKeys }) }}
                    <template v-if="task.movedBytes"> · {{ formatBytes(task.movedBytes, 1) }}</template>
                </div>

                <div v-if="task.error" class="slot-task-error">{{ task.error }}</div>

                <div class="slot-task-actions flex-box-h">
                    <n-button v-if="task.state === 'running'" size="small" secondary @click="pause">
                        {{ $t('slot.pause') }}
                    </n-button>
                    <n-button v-if="task.state === 'paused'" size="small" type="primary" @click="resume">
                        {{ $t('slot.resume') }}
                    </n-button>
                    <n-button
                        v-if="taskActive"
                        size="small"
                        secondary
                        :disabled="!task.batch && task.movedKeys > 0"
                        @click="cancel">
                        {{ $t('config.cancel') }}
                    </n-button>
                    <span v-if="task.batch && task.state === 'running'" class="slot-batch-hint">
                        {{ $t('slot.cancel_after_slot') }}
                    </span>
                    <span v-if="task.state === 'completed'" class="slot-completed">
                        {{ $t('slot.completed_hint') }}
                    </span>
                </div>

                <div v-if="task.recovery.length" class="slot-recovery">
                    <div class="slot-label">{{ $t('slot.recovery') }}</div>
                    <pre v-for="line in task.recovery" :key="line">{{ line }}</pre>
                </div>
            </div>
            </div>
        </template>
    </div>
</template>

<style lang="scss" scoped>
.slot-pane {
    height: 100%;
    padding: 12px;
    box-sizing: border-box;
    min-height: 0;
    overflow: auto;
}

.slot-header {
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;

    .slot-title {
        font-weight: 600;
    }
}

.slot-empty {
    margin-top: 60px;
}

.slot-topology {
    height: 240px;
    min-height: 200px;
    margin-bottom: 10px;
    border: 1px solid v-bind('themeVars.borderColor');
    border-radius: 6px;
    overflow: hidden;
}

.slot-masters {
    :deep(.slot-node-id) {
        font-size: 11px;
        color: v-bind('themeVars.textColor3');
        word-break: break-all;
    }

}

:deep(.slot-range-cell) {
    display: flex;
    align-items: center;
    gap: 6px;
    max-width: 100%;
    min-width: 0;
}

:deep(.slot-range-text) {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
}

:deep(.slot-range-copy) {
    flex: none;
}

:deep(.slot-range-more) {
    flex: none;
    padding: 0 6px;
    border-radius: 9px;
    border: 1px solid v-bind('themeVars.borderColor');
    background: v-bind('themeVars.actionColor');
    color: v-bind('themeVars.textColor2');
    font-size: 11px;
    line-height: 18px;
    cursor: help;
}

.slot-range-tooltip {
    max-height: 220px;
    overflow: auto;
    padding-right: 6px;
    font-size: 12px;
    line-height: 1.6;

    .slot-range-tooltip-title {
        margin-bottom: 4px;
        color: v-bind('themeVars.textColor3');
    }

    .slot-range-tooltip-line {
        white-space: nowrap;
    }
}

.slot-migration-panel {
    margin-top: 12px;
    padding: 12px;
    border: 1px solid v-bind('themeVars.borderColor');
    border-radius: 6px;
}

.slot-panel-header {
    display: flex;
    flex-direction: column;
    gap: 2px;
    margin-bottom: 8px;

    .slot-panel-title {
        font-weight: 600;
    }

    .slot-panel-hint {
        font-size: 12px;
        color: v-bind('themeVars.textColor3');
    }
}

.slot-migrate {
    margin-top: 8px;
}

.slot-migration-panel .slot-task {
    margin-top: 12px;
    padding-top: 12px;
    border: none;
    border-top: 1px dashed v-bind('themeVars.borderColor');
    border-radius: 0;
}

.slot-task {
    padding: 10px;
}

.slot-migrate-title {
    font-weight: 600;
    margin-bottom: 8px;
}

.slot-form,
.slot-advanced,
.slot-task-actions,
.slot-metrics,
.slot-warnings {
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
}

.slot-label {
    font-size: 12px;
    color: v-bind('themeVars.textColor3');
    white-space: nowrap;
}

.slot-select {
    width: 200px;
}

.slot-strategy {
    width: 160px;
}

.slot-number {
    width: 120px;
}

.slot-slots-input {
    width: 200px;
}

.slot-metrics {
    font-size: 12px;
    color: v-bind('themeVars.textColor3');
}

.slot-warnings {
    margin-top: 8px;
}

.slot-no-warnings {
    margin-top: 8px;
    font-size: 12px;
    color: v-bind('themeVars.textColor3');
}

.slot-advanced {
    margin-top: 8px;
}

.slot-actions {
    margin-top: 10px;
}

.slot-batch-hint-line {
    margin-top: 6px;
    font-size: 12px;
    color: v-bind('themeVars.textColor3');
}

.slot-batch-footer {
    align-items: center;
    gap: 8px;
    margin-top: 10px;
}

.slot-batch-summary {
    font-size: 12px;
    color: v-bind('themeVars.textColor3');
}

.slot-task-header {
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;
}

.slot-plan-table {
    margin-top: 8px;
}

.slot-batch-hint {
    font-size: 11px;
    color: v-bind('themeVars.textColor3');
}

.slot-task-slot,
.slot-task-step,
.slot-task-elapsed,
.slot-task-progress,
.slot-completed {
    font-size: 12px;
    color: v-bind('themeVars.textColor3');
}

.slot-task-progress {
    margin-top: 4px;
}

.slot-task-error {
    margin-top: 6px;
    color: v-bind('themeVars.errorColor');
    font-size: 12px;
}

.slot-task-actions {
    margin-top: 10px;
}

.slot-recovery {
    margin-top: 10px;

    pre {
        margin: 4px 0;
        font-size: 12px;
    }
}
</style>
