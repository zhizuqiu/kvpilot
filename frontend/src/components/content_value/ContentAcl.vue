<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
    NButton,
    NCheckbox,
    NCheckboxGroup,
    NDataTable,
    NEmpty,
    NInput,
    NModal,
    NRadioButton,
    NRadioGroup,
    NSelect,
    NTabPane,
    NTabs,
    NTag,
    NSwitch,
    useThemeVars,
} from 'naive-ui'
import Refresh from '@/components/icons/Refresh.vue'
import Play from '@/components/icons/Play.vue'
import Plus from '@/components/icons/Plus.vue'
import Save from '@/components/icons/Save.vue'
import Delete from '@/components/icons/Delete.vue'
import {
    AclCategories,
    AclDeleteUser,
    AclDryRun,
    AclLog,
    AclOverview,
    AclSetUser,
    AclUser,
} from 'wailsjs/go/services/browserService.js'

const props = defineProps({
    server: String,
    pause: Boolean,
})

const i18n = useI18n()
const themeVars = useThemeVars()

const loading = ref(false)
const whoami = ref('')
const users = ref([])
const nodeCount = ref(1)
const selected = ref('')
const detail = ref(null)
const categories = ref([])
const logs = ref([])
const logLoading = ref(false)
const tab = ref('users')
const saving = ref(false)

const showCreate = ref(false)
const newUserName = ref('')

const form = reactive({
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
})

const newKey = ref('')
const newChannel = ref('')

const dryCommand = ref('')
const dryResult = ref(null)
const dryLoading = ref(false)

let detailSeq = 0
const baseline = ref('')

const totalUsers = computed(() => users.value.length)
const isSelf = computed(() => selected.value !== '' && selected.value === whoami.value)
const editable = computed(() => selected.value !== '' && !isSelf.value && detail.value != null)
const hashCount = computed(() => form.hashes.length)
const isPartial = computed(() => (detail.value?.nodes || 0) > 0 && detail.value.nodes < detail.value.totalNodes)

// 把可编辑字段规整成可比较的字符串，用于判断是否有未保存修改
const snapshot = () =>
    JSON.stringify({
        enabled: form.enabled,
        nopass: form.nopass,
        hashes: [...form.hashes].sort(),
        mode: form.commandMode,
        categories: [...form.categories].sort(),
        extraRules: form.extraRules
            .split(/\s+/)
            .filter(Boolean)
            .sort()
            .join(' '),
        keysAll: form.keysAll,
        keys: form.keysAll ? [] : [...form.keyPatterns].sort(),
        channelsAll: form.channelsAll,
        channels: form.channelsAll ? [] : [...form.channelPatterns].sort(),
        hasNewPassword: form.newPassword.trim() !== '',
    })

const dirty = computed(() => baseline.value !== '' && snapshot() !== baseline.value)

const categoryOptions = computed(() =>
    categories.value.map((cat) => ({ label: '@' + cat, value: cat }))
)

const parseCommands = (text) => {
    const tokens = (text || '').split(/\s+/).filter(Boolean)
    const cats = []
    const extras = []
    let all = false
    tokens.forEach((t) => {
        if (t === '+@all') all = true
        else if (t === '-@all') all = false
        else if (t.startsWith('+@')) cats.push(t.slice(2))
        else extras.push(t)
    })
    return { all, cats, extras }
}

const fillForm = (data) => {
    if (!data) return
    const { all, cats, extras } = parseCommands(data.commands)
    form.enabled = (data.flags || []).includes('on')
    form.nopass = (data.flags || []).includes('nopass')
    form.hashes = [...(data.passwordHashes || [])]
    form.newPassword = ''
    form.commandMode = all && cats.length === 0 ? 'all' : 'custom'
    form.categories = cats
    form.extraRules = extras.join(' ')
    const keys = (data.keys || '').trim()
    form.keysAll = keys === '~*'
    form.keyPatterns = form.keysAll ? [] : keys.split(/\s+/).filter(Boolean)
    const channels = (data.channels || '').trim()
    form.channelsAll = channels === '&*'
    form.channelPatterns = form.channelsAll ? [] : channels.split(/\s+/).filter(Boolean)
}

