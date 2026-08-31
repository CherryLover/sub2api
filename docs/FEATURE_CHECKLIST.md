# 功能验收清单

> **这份文档是什么**
>
> 大规模裁剪（批次 1–5，净删约 20 万行）之后，**这个系统现在还能做什么**的完整清单。
> 它同时是一份**回归验收清单**：每次改完代码、每次发版之后，照着这张表逐条勾一遍，
> 就能确认没有把还活着的功能碰坏。
>
> **它不是凭印象写的。** 每一条都是从下面四张表里**导出**并双向核对过的：
>
> | 导出来源 | 文件 |
> |---|---|
> | 前端路由表（26 条静态路由，含 3 条重定向；另有 1 条隐藏登录入口是运行时注册的） | `frontend/src/router/index.ts` |
> | 后端路由注册 | `backend/internal/server/routes/{auth,user,admin,gateway,key_usage,common}.go` |
> | 后台侧边栏菜单 | `frontend/src/components/layout/AppSidebar.vue`（第 530–610 行） |
> | 前端 API 封装（约 300 个调用点） | `frontend/src/api/`、`frontend/src/features/*/api.ts` |
>
> 核对方式是双向的：路由要能找到页面文件、页面要能找到后端处理器、后端接口要能找到前端调用方。
> **三个方向对不上的，全部收进了「发现的残留」一节**，没有塞进功能表里充数。

## 怎么用这份清单

