/**
 * 槽位分布图的热点色阶。
 *
 * 颜色表达的是“相对热点的程度”，而不是绝对键数：
 * 以非零槽位键数的中位数为基准，1 倍中位数及以下为最浅，
 * 2 倍中位数及以上为最深。均匀集群因此整体呈浅色，只有真正
 * 偏离中位数的槽才会显眼。
 */

const clamp01 = (value) => Math.min(1, Math.max(0, value))

// 计算热点基准；没有非零槽位时返回 0。
export function buildHotspotScale(counts) {
    const values = []
    for (let i = 0; i < counts.length; i++) {
        if (counts[i] > 0) values.push(counts[i])
    }
    if (values.length === 0) return { median: 0 }
    values.sort((a, b) => a - b)
    const mid = Math.floor(values.length / 2)
    const median = values.length % 2 === 1 ? values[mid] : (values[mid - 1] + values[mid]) / 2
    return { median }
}

// 把键数映射到 0~1 的热点比例：1x 中位数为 0，2x 及以上为 1。
export function hotspotRatio(value, scale) {
    if (value <= 0 || !scale || scale.median <= 0) return 0
    if (value <= scale.median) return 0
    return clamp01(Math.log2(value / scale.median))
}

// 把热点比例映射为不透明度：1x 中位数为 0.05，2x 及以上为 1。
export function hotspotAlpha(ratio) {
    return 0.05 + 0.95 * clamp01(ratio)
}