const loadOverview = async (silent = false) => {
    if (loading.value) return
    loading.value = true
    try {
        const { success, data, msg } = await AclOverview(props.server)
        if (success) {
            whoami.value = data?.whoami || ''
            users.value = data?.users || []
            nodeCount.value = data?.nodes || 1
            const exists = users.value.some((u) => u.name === selected.value)
            if (!exists) {
                const preferred = users.value.find((u) => u.name === whoami.value) || users.value[0]
                if (preferred) selectUser(preferred.name)
            }
        } else if (!silent && msg) {
            $message.warning(msg)
        }
    } finally {
        loading.value = false
    }
}

const loadDetail = async (user) => {
    if (!user) return
    const seq = ++detailSeq
    const { success, data, msg } = await AclUser(props.server, user)
    if (seq !== detailSeq) return
    if (success) {
        detail.value = data || null
        fillForm(detail.value)
        baseline.value = snapshot()
        dryResult.value = null
    } else {
        detail.value = null
        if (msg) $message.warning(msg)
    }
}

const loadCategories = async () => {
    const { success, data } = await AclCategories(props.server)
    if (success) categories.value = data || []
}

const loadLogs = async () => {
    logLoading.value = true
    try {
        const { success, data } = await AclLog(props.server, 20)
        logs.value = success ? data || [] : []
    } finally {
        logLoading.value = false
    }
}

const selectUser = (name) => {
    selected.value = name
    loadDetail(name)
}

const addPattern = (kind) => {
    const value = (kind === 'key' ? newKey.value : newChannel.value).trim()
    if (value === '') return
    const list = kind === 'key' ? form.keyPatterns : form.channelPatterns
    if (!list.includes(value)) list.push(value)
    if (kind === 'key') newKey.value = ''
    else newChannel.value = ''
}

const removePattern = (kind, value) => {
    if (kind === 'key') form.keyPatterns = form.keyPatterns.filter((i) => i !== value)
    else form.channelPatterns = form.channelPatterns.filter((i) => i !== value)
}


const buildRules = () => {
    const rules = ['reset', form.enabled ? 'on' : 'off']

    if (form.nopass) {
        rules.push('nopass')
    } else {
        form.hashes.forEach((hash) => rules.push('#' + hash))
        const password = form.newPassword.trim()
        if (password !== '') rules.push('>' + password)
    }

    if (form.commandMode === 'all') {
        rules.push('+@all')
    } else {
        rules.push('-@all')
        form.categories.forEach((cat) => rules.push('+@' + cat))
        form.extraRules
            .split(/\s+/)
            .filter(Boolean)
            .forEach((rule) => rules.push(rule))
    }

    if (form.keysAll) rules.push('~*')
    else form.keyPatterns.forEach((p) => rules.push(/^(~|%R~|%W~)/.test(p) ? p : '~' + p))

    if (form.channelsAll) rules.push('&*')
    else form.channelPatterns.forEach((p) => rules.push(p.startsWith('&') ? p : '&' + p))

    return rules
}

const removeHash = (hash) => {
    form.hashes = form.hashes.filter((item) => item !== hash)
}

const saveUser = () => {
    if (!editable.value) return
    const rules = buildRules()
    const content = i18n.t('acl.save_title') + ': ' + i18n.t('acl.save_confirm', { user: selected.value, rules: rules.join(' ') })
    $dialog.warning(content, async () => {
        saving.value = true
        try {
            const { success, data, msg } = await AclSetUser(props.server, selected.value, rules)
            if (success) {
                detail.value = data || detail.value
                fillForm(detail.value)
                baseline.value = snapshot()
                $message.success(i18n.t('acl.save_success'))
                loadOverview(true)
            } else {
                $message.error(msg || i18n.t('acl.save_fail'))
            }
        } finally {
            saving.value = false
        }
    })
}

const resetForm = () => {
    fillForm(detail.value)
    baseline.value = snapshot()
    dryResult.value = null
}

const deleteUser = () => {
    if (!editable.value) return
    const name = selected.value
    const content = i18n.t('acl.delete_title') + ': ' + i18n.t('acl.delete_confirm', { user: name })
    $dialog.warning(content, async () => {
        const { success, msg } = await AclDeleteUser(props.server, name)
        if (success) {
            $message.success(i18n.t('acl.delete_success'))
            selected.value = ''
            detail.value = null
            loadOverview(true)
        } else {
            $message.error(msg || i18n.t('acl.delete_fail'))
        }
    })
}

