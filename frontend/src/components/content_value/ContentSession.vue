<script setup>
import { computed, h, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NEmpty, NInput, NTag, useThemeVars } from 'naive-ui'
import Refresh from '@/components/icons/Refresh.vue'
import { toHumanReadable } from '@/utils/date.js'
import { KillSession, ListSessions } from 'wailsjs/go/services/browserService.js'

const props = defineProps({
    server: String,
    pause: Boolean,
})

const i18n = useI18n()
const themeVars = useThemeVars()

const loading = ref(false)
const sessions = ref([])
const nodes = ref([])
const unavailable = ref([])
const total = ref(0)
const idle = ref(0)
const sources = ref([])
const keyword = ref('')
const idleOnly = ref(false)
const dbFilter = ref('all')
const killing = ref(0)

const pagination = reactive({
    pageSize: 20,
    pageSizes: [20, 50, 100],
    showSizePicker: true,
})

const load = async (silent = false) => {
    if (loading.value) return
    loading.value = true
    try {
        const { success, data, msg } = await ListSessions(props.server)
        if (success) {
            sessions.value = data?.sessions || []
            nodes.value = data?.nodes || []
            unavailable.value = data?.unavailable || []
            total.value = data?.total || 0
            idle.value = data?.idle || 0
            sources.value = data?.sources || []
        } else if (!silent && msg) {
            $message.warning(msg)
        }
    } finally {
        loading.value = false
    }
}

const dbOptions = computed(() => {
    const dbs = [...new Set(sessions.value.map((session) => session.db))].sort((a, b) => a - b)
    return [
        { label: i18n.t('session.db_all'), value: 'all' },
        ...dbs.map((db) => ({ label: `DB ${db}`, value: db })),
    ]
})

const filtered = computed(() => {
    const kw = keyword.value.trim().toLowerCase()
    return sessions.value.filter((session) => {
        if (dbFilter.value !== 'all' && session.db !== dbFilter.value) return false
        if (idleOnly.value && session.idle < 60) return false
        if (kw === '') return true
        return [session.addr, session.name, session.user, session.cmd, session.flags, session.node].some((value) =>
            String(value || '')
                .toLowerCase()
                .includes(kw)
        )
    })
})

const topSources = computed(() => sources.value.slice(0, 5))

const killSession = (session) => {
    if (session.protected || killing.value) return
    $dialog.warning(
        i18n.t('session.kill_confirm', {
            addr: session.addr,
            cmd: session.cmd || '-',
            idle: toHumanReadable(session.idle),
        }),
        async () => {
            killing.value = session.id
            try {
                const { success, msg } = await KillSession(props.server, session.node, session.id, session.addr)
                if (!success) {
                    $message.error(msg || i18n.t('session.kill_fail'))
                    return
                }
                $message.success(i18n.t('session.killed', { addr: session.addr }))
                await load(true)
            } finally {
                killing.value = 0
            }
        }
    )
}

const columns = computed(() => [
    {
        title: () => i18n.t('session.col_node'),
        key: 'node',
        width: 170,
        fixed: 'left',
        ellipsis: { tooltip: true },
    },
    {
        title: 'ID',
        key: 'id',
        width: 55,
        sorter: (a, b) => a.id - b.id,
    },
    {
        title: () => i18n.t('session.col_addr'),
        key: 'addr',
        width: 170,
        ellipsis: { tooltip: true },
    },
    {
        title: () => i18n.t('session.col_name'),
        key: 'name',
        width: 110,
        ellipsis: { tooltip: true },
        render: ({ name }) => name || '-',
    },
    {
        title: () => i18n.t('session.col_user'),
        key: 'user',
        width: 100,
        render: ({ user }) => user || '-',
    },
    {
        title: 'DB',
        key: 'db',
        width: 50,
    },
    {
        title: () => i18n.t('session.col_age'),
        key: 'age',
        width: 85,
        sorter: (a, b) => a.age - b.age,
        render: ({ age }) => toHumanReadable(age),
    },
    {
        title: () => i18n.t('session.col_idle'),
        key: 'idle',
        width: 80,
        sorter: (a, b) => a.idle - b.idle,
        render: ({ idle: value }) => toHumanReadable(value),
    },
    {
        title: () => i18n.t('session.col_flags'),
        key: 'flags',
        width: 85,
        render: (session) =>
            h(
                NTag,
                { size: 'tiny', bordered: false, type: session.protected ? 'warning' : 'default' },
                { default: () => (session.protected ? i18n.t('session.protected') : session.flags || 'N') }
            ),
    },
    {
        title: () => i18n.t('session.col_cmd'),
        key: 'cmd',
        minWidth: 105,
        ellipsis: { tooltip: true },
    },
    {
        title: () => i18n.t('session.col_actions'),
        key: 'actions',
        width: 75,
        align: 'center',
        fixed: 'right',
        render: (session) =>
            h(
                NButton,
                {
                    size: 'tiny',
                    secondary: true,
                    type: 'error',
                    disabled: session.protected,
                    loading: killing.value === session.id,
                    title: session.protected ? i18n.t('session.protected_hint') : '',
                    onClick: () => killSession(session),
                },
                { default: () => i18n.t('session.kill') }
            ),
    },
])

