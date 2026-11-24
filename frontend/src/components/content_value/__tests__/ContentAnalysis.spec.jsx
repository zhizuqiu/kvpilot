import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { h } from 'vue'
import { NConfigProvider, NMessageProvider, NSelect } from 'naive-ui'
import { createI18n } from 'vue-i18n'
import zhCn from '@/langs/zh-cn.json'

const api = vi.hoisted(() => ({
    StartBigKeyScan: vi.fn(),
    GetBigKeyScan: vi.fn(),
    CancelBigKeyScan: vi.fn(),
    ExportBigKeys: vi.fn(),
    SaveFile: vi.fn(),
}))
vi.mock('wailsjs/go/services/browserService.js', () => api)
vi.mock('wailsjs/go/services/systemService.js', () => ({ SaveFile: api.SaveFile }))

import ContentAnalysis from '@/components/content_value/ContentAnalysis.vue'

const progress = (overrides = {}) => ({
    state: 'idle',
    scanned: 0,
    total: 0,
    percent: 0,
    found: 0,
    results: [],
    unavailable: [],
    elapsedMs: 0,
    error: '',
    ...overrides,
})

const bigKey = (overrides = {}) => ({
    key: 'cache:big',
    type: 'string',
    size: 20 * 1024 * 1024,
    node: '10.0.0.1:6379',
    db: 0,
    ...overrides,
})

const mountAnalysis = async () => {
    window.$message = { warning: vi.fn(), error: vi.fn(), success: vi.fn(), info: vi.fn() }
    const i18n = createI18n({ legacy: false, locale: 'zh-cn', messages: { 'zh-cn': zhCn } })
    const wrapper = mount(
        {
            render: () =>
                h(NConfigProvider, null, () =>
                    h(NMessageProvider, null, () => h(ContentAnalysis, { server: 'test-server' }))
                ),
        },
        { global: { plugins: [i18n] } }
    )
    await flushPromises()
    return wrapper
}

const bodyRows = (wrapper) => wrapper.findAll('.n-data-table-tbody .n-data-table-tr')

// 页面上还有数字输入框的加减按钮，按文案定位真正的操作按钮
const findButton = (wrapper, text) => wrapper.findAll('button').find((button) => button.text().includes(text))

