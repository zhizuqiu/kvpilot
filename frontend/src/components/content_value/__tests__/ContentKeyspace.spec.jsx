import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import zhCn from '@/langs/zh-cn.json'

const api = vi.hoisted(() => ({
    GetKeyspaceStats: vi.fn(),
    ExportCSV: vi.fn(),
    SaveFile: vi.fn(),
}))
vi.mock('wailsjs/go/services/browserService.js', () => api)
vi.mock('wailsjs/go/services/systemService.js', () => ({ SaveFile: api.SaveFile }))

import ContentKeyspace from '@/components/content_value/ContentKeyspace.vue'

const stats = (overrides = {}) => ({
    success: true,
    data: {
        nodes: ['10.0.0.1:6379'],
        databases: [
            { node: '10.0.0.1:6379', db: 0, keys: 500, expires: 120, avgTtl: 3600000 },
            { node: '10.0.0.1:6379', db: 1, keys: 20, expires: 0, avgTtl: 0 },
        ],
        unavailable: [],
        sampled: 100,
        noTtl: 60,
        withTtl: 40,
        types: [
            { type: 'string', count: 70 },
            { type: 'hash', count: 30 },
        ],
        prefixes: [{ prefix: 'demo', count: 80 }],
        elapsedMs: 120,
        ...overrides,
    },
})

const mountKeyspace = async (props = {}) => {
    window.$message = { warning: vi.fn(), error: vi.fn(), success: vi.fn(), info: vi.fn() }
    const i18n = createI18n({ legacy: false, locale: 'zh-cn', messages: { 'zh-cn': zhCn } })
    const wrapper = mount(ContentKeyspace, {
        props: { server: 'test-server', pause: false, ...props },
        global: { plugins: [i18n] },
    })
    await flushPromises()
    return wrapper
}

describe('ContentKeyspace', () => {
    beforeEach(() => {
        vi.clearAllMocks()
        api.GetKeyspaceStats.mockResolvedValue(stats())
    })

    it('展示数据库规模、采样与 TTL 占比', async () => {
        const wrapper = await mountKeyspace()

        expect(api.GetKeyspaceStats).toHaveBeenCalledWith('test-server', 1000)
        expect(wrapper.text()).toContain('共 520 个键')
        expect(wrapper.text()).toContain('已采样 100')
        expect(wrapper.text()).toContain('TTL 40.0%')
        expect(wrapper.text()).not.toContain('%%')
        expect(wrapper.text()).toContain('10.0.0.1:6379')

        wrapper.unmount()
    })

    it('展示类型与前缀分布', async () => {
        const wrapper = await mountKeyspace()

        expect(wrapper.text()).toContain('类型分布（采样）')
        expect(wrapper.text()).toContain('string')
        expect(wrapper.text()).toContain('70.0%')
        expect(wrapper.text()).toContain('前缀 Top 10（采样）')
        expect(wrapper.text()).toContain('demo')

        wrapper.unmount()
    })

    it('节点不可达时给出提示', async () => {
        api.GetKeyspaceStats.mockResolvedValue(stats({ unavailable: ['10.0.0.9:6379'] }))
        const wrapper = await mountKeyspace()

        expect(wrapper.text()).toContain('1 个节点未响应')

        wrapper.unmount()
    })

    it('没有数据时展示空态', async () => {
        api.GetKeyspaceStats.mockResolvedValue({
            success: true,
            data: { nodes: [], databases: [], unavailable: [], sampled: 0, noTtl: 0, withTtl: 0, types: [], prefixes: [], elapsedMs: 0 },
        })
        const wrapper = await mountKeyspace()

        expect(wrapper.text()).toContain('暂无键空间数据')

        wrapper.unmount()
    })

    it('页面未激活时不主动统计，激活后再加载', async () => {
        const wrapper = await mountKeyspace({ pause: true })
        expect(api.GetKeyspaceStats).not.toHaveBeenCalled()

        await wrapper.setProps({ pause: false })
        await flushPromises()
        expect(api.GetKeyspaceStats).toHaveBeenCalled()

        wrapper.unmount()
    })

    it('桌面端导出把数据库规模写入选择好的路径', async () => {
        api.SaveFile.mockResolvedValue({ success: true, data: { path: '/tmp/keyspace.csv' } })
        api.ExportCSV.mockResolvedValue({ success: true, data: { count: 2 } })
        const wrapper = await mountKeyspace()

        const exportButton = wrapper.findAll('button').find((button) => button.text().includes('导出 CSV'))
        await exportButton.trigger('click')
        await flushPromises()

        expect(api.ExportCSV).toHaveBeenCalledWith(
            '/tmp/keyspace.csv',
            ['node', 'db', 'keys', 'expires', 'avg_ttl'],
            expect.arrayContaining([[ '10.0.0.1:6379', 0, 500, 120, 3600000 ]])
        )
        expect(window.$message.success).toHaveBeenCalled()

        wrapper.unmount()
    })
})
