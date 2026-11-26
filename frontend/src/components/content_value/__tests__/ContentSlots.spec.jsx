import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { NSelect } from 'naive-ui'
import zhCn from '@/langs/zh-cn.json'

const clipboard = vi.hoisted(() => vi.fn())
vi.mock('wailsjs/runtime/runtime.js', () => ({ ClipboardSetText: clipboard }))

const api = vi.hoisted(() => ({
    SlotOverview: vi.fn(),
    GetSlotMigration: vi.fn(),
    SlotRebalancePlan: vi.fn(),
    StartSlotRebalance: vi.fn(),
    PauseSlotMigration: vi.fn(),
    ResumeSlotMigration: vi.fn(),
    CancelSlotMigration: vi.fn(),
    Topology: vi.fn(),
    GetSlotDistribution: vi.fn(),
    ScanSlotDistribution: vi.fn(),
}))
vi.mock('wailsjs/go/services/browserService.js', () => api)

import ContentSlots from '@/components/content_value/ContentSlots.vue'
import SlotDistribution from '@/components/content_value/SlotDistribution.vue'

const overviewData = (overrides = {}) => ({
    success: true,
    data: {
        mode: 'cluster',
        state: 'ok',
        slotsAssigned: 16384,
        knownNodes: 6,
        migratingSlots: [],
        unavailable: [],
        masters: [
            {
                id: 'master-a',
                addr: '10.0.0.1:6379',
                slotRanges: [{ start: 0, end: 5460 }],
                slotCount: 5461,
                keys: 100,
                usedMemory: 1048576,
                maxMemory: 0,
            },
            {
                id: 'master-b',
                addr: '10.0.0.2:6379',
                slotRanges: [{ start: 5461, end: 10922 }],
                slotCount: 5462,
                keys: 200,
                usedMemory: 2097152,
                maxMemory: 0,
            },
        ],
        ...overrides,
    },
})

const taskData = (overrides = {}) => ({
    state: 'idle',
    step: '',
    slot: 0,
    source: {},
    target: {},
    totalKeys: 0,
    movedKeys: 0,
    movedBytes: 0,
    elapsedMs: 0,
    error: '',
    recovery: [],
    ...overrides,
})

const mountSlots = async () => {
    window.$message = { warning: vi.fn(), error: vi.fn(), success: vi.fn(), info: vi.fn() }
    window.$dialog = { warning: vi.fn((msg, cb) => cb && cb()) }
    const i18n = createI18n({ legacy: false, locale: 'zh-cn', messages: { 'zh-cn': zhCn } })
    const wrapper = mount(ContentSlots, {
        props: { server: 'test-server', pause: false },
        global: { plugins: [i18n] },
    })
    await flushPromises()
    return wrapper
}

const findButton = (wrapper, text) => wrapper.findAll('button').find((button) => button.text().includes(text))

const switchTab = async (wrapper, label) => {
    const tab = wrapper.findAll('.n-tabs-tab').find((item) => item.text().includes(label))
    await tab.trigger('click')
    await flushPromises()
}

