import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { NConfigProvider } from 'naive-ui'
import { createI18n } from 'vue-i18n'
import zhCn from '@/langs/zh-cn.json'

const api = vi.hoisted(() => ({ Topology: vi.fn() }))
vi.mock('wailsjs/go/services/browserService.js', () => api)

import ContentDeployTopology from '@/components/content_value/ContentDeployTopology.vue'

const topology = (nodes) => ({ success: true, data: { mode: 'cluster', self: 'm1', version: '7.2.14', nodes } })

const master = (addr, slots, extra = {}) => ({
    addr,
    role: 'master',
    state: 'online',
    slots,
    slotStart: Number(String(slots).split('-')[0] || 0),
    ...extra,
})

const replica = (addr, masterAddr, extra = {}) => ({ addr, role: 'replica', state: 'online', master: masterAddr, ...extra })

const mountTopology = async () => {
    window.$message = { warning: vi.fn(), error: vi.fn(), success: vi.fn(), info: vi.fn() }
    const i18n = createI18n({ legacy: false, locale: 'zh-cn', messages: { 'zh-cn': zhCn } })
    const wrapper = mount(
        {
            render: () => (
                <NConfigProvider>
                    <ContentDeployTopology server="test-server" />
                </NConfigProvider>
            ),
        },
        { global: { plugins: [i18n] } }
    )
    await flushPromises()
    return wrapper
}

beforeEach(() => {
    vi.clearAllMocks()
})

