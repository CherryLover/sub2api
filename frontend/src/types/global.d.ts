import type { PublicSettings } from '@/types'

declare global {
  interface Window {
    __APP_CONFIG__?: PublicSettings
    /**
     * 隐藏登录入口标记，只由后端在服务 index.html 时注入，且只有请求路径正好命中
     * 自定义登录路径的那一次才是 1；其它页面都是 0，登录入口公开时该字段不存在。
     * 路径本身永远不会出现在这里——前端只能从当前 URL 得知它。
     */
    __LOGIN_ENTRY__?: number
  }
}

export {}