describe('ContentSlots', () => {
    beforeEach(() => {
        vi.clearAllMocks()
        api.SlotOverview.mockResolvedValue(overviewData())
        api.GetSlotMigration.mockResolvedValue({ success: true, data: taskData() })
        api.Topology.mockResolvedValue({ success: true, data: { mode: 'cluster', nodes: [] } })
        api.GetSlotDistribution.mockResolvedValue({
            success: true,
            data: { slots: new Array(16384).fill(0), nodes: [], scannedAt: 0, scanning: false },
        })
        api.ScanSlotDistribution.mockResolvedValue({
            success: true,
            data: { slots: new Array(16384).fill(0), nodes: [], scannedAt: 0, scanning: false },
        })
        api.StartSlotRebalance.mockResolvedValue({
            success: true,
            data: taskData({ state: 'running', step: 'preparing', batch: true, totalSlots: 2 }),
        })
    })

    it('可以复制单个节点与全部节点的槽位范围', async () => {
        const wrapper = await mountSlots()

        const copyButton = wrapper.findAll('button').find((button) => button.text().includes('复制全部槽位'))
        await copyButton.trigger('click')
        await flushPromises()
        expect(clipboard).toHaveBeenCalledWith(expect.stringContaining('10.0.0.1:6379\t0-5460'))

        clipboard.mockClear()
        const rows = wrapper.findAll('.slot-masters .n-data-table-tbody .n-data-table-tr')
        await rows[0].find('.slot-range-copy').trigger('click')
        await flushPromises()
        expect(clipboard).toHaveBeenCalledWith('0-5460')

        wrapper.unmount()
    })

    it('批量迁移 tab 下点击迁移此槽会加入列表', async () => {
        const wrapper = await mountSlots()
        await switchTab(wrapper, '批量迁移')

        wrapper.findComponent(SlotDistribution).vm.$emit('migrate', {
            slot: 5461,
            nodeId: 'master-b',
            targetId: 'master-a',
            targetAddr: '10.0.0.1:6379',
            keys: 5,
        })
        await flushPromises()

        expect(wrapper.text()).toContain('1 个待迁移槽位 · 5 键')
        expect(wrapper.findAll('.slot-plan-table .n-data-table-tbody .n-data-table-tr').length).toBe(1)

        wrapper.unmount()
    })

    it('重复加入同一槽位会被忽略并提示', async () => {
        const wrapper = await mountSlots()
        await switchTab(wrapper, '批量迁移')

        const distribution = wrapper.findComponent(SlotDistribution)
        distribution.vm.$emit('migrate', { slot: 5461, nodeId: 'master-b', targetId: 'master-a', targetAddr: '10.0.0.1:6379', keys: 5 })
        distribution.vm.$emit('migrate', { slot: 5461, nodeId: 'master-b', targetId: 'master-a', targetAddr: '10.0.0.1:6379', keys: 5 })
        await flushPromises()

        expect(wrapper.text()).toContain('1 个待迁移槽位')
        expect(window.$message.info).toHaveBeenCalledWith('槽位 5461 已在列表中')

        wrapper.unmount()
    })

    it('开始批量迁移时按源节点合并 move', async () => {
        const wrapper = await mountSlots()
        await switchTab(wrapper, '批量迁移')

        const distribution = wrapper.findComponent(SlotDistribution)
        distribution.vm.$emit('migrate', { slot: 5461, nodeId: 'master-b', targetId: 'master-a', targetAddr: '10.0.0.1:6379', keys: 5 })
        distribution.vm.$emit('migrate', { slot: 5462, nodeId: 'master-b', targetId: 'master-a', targetAddr: '10.0.0.1:6379', keys: 2 })
        await flushPromises()

        await findButton(wrapper, '开始批量迁移').trigger('click')
        await flushPromises()

        expect(api.StartSlotRebalance).toHaveBeenCalledTimes(1)
        const [server, moves, options] = api.StartSlotRebalance.mock.calls[0]
        expect(server).toBe('test-server')
        expect(moves).toEqual([
            {
                source: { id: 'master-b', addr: '10.0.0.2:6379' },
                target: { id: 'master-a', addr: '10.0.0.1:6379' },
                slots: [5461, 5462],
                approxKeys: 7,
            },
        ])
        expect(options).toMatchObject({ batchKeys: 100, migrateTimeoutMs: 5000 })

        wrapper.unmount()
    })

    it('多个源节点可以迁移到同一个目标节点', async () => {
        api.SlotOverview.mockResolvedValue(
            overviewData({
                masters: [
                    {
                        id: 'master-a',
                        addr: '10.0.0.1:6379',
                        slotRanges: [{ start: 0, end: 5460 }],
                        slotCount: 5461,
                        keys: 100,
                        usedMemory: 1048576,
                        maxMemory: 0,
                    },
                    {
                        id: 'master-b',
                        addr: '10.0.0.2:6379',
                        slotRanges: [{ start: 5461, end: 10922 }],
                        slotCount: 5462,
                        keys: 200,
                        usedMemory: 2097152,
                        maxMemory: 0,
                    },
                    {
                        id: 'master-c',
                        addr: '10.0.0.3:6379',
                        slotRanges: [{ start: 10923, end: 16383 }],
                        slotCount: 5461,
                        keys: 300,
                        usedMemory: 1048576,
                        maxMemory: 0,
                    },
                ],
            })
        )
        const wrapper = await mountSlots()
        await switchTab(wrapper, '批量迁移')

        const distribution = wrapper.findComponent(SlotDistribution)
        // 两条记录分别来自 master-a / master-c，但都选同一个目标 master-b
        distribution.vm.$emit('migrate', {
            slot: 42,
            nodeId: 'master-a',
            targetId: 'master-b',
            targetAddr: '10.0.0.2:6379',
            keys: 1,
        })
        distribution.vm.$emit('migrate', {
            slot: 10923,
            nodeId: 'master-c',
            targetId: 'master-b',
            targetAddr: '10.0.0.2:6379',
            keys: 2,
        })
        await flushPromises()

        expect(wrapper.text()).toContain('2 个待迁移槽位 · 3 键')
        await findButton(wrapper, '开始批量迁移').trigger('click')
        await flushPromises()

        const [, moves] = api.StartSlotRebalance.mock.calls[0]
        expect(moves).toHaveLength(2)
        expect(moves.map((move) => move.source.id).sort()).toEqual(['master-a', 'master-c'])
        expect(moves.every((move) => move.target.id === 'master-b')).toBe(true)

        wrapper.unmount()
    })

    it('槽位号输入支持逗号分隔与区间批量添加', async () => {
        const wrapper = await mountSlots()
        await switchTab(wrapper, '批量迁移')

        const input = wrapper.find('.slot-slots-input input')
        await input.setValue('0-6, 8')
        await findButton(wrapper, '添加').trigger('click')
        await flushPromises()

        expect(wrapper.text()).toContain('8 个待迁移槽位 · 0 键')
        expect(window.$message.success).toHaveBeenCalledWith('已加入 8 个槽位')

        wrapper.unmount()
    })

    it('同源同目标的槽位合并成一行，移除会删除整组', async () => {
        const wrapper = await mountSlots()
        await switchTab(wrapper, '批量迁移')

        const input = wrapper.find('.slot-slots-input input')
        await input.setValue('0-2, 4')
        await findButton(wrapper, '添加').trigger('click')
        await flushPromises()

        const rows = wrapper.findAll('.slot-plan-table .n-data-table-tbody .n-data-table-tr')
        expect(rows).toHaveLength(1)
        expect(rows[0].text()).toContain('0-2, 4')

        await rows[0].find('button').trigger('click')
        await flushPromises()
        expect(wrapper.text()).toContain('0 个待迁移槽位')

        wrapper.unmount()
    })

    it('槽位号输入非法时整体拒绝并提示', async () => {
        const wrapper = await mountSlots()
        await switchTab(wrapper, '批量迁移')

        const input = wrapper.find('.slot-slots-input input')
        await input.setValue('0-6, x')
        await findButton(wrapper, '添加').trigger('click')
        await flushPromises()

        expect(window.$message.warning).toHaveBeenCalledWith('无法解析槽位号：x')
        expect(wrapper.text()).toContain('0 个待迁移槽位')

        wrapper.unmount()
    })

    it('框选多个槽位可以一次性加入批量列表', async () => {
        api.SlotOverview.mockResolvedValue(
            overviewData({
                masters: [
                    { id: 'master-a', addr: '10.0.0.1:6379', slotRanges: [{ start: 0, end: 5460 }], slotCount: 5461, keys: 100, usedMemory: 1, maxMemory: 0 },
                    { id: 'master-b', addr: '10.0.0.2:6379', slotRanges: [{ start: 5461, end: 10922 }], slotCount: 5462, keys: 200, usedMemory: 1, maxMemory: 0 },
                    { id: 'master-c', addr: '10.0.0.3:6379', slotRanges: [{ start: 10923, end: 16383 }], slotCount: 5461, keys: 300, usedMemory: 1, maxMemory: 0 },
                ],
            })
        )
        const wrapper = await mountSlots()
        await switchTab(wrapper, '批量迁移')

        wrapper.findComponent(SlotDistribution).vm.$emit('migrate', {
            items: [
                { slot: 42, nodeId: 'master-a', keys: 2 },
                { slot: 10923, nodeId: 'master-c', keys: 5 },
            ],
            targetId: 'master-b',
            targetAddr: '10.0.0.2:6379',
        })
        await flushPromises()

        expect(wrapper.text()).toContain('2 个待迁移槽位 · 7 键')
        expect(window.$message.success).toHaveBeenCalledWith('已加入 2 个槽位')
        expect(wrapper.findAll('.slot-plan-table .n-data-table-tbody .n-data-table-tr')).toHaveLength(2)

        wrapper.unmount()
    })

    it('框选结果里不可迁移的槽位会跳过并汇总提示', async () => {
        const wrapper = await mountSlots()
        await switchTab(wrapper, '批量迁移')

        wrapper.findComponent(SlotDistribution).vm.$emit('migrate', {
            items: [
                { slot: 5461, nodeId: 'master-b', keys: 1 },
                { slot: 5462, nodeId: 'master-b', keys: 1 },
            ],
            targetId: 'master-b',
            targetAddr: '10.0.0.2:6379',
        })
        await flushPromises()

        expect(wrapper.text()).toContain('0 个待迁移槽位')
        expect(window.$message.warning).toHaveBeenCalledWith('已跳过 2 个不可迁移的槽位')

        wrapper.unmount()
    })

    it('目标节点与槽位所属节点相同时拒绝并提示', async () => {
        const wrapper = await mountSlots()
        await switchTab(wrapper, '批量迁移')

        wrapper.findComponent(SlotDistribution).vm.$emit('migrate', {
            slot: 5461,
            nodeId: 'master-b',
            targetId: 'master-b',
            targetAddr: '10.0.0.2:6379',
            keys: 1,
        })
        await flushPromises()

        expect(window.$message.warning).toHaveBeenCalledWith('目标节点不能与源节点相同')
        expect(wrapper.text()).toContain('0 个待迁移槽位')

        wrapper.unmount()
    })

    it('加入批量列表前校验槽位归属，不属于源节点时拒绝', async () => {
        const wrapper = await mountSlots()
        await switchTab(wrapper, '批量迁移')

        wrapper.findComponent(SlotDistribution).vm.$emit('migrate', {
            slot: 42,
            nodeId: 'master-b',
            targetId: 'master-a',
            targetAddr: '10.0.0.1:6379',
            keys: 0,
        })
        await flushPromises()

        expect(window.$message.warning).toHaveBeenCalledWith('槽位 42 不属于所选源节点')
        expect(wrapper.text()).toContain('0 个待迁移槽位')

        wrapper.unmount()
    })

    it('加入批量列表前校验迁移中状态', async () => {
        api.SlotOverview.mockResolvedValue(overviewData({ migratingSlots: [5461] }))
        const wrapper = await mountSlots()
        await switchTab(wrapper, '批量迁移')

        wrapper.findComponent(SlotDistribution).vm.$emit('migrate', {
            slot: 5461,
            nodeId: 'master-b',
            targetId: 'master-a',
            targetAddr: '10.0.0.1:6379',
            keys: 0,
        })
        await flushPromises()

        expect(window.$message.warning).toHaveBeenCalledWith('槽位 5461 正在迁移，不能加入列表')
        expect(wrapper.text()).toContain('0 个待迁移槽位')

        wrapper.unmount()
    })

    it('集群状态异常时拒绝加入批量列表', async () => {
        api.SlotOverview.mockResolvedValue(overviewData({ state: 'fail' }))
        const wrapper = await mountSlots()
        await switchTab(wrapper, '批量迁移')

        wrapper.findComponent(SlotDistribution).vm.$emit('migrate', {
            slot: 5461,
            nodeId: 'master-b',
            targetId: 'master-a',
            targetAddr: '10.0.0.1:6379',
            keys: 0,
        })
        await flushPromises()

        expect(window.$message.warning).toHaveBeenCalledWith('集群状态为 fail，暂不能迁移')
        expect(wrapper.text()).toContain('0 个待迁移槽位')

        wrapper.unmount()
    })

    it('在均衡 tab 点击迁移此槽会切到批量并加入列表', async () => {
        const wrapper = await mountSlots()
        await switchTab(wrapper, '均衡迁移')

        wrapper.findComponent(SlotDistribution).vm.$emit('migrate', {
            slot: 5461,
            nodeId: 'master-b',
            targetId: 'master-a',
            targetAddr: '10.0.0.1:6379',
            keys: 3,
        })
        await flushPromises()

        const activeTabs = wrapper.findAll('.n-tabs-tab--active').map((tab) => tab.text())
        expect(activeTabs.some((text) => text.includes('批量迁移'))).toBe(true)
        expect(wrapper.text()).toContain('1 个待迁移槽位 · 3 键')

        wrapper.unmount()
    })

    it('非集群连接给出提示', async () => {
        api.SlotOverview.mockResolvedValue(overviewData({ mode: 'standalone', masters: [] }))
        const wrapper = await mountSlots()

        expect(wrapper.text()).toContain('槽位管理仅支持集群连接')

        wrapper.unmount()
    })

    it('展示主节点槽位分布', async () => {
        const wrapper = await mountSlots()

        expect(wrapper.text()).toContain('10.0.0.1:6379')
        expect(wrapper.text()).toContain('0-5460')
        expect(wrapper.text()).toContain('5461-10922')

        wrapper.unmount()
    })

    it('批量迁移后轮询到完成并重扫分布', async () => {
        vi.useFakeTimers()
        try {
            api.GetSlotMigration
                .mockResolvedValueOnce({ success: true, data: taskData() })
                .mockResolvedValue({
                    success: true,
                    data: taskData({
                        state: 'completed',
                        step: 'done',
                        slot: 5461,
                        totalKeys: 10,
                        movedKeys: 10,
                        batch: true,
                        totalSlots: 1,
                        doneSlots: 1,
                    }),
                })
            api.StartSlotRebalance.mockResolvedValue({
                success: true,
                data: taskData({ state: 'running', step: 'preparing', batch: true, totalSlots: 1, totalKeys: 10 }),
            })
            const wrapper = await mountSlots()
            await switchTab(wrapper, '批量迁移')

            wrapper.findComponent(SlotDistribution).vm.$emit('migrate', {
                slot: 5461,
                nodeId: 'master-b',
                targetId: 'master-a',
                targetAddr: '10.0.0.1:6379',
                keys: 10,
            })
            await flushPromises()

            await findButton(wrapper, '开始批量迁移').trigger('click')
            await flushPromises()
            expect(api.StartSlotRebalance).toHaveBeenCalledWith(
                'test-server',
                [
                    {
                        source: { id: 'master-b', addr: '10.0.0.2:6379' },
                        target: { id: 'master-a', addr: '10.0.0.1:6379' },
                        slots: [5461],
                        approxKeys: 10,
                    },
                ],
                expect.objectContaining({ batchKeys: 100, migrateTimeoutMs: 5000 })
            )

            await vi.advanceTimersByTimeAsync(800)
            await flushPromises()

            expect(wrapper.text()).toContain('已完成')
            expect(wrapper.text()).toContain('已迁移 10/10')
            // 迁移完成后 Map 归属已变化，应自动重新扫描分布而不是保留旧快照。
            expect(api.ScanSlotDistribution).toHaveBeenCalledWith('test-server')

            wrapper.unmount()
        } finally {
            vi.useRealTimers()
        }
    })

    it('生成均衡计划并展示迁移项', async () => {
        api.SlotRebalancePlan.mockResolvedValue({
            success: true,
            data: {
                balanceTarget: 5461,
                totalSlots: 2,
                moves: [
                    {
                        source: { id: 'master-a', addr: '10.0.0.1:6379' },
                        target: { id: 'master-b', addr: '10.0.0.2:6379' },
                        slots: [1, 2],
                        approxKeys: 10,
                    },
                ],
            },
        })
        const wrapper = await mountSlots()

        await findButton(wrapper, '生成均衡计划').trigger('click')
        await flushPromises()

        expect(api.SlotRebalancePlan).toHaveBeenCalledWith('test-server', 'slots', 100, 10)
        expect(wrapper.text()).toContain('计划迁移 2 个')
        expect(wrapper.text()).toContain('10.0.0.2:6379')
        expect(findButton(wrapper, '开始均衡').attributes('disabled')).toBeUndefined()

        wrapper.unmount()
    })

    it('切换为按键数量均衡时按新策略生成计划', async () => {
        api.SlotRebalancePlan.mockResolvedValue({
            success: true,
            data: {
                strategy: 'keys',
                balanceTarget: 800,
                totalSlots: 1,
                moves: [
                    {
                        source: { id: 'master-a', addr: '10.0.0.1:6379' },
                        target: { id: 'master-b', addr: '10.0.0.2:6379' },
                        slots: [10],
                        approxKeys: 600,
                    },
                ],
            },
        })
        const wrapper = await mountSlots()

        const strategySelect = wrapper
            .findAllComponents(NSelect)
            .find((select) => (select.props('options') || []).some((option) => option.value === 'keys'))
        strategySelect.vm.$emit('update:value', 'keys')
        await flushPromises()
        await findButton(wrapper, '生成均衡计划').trigger('click')
        await flushPromises()

        expect(api.SlotRebalancePlan).toHaveBeenCalledWith('test-server', 'keys', 100, 10)
        expect(wrapper.text()).toContain('目标每节点约 800 个键')

        wrapper.unmount()
    })

    it('开始均衡按计划调用接口并展示批量进度', async () => {
        api.SlotRebalancePlan.mockResolvedValue({
            success: true,
            data: {
                balanceTarget: 5461,
                totalSlots: 2,
                moves: [
                    {
                        source: { id: 'master-a', addr: '10.0.0.1:6379' },
                        target: { id: 'master-b', addr: '10.0.0.2:6379' },
                        slots: [1, 2],
                        approxKeys: 10,
                    },
                ],
            },
        })
        api.StartSlotRebalance.mockResolvedValue({
            success: true,
            data: taskData({ state: 'running', batch: true, totalSlots: 2, doneSlots: 0, currentSlot: 1 }),
        })
        const wrapper = await mountSlots()

        await findButton(wrapper, '生成均衡计划').trigger('click')
        await flushPromises()
        await findButton(wrapper, '开始均衡').trigger('click')
        await flushPromises()

        expect(api.StartSlotRebalance).toHaveBeenCalledWith(
            'test-server',
            expect.arrayContaining([expect.objectContaining({ approxKeys: 10 })]),
            expect.objectContaining({ batchKeys: 100 })
        )
        expect(wrapper.text()).toContain('已完成 0/2 个槽位')
        expect(wrapper.text()).toContain('当前槽位 1')

        wrapper.unmount()
    })

    it('运行中可以暂停', async () => {
        api.GetSlotMigration.mockResolvedValue({
            success: true,
            data: taskData({ state: 'running', step: 'migrating', slot: 0, totalKeys: 10 }),
        })
        api.PauseSlotMigration.mockResolvedValue({
            success: true,
            data: taskData({ state: 'paused', step: 'migrating', slot: 0, totalKeys: 10, movedKeys: 2 }),
        })
        const wrapper = await mountSlots()

        await findButton(wrapper, '暂停').trigger('click')
        await flushPromises()

        expect(api.PauseSlotMigration).toHaveBeenCalledWith('test-server')
        expect(wrapper.text()).toContain('已暂停')

        wrapper.unmount()
    })
})
