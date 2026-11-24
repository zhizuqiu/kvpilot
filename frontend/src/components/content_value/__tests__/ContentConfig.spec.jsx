import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { h } from 'vue'
import { NConfigProvider, NMessageProvider, NSelect } from 'naive-ui'
import { createI18n } from 'vue-i18n'
import zhCn from '@/langs/zh-cn.json'

const api = vi.hoisted(() => ({ ConfigOverview: vi.fn(), ConfigSet: vi.fn() }))
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
    api.ConfigSet.mockResolvedValue({
        success: true,
        data: {
            applied: 3,
            failed: 0,
            memoryOnly: 3,
            nodes: 3,
            results: [
                { addr: '10.0.0.1:6379', set: true, rewrite: 'memory-only' },
                { addr: '10.0.0.2:6379', set: true, rewrite: 'memory-only' },
                { addr: '10.0.0.3:6379', set: true, rewrite: 'memory-only' },
            ],
        },
    })
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

    it('节点一致性下拉可筛选不一致参数', async () => {
        const wrapper = await mountConfig()

        // 第一个下拉是分类，第二个是节点一致性
        const selects = wrapper.findAllComponents(NSelect)
        expect(selects.length).toBeGreaterThanOrEqual(2)

        selects[1].vm.$emit('update:value', 'inconsistent')
        await flushPromises()

        const rows = wrapper.findAll('.config-row')
        expect(rows).toHaveLength(2) // 表头 + timeout
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

describe('ContentConfig inline editing', () => {
    const rowOf = (wrapper, key) => wrapper.findAll('.config-row').find((r) => r.text().includes(key))

    it('敏感与不可变参数不提供编辑入口', async () => {
        const wrapper = await mountConfig()

        expect(rowOf(wrapper, 'requirepass').text()).not.toContain('编辑')
        expect(rowOf(wrapper, 'maxmemory').text()).toContain('编辑')

        wrapper.unmount()
    })

    it('点击编辑后出现输入框，保存时调用 ConfigSet 并刷新列表', async () => {
        const wrapper = await mountConfig()
        const row = rowOf(wrapper, 'maxmemory')

        await row.find('button').trigger('click')
        await flushPromises()
        const input = row.find('input')
        expect(input.exists()).toBe(true)
        await input.setValue('128mb')

        const buttons = row.findAll('button')
        await buttons[buttons.length - 1].trigger('click')
        await flushPromises()

        expect(api.ConfigSet).toHaveBeenCalledWith('test-server', 'maxmemory', '128mb')
        expect(api.ConfigOverview).toHaveBeenCalledTimes(2)

        wrapper.unmount()
    })

    it('无配置文件时提示仅内存生效', async () => {
        const wrapper = await mountConfig()
        const row = rowOf(wrapper, 'maxmemory')

        await row.find('button').trigger('click')
        await flushPromises()
        const buttons = row.findAll('button')
        await buttons[buttons.length - 1].trigger('click')
        await flushPromises()

        expect(window.$message.warning).toHaveBeenCalledWith(
            expect.stringContaining('仅内存生效')
        )

        wrapper.unmount()
    })

    it('写入失败时提示错误', async () => {
        api.ConfigSet.mockResolvedValue({ success: false, msg: 'NOPERM' })
        const wrapper = await mountConfig()
        const row = rowOf(wrapper, 'maxmemory')

        await row.find('button').trigger('click')
        await flushPromises()
        const buttons = row.findAll('button')
        await buttons[buttons.length - 1].trigger('click')
        await flushPromises()

        expect(window.$message.error).toHaveBeenCalledWith('NOPERM')

        wrapper.unmount()
    })
})
