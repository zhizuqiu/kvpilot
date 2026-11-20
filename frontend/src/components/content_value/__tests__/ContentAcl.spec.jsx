import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { NConfigProvider } from 'naive-ui'
import { createI18n } from 'vue-i18n'
import zhCn from '@/langs/zh-cn.json'

// 后端接口全部替换为可断言的 mock，避免组件真的发起 HTTP 请求
const api = vi.hoisted(() => ({
    AclOverview: vi.fn(),
    AclUser: vi.fn(),
    AclCategories: vi.fn(),
    AclSetUser: vi.fn(),
    AclDeleteUser: vi.fn(),
    AclLog: vi.fn(),
    AclDryRun: vi.fn(),
}))

vi.mock('wailsjs/go/services/browserService.js', () => api)

import ContentAcl from '@/components/content_value/ContentAcl.vue'

const defaultDetail = {
    flags: ['on', 'nopass'],
    passwordHashes: [],
    commands: '+@all',
    keys: '~*',
    channels: '&*',
    nodes: 3,
    totalNodes: 3,
}

const appDetail = {
    flags: ['on'],
    passwordHashes: ['abc123'],
    commands: '-@all +@read',
    keys: '~cache:*',
    channels: '&*',
    nodes: 1,
    totalNodes: 3,
}

const overview = {
    success: true,
    data: {
        whoami: 'default',
        nodes: 3,
        users: [
            { name: 'app', enabled: true, nodes: 1, totalNodes: 3 },
            { name: 'default', enabled: true, nodes: 3, totalNodes: 3 },
        ],
    },
}

// 组件内部直接用全局的 $dialog/$message，测试里替换成可断言的桩
const setupGlobals = () => {
    const dialog = { warning: vi.fn((content, onConfirm) => onConfirm && onConfirm()) }
    const message = { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() }
    window.$dialog = dialog
    window.$message = message
    return { dialog, message }
}

const mountAcl = async () => {
    const { dialog, message } = setupGlobals()
    const i18n = createI18n({ legacy: false, locale: 'zh-cn', messages: { 'zh-cn': zhCn } })
    const wrapper = mount(
        {
            render: () => (
                <NConfigProvider>
                    <ContentAcl server="test-server" />
                </NConfigProvider>
            ),
        },
        { global: { plugins: [i18n] } }
    )
    await flushPromises()
    return { wrapper, dialog, message }
}

const buttonByText = (wrapper, text) => wrapper.findAll('button').find((b) => b.text().includes(text))

const selectUser = async (wrapper, name) => {
    const row = wrapper.findAll('.acl-user-item').find((r) => r.text().includes(name))
    await row.trigger('click')
    await flushPromises()
}

beforeEach(() => {
    vi.clearAllMocks()
    api.AclOverview.mockResolvedValue(overview)
    api.AclCategories.mockResolvedValue({ success: true, data: ['read', 'write'] })
    api.AclUser.mockImplementation((server, user) => ({
        success: true,
        data: user === 'app' ? appDetail : defaultDetail,
    }))
    api.AclSetUser.mockResolvedValue({ success: true, data: appDetail })
    api.AclDeleteUser.mockResolvedValue({ success: true, data: { deleted: 1 } })
})

