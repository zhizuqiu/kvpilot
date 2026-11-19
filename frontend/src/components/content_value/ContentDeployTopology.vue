<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton, NEmpty, useThemeVars } from 'naive-ui'
import Refresh from '@/components/icons/Refresh.vue'
import TopologyNodeCard from '@/components/content_value/TopologyNodeCard.vue'
import { Topology } from 'wailsjs/go/services/browserService.js'

const props = defineProps({
    server: String,
    pause: Boolean,
})

const i18n = useI18n()
const themeVars = useThemeVars()

const loading = ref(false)
const topology = ref({ mode: '', self: '', version: '', nodes: [] })
const groupEls = ref([])
const links = ref({})
let timer = null
let resizeObserver = null

const nodes = computed(() => topology.value?.nodes || [])
const sentinels = computed(() => nodes.value.filter((n) => n.role === 'sentinel'))
const masters = computed(() => nodes.value.filter((n) => n.role === 'master'))
const replicas = computed(() => nodes.value.filter((n) => n.role === 'replica'))
const isEmpty = computed(() => nodes.value.length === 0)
const isCluster = computed(() => topology.value?.mode === 'cluster')

// 按主节点分组：每个从节点只挂到它自己的主节点下
const groups = computed(() =>
    masters.value.map((master) => ({
        master,
        replicas: replicas.value.filter((r) => r.master === master.addr),
    }))
)

const modeLabel = computed(() => {
    const mode = (topology.value?.mode || '').replaceAll('-', '_')
    return mode === '' ? '' : i18n.t(`topology.mode_${mode}`)
})

// 计算主节点到各从节点的连线（贝塞尔曲线，避免横向母线与级联的歧义）
const computeLinks = async () => {
    await nextTick()
    const result = {}
    groupEls.value.forEach((el) => {
        if (!el) return
        const cards = el.querySelectorAll('.topology-node')
        if (cards.length < 2) return
        const base = el.getBoundingClientRect()
        const master = cards[0].getBoundingClientRect()
        const x1 = master.left + master.width / 2 - base.left
        const y1 = master.bottom - base.top
        const paths = []
        for (let i = 1; i < cards.length; i++) {
            const replica = cards[i].getBoundingClientRect()
            const x2 = replica.left + replica.width / 2 - base.left
            const y2 = replica.top - base.top
            const midY = y1 + (y2 - y1) / 2
            paths.push({ d: `M ${x1} ${y1} C ${x1} ${midY}, ${x2} ${midY}, ${x2} ${y2}` })
        }
        result[el.dataset.key] = paths
    })
    links.value = result
}

const load = async (silent = false) => {
    if (loading.value) return
    loading.value = true
    try {
        const { success, data, msg } = await Topology(props.server)
        if (success) {
            topology.value = data || { mode: '', self: '', version: '', nodes: [] }
            computeLinks()
        } else if (!silent && msg) {
            $message.warning(msg)
        }
    } finally {
        loading.value = false
    }
}

onMounted(() => {
    load()
    timer = setInterval(() => {
        if (!props.pause) load(true)
    }, 5000)
    window.addEventListener('resize', computeLinks)
    if (typeof ResizeObserver !== 'undefined') {
        resizeObserver = new ResizeObserver(() => computeLinks())
        if (groupEls.value[0]) resizeObserver.observe(groupEls.value[0])
    }
})

onUnmounted(() => {
    if (timer != null) clearInterval(timer)
    window.removeEventListener('resize', computeLinks)
    resizeObserver?.disconnect()
})

watch(
    () => props.pause,
    (paused) => {
        if (!paused) load(true)
    }
)
</script>