const createUser = async () => {
    const name = newUserName.value.trim()
    if (name === '') return
    const { success, msg } = await AclSetUser(props.server, name, ['reset', 'off', 'resetpass', '-@all', '~*', '&*'])
    if (success) {
        showCreate.value = false
        newUserName.value = ''
        await loadOverview(true)
        selectUser(name)
        $message.success(i18n.t('acl.create_success'))
    } else {
        $message.error(msg || i18n.t('acl.create_fail'))
    }
}

watch(dirty, (isDirty) => {
    if (isDirty) dryResult.value = null
})

const runDryRun = async () => {
    if (dirty.value) return
    const parts = dryCommand.value.trim().split(/\s+/).filter((i) => i !== '')
    if (parts.length === 0 || !selected.value) return
    dryLoading.value = true
    try {
        const { success, data, msg } = await AclDryRun(props.server, selected.value, parts)
        if (success) {
            dryResult.value = { allowed: data?.allowed === true, message: data?.message || '' }
        } else {
            dryResult.value = { allowed: false, message: msg || '' }
        }
    } finally {
        dryLoading.value = false
    }
}

const logColumns = computed(() => [
    { title: i18n.t('acl.log_reason'), key: 'reason', width: 110 },
    { title: i18n.t('acl.log_context'), key: 'context', width: 110 },
    { title: i18n.t('acl.log_username'), key: 'username', width: 140 },
    { title: i18n.t('acl.log_object'), key: 'object', ellipsis: { tooltip: true } },
    { title: i18n.t('acl.log_age'), key: 'age-seconds', width: 100 },
])

const rulePreview = computed(() => {
    const rules = []
    if (form.commandMode === 'all') {
        rules.push('+@all')
    } else {
        rules.push('-@all')
        form.categories.forEach((cat) => rules.push('+@' + cat))
        form.extraRules
            .split(/\s+/)
            .filter(Boolean)
            .forEach((rule) => rules.push(rule))
    }
    return rules
})

onMounted(() => {
    loadOverview()
    loadCategories()
})

watch(tab, (value) => {
    if (value === 'log') loadLogs()
})

watch(
    () => props.pause,
    (paused) => {
        if (!paused) loadOverview(true)
    }
)
</script>

