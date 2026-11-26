import { describe, expect, it } from 'vitest'
import { formatSlotRanges, parseSlotList } from '@/utils/slot_text.js'

// 验证从槽位范围复制出来的文本可以直接粘贴：逗号分隔、区间、空格和去重。
describe('parseSlotList', () => {
    it('解析逗号分隔的单个槽位与区间', () => {
        expect(parseSlotList('0-6, 8, 10-12').slots).toEqual([0, 1, 2, 3, 4, 5, 6, 8, 10, 11, 12])
    })

    it('兼容中文逗号和区间两侧空格', () => {
        expect(parseSlotList('0，2, 10 - 12').slots).toEqual([0, 2, 10, 11, 12])
    })

    it('重复槽位只保留一次', () => {
        expect(parseSlotList('1, 1, 0-2').slots).toEqual([1, 0, 2])
    })

    it('非法内容返回错误而不是部分解析', () => {
        expect(parseSlotList('abc').error).toBe('abc')
        expect(parseSlotList('5-3').error).toBe('5-3')
        expect(parseSlotList('16384').error).toBe('16384')
        expect(parseSlotList('0-6, x').slots).toEqual([])
    })

    it('空文本返回空列表', () => {
        expect(parseSlotList('  ').slots).toEqual([])
        expect(parseSlotList(null).slots).toEqual([])
    })
})

// 验证槽号列表压缩：连续槽合并为区间，乱序和重复不影响结果。
describe('formatSlotRanges', () => {
    it('连续槽位合并成区间', () => {
        expect(formatSlotRanges([0, 1, 2, 4, 5, 7])).toEqual(['0-2', '4-5', '7'])
    })

    it('乱序和重复输入先去重排序', () => {
        expect(formatSlotRanges([5, 1, 2, 2, 1])).toEqual(['1-2', '5'])
    })

    it('空列表返回空数组', () => {
        expect(formatSlotRanges([])).toEqual([])
    })
})
