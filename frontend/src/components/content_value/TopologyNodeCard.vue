<script setup>
import { computed } from 'vue'
import { NTag, useThemeVars } from 'naive-ui'
import { useI18n } from 'vue-i18n'

const props = defineProps({
    node: { type: Object, required: true },
})

const i18n = useI18n()
const themeVars = useThemeVars()

const roleLabel = computed(() => i18n.t(`topology.${props.node.role}`))
const stateLabel = computed(() => {
    const state = props.node.state
    if (state === 'online') return i18n.t('topology.online')
    if (state === 'down') return i18n.t('topology.down')
    return state || ''
})
</script>

<template>
    <div class="topology-node" :class="[`role-${node.role}`, { self: node.self, down: node.state === 'down' }]">
        <div class="node-head">
            <span class="node-role">{{ roleLabel }}</span>
            <n-tag v-if="node.self" size="tiny" :bordered="false" class="self-tag">{{ $t('topology.self') }}</n-tag>
        </div>
        <div class="node-addr">{{ node.addr }}</div>
        <div class="node-meta" v-if="node.slots">{{ $t('topology.slots') }}: {{ node.slots }}</div>
        <div class="node-meta" v-if="node.master">{{ $t('topology.master_node') }}: {{ node.master }}</div>
        <div class="node-meta" v-if="node.offset">{{ $t('topology.offset') }}: {{ node.offset }}</div>
        <div class="node-meta" v-if="node.lag">{{ $t('topology.lag') }}: {{ node.lag }}</div>
        <div class="node-meta" v-if="node.state">{{ stateLabel }}</div>
    </div>
</template>

<style lang="scss" scoped>
.topology-node {
    box-sizing: border-box;
    border: 1px solid v-bind('themeVars.borderColor');
    border-radius: 8px;
    background: v-bind('themeVars.cardColor');
    color: v-bind('themeVars.textColor1');
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);
    font-size: 12px;
    padding: 10px 12px;

    &.self {
        outline: 1px dashed #35e0c8;
        outline-offset: 2px;
    }

    &.down {
        opacity: 0.55;
    }

    .node-head {
        display: flex;
        align-items: center;
        gap: 8px;
        margin-bottom: 6px;
    }

    .node-role {
        display: inline-flex;
        align-items: center;
        padding: 1px 8px;
        border-radius: 10px;
        font-size: 11px;
        font-weight: 700;
        line-height: 18px;
    }

    .node-addr {
        font-family: monospace;
        font-size: 13px;
        word-break: break-all;
    }

    .node-meta {
        margin-top: 2px;
        font-size: 12px;
        color: v-bind('themeVars.textColor3');
        word-break: break-all;
    }

    /* ---------- 主节点：强调 ---------- */
    &.role-master {
        min-width: 210px;
        max-width: 280px;
        border-left: 5px solid #e05252;
        background: linear-gradient(0deg, rgba(224, 82, 82, 0.05), rgba(224, 82, 82, 0.05)), v-bind('themeVars.cardColor');

        .node-role {
            color: #b02b2b;
            background: rgba(224, 82, 82, 0.14);
        }

        .node-addr {
            font-size: 14px;
            font-weight: 700;
        }
    }

    /* ---------- 从节点：弱化 ---------- */
    &.role-replica {
        min-width: 180px;
        max-width: 240px;
        border-left: 3px solid #7fb2ff;
        border-style: dashed;
        border-left-style: solid;

        .node-role {
            color: #2f6fd0;
            background: rgba(88, 143, 226, 0.12);
        }

        .node-addr {
            font-size: 12.5px;
            color: v-bind('themeVars.textColor2');
        }
    }

    /* ---------- 哨兵：琥珀色 ---------- */
    &.role-sentinel {
        min-width: 190px;
        max-width: 260px;
        border-left: 4px solid #d9a13b;
        background: linear-gradient(0deg, rgba(217, 161, 59, 0.06), rgba(217, 161, 59, 0.06)), v-bind('themeVars.cardColor');

        .node-role {
            color: #8a5f00;
            background: rgba(217, 161, 59, 0.18);
        }
    }
}
</style>
