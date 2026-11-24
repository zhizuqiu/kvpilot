<script setup>
import { computed, h, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NEmpty, NInput, NInputNumber, NProgress, NSelect, NSwitch, NTag, useThemeVars } from 'naive-ui'
import { formatBytes } from '@/utils/byte_convert.js'
import { isWeb } from '@/utils/platform.js'
import {
    CancelBigKeyScan,
    ExportBigKeys,
    GetBigKeyScan,
    StartBigKeyScan,
} from 'wailsjs/go/services/browserService.js'
import { SaveFile } from 'wailsjs/go/services/systemService.js'
import ContentKeyspace from '@/components/content_value/ContentKeyspace.vue'

const props = defineProps({
    server: String,
    pause: Boolean,
})

const i18n = useI18n()
const themeVars = useThemeVars()

const options = reactive({
    sampleSize: 1000,
    thresholdMB: 10,
    full: false,
})

const task = reactive({
    state: 'idle',
    scanned: 0,
    total: 0,
    percent: 0,
    found: 0,
    results: [],
    unavailable: [],
    elapsedMs: 0,
    error: '',
})

const section = ref('bigkey')
const starting = ref(false)
const exporting = ref(false)
const typeFilter = ref('all')
const keyword = ref('')

let timer = null

const pagination = reactive({
    pageSize: 20,
    pageSizes: [20, 50, 100],
    showSizePicker: true,
})

const running = computed(() => task.state === 'running')

const apply = (data) => {
    if (!data) return
    task.state = data.state || 'idle'
    task.scanned = data.scanned || 0
    task.total = data.total || 0
    task.percent = data.percent || 0
    task.found = data.found || 0
    task.results = data.results || []
    task.unavailable = data.unavailable || []
    task.elapsedMs = data.elapsedMs || 0
    task.error = data.error || ''
    // 恢复历史任务时同步参数，避免界面显示与实际任务不一致
    if (data.options) {
        options.sampleSize = data.options.sampleSize || options.sampleSize
        options.thresholdMB = data.options.thresholdMB || options.thresholdMB
        options.full = data.options.full === true
    }
}

const stateTag = computed(() => {
    switch (task.state) {
        case 'running':
            return { label: i18n.t('analysis.running'), type: 'info' }
        case 'done':
            return { label: i18n.t('analysis.done'), type: 'success' }
        case 'canceled':
            return { label: i18n.t('analysis.canceled'), type: 'warning' }
        case 'failed':
            return { label: i18n.t('analysis.failed'), type: 'error' }
        default:
            return null
    }
})

const progressStatus = computed(() => {
    if (task.state === 'failed') return 'error'
    if (task.state === 'done') return 'success'
    if (task.state === 'canceled') return 'warning'
    return 'default'
})

const emptyText = computed(() => {
    if (task.state === 'idle') return i18n.t('analysis.idle_hint')
    if (task.state === 'running') return i18n.t('analysis.scanning_hint')
    if (task.state === 'failed') return task.error || i18n.t('analysis.failed')
    return i18n.t('analysis.empty')
})

const typeOptions = computed(() => {
    const types = [...new Set(task.results.map((item) => item.type))].sort()
    return [
        { label: i18n.t('analysis.type_all'), value: 'all' },
        ...types.map((type) => ({ label: type, value: type })),
    ]
})

const filtered = computed(() => {
    const kw = keyword.value.trim().toLowerCase()
    return task.results.filter((item) => {
        if (typeFilter.value !== 'all' && item.type !== typeFilter.value) return false
        if (kw === '') return true
        return String(item.key).toLowerCase().includes(kw) || String(item.node).toLowerCase().includes(kw)
    })
})

const columns = computed(() => [
    {
        title: () => i18n.t('analysis.col_key'),
        key: 'key',
        minWidth: 240,
        ellipsis: { tooltip: true },
    },
    {
        title: () => i18n.t('analysis.col_type'),
        key: 'type',
        width: 90,
    },
    {
        title: () => i18n.t('analysis.col_size'),
        key: 'size',
        width: 110,
        sorter: (a, b) => a.size - b.size,
        render: ({ size }) => formatBytes(size, 1),
    },
    {
        title: () => i18n.t('analysis.col_db'),
        key: 'db',
        width: 60,
    },
    {
        title: () => i18n.t('analysis.col_node'),
        key: 'node',
        width: 170,
        ellipsis: { tooltip: true },
    },
])

