/**
 * 配置参数的筛选逻辑。
 *
 * 放在独立模块里便于单测：分类、关键字与节点一致性三类条件需要组合生效，
 * 且一致性枚举（一致 / 不一致 / 敏感 / 不可变）语义各不相同。
 */

/**
 * @param {Array} params 后端返回的参数列表
 * @param {{category?: string, keyword?: string, consistency?: string}} filters
 * @return {Array}
 */
export const filterConfigParams = (params, filters = {}) => {
    const { category = 'all', keyword = '', consistency = 'all' } = filters
    const kw = String(keyword).trim().toLowerCase()

    return (params || []).filter((param) => {
        if (category !== 'all' && param.category !== category) return false

        switch (consistency) {
            case 'consistent':
                if (!param.consistent) return false
                break
            case 'inconsistent':
                if (param.consistent) return false
                break
            case 'sensitive':
                if (!param.sensitive) return false
                break
            case 'immutable':
                if (!param.immutable) return false
                break
            default:
                break
        }

        if (kw === '') return true
        return (
            String(param.key).toLowerCase().includes(kw) ||
            String(param.value).toLowerCase().includes(kw)
        )
    })
}