- **第一列 `[ ]` 就是勾选框。** 实测通过就改成 `[x]`，测不过就在该行的「风险」列里补一句现象。
- **优先级看「风险」列**：标「高」的是本轮删改的重灾区，幸存下来的主链路，必须实测；标「无」的是本轮零改动，可以只做冒烟。
- 赶时间的时候，直接跳到最后的 **[本轮裁剪的高风险面](#本轮裁剪的高风险面)**，那里把该重点盯的条目集中列好了，共 14 条。
- 「入口」列里写的是**用户从哪儿点进去**，下面跟着对应的后端接口和关键文件，排查时可以直接照着定位。

## 已经不存在的功能（别再找了）

本轮整体删除，代码、路由、页面、数据表都已移除，**不在本清单的验收范围内**：

注册体系 · 人机验证 · 支付 · 兑换码 · 促销分销 · 第三方登录 · 邮件体系 ·
批量生图 · 内容安全审计 · 渠道监控 V1 主动探测 · 公告 · 模型广场 · 订阅体系 · 用户余额

其中两条的**语义变化**最容易踩坑，请先读一遍：

1. **额度现在直接绑在 API Key 上。** Key 的 `quota` 填 0 或不填 = **无限额，永远不会被拦**。
   过去那种「用户余额扣光就自动停」的兜底已经拆掉了。升级前建议先查一遍库里 `quota=0` 的 Key 有多少把。
2. **专属分组只认授权记录。** 判定唯一依据是 `user_allowed_groups` 里那行授权，没有就是 403，没有任何旁路。
   历史上「靠订阅拿到分组、但授权表里从没写过行」的用户，升级后 Key 会当场 403。上线前建议在库里对一遍。

---

## 1. 认证与用户

> 本轮重灾区。`auth_handler.go` 减 400 行、`user_handler.go` 减 440 行、`api/auth.ts` 减 514 行、
> `router/index.ts` 减 630 行，`views/auth/` 直接删掉 11 个页面组件。登录主链路是在这堆删除中**幸存**下来的，不是没动过。

| ✓ | 功能 | 入口 / 接口 / 文件 | 验证方法 | 风险 |
|---|---|---|---|---|
| [ ] | 邮箱 + 密码登录 | `/login`（隐藏入口开启后为自定义路径）<br>`POST /api/v1/auth/login` → `GET /api/v1/auth/me`<br>`views/auth/LoginView.vue`、`api/auth.ts`、`stores/auth.ts`、`handler/auth_handler.go:124` | 管理员登录 → 跳 `/admin/dashboard`；普通用户 → 跳 `/dashboard`。故意输错一次密码 → 弹「登录失败」且不跳转。开发者工具 Application → Local Storage 里确认 `auth_token`、`refresh_token`、`token_expires_at`、`auth_user` 四个值都写进去了 | **高** |
| [ ] | 登录两步验证（TOTP） | `/login` 密码通过后自动弹 6 位验证码框<br>`POST /auth/login` 返回 `requires_2fa`+`temp_token` → `POST /auth/login/2fa`<br>`components/auth/TotpLoginModal.vue`、`handler/auth_handler.go:180` | 先确认 设置 → 安全与认证 → 2FA 开关已开且账号已绑。重新登录 → 应弹验证码框并显示打码邮箱（`ad***@example.com`）。先输错码 → 框内报错不关闭；再输正确码 → 进面板 | 中 |
| [ ] | Passkey 免密登录 | `/login` 密码框下方「使用 Passkey 登录」<br>`POST /auth/passkey/login/begin` → `/finish`<br>`api/passkey.ts`、`LoginView.vue:97-113`、`handler/passkey_handler.go` | 设置 → 安全与认证 → 打开 Passkey，确认 RPID/Origins 与实际域名一致。退出后 `/login` 应出现按钮，点击 → 弹指纹/面容/PIN → 直接进面板。关掉开关刷新，按钮应消失 | 中 |
| [ ] | 退出登录 | 右上角头像 → 退出登录<br>`POST /auth/logout`（带 refresh_token）<br>`components/layout/AppHeader.vue:94,189`、`handler/auth_handler.go:341` | 退出 → 跳回登录页。Local Storage 里 `auth_token`/`refresh_token`/`auth_user` 应全部清掉。地址栏直接敲 `/dashboard` → 应被拦回登录页而不是白屏 | 低 |
| [ ] | 登录态自动续期 | 无界面：过期前 2 分钟自动续，或任意请求 401 时补救一次<br>`POST /auth/refresh`<br>`api/tokenRefresh.ts`、`stores/auth.ts`（scheduleTokenRefresh） | 把 Local Storage 的 `token_expires_at` 手工改成「一分钟后」，在面板里点几下 → Network 出现一次 `POST /auth/refresh` 返回 200，`auth_token` 变新值，页面不掉线。开两个标签页同样操作，两边不应互相挤下线（代码用 Web Lock 做了跨标签互斥） | 中 |
| [ ] | 会话绑定 IP 与浏览器指纹 | 设置 → 安全与认证 → 「会话 IP/UA 绑定」<br>设置项 `session_binding_enabled`<br>`middleware/session_binding.go`、`SettingsView.vue:1583-1590` | 打开并保存。复制 `auth_token`，从另一台机器（不同出口 IP）带同一 token 调 `GET /auth/me` → 应返回 401；本机同浏览器一切正常。注意用手机流量/公司网络切换的用户会被频繁踢下线 | 低 |
| [ ] | 登录接口限流（防撞库） | 无界面，服务端强制：登录/2FA/Passkey 各 20 次每分钟，刷新 30 次每分钟；**Redis 挂掉直接拒绝**（fail-close）<br>`routes/auth.go:33-52`、`middleware/rate_limiter.go` | curl 对 `POST /auth/login` 连发 25 次错误密码（同 IP 一分钟内）→ 前 20 次 401，之后 429。等一分钟恢复正常 | 低 |
| [ ] | 隐藏登录入口 + 默认首页 | 设置 → 安全与认证 → 「登录入口」卡片<br>字段 `login_entry_public`/`login_entry_path`/`default_home_path`<br>`router/loginEntry.ts`、`handler/admin/setting_handler_web_entry.go`、`web/embed_on.go:263` | 打开隐藏入口 → 生成随机路径（如 `/a8f3k2`）→ 保存 → **先把完整 URL 存进密码管理器**。①无痕窗口访问 `/login` 应被重定向到默认首页；②访问 `/a8f3k2` 应正常渲染登录页；③查看网页源代码搜 `a8f3k2`，JS 产物里应搜不到；④`GET /api/v1/settings/public` 返回里应没有 `login_entry_path` 字段。再把默认首页改成 `/home`，访问根路径 `/` 应落到 `/home` | 中 |
| [ ] | 后台模式（只让管理员用面板） | 设置 → 通用设置 → 「后台模式」<br>设置项 `backend_mode_enabled`<br>`middleware/backend_mode_guard.go`、`SettingsView.vue:4005` | 打开后普通用户登录 → 密码正确但被告知无权使用面板，`/dashboard`、`/keys` 都被挡回；管理员照常。确认未登录时 `/key-usage` 仍打得开（后台模式下唯一放行的公开页）。**建议和「隐藏登录入口」组合各试一遍**，两个开关的落点逻辑纠缠在一起（防重定向死循环） | 中 |
| [ ] | 查看和修改个人资料 | `/profile`；普通用户在主菜单末项，管理员在「我的账户」分组<br>`GET /user/profile`、`PUT /user`<br>`views/user/ProfileView.vue`、`handler/user_handler.go:85,137` | 页面应完整渲染四张卡片：基本信息、修改密码、双因素认证、Passkey。改用户名保存 → 提示成功，刷新后新名字还在，右上角头像旁的名字也跟着变。基本信息里邮箱**现在是只读的**（写着「登录邮箱由管理员维护」），这是正常的 | **高** |
| [ ] | 修改自己的密码 | `/profile` → 修改密码卡片<br>`PUT /user/password`<br>`components/user/profile/ProfilePasswordForm.vue`、`handler/user_handler.go:109` | 先故意填错当前密码 → 提示旧密码不正确且不生效；再填对并设新密码 → 提示成功。退出后用新密码能登、旧密码不能登。注意改密码会让已签发令牌失效（令牌版本由密码哈希推导），改完页面应把你踢到登录页而不是一直报错 | 中 |
| [ ] | 自己开关两步验证 | `/profile` → 双因素认证卡片<br>`GET /user/totp/status`、`POST /user/totp/setup`→`/enable`、`/disable`<br>`ProfileTotpCard.vue`、`TotpSetupModal.vue`、`TotpDisableDialog.vue` | 先确认服务端配了 `TOTP_ENCRYPTION_KEY`（没配则开关是灰的）。点开启 → 弹窗要求输**当前账号密码** → 显示二维码 → 认证器扫码 → 填 6 位码 → 状态变「已开启」。再关闭 → 同样要密码 → 状态变回「未开启」。**全程不该再出现任何「发送邮箱验证码」入口** | **高** |
| [ ] | 管理自己的 Passkey | `/profile` → Passkey 卡片（后台开关关闭时整张卡片不显示）<br>`GET/POST/PATCH/DELETE /user/passkeys*`<br>`ProfilePasskeyCard.vue`、`handler/passkey_handler.go` | 添加 → 输名字和当前密码 → 系统弹指纹/面容/PIN → 列表出现凭据。改名一次，刷新确认还在。退出登录用「Passkey 登录」验证这条凭据真能登进来。回来删掉（删除同样要密码），列表清空 | 中 |
| [ ] | 敏感操作二次确认（sudo 窗口） | 设置 → 安全与认证 → 「敏感操作二次验证」<br>`POST /user/totp/step-up` 换时间窗；保护 `GET /admin/accounts/data`、`/admin/proxies/data`、`POST /admin/backup`、`/backup/:id/download-url`、`/backup/:id/restore`、S3 配置<br>`middleware/step_up.go`、`composables/useStepUp.ts`、`routes/admin.go:348,442,506-544` | 给管理员绑好 2FA 并打开开关。账号管理 → 导出数据 → 应弹 6 位验证码框，输对后导出开始。**紧接着几分钟内**去 设置 → 数据备份 → 创建备份 → 这次不该再弹框（复用同一时间窗）。再用一个没绑 2FA 的管理员做同样操作 → 应明确提示「请先启用 2FA」而不是静默失败 | 中 |
| [ ] | 清空审计日志的现场验证码 | `/admin/audit-logs` → 右上「清空全部」<br>`POST /admin/audit-logs/clear`（请求体必须带当次 6 位 TOTP）<br>`AuditLogView.vue:78,316-347`、`handler/admin/audit_log_handler.go:115` | 点清空 → 先二次确认，再弹验证码框。**这里和上一条不同：即使刚做过 step-up，也必须重新输当前验证码**（故意不复用时间窗）。先输错码应被拒，正确码后日志清空。另外用管理员 API Key（而非浏览器登录）调这个接口应直接被拒 | 无 |
| [ ] | 管理员新建用户 | `/admin/users` → 右上「新建用户」<br>`POST /admin/users`<br>`components/admin/user/UserCreateModal.vue`、`handler/admin/user_handler.go:267` | 填邮箱、密码、用户名、并发数、RPM 上限、可用分组，角色选普通用户 → 列表出现。**退出登录用这个新账号真登一次**，能进 `/dashboard`。再建一个选「管理员」→ 二次验证开着时应额外弹验证码框。注册体系删除后，这里是唯一的开户途径 | 中 |
| [ ] | 管理员编辑用户 | `/admin/users` → 行内编辑；可用分组另有独立弹窗<br>`PUT /admin/users/:id`<br>`UserEditModal.vue`、`UserAllowedGroupsModal.vue`、`handler/admin/user_handler.go:320` | 改用户名和备注 → 保存后列表显示新值。重设密码 → 新密码能登、旧密码失败。把角色从普通用户改成**管理员** → 二次验证开着时应弹验证码框（提权保护）；编辑一个本来就是管理员的人时**不应**被打断。用「可用分组」弹窗勾掉一个分组 → 他建 Key 时该分组应不可选 | 中 |
| [ ] | 禁用 / 启用用户 | `/admin/users` → 行内状态开关<br>`PUT /admin/users/:id`（只带 status）<br>`UsersView.vue:1616`、`api/admin/users.ts` | 先让该用户在另一浏览器保持登录。禁用 → 状态变「已禁用」，他那边点任意页面应被踢出或提示无权限，重新登录也登不进。再启用回来 → 能正常登录。**顺便确认他名下的 API Key 在禁用期间调网关也是被拒的**（两条路都要生效） | 中 |
| [ ] | 删除用户 | `/admin/users` → 行内删除（二次确认）<br>`DELETE /admin/users/:id`<br>`UsersView.vue:1668`、`handler/admin/user_handler.go` | 拿一个刚建的测试用户（先给他建 Key、跑一两次请求产生用量），删掉 → 列表消失。去 使用记录 搜他的用量，**历史记录应该还在**（软删除，不该连带删账目）。再用他那把 Key 调网关 → 应被拒绝。这条牵扯 Key/用量/分组多张表，而本轮删了大量关联表，**必须真删一个测试用户试试** | 中 |
| [ ] | 批量调整并发数 / RPM | `/admin/users` → 勾选多行 → 批量编辑<br>`POST /admin/users/batch-limits`<br>`BulkEditUserModal.vue` | 勾 3 个用户 → 并发设 5、RPM 设 60 → 提示「已更新 3 个用户」→ 列表对应列全变。再单独打开其中一个的编辑弹窗确认值确实落库（不只是前端显示） | 无 |
| [ ] | 单用户平台额度（日/周/月） | `/admin/users` → 行内「平台额度」；默认值在 设置 → 用户默认值<br>`GET/PUT /admin/users/:id/platform-quotas`、`POST .../reset`、用户侧 `GET /user/platform-quotas`<br>`UserPlatformQuotaModal.vue`、`handler/user_handler.go:39` | 给某平台设一个很小的日额度（如 1）保存。让该用户跑请求把额度用掉 → 再请求应被拒并提示额度用尽。点「重置窗口」→ 立即恢复可用。**注意这是按平台+时间窗算的，和「额度绑在 API Key 上」是两套东西，别混淆** | 中 |
| [ ] | 客户端真实 IP 来源配置 | 设置 → 安全与认证 → 「API Key IP 访问控制」卡片<br>字段 `api_key_acl_trust_forwarded_ip`、`forwarded_client_ip_headers`<br>`SettingsView.vue:1795-1880`、`service/setting_service.go:168`、`pkg/ip/ip.go` | 这个配置决定三件事用哪个 IP：Key 的 IP 名单、审计日志的来访 IP、会话 IP 绑定。把自己 Key 的 IP 白名单设成当前公网 IP → 调网关应通过；改成无关 IP → 应被拒。再去审计日志确认最近几条的「来访 IP」是真实公网 IP **而不是反代内网地址（如 172.x）**；如果是内网地址就是这里配错了 | 低 |
| [ ] | 密码自救工具 adminpass | 无网页入口，服务器上执行：<br>`docker exec -it <容器名> /app/adminpass -email admin@example.com`<br>`backend/cmd/adminpass/main.go` | 按提示输新密码（生产建议用 `-stdin` 或 `ADMINPASS_NEW_PASSWORD` 环境变量传，避免进命令行历史）。回浏览器用新密码能登进面板。**「找回密码」随邮件体系整体删除，这是唯一的自救通道**；上一次发版曾漏打进镜像（已修），所以每次发版后都值得确认 `/app/adminpass` 这个文件确实存在 | **高** |
| [ ] | 首次安装向导 | `/setup`，仅在服务未初始化时可达<br>`GET /setup/status`、`POST /setup/test-db`、`/setup/test-redis`、`/setup/install`（**不在 /api/v1 下**）<br>`views/setup/SetupWizardView.vue`、`routes/common.go:21-27` | 已装好的线上环境访问 `/setup` → 应被自动重定向到后台首页，不该看到安装表单。真正验证向导需要一套全新空环境：依次填数据库、Redis、管理员邮箱密码，每步「测试连接」都返回成功，装完服务重启后能用新管理员账号登录 | 低 |

---

## 2. API Key 与分组

> 额度语义本轮变过：**Key 的 `quota` 填 0 = 无限额**，用户余额兜底已拆除。
> 专属分组判定也变了：**只认 `user_allowed_groups` 里的授权行**，订阅旁路已拆除。
> 这两处是升级后最可能被投诉的地方，下表里标了「高关注」。

| ✓ | 功能 | 入口 / 接口 / 文件 | 验证方法 | 风险 |
|---|---|---|---|---|
| [ ] | API Key 列表（搜索/筛选/排序/自定义列） | `/keys`；侧边栏「我的账户」→ API 密钥（简易模式下挪到管理员菜单末尾）<br>`GET /api/v1/keys?page&search&status&group_id&sort_by`<br>`views/user/KeysView.vue`、`handler/api_key_handler.go:106` | ①搜索框输 Key 名片段，只剩匹配项；②状态选「已停用」只剩 inactive；③分组下拉选具体分组只剩该组，选「未分组」只剩没分组的；④「列设置」勾掉创建时间，**刷新后仍隐藏**（存在 localStorage 的 `api-key-hidden-columns`）；⑤点表头「创建时间」切换升降序，首行随之变化 | 无 |
| [ ] | 新建 API Key（可自定义 Key 串） | `/keys` → 右上「创建密钥」<br>`POST /api/v1/keys`<br>`KeysView.vue`（handleSubmit）、`service/api_key_service.go` | 只填名字直接提交 → 应报「请选择分组」（分组必填）。选分组后提交 → 列表出现新行，Key 串以站点前缀开头。再建一个勾「自定义密钥」：填 16 位以下 → 报「太短」；填含中文/空格 → 报「非法字符」；填 20 位英数字下划线 → 创建成功且显示的就是你填的那串 | 无 |
| [ ] | 编辑 Key（改名/启停/IP 名单） | `/keys` → 行内「编辑」；行内另有启用/停用快捷按钮<br>`PUT /api/v1/keys/:id`<br>`handler/api_key_handler.go:231`、`service/api_key_service.go:749` | 改名保存 → 列表立即变。打开 IP 限制开关填 `1.2.3.4` 保存 → 该行出现 IP 限制小图标。故意填 `999.999.1.1` → 后端应返回「无效 IP 规则」并列出错的那条。点行内「停用」→ 状态变 inactive，拿这把 Key 调 `/v1/usage` 应被拒 | 无 |
| [ ] | **Key 额度上限（quota，0 = 不限额）** | `/keys` → 新建/编辑弹窗 → 「额度限制」开关<br>`POST/PUT /api/v1/keys/:id` 的 `quota`；判定在 `GET /v1/usage` 与所有 `/v1` 请求<br>`service/api_key.go:85`（IsQuotaExhausted）、`middleware/api_key_auth.go:209` | **必测三种情况**。A) 额度开关关闭（quota=0）→ `curl -H "Authorization: Bearer <key>" https://<站点>/v1/usage` 必须回 `"mode":"unrestricted"`、`planName` 是分组名、`"remaining":-1`，且实际对话能走通，不能出现任何「额度不足」。B) 额度设 0.01 → `/v1/usage` 回 `"mode":"quota_limited"`、`quota.limit=0.01`；跑几次花超 → status 变 `quota_exhausted`，继续请求返回 429 `API_KEY_QUOTA_EXHAUSTED`。C) 把耗尽的 Key 额度改回 0 保存 → 状态应**自动**从 quota_exhausted 回到 active 并立刻可用（`api_key_service.go:823` 自动复活） | **高关注** |
| [ ] | 重置 Key 已用额度 | `/keys` → 编辑弹窗 → 「已用额度」旁「重置」（二次确认）<br>`PUT /api/v1/keys/:id` 带 `reset_quota=true`<br>`service/api_key_service.go:828-834` | 挑一把有消费的 Key 点重置 → 弹窗里已用额度变 0，**列表用量列的累计金额不变**（那是流水统计，跟额度计数是两码事）。若之前是 quota_exhausted，重置后应自动回 active | 无 |
| [ ] | Key 限速（5h/1d/7d 窗口）与重置限速用量 | `/keys` → 弹窗「速率限制」开关；列表「限速」列有「重置用量」按钮（默认隐藏，需在列设置里打开）<br>`rate_limit_5h/1d/7d`、`reset_rate_limit_usage`；读取在 `GET /v1/usage` 的 `rate_limits`<br>`service/api_key_service.go`（GetRateLimitData） | 设 5 小时限额 0.01 保存。列设置打开「限速」列 → 显示 `$0.00 / $0.01` 和倒计时（⟳ 4h 59m）。`GET /v1/usage` 应看到 `rate_limits` 里 `window=5h`、`limit=0.01`、带 `reset_at`。花超 → 请求被拒。点「重置用量」→ 已用回 0 立刻可用。**注意：设了限速但没设总额度的 Key，`/v1/usage` 也会走 quota_limited 模式**（有任一限速就算），这是预期的 | 无 |
| [ ] | Key 有效期 | `/keys` → 弹窗「有效期」开关，7/30/90 天或自定义<br>创建走 `expires_in_days`，编辑走 `expires_at`（传空串 = 清除）<br>`handler/api_key_handler.go:268-283` | 新建选「30 天」→ 过期时间列显示 30 天后。编辑改成昨天保存 → 发请求返回 403 `API_KEY_EXPIRED`，刷新列表状态变 expired。再关掉有效期开关保存（前端发 `expires_at=""`）→ 显示「永不过期」，状态自动从 expired 回到 active | 无 |
| [ ] | 在列表里给 Key 换分组 | `/keys` → 「分组」列徽标（带搜索的选择器）<br>`GET /api/v1/groups/available`、`GET /api/v1/groups/rates` → `PUT /api/v1/keys/:id`<br>`handler/api_key_handler.go:322,344`、`service/api_key_service.go:1009` | 下拉里应只列出「活跃 + 你有权用」的分组：公开分组全在，专属分组只有被授权过的才出现。选一个保存 → 徽标变新分组名，倍率角标跟着变（有专属倍率则显示专属值）。反向验证：让管理员取消你对某专属分组的授权，刷新 `/keys` 该分组应消失；用接口硬传那个 `group_id` → 后端返回 403 `GROUP_NOT_ALLOWED` | 中 |
| [ ] | 删除 API Key | `/keys` → 行内删除（确认文案带 Key 名）<br>`DELETE /api/v1/keys/:id` | 先复制 Key 串再删。删除后列表消失，拿复制的串调 `/v1/usage` → 返回 401（软删，认证查不到） | 无 |
| [ ] | Key 用量展示（今日/累计/额度进度） | `/keys` 列表「用量」列<br>`POST /api/v1/usage/dashboard/api-keys-usage`（批量，挂 Heavy 限流）<br>`KeysView.vue:1481`、`api/usage.ts` | 打开 `/keys`，Network 里应看到**一次**批量请求，请求体是当前页的 key id 数组，**不是每行一个请求**。用某把 Key 跑一次对话再刷新 → 该行「今日」和「累计」都增加；设了额度的还多显示一行「额度: 已用/总额」。翻第二页应再发一次批量请求（只带第二页 id） | 无 |
| [ ] | 复制 Key 串 / 一键导入 CC Switch | `/keys` → Key 列复制图标；行内「使用密钥」和「导入 CC Switch」<br>导入纯前端拼 deeplink，不打后端；教程弹窗读 `GET /api/v1/settings/public`<br>`KeysView.vue:1860-1920` | 点复制 → 图标变对勾并提示已复制，粘贴出来是完整 Key 串。点「导入 CC Switch」：分组平台是 antigravity 时应先弹「Claude Code / Gemini CLI」二选一，选完才跳转；其他平台直接唤起 `ccswitch://`。本机没装 CC Switch 时约 0.1 秒后应弹「未安装」提示而不是卡住 | 低 |
| [ ] | 免登录 Key 用量查询页 | `/key-usage`（无需登录，可设为默认首页）<br>`POST /api/v1/key-usage/session` → `GET /api/v1/key-usage/report`<br>`views/KeyUsageView.vue`、`handler/key_usage_handler.go`、`routes/key_usage.go` | 无痕窗口打开，粘贴有效 Key 查询 → 出现名称、状态、今日/累计请求数与花费。**关键：地址栏出现的是 `?t=<令牌>`，绝对不能是原始 Key**。粘一把不存在的 Key → 提示应是笼统的「Invalid or expired key」，不能出现「此 Key 不存在」这类可被批量试探的措辞。连续快速提交十几次错误 Key → 应开始限流（每 IP 每分钟 10 次）。配置 `key_usage.enabled` 可一键关掉，关掉后接口返回 404 | 中低 |
| [ ] | 免登录页分享链接与额度显示 | `/key-usage?t=<令牌>`<br>`GET /api/v1/key-usage/report?token=…`（令牌路径限流放宽到每 IP 每分钟 60 次）<br>`KeyUsageView.vue:880-1025`、`handler/gateway_handler.go:1602` | 用「设了额度」的 Key 查询后复制带 `?t=` 的链接，换个浏览器打开 → 不用输 Key 直接出结果，有环形进度圈显示总额度和已用。再用「没设额度也没设限速」的 Key 查 → **环形进度圈整块消失**（不限额没有进度可画），明细只剩两行，且不该出现 `-$1.00` 这种负数（后端回 `remaining=-1`，前端遇负数显示「-」） | 低 |
| [ ] | 免登录页排行榜与时间窗口 | `/key-usage` 结果页的窗口切换（今天/近7天/近30天/全部）和「本账号/全站」切换<br>`GET /key-usage/report?window=&metric=`<br>`handler/key_usage_handler.go:106` | 切「近7天」→ 应重新发一次请求，返回体 `window` 字段回显对应值，标题跟着变。切「本账号/全站」→ **这一步不该发请求**（两个作用域一次返回、前端本地切换）。自己那一行应有高亮（`is_self`）。某作用域 `total_keys=0` 且 `self_rank=0` 时应显示「暂不可用」，不能显示成「第 1 名 / 共 1 名」 | 无 |
| [ ] | 分组列表（搜索/筛选/拖拽排序/容量概览） | `/admin/groups`；侧边栏「分组管理」（简易模式下隐藏）<br>`GET /admin/groups`、`/usage-summary`、`/capacity-summary`、`/live-capability`、`PUT /admin/groups/sort-order`<br>`views/admin/GroupsView.vue`、`handler/admin/group_handler.go` | 每行应同时显示账号数（可用/总数/限流中）和今日/昨日/累计消费。拖动改顺序保存 → 提示「排序已更新」，刷新后保持。搜索框输分组名片段应即时过滤。反向验证简易模式：切到简易模式后侧边栏不该有「分组管理」，直接敲 `/admin/groups` 应被重定向到 `/admin/dashboard` | 无 |
| [ ] | 新建分组（含从其他分组复制账号） | `/admin/groups` → 右上「创建分组」<br>`POST /admin/groups`<br>`GroupsView.vue:4668`、`service/admin_group.go:394` | 填名字、平台 anthropic、倍率 1.5，不勾专属 → 列表出现新分组且倍率列显示 1.5。再建一个，在「从分组复制账号」里选一个已绑账号的分组 → 新分组账号数应等于源分组（不是 0）。**平台一旦创建就不能改**（编辑时该字段禁用），建的时候要选对 | 无 |
| [ ] | 编辑分组与分组定价 | `/admin/groups` → 行内「编辑」（默认倍率/高峰倍率/按模型定价/图片视频语音单价/RPM 上限/启停）<br>`PUT /admin/groups/:id`；模型候选来自 `GET /admin/groups/:id/models-list-candidates`<br>`GroupsView.vue`、`groupsImagePricing.ts`、`groupsVideoModelPricing.ts`、`groupsProfitControl.ts` | 默认倍率从 1.0 改成 2.0 保存 → 绑该分组的用户在 `/keys` 看到徽标显示 2.0x。跑一次真实对话，去 `/admin/usage` 查这条的计费金额 → 应是原价两倍。再打开「高峰时段倍率」设成当前时段、倍率 3.0 → 再跑一次按 3.0 算。最后把分组状态改成停用 → 绑它的 Key 立刻被拒，返回 403 `GROUP_DISABLED`（分组改动会主动清认证缓存，不用等过期） | 低 |
| [ ] | 复制分组 | `/admin/groups` → 行内「复制」<br>`POST /admin/groups/:id/duplicate`（带 Idempotency-Key，重试不会多建）<br>`api/admin/groups.ts:187`、`routes/admin.go:287` | 挑一个配置复杂的分组（设过按模型定价、绑了账号）点复制 → 提示「已复制为 xxx」。打开新分组编辑弹窗逐项对：倍率、高峰配置、模型定价、账号绑定都应与源一致（服务端复制，不是前端搬字段）。**测幂等**：点复制瞬间断网再恢复重试，最终应只多出一个分组 | 无 |
| [ ] | 删除分组 | `/admin/groups` → 行内删除（二次确认）<br>`DELETE /admin/groups/:id`（级联清授权行、账号绑定、复合路由，分组软删）<br>`service/admin_group.go`、`repository/group_repo.go:555` | 造场景：建专属分组 → 授权给某用户 → 该用户建 Key 绑上去。删除后检查三处：①列表消失；②该用户 `/keys` 里这把 Key 的分组徽标指向已不存在的分组；③拿这把 Key 发请求 → 403 `GROUP_DELETED`。**这是设计如此**——删分组不会顺手删别人的 Key，但也不会有任何提前告警 | 无 |
| [ ] | **专属分组与用户授权名单** | 分组侧：编辑弹窗「专属」开关；授权侧：`/admin/users` → 「用户分组配置」弹窗<br>`PUT /admin/groups/:id` 的 `is_exclusive`；`PUT /admin/users/:id` 的 `allowed_groups`<br>`UserAllowedGroupsModal.vue`、`service/user.go:68`（CanBindGroup） | **按顺序走完六步**：①建分组勾「专属」；②普通用户 `/keys` 点分组徽标 → 下拉里**不该**出现它；③管理员在「用户分组配置」勾上并保存；④用户刷新 `/keys` → 出现了，绑上去发请求正常；⑤管理员把勾去掉保存；⑥用户**不用重登、不用等缓存过期**，直接拿那把 Key 再发请求 → 必须**立刻** 403 `GROUP_NOT_ALLOWED`。第 6 步是关键（后端在用户更新时主动清了该用户所有 Key 的认证缓存）。Gemini 风格接口（`/v1beta`）走另一套中间件，最好也用 Gemini 客户端复测一次第 6 步 | **高关注** |
| [ ] | 用户在某分组内的专属倍率 | `/admin/groups` → 行内「专属倍率」；另一入口在 `/admin/users` 的「用户分组配置」<br>`GET/PUT/DELETE /admin/groups/:id/rate-multipliers`；用户侧读 `GET /api/v1/groups/rates`<br>`GroupRateMultipliersModal.vue`、`repository/user_group_rate_repo.go` | 分组默认倍率设 2.0，给用户 A 填专属 0.5 保存 → A 的 `/keys` 徽标旁显示 **0.5x**（不是 2.0x）。用 A 的 Key 跑一次对话，`/admin/usage` 里这条按 0.5 倍算。清空 A 的值保存 → 回到 2.0x。**这个弹窗只动倍率列，不会碰同一张表里的 RPM 覆盖值** | 无 |
| [ ] | 用户在某分组内的 RPM 覆盖 | `/admin/groups` → 行内「RPM 覆盖」<br>`PUT/DELETE /admin/groups/:id/rpm-overrides`；读取复用 `GET /admin/groups/:id/rate-multipliers`<br>`GroupRPMOverridesModal.vue`、`routes/admin.go:294` | 给用户 A 设 RPM 覆盖 = 2 保存 → 用 A 的 Key 一分钟内快发 3 次以上，第 3 次起应被限流。清空后回落到分组 `rpm_limit`，分组没设再回落到用户级。**优先级：用户×分组覆盖 > 分组 rpm_limit > 用户 rpm_limit**。读取走 rate-multipliers 接口（后端没有单独的 GET rpm-overrides），这是有意为之 | 无 |
| [ ] | 管理员改某把 Key 的分组（含自动补授权） | `/admin/users` → 行内「API 密钥」弹窗 → 点 Key 的分组徽标<br>`PUT /api/v1/admin/api-keys/:id`（`group_id` 传 0 = 解绑）<br>`UserApiKeysModal.vue:211`、`service/admin_group.go:1030` | 把一把 Key 改绑到该用户**没有**被授权的专属分组 → 应当成功，且界面提示「已自动授予该分组权限」。回到该用户的「用户分组配置」，那个专属分组应已被**自动勾上**（后端同事务补了授权行）。再把这把 Key 解绑（选「无分组」）→ 发请求应 403「未分配分组不能使用」，除非设置里开了「允许未分组 Key 调度」。**这是「专属分组严格按授权名单」的唯一合法例外** | 中 |
| [ ] | 替换用户的专属分组（迁移全部 Key） | `/admin/users` → 「用户分组配置」→ 「替换分组」<br>`POST /admin/users/:id/replace-group`<br>`GroupReplaceModal.vue`、`service/admin_group.go:1138` | 让用户 A 被授权分组 X 且有 3 把 Key 绑 X。把 X 换成专属分组 Y → 提示「已迁移 3 把密钥」。核对三件事：A 的授权名单里 Y 在、X 已移除；三把 Key 徽标都变 Y；A 的 Key **立刻**能用（事务提交后清了认证缓存）。边界：目标分组不是「专属」应返回 `GROUP_NOT_EXCLUSIVE`，新旧填成同一个应返回 `SAME_GROUP`。这是三步事务，任何一步没成整体回滚 | 中 |
| [ ] | 用户侧「可用渠道」页 | `/available-channels`；侧边栏「我的账户」→ 可用渠道（**默认关闭**，需在 `/admin/settings` 功能开关里打开）<br>`GET /api/v1/channels/available`（受 `available_channels_enabled` 控制）+ `GET /api/v1/groups/rates`<br>`views/user/AvailableChannelsView.vue`、`handler/available_channel_handler.go` | 先确认开关关闭时侧边栏没有这项、直接敲 `/available-channels` 是空表。打开开关后核对：①只列出「启用 + 与你有权访问的分组有交集」的渠道，别人的专属渠道不能出现；②按平台分块，每块下是你能用的分组和模型价格；③设过专属倍率的显示专属值；④搜索模型名只剩包含该模型的渠道块。管理员也从「我的账户」进，没有单独后台入口 | 低 |

