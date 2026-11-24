<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NEmpty, NInput, NSelect, NTag, useThemeVars } from 'naive-ui'
import Refresh from '@/components/icons/Refresh.vue'
import { filterConfigParams } from '@/utils/config.js'
import { ConfigOverview, ConfigSet } from 'wailsjs/go/services/browserService.js'

const props = defineProps({
    server: String,
    pause: Boolean,
})

const i18n = useI18n()
const themeVars = useThemeVars()

const loading = ref(false)
const params = ref([])
const nodes = ref([])
const merged = ref(true)
const keyword = ref('')
const category = ref('all')
const consistency = ref('all')
const editingKey = ref('')
const editValue = ref('')
const saving = ref(false)

const categories = computed(() => {
    const counts = new Map()
    params.value.forEach((p) => counts.set(p.category, (counts.get(p.category) || 0) + 1))
    return [...counts.entries()].sort((a, b) => b[1] - a[1])
})

const categoryOptions = computed(() => [
    { label: i18n.t('config.category_all', { n: params.value.length }), value: 'all' },
    ...categories.value.map(([key, count]) => ({
        label: i18n.t(`config.category.${key}`) + ` (${count})`,
        value: key,
    })),
])

const consistencyOptions = computed(() => [
    { label: i18n.t('config.consistency_all'), value: 'all' },
    { label: i18n.t('config.consistency_consistent'), value: 'consistent' },
    { label: i18n.t('config.consistency_inconsistent'), value: 'inconsistent' },
    { label: i18n.t('config.consistency_sensitive'), value: 'sensitive' },
    { label: i18n.t('config.consistency_immutable'), value: 'immutable' },
])

const filtered = computed(() =>
    filterConfigParams(params.value, {
        category: category.value,
        keyword: keyword.value,
        consistency: consistency.value,
    })
)

const load = async (silent = false) => {
    if (loading.value) return
    loading.value = true
    try {
        const { success, data, msg } = await ConfigOverview(props.server)
        if (success) {
            params.value = data?.params || []
            nodes.value = data?.nodes || []
            merged.value = data?.merged !== false
        } else if (!silent && msg) {
            $message.warning(msg)
        }
    } finally {
        loading.value = false
    }
}

// 敏感与不可变参数不提供编辑入口
const editable = (param) => !param.sensitive && !param.immutable

const startEdit = (param) => {
    editingKey.value = param.key
    editValue.value = param.value
}

const cancelEdit = () => {
    editingKey.value = ''
    editValue.value = ''
}

const saveEdit = async (param) => {
    if (saving.value) return
    saving.value = true
    try {
        const { success, data, msg } = await ConfigSet(props.server, param.key, editValue.value)
        if (!success) {
            $message.error(msg || i18n.t('config.save_fail'))
            return
        }
        const applied = data?.applied || 0
        const total = data?.nodes || 0
        const failed = data?.failed || 0
        const memoryOnly = data?.memoryOnly || 0
        $message.success(i18n.t('config.saved', { ok: applied, total }))
        if (memoryOnly > 0) {
            $message.warning(i18n.t('config.memory_only', { n: memoryOnly }))
        }
        if (failed > 0) {
            const first = (data?.results || []).find((r) => !r.set)
            $message.error(i18n.t('config.partial', { n: failed, msg: first?.msg || '' }))
        }
        cancelEdit()
        await load(true)
    } finally {
        saving.value = false
    }
}

// 展示某一参数在各节点上的不同取值（最多列出 8 个节点）
const nodeValues = (param) => {
    if (!param.values) return []
    return Object.entries(param.values)
        .sort(([a], [b]) => a.localeCompare(b))
        .slice(0, 8)
        .map(([addr, value]) => `${addr} = ${value}`)
}

onMounted(() => load())

watch(
    () => props.pause,
    (paused) => {
        if (!paused) load(true)
    }
)
</script>

