<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NEmpty, NInputNumber, useThemeVars } from 'naive-ui'
import { toHumanReadable } from '@/utils/date.js'
import { isWeb } from '@/utils/platform.js'
import { ExportCSV, GetKeyspaceStats } from 'wailsjs/go/services/browserService.js'
import { SaveFile } from 'wailsjs/go/services/systemService.js'

const props = defineProps({
    server: String,
    pause: Boolean,
})

const i18n = useI18n()
const themeVars = useThemeVars()

const loading = ref(false)
const exporting = ref(false)
const sampleSize = ref(1000)
const data = reactive({
    nodes: [],
    databases: [],
    unavailable: [],
    sampled: 0,
    noTtl: 0,
    withTtl: 0,
    types: [],
    prefixes: [],
    elapsedMs: 0,
})

const load = async () => {
    if (loading.value) return
    loading.value = true
    try {
        const { success, data: result, msg } = await GetKeyspaceStats(props.server, sampleSize.value)
        if (!success) {
            $message.warning(msg || i18n.t('keyspace.load_fail'))
            return
        }
        Object.assign(data, {
            nodes: result?.nodes || [],
            databases: result?.databases || [],
            unavailable: result?.unavailable || [],
            sampled: result?.sampled || 0,
            noTtl: result?.noTtl || 0,
            withTtl: result?.withTtl || 0,
            types: result?.types || [],
            prefixes: result?.prefixes || [],
            elapsedMs: result?.elapsedMs || 0,
        })
    } finally {
        loading.value = false
    }
}

const totalKeys = computed(() => data.databases.reduce((sum, row) => sum + (row.keys || 0), 0))
const ttlRatio = computed(() => {
    if (data.sampled <= 0) return '-'
    return ((data.withTtl / data.sampled) * 100).toFixed(1)
})

const databaseColumns = computed(() => [
    {
        title: () => i18n.t('keyspace.col_node'),
        key: 'node',
        width: 190,
        ellipsis: { tooltip: true },
    },
    {
        title: 'DB',
        key: 'db',
        width: 70,
    },
    {
        title: () => i18n.t('keyspace.col_keys'),
        key: 'keys',
        width: 110,
        sorter: (a, b) => a.keys - b.keys,
    },
    {
        title: () => i18n.t('keyspace.col_expires'),
        key: 'expires',
        width: 110,
    },
    {
        title: () => i18n.t('keyspace.col_avg_ttl'),
        key: 'avgTtl',
        width: 130,
        render: ({ avgTtl }) => formatTTL(avgTtl),
    },
])

const typeColumns = computed(() => [
    {
        title: () => i18n.t('keyspace.col_type'),
        key: 'type',
    },
    {
        title: () => i18n.t('keyspace.col_count'),
        key: 'count',
        align: 'right',
    },
    {
        title: () => i18n.t('keyspace.col_percent'),
        key: 'percent',
        align: 'right',
        render: ({ count }) => (data.sampled > 0 ? ((count / data.sampled) * 100).toFixed(1) + '%' : '-'),
    },
])

const prefixColumns = computed(() => [
    {
        title: () => i18n.t('keyspace.col_prefix'),
        key: 'prefix',
        ellipsis: { tooltip: true },
    },
    {
        title: () => i18n.t('keyspace.col_count'),
        key: 'count',
        align: 'right',
    },
])

const formatTTL = (milliseconds) => {
    if (!milliseconds || milliseconds <= 0) return '-'
    return toHumanReadable(Math.round(milliseconds / 1000))
}

const exportCsv = async () => {
    if (data.databases.length === 0) return
    const header = ['node', 'db', 'keys', 'expires', 'avg_ttl']
    const rows = data.databases.map((row) => [row.node, row.db, row.keys, row.expires, row.avgTtl])

    exporting.value = true
    try {
        if (isWeb()) {
            const lines = [header.join(','), ...rows.map((row) => row.map((cell) => `"${cell}"`).join(','))]
            const blob = new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8' })
            const url = URL.createObjectURL(blob)
            const link = document.createElement('a')
            link.href = url
            link.download = 'keyspace.csv'
            link.click()
            URL.revokeObjectURL(url)
            $message.success(i18n.t('keyspace.exported', { n: rows.length }))
            return
        }

        const { success, data: saved } = await SaveFile(null, 'keyspace.csv', ['csv'])
        if (!success || !saved?.path) return
        const resp = await ExportCSV(saved.path, header, rows)
        if (!resp.success) {
            $message.error(resp.msg || i18n.t('keyspace.export_fail'))
            return
        }
        $message.success(i18n.t('keyspace.exported', { n: rows.length }))
    } finally {
        exporting.value = false
    }
}

