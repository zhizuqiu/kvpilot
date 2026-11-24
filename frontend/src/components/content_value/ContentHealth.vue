<script setup>
import { computed, h, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NEmpty, NSelect, NTag, useThemeVars } from 'naive-ui'
import Refresh from '@/components/icons/Refresh.vue'
import useTabStore from 'stores/tab.js'
import { BrowserTabType } from '@/consts/browser_tab_type.js'
import { isWeb } from '@/utils/platform.js'
import { CheckHealth, ExportCSV } from 'wailsjs/go/services/browserService.js'
import { SaveFile } from 'wailsjs/go/services/systemService.js'

const props = defineProps({
    server: String,
    pause: Boolean,
})

const i18n = useI18n()
const themeVars = useThemeVars()
const tabStore = useTabStore()

const loading = ref(false)
const exporting = ref(false)
const filter = ref('issues')
const data = reactive({
    nodes: [],
    unavailable: [],
    items: [],
    counts: { ok: 0, warn: 0, error: 0 },
    mode: '',
    elapsedMs: 0,
})

const severityOptions = computed(() => [
    { label: i18n.t('health.filter_issues'), value: 'issues' },
    { label: i18n.t('health.filter_error'), value: 'error' },
    { label: i18n.t('health.filter_all'), value: 'all' },
])

const filtered = computed(() => {
    return data.items.filter((item) => {
        if (filter.value === 'all') return true
        if (filter.value === 'error') return item.severity === 'error'
        return item.severity !== 'ok'
    })
})

const severityTagType = (severity) => {
    switch (severity) {
        case 'error':
            return 'error'
        case 'warn':
            return 'warning'
        default:
            return 'success'
    }
}

const ruleTitle = (rule) => i18n.t(`health.rule.${rule}`)
const ruleSuggestion = (rule) => i18n.t(`health.suggest.${rule}`)

// 详情文案在本地组装，后端只返回原始值，便于多语言展示
const ruleDetail = (item) => {
    const values = item.values || {}
    switch (item.rule) {
        case 'fragmentation':
            return i18n.t('health.detail.fragmentation', { ratio: values.ratio })
        case 'maxmemory':
            return values.max
                ? i18n.t('health.detail.maxmemory', { used: values.used, max: values.max, percent: values.percent })
                : i18n.t('health.detail.maxmemory_unset', { used: values.used })
        case 'policy':
            return i18n.t('health.detail.policy', { policy: values.policy })
        case 'evicted':
            return i18n.t('health.detail.evicted', { count: values.count })
        case 'persistence':
            return i18n.t('health.detail.persistence', { rdb: values.rdb || '-', aof: values.aof || '-' })
        case 'replication':
            return i18n.t('health.detail.replication', {
                role: values.role || '-',
                link: values.link || '-',
                lag: values.lag || '0',
                slaves: values.slaves || '0',
            })
        case 'blocked':
            return i18n.t('health.detail.blocked', { count: values.count })
        case 'slowlog':
            return i18n.t('health.detail.slowlog', { threshold: values.threshold })
        case 'auth':
            return i18n.t('health.detail.auth', { protected: values.protected, password: values.password })
        case 'persistence_config':
            return i18n.t('health.detail.persistence_config', {
                save: values.save || '-',
                appendonly: values.appendonly || '-',
            })
        case 'cluster':
            return i18n.t('health.detail.cluster', { state: values.state || '-' })
        default:
            return JSON.stringify(values)
    }
}

const jumpTab = (rule) => {
    if (['auth', 'slowlog', 'persistence_config'].includes(rule)) {
        return BrowserTabType.Config
    }
    return BrowserTabType.Status
}

const jump = (item) => tabStore.switchSubTab(jumpTab(item.rule))

const load = async () => {
    if (loading.value) return
    loading.value = true
    try {
        const { success, data: result, msg } = await CheckHealth(props.server)
        if (!success) {
            $message.warning(msg || i18n.t('health.load_fail'))
            return
        }
        Object.assign(data, {
            nodes: result?.nodes || [],
            unavailable: result?.unavailable || [],
            items: result?.items || [],
            counts: result?.counts || { ok: 0, warn: 0, error: 0 },
            mode: result?.mode || '',
            elapsedMs: result?.elapsedMs || 0,
        })
    } finally {
        loading.value = false
    }
}