<template>
    <div class="config-pane flex-box-v">
        <div class="config-header flex-box-h">
            <span class="config-nodes">{{ $t('config.nodes', { n: nodes.length }) }}</span>
            <n-tag v-if="!merged" size="small" type="warning" :bordered="false">
                {{ $t('config.merged_hint') }}
            </n-tag>
            <div class="flex-item-expand"></div>
            <n-select
                v-model:value="category"
                :options="categoryOptions"
                class="config-category"
                size="small" />
            <n-select
                v-model:value="consistency"
                :options="consistencyOptions"
                class="config-consistency-filter"
                size="small" />
            <n-input
                v-model:value="keyword"
                :placeholder="$t('config.search')"
                class="config-search"
                clearable
                size="small" />
            <n-button size="small" secondary :loading="loading" @click="load()">
                <template #icon>
                    <n-icon :component="Refresh" />
                </template>
                {{ $t('config.refresh') }}
            </n-button>
        </div>

        <div class="config-table flex-item-expand">
            <div class="config-row config-head">
                <span>{{ $t('config.param') }}</span>
                <span>{{ $t('config.value') }}</span>
                <span class="config-consistency">{{ $t('config.consistency') }}</span>
                <span class="config-actions-head">{{ $t('config.actions') }}</span>
            </div>
            <div v-for="param in filtered" :key="param.key" class="config-row">
                <span class="config-key mono">{{ param.key }}</span>
                <n-input
                    v-if="editingKey === param.key"
                    v-model:value="editValue"
                    class="config-edit-input mono"
                    size="tiny" />
                <span v-else class="config-value mono" :title="param.value">{{ param.value }}</span>
                <span class="config-consistency">
                    <n-tag v-if="param.sensitive" size="tiny" :bordered="false" type="info">
                        {{ $t('config.sensitive') }}
                    </n-tag>
                    <n-tag v-else-if="param.immutable" size="tiny" :bordered="false">
                        {{ $t('config.immutable') }}
                    </n-tag>
                    <n-tag v-else-if="!param.consistent" size="tiny" :bordered="false" type="warning">
                        {{ param.nodes }}/{{ nodes.length }}
                    </n-tag>
                    <span v-else class="config-ok">
                        <i class="config-dot"></i>
                        {{ $t('config.consistent') }}
                    </span>
                    <n-tooltip v-if="!param.consistent && !param.sensitive" trigger="hover">
                        <template #trigger>
                            <span class="config-diff">{{ $t('config.view_diff') }}</span>
                        </template>
                        <div class="config-diff-list">
                            <div v-for="line in nodeValues(param)" :key="line">{{ line }}</div>
                        </div>
                    </n-tooltip>
                </span>
                <span class="config-actions">
                    <template v-if="editable(param)">
                        <template v-if="editingKey === param.key">
                            <n-button size="tiny" secondary :disabled="saving" @click="cancelEdit">
                                {{ $t('config.cancel') }}
                            </n-button>
                            <n-button size="tiny" type="primary" :loading="saving" @click="saveEdit(param)">
                                {{ $t('config.save') }}
                            </n-button>
                        </template>
                        <n-button v-else size="tiny" secondary @click="startEdit(param)">
                            {{ $t('config.edit') }}
                        </n-button>
                    </template>
                </span>
            </div>
            <n-empty v-if="filtered.length === 0" :description="$t('config.empty')" class="config-empty" />
        </div>
    </div>
</template>

<style lang="scss" scoped>
.config-pane {
    height: 100%;
    padding: 12px;
    box-sizing: border-box;
    min-height: 0;
}

.config-header {
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;

    .config-nodes {
        font-size: 12px;
        color: v-bind('themeVars.textColor3');
    }

    .config-category {
        width: 180px;
    }

    .config-consistency-filter {
        width: 150px;
    }

    .config-search {
        width: 240px;
    }
}

.config-table {
    overflow: auto;
    border: 1px solid v-bind('themeVars.borderColor');
    border-radius: 6px;
    min-height: 0;
}

.config-row {
    display: grid;
    grid-template-columns: minmax(180px, 1fr) minmax(200px, 1.2fr) 180px 130px;
    align-items: center;
    gap: 12px;
    padding: 6px 12px;
    font-size: 12px;
    border-bottom: 1px solid v-bind('themeVars.dividerColor');

    &.config-head {
        position: sticky;
        top: 0;
        z-index: 1;
        background: v-bind('themeVars.cardColor');
        font-weight: 600;
        color: v-bind('themeVars.textColor2');
    }

    &:hover {
        background: v-bind('themeVars.actionColor');
    }

    .config-key {
        font-weight: 500;
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .config-value {
        color: v-bind('themeVars.textColor3');
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .config-consistency {
        display: flex;
        align-items: center;
        gap: 6px;
    }

    .config-edit-input {
        width: 100%;
    }

    .config-diff {
        font-size: 12px;
        cursor: help;
        color: v-bind('themeVars.textColor3');
        text-decoration: underline dotted;
    }

    .config-ok {
        display: inline-flex;
        align-items: center;
        gap: 6px;
        font-size: 12px;
        color: v-bind('themeVars.textColor3');
    }

    .config-dot {
        width: 6px;
        height: 6px;
        border-radius: 50%;
        background: v-bind('themeVars.successColor');
    }

    .config-actions {
        display: flex;
        justify-content: flex-end;
        align-items: center;
        gap: 6px;
    }

    .config-actions-head {
        text-align: right;
    }
}

.config-diff-list {
    font-size: 12px;
    font-family: monospace;
    line-height: 1.6;
}

.config-empty {
    padding: 32px 0;
}

.mono {
    font-family: monospace;
}
</style>