describe('ContentAnalysis', () => {
    beforeEach(() => {
        vi.clearAllMocks()
        api.GetBigKeyScan.mockResolvedValue({ success: true, data: progress() })
    })

    it('未开始时展示引导文案', async () => {
        const wrapper = await mountAnalysis()

        expect(wrapper.text()).toContain('大 key 分析')
        expect(wrapper.text()).toContain('设置采样数量与阈值后开始分析')
        expect(bodyRows(wrapper)).toHaveLength(0)

        wrapper.unmount()
    })

    it('开始分析使用当前采样参数并进入扫描状态', async () => {
        api.StartBigKeyScan.mockResolvedValue({ success: true, data: progress({ state: 'running', total: 1000 }) })
        const wrapper = await mountAnalysis()

        await findButton(wrapper, '开始分析').trigger('click')
        await flushPromises()

        expect(api.StartBigKeyScan).toHaveBeenCalledWith('test-server', 1000, 10, false)
        expect(wrapper.text()).toContain('扫描中')
        expect(wrapper.findAll('.analysis-options button').map((b) => b.text())).toContain('取消')

        wrapper.unmount()
    })

    it('轮询后在任务完成时展示结果', async () => {
        vi.useFakeTimers()
        try {
            api.StartBigKeyScan.mockResolvedValue({ success: true, data: progress({ state: 'running' }) })
            api.GetBigKeyScan
                .mockResolvedValueOnce({ success: true, data: progress() })
                .mockResolvedValue({
                    success: true,
                    data: progress({
                        state: 'done',
                        scanned: 1000,
                        total: 1000,
                        percent: 100,
                        found: 2,
                        results: [bigKey(), bigKey({ key: 'hash:big', type: 'hash', size: 30 * 1024 * 1024 })],
                    }),
                })

            const wrapper = await mountAnalysis()
            await findButton(wrapper, '开始分析').trigger('click')
            await flushPromises()

            await vi.advanceTimersByTimeAsync(800)
            await flushPromises()

            expect(api.GetBigKeyScan).toHaveBeenCalled()
            expect(wrapper.text()).toContain('已完成')
            expect(wrapper.text()).toContain('命中 2 个')
            expect(bodyRows(wrapper)).toHaveLength(2)
            expect(wrapper.text()).toContain('30MB')

            wrapper.unmount()
        } finally {
            vi.useRealTimers()
        }
    })

    it('按类型与关键字筛选结果', async () => {
        api.GetBigKeyScan.mockResolvedValue({
            success: true,
            data: progress({
                state: 'done',
                found: 2,
                results: [bigKey(), bigKey({ key: 'hash:big', type: 'hash' })],
            }),
        })
        const wrapper = await mountAnalysis()

        wrapper.findAllComponents(NSelect)[0].vm.$emit('update:value', 'hash')
        await flushPromises()
        expect(bodyRows(wrapper)).toHaveLength(1)
        expect(wrapper.text()).toContain('hash:big')

        await wrapper.find('.analysis-search input').setValue('cache')
        await flushPromises()
        expect(bodyRows(wrapper)).toHaveLength(0)

        wrapper.unmount()
    })

    it('运行中可以取消任务', async () => {
        api.GetBigKeyScan.mockResolvedValue({ success: true, data: progress({ state: 'running', total: 1000 }) })
        api.CancelBigKeyScan.mockResolvedValue({ success: true, data: progress({ state: 'canceled', scanned: 100 }) })
        const wrapper = await mountAnalysis()

        expect(wrapper.findAll('button').map((b) => b.text())).toContain('取消')
        await findButton(wrapper, '取消').trigger('click')
        await flushPromises()

        expect(api.CancelBigKeyScan).toHaveBeenCalledWith('test-server')
        expect(wrapper.text()).toContain('已取消')

        wrapper.unmount()
    })

    it('启动失败时提示原因', async () => {
        api.StartBigKeyScan.mockResolvedValue({ success: false, msg: 'scan already running' })
        const wrapper = await mountAnalysis()

        await findButton(wrapper, '开始分析').trigger('click')
        await flushPromises()

        expect(window.$message.error).toHaveBeenCalledWith('scan already running')

        wrapper.unmount()
    })

    it('节点不可达时展示未响应数量', async () => {
        api.GetBigKeyScan.mockResolvedValue({
            success: true,
            data: progress({ state: 'done', unavailable: ['10.0.0.9:6379'] }),
        })
        const wrapper = await mountAnalysis()

        expect(wrapper.text()).toContain('1 个节点未响应')

        wrapper.unmount()
    })

    it('桌面端导出把结果写入选择好的路径', async () => {
        api.GetBigKeyScan.mockResolvedValue({
            success: true,
            data: progress({ state: 'done', found: 1, results: [bigKey()] }),
        })
        api.SaveFile.mockResolvedValue({ success: true, data: { path: '/tmp/bigkeys.csv' } })
        api.ExportBigKeys.mockResolvedValue({ success: true, data: { count: 1 } })
        const wrapper = await mountAnalysis()

        const exportButton = findButton(wrapper, '导出')
        await exportButton.trigger('click')
        await flushPromises()

        expect(api.SaveFile).toHaveBeenCalled()
        expect(api.ExportBigKeys).toHaveBeenCalledWith('/tmp/bigkeys.csv', expect.arrayContaining([expect.objectContaining({ key: 'cache:big' })]))
        expect(window.$message.success).toHaveBeenCalled()

        wrapper.unmount()
    })
})