---

## 3. 上游账号与转发

> 账号侧代码本轮基本没动（`AccountsView.vue` 与裁剪起点 `3548256` 相比零差异）。
> 但**渠道监控 V1 的主动探测已整体删除**：账号挂了不会再被自动探活、自动放回调度。
> 现在只剩「手动测试 + 转发失败时的熔断/冷却」，恢复更依赖手动按钮，这一点站长必须知道。

| ✓ | 功能 | 入口 / 接口 / 文件 | 验证方法 | 风险 |
|---|---|---|---|---|
| [ ] | 上游账号列表（筛选/搜索/自动刷新/分页） | 侧边栏「账号管理」→ `/admin/accounts`<br>`GET /admin/accounts`（platform/type/status/group/search/privacy_mode 过滤）<br>`views/admin/AccountsView.vue`、`components/admin/account/AccountTableFilters.vue`、`routes/admin.go:303` | ①平台选 anthropic → 只剩该平台且分页总数变小；②搜索框输账号名前三个字 → 只剩那行；③右上「自动刷新」→ 启用 → 选 30 秒 → 按钮转圈显示倒计时，归零后表格刷新一次。Network 里应看到 `GET /admin/accounts?platform=…&search=…` 返回 200 | 无 |
| [ ] | 新建上游账号（9 平台 × 6 种接入方式） | `/admin/accounts` → 右上「新增账号」<br>`POST /admin/accounts`；`POST /admin/accounts/check-mixed-channel`<br>`components/account/CreateAccountModal.vue`、`credentialsBuilder.ts` | 平台 anthropic + 类型「API Key（上游透传）」→ 填名称、Base URL、API Key → 勾一个分组 → 并发 3、优先级 5 → 保存。列表出现且状态 active、调度开关为开。调 `GET /admin/accounts/{新ID}` 确认 `group_ids`、`concurrency=3`、`priority=5` 与填的一致。再把平台依次切到 openai/gemini/antigravity/grok/kimi/zhipu/deepseek/composite，每切一次「账号类型」区块应跟着换且控制台无报错 | 低 |
| [ ] | OAuth / Setup Token 授权接入 | 新增账号弹窗里选 OAuth 或 Setup Token 后出现的授权面板<br>`POST /admin/accounts/generate-auth-url`、`/exchange-code`、`/cookie-auth` 等；OpenAI 走 `/admin/openai/*`；另有 `/admin/{gemini,antigravity,grok}/oauth/*`<br>`OAuthAuthorizationFlow.vue`、`composables/useAccountOAuth.ts`、`handler/admin/*_oauth_handler.go` | anthropic + OAuth → 点「生成授权链接」→ 面板出现 claude.ai 授权 URL 和 session_id（Network 里返回含 `auth_url`、`session_id`）。把链接在浏览器打开授权，回调 code 粘回 → `POST /exchange-code` 返回 200 并自动带出账号邮箱，保存后类型徽标显示 OAuth。**不想真授权的话，至少验到「生成授权链接」返回 auth_url 即可证明链路通**。注意这和已删除的「站点用户第三方登录」是两套东西 | 无 |
| [ ] | 账号调度参数（并发/负载因子/优先级/计费倍率/到期停用） | `/admin/accounts` → 行内「编辑」→ 「调度设置」<br>`PUT /admin/accounts/:id`<br>`EditAccountModal.vue`（并发 1519、负载因子 1525、优先级 1531、倍率 1543 行）、`service/account.go:22` | 并发 1→5、优先级 1→20、计费倍率 0.8 → 保存 → 列表优先级显示 20、倍率显示 0.80x，**刷新页面后数值仍在**（确认落库不是只改前端状态）。约定：`load_factor <= 0` 表示清除、`proxy_id = 0` 表示解绑，前端已按此转换（`EditAccountModal.vue:4519-4526`） | 无 |
| [ ] | 调度开关与临时不可调度 | `/admin/accounts` → 「可调度」列开关；状态列告警标记 → 「临时不可调度」弹窗<br>`POST /admin/accounts/:id/schedulable`、`GET /:id/temp-unschedulable`、`POST /:id/recover-state`<br>`TempUnschedStatusModal.vue`、`AccountStatusIndicator.vue`、`handler/admin/account_handler.go:1122` | 关掉某账号的可调度开关 → 立刻用该分组的 Key 发一次 `/v1/messages`：这次**不该**落到这个账号（去 使用记录 看账号列，应是同组另一个；若是组里唯一账号则应返回「无可用账号」）。打开后再发一次应能回到它。临时不可调度：点告警标记看解封剩余时间和原因 → 点「恢复运行状态」→ 倒计时消失、账号回到可调度 | 无 |
| [ ] | 账号连通性测试（真发一次上游请求） | `/admin/accounts` → 行内「更多」→「测试」<br>`POST /admin/accounts/:id/test`（SSE 流式）<br>`AccountTestModal.vue:881`、`handler/admin/account_handler.go:1091` | 选 active 账号 → 选模型 → 开始 → 弹窗里**逐字流出**上游返回文本（SSE 生效），底部显示耗时和 token 数，无红色错误。反例：把 API Key 改错再测 → 应看到红色上游错误信息而不是前端白屏。**测试成功还会顺带解除该账号的限流状态**，所以对刚被 429 冷却的账号测通后状态应变回正常。V1 主动探测删除后，这是唯一的账号可用性主动验证手段，排障第一步就该点它 | 无 |
| [ ] | 模型白名单与模型映射（含从上游同步） | 新增/编辑账号弹窗的「模型映射」区块<br>`GET /admin/accounts/:id/models`、`POST /:id/models/sync-upstream`、`POST /models/sync-upstream-preview`；映射存在 `credentials.model_mapping`<br>`ModelWhitelistSelector.vue`、`composables/useModelWhitelist.ts`、`service/account.go`（GetModelMapping 热路径缓存） | 点「从上游同步」→ 弹出预览并列出上游真实模型 ID。手工加一条：from `claude-3-5-haiku-20241022` → to 该账号真实支持的模型 → 保存。用该分组的 Key 发请求，body 里 model 写 `claude-3-5-haiku-20241022` → 请求成功，且 使用记录 里模型列显示成 `a→b` 的映射链（`UsageTable.vue:57` 按 → 拆开逐级显示）。再验白名单语义：只留这一条映射后调 `GET /v1/models`，返回列表应只剩被映射覆盖的那些 | 无 |
| [ ] | 账号绑定分组 + 混合渠道风险确认 | 新增/编辑弹窗的「分组」多选；列表「分组」列<br>`group_ids` 随 `POST/PUT /admin/accounts` 提交；`POST /admin/accounts/check-mixed-channel` 预检<br>`EditAccountModal.vue:2716`、`AccountGroupsCell.vue`、`handler/admin/account_handler.go:787` | 再勾一个分组保存 → 列表出现两个分组徽标。用第二个分组的 Key 发请求，应能命中这个账号（使用记录核对账号列）。反例验预检：把一个账号同时绑到两个绑了不同「渠道」的分组 → 保存时应弹混合渠道风险确认框（Network 可见 check-mixed-channel 返回风险信息），点确认后才提交 | 低 |
| [ ] | 账号批量编辑与批量操作 | `/admin/accounts` → 勾选多行后出现的批量操作条<br>`POST /admin/accounts/bulk-update`、`/batch-delete`、`/batch-clear-error`、`/batch-refresh`、`/batch`<br>`BulkEditAccountModal.vue`、`AccountBulkActionsBar.vue` | 勾 3 个同平台账号 → 批量编辑 → **只改优先级为 9** → 确认返回成功 3 条，这 3 行优先级都变 9，**其它字段（并发、倍率、分组）必须保持不变**——这点最容易出 bug，务必逐行核对。再验按筛选批量：先用平台筛选，勾「选中全部结果」，批量关掉调度开关 → 影响条数应等于**筛选后总数**而不是当前页条数 | 无 |
| [ ] | 数据导入导出 / CRS 同步 / Codex 会话导入 | `/admin/accounts` → 右上「更多」<br>`GET /admin/accounts/data`（**需 step-up**）、`POST /admin/accounts/data`、`POST /sync/crs` 与 `/sync/crs/preview`、`POST /import/codex-session`<br>`SyncFromCrsModal.vue`、`components/admin/account/ImportDataModal.vue`、`handler/admin/account_data.go` | 「数据导出」应弹二次验证，通过后下载 JSON。把该 JSON 原样从「数据导入」传回去 → 应返回「跳过 N 条已存在」而不是重复建号，**账号总数不变**。CRS 同步：填地址与令牌点「预览」，先看到待导入清单再决定是否真同步（预览返回 200 且列出账号名即算通） | 无 |
| [ ] | 账号故障自愈（清错/恢复运行态/清限流/重置额度/刷新令牌） | `/admin/accounts` → 行内「更多」下拉<br>`POST /:id/clear-error`、`/recover-state`、`/clear-rate-limit`、`/reset-quota`、`/refresh`、`/apply-oauth-credentials`、`/set-privacy`、`/revert-proxy-fallback`<br>`AccountActionMenu.vue`、`handler/admin/account_handler.go` | 挑一个 error 状态账号 → 「清除错误」→ 状态立刻回 active、错误信息消失。对被 429 冷却的账号点「恢复运行状态」→ 倒计时清零、重新参与调度（发一次请求看使用记录是否命中它）。OAuth 账号点「刷新令牌」→ 返回 200 且过期时间往后推。**V1 探测删除后，这些按钮从「兜底」变成了「主要恢复手段」** | 无 |
| [ ] | 分组模型路由（把指定模型钉到指定账号） | `/admin/groups` → 编辑分组 → 「模型路由」区块<br>`model_routing`/`model_routing_enabled` 随 `PUT /admin/groups/:id` 提交<br>`GroupsView.vue:1768-1890`、`service/group.go` | 挑一个至少绑两个账号的分组 → 打开开关 → 加规则：模型模式 `claude-opus-*`、优先账号只勾 A → 保存。连发 5 次 `model=claude-opus-4-*`，在 `/admin/usage` 按分组筛选，这 5 条账号列应**全是 A**。再发 5 次 `claude-sonnet-*` → 账号列应出现 B（说明规则只对匹配的模型生效，没把整组钉死） | 无 |
| [ ] | 组合平台模型路由（composite） | `/admin/groups` → platform=composite 的分组 → 行内「组合路由」<br>`GET/POST /admin/groups/:id/composite-routes`、`PUT/DELETE /:route_id`、`POST /composite-routes/preview`<br>`api/admin/groups.ts:282-330`、`handler/admin/group_handler.go:221`、`routes/gateway.go`（compositeTargetPlatformMiddleware） | 建 composite 分组 → 加一条：对外模型 `gpt-5`、匹配 exact、目标平台 openai、上游模型 `gpt-5`。先用「预览」（`POST /composite-routes/preview`，body `{"model":"gpt-5","endpoint":"/v1/responses"}`）→ 返回决策里 `target_platform` 应是 openai。再真发请求：用该组 Key 调 `POST /v1/responses`，model=gpt-5 → 正常返回且使用记录里账号属于 openai 平台。换 `claude-sonnet-*` 再发一次 → 应按另一条规则落到 anthropic 账号 | 无 |
| [ ] | 分组账号准入约束与利润控制 | `/admin/groups` → 编辑分组 → 调度约束区（仅 OAuth / 必须已设隐私 / 仅 Claude Code / RPM 上限 / 利润控制）<br>`require_oauth_only`、`require_privacy_set`、`claude_code_only`、`rpm_limit`、`profit_control_enabled`/`profit_min_margin`/`profit_safety_buffer` 随 `PUT /admin/groups/:id` 提交<br>`handler/admin/group_handler.go:64,79,91-92,126,141,153` | 打开「仅 OAuth」保存 → 组内的 API Key 型账号应不再被调度（发请求看使用记录命中的账号类型）。打开「仅 Claude Code」→ 用普通 HTTP 客户端（非 Claude Code UA）发请求应被拒，用 Claude Code 正常。打开利润控制并把最小毛利设高 → 亏本的模型请求应被拦。每项改完都发一次真实请求验证，**不要只看开关存住了** | 中 |
| [ ] | 代理 IP 管理（增删改查/测试/质量检测/批量/导入导出） | 侧边栏「代理管理」→ `/admin/proxies`<br>`GET/POST /admin/proxies`、`PUT/DELETE /:id`、`POST /:id/test`、`/:id/quality-check`、`GET /:id/accounts`、`/batch`、`/batch-delete`、`GET /admin/proxies/data`（**需 step-up**）、`POST /admin/proxies/data`<br>`views/admin/ProxiesView.vue`、`components/admin/proxy/ImportDataModal.vue`、`routes/admin.go:439-453` | 新建一个代理 → 点「测试」应返回连通结果和延迟；点「质量检测」返回质量评分。把它绑到某账号（账号编辑弹窗的代理选择），再回代理页点「查看账号」应列出这个账号。导出数据应弹二次验证并下载 JSON，原样导入应跳过已存在项。批量删除勾选多行确认后列表清空 | 低 |
| [ ] | 账号定时测试计划 | `/admin/accounts` → 行内「更多」→ 定时测试<br>`POST/PUT/DELETE /admin/scheduled-test-plans`、`GET /:id/results`、`GET /admin/accounts/:id/scheduled-test-plans`<br>`components/admin/account/ScheduledTestsPanel.vue`、`api/admin/scheduledTests.ts` | 给某账号建一个定时测试计划（选模型和周期）→ 保存后面板里出现该计划。等一个周期或手动触发后看「结果」列表应有记录（成功/失败 + 耗时）。删除计划后不再产生新结果。**这是 V1 主动探测删除后，唯一能自动周期性验证账号可用性的东西**，建议给关键账号都配上 | 低 |
| [ ] | 上游错误透传规则 | `/admin/accounts` → 工具栏打开「错误透传规则」弹窗<br>`GET/POST /admin/error-passthrough-rules`、`PUT/DELETE /:id`<br>`components/admin/ErrorPassthroughRulesModal.vue`、`AccountsView.vue:482` | 加一条规则（匹配某个上游错误码/文案）→ 保存。构造一次会触发该上游错误的请求 → 客户端收到的应是**透传的原始上游错误**而不是被包装过的通用错误。删掉规则后同样请求应回到通用错误 | 无 |
| [ ] | TLS 指纹配置档 | `/admin/accounts` → 工具栏「TLS 指纹配置」；账号新建/编辑弹窗里可选用<br>`GET/POST /admin/tls-fingerprint-profiles`、`PUT/DELETE /:id`<br>`components/admin/TLSFingerprintProfilesModal.vue`、`AccountsView.vue:483` | 新建一个指纹档保存 → 在账号编辑弹窗的下拉里能选到它 → 绑定后发一次真实请求应正常返回（说明指纹没把上游握手打坏）。删除档案时若仍被账号引用，应给出提示而不是静默删掉 | 无 |
| [ ] | 上游计费探测 / Ollama Cloud 用量 | `/admin/accounts` 列表的计费倍率单元格与账号更多菜单；开关在 系统设置<br>`GET/PUT /admin/accounts/upstream-billing-probe/settings`、`POST /upstream-billing-probe/batch`、`PUT/POST /:id/upstream-billing-probe`；`GET/PUT/DELETE /:id/ollama-cloud-usage*`<br>`components/account/UpstreamBillingRateCell.vue`、`CNProviderQuotaCell.vue`、`CNProviderBalanceCell.vue` | 打开探测开关后对某账号点单个探测 → 列表该行显示上游计费倍率/额度。批量探测应按勾选条数返回结果。国产平台（kimi/zhipu/deepseek）的额度和余额单元格应显示真实数字，`GET /admin/cn-providers/accounts/:id/quota`、`/balance` 返回 200 | 低 |
| [ ] | 网关转发端点矩阵 | 无界面，客户端直连：`/v1/*` 与 `/v1beta/*`（另有根路径同名端点和 `/antigravity/*`）<br>对话 `POST /v1/messages`、`/v1/responses`、`/v1/chat/completions`；模型表 `GET /v1/models`；计数 `/v1/messages/count_tokens`；用量 `GET /v1/usage`、`GET /v1/sub2api/billing`；图片 `/v1/images/*`（含 `/async` 与 `/images/tasks/:id`）；视频 `/v1/videos/*`；语音 `/v1/tts`、`/v1/stt`、`/v1/custom-voices*`；实时 `/v1/realtime`、`/v1/live`；搜索 `/v1/web_search`、`/v1/x_search`；Gemini 风格 `/v1beta/models/*`<br>`routes/gateway.go:187-488` | 至少覆盖三类客户端各跑一次：①Claude Code 走 `/v1/messages` 流式对话；②Codex/OpenAI 客户端走 `/v1/responses`；③Gemini CLI 走 `/v1beta/models/*`。每次都去 使用记录 确认落了账（有 token 数和金额）。再调 `GET /v1/models` 确认返回的模型表符合该分组账号的白名单/映射结果 | 中 |

