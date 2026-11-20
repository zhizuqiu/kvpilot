/**
 * ACL 规则拼装与表单回填的纯逻辑。
 *
 * 这些函数从 ContentAcl.vue 中抽出，原因有二：
 * 1. 保存时漏掉 `#hash` 会直接清空用户密码，必须能被单测覆盖；
 * 2. 脏状态判断需要稳定的快照序列化，散在组件里无法验证。
 */

const splitTokens = (text) => String(text || '').split(/\s+/).filter(Boolean)

/**
 * 解析 ACL GETUSER 返回的 commands 字段。
 * @param {string} text 例如 "-@all +@read -flushall"
 * @return {{all: boolean, cats: string[], extras: string[]}}
 */
export const parseAclCommands = (text) => {
    const cats = []
    const extras = []
    let all = false
    splitTokens(text).forEach((t) => {
        if (t === '+@all') all = true
        else if (t === '-@all') all = false
        else if (t.startsWith('+@')) cats.push(t.slice(2))
        else extras.push(t)
    })
    return { all, cats, extras }
}

/**
 * 把表单拼装成 ACL SETUSER 的规则数组。
 * 始终以 reset 开头，再回填已有密码哈希，保证修改规则不会丢密码。
 * @param {object} form ACL 表单
 * @return {string[]}
 */
export const buildAclRules = (form) => {
    const rules = ['reset', form.enabled ? 'on' : 'off']

    if (form.nopass) {
        rules.push('nopass')
    } else {
        form.hashes.forEach((hash) => rules.push('#' + hash))
        const password = String(form.newPassword || '').trim()
        if (password !== '') rules.push('>' + password)
    }

    if (form.commandMode === 'all') {
        rules.push('+@all')
    } else {
        rules.push('-@all')
        form.categories.forEach((cat) => rules.push('+@' + cat))
        splitTokens(form.extraRules).forEach((rule) => rules.push(rule))
    }

    if (form.keysAll) rules.push('~*')
    else form.keyPatterns.forEach((p) => rules.push(/^(~|%R~|%W~)/.test(p) ? p : '~' + p))

    if (form.channelsAll) rules.push('&*')
    else form.channelPatterns.forEach((p) => rules.push(p.startsWith('&') ? p : '&' + p))

    return rules
}

/**
 * 用 ACL GETUSER 的详情回填表单字段。
 * @param {object|null} data aclUserDetail
 * @return {object} 表单字段（不含 newPassword，由调用方显式清空）
 */
export const aclFormFromDetail = (data) => {
    if (!data) {
        return {
            enabled: false,
            nopass: false,
            hashes: [],
            commandMode: 'all',
            categories: [],
            extraRules: '',
            keysAll: true,
            keyPatterns: [],
            channelsAll: true,
            channelPatterns: [],
        }
    }

    const flags = data.flags || []
    const { all, cats, extras } = parseAclCommands(data.commands)
    const keys = String(data.keys || '').trim()
    const channels = String(data.channels || '').trim()
    const keysAll = keys === '~*'
    const channelsAll = channels === '&*'

    return {
        enabled: flags.includes('on'),
        nopass: flags.includes('nopass'),
        hashes: [...(data.passwordHashes || [])],
        // 只有全权限且没有单列类别时才视为"全部"模式，否则回填会丢规则
        commandMode: all && cats.length === 0 ? 'all' : 'custom',
        categories: cats,
        extraRules: extras.join(' '),
        keysAll,
        keyPatterns: keysAll ? [] : splitTokens(keys),
        channelsAll,
        channelPatterns: channelsAll ? [] : splitTokens(channels),
    }
}

/**
 * 生成用于判断"是否有未保存修改"的稳定快照。
 * 哈希、类别与附加规则的顺序不影响结果，避免仅调整顺序就被标记为脏。
 * @param {object} form
 * @return {string}
 */
export const aclSnapshot = (form) =>
    JSON.stringify({
        enabled: form.enabled,
        nopass: form.nopass,
        hashes: [...form.hashes].sort(),
        mode: form.commandMode,
        categories: [...form.categories].sort(),
        extraRules: splitTokens(form.extraRules).sort().join(' '),
        keysAll: form.keysAll,
        keys: form.keysAll ? [] : [...form.keyPatterns].sort(),
        channelsAll: form.channelsAll,
        channels: form.channelsAll ? [] : [...form.channelPatterns].sort(),
        hasNewPassword: String(form.newPassword || '').trim() !== '',
    })

/**
 * 比较两份表单是否等价。
 * @param {object} a
 * @param {object} b
 * @return {boolean}
 */
export const aclSnapshotsEqual = (a, b) => aclSnapshot(a) === aclSnapshot(b)
