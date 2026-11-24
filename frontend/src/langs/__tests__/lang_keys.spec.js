import { describe, expect, it } from 'vitest'
import en from '@/langs/en-us.json'
import zhCn from '@/langs/zh-cn.json'
import zhTw from '@/langs/zh-tw.json'
import ja from '@/langs/ja-jp.json'
import ko from '@/langs/ko-kr.json'
import ru from '@/langs/ru-ru.json'
import es from '@/langs/es-es.json'
import fr from '@/langs/fr-fr.json'
import pt from '@/langs/pt-br.json'
import tr from '@/langs/tr-tr.json'

const messages = {
    'en-us': en,
    'zh-cn': zhCn,
    'zh-tw': zhTw,
    'ja-jp': ja,
    'ko-kr': ko,
    'ru-ru': ru,
    'es-es': es,
    'fr-fr': fr,
    'pt-br': pt,
    'tr-tr': tr,
}

const flatten = (value, prefix = '') => {
    const result = {}
    for (const [key, item] of Object.entries(value)) {
        const path = prefix ? `${prefix}.${key}` : key
        if (item !== null && typeof item === 'object') {
            Object.assign(result, flatten(item, path))
        } else {
            result[path] = item
        }
    }
    return result
}

const placeholders = (value) => (String(value).match(/\{[a-zA-Z_]+\}/g) || []).sort().join(',')

describe('language files', () => {
    const english = flatten(en)

    // 每种语言的键必须与英文完全对齐，避免新功能只翻译部分语言
    it.each(Object.keys(messages))('%s 与英文键一致', (lang) => {
        const target = flatten(messages[lang])
        const missing = Object.keys(english).filter((key) => !(key in target))
        const extra = Object.keys(target).filter((key) => !(key in english))
        expect(missing).toEqual([])
        expect(extra).toEqual([])
    })

    // 插值占位符必须与英文一致，否则运行时会显示错误内容
    it.each(Object.keys(messages))('%s 占位符与英文一致', (lang) => {
        const target = flatten(messages[lang])
        const mismatched = Object.keys(english).filter(
            (key) => key in target && placeholders(english[key]) !== placeholders(target[key])
        )
        expect(mismatched).toEqual([])
    })
})