---

## 4. 用量、计费与统计

> 计费口径本轮变过：**扣的是 API Key 自己的额度**，不再有用户余额兜底。
> 统计和流水本身没被裁剪，但它们是「额度到底扣没扣对」的唯一证据面，值得连着额度一起验。

| ✓ | 功能 | 入口 / 接口 / 文件 | 验证方法 | 风险 |
|---|---|---|---|---|
| [ ] | 用户仪表盘 | `/dashboard`；用户端主菜单第一项<br>`GET /api/v1/usage/dashboard/stats`、`/trend`、`/models`、`GET /api/v1/usage`（近期记录）、`GET /api/v1/user/platform-quotas`<br>`views/user/DashboardView.vue`、`components/user/dashboard/*` | 用一个有历史用量的账号打开 → 四块内容都要出：顶部统计卡、趋势图、模型分布、最近 5 条使用记录，右侧快捷操作。改一次日期区间（默认近 7 天）→ 图表应重新拉数（Network 里出现新的 trend/models 请求）。切换粒度（天/小时）图表跟着变。设过平台额度的用户，统计卡里应能看到平台额度余量 | 低 |
| [ ] | 用户使用记录（含错误请求） | `/usage`；主菜单「用量」（简易模式下隐藏）<br>`GET /api/v1/usage`、`/stats`、`/errors`、`/errors/:id`、`/dashboard/snapshot-v2`、`/dashboard/models`（整组挂 Heavy 限流）<br>`views/user/UsageView.vue` | 「使用记录」页签：按日期、模型、API Key 筛选后列表和统计都要跟着变；点某行看详情（token 明细和计费）。切到「错误请求」页签 → 应能看到自己的失败请求（**这一项受系统设置 `allow_user_view_error_requests` 控制，关掉后该页签应无数据或不可见**）。点「导出 CSV」应下载当前筛选结果 | 低 |
| [ ] | 管理员仪表盘 | `/admin/dashboard`；后台第一项<br>`GET /admin/dashboard/snapshot-v2`、`/users-ranking`、`/users-trend`<br>`views/admin/DashboardView.vue` | 打开后应一次拿到快照（Network 里主要是 `snapshot-v2` 这一个请求，不是十几个小请求）。核对：总请求数、总花费、活跃用户数、消费排行榜。跑一次真实对话后刷新 → 数字应增加。切换时间范围排行榜跟着重算 | 低 |
| [ ] | 管理员使用记录（流水/错误/排行三页签） | `/admin/usage`；后台「用量」<br>`GET /admin/usage`、`/stats`、`/search-users`、`/search-api-keys`、`GET /admin/dashboard/models`<br>`views/admin/UsageView.vue`、`components/admin/usage/UsageFilters.vue` | 三个页签都要点开：①「使用记录」按用户/Key/模型/日期筛选（用户和 Key 的下拉是**搜索式**的，输入邮箱片段应能搜出来）；②「错误请求」能看到失败请求及原因；③「排行」显示消费排名。**用它验计费**：拿一把设了倍率的分组的 Key 跑一次对话，然后在这里找到这条记录，确认金额 = 原价 × 分组倍率（或用户专属倍率），且 token 数与上游返回一致。点「导出 Excel」应下载当前筛选结果 | 中 |
| [ ] | 用量数据清理任务 | `/admin/usage` → 筛选栏的「清理」按钮<br>`GET/POST /admin/usage/cleanup-tasks`、`POST /cleanup-tasks/:id/cancel`<br>`views/admin/UsageView.vue:169,548` | 创建一个清理任务（选一个很早的截止日期，避免误删有用数据）→ 任务列表出现并显示进度。点「取消」→ 任务状态变为已取消。**危险操作，生产上先在测试环境验一遍**，确认删除范围就是你选的日期区间 | 中 |
| [ ] | Key 计费信息查询端点 | 无界面，客户端直连：`GET /v1/sub2api/billing`<br>`routes/gateway.go:187`、`handler/gateway_handler.go`（KeyBillingInfo） | 带 Key 调一次 → 返回该 Key 的计费与额度信息。**和 `/v1/usage` 对照着看**：不限额的 Key 两边都应表达「无限制」，设了额度的两边的剩余数字应一致（这是第三方客户端读余量的入口，口径不一致会直接显示错） | 中 |
| [ ] | 用户自定义属性 | `/admin/users` → 「属性配置」；用户编辑弹窗里填值<br>`GET/POST /admin/user-attributes`、`PUT/DELETE /:id`、`POST /admin/user-attributes/batch`、`GET/PUT /admin/users/:id/attributes`<br>`components/user/UserAttributesConfigModal.vue`、`UserAttributeForm.vue` | 新建一个属性定义（如「所属部门」，文本类型）→ 在用户编辑弹窗里出现该字段，填值保存后刷新还在。用户列表应能按该属性显示/筛选。删除定义后，用户编辑弹窗里该字段消失 | 无 |

