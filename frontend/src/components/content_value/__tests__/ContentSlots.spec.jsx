import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import zhCn from '@/langs/zh-cn.json'

const api = vi.hoisted(() => ({
    SlotOverview: vi.fn(),
    PreviewSlotMigration: vi.fn(),
    StartSlotMigration: vi.fn(),
    GetSlotMigration: vi.fn(),
    PauseSlotMigration: vi.fn(),
    ResumeSlotMigration: vi.fn(),
    CancelSlotMigration: vi.fn(),
    Topology: vi.fn(),
}))
vi.mock('wailsjs/go/services/browserService.js', () => api)

import ContentSlots from '@/components/content_value/ContentSlots.vue'

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

const previewData = (overrides = {}) => ({
    canStart: true,
    source: { id: 'master-a', addr: '10.0.0.1:6379' },
    target: { id: 'master-b', addr: '10.0.0.2:6379' },
    slot: 0,
    keyCount: 10,
    sampledKeys: 10,
    estimatedBytes: 1024,
    targetUsed: 1,
    targetMax: 0,
    resume: false,
    warnings: [],
    ...overrides,
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

describe('ContentSlots', () => {
    beforeEach(() => {
        vi.clearAllMocks()
        api.SlotOverview.mockResolvedValue(overviewData())
        api.GetSlotMigration.mockResolvedValue({ success: true, data: taskData() })
        api.Topology.mockResolvedValue({ success: true, data: { mode: 'cluster', nodes: [] } })
        api.PreviewSlotMigration.mockResolvedValue({ success: true, data: previewData() })
        api.StartSlotMigration.mockResolvedValue({
            success: true,
            data: taskData({ state: 'running', step: 'preparing', slot: 0, totalKeys: 10 }),
        })
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

    it('预览展示键数与告警', async () => {
        api.PreviewSlotMigration.mockResolvedValue({
            success: true,
            data: previewData({
                warnings: [
                    { code: 'large_key', level: 'warn', values: { key: 'big:key', size: '120MB' } },
                ],
            }),
        })
        const wrapper = await mountSlots()

        await findButton(wrapper, '预览迁移').trigger('click')
        await flushPromises()

        expect(api.PreviewSlotMigration).toHaveBeenCalledWith('test-server', 'master-a', 'master-b', 0)
        expect(wrapper.text()).toContain('10 个键')
        expect(wrapper.text()).toContain('big:key')
        expect(findButton(wrapper, '开始迁移').attributes('disabled')).toBeUndefined()

        wrapper.unmount()
    })

    it('硬性错误时禁用开始按钮', async () => {
        api.PreviewSlotMigration.mockResolvedValue({
            success: true,
            data: previewData({ canStart: false, warnings: [{ code: 'cluster_state', level: 'error', values: { state: 'fail' } }] }),
        })
        const wrapper = await mountSlots()

        await findButton(wrapper, '预览迁移').trigger('click')
        await flushPromises()

        expect(findButton(wrapper, '开始迁移').attributes('disabled')).toBeDefined()

        wrapper.unmount()
    })

    it('开始迁移后轮询到完成', async () => {
        vi.useFakeTimers()
        try {
            api.GetSlotMigration
                .mockResolvedValueOnce({ success: true, data: taskData() })
                .mockResolvedValue({
                    success: true,
                    data: taskData({ state: 'completed', step: 'done', slot: 0, totalKeys: 10, movedKeys: 10 }),
                })
            const wrapper = await mountSlots()

            await findButton(wrapper, '预览迁移').trigger('click')
            await flushPromises()
            await findButton(wrapper, '开始迁移').trigger('click')
            await flushPromises()

            expect(api.StartSlotMigration).toHaveBeenCalledWith(
                'test-server',
                'master-a',
                'master-b',
                0,
                expect.objectContaining({ batchKeys: 100, migrateTimeoutMs: 5000 })
            )

            await vi.advanceTimersByTimeAsync(800)
            await flushPromises()

            expect(wrapper.text()).toContain('已完成')
            expect(wrapper.text()).toContain('已迁移 10/10')

            wrapper.unmount()
        } finally {
            vi.useRealTimers()
        }
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
