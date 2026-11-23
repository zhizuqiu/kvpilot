/**
 * 部署拓扑的分区逻辑。
 *
 * 原则：**复制关系优先于状态**。只要从节点的 master 字段能在节点列表里找到对应主节点，
 * 就必须保留在同一个分组里，这样组件才能画出连线——无论主节点还是从节点是否故障。
 * 只有当主节点已从 `CLUSTER NODES` 里彻底消失时，才退化成无法连线的孤立从节点。
 */

const hasSlots = (node) => String(node?.slots ?? '').trim() !== ''

/**
 * 判断单个节点是否属于故障节点（只影响配色与计数，不影响分组）。
 * - 离线（fail / noaddr / disconnected 都会归一化为 down）
 * - 主节点没有槽位
 * @param {object} node
 * @return {boolean}
 */
export const isFaultyNode = (node) => {
    if (!node) return false
    if (node.state === 'down') return true
    return node.role === 'master' && !hasSlots(node)
}

/**
 * 解析槽位串，剥离 `[slot->-nodeid]` / `[slot-<-nodeid]` 这类迁移标记，
 * 并告诉调用方当前是否处于槽位迁移中。
 * @param {string} slots
 * @return {{text: string, migrating: boolean}}
 */
export const parseSlots = (slots) => {
    const raw = String(slots ?? '')
    const migrating = /\[\d+->|\[\d+-<-/.test(raw)
    const text = raw
        .replace(/\[[^\]]*\]/g, ' ')
        .replace(/\s+/g, ' ')
        .trim()
    return { text, migrating }
}

/**
 * 按复制关系分组：每个主节点（含故障主节点）带上它名下的全部从节点（含故障从节点）。
 * @param {Array} nodes 后端返回的节点数组
 * @return {{
 *   sentinels: Array,
 *   groups: Array<{master: object, replicas: Array}>,
 *   orphans: Array,
 *   offlineCount: number,
 *   orphanCount: number,
 *   slotlessCount: number
 * }}
 */
export const groupTopology = (nodes) => {
    const all = nodes || []

    const sentinels = all.filter((n) => n.role === 'sentinel')
    const masters = all.filter((n) => n.role === 'master')
    const replicas = all.filter((n) => n.role === 'replica')

    const groups = masters.map((master) => ({
        master,
        replicas: replicas.filter((r) => r.master === master.addr),
    }))

    // 主节点已经不在列表里，无法连线，只能单独展示
    const orphans = replicas.filter((r) => !masters.some((m) => m.addr === r.master))

    // 在线但没有任何槽位的主节点：不算离线，但也不健康，需要单独提示
    const slotlessCount = all.filter(
        (n) => n.role === 'master' && n.state !== 'down' && !hasSlots(n)
    ).length

    return {
        sentinels,
        groups,
        orphans,
        offlineCount: all.filter((n) => n.state === 'down').length,
        orphanCount: orphans.length,
        slotlessCount,
    }
}