const clearTimer = () => {
    if (timer) {
        clearTimeout(timer)
        timer = null
    }
}

const poll = async () => {
    const { success, data } = await GetBigKeyScan(props.server)
    if (!success) return
    apply(data)
    if (data.state === 'running') {
        timer = setTimeout(poll, 700)
    }
}

const load = async () => {
    const { success, data } = await GetBigKeyScan(props.server)
    if (!success) return
    apply(data)
    if (data.state === 'running') {
        clearTimer()
        timer = setTimeout(poll, 700)
    }
}

const start = async () => {
    if (starting.value || running.value) return
    starting.value = true
    clearTimer()
    try {
        const { success, data, msg } = await StartBigKeyScan(
            props.server,
            options.sampleSize,
            options.thresholdMB,
            options.full
        )
        if (!success) {
            $message.error(msg || i18n.t('analysis.start_fail'))
            return
        }
        apply(data)
        timer = setTimeout(poll, 700)
    } finally {
        starting.value = false
    }
}

const cancel = async () => {
    clearTimer()
    const { success, data, msg } = await CancelBigKeyScan(props.server)
    if (!success) {
        $message.error(msg || i18n.t('analysis.cancel_fail'))
        return
    }
    apply(data)
    timer = setTimeout(poll, 700)
}

// 导出列顺序与后端 CSV 保持一致，Web 模式直接在浏览器下载
const csvCell = (value) => `"${String(value ?? '').replace(/"/g, '""')}"`

const downloadCsv = (rows) => {
    const lines = [
        'key,type,size,node,db',
        ...rows.map((row) => [row.key, row.type, row.size, row.node, row.db].map(csvCell).join(',')),
    ]
    const blob = new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = 'bigkeys.csv'
    link.click()
    URL.revokeObjectURL(url)
}

const exportCsv = async () => {
    if (task.results.length === 0) return
    exporting.value = true
    try {
        if (isWeb()) {
            downloadCsv(task.results)
            $message.success(i18n.t('analysis.exported', { n: task.results.length }))
            return
        }

        const { success, data } = await SaveFile(null, 'bigkeys.csv', ['csv'])
        if (!success || !data?.path) return
        const resp = await ExportBigKeys(data.path, task.results)
        if (!resp.success) {
            $message.error(resp.msg || i18n.t('analysis.export_fail'))
            return
        }
        $message.success(i18n.t('analysis.exported', { n: task.results.length }))
    } finally {
        exporting.value = false
    }
}

onMounted(() => load())
onUnmounted(() => clearTimer())

watch(
    () => props.pause,
    (paused) => {
        if (paused) {
            clearTimer()
            return
        }
        load()
    }
)
</script>

