import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { h } from 'vue'
import { NCheckbox, NConfigProvider, NMessageProvider, NSelect } from 'naive-ui'
import { createI18n } from 'vue-i18n'
import zhCn from '@/langs/zh-cn.json'

const api = vi.hoisted(() => ({ ListSessions: vi.fn(), KillSession: vi.fn() }))
vi.mock('wailsjs/go/services/browserService.js', () => api)

import ContentSession from '@/components/content_value/ContentSession.vue'

const session = (overrides) => ({
    node: '10.0.0.1:6379',
    id: 1,
    addr: '10.0.0.9:50001',
    name: '',
    user: 'default',
    db: 0,
    age: 120,
    idle: 0,
    flags: 'N',
    cmd: 'client|list',
    protected: false,
    ...overrides,
})

const buildData = (overrides = {}) => ({
    success: true,
    data: {
        nodes: ['10.0.0.1:6379', '10.0.0.2:6379'],
        unavailable: [],
        total: 3,
        idle: 1,
        sources: [
            { addr: '10.0.0.9', count: 2 },
            { addr: '10.0.0.8', count: 1 },
        ],
        sessions: [
            session({}),
            session({ id: 2, addr: '10.0.0.9:50002', name: 'worker', user: 'app', db: 2, age: 300, idle: 120, flags: 'P', cmd: 'subscribe' }),
            session({ node: '10.0.0.2:6379', id: 3, addr: '10.0.0.8:50003', name: 'replica', age: 600, flags: 'S', cmd: 'psync', protected: true }),
        ],
        ...overrides,
    },
})

const mountSession = async () => {
    window.$message = { warning: vi.fn(), error: vi.fn(), success: vi.fn(), info: vi.fn() }
    window.$dialog = { warning: vi.fn() }
    const i18n = createI18n({ legacy: false, locale: 'zh-cn', messages: { 'zh-cn': zhCn } })
    const wrapper = mount(
        {
            render: () =>
                h(NConfigProvider, null, () =>
                    h(NMessageProvider, null, () => h(ContentSession, { server: 'test-server' }))
                ),
        },
        { global: { plugins: [i18n] } }
    )
    await flushPromises()
    return wrapper
}

const bodyRows = (wrapper) => wrapper.findAll('.n-data-table-tbody .n-data-table-tr')

describe('ContentSession', () => {
    beforeEach(() => {
        vi.clearAllMocks()
        api.ListSessions.mockResolvedValue(buildData())
    })

    it('渲染会话列表与概览统计', async () => {
        const wrapper = await mountSession()

        expect(wrapper.text()).toContain('共 3 个会话')
        expect(wrapper.text()).toContain('空闲 ≥60s：1 个')
        expect(bodyRows(wrapper)).toHaveLength(3)
        expect(wrapper.text()).toContain('10.0.0.9:50001')
        expect(wrapper.text()).toContain('10.0.0.8:50003')

        wrapper.unmount()
    })

    it('关键字筛选地址、名称与命令', async () => {
        const wrapper = await mountSession()

        await wrapper.find('.session-search input').setValue('worker')
        await flushPromises()
        expect(bodyRows(wrapper)).toHaveLength(1)
        expect(wrapper.text()).toContain('10.0.0.9:50002')

        wrapper.unmount()
    })

    it('仅空闲筛选只保留空闲超过阈值的会话', async () => {
        const wrapper = await mountSession()

        wrapper.findComponent(NCheckbox).vm.$emit('update:checked', true)
        await flushPromises()
        expect(bodyRows(wrapper)).toHaveLength(1)
        expect(wrapper.text()).toContain('10.0.0.9:50002')

        wrapper.unmount()
    })

    it('按 DB 筛选只保留对应库的会话', async () => {
        const wrapper = await mountSession()

        wrapper.findAllComponents(NSelect)[0].vm.$emit('update:value', 2)
        await flushPromises()
        expect(bodyRows(wrapper)).toHaveLength(1)
        expect(wrapper.text()).toContain('10.0.0.9:50002')

        wrapper.unmount()
    })

    it('受保护的主从链路不能关闭', async () => {
        const wrapper = await mountSession()

        const protectedRow = bodyRows(wrapper).find((row) => row.text().includes('受保护'))
        expect(protectedRow).toBeTruthy()
        expect(protectedRow.find('button').attributes('disabled')).toBeDefined()

        wrapper.unmount()
    })

    it('关闭会话必须确认后才调用接口', async () => {
        const wrapper = await mountSession()
        const row = bodyRows(wrapper).find((item) => item.text().includes('10.0.0.9:50001'))

        await row.find('button').trigger('click')
        await flushPromises()
        expect(api.KillSession).not.toHaveBeenCalled()

        const confirm = window.$dialog.warning.mock.calls[0][1]
        api.KillSession.mockResolvedValue({ success: true, data: { killed: 1 } })
        await confirm()
        await flushPromises()

        expect(api.KillSession).toHaveBeenCalledWith('test-server', '10.0.0.1:6379', 1, '10.0.0.9:50001')
        expect(window.$message.success).toHaveBeenCalled()

        wrapper.unmount()
    })

    it('关闭失败时提示原因', async () => {
        const wrapper = await mountSession()
        const row = bodyRows(wrapper).find((item) => item.text().includes('10.0.0.9:50001'))

        await row.find('button').trigger('click')
        const confirm = window.$dialog.warning.mock.calls[0][1]
        api.KillSession.mockResolvedValue({ success: false, msg: 'NOPERM' })
        await confirm()
        await flushPromises()

        expect(window.$message.error).toHaveBeenCalledWith('NOPERM')

        wrapper.unmount()
    })

    it('节点不可达时提示未响应数量', async () => {
        api.ListSessions.mockResolvedValue(buildData({ unavailable: ['10.0.0.3:6379'] }))
        const wrapper = await mountSession()

        expect(wrapper.text()).toContain('1 个节点未响应')

        wrapper.unmount()
    })

    it('加载失败时提示错误而不是空白列表', async () => {
        api.ListSessions.mockResolvedValue({ success: false, msg: 'get client list fail' })
        const wrapper = await mountSession()

        expect(window.$message.warning).toHaveBeenCalledWith('get client list fail')

        wrapper.unmount()
    })
})
