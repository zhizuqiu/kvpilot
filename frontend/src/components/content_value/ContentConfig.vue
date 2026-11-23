<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NEmpty, NInput, NSelect, NTag, useThemeVars } from 'naive-ui'
import Refresh from '@/components/icons/Refresh.vue'
import { ConfigOverview } from 'wailsjs/go/services/browserService.js'

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

const filtered = computed(() => {
    const kw = keyword.value.trim().toLowerCase()
    return params.value.filter((p) => {
        if (category.value !== 'all' && p.category !== category.value) return false
        if (kw === '') return true
        return p.key.toLowerCase().includes(kw) || String(p.value).toLowerCase().includes(kw)
    })
})

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
            </div>
            <div v-for="param in filtered" :key="param.key" class="config-row">
                <span class="config-key mono">{{ param.key }}</span>
                <span class="config-value mono" :title="param.value">{{ param.value }}</span>
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
                    <span v-else class="hint">{{ $t('config.consistent') }}</span>
                    <n-tooltip v-if="!param.consistent && !param.sensitive" trigger="hover">
                        <template #trigger>
                            <span class="config-diff">{{ $t('config.view_diff') }}</span>
                        </template>
                        <div class="config-diff-list">
                            <div v-for="line in nodeValues(param)" :key="line">{{ line }}</div>
                        </div>
                    </n-tooltip>
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
    grid-template-columns: minmax(180px, 1fr) minmax(200px, 1.2fr) 200px;
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

    .config-diff {
        font-size: 12px;
        cursor: help;
        color: v-bind('themeVars.textColor3');
        text-decoration: underline dotted;
    }

    .hint {
        font-size: 12px;
        color: v-bind('themeVars.textColor3');
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