---

## 5. 系统设置、运维与数据

> 系统设置页共 **6 个页签**：通用 / 功能开关 / 安全与认证 / 用户默认值 / 网关 / 数据备份
> （`SettingsView.vue:4447-4454`）。安全与认证页签下的条目已在第 1 章逐条列出，这里不重复。

| ✓ | 功能 | 入口 / 接口 / 文件 | 验证方法 | 风险 |
|---|---|---|---|---|
| [ ] | 通用设置 | 设置 → 通用<br>字段 `backend_mode_enabled`、`doc_url`<br>`SettingsView.vue:3979-4164` | 改文档地址保存 → 刷新后仍在，且前台的文档入口指向新地址。非法 URL 会被**自动清空**而不是报错（`saveSettings` 里的 `isValidHttpUrl` 兜底），填错了不要以为存住了。「后台模式」见第 1 章 | 低 |
| [ ] | 功能开关 | 设置 → 功能开关<br>字段 `available_channels_enabled`、`channel_monitor_enabled`、`channel_monitor_hide_throughput`、`risk_control_enabled`、`cyber_session_block_enabled`、`cyber_session_block_ttl`<br>`SettingsView.vue:4164-4307`、`utils/featureFlags.ts` | 逐个开关翻一遍，每翻一次都刷新页面看**侧边栏菜单是否跟着增减**：可用渠道 → `/available-channels`；渠道监控 → 用户端「渠道状态」和后台「渠道管理 → 渠道监控」；风控 → 后台「安全审计 → 提示词审计」。关掉后除了菜单消失，直接敲路由也应被挡住（路由守卫 + 后端接口双重生效，不能只靠藏菜单） | 中 |
| [ ] | 用户默认值 | 设置 → 用户默认值<br>字段 `default_concurrency`、`default_user_rpm_limit`、`default_platform_quotas`（按平台的日/周/月）<br>`SettingsView.vue:2075-2196` | 把默认并发改成 3、默认 RPM 改成 60、给 anthropic 设一个默认日额度 → 保存。然后**新建一个用户**，打开它的编辑弹窗和平台额度弹窗 → 应带出这些默认值（对已存在的用户不追溯，这是预期的） | 低 |
| [ ] | 网关行为设置 | 设置 → 网关<br>`allow_ungrouped_key_scheduling`、`allow_user_view_error_requests`、客户端版本门槛（`min/max_claude_code_version`、`min/max_codex_version`）、`enable_fingerprint_unification`、`enable_metadata_passthrough`、`enable_anthropic_cache_ttl_1h_injection`、`rewrite_message_cache_control`、`enable_client_dateline_normalization`、`enable_cch_signing`、OpenAI 高级调度 `openai_advanced_scheduler_*`、Grok 相关<br>`SettingsView.vue:2196-3979` | 挑三个影响最直接的验：①关掉「允许未分组 Key 调度」→ 未绑分组的 Key 发请求应被拒；②把 `min_claude_code_version` 设成一个比你本机高的版本 → Claude Code 请求应被拒并提示升级，设回空恢复；③打开「允许用户查看错误请求」→ 用户 `/usage` 的「错误请求」页签出数据，关掉后应看不到。其余开关改动后至少各发一次真实请求确认没把转发打坏 | 中 |
| [ ] | 管理员 API Key | 设置 → 安全与认证 → 「管理员 API Key」卡片<br>`GET /admin/settings/admin-api-key`、`POST /admin-api-key/regenerate`、`DELETE /admin-api-key`<br>`SettingsView.vue:55` | 生成一把 → 用它带 header 调一个管理接口应成功。**再用它调受 step-up 保护的接口（如清空审计日志、导出账号数据）→ 应被明确拒绝**（前端文案 `stepUp.adminApiKeyForbidden`），这是有意的：脚本身份不能做敏感操作。点重新生成 → 旧 Key 立刻失效。删除后所有调用都应 401 | 中 |
| [ ] | 上游冷却与限流参数 | 设置 → 网关（各自独立的保存按钮）<br>`GET/PUT /admin/settings/overload-cooldown`、`/rate-limit-429-cooldown`、`/panel-rate-limit`、`/stream-timeout`、`/rectifier`、`/beta-policy`<br>`api/admin/settings.ts` | 每张卡片各自保存后**刷新页面确认回显**（这几项走的是独立接口，不是主表单，容易出现「点了保存但没落库」）。把 429 冷却时间改短，人为触发一次上游 429 → 账号进入冷却的时长应符合新设置。面板限流调小后，快速刷面板接口应更早出现限流提示 | 中 |
| [ ] | Web 搜索模拟 | 设置 → 网关 → Web 搜索模拟卡片<br>`GET/PUT /admin/settings/web-search-emulation`、`POST /web-search-emulation/test`、`/reset-usage`<br>`SettingsView.vue`、`views/admin/ChannelsView.vue` | 配好搜索服务参数 → 点「测试」应返回真实搜索结果而不是超时。开启后用一个带 web_search 工具的请求跑一次 → 应能拿到搜索结果。点「重置用量」后配额计数归零 | 低 |
| [ ] | 数据备份（S3 / 计划 / 创建 / 下载 / 恢复） | 设置 → 数据备份页签<br>`GET/PUT /admin/backup/s3-config`（改 S3 **需 step-up**）、`/s3-config/test`、`GET/PUT /admin/backup/schedule`、`POST /admin/backup`（**需 step-up**）、`GET /admin/backup`、`GET /:id/download-url`（**需 step-up**）、`POST /:id/restore`（**需 step-up**）、`DELETE /:id`<br>`views/admin/BackupView.vue`、`api/admin/backup.ts` | 配好 S3 点「测试连接」→ 成功。点「创建备份」→ 弹二次验证 → 任务出现在列表并最终变成完成。点「下载」→ 同样弹二次验证 → 能拿到文件且能解开。设一个每日计划 → 保存后回显正确。**恢复功能务必先在测试环境验**，确认恢复后数据完整、服务能正常起来 | **高** |
| [ ] | 图片存储配置 | 设置 → 数据备份 → 图片存储卡片<br>`GET/PUT /admin/backup/image-storage`（**需 step-up**）、`POST /image-storage/test` | 配好后点「测试」应成功。跑一次图片生成请求 → 生成的图片应能通过返回的地址正常打开（批量生图已删，但**走网关的单次图片生成仍在**，图片要有地方存） | 中 |
| [ ] | 审计日志 | 侧边栏「审计日志」→ `/admin/audit-logs`（简易模式下隐藏）<br>`GET /admin/audit-logs`、`/:id`、`POST /clear`<br>`views/admin/AuditLogView.vue`、`handler/admin/audit_log_handler.go` | 做几个动作（登录、改设置、建用户、启停 2FA）后打开这一页 → 应能看到对应记录，含操作人、动作、来访 IP、时间。**重点核对来访 IP 是真实公网 IP 而不是反代内网地址**（否则去查第 1 章的 IP 来源配置）。点某条看详情。保留天数在 设置 → 安全与认证 的 `audit_log_retention_days`（0 = 永久）。清空功能见第 1 章 | 低 |
| [ ] | 运维监控大盘 | 侧边栏「运维监控」→ `/admin/ops`（受设置项 `ops_monitoring_enabled` 控制，**但设置页里没有这个开关**，见「发现的残留」）<br>`GET /admin/ops/dashboard/*`、`/concurrency`、`/realtime-traffic`、`/errors`、`/request-errors`、`/upstream-errors`、`/requests`、`/system-logs`、`/alert-rules`、`/alert-events`<br>`views/admin/ops/OpsDashboard.vue` 及 `components/Ops*.vue` | 打开后各卡片都应出数：并发情况、吞吐趋势、延迟直方图、错误趋势与分布、OpenAI token 统计、系统日志表。制造一次失败请求（用错的上游 Key）→ 几秒后应出现在「请求错误」里，点开能看到上游错误详情。建一条告警规则（如错误率阈值）→ 触发后在「告警事件」里出现，能标记状态、能建静默 | 中 |
| [ ] | 运维高级设置与指标阈值 | `/admin/ops` → 右上设置对话框<br>`GET/PUT /admin/ops/advanced-settings`、`GET/PUT /admin/ops/settings/metric-thresholds`、`GET/PUT /admin/ops/runtime/alert`、`/runtime/logging`、`POST /runtime/logging/reset`<br>`OpsSettingsDialog.vue`、`OpsSystemLogTable.vue` | 改一个指标阈值保存 → 刷新回显正确，大盘上对应指标的红黄绿判定跟着变。改日志级别后新产生的日志详细程度应跟着变，点「重置」回到默认。**注意：`OpsRuntimeSettingsCard.vue` 这个组件没有任何页面引用**（见「发现的残留」），告警运行时设置目前只能从别处改 | 中 |
| [ ] | 系统日志清理 | `/admin/ops` → 系统日志表格的清理入口<br>`POST /admin/ops/system-logs/cleanup`、`GET /system-logs/health`<br>`OpsSystemLogTable.vue` | 执行一次清理 → 返回清理条数，列表随之变短。`/system-logs/health` 应返回采集健康状态（用来判断日志有没有在正常写入） | 低 |
| [ ] | 渠道定价管理 | 侧边栏「渠道管理 → 渠道定价」→ `/admin/channels/pricing`<br>`GET/POST /admin/channels`、`PUT/DELETE /:id`、`GET /admin/channels/model-pricing`、`/pricing/sync-models`<br>`views/admin/ChannelsView.vue` | 新建一个渠道并配模型单价 → 保存后列表出现。点「同步模型」→ 拉回上游模型列表供选择。**改完价格后跑一次真实请求，去 `/admin/usage` 核对这条记录的金额是按新价 × 分组倍率算的**——这是渠道定价唯一可信的验证方式 | 中 |
| [ ] | 渠道监控 V2（管理端配置） | 侧边栏「渠道管理 → 渠道监控」→ `/admin/channels/monitor`（受 `channel_monitor_enabled` 控制）<br>`GET/PUT /admin/channel-monitor-v2/config`、`GET /snapshot`、`/models`、`/matrix`、`/errors`、`/users`、`/dimensions`<br>`views/admin/ChannelMonitorView.vue` → `features/channel-monitor-v2/MonitorSettingsPanel.vue` | 打开配置面板改一项（如统计窗口）保存 → 回显正确。**这是被动聚合视图，不会主动向上游发探测请求**（V1 主动探测已删），所以数据只在有真实流量时才动——先跑几次请求再来看 | 中 |
| [ ] | 渠道状态（用户端只读） | 用户菜单「渠道状态」→ `/monitor`（受同一开关控制）<br>`GET /api/v1/channel-monitor-v2/dimensions`、`/snapshot`、`/models`、`/matrix`、`/errors`、`/users`（挂 Heavy 限流 + 功能开关守卫）<br>`views/user/ChannelStatusV2View.vue` | 用普通用户账号打开 → 能看到矩阵、趋势、模型维度和错误分布。**核对越权**：用户只应看到自己有权访问的分组维度。打开 `channel_monitor_hide_throughput` 后，吞吐相关的数字应对用户隐藏。关掉总开关后这一页应完全不可达（接口返回也要被守卫拦住，不能只是菜单消失） | 中 |
| [ ] | 提示词审计（风控） | 侧边栏「安全审计 → 提示词审计」→ `/admin/prompt-audit`（受 `risk_control_enabled` 控制）<br>`GET/PUT /admin/prompt-audit/config`、`POST /endpoints/probe`、`GET /runtime`、`GET /events`、`/events/:id`、`DELETE /events/:id`、`POST /events/batch-delete`、`/delete-preview`、`/delete-by-filter`<br>`features/prompt-audit/PromptAuditView.vue` 及 `components/` | 配一个审计端点点「探测」→ 应返回连通结果。开启后跑几次对话 → 「事件」列表应出现审计记录，点开能看到判定详情。试一次按条件批量删除：**先点「预览」看清将删掉多少条**再执行。关掉总开关后菜单和接口都应不可用 | 中 |
| [ ] | 落地页与 404 | `/home`（可设为默认首页）、任意不存在的路径<br>`views/HomeView.vue`、`views/NotFoundView.vue`、`router/index.ts:48,319` | 未登录访问 `/home` 应正常渲染落地页且「登录」按钮指向**当前生效的登录入口**（隐藏入口开启时不能暴露真实路径）。随便敲一个不存在的路径 → 应显示 404 页而不是白屏或跳登录 | 低 |
| [ ] | 健康检查 | `GET /health`（不在 `/api/v1` 下，无需认证）<br>`routes/common.go:12` | `curl https://<站点>/health` 返回 200。这是反代/容器编排探活用的，**发版后第一个该确认的接口** | 低 |

