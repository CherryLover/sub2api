/**
 * 管理员访问用户端页面时的管理端对应页。
 *
 * 只收录对管理员来说纯属重复入口的两页：用户端仪表盘和用量页在管理端各有
 * 独立版本（/admin/dashboard、/admin/usage），侧栏"我的账户"区也不再列出它们。
 * /keys 不在此列——管理员自己的密钥仍走用户端页面。
 */
const ADMIN_EQUIVALENT_PATHS: Readonly<Record<string, string>> = {
  '/dashboard': '/admin/dashboard',
  '/usage': '/admin/usage',
}

/** 返回该路径的管理端对应页；不需要重定向时返回 null。 */
export function resolveAdminEquivalentPath(path: string): string | null {
  return ADMIN_EQUIVALENT_PATHS[path] ?? null
}
