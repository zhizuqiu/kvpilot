import { describe, expect, it } from 'vitest'
import { computeSlotLayout, slotAtPoint, slotsInRect } from '@/utils/slot_layout.js'

const node = (id, start, end) => ({
    id,
    addr: `10.0.0.${id}:6379`,
    slotRanges: [{ start, end }],
    slotCount: end - start + 1,
    keys: 0,
})

// 验证节点矩形按槽位数分配宽度：块宽比例应与槽位比例一致，
// 否则跨节点的颜色密度就无法直接比较。
describe('computeSlotLayout', () => {
    it('矩形宽度与槽位数成正比且总宽不超出容器', () => {
        const layout = computeSlotLayout(
            [node(1, 0, 5460), node(2, 5461, 10922), node(3, 10923, 16383)],
            900,
            300
        )

        expect(layout).not.toBeNull()
        expect(layout.blocks).toHaveLength(3)
        const widths = layout.blocks.map((block) => block.width)
        const ratio = widths[2] / widths[0]
        expect(ratio).toBeGreaterThan(0.97)
        expect(ratio).toBeLessThan(1.03)
        expect(layout.blocks[2].x + widths[2]).toBeLessThanOrEqual(900)
        expect(layout.cell).toBeGreaterThan(1)
    })

    it('每个矩形都能容纳自己的全部槽位', () => {
        const layout = computeSlotLayout([node(1, 0, 99), node(2, 100, 499)], 600, 300)

        for (const block of layout.blocks) {
            expect(block.cols * block.rows).toBeGreaterThanOrEqual(block.slots.length)
            expect(block.slots[0]).toBeGreaterThanOrEqual(0)
        }
        expect(layout.blocks[0].slots).toHaveLength(100)
        expect(layout.blocks[1].slots).toHaveLength(400)
    })

    it('按坐标能反查到正确的槽号，节点间隙返回 null', () => {
        const layout = computeSlotLayout([node(1, 0, 99), node(2, 100, 499)], 600, 300)
        const first = layout.blocks[0]
        const second = layout.blocks[1]

        expect(slotAtPoint(layout, first.x + 1, layout.labelHeight + 1)).toBe(0)
        expect(slotAtPoint(layout, second.x + 1, layout.labelHeight + 1)).toBe(100)

        const gapX = first.x + first.width + 1
        expect(slotAtPoint(layout, gapX, layout.labelHeight + 1)).toBeNull()
        expect(slotAtPoint(layout, first.x + 1, 1)).toBeNull()
    })

    it('容器过窄时仍能生成布局而不是崩溃', () => {
        const layout = computeSlotLayout([node(1, 0, 16383)], 40, 300)

        expect(layout).not.toBeNull()
        expect(layout.blocks).toHaveLength(1)
        expect(layout.cell).toBeGreaterThan(0)
        expect(layout.blocks[0].cols * layout.blocks[0].rows).toBeGreaterThanOrEqual(16384)
    })
})

// 验证框选：单格、跨节点块、标签区域和越界坐标都要得到正确结果。
describe('slotsInRect', () => {
    const nodes = [node(1, 0, 99), node(2, 100, 499)]

    it('框选单个单元格只返回该槽号', () => {
        const layout = computeSlotLayout(nodes, 600, 300)
        const block = layout.blocks[0]
        const x = block.x + layout.cell * 0.5
        const y = layout.labelHeight + layout.cell * 0.5
        expect(slotsInRect(layout, x, y, x, y)).toEqual([0])
    })

    it('跨节点块的矩形会包含两个节点的槽号', () => {
        const layout = computeSlotLayout(nodes, 600, 300)
        const first = layout.blocks[0]
        const second = layout.blocks[1]
        const slots = slotsInRect(layout, first.x, layout.labelHeight, second.x + layout.cell, layout.labelHeight + layout.cell)
        expect(slots).toContain(0)
        expect(slots).toContain(100)
    })

    it('标签区域和画布外不会选中槽位', () => {
        const layout = computeSlotLayout(nodes, 600, 300)
        expect(slotsInRect(layout, 0, 0, 600, layout.labelHeight - 1)).toEqual([])
        expect(slotsInRect(layout, -100, -100, -1, -1)).toEqual([])
    })

    it('拖拽方向反转时结果一致', () => {
        const layout = computeSlotLayout(nodes, 600, 300)
        const block = layout.blocks[0]
        const x1 = block.x + layout.cell * 2
        const y1 = layout.labelHeight + layout.cell * 2
        const forward = slotsInRect(layout, block.x + 1, layout.labelHeight + 1, x1, y1)
        const backward = slotsInRect(layout, x1, y1, block.x + 1, layout.labelHeight + 1)
        expect(forward).toEqual(backward)
    })
})
