import { describe, expect, it } from 'vitest'
import { buildHotspotScale, hotspotAlpha, hotspotRatio } from '@/utils/slot_color.js'

// 验证热点基准取非零槽位的中位数：既要忽略空槽，也要不被极端热点带偏。
describe('buildHotspotScale', () => {
    it('忽略 0 键槽位，奇数个样本直接取中位数', () => {
        expect(buildHotspotScale([0, 1, 16, 25, 0])).toEqual({ median: 16 })
    })

    it('偶数个样本取中间两个值的平均', () => {
        expect(buildHotspotScale([5, 10])).toEqual({ median: 7.5 })
    })

    it('全部为 0 时基准为 0', () => {
        expect(buildHotspotScale([0, 0, 0])).toEqual({ median: 0 })
    })

    it('只有一个非零槽位时基准就是该槽的键数', () => {
        expect(buildHotspotScale([0, 0, 42, 0])).toEqual({ median: 42 })
    })

    it('大量 0 键槽位不会把基准拉到 0', () => {
        const counts = new Array(1000).fill(0)
        counts[10] = 1
        counts[20] = 1
        counts[30] = 2
        counts[40] = 3
        expect(buildHotspotScale(counts)).toEqual({ median: 1.5 })
    })

    it('单个超大热点不会改变中位数', () => {
        const counts = new Array(16384).fill(11)
        counts[10292] = 2509
        expect(buildHotspotScale(counts)).toEqual({ median: 11 })
    })

    it('完全均匀的集群基准就是每槽键数', () => {
        expect(buildHotspotScale(new Array(16384).fill(16))).toEqual({ median: 16 })
    })
})

// 验证热点比例：中位数及以下为 0，2 倍中位数达到 1，超过后饱和。
describe('hotspotRatio', () => {
    const scale = { median: 16 }

    it('0 键和低于中位数都返回 0', () => {
        expect(hotspotRatio(0, scale)).toBe(0)
        expect(hotspotRatio(10, scale)).toBe(0)
        expect(hotspotRatio(16, scale)).toBe(0)
    })

    it('基准为 0 时任何值都返回 0，避免除零', () => {
        expect(hotspotRatio(100, { median: 0 })).toBe(0)
        expect(hotspotRatio(100, null)).toBe(0)
    })

    it('2 倍中位数达到最深，超过后饱和', () => {
        expect(hotspotRatio(32, scale)).toBe(1)
        expect(hotspotRatio(3200, scale)).toBe(1)
    })

    it('1.25x / 1.5x 按对数比例递增', () => {
        expect(hotspotRatio(20, scale)).toBeCloseTo(0.322, 3)
        expect(hotspotRatio(24, scale)).toBeCloseTo(0.585, 3)
        expect(hotspotRatio(24, scale)).toBeGreaterThan(hotspotRatio(20, scale))
    })

    it('全部槽位键数相同时所有热点比例都是 0', () => {
        const uniform = { median: 16 }
        expect([16, 16, 16].map((value) => hotspotRatio(value, uniform))).toEqual([0, 0, 0])
    })

    it('热点相对中位数的倍数决定强度', () => {
        const real = { median: 11 }
        expect(hotspotRatio(11, real)).toBe(0)
        expect(hotspotRatio(17, real)).toBeCloseTo(0.628, 3)
        expect(hotspotRatio(1211, real)).toBe(1)
        expect(hotspotRatio(2509, real)).toBe(1)
    })
})

// 验证不透明度映射：均匀时为 0.05，2 倍中位数为 1。
describe('hotspotAlpha', () => {
    it('最低和最高不透明度符合约定', () => {
        expect(hotspotAlpha(0)).toBeCloseTo(0.05)
        expect(hotspotAlpha(1)).toBeCloseTo(1)
        expect(hotspotAlpha(1.5)).toBeCloseTo(1)
        expect(hotspotAlpha(-1)).toBeCloseTo(0.05)
    })

    it('中间值按比例落在 0.05~1 之间', () => {
        expect(hotspotAlpha(0.5)).toBeCloseTo(0.525)
        expect(hotspotAlpha(0.585)).toBeCloseTo(0.60575)
    })
})
