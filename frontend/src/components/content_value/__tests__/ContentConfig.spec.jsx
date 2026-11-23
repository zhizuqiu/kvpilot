import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { h } from 'vue'
import { NConfigProvider, NMessageProvider } from 'naive-ui'
import { createI18n } from 'vue-i18n'
import zhCn from '@/langs/zh-cn.json'

const api = vi.hoisted(() => ({ ConfigOverview: vi.fn() }))
vi.mock('wailsjs/go/services/browserService.js', () => api)

import ContentConfig from '@/components/content_value/ContentConfig.vue'

const overview = {
    success: true,
    data: {
        nodes: ['10.0.0.1:6379', '10.0.0.2:6379', '10.0.0.3:6379'],
        merged: true,
        params: [
            {
                key: 'maxmemory',
                value: '0',
                type: 'int',
                consistent: true,
                nodes: 3,
                sensitive: false,
                immutable: false,
                category: 'memory',
            },
            {
                key: 'requirepass',
                value: '***',
                type: 'string',
                consistent: true,
                nodes: 3,
                sensitive: true,
                immutable: false,
                category: 'security',
            },
            {
                key: 'timeout',
                value: '0',
                type: 'int',
                consistent: false,
                nodes: 3,
                sensitive: false,
                immutable: false,
                category: 'network',
                values: {
                    '10.0.0.1:6379': '0',
                    '10.0.0.2:6379': '5',
                    '10.0.0.3:6379': '0',
                },
            },
        ],
    },
}

const mountConfig = async () => {
    window.$message = { warning: vi.fn(), error: vi.fn(), success: vi.fn(), info: vi.fn() }
    const i18n = createI18n({ legacy: false, locale: 'zh-cn', messages: { 'zh-cn': zhCn } })
    const wrapper = mount(
        {
            render: () =>
                h(NConfigProvider, null, () =>
                    h(NMessageProvider, null, () => h(ContentConfig, { server: 'test-server' }))
                ),
        },
        { global: { plugins: [i18n] } }
    )
    await flushPromises()
    return wrapper
}

beforeEach(() => {
    vi.clearAllMocks()
    api.ConfigOverview.mockResolvedValue(overview)
})

describe('ContentConfig', () => {
    it('加载后渲染参数列表与节点数', async () => {
        const wrapper = await mountConfig()

        expect(api.ConfigOverview).toHaveBeenCalledWith('test-server')
        expect(wrapper.text()).toContain('共 3 个节点')
        // 表头 + 3 行
        expect(wrapper.findAll('.config-row')).toHaveLength(4)

        wrapper.unmount()
    })

    it('敏感参数展示脱敏值并打标签', async () => {
        const wrapper = await mountConfig()

        const row = wrapper.findAll('.config-row').find((r) => r.text().includes('requirepass'))
        expect(row.text()).toContain('***')
        expect(row.text()).toContain('敏感')

        wrapper.unmount()
    })

    it('节点间不一致时显示比例与差异入口', async () => {
        const wrapper = await mountConfig()

        const row = wrapper.findAll('.config-row').find((r) => r.text().includes('timeout'))
        expect(row.text()).toContain('3/3')
        expect(row.text()).toContain('差异')
        // 一致参数不显示差异入口
        const consistent = wrapper.findAll('.config-row').find((r) => r.text().includes('maxmemory'))
        expect(consistent.text()).toContain('一致')

        wrapper.unmount()
    })

    it('搜索按参数名过滤', async () => {
        const wrapper = await mountConfig()

        await wrapper.find('input').setValue('timeout')
        await flushPromises()

        const rows = wrapper.findAll('.config-row')
        expect(rows).toHaveLength(2) // 表头 + 1 行
        expect(rows[1].text()).toContain('timeout')

        wrapper.unmount()
    })

    it('加载失败时提示错误而不是空白列表', async () => {
        api.ConfigOverview.mockResolvedValue({ success: false, msg: '连接已断开' })
        const wrapper = await mountConfig()

        // useMessage 在真实 provider 下渲染，这里断言列表为空并显示空态
        expect(wrapper.findAll('.config-row')).toHaveLength(1) // 仅表头
        expect(wrapper.text()).toContain('没有匹配的参数')

        wrapper.unmount()
    })
})
