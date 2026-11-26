/**
 * 槽位分布图的布局计算。
 *
 * 16384 个槽按主节点切成若干矩形，矩形从左到右排列；矩形宽度与节点槽位数成正比，
 * 这样各节点的单元格大小一致，密度可以直接横向比较。矩形高度固定，槽号在矩形内
 * 按升序逐行排列。
 */

export const SLOT_COUNT = 16384

// 把节点的槽位区间展开为升序槽号列表。
export const buildSlotLists = (nodes) =>
    nodes.map((node) => {
        const slots = []
        for (const range of node.slotRanges || []) {
            const start = Math.max(0, range.start)
            const end = Math.min(SLOT_COUNT - 1, range.end)
            for (let slot = start; slot <= end; slot++) {
                slots.push(slot)
            }
        }
        slots.sort((a, b) => a - b)
        return slots
    })

// 计算每个节点的矩形位置。优先保证单元格尽量大：在能放进给定宽度的前提下取最少行数。
export function computeSlotLayout(nodes, width, height, options = {}) {
    const labelHeight = options.labelHeight ?? 16
    const gap = options.gap ?? 8
    const count = nodes.length
    const usableWidth = width - gap * Math.max(0, count - 1)
    const usableHeight = height - labelHeight
    if (count === 0 || usableWidth <= 0 || usableHeight <= 0) {
        return null
    }

    const lists = buildSlotLists(nodes)
    let rows = 1
    let cell = usableHeight
    let found = false
    for (let candidate = 1; candidate <= usableHeight; candidate++) {
        const size = usableHeight / candidate
        let totalWidth = 0
        for (const slots of lists) {
            const cols = Math.ceil(Math.max(1, slots.length) / candidate)
            totalWidth += cols * size
            if (totalWidth > usableWidth) break
        }
        if (totalWidth <= usableWidth) {
            rows = candidate
            cell = size
            found = true
            break
        }
    }
    if (!found) {
        // 极窄容器：退化为每个节点一列，允许水平方向被压缩。
        rows = Math.max(1, Math.ceil(Math.sqrt(SLOT_COUNT)))
        cell = Math.min(usableHeight / rows, usableWidth / Math.max(1, count))
    }

    const blocks = []
    let x = 0
    lists.forEach((slots, index) => {
        const cols = Math.ceil(Math.max(1, slots.length) / rows)
        const blockWidth = cols * cell
        blocks.push({ index, x, width: blockWidth, cols, rows, slots })
        x += blockWidth
    })

    // 富余宽度均分到节点之间，避免右侧出现大片空白。
    const usedWidth = x
    const leftover = Math.max(0, usableWidth - usedWidth)
    const extraGap = count > 1 ? leftover / (count - 1) : 0
    let shift = 0
    blocks.forEach((block, index) => {
        block.x += shift
        if (index < count - 1) {
            shift += extraGap
        }
    })

    return { rows, cell, labelHeight, blocks, width, height }
}

// 根据画布坐标反查槽号；落在节点间隙或空单元格时返回 null。
export function slotAtPoint(layout, x, y) {
    if (!layout || y < layout.labelHeight) return null
    for (const block of layout.blocks) {
        if (x < block.x || x >= block.x + block.width) continue
        const col = Math.floor((x - block.x) / layout.cell)
        const row = Math.floor((y - layout.labelHeight) / layout.cell)
        if (col < 0 || col >= block.cols || row < 0 || row >= block.rows) return null
        const index = row * block.cols + col
        return index < block.slots.length ? block.slots[index] : null
    }
    return null
}

// 返回矩形范围内的槽号；矩形可跨多个节点块，标签区域不计入。
export function slotsInRect(layout, x1, y1, x2, y2) {
    if (!layout) return []
    const left = Math.min(x1, x2)
    const right = Math.max(x1, x2)
    const top = Math.max(layout.labelHeight, Math.min(y1, y2))
    const bottom = Math.max(y1, y2)
    const slots = []
    for (const block of layout.blocks) {
        if (right < block.x || left >= block.x + block.width) continue
        const startCol = Math.max(0, Math.floor((left - block.x) / layout.cell))
        const endCol = Math.min(block.cols - 1, Math.floor((right - block.x) / layout.cell))
        const startRow = Math.max(0, Math.floor((top - layout.labelHeight) / layout.cell))
        const endRow = Math.min(block.rows - 1, Math.floor((bottom - layout.labelHeight) / layout.cell))
        for (let row = startRow; row <= endRow; row++) {
            for (let col = startCol; col <= endCol; col++) {
                const index = row * block.cols + col
                if (index < block.slots.length) {
                    slots.push(block.slots[index])
                }
            }
        }
    }
    return slots
}
