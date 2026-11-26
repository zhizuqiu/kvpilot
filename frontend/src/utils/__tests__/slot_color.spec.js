import { describe, expect, it } from 'vitest'
import { buildSlotScale, scaleRatio } from '@/utils/slot_color.js'

// 验证色阶范围使用非零槽位的分位数，避免均匀集群全部挤在色阶顶端。
describe('buildSlotScale', () => {
    it('忽略 0 键槽位，并按 P2~P98 取范围', () => {
        const counts = new Array(100).fill(0)
        for (let i = 0; i < 50; i++) {
            counts[i] = 10 + (i % 16)
        }
        const scale = buildSlotScale(counts)
        expect(scale.lo).toBeGreaterThanOrEqual(10)
        expect(scale.hi).toBeLessThanOrEqual(25)
        expect(scale.hi).toBeGreaterThan(scale.lo)
    })

    it('所有槽位键数相同时标记为 uniform', () => {
        expect(buildSlotScale([16, 16, 16])).toEqual({ lo: 16, hi: 16, uniform: true })
    })

    it('没有非零槽位时返回空范围', () => {
        expect(buildSlotScale([0, 0, 0])).toEqual({ lo: 0, hi: 0, uniform: true })
    })
})

describe('scaleRatio', () => {
    const scale = { lo: 10, hi: 20, uniform: false }

    it('范围外饱和，范围内按比例映射', () => {
        expect(scaleRatio(0, scale, 'linear')).toBe(0)
        expect(scaleRatio(5, scale, 'linear')).toBe(0)
        expect(scaleRatio(15, scale, 'linear')).toBeCloseTo(0.5)
        expect(scaleRatio(25, scale, 'linear')).toBe(1)
    })

    it('对数模式下低值区间被拉开且保持单调', () => {
        const low = scaleRatio(11, scale, 'log')
        const mid = scaleRatio(15, scale, 'log')
        const high = scaleRatio(19, scale, 'log')
        expect(low).toBeGreaterThan(0)
        expect(mid).toBeGreaterThan(low)
        expect(high).toBeGreaterThan(mid)
        expect(high).toBeLessThan(1)
    })

    it('uniform 时返回中间色', () => {
        expect(scaleRatio(16, { lo: 16, hi: 16, uniform: true }, 'log')).toBe(0.55)
    })
})
