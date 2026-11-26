/**
 * 解析用户粘贴的槽位号文本。
 *
 * 支持逗号（含中文逗号）分隔的单个槽位或区间，例如：
 * `0-6, 8, 10-12`，与分布图上方复制的槽位范围格式一致。
 */

export const SLOT_MAX = 16383

export function parseSlotList(text, max = SLOT_MAX) {
    const tokens = String(text || '')
        .split(/[,，]/)
        .map((token) => token.trim())
        .filter(Boolean)

    const slots = []
    const seen = new Set()
    for (const token of tokens) {
        const match = /^(\d+)(?:\s*-\s*(\d+))?$/.exec(token)
        if (!match) {
            return { slots: [], error: token }
        }
        const start = Number(match[1])
        const end = match[2] === undefined ? start : Number(match[2])
        if (start > end || end > max) {
            return { slots: [], error: token }
        }
        for (let slot = start; slot <= end; slot++) {
            if (!seen.has(slot)) {
                seen.add(slot)
                slots.push(slot)
            }
        }
    }
    return { slots, error: '' }
}

// 把槽号列表压缩成区间文本，例如 [0,1,2,4] -> ["0-2", "4"]。
export function formatSlotRanges(slots) {
    const sorted = [...new Set(slots)].sort((a, b) => a - b)
    const ranges = []
    let start = null
    let prev = null
    for (const slot of sorted) {
        if (start === null) {
            start = slot
            prev = slot
            continue
        }
        if (slot === prev + 1) {
            prev = slot
            continue
        }
        ranges.push(start === prev ? `${start}` : `${start}-${prev}`)
        start = slot
        prev = slot
    }
    if (start !== null) {
        ranges.push(start === prev ? `${start}` : `${start}-${prev}`)
    }
    return ranges
}
