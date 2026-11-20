import { describe, expect, it } from 'vitest'
import { compareVersion, toVersionArray } from '@/utils/version.js'

describe('toVersionArray', () => {
    it('去掉 v 前缀并按点切分为数字数组', () => {
        expect(toVersionArray('v1.2.8')).toEqual([1, 2, 8])
    })

    it('非数字段退化为 0，避免脏版本号导致 NaN 参与比较', () => {
        // 服务端可能返回 7.2.14-rc1 之类的版本串，取不到数字时不能产生 NaN
        expect(toVersionArray('1.2.beta')).toEqual([1, 2, 0])
    })
})

describe('compareVersion', () => {
    it('按数值比较而不是按字符串字典序比较', () => {
        // 字典序会把 '10' 排在 '2' 前面，这里必须保证 1.10.0 大于 1.2.8
        expect(compareVersion('1.2.8', '1.10.0')).toBe(-1)
        expect(compareVersion('1.10.0', '1.2.8')).toBe(1)
    })

    it('忽略 v 前缀，相同版本返回 0', () => {
        expect(compareVersion('v1.2.8', '1.2.8')).toBe(0)
    })

    it('位数不同时缺失的段按 0 补齐', () => {
        // 用于判断服务端版本是否满足最低要求，1.2 与 1.2.0 必须等价
        expect(compareVersion('1.2', '1.2.0')).toBe(0)
        expect(compareVersion('1.2', '1.2.1')).toBe(-1)
    })
})