const columns = computed(() => [
    {
        title: () => i18n.t('health.col_severity'),
        key: 'severity',
        width: 90,
        render: (item) =>
            h(
                NTag,
                { size: 'small', bordered: false, type: severityTagType(item.severity) },
                { default: () => i18n.t(`health.severity.${item.severity}`) }
            ),
    },
    {
        title: () => i18n.t('health.col_rule'),
        key: 'rule',
        width: 170,
        render: (item) => {
            const title = ruleTitle(item.rule)
            if (item.severity === 'ok') return title
            return h(
                'span',
                { class: 'health-rule-warn', title: ruleSuggestion(item.rule) },
                title
            )
        },
    },
    {
        title: () => i18n.t('health.col_detail'),
        key: 'detail',
        minWidth: 260,
        ellipsis: { tooltip: true },
        render: (item) => ruleDetail(item),
    },
    {
        title: () => i18n.t('health.col_node'),
        key: 'node',
        width: 170,
        ellipsis: { tooltip: true },
    },
    {
        title: () => i18n.t('health.col_action'),
        key: 'action',
        width: 90,
        align: 'center',
        render: (item) =>
            h(
                NButton,
                { size: 'tiny', secondary: true, onClick: () => jump(item) },
                { default: () => i18n.t('health.jump') }
            ),
    },
])

const exportCsv = async () => {
    if (filtered.value.length === 0) return
    const header = ['node', 'rule', 'severity', 'detail']
    const rows = filtered.value.map((item) => [item.node, item.rule, item.severity, ruleDetail(item)])

    exporting.value = true
    try {
        if (isWeb()) {
            const lines = [header.join(','), ...rows.map((row) => row.map((cell) => `"${cell}"`).join(','))]
            const blob = new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8' })
            const url = URL.createObjectURL(blob)
            const link = document.createElement('a')
            link.href = url
            link.download = 'health.csv'
            link.click()
            URL.revokeObjectURL(url)
            $message.success(i18n.t('health.exported', { n: rows.length }))
            return
        }

        const { success, data: saved } = await SaveFile(null, 'health.csv', ['csv'])
        if (!success || !saved?.path) return
        const resp = await ExportCSV(saved.path, header, rows)
        if (!resp.success) {
            $message.error(resp.msg || i18n.t('health.export_fail'))
            return
        }
        $message.success(i18n.t('health.exported', { n: rows.length }))
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
    <div class="health-pane flex-box-v">
        <div class="health-header flex-box-h">
            <span class="health-title">{{ $t('health.title') }}</span>
            <n-tag size="small" :bordered="false" type="error">
                {{ $t('health.counts.error', { n: data.counts.error }) }}
            </n-tag>
            <n-tag size="small" :bordered="false" type="warning">
                {{ $t('health.counts.warn', { n: data.counts.warn }) }}
            </n-tag>
            <n-tag size="small" :bordered="false" type="success">
                {{ $t('health.counts.ok', { n: data.counts.ok }) }}
            </n-tag>
            <n-tag v-if="data.mode" size="small" :bordered="false">
                {{ data.mode }}
            </n-tag>
            <n-tooltip v-if="data.unavailable.length" trigger="hover">
                <template #trigger>
                    <n-tag size="small" type="warning" :bordered="false">
                        {{ $t('health.unavailable', { n: data.unavailable.length }) }}
                    </n-tag>
                </template>
                <div class="health-unavailable">
                    <div v-for="addr in data.unavailable" :key="addr">{{ addr }}</div>
                </div>
            </n-tooltip>
            <div class="flex-item-expand"></div>
            <span v-if="data.elapsedMs" class="health-elapsed">
                {{ $t('health.elapsed', { s: (data.elapsedMs / 1000).toFixed(2) }) }}
            </span>
            <n-select v-model:value="filter" :options="severityOptions" class="health-filter" size="small" />
            <n-button size="small" secondary :loading="loading" @click="load">
                <template #icon>
                    <n-icon :component="Refresh" />
                </template>
                {{ $t('health.run') }}
            </n-button>
            <n-button size="small" secondary :disabled="filtered.length === 0" :loading="exporting" @click="exportCsv">
                {{ $t('health.export') }}
            </n-button>
        </div>

        <div class="health-table flex-item-expand">
            <n-data-table
                :columns="columns"
                :data="filtered"
                :loading="loading"
                :pagination="false"
                :scroll-x="800"
                flex-height
                size="small"
                striped>
                <template #empty>
                    <n-empty :description="$t('health.empty')" />
                </template>
            </n-data-table>
        </div>
    </div>
</template>

<style lang="scss" scoped>
.health-pane {
    height: 100%;
    padding: 12px;
    box-sizing: border-box;
    min-height: 0;
}

.health-header {
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;

    .health-title {
        font-weight: 600;
    }

    .health-elapsed {
        font-size: 12px;
        color: v-bind('themeVars.textColor3');
        white-space: nowrap;
    }

    .health-filter {
        width: 130px;
    }
}

.health-table {
    flex: 1;
    min-height: 0;

    :deep(.n-data-table) {
        height: 100%;
    }
}

.health-rule-warn {
    cursor: help;
    text-decoration: underline dotted;
}

.health-unavailable {
    font-size: 12px;
}
</style>
