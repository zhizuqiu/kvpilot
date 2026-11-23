import { describe, expect, it } from 'vitest'
import { groupTopology, isFaultyNode, parseSlots } from '@/utils/topology.js'

const master = (addr, extra = {}) => ({
    addr,
    role: 'master',
    state: 'online',
    slots: '0-5460',
    ...extra,
})

const replica = (addr, masterAddr, extra = {}) => ({
    addr,
    role: 'replica',
    state: 'online',
    master: masterAddr,
    ...extra,
})

describe('isFaultyNode', () => {
    it('离线节点判定为故障', () => {
        expect(isFaultyNode(master('a:1', { state: 'down' }))).toBe(true)
        expect(isFaultyNode(replica('b:1', 'a:1', { state: 'down' }))).toBe(true)
    })

    it('没有槽位的主节点判定为故障', () => {
        expect(isFaultyNode(master('a:1', { slots: '' }))).toBe(true)
        expect(isFaultyNode(master('a:1', { slots: '   ' }))).toBe(true)
    })

    it('健康主节点与从节点不算故障', () => {
        expect(isFaultyNode(master('a:1'))).toBe(false)
        expect(isFaultyNode(replica('b:1', 'a:1'))).toBe(false)
    })

    it('空值安全', () => {
        expect(isFaultyNode(null)).toBe(false)
    })
})

describe('groupTopology', () => {
    it('健康主节点按原顺序分组，从节点挂到各自主节点下', () => {
        const view = groupTopology([
            master('m1'),
            master('m2'),
            replica('r1', 'm1'),
            replica('r2', 'm1'),
            replica('r3', 'm2'),
        ])

        expect(view.groups.map((g) => g.master.addr)).toEqual(['m1', 'm2'])
        expect(view.groups[0].children.map((c) => c.node.addr)).toEqual(['r1', 'r2'])
        expect(view.groups[1].children.map((c) => c.node.addr)).toEqual(['r3'])
        expect(view.orphans).toHaveLength(0)
        expect(view.offlineCount).toBe(0)
    })

    it('主节点故障时分组保留，从节点仍挂在它下面以保持连线', () => {
        const view = groupTopology([
            master('m1'),
            master('m2', { state: 'down', slots: '' }),
            replica('r1', 'm1'),
            replica('r2', 'm2'),
        ])

        expect(view.groups.map((g) => g.master.addr)).toEqual(['m1', 'm2'])
        expect(view.groups[1].children.map((c) => c.node.addr)).toEqual(['r2'])
        expect(view.offlineCount).toBe(1)
        // 主节点还在列表里，从节点不算"失去主节点"
        expect(view.orphans).toHaveLength(0)
        expect(view.orphanCount).toBe(0)
    })

    it('从节点故障时仍保留在主节点分组内，避免连线消失', () => {
        const view = groupTopology([master('m1'), replica('r1', 'm1', { state: 'down' })])

        expect(view.groups).toHaveLength(1)
        expect(view.groups[0].children.map((c) => c.node.addr)).toEqual(['r1'])
        expect(view.offlineCount).toBe(1)
    })

    it('在线但没分配槽位的主节点单独计数', () => {
        const view = groupTopology([
            master('m1'),
            master('m2', { slots: '' }),
            master('m3', { slots: '', state: 'down' }),
        ])

        // m2 在线但无槽位；m3 已离线，只计入离线数，不重复计入无槽位
        expect(view.slotlessCount).toBe(1)
        expect(view.offlineCount).toBe(1)
    })

    it('主节点已从列表消失时，从节点归入无法连线的孤立集合', () => {
        const view = groupTopology([master('m1'), replica('r9', 'gone')])

        expect(view.groups).toHaveLength(1)
        expect(view.orphans.map((r) => r.addr)).toEqual(['r9'])
        expect(view.orphanCount).toBe(1)
    })

    it('级联从节点递归挂在父节点下，不算失去主节点', () => {
        const view = groupTopology([
            master('m1'),
            replica('r1', 'm1'),
            replica('c1', 'r1'),
            replica('c2', 'c1'),
        ])

        expect(view.groups).toHaveLength(1)
        const r1 = view.groups[0].children[0]
        expect(r1.node.addr).toBe('r1')
        expect(r1.children[0].node.addr).toBe('c1')
        expect(r1.children[0].children[0].node.addr).toBe('c2')
        expect(view.orphans).toHaveLength(0)
        expect(view.orphanCount).toBe(0)
    })

    it('主节点与从节点都可能有多个子节点', () => {
        const view = groupTopology([
            master('m1'),
            replica('r1', 'm1'),
            replica('r2', 'm1'),
            replica('c1', 'r1'),
            replica('c2', 'r1'),
        ])

        const r1 = view.groups[0].children.find((c) => c.node.addr === 'r1')
        expect(r1.children.map((c) => c.node.addr)).toEqual(['c1', 'c2'])
    })

    it('哨兵单独成行，离线哨兵也计入离线数', () => {
        const view = groupTopology([
            { addr: 's1', role: 'sentinel', state: 'down' },
            { addr: 's2', role: 'sentinel', state: 'online' },
        ])

        expect(view.sentinels.map((s) => s.addr)).toEqual(['s1', 's2'])
        expect(view.offlineCount).toBe(1)
    })

    it('空输入返回空结构', () => {
        const view = groupTopology([])
        expect(view).toEqual({
            sentinels: [],
            groups: [],
            orphans: [],
            offlineCount: 0,
            orphanCount: 0,
            slotlessCount: 0,
        })
    })
})

describe('parseSlots', () => {
    it('普通槽位串原样展示', () => {
        expect(parseSlots('0-5460')).toEqual({ text: '0-5460', migrating: false })
    })

    it('迁移中的槽位要去掉标记并给出提示标记', () => {
        // CLUSTER NODES 形如 "0-2729 [100->-<id>]"
        expect(parseSlots('0-2729 [100->-605a6440]')).toEqual({ text: '0-2729', migrating: true })
    })

    it('导入中的槽位同样识别为迁移中', () => {
        expect(parseSlots('2730-5460 [100-<-605a6440]')).toEqual({ text: '2730-5460', migrating: true })
    })

    it('空值安全', () => {
        expect(parseSlots('')).toEqual({ text: '', migrating: false })
        expect(parseSlots(null)).toEqual({ text: '', migrating: false })
    })
})
