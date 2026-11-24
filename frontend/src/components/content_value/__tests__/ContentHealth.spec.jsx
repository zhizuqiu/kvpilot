import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { NConfigProvider, NMessageProvider, NSelect } from 'naive-ui'
import { createI18n } from 'vue-i18n'
import zhCn from '@/langs/zh-cn.json'
import { BrowserTabType } from '@/consts/browser_tab_type.js'

const switchTab = vi.hoisted(() => vi.fn())
const api = vi.hoisted(() => ({ CheckHealth: vi.fn(), ExportCSV: vi.fn(), SaveFile: vi.fn() }))

vi.mock('wailsjs/go/services/browserService.js', () => api)
vi.mock('wailsjs/go/services/systemService.js', () => ({ SaveFile: api.SaveFile }))
vi.mock('stores/tab.js', () => ({ default: () => ({ switchSubTab: switchTab }) }))

import ContentHealth from '@/components/content_value/ContentHealth.vue'
import { h } from 'vue'

const healthData = (overrides = {}) => ({
    success: true,
    data: {
        nodes: ['10.0.0.1:6379'],
        unavailable: [],
        mode: 'cluster',
        elapsedMs: 120,
        counts: { error: 1, warn: 1, ok: 1 },
        items: [
            {
                node: '10.0.0.1:6379',
                rule: 'auth',
                severity: 'error',
                values: { protected: 'no', password: 'no' },
            },
            {
                node: '10.0.0.1:6379',
                rule: 'maxmemory',
                severity: 'warn',
                values: { used: '900MB', max: '1.0GB', percent: '87' },
            },
            {
                node: '10.0.0.1:6379',
                rule: 'fragmentation',
                severity: 'ok',
                values: { ratio: '1.10' },
            },
        ],
        ...overrides,
    },
})

const mountHealth = async (props = {}) => {
    window.$message = { warning: vi.fn(), error: vi.fn(), success: vi.fn(), info: vi.fn() }
    const i18n = createI18n({ legacy: false, locale: 'zh-cn', messages: { 'zh-cn': zhCn } })
    const wrapper = mount(
        {
            render: () =>
                h(NConfigProvider, null, () =>
                    h(NMessageProvider, null, () => h(ContentHealth, { server: 'test-server', pause: false, ...props }))
                ),
        },
        { global: { plugins: [i18n] } }
    )
    await flushPromises()
    return wrapper
}

const bodyRows = (wrapper) => wrapper.findAll('.n-data-table-tbody .n-data-table-tr')

describe('ContentHealth', () => {
    beforeEach(() => {
        vi.clearAllMocks()
        api.CheckHealth.mockResolvedValue(healthData())
    })

    it('默认只展示异常项并给出统计', async () => {
        const wrapper = await mountHealth()

        expect(wrapper.text()).toContain('错误 1')
        expect(wrapper.text()).toContain('告警 1')
        expect(wrapper.text()).toContain('通过 1')
        expect(bodyRows(wrapper)).toHaveLength(2)
        expect(wrapper.text()).toContain('认证配置')
        expect(wrapper.text()).toContain('内存上限')

        wrapper.unmount()
    })

    it('切换全部结果后展示正常项与详情', async () => {
        const wrapper = await mountHealth()

        wrapper.findComponent(NSelect).vm.$emit('update:value', 'all')
        await flushPromises()

        expect(bodyRows(wrapper)).toHaveLength(3)
        expect(wrapper.text()).toContain('内存碎片率')
        expect(wrapper.text()).toContain('碎片率 1.10')

        wrapper.unmount()
    })

    it('按规则跳到对应页签', async () => {
        const wrapper = await mountHealth()

        const authRow = bodyRows(wrapper).find((row) => row.text().includes('认证配置'))
        await authRow.find('button').trigger('click')
        expect(switchTab).toHaveBeenCalledWith(BrowserTabType.Config)

        switchTab.mockClear()
        const memoryRow = bodyRows(wrapper).find((row) => row.text().includes('内存上限'))
        await memoryRow.find('button').trigger('click')
        expect(switchTab).toHaveBeenCalledWith(BrowserTabType.Status)

        wrapper.unmount()
    })

    it('导出当前筛选结果', async () => {
        api.SaveFile.mockResolvedValue({ success: true, data: { path: '/tmp/health.csv' } })
        api.ExportCSV.mockResolvedValue({ success: true, data: { count: 2 } })
        const wrapper = await mountHealth()

        const exportButton = wrapper.findAll('button').find((button) => button.text().includes('导出 CSV'))
        await exportButton.trigger('click')
        await flushPromises()

        expect(api.SaveFile).toHaveBeenCalled()
        expect(api.ExportCSV).toHaveBeenCalledWith(
            '/tmp/health.csv',
            ['node', 'rule', 'severity', 'detail'],
            expect.arrayContaining([['10.0.0.1:6379', 'auth', 'error', expect.stringContaining('protected-mode no')]])
        )
        expect(window.$message.success).toHaveBeenCalled()

        wrapper.unmount()
    })

    it('加载失败时提示原因', async () => {
        api.CheckHealth.mockResolvedValue({ success: false, msg: 'no reachable node' })
        const wrapper = await mountHealth()

        expect(window.$message.warning).toHaveBeenCalledWith('no reachable node')

        wrapper.unmount()
    })
})