onMounted(() => {
    if (!props.pause) load()
})

watch(
    () => props.pause,
    (paused) => {
        if (!paused) load()
    }
)
</script>

<template>
    <div class="keyspace-pane flex-box-v">
        <div class="keyspace-header flex-box-h">
            <span class="keyspace-title">{{ $t('keyspace.title') }}</span>
            <n-tag v-if="totalKeys" size="small" :bordered="false" type="info">
                {{ $t('keyspace.total_keys', { n: totalKeys }) }}
            </n-tag>
            <n-tag v-if="data.sampled" size="small" :bordered="false">
                {{ $t('keyspace.sampled', { n: data.sampled }) }}
            </n-tag>
            <n-tag v-if="data.sampled" size="small" :bordered="false">
                {{ $t('keyspace.ttl_ratio', { p: ttlRatio }) }}
            </n-tag>
            <n-tooltip v-if="data.unavailable.length" trigger="hover">
                <template #trigger>
                    <n-tag size="small" type="warning" :bordered="false">
                        {{ $t('keyspace.unavailable', { n: data.unavailable.length }) }}
                    </n-tag>
                </template>
                <div class="keyspace-unavailable">
                    <div v-for="addr in data.unavailable" :key="addr">{{ addr }}</div>
                </div>
            </n-tooltip>
            <div class="flex-item-expand"></div>
            <n-input-number
                v-model:value="sampleSize"
                class="keyspace-number"
                :max="100000"
                :min="0"
                :step="100"
                size="small" />
            <n-button size="small" type="primary" :loading="loading" @click="load">
                {{ $t('keyspace.refresh') }}
            </n-button>
            <n-button size="small" secondary :disabled="data.databases.length === 0" :loading="exporting" @click="exportCsv">
                {{ $t('keyspace.export') }}
            </n-button>
        </div>

        <div class="keyspace-hint">{{ $t('keyspace.hint') }}</div>

        <div class="keyspace-databases">
            <n-data-table
                :columns="databaseColumns"
                :data="data.databases"
                :loading="loading"
                :max-height="240"
                :pagination="false"
                size="small"
                striped>
                <template #empty>
                    <n-empty :description="$t('keyspace.empty')" />
                </template>
            </n-data-table>
        </div>

        <div class="keyspace-distributions flex-box-h">
            <div class="keyspace-distribution">
                <div class="keyspace-section-title">{{ $t('keyspace.types_title') }}</div>
                <n-data-table
                    :columns="typeColumns"
                    :data="data.types"
                    :max-height="220"
                    :pagination="false"
                    size="small" />
            </div>
            <div class="keyspace-distribution">
                <div class="keyspace-section-title">{{ $t('keyspace.prefixes_title') }}</div>
                <n-data-table
                    :columns="prefixColumns"
                    :data="data.prefixes"
                    :max-height="220"
                    :pagination="false"
                    size="small" />
            </div>
        </div>
    </div>
</template>

<style lang="scss" scoped>
.keyspace-pane {
    height: 100%;
    padding: 12px;
    box-sizing: border-box;
    min-height: 0;
}

.keyspace-header {
    align-items: center;
    gap: 8px;

    .keyspace-title {
        font-weight: 600;
    }

    .keyspace-number {
        width: 110px;
    }
}

.keyspace-hint {
    margin-top: 6px;
    font-size: 12px;
    color: v-bind('themeVars.textColor3');
}

.keyspace-databases {
    margin-top: 8px;
}

.keyspace-distributions {
    gap: 12px;
    margin-top: 12px;
    min-height: 0;
    align-items: flex-start;

    .keyspace-distribution {
        flex: 1;
        min-width: 0;
    }

    .keyspace-section-title {
        margin-bottom: 4px;
        font-size: 12px;
        color: v-bind('themeVars.textColor2');
    }
}

.keyspace-unavailable {
    font-size: 12px;
}
</style>
