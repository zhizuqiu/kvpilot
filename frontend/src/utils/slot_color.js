/**
 * 槽位分布图的颜色标尺。
 *
 * 直接按 0~最大值 归一化时，均匀集群（每槽 10~25 键）会全部落在色阶顶端，
 * 看起来是一片同色。这里改用非零槽位的 P2~P98 作为色阶区间，让常见区间
 * 占满整条梯度，极端值饱和处理。
 */

const clamp01 = (value) => Math.min(1, Math.max(0, value))

// 计算非零槽位的色阶范围；样本很少时退化为 min~max。
export function buildSlotScale(counts, lowPercentile = 0.02, highPercentile = 0.98) {
    const values = []
    for (let i = 0; i < counts.length; i++) {
        if (counts[i] > 0) values.push(counts[i])
    }
    if (values.length === 0) {
        return { lo: 0, hi: 0, uniform: true }
    }
    values.sort((a, b) => a - b)
    const pick = (percentile) => values[Math.min(values.length - 1, Math.floor((values.length - 1) * percentile))]
    let lo = pick(lowPercentile)
    let hi = pick(highPercentile)
    if (hi <= lo) {
        lo = values[0]
        hi = values[values.length - 1]
    }
    return { lo, hi, uniform: hi <= lo }
}

// 把单个槽的键数映射到 0~1；0 键返回 0，范围外饱和。
export function scaleRatio(value, scale, mode = 'log') {
    if (value <= 0) return 0
    if (!scale || scale.uniform) return 0.55
    const { lo, hi } = scale
    if (value <= lo) return 0
    if (value >= hi) return 1
    if (mode === 'log') {
        return clamp01((Math.log1p(value) - Math.log1p(lo)) / (Math.log1p(hi) - Math.log1p(lo)))
    }
    return clamp01((value - lo) / (hi - lo))
}
