import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { NConfigProvider } from 'naive-ui'
import RedisTypeTag from '@/components/common/RedisTypeTag.vue'

// 组件依赖主题变量，必须包在 n-config-provider 内才能挂载
const mountTag = (props = {}) =>
    mount({
        render: () => (
            <NConfigProvider>
                <RedisTypeTag {...props} />
            </NConfigProvider>
        ),
    })

describe('RedisTypeTag', () => {
    it('默认展示大写类型名', () => {
        expect(mountTag({ type: 'hash' }).text()).toBe('HASH')
    })

    it('short 模式使用单字母缩写', () => {
        // key 列表列宽有限，只有缩写能放得下
        expect(mountTag({ type: 'zset', short: true }).text()).toBe('Z')
    })

    it('short 模式遇到未知类型时回退到 defaultLabel', () => {
        expect(mountTag({ type: 'unknown', short: true, defaultLabel: '?' }).text()).toBe('?')
    })

    it('point 模式渲染圆点，并把尺寸下限钳制为 5px', () => {
        // 迷你拓扑图里圆点过小会点不到，这里验证下限生效
        const wrapper = mountTag({ type: 'string', point: true, pointSize: 2 })
        const dot = wrapper.find('.redis-type-tag-point')
        expect(dot.exists()).toBe(true)
        expect(dot.attributes('style')).toContain('width: 5px')
        expect(dot.attributes('style')).toContain('height: 5px')
    })
})
