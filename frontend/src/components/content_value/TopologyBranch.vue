<script setup>
import TopologyNodeCard from '@/components/content_value/TopologyNodeCard.vue'

// 递归渲染复制子树：从节点本身也可能带从节点（级联复制）
defineProps({
    branch: { type: Object, required: true },
    showSlots: { type: Boolean, default: true },
})
</script>

<template>
    <div class="topology-branch">
        <topology-node-card
            :node="branch.node"
            :show-slots="showSlots"
            :data-addr="branch.node.addr"
            :data-parent="branch.node.master || ''" />
        <div class="topology-branch-children" v-if="branch.children.length > 0">
            <topology-branch
                v-for="child in branch.children"
                :key="child.node.addr"
                :branch="child"
                :show-slots="showSlots" />
        </div>
    </div>
</template>

<style lang="scss" scoped>
.topology-branch {
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
}

// 级联层级向右缩进，配合连线区分层级
.topology-branch-children {
    position: relative;
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 14px;
    margin-top: 18px;
    margin-left: 26px;
}
</style>
