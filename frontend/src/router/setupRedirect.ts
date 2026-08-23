import { resolveLoginHref } from './loginEntry'

export function resolveCompletedSetupRedirectPath(isAuthenticated: boolean, isAdmin: boolean): string {
  if (!isAuthenticated) {
    // 登录入口隐藏且本标签页不知道入口时，resolveLoginHref 会回落到默认首页。
    return resolveLoginHref()
  }

  return isAdmin ? '/admin/dashboard' : '/dashboard'
}
