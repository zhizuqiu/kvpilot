import { describe, expect, it } from 'vitest'
import { aclFormFromDetail, aclSnapshotsEqual, buildAclRules, parseAclCommands } from '@/utils/acl.js'

// 构造一份默认表单，避免每个用例重复写全字段
const form = (overrides = {}) => ({
    enabled: true,
    nopass: false,
    hashes: [],
    newPassword: '',
    commandMode: 'all',
    categories: [],
    extraRules: '',
    keysAll: true,
    keyPatterns: [],
    channelsAll: true,
    channelPatterns: [],
    ...overrides,
})

describe('buildAclRules', () => {
    it('全权限用户生成 reset/on/+@all/~*/&*', () => {
        expect(buildAclRules(form())).toEqual(['reset', 'on', '+@all', '~*', '&*'])
    })

    it('保存时必须回填已有密码哈希，否则会清空用户密码', () => {
        // 这是整个 ACL 功能风险最高的一条：漏掉 #hash 就等于把密码删了
        const rules = buildAclRules(form({ hashes: ['aaa', 'bbb'] }))
        expect(rules).toContain('#aaa')
        expect(rules).toContain('#bbb')
        expect(rules.some((r) => r.startsWith('>'))).toBe(false)
    })

    it('新填密码以 > 明文追加，与已有哈希共存', () => {
        const rules = buildAclRules(form({ hashes: ['aaa'], newPassword: 'pw123' }))
        expect(rules).toContain('#aaa')
        expect(rules).toContain('>pw123')
    })

    it('nopass 时不输出任何密码规则', () => {
        const rules = buildAclRules(form({ nopass: true, hashes: ['aaa'], newPassword: 'x' }))
        expect(rules).toContain('nopass')
        expect(rules.some((r) => r.startsWith('#') || r.startsWith('>'))).toBe(false)
    })

    it('禁用用户输出 off', () => {
        expect(buildAclRules(form({ enabled: false }))).toContain('off')
    })

    it('自定义命令模式输出 -@all 加类别与附加规则，并忽略多余空白', () => {
        const rules = buildAclRules(
            form({
                commandMode: 'custom',
                categories: ['read', 'write'],
                extraRules: '+get   -flushall ',
            })
        )
        expect(rules).toEqual(['reset', 'on', '-@all', '+@read', '+@write', '+get', '-flushall', '~*', '&*'])
    })

    it('指定的 key 模式自动补 ~ 前缀，已带前缀的保持原样', () => {
        const rules = buildAclRules(form({ keysAll: false, keyPatterns: ['cache:*', '~x', '%R~read:*'] }))
        expect(rules).toContain('~cache:*')
        expect(rules).toContain('~x')
        expect(rules).toContain('%R~read:*')
        expect(rules).not.toContain('~*')
    })

    it('指定的频道模式自动补 & 前缀，已带前缀的保持原样', () => {
        const rules = buildAclRules(form({ channelsAll: false, channelPatterns: ['chan:*', '&other'] }))
        expect(rules).toContain('&chan:*')
        expect(rules).toContain('&other')
        expect(rules).not.toContain('&*')
    })
})

describe('parseAclCommands', () => {
    it('识别全权限标记', () => {
        expect(parseAclCommands('+@all')).toEqual({ all: true, cats: [], extras: [] })
    })

    it('拆分类别与具体命令规则', () => {
        expect(parseAclCommands('-@all +@read +@write -flushall')).toEqual({
            all: false,
            cats: ['read', 'write'],
            extras: ['-flushall'],
        })
    })

    it('空值返回安全的空结构', () => {
        expect(parseAclCommands('')).toEqual({ all: false, cats: [], extras: [] })
        expect(parseAclCommands(null)).toEqual({ all: false, cats: [], extras: [] })
    })
})

describe('aclFormFromDetail', () => {
    it('全权限用户回填为 all 模式且 key/channel 为全部', () => {
        const result = aclFormFromDetail({
            flags: ['on'],
            passwordHashes: ['h1'],
            commands: '+@all',
            keys: '~*',
            channels: '&*',
        })
        expect(result.enabled).toBe(true)
        expect(result.nopass).toBe(false)
        expect(result.hashes).toEqual(['h1'])
        expect(result.commandMode).toBe('all')
        expect(result.keysAll).toBe(true)
        expect(result.channelsAll).toBe(true)
    })

    it('自定义规则回填为 custom 模式并拆出类别与附加规则', () => {
        const result = aclFormFromDetail({
            flags: ['on', 'nopass'],
            commands: '-@all +@read -flushall',
            keys: '~a:* ~b:*',
            channels: '&c:*',
        })
        expect(result.nopass).toBe(true)
        expect(result.commandMode).toBe('custom')
        expect(result.categories).toEqual(['read'])
        expect(result.extraRules).toBe('-flushall')
        expect(result.keysAll).toBe(false)
        expect(result.keyPatterns).toEqual(['~a:*', '~b:*'])
        expect(result.channelsAll).toBe(false)
        expect(result.channelPatterns).toEqual(['&c:*'])
    })

    it('缺少 on 标记时视为禁用，空详情不抛错', () => {
        expect(aclFormFromDetail({ flags: [] }).enabled).toBe(false)
        expect(() => aclFormFromDetail(null)).not.toThrow()
    })
})

describe('aclSnapshotsEqual', () => {
    it('哈希与类别的顺序不影响脏状态判断', () => {
        const a = form({ hashes: ['a', 'b'], categories: ['read', 'write'], commandMode: 'custom' })
        const b = form({ hashes: ['b', 'a'], categories: ['write', 'read'], commandMode: 'custom' })
        expect(aclSnapshotsEqual(a, b)).toBe(true)
    })

    it('附加规则的顺序与多余空白不影响脏状态判断', () => {
        const a = form({ commandMode: 'custom', extraRules: '+get  +set' })
        const b = form({ commandMode: 'custom', extraRules: '+set   +get ' })
        expect(aclSnapshotsEqual(a, b)).toBe(true)
    })

    it('新密码会立刻标记为有未保存修改', () => {
        expect(aclSnapshotsEqual(form(), form({ newPassword: 'x' }))).toBe(false)
    })

    it('启用状态变化必须被识别为脏', () => {
        expect(aclSnapshotsEqual(form(), form({ enabled: false }))).toBe(false)
    })

    it('key 模式在"全部"与"指定"之间切换必须被识别为脏', () => {
        expect(aclSnapshotsEqual(form(), form({ keysAll: false }))).toBe(false)
    })
})
