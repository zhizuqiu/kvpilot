import vue from '@vitejs/plugin-vue'
import vueJsx from '@vitejs/plugin-vue-jsx'
import AutoImport from 'unplugin-auto-import/vite'
import Icons from 'unplugin-icons/vite'
import { NaiveUiResolver } from 'unplugin-vue-components/resolvers'
import Components from 'unplugin-vue-components/vite'
import { defineConfig } from 'vite'

const rootPath = new URL('.', import.meta.url).pathname
const isWeb = process.env.VITE_WEB === 'true'
const webBasePath = process.env.VITE_BASE_PATH || './'

// https://vitejs.dev/config/
export default defineConfig({
    base: isWeb ? webBasePath : '/',
    plugins: [
        vue(),
        // 允许在测试与组件中使用 JSX/TSX
        vueJsx(),
        AutoImport({
            imports: [
                {
                    'naive-ui': ['useDialog', 'useMessage', 'useNotification', 'useLoadingBar'],
                },
            ],
        }),
        Components({
            resolvers: [NaiveUiResolver()],
        }),
        Icons(),
    ],
    resolve: {
        alias: {
            '@': rootPath + 'src',
            stores: rootPath + 'src/stores',
            // Web mode: redirect wailsjs imports to HTTP/WebSocket adapters
            // Desktop mode (wails build): use real Wails RPC bindings
            ...(isWeb
                ? {
                      'wailsjs/runtime/runtime.js': rootPath + 'src/utils/wails_runtime.js',
                      'wailsjs/go/services/connectionService.js': rootPath + 'src/utils/api.js',
                      'wailsjs/go/services/browserService.js': rootPath + 'src/utils/api.js',
                      'wailsjs/go/services/cliService.js': rootPath + 'src/utils/api.js',
                      'wailsjs/go/services/monitorService.js': rootPath + 'src/utils/api.js',
                      'wailsjs/go/services/pubsubService.js': rootPath + 'src/utils/api.js',
                      'wailsjs/go/services/preferencesService.js': rootPath + 'src/utils/api.js',
                      'wailsjs/go/services/systemService.js': rootPath + 'src/utils/api.js',
                  }
                : {}),
            wailsjs: rootPath + 'wailsjs',
        },
    },
    css: {
        preprocessorOptions: {
            scss: {
                api: 'modern-compiler',
            },
        },
    },
    ...(isWeb
        ? {
              server: {
                  proxy: {
                      '/api': {
                          target: 'http://localhost:8088',
                          changeOrigin: true,
                      },
                      '/ws': {
                          target: 'ws://localhost:8088',
                          ws: true,
                      },
                  },
              },
          }
        : {}),
    // 单元测试配置：纯函数与组件行为测试统一放在各目录的 __tests__/ 下
    test: {
        environment: 'jsdom',
        include: ['src/**/__tests__/*.spec.{js,jsx}'],
    },
})