<template>
    <div class="acl-pane flex-box-v">
        <div class="acl-header flex-box-h">
            <span class="acl-whoami">{{ $t('acl.whoami') }}: <b>{{ whoami || '-' }}</b></span>
            <n-tag size="small" :bordered="false" type="info">{{ $t('acl.user_count', { n: totalUsers }) }}</n-tag>
            <n-tag v-if="nodeCount > 1" size="small" :bordered="false">{{ $t('acl.node_count', { n: nodeCount }) }}</n-tag>
            <div class="flex-item-expand"></div>
            <n-button size="tiny" secondary @click="showCreate = true">
                <template #icon><n-icon :component="Plus" /></template>
                {{ $t('acl.create') }}
            </n-button>
            <n-button size="tiny" secondary :loading="loading" @click="loadOverview()">
                <template #icon><n-icon :component="Refresh" /></template>
                {{ $t('acl.refresh') }}
            </n-button>
        </div>

        <n-tabs v-model:value="tab" class="flex-item-expand acl-tabs" display-directive="show:lazy" size="small" type="line">
            <n-tab-pane :tab="$t('acl.users')" name="users">
                <div class="acl-layout flex-box-h">
                    <aside class="acl-user-list">
                        <div
                            v-for="user in users"
                            :key="user.name"
                            class="acl-user-item"
                            :class="{ active: user.name === selected, disabled: !user.enabled }"
                            @click="selectUser(user.name)">
                            <span class="user-dot" :class="{ on: user.enabled }"></span>
                            <span class="user-name">{{ user.name }}</span>
                            <n-tag
                                v-if="user.totalNodes > 1 && user.nodes < user.totalNodes"
                                size="tiny"
                                type="warning"
                                :bordered="false">
                                {{ user.nodes }}/{{ user.totalNodes }}
                            </n-tag>
                        </div>
                        <n-empty v-if="users.length === 0" :description="$t('acl.empty')" />
                    </aside>

                    <section class="acl-detail flex-item-expand">
                        <template v-if="detail">
                            <div class="detail-head flex-box-h">
                                <div class="user-title">
                                    <span>{{ selected }}</span>
                                    <n-tag v-if="isSelf" size="tiny" :bordered="false" type="info">{{ $t('acl.self') }}</n-tag>
                                    <n-tag size="tiny" :bordered="false" :type="form.enabled ? 'success' : 'default'">
                                        {{ form.enabled ? $t('acl.on') : $t('acl.off') }}
                                    </n-tag>
                                </div>
                                <div class="flex-item-expand"></div>
                                <div v-if="editable" class="head-actions">
                                    <n-button size="small" secondary :disabled="!dirty" @click="resetForm">
                                        {{ $t('acl.reset') }}
                                    </n-button>
                                    <n-button size="small" type="error" secondary @click="deleteUser">
                                        <template #icon><n-icon :component="Delete" /></template>
                                        {{ $t('acl.delete') }}
                                    </n-button>
                                    <n-button size="small" type="primary" :disabled="!dirty" :loading="saving" @click="saveUser">
                                        <template #icon><n-icon :component="Save" /></template>
                                        {{ $t('acl.save') }}
                                    </n-button>
                                </div>
                            </div>

                            <div v-if="isSelf" class="notice">{{ $t('acl.self_readonly') }}</div>
                            <div v-if="isPartial" class="notice">
                                {{ $t('acl.node_partial', { n: detail.nodes, total: detail.totalNodes }) }}
                            </div>

                            <div class="detail-scroll">
                                <!-- 状态 -->
                                <div class="section">
                                    <div class="section-title">{{ $t('acl.section_status') }}</div>
                                    <div class="section-body row">
                                        <n-switch v-model:value="form.enabled" :disabled="!editable" />
                                        <span class="hint">{{ form.enabled ? $t('acl.on') : $t('acl.off') }}</span>
                                    </div>
                                </div>

                                <!-- 密码 -->
                                <div class="section">
                                    <div class="section-title">{{ $t('acl.passwords') }}</div>
                                    <div class="section-body">
                                        <div class="row">
                                            <n-checkbox v-model:checked="form.nopass" :disabled="!editable">
                                                {{ $t('acl.nopass') }}
                                            </n-checkbox>
                                            <span class="hint">{{ $t('acl.password_count', { n: hashCount }) }}</span>
                                        </div>
                                        <template v-if="!form.nopass">
                                            <div v-for="hash in form.hashes" :key="hash" class="list-row">
                                                <span class="mono">#{{ hash.slice(0, 20) }}…</span>
                                                <n-button text size="tiny" type="error" :disabled="!editable" @click="removeHash(hash)">
                                                    {{ $t('acl.remove') }}
                                                </n-button>
                                            </div>
                                            <div class="row">
                                                <n-input
                                                    v-model:value="form.newPassword"
                                                    :disabled="!editable"
                                                    :placeholder="$t('acl.password_new')"
                                                    show-password-on="click"
                                                    size="small"
                                                    type="password" />
                                            </div>
                                            <span class="hint">{{ $t('acl.password_save_hint') }}</span>
                                        </template>
                                    </div>
                                </div>

                                <!-- 命令权限 -->
                                <div class="section">
                                    <div class="section-title">{{ $t('acl.commands') }}</div>
                                    <div class="section-body">
                                        <n-radio-group v-model:value="form.commandMode" :disabled="!editable" size="small">
                                            <n-radio-button value="all">{{ $t('acl.command_all') }}</n-radio-button>
                                            <n-radio-button value="custom">{{ $t('acl.command_custom') }}</n-radio-button>
                                        </n-radio-group>

                                        <template v-if="form.commandMode === 'custom'">
                                            <div class="sub-label">{{ $t('acl.command_categories') }}</div>
                                            <n-checkbox-group v-model:value="form.categories" :disabled="!editable">
                                                <div class="chips">
                                                    <n-checkbox v-for="cat in categoryOptions" :key="cat.value" :value="cat.value">
                                                        {{ cat.label }}
                                                    </n-checkbox>
                                                </div>
                                            </n-checkbox-group>
                                            <div class="sub-label">{{ $t('acl.command_extra') }}</div>
                                            <n-input
                                                v-model:value="form.extraRules"
                                                :disabled="!editable"
                                                placeholder="-flushall +get"
                                                size="small" />
                                        </template>

                                        <div class="chips preview">
                                            <n-tag
                                                v-for="rule in rulePreview"
                                                :key="rule"
                                                size="small"
                                                :bordered="false"
                                                :type="rule.startsWith('-') ? 'error' : 'success'">
                                                {{ rule }}
                                            </n-tag>
                                        </div>
                                    </div>
                                </div>

                                <!-- Key 范围 -->
                                <div class="section">
                                    <div class="section-title">{{ $t('acl.keys') }}</div>
                                    <div class="section-body">
                                        <n-radio-group v-model:value="form.keysAll" :disabled="!editable" size="small">
                                            <n-radio-button :value="true">{{ $t('acl.keys_all') }}</n-radio-button>
                                            <n-radio-button :value="false">{{ $t('acl.scope_custom') }}</n-radio-button>
                                        </n-radio-group>
                                        <template v-if="!form.keysAll">
                                            <div class="row" style="margin-top: 8px">
                                                <n-input
                                                    v-model:value="newKey"
                                                    :disabled="!editable"
                                                    :placeholder="$t('acl.keys_placeholder')"
                                                    size="small"
                                                    @keyup.enter="addPattern('key')" />
                                                <n-button size="small" :disabled="!editable" @click="addPattern('key')">
                                                    {{ $t('acl.add') }}
                                                </n-button>
                                            </div>
                                            <div class="chips">
                                                <n-tag v-for="p in form.keyPatterns" :key="p" size="small" closable :bordered="false"
                                                    @close="removePattern('key', p)">
                                                    {{ p }}
                                                </n-tag>
                                            </div>
                                        </template>
                                    </div>
                                </div>

                                <!-- Channel 范围 -->
                                <div class="section">
                                    <div class="section-title">{{ $t('acl.channels') }}</div>
                                    <div class="section-body">
                                        <n-radio-group v-model:value="form.channelsAll" :disabled="!editable" size="small">
                                            <n-radio-button :value="true">{{ $t('acl.channels_all') }}</n-radio-button>
                                            <n-radio-button :value="false">{{ $t('acl.scope_custom') }}</n-radio-button>
                                        </n-radio-group>
                                        <template v-if="!form.channelsAll">
                                            <div class="row" style="margin-top: 8px">
                                                <n-input
                                                    v-model:value="newChannel"
                                                    :disabled="!editable"
                                                    :placeholder="$t('acl.channels_placeholder')"
                                                    size="small"
                                                    @keyup.enter="addPattern('channel')" />
                                                <n-button size="small" :disabled="!editable" @click="addPattern('channel')">
                                                    {{ $t('acl.add') }}
                                                </n-button>
                                            </div>
                                            <div class="chips">
                                                <n-tag v-for="p in form.channelPatterns" :key="p" size="small" closable :bordered="false"
                                                    @close="removePattern('channel', p)">
                                                    {{ p }}
                                                </n-tag>
                                            </div>
                                        </template>
                                    </div>
                                </div>

                                <!-- 权限测试 -->
                                <div class="section">
                                    <div class="section-title">{{ $t('acl.dryrun') }}</div>
                                    <div class="section-body">
                                        <div class="row">
                                            <n-input
                                                v-model:value="dryCommand"
                                                :placeholder="$t('acl.dryrun_placeholder')"
                                                size="small"
                                                @keyup.enter="runDryRun" />
                                            <n-button size="small" type="primary" :disabled="dirty" :loading="dryLoading" @click="runDryRun">
                                                <template #icon><n-icon :component="Play" /></template>
                                                {{ $t('acl.run') }}
                                            </n-button>
                                        </div>
                                        <span v-if="dirty" class="hint">{{ $t('acl.test_dirty_hint') }}</span>
                                        <div v-if="dryResult" class="dryrun-result" :class="dryResult.allowed ? 'ok' : 'denied'">
                                            {{ dryResult.allowed ? $t('acl.dryrun_allowed') : $t('acl.dryrun_denied') }}
                                            <span v-if="!dryResult.allowed"> — {{ dryResult.message }}</span>
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </template>

                        <n-empty v-else :description="$t('acl.select_user')" />
                    </section>
                </div>
            </n-tab-pane>

            <n-tab-pane :tab="$t('acl.log')" name="log">
                <div class="acl-log">
                    <n-data-table
                        :columns="logColumns"
                        :data="logs"
                        :loading="logLoading"
                        :bordered="false"
                        :max-height="440"
                        size="small">
                        <template #empty>
                            <n-empty :description="$t('acl.log_empty')" />
                        </template>
                    </n-data-table>
                </div>
            </n-tab-pane>
        </n-tabs>

        <n-modal v-model:show="showCreate" preset="dialog" :title="$t('acl.create')">
            <n-input v-model:value="newUserName" :placeholder="$t('acl.create_placeholder')" />
            <template #action>
                <n-button size="small" @click="showCreate = false">{{ $t('acl.cancel') }}</n-button>
                <n-button size="small" type="primary" :disabled="newUserName.trim() === ''" @click="createUser">
                    {{ $t('common.confirm') }}
                </n-button>
            </template>
        </n-modal>
    </div>
