/**
 * 本分支已移除匿名使用统计：不再加载任何第三方统计脚本。
 * 保留同名导出，避免调用点改动；如需接入自有统计，在此实现即可。
 */

/**
 * 统计模块加载（空实现）
 * @param {boolean} _allowTrack
 * @return {Promise<void>}
 */
export const loadModule = async (_allowTrack = false) => {}

/**
 * 事件上报（空实现）
 * @param {string} _event
 * @param {object} _data
 * @return {Promise<void>}
 */
export const trackEvent = async (_event, _data) => {}
