import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import zhCn from '@/langs/zh-cn.json'

const api = vi.hoisted(() => ({
    GetSlotDistribution: vi.fn(),
    ScanSlotDistribution: vi.fn(),
}))
vi.mock('wailsjs/go/services/browserService.js', () => api)

import SlotDistribution from '@/components/content_value/SlotDistribution.vue'

const distributionData = () => {
    const slots = new Array(16384).fill(0)
    slots[0] = 10
    slots[5461] = 5
    return {
        success: true,
        data: {
            slots,
            nodes: [
                { id: 'a', addr: '10.0.0.1:6379', slotRanges: [{ start: 0, end: 5460 }], slotCount: 5461, keys: 10 },
                { id: 'b', addr: '10.0.0.2:6379', slotRanges: [{ start: 5461, end: 10922 }], slotCount: 5462, keys: 5 },
            ],
            scannedAt: Date.now(),
            scanning: false,
            unavailable: [],
        },
    }
}

const emptyData = () => ({
    success: true,
    data: { slots: new Array(16384).fill(0), nodes: [], scannedAt: 0, scanning: false },
})

const uniformData = (value) => ({
    success: true,
    data: {
        slots: new Array(16384).fill(value),
        nodes: [
            { id: 'a', addr: '10.0.0.1:6379', slotRanges: [{ start: 0, end: 5460 }], slotCount: 5461, keys: value * 5461 },
            { id: 'b', addr: '10.0.0.2:6379', slotRanges: [{ start: 5461, end: 10922 }], slotCount: 5462, keys: value * 5462 },
            { id: 'c', addr: '10.0.0.3:6379', slotRanges: [{ start: 10923, end: 16383 }], slotCount: 5461, keys: value * 5461 },
        ],
        scannedAt: Date.now(),
        scanning: false,
        unavailable: [],
    },
})

const mountDistribution = async () => {
    const i18n = createI18n({ legacy: false, locale: 'zh-cn', messages: { 'zh-cn': zhCn } })
    const wrapper = mount(SlotDistribution, {
        props: { server: 'test-server', pause: false },
        global: { plugins: [i18n] },
    })
    await flushPromises()
    return wrapper
}

describe('SlotDistribution', () => {
    beforeEach(() => {
        vi.clearAllMocks()
        vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue(null)
        window.$message = { warning: vi.fn(), error: vi.fn(), success: vi.fn(), info: vi.fn() }
    })

    it('进入页面直接展示后端缓存的上次扫描结果', async () => {
        api.GetSlotDistribution.mockResolvedValue(distributionData())
        const wrapper = await mountDistribution()

        expect(api.GetSlotDistribution).toHaveBeenCalledWith('test-server')
        expect(wrapper.text()).toContain('上次扫描')
        // 5 和 10 的中位数是 7.5，界面四舍五入显示为 8
        expect(wrapper.text()).toContain('热点基准：中位数 8 键 · 1× 最浅，2× 最深')
        expect(wrapper.text()).not.toContain('对数')
        expect(wrapper.text()).not.toContain('线性')
        expect(wrapper.find('.slot-dist-canvas').exists()).toBe(true)

        wrapper.unmount()
    })

    it('均匀分布时以每槽键数作为热点基准', async () => {
        api.GetSlotDistribution.mockResolvedValue(uniformData(16))
        const wrapper = await mountDistribution()

        expect(wrapper.text()).toContain('热点基准：中位数 16 键')
        expect(wrapper.find('.slot-dist-canvas').exists()).toBe(true)

        wrapper.unmount()
    })

    it('全部为 0 键时不显示热点基准', async () => {
        api.GetSlotDistribution.mockResolvedValue(uniformData(0))
        const wrapper = await mountDistribution()

        expect(wrapper.find('.slot-dist-canvas').exists()).toBe(true)
        expect(wrapper.text()).not.toContain('热点基准')

        wrapper.unmount()
    })

    it('没有缓存时提示扫描，扫描后展示分布图', async () => {
        api.GetSlotDistribution.mockResolvedValue(emptyData())
        api.ScanSlotDistribution.mockResolvedValue(distributionData())
        const wrapper = await mountDistribution()

        expect(wrapper.text()).toContain('还没有扫描结果')

        const scanButton = wrapper.findAll('button').find((button) => button.text().includes('扫描'))
        await scanButton.trigger('click')
        await flushPromises()

        expect(api.ScanSlotDistribution).toHaveBeenCalledWith('test-server')
        expect(wrapper.find('.slot-dist-placeholder').exists()).toBe(false)
        expect(wrapper.find('.slot-dist-canvas').exists()).toBe(true)
        expect(wrapper.text()).toContain('上次扫描')
        expect(wrapper.text()).toContain('热点基准：中位数 8 键')

        wrapper.unmount()
    })

    it('重新扫描后更新热点基准', async () => {
        api.GetSlotDistribution.mockResolvedValue(emptyData())
        api.ScanSlotDistribution.mockResolvedValue(uniformData(20))
        const wrapper = await mountDistribution()

        const scanButton = wrapper.findAll('button').find((button) => button.text().includes('扫描'))
        await scanButton.trigger('click')
        await flushPromises()

        expect(wrapper.text()).toContain('热点基准：中位数 20 键')

        wrapper.unmount()
    })

})