<template>
    <div class="analysis-pane flex-box-v">
        <div class="analysis-sections flex-box-h">
            <n-button-group size="small">
                <n-button :type="section === 'bigkey' ? 'primary' : 'default'" @click="section = 'bigkey'">
                    {{ $t('analysis.tab_bigkey') }}
                </n-button>
                <n-button :type="section === 'keyspace' ? 'primary' : 'default'" @click="section = 'keyspace'">
                    {{ $t('analysis.tab_keyspace') }}
                </n-button>
            </n-button-group>
            <span v-if="section === 'bigkey' && task.elapsedMs" class="analysis-elapsed">
                {{ $t('analysis.elapsed', { s: (task.elapsedMs / 1000).toFixed(1) }) }}
            </span>
        </div>

        <div v-show="section === 'bigkey'" class="analysis-section flex-box-v">
        <div class="analysis-header flex-box-h">
            <span class="analysis-title">{{ $t('analysis.title') }}</span>
            <n-tag v-if="task.found" size="small" :bordered="false" type="info">
                {{ $t('analysis.found', { n: task.found }) }}
            </n-tag>
            <n-tooltip v-if="task.unavailable.length" trigger="hover">
                <template #trigger>
                    <n-tag size="small" type="warning" :bordered="false">
                        {{ $t('analysis.unavailable', { n: task.unavailable.length }) }}
                    </n-tag>
                </template>
                <div class="analysis-unavailable">
                    <div v-for="addr in task.unavailable" :key="addr">{{ addr }}</div>
                </div>
            </n-tooltip>
            <n-tag v-if="stateTag" size="small" :bordered="false" :type="stateTag.type">
                {{ stateTag.label }}
            </n-tag>
            <div class="flex-item-expand"></div>
        </div>

        <div class="analysis-options flex-box-h">
            <span class="analysis-label">{{ $t('analysis.sample_size') }}</span>
            <n-input-number
                v-model:value="options.sampleSize"
                class="analysis-number"
                :disabled="running"
                :max="100000"
                :min="100"
                :step="100"
                size="small" />
            <span class="analysis-label">{{ $t('analysis.threshold') }}</span>
            <n-input-number
                v-model:value="options.thresholdMB"
                class="analysis-number"
                :disabled="running"
                :max="1024"
                :min="1"
                size="small" />
            <n-tooltip trigger="hover">
                <template #trigger>
                    <span class="analysis-toggle">
                        <n-switch v-model:value="options.full" :disabled="running" size="small" />
                        {{ $t('analysis.full') }}
                    </span>
                </template>
                {{ $t('analysis.full_hint') }}
            </n-tooltip>
            <n-button v-if="!running" :loading="starting" size="small" type="primary" @click="start">
                {{ $t('analysis.start') }}
            </n-button>
            <n-button v-else size="small" secondary @click="cancel">
                {{ $t('analysis.cancel') }}
            </n-button>
        </div>

        <div class="analysis-progress flex-box-h">
            <n-progress
                class="flex-item-expand"
                :percentage="task.percent"
                :show-indicator="false"
                :status="progressStatus"
                type="line" />
            <span class="analysis-progress-text">
                {{ $t('analysis.scanned', { n: task.scanned }) }}
                <template v-if="task.total"> / {{ task.total }}</template>
            </span>
        </div>

        <div class="analysis-filters flex-box-h">
            <n-select v-model:value="typeFilter" :options="typeOptions" class="analysis-type" size="small" />
            <n-input
                v-model:value="keyword"
                :placeholder="$t('analysis.search')"
                class="analysis-search"
                clearable
                size="small" />
            <div class="flex-item-expand"></div>
            <n-button
                :disabled="task.results.length === 0"
                :loading="exporting"
                size="small"
                secondary
                @click="exportCsv">
                {{ $t('analysis.export') }}
            </n-button>
        </div>

        <div class="analysis-table flex-item-expand">
            <n-data-table
                :columns="columns"
                :data="filtered"
                :pagination="pagination"
                :scroll-x="760"
                flex-height
                size="small"
                striped>
                <template #empty>
                    <n-empty :description="emptyText" />
                </template>
            </n-data-table>
        </div>
        </div>

        <div v-show="section === 'keyspace'" class="analysis-section flex-box-v">
            <content-keyspace :pause="props.pause || section !== 'keyspace'" :server="props.server" />
        </div>
    </div>
</template>

<style lang="scss" scoped>
.analysis-pane {
    height: 100%;
    padding: 12px;
    box-sizing: border-box;
    min-height: 0;
}

.analysis-sections {
    align-items: center;
    gap: 8px;
    margin-bottom: 10px;
}

.analysis-section {
    flex: 1;
    min-height: 0;
}

.analysis-header {
    align-items: center;
    gap: 8px;

    .analysis-title {
        font-weight: 600;
    }

    .analysis-elapsed {
        font-size: 12px;
        color: v-bind('themeVars.textColor3');
    }
}

.analysis-options {
    align-items: center;
    gap: 8px;
    margin-top: 10px;

    .analysis-label {
        font-size: 12px;
        color: v-bind('themeVars.textColor3');
    }

    .analysis-number {
        width: 110px;
    }

    .analysis-toggle {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        font-size: 12px;
        color: v-bind('themeVars.textColor3');
    }
}

.analysis-progress {
    align-items: center;
    gap: 12px;
    margin-top: 10px;

    .analysis-progress-text {
        font-size: 12px;
        color: v-bind('themeVars.textColor3');
        white-space: nowrap;
    }
}

.analysis-filters {
    align-items: center;
    gap: 8px;
    margin: 10px 0 8px;

    .analysis-type {
        width: 130px;
    }

    .analysis-search {
        width: 240px;
    }
}

.analysis-table {
    flex: 1;
    min-height: 0;

    :deep(.n-data-table) {
        height: 100%;
    }
}

.analysis-unavailable {
    font-size: 12px;
}
</style>