describe('ContentAcl', () => {
    it('挂载后加载用户列表并展示当前连接用户与节点数', async () => {
        const { wrapper } = await mountAcl()

        expect(api.AclOverview).toHaveBeenCalledWith('test-server')
        expect(wrapper.findAll('.acl-user-item')).toHaveLength(2)
        expect(wrapper.find('.acl-whoami').text()).toContain('default')
        expect(wrapper.text()).toContain('共 2 个用户')
        expect(wrapper.text()).toContain('共 3 个节点')
    })

    it('自动选中当前连接用户并标记为只读，不提供保存入口', async () => {
        const { wrapper } = await mountAcl()
        await flushPromises()

        expect(api.AclUser).toHaveBeenCalledWith('test-server', 'default')
        expect(wrapper.text()).toContain('当前连接正在使用该用户')
        expect(buttonByText(wrapper, '保存')).toBeUndefined()
        expect(buttonByText(wrapper, '删除')).toBeUndefined()
    })

    it('切换到其他用户后展示保存入口，未修改时按钮禁用', async () => {
        const { wrapper } = await mountAcl()

        await selectUser(wrapper, 'app')

        const save = buttonByText(wrapper, '保存')
        expect(save).toBeTruthy()
        expect(save.attributes('disabled')).toBeDefined()
        // 该用户只在 1/3 个节点存在，必须给出部分节点提示
        expect(wrapper.text()).toContain('1/3')
    })

    it('保存时把已有密码哈希一并回填，避免清空用户密码', async () => {
        const { wrapper, dialog } = await mountAcl()
        await selectUser(wrapper, 'app')

        await wrapper.find('input[type="password"]').setValue('newpass')
        await flushPromises()

        const save = buttonByText(wrapper, '保存')
        expect(save.attributes('disabled')).toBeUndefined()
        await save.trigger('click')
        await flushPromises()

        expect(dialog.warning).toHaveBeenCalled()
        expect(api.AclSetUser).toHaveBeenCalledTimes(1)
        const [, user, rules] = api.AclSetUser.mock.calls[0]
        expect(user).toBe('app')
        expect(rules).toContain('reset')
        expect(rules).toContain('on')
        expect(rules).toContain('#abc123')
        expect(rules).toContain('>newpass')
    })

    it('有未保存修改时禁用权限测试并提示先保存', async () => {
        const { wrapper } = await mountAcl()
        await selectUser(wrapper, 'app')

        const testButton = buttonByText(wrapper, '测试')
        expect(testButton.attributes('disabled')).toBeUndefined()

        await wrapper.find('input[type="password"]').setValue('x')
        await flushPromises()

        expect(buttonByText(wrapper, '测试').attributes('disabled')).toBeDefined()
        expect(wrapper.text()).toContain('有未保存的修改，保存后才能测试')
    })

    it('删除用户必须二次确认，确认后才调用后端', async () => {
        const { wrapper, dialog } = await mountAcl()
        await selectUser(wrapper, 'app')

        await buttonByText(wrapper, '删除').trigger('click')
        await flushPromises()

        expect(dialog.warning).toHaveBeenCalledTimes(1)
        expect(dialog.warning.mock.calls[0][0]).toContain('app')
        expect(api.AclDeleteUser).toHaveBeenCalledWith('test-server', 'app')
    })

    it('逐条删除密码哈希后保存，不再提交该哈希', async () => {
        const { wrapper } = await mountAcl()
        await selectUser(wrapper, 'app')

        // 密码列表里每行有一个"删除"按钮
        const removePassword = wrapper.find('.list-row button')
        expect(removePassword.exists()).toBe(true)
        await removePassword.trigger('click')
        await flushPromises()

        const save = buttonByText(wrapper, '保存')
        expect(save.attributes('disabled')).toBeUndefined()
        await save.trigger('click')
        await flushPromises()

        const [, , rules] = api.AclSetUser.mock.calls[0]
        expect(rules).not.toContain('#abc123')
    })

    it('切换为指定 Key 模式后添加的模式会带 ~ 前缀提交', async () => {
        const { wrapper } = await mountAcl()
        await selectUser(wrapper, 'app')

        // Key 与 Channel 各有一组"全部/指定"，第 2 个单选项属于 Key
        const customRadios = wrapper
            .findAll('.n-radio-button')
            .filter((r) => r.text().includes('指定模式'))
        expect(customRadios.length).toBe(2)
        await customRadios[0].trigger('click')
        await flushPromises()

        const keyInput = wrapper.find('input[placeholder="Key 模式，空格分隔，如：app:* cache:*"]')
        await keyInput.setValue('order:*')
        await buttonByText(wrapper, '添加').trigger('click')
        await flushPromises()

        await buttonByText(wrapper, '保存').trigger('click')
        await flushPromises()

        const [, , rules] = api.AclSetUser.mock.calls[0]
        expect(rules).toContain('~order:*')
        expect(rules).not.toContain('~*')
    })

    it('权限测试允许时展示允许结果', async () => {
        const { wrapper } = await mountAcl()
        await selectUser(wrapper, 'app')
        api.AclDryRun.mockResolvedValue({ success: true, data: { allowed: true, message: 'OK' } })

        await wrapper.find('input[placeholder="例如：set app:foo 1"]').setValue('get cache:1')
        await buttonByText(wrapper, '测试').trigger('click')
        await flushPromises()

        expect(api.AclDryRun).toHaveBeenCalledWith('test-server', 'app', ['get', 'cache:1'])
        expect(wrapper.text()).toContain('允许执行')
    })

    it('切换到日志页签时按固定条数拉取并渲染拒绝记录', async () => {
        const { wrapper } = await mountAcl()
        api.AclLog.mockResolvedValue({
            success: true,
            data: [{ reason: 'command', context: 'toplevel', username: 'app', object: 'del', 'age-seconds': '12' }],
        })

        const logTab = wrapper.findAll('.n-tabs-tab').find((t) => t.text().includes('ACL 日志'))
        await logTab.trigger('click')
        await flushPromises()

        expect(api.AclLog).toHaveBeenCalledWith('test-server', 20)
        expect(wrapper.text()).toContain('command')
        expect(wrapper.text()).toContain('toplevel')
    })

    it('列表加载失败时给出警告而不是显示空列表', async () => {
        setupGlobals()
        api.AclOverview.mockResolvedValue({ success: false, msg: '连接已断开' })

        const i18n = createI18n({ legacy: false, locale: 'zh-cn', messages: { 'zh-cn': zhCn } })
        mount(
            {
                render: () => (
                    <NConfigProvider>
                        <ContentAcl server="test-server" />
                    </NConfigProvider>
                ),
            },
            { global: { plugins: [i18n] } }
        )
        await flushPromises()

        expect(window.$message.warning).toHaveBeenCalledWith('连接已断开')
    })
})
