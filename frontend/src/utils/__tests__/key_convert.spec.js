import { describe, expect, it } from 'vitest'
import { decodeRedisKey, nativeRedisKey } from '@/utils/key_convert.js'

describe('decodeRedisKey', () => {
    it('可打印字符按原样拼接', () => {
        expect(decodeRedisKey([104, 105])).toBe('hi')
    })

    it('不可打印字节转为大写十六进制转义，保证界面上可见', () => {
        // 二进制 key 直接渲染会变成空白或乱码，必须显式转义并统一大写
        expect(decodeRedisKey([0, 255])).toBe('\\x00\\xFF')
    })

    it('字符串 key 原样返回', () => {
        expect(decodeRedisKey('user:1')).toBe('user:1')
    })
})

describe('nativeRedisKey', () => {
    it('按 char code 还原为原始字符串', () => {
        expect(nativeRedisKey([0x4f60, 0x597d])).toBe('你好')
    })

    it('给定长度时截断，用于列表页只展示前若干字符', () => {
        expect(nativeRedisKey([104, 105, 106], 2)).toBe('hi')
    })

    it('长度为 0 时不截断，避免把 key 整体截空', () => {
        expect(nativeRedisKey([104, 105, 106], 0)).toBe('hij')
    })
})