---

## 发现的残留

导出清单时做的是**双向核对**，对不上的东西都收在这里。按「会不会害到站长」排序，不是按数量排。
每条都标了具体文件/行号，可以直接照着处理。

### P0 · 会误导人做出错误判断（建议尽快处理）

| ✓ | 残留 | 位置 | 为什么危险 | 建议 |
|---|---|---|---|---|
| [ ] | **「注册设置」卡片还能填能存，但邮箱域名白名单零生效** | `views/admin/SettingsView.vue:1425-1503`；`service/setting_features.go`；`service/registration_email_policy.go:49-74` | 自助注册已整体删除。`IsRegistrationEmailSuffixAllowed` 只被同文件的 `IsRegistrationEmailSuffixLimited` 调用，而后者**在生产代码里零调用点**（已实测确认，只有单元测试引用）。站长照着这个输入框限制域名，会以为生效了——实际上后台建用户时填任何邮箱都能建成功，**这是一种完全虚假的安全感** | 删掉整张卡片（只留下面的 2FA/Passkey/二次验证三个开关），或者把白名单改成在「后台建用户」时真正生效 |
| [ ] | 解绑第三方登录接口仍然活着 | `DELETE /api/v1/user/account-bindings/:provider`；`handler/user_handler.go:171`；`routes/user.go:32` | 第三方登录整套已删，前端零调用，但这个接口不但活着，**成功后还会撤销该用户的全部令牌**。等于留了一个没人看守、副作用不小的入口 | 连同路由一起删掉 |
| [ ] | 管理员手工绑定第三方身份接口 + 前端封装都还在 | `POST /api/v1/admin/users/:id/auth-identities`（`routes/admin.go:250`）；`api/admin/users.ts:247` 的 `bindUserAuthIdentity` | 后端活着、前端封装活着，但后台界面上根本没有这个入口。属于「随时可能被误调用」的悬空能力 | 后端路由 + 前端封装一起删 |

