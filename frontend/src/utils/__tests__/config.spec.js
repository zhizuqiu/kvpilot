import { describe, expect, it } from 'vitest'
import { filterConfigParams } from '@/utils/config.js'

const params = [
    { key: 'maxmemory', value: '0', category: 'memory', consistent: true, sensitive: false, immutable: false },
    { key: 'maxmemory-policy', value: 'noeviction', category: 'memory', consistent: true, sensitive: false, immutable: false },
    { key: 'timeout', value: '0', category: 'network', consistent: false, sensitive: false, immutable: false },
    { key: 'requirepass', value: '***', category: 'security', consistent: true, sensitive: true, immutable: false },
    { key: 'databases', value: '16', category: 'other', consistent: true, sensitive: false, immutable: true },
]

describe('filterConfigParams', () => {
    it('默认返回全部参数', () => {
        expect(filterConfigParams(params)).toHaveLength(5)
    })

    it('按一致性筛选：一致 / 不一致', () => {
        expect(filterConfigParams(params, { consistency: 'consistent' }).map((p) => p.key)).toEqual([
            'maxmemory',
            'maxmemory-policy',
            'requirepass',
            'databases',
        ])
        expect(filterConfigParams(params, { consistency: 'inconsistent' }).map((p) => p.key)).toEqual(['timeout'])
    })

    it('按敏感与不可变筛选', () => {
        expect(filterConfigParams(params, { consistency: 'sensitive' }).map((p) => p.key)).toEqual(['requirepass'])
        expect(filterConfigParams(params, { consistency: 'immutable' }).map((p) => p.key)).toEqual(['databases'])
    })

    it('分类与一致性条件同时生效', () => {
        // memory 分类下只有一致参数，叠加"不一致"后应为空
        expect(filterConfigParams(params, { category: 'memory', consistency: 'inconsistent' })).toHaveLength(0)
        expect(filterConfigParams(params, { category: 'memory', consistency: 'consistent' })).toHaveLength(2)
    })

    it('关键字匹配参数名或取值，忽略大小写', () => {
        expect(filterConfigParams(params, { keyword: 'MAXMEMORY' }).map((p) => p.key)).toEqual([
            'maxmemory',
            'maxmemory-policy',
        ])
        expect(filterConfigParams(params, { keyword: 'noeviction' }).map((p) => p.key)).toEqual(['maxmemory-policy'])
    })

    it('空输入安全', () => {
        expect(filterConfigParams(null)).toEqual([])
        expect(filterConfigParams([], { keyword: 'x' })).toEqual([])
    })
})