</template>

<style lang="scss" scoped>
.acl-pane {
    padding: 12px;
    height: 100%;
    box-sizing: border-box;

    .acl-header {
        gap: 8px;
        align-items: center;
        padding-bottom: 10px;
        border-bottom: 1px solid v-bind('themeVars.borderColor');

        .acl-whoami {
            font-size: 12px;

            b {
                font-family: monospace;
            }
        }
    }

    .acl-tabs {
        margin-top: 4px;
    }

    .acl-layout {
        height: 100%;
        min-height: 500px;
        gap: 0;
        align-items: stretch;
    }

    .acl-user-list {
        width: 220px;
        min-width: 200px;
        border-right: 1px solid v-bind('themeVars.borderColor');
        padding: 4px 8px 4px 0;
        overflow: auto;

        .acl-user-item {
            display: flex;
            align-items: center;
            gap: 8px;
            padding: 8px 10px;
            border-radius: 6px;
            cursor: pointer;
            font-size: 13px;

            &:hover {
                background: v-bind('themeVars.actionColor');
            }

            // 与左侧键树选中样式保持一致：主色浅底 + 常规文字色
            &.active {
                background: rgba(211, 58, 49, 0.12);
                color: v-bind('themeVars.textColor1');
            }

            .user-dot {
                width: 7px;
                height: 7px;
                border-radius: 50%;
                background: #c8c8c8;
                flex: none;

                &.on {
                    background: #18a058;
                }
            }

            .user-name {
                flex: 1;
                overflow: hidden;
                text-overflow: ellipsis;
            }
        }
    }

    .acl-detail {
        display: flex;
        flex-direction: column;
        padding-left: 16px;
        min-width: 0;

        .detail-head {
            align-items: center;
            padding-bottom: 10px;
            border-bottom: 1px solid v-bind('themeVars.borderColor');

            .user-title {
                display: flex;
                align-items: center;
                gap: 8px;
                font-size: 16px;
                font-weight: 600;
            }

            .head-actions {
                display: flex;
                gap: 8px;
            }
        }

        .notice {
            margin-top: 10px;
            font-size: 12px;
            padding: 6px 10px;
            border-radius: 6px;
            background: rgba(217, 161, 59, 0.12);
            color: #8a5f00;
        }

        .detail-scroll {
            flex: 1;
            overflow: auto;
            padding-top: 6px;
        }

        .section {
            padding: 12px 0;
            border-bottom: 1px dashed v-bind('themeVars.borderColor');

            &:last-child {
                border-bottom: none;
            }

            .section-title {
                font-size: 13px;
                font-weight: 600;
                margin-bottom: 8px;
            }

            .section-body {
                font-size: 13px;
            }

            .row {
                display: flex;
                align-items: center;
                gap: 10px;

                .n-input {
                    flex: 1;
                }
            }

            .hint {
                font-size: 12px;
                color: v-bind('themeVars.textColor3');
            }

            .sub-label {
                margin: 10px 0 6px;
                font-size: 12px;
                color: v-bind('themeVars.textColor3');
            }

            .chips {
                display: flex;
                flex-wrap: wrap;
                gap: 8px 12px;
                margin-top: 8px;
            }

            .preview {
                margin-top: 12px;
                gap: 6px;
            }

            .list-row {
                display: flex;
                align-items: center;
                justify-content: space-between;
                padding: 4px 0;

                .mono {
                    font-family: monospace;
                    font-size: 12px;
                }
            }

            .dryrun-result {
                margin-top: 8px;
                font-size: 12px;

                &.ok {
                    color: #18a058;
                }

                &.denied {
                    color: #d03050;
                }
            }
        }
    }

    .acl-log {
        padding-top: 6px;
    }
}
</style>