describe('ContentDeployTopology', () => {
    it('健康集群不显示提示条，主节点之间用 + 连接', async () => {
        api.Topology.mockResolvedValue(
            topology([master('m1', '0-5460'), master('m2', '5461-10922'), replica('r1', 'm1')])
        )

        const wrapper = await mountTopology()

        expect(wrapper.find('.topology-alert').exists()).toBe(false)
        expect(wrapper.findAll('.group-plus')).toHaveLength(1)
        expect(wrapper.findAll('.topology-group')).toHaveLength(2)

        wrapper.unmount()
    })

    it('离线节点与健康节点同区展示，只有离线节点置灰', async () => {
        api.Topology.mockResolvedValue(
            topology([
                master('m1', '0-5460'),
                master('m2', '5461-10922'),
                master('m3', '', { state: 'down' }),
                replica('r1', 'm1'),
            ])
        )

        const wrapper = await mountTopology()

        // 不再有独立的故障区域
        expect(wrapper.find('.topology-faulty').exists()).toBe(false)

        const cards = wrapper.findAll('.topology-node')
        const down = cards.find((c) => c.text().includes('m3'))
        expect(down).toBeTruthy()
        expect(down.classes()).toContain('down')
        // 无槽位时保留占位，避免卡片高度不一致
        expect(down.text()).toContain('槽位: -')

        const healthy = cards.find((c) => c.text().includes('m1'))
        expect(healthy.classes()).not.toContain('down')

        wrapper.unmount()
    })

    it('主节点或从节点故障时仍然画出复制关系连线', async () => {
        api.Topology.mockResolvedValue(
            topology([
                master('m1', '0-5460'),
                master('m2', '', { state: 'down' }),
                replica('r1', 'm1'),
                replica('r2', 'm2', { state: 'down' }),
            ])
        )

        const wrapper = await mountTopology()
        await flushPromises()

        // 故障主节点也要保留分组，否则连线无从绘制
        expect(wrapper.findAll('.topology-group')).toHaveLength(2)
        expect(wrapper.find('.topology-group[data-key="m2"]').exists()).toBe(true)
        expect(wrapper.find('.topology-group[data-key="m2"]').findAll('.topology-node')).toHaveLength(2)
        expect(wrapper.find('.topology-group[data-key="m2"]').findAll('path').length).toBeGreaterThan(0)

        wrapper.unmount()
    })

    it('主节点已消失的从节点单独标注为失去主节点', async () => {
        api.Topology.mockResolvedValue(
            topology([master('m1', '0-5460'), replica('r1', 'm1'), replica('r9', 'gone:1')])
        )

        const wrapper = await mountTopology()

        const alert = wrapper.find('.topology-alert')
        expect(alert.exists()).toBe(true)
        expect(alert.text()).toContain('1 个从节点失去主节点')
        expect(alert.text()).not.toContain('离线节点')

        const orphan = wrapper.findAll('.topology-node').find((c) => c.text().includes('r9'))
        expect(orphan).toBeTruthy()
        expect(orphan.classes()).not.toContain('down')

        wrapper.unmount()
    })

    it('只有离线节点时不出现从节点相关文案', async () => {
        api.Topology.mockResolvedValue(topology([master('m1', '0-5460'), master('m2', '', { state: 'down' })]))

        const wrapper = await mountTopology()

        const alert = wrapper.find('.topology-alert')
        expect(alert.text()).toContain('1 个离线节点')
        expect(alert.text()).not.toContain('失去主节点')

        wrapper.unmount()
    })

    it('槽位迁移中给出角标，并剥离迁移标记', async () => {
        api.Topology.mockResolvedValue(
            topology([master('m1', '0-2729 [100->-abc123]'), replica('r1', 'm1')])
        )

        const wrapper = await mountTopology()

        const card = wrapper.findAll('.topology-node').find((c) => c.text().includes('m1'))
        expect(card.text()).toContain('迁移中')
        expect(card.text()).toContain('0-2729')
        expect(card.text()).not.toContain('abc123')

        wrapper.unmount()
    })

    it('在线但无槽位的主节点在提示条中单独计数', async () => {
        api.Topology.mockResolvedValue(topology([master('m1', '0-5460'), master('m2', '')]))

        const wrapper = await mountTopology()

        const alert = wrapper.find('.topology-alert')
        expect(alert.exists()).toBe(true)
        expect(alert.text()).toContain('1 个主节点未分配槽位')
        // 在线的主节点不能被算成离线
        expect(alert.text()).not.toContain('离线节点')

        wrapper.unmount()
    })

    it('非集群模式不显示槽位行', async () => {
        api.Topology.mockResolvedValue({
            success: true,
            data: {
                mode: 'sentinel',
                self: 's:26390',
                version: '7.2.14',
                nodes: [
                    { addr: '10.0.0.1:6379', role: 'master', state: 'online', self: true },
                    { addr: '10.0.0.2:6379', role: 'replica', state: 'online', master: '10.0.0.1:6379' },
                    { addr: '10.0.0.3:26390', role: 'sentinel', state: 'online' },
                ],
            },
        })

        const wrapper = await mountTopology()

        // 卡片里不应出现槽位行（提示条文案中的"槽位"不算）
        const cards = wrapper.findAll('.topology-node')
        expect(cards.length).toBeGreaterThan(0)
        for (const card of cards) {
            expect(card.text()).not.toContain('槽位')
        }

        wrapper.unmount()
    })

    it('分片过多未合并时给出迁移状态不完整的提示', async () => {
        api.Topology.mockResolvedValue({
            success: true,
            data: { mode: 'cluster', self: 'm1', version: '7.2.14', merged: false, nodes: [master('m1', '0-5460')] },
        })

        const wrapper = await mountTopology()
        expect(wrapper.find('.topology-partial').exists()).toBe(true)
        wrapper.unmount()
    })

    it('合并完整时不显示迁移状态提示', async () => {
        api.Topology.mockResolvedValue({
            success: true,
            data: { mode: 'cluster', self: 'm1', version: '7.2.14', merged: true, nodes: [master('m1', '0-5460')] },
        })

        const wrapper = await mountTopology()
        expect(wrapper.find('.topology-partial').exists()).toBe(false)
        wrapper.unmount()
    })

    it('主节点行不换行，由容器横向滚动', async () => {
        api.Topology.mockResolvedValue(
            topology([master('m1', '0-5460'), master('m2', '5461-10922'), master('m3', '10923-16383')])
        )

        const wrapper = await mountTopology()

        expect(wrapper.findAll('.topology-group')).toHaveLength(3)
        expect(wrapper.find('.topology-canvas').exists()).toBe(true)

        wrapper.unmount()
    })

    it('接口失败时提示错误而不是渲染空拓扑', async () => {
        api.Topology.mockResolvedValue({ success: false, msg: '连接失败' })

        const wrapper = await mountTopology()

        expect(window.$message.warning).toHaveBeenCalledWith('连接失败')
        expect(wrapper.find('.topology-groups').exists()).toBe(false)

        wrapper.unmount()
    })
})
