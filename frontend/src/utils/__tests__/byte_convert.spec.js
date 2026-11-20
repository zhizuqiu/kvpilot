import { describe, expect, it } from 'vitest'
import { convertBytes, formatBytes } from '@/utils/byte_convert.js'

describe('convertBytes', () => {
    it('0 与负数统一返回 0 B，避免出现 NaN 单位', () => {
        expect(convertBytes(0)).toEqual({ value: 0, unit: 'B' })
        expect(convertBytes(-5)).toEqual({ value: 0, unit: 'B' })
    })

    it('按 1024 进位换算到合适单位', () => {
        expect(convertBytes(1536)).toEqual({ value: 1.5, unit: 'KB' })
    })

    it('按指定小数位四舍五入', () => {
        expect(convertBytes(1234, 0)).toEqual({ value: 1, unit: 'KB' })
    })

    it('超出 TB 的数值仍停在 TB，不产生未定义单位', () => {
        // 单位表只到 TB，超大值必须被钳制，否则会取到 undefined
        expect(convertBytes(1024 ** 5).unit).toBe('TB')
    })
})

describe('formatBytes', () => {
    it('拼接数值与单位供界面直接展示', () => {
        expect(formatBytes(1024 * 1024)).toBe('1MB')
    })
})