### P1 · 有能力没入口 / 有入口没能力（功能上白写了）

| ✓ | 残留 | 位置 | 说明 |
|---|---|---|---|
| [ ] | **运维监控的三个开关在界面上根本改不了** | 设置项 `ops_monitoring_enabled`、`ops_realtime_monitoring_enabled`、`ops_query_mode_default`；`stores/adminSettings.ts:47-66` 读取，`AppSidebar.vue` 用它控制「运维监控」菜单；但 `SettingsView.vue` 里只有第 5053 行的表单默认值，**没有任何输入控件，保存时也不在 payload 里** | 站长想关掉运维监控菜单，只能直接改数据库或调接口。要么在设置页补上这三个控件，要么把开关去掉恒定开启 |
| [ ] | 告警运行时设置卡片是个孤儿组件 | `views/admin/ops/components/OpsRuntimeSettingsCard.vue`（全前端零引用，已实测） | 组件写完了但没挂到任何页面上，`/admin/ops/runtime/alert` 这对接口因此没有界面入口 | 
| [ ] | 「撤销我的全部登录会话」有接口没按钮 | `POST /api/v1/auth/revoke-all-sessions`（`routes/auth.go:72`）；`api/auth.ts:169` 有封装 | 这功能对站长其实有用（改完密码一键踢掉所有设备），现在等于白写。要么在个人资料页补个按钮，要么删掉 |
| [ ] | 整个「数据管理」接口族没有任何前端调用 | `/admin/data-management/*` 共 14 条（`routes/admin.go:495-512`）；`api/admin/dataManagement.ts` 整个模块零调用（已实测） | 备份实际走的是 `/admin/backup` 那一套。这一族是并行实现的另一套，界面从没接上 |
| [ ] | 版本查询与**重启服务**接口没有 UI | `GET /admin/system/version`、`POST /admin/system/restart`（`routes/admin.go:553-554`）；`api/admin/system.ts` 零调用 | 「重启服务」这种高危动作挂在那儿没人用，建议要么接上并加 step-up 保护，要么删掉 |
| [ ] | 实时 QPS 的 WebSocket 没有消费方 | `GET /admin/ops/ws/qps`（`routes/admin.go:183`）；`api/admin/ops.ts:507` 的 `subscribeQPS` 零调用 | 大盘现在靠轮询，这条 WS 通道白开着 |
| [ ] | 一批后端接口前端零引用 | `POST /admin/accounts/:id/refresh-tier`、`/batch-refresh-tier`；`GET /admin/grok/runtime-sanity`；`POST /admin/grok/oauth/reconcile`；`GET /admin/ops/ingress-rejections`(+`/health`)；`GET /admin/ops/auth-cache-invalidation/health`；`POST /admin/dashboard/aggregation/backfill`；`POST /admin/users/batch-concurrency`（已被 `/batch-limits` 取代）；`GET /admin/users/:id/rpm-status`；`GET /admin/users/:id/usage` | 全部实测确认前端零调用。其中 ingress-rejections 那两条有配套的命令行工具（`backend/cmd/cleanup-ingress-reject-logs`），可能是有意只留给运维用；其余建议清理 |
| [ ] | 一批前端封装零调用方 | `api/keys.ts` 的 `getById`；`api/usage.ts` 的 `getMyApiKeyDailyUsage`、`getStatsByDateRange`；`api/admin/groups.ts` 的 `getGroupApiKeys`、`getStats`、`getByPlatform`、`toggleStatus`；`api/admin/users.ts` 的 `updateConcurrency`、`getUserUsageStats`；`api/admin/dashboard.ts` 的 `getRealtimeMetrics`、`getUsageTrend`、`getGroupStats`、`getApiKeyUsageTrend`、`getBatchApiKeysUsage`；`api/admin/userAttributes.ts` 的 `reorderDefinitions`；`api/admin/ops.ts` 的 `getErrorLogDetail`、`updateErrorResolved`、`updateRequestErrorResolved`、`updateUpstreamErrorResolved`；`api/totp.ts:30` 的 `getVerificationMethod` | 死封装，不影响运行。其中 `GET /user/totp/verification-method` 在邮件体系删除后**恒定返回 password**，可以直接连接口一起清掉 |