<template>
    <div class="topology-pane flex-box-v">
        <div class="topology-header flex-box-h">
            <n-tag v-if="modeLabel" size="small" :bordered="false" type="info">{{ modeLabel }}</n-tag>
            <span class="topology-self" v-if="topology.self">
                {{ $t('topology.self') }}: <b>{{ topology.self }}</b>
            </span>
            <span class="topology-version" v-if="topology.version">v{{ topology.version }}</span>
            <div class="flex-item-expand"></div>
            <n-button size="tiny" secondary :loading="loading" @click="load()">
                <template #icon>
                    <n-icon :component="Refresh" />
                </template>
                {{ $t('topology.refresh') }}
            </n-button>
        </div>

        <div class="topology-body flex-item-expand">
            <template v-if="!isEmpty">
                <!-- 哨兵节点 -->
                <div class="topology-row" v-if="sentinels.length > 0">
                    <topology-node-card v-for="node in sentinels" :key="node.addr" :node="node" />
                </div>
                <div class="topology-link" v-if="sentinels.length > 0 && groups.length > 0"></div>

                <!-- 主节点 + 各自的从节点 -->
                <div class="topology-groups">
                    <template v-for="(group, idx) in groups" :key="group.master.addr">
                        <span v-if="idx > 0 && isCluster" class="group-plus">+</span>
                        <div
                            :ref="(el) => (groupEls[idx] = el)"
                            :data-key="group.master.addr"
                            class="topology-group">
                            <svg class="group-links">
                                <path
                                    v-for="(link, li) in links[group.master.addr] || []"
                                    :key="li"
                                    :d="link.d"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2" />
                            </svg>

                            <topology-node-card :node="group.master" />

                            <div v-if="group.replicas.length > 0" class="topology-replicas">
                                <topology-node-card
                                    v-for="replica in group.replicas"
                                    :key="replica.addr"
                                    :node="replica" />
                            </div>
                        </div>
                    </template>
                </div>
            </template>

            <n-empty v-else :description="$t('topology.empty')" class="topology-empty" />
        </div>
    </div>
</template>

<style lang="scss" scoped>
.topology-pane {
    padding: 12px;
    height: 100%;
    box-sizing: border-box;
    overflow: auto;

    .topology-header {
        gap: 10px;
        align-items: center;
        padding-bottom: 10px;
        border-bottom: 1px solid v-bind('themeVars.borderColor');

        .topology-self {
            font-size: 12px;

            b {
                font-family: monospace;
            }
        }

        .topology-version {
            font-size: 12px;
            opacity: 0.6;
        }
    }

    .topology-body {
        padding: 24px 12px;
        overflow: auto;
    }

    .topology-row {
        display: flex;
        justify-content: center;
        align-items: flex-start;
        gap: 16px;
        flex-wrap: wrap;
    }

    .topology-link {
        width: 2px;
        height: 24px;
        margin: 6px auto;
        background: v-bind('themeVars.borderColor');
    }

    .topology-groups {
        display: flex;
        justify-content: center;
        align-items: flex-start;
        gap: 36px;
        flex-wrap: wrap;
    }

    // 单个主节点分组：主节点在上，从节点在下方一排，连线由 svg 绘制
    .topology-group {
        position: relative;
        display: flex;
        flex-direction: column;
        align-items: center;

        .topology-replicas {
            display: flex;
            align-items: flex-start;
            justify-content: center;
            gap: 18px;
            margin-top: 40px;
            flex-wrap: wrap;
        }

        .group-links {
            position: absolute;
            inset: 0;
            width: 100%;
            height: 100%;
            color: v-bind('themeVars.borderColor');
            pointer-events: none;
            z-index: 0;
            overflow: visible;
        }

        > .topology-node,
        .topology-replicas {
            position: relative;
            z-index: 1;
        }
    }

    // 集群模式下主节点之间的分隔符
    .group-plus {
        align-self: flex-start;
        margin-top: 38px;
        font-size: 20px;
        font-weight: 600;
        opacity: 0.45;
        user-select: none;
    }

    .topology-empty {
        padding-top: 40px;
    }
}
</style>