onMounted(() => load())

watch(
    () => props.pause,
    (paused) => {
        if (!paused) load(true)
    }
)
</script>

<template>
    <div class="session-pane flex-box-v">
        <div class="session-header flex-box-h">
            <n-tooltip trigger="hover">
                <template #trigger>
                    <span class="session-total">{{ $t('session.total', { n: total }) }}</span>
                </template>
                <div class="session-sources">
                    <div class="session-sources-title">{{ $t('session.sources', { n: topSources.length }) }}</div>
                    <div v-for="source in topSources" :key="source.addr">{{ source.addr }} · {{ source.count }}</div>
                </div>
            </n-tooltip>
            <n-tag v-if="idle > 0" size="small" :bordered="false">
                {{ $t('session.idle_count', { n: idle }) }}
            </n-tag>
            <n-tooltip v-if="unavailable.length" trigger="hover">
                <template #trigger>
                    <n-tag size="small" type="warning" :bordered="false">
                        {{ $t('session.unavailable', { n: unavailable.length }) }}
                    </n-tag>
                </template>
                <div class="session-sources">
                    <div v-for="addr in unavailable" :key="addr">{{ addr }}</div>
                </div>
            </n-tooltip>
            <div class="flex-item-expand"></div>
            <n-select
                v-model:value="dbFilter"
                :options="dbOptions"
                class="session-db-filter"
                size="small" />
            <n-checkbox v-model:checked="idleOnly">{{ $t('session.idle_only') }}</n-checkbox>
            <n-input
                v-model:value="keyword"
                :placeholder="$t('session.search')"
                class="session-search"
                clearable
                size="small" />
            <n-button size="small" secondary :loading="loading" @click="load()">
                <template #icon>
                    <n-icon :component="Refresh" />
                </template>
                {{ $t('session.refresh') }}
            </n-button>
        </div>

        <div class="session-table flex-item-expand">
            <n-data-table
                :columns="columns"
                :data="filtered"
                :loading="loading"
                :pagination="pagination"
                :scroll-x="1085"
                flex-height
                size="small"
                striped>
                <template #empty>
                    <n-empty :description="$t('session.empty')" />
                </template>
            </n-data-table>
        </div>
    </div>
</template>

<style lang="scss" scoped>
.session-pane {
    height: 100%;
    padding: 12px;
    box-sizing: border-box;
    min-height: 0;
}

.session-header {
    align-items: center;
    gap: 8px;
    margin-bottom: 8px;

    .session-total {
        font-size: 12px;
        color: v-bind('themeVars.textColor3');
        cursor: help;
    }

    .session-db-filter {
        width: 110px;
    }

    .session-search {
        width: 240px;
    }
}

.session-sources {
    min-width: 140px;
    font-size: 12px;

    .session-sources-title {
        margin-bottom: 4px;
        color: v-bind('themeVars.textColor3');
    }
}

.session-table {
    flex: 1;
    min-height: 0;

    :deep(.n-data-table) {
        height: 100%;
    }
}
</style>