### P2 · 文案与死代码（不影响功能，看着不对）

| ✓ | 残留 | 位置 | 说明 |
|---|---|---|---|
| [ ] | `subscription_type` 字段前端还在 8 个文件里传来传去 | `types/index.ts:333`；`views/user/KeysView.vue`(144/475/489/1087/1416)；`UserApiKeysModal.vue`；`GroupSelector.vue`；`AccountGroupsCell.vue`；`AvailableChannelsTable.vue`；`api/channels.ts`；数据库 `ent/schema/group.go` 的列也还在 | 后端 Group DTO 已彻底移除该字段，运行时永远是 `undefined`，GroupBadge 默认回落 `'standard'`，所以只是徽标永远显示「标准型」，不报错 |
| [ ] | `/key-usage` 页写着「订阅类型」和「钱包余额」 | `views/KeyUsageView.vue:901,1012`（i18n `keyUsage.subscriptionType`、`keyUsage.walletBalance`） | 不限额模式下明细第一行标签写「订阅类型」，值其实是分组名；Key 没绑分组时兜底文案是「钱包余额」。订阅和余额都已删除，应改成「所属分组 / 不限额度」 |
| [ ] | 一组失效 i18n 文案零引用 | `i18n/locales/zh/admin/overview.ts:561` 的 `allowedGroupsHint`（写着「订阅类型分组请在订阅管理中配置」）及同组 `setAllowedGroups`/`allowAllGroups`/`allowAllGroupsHint`/`noStandardGroups`/`allowedGroupsUpdated`/`failedToUpdateAllowedGroups`，还有 `insufficientBalance` | 旧的「设置允许分组」弹窗已被 `UserAllowedGroupsModal` 取代，这批文案没清干净 |
| [ ] | `views/auth/` 下三份文档还在教人用已删除的注册页 | `frontend/src/views/auth/README.md`、`VISUAL_GUIDE.md`、`USAGE_EXAMPLES.md`（均确认仍含 `RegisterView` 段落） | 里面给出 `import RegisterView from '@/views/auth/RegisterView.vue'` 这类示例，而该文件已删除，**照着这份文档动手会直接编译不过** |
| [ ] | 个人资料页的「账号绑定」区块只剩一块只读展示 | `components/user/profile/ProfileIdentityBindingsSection.vue` | 邮箱绑定/换绑已删，卡片现在纯只读、下面写着「登录邮箱由管理员维护」。功能没坏，但对用户来说是一块什么也做不了的东西 |
| [ ] | 订阅相关死类型定义 | `types/index.ts:146-175`（`Subscription`、`CreateSubscriptionRequest`、`UpdateSubscriptionRequest`） | 订阅体系已整体删除，前端也没有对应的 api 模块 |
| [ ] | Key 用量脚本里的余额兜底字段 | `views/user/KeysView.vue`（生成的脚本里读 `response?.balance`） | 余额体系删掉后这个字段永远不会出现，但它排在 `remaining` 和 `quota.remaining` 之后，取不到也不影响 |
| [ ] | 审计日志路由白收一个 step-up 参数 | `routes/admin.go:122` `registerAuditLogRoutes(admin, h, _ middleware.StepUpAuthMiddleware)` | 清空审计日志改成了在 handler 内做现场 TOTP 校验，这个参数用 `_` 忽略掉了。留着容易让人误以为这条路由被 step-up 保护 |

> **明确不需要处理的一条**：`config/web_entry.go:63` 的保留路径名单里还有 `/model-plaza` 和 `/subscriptions`。
> 这是**刻意保留**的——名单只是禁止把这些路径设成隐藏登录入口，跟页面是否存在无关；
> 默认首页白名单里 `/model-plaza` 已经删掉，且有 `web_entry_lock_test.go:128` 锁着。

---

## 本轮裁剪的高风险面

赶时间的话只测这些。按「坏了以后有多疼」排序。

### 必测（坏了 = 站长自己进不去，或者用户当场报障）

| ✓ | 条目 | 所在章节 | 为什么 |
|---|---|---|---|
| [ ] | 邮箱 + 密码登录 | 1 | `auth_handler.go` 减 400 行、`api/auth.ts` 减 514 行、`stores/auth.ts` 减 178 行，登录主链路是从这堆删除里**幸存**下来的 |
| [ ] | adminpass 自救工具存在于运行镜像里 | 1 | 「找回密码」随邮件体系删干净了，这是**唯一**的自救通道。上次发版曾漏打进镜像（已修），每次发版都值得确认 `/app/adminpass` 在 |
| [ ] | 个人资料页四张卡片 | 1 | `ProfileView.vue` 减 66 行、`api/user.ts` 减 147 行、`user_handler.go` 减 440 行（邮箱换绑、第三方解绑、余额全删），整页要逐张卡片点一遍 |
| [ ] | 自己开关 2FA | 1 | 原来可用邮箱验证码，邮件体系删除后统一改成密码验证，`TotpSetupModal.vue` 改了 158 行。**最容易留下「点了按钮转圈不动」** |
| [ ] | 删除用户 | 1 | 牵扯 Key、用量、分组多张表，而本轮删了大量关联表。后端若还在清理已不存在的表会直接报错，**必须真删一个测试用户** |
| [ ] | Key 额度语义（quota=0 = 不限额） | 2 | 用户余额兜底已拆，判定完全落在 Key 自己的 quota 上。老 Key 里「没设额度、靠账户余额控着」的，现在全是不限额 |
| [ ] | 专属分组授权（撤销后当场 403） | 2 | 判定只认 `user_allowed_groups` 那行授权，订阅旁路已拆。历史上靠订阅拿分组、授权表没写行的用户，**升级后 Key 会当场 403** |
| [ ] | 数据备份的创建 / 下载 / 恢复 | 5 | 备份链路本轮动过，且恢复一旦出问题就是灾难性的。**务必先在测试环境验一次完整恢复** |

### 建议测（改动集中区，或开关组合处容易出问题）

| ✓ | 条目 | 所在章节 | 为什么 |
|---|---|---|---|
| [ ] | 隐藏登录入口 + 后台模式（**两个开关组合起来各试一遍**） | 1 | 隐藏路由是运行时动态注册的，而 `router/index.ts` 本轮减了 630 行；两个开关的落点逻辑纠缠在一起（防重定向死循环） |
| [ ] | 登录态自动续期 | 1 | 刷新逻辑本身没动，但和登录状态存储耦合紧。坏了的表现是「用着用着突然被踢回登录页」 |
| [ ] | 登录接口限流仍然挂着 | 1 | `routes/auth.go` 本轮减了 192 行（删注册/找回/第三方那批路由），限流包裹有没有跟着丢要确认 |
| [ ] | 管理员建用户 + 新账号真能登录 | 1 | 注册体系删除后这是唯一开户途径，`admin/user_handler.go` 减 115 行。别只看列表里有没有出现 |
| [ ] | 提权时的二次验证 | 1 | 「改成管理员才弹验证码」这段条件写在 handler 里而不是路由上，属改动集中区 |
| [ ] | 用户平台额度 | 1 | 订阅和余额整套删掉后，平台额度是幸存下来的另一套限额，要确认没被误伤 |
| [ ] | 管理员改 Key 分组时自动补授权 | 2 | 这是「专属分组严格按授权名单」的唯一合法例外。**如果自动补授权没生效，管理员改完绑定用户立刻 403** |
| [ ] | 替换用户专属分组（三步事务） | 2 | 授权新分组 → 迁移 Key → 撤销旧授权，任一步失败要整体回滚。没回滚干净的话用户会落在「绑了 Y 但没 Y 的授权」状态 |
| [ ] | 分组增删改（限额输入框应该已经没有了） | 2/3 | `GroupsView.vue` 少了 578 行，后端删了 `subscription_type` 和日/周/月限额。**站长以前靠分组限额控成本的话，这条路没了**——额度现在只在 API Key 上 |
| [ ] | 账号挂掉后的恢复路径 | 3 | 渠道监控 V1 主动探测已删：账号不会再被自动探活、自动放回调度。「手动测试」和「恢复运行状态」从兜底变成了**主要**恢复手段，定时测试计划值得给关键账号都配上 |
| [ ] | 计费金额对不对 | 4 | 扣的是 Key 自己的额度，没有余额兜底。跑一次真实请求，去 `/admin/usage` 核对金额 = 原价 × 倍率，这是唯一可信的验证方式 |
| [ ] | 功能开关关掉后接口也被挡 | 5 | 可用渠道 / 渠道监控 / 风控三个开关，关掉后不能只是菜单消失，直接敲路由和直接调接口都应被拦 |

---

## 怎么维护这份文档

这份清单的价值全在「和代码一致」。不一致的清单比没有清单更糟——它会让人以为验过了。

**规矩很简单：改功能的那个 PR 里，顺手改这里。**

- **新增功能** → 在对应章节的表格里**加一行**。四列都要填，`验证方法` 那列必须写成
  「点哪里 → 应该看到什么」，能让一个没读过代码的人照着跑完。入口列记得带上路由、后端接口和关键文件。
- **删除功能** → 把那一行**删掉**，别留着划线的历史。如果删完留下了孤儿路由/孤儿页面/孤儿封装，
  补一条到「发现的残留」里，等清理完再删。
- **改了入口或接口** → 只改那一行，不要新增一行。文件行号会飘，**行号对不上时以函数名/组件名为准**。
- **风险列的维护**：默认写「无」。只有当这条功能所在的文件在本次改动里被大幅增删、
  或者它的语义变了（像本轮的额度和专属分组），才升成「中」或「高」，并在
  [本轮裁剪的高风险面](#本轮裁剪的高风险面) 里同步加一行。等下一轮改动过去、这条稳定了，再降回「无」。
- **每次发版后**：至少把「必测」那 8 条勾一遍。全绿再宣布发版完成。
- **勾选状态怎么办**：一轮验收结束后，把所有 `[x]` 批量改回 `[ ]`，让文档回到「待验」状态。
  想留存记录就把那一版的勾选结果贴进当次的发版记录里，不要在这份文档里累积。

**重新导出一次清单**（怀疑文档已经和代码脱节时，照这四步重跑）：

1. 前端路由表 `frontend/src/router/index.ts` —— 每条路由都要在本文档里找得到对应行。
2. 后端路由注册 `backend/internal/server/routes/*.go` —— 每个注册的接口都要有前端调用方或被明确列为残留。
3. 侧边栏菜单 `frontend/src/components/layout/AppSidebar.vue` —— 每个菜单项都要指向存在的路由。
4. 前端 API 封装 `frontend/src/api/` 与 `frontend/src/features/*/api.ts` —— 每个封装都要有调用方。

四步跑完，对不上的就是新的残留，加到「发现的残留」里。
