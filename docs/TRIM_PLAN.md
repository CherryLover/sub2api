# 内部部署裁剪：计划·进度·验证

> 目标：将 sub2api 裁剪为**单管理员、内部部署**的 API 转发网关。
> 约定：转发链路（全部上游平台、全部入站协议、协议转换层、Codex 版本同步）**全量保留**；
> SaaS 商业化与多用户外围逐批删除。每批：拆包 → 多 Agent 并行实施 → 逐包验证 →
> 合并后统一验证 → CI 全绿 → RC 镜像 → 内网部署验证 → 合并 main。
>
> 维护说明：本文档由裁剪工程随批次更新，是唯一权威的计划、进度与部署验证记录（验证清单见第五节，问题追踪见第六节）。

---

## 一、总体进度（对照最初 7 阶段计划）

| 原计划阶段 | 状态 | 说明 |
|---|---|---|
| 1. Fork 与范围冻结 | 🔵 进行中（批次 2） | 范围与协议决策已定（转发面全保留）；应用内更新检查/在线升级/回滚**批次 1 已整删**；批次 2 做 **A1 尾款**（install.sh 的 update/rollback 子命令、compose/.env.example/README 过时注释） |
| 2. 数据模型收敛 | ⚪ 未开始 | 修订：不删 User 实体、不建 key_period_usage、不改名 RoutePool；剩余=订阅/余额语义改造 + ent 死实体清理（并入数据库压平） |
| 3. Key 认证与额度引擎 | 🟢 大部分已满足 | 防爆破/IP ACL/幂等结算/惰性窗口/时区均已存在并经审查确认；待做=周额度改自然周锚点（A3） |
| 4. 网关与上游账户池 | ✅ 完成（按决策零改动） | 全量保留 |
| 5. 最小管理员后台 | 🔵 进行中（批次 2） | 商业化入口已拆；批次 2 做 **B1**（登录条款/合规确认）+ **B2**（通用设置冗余项）；Keys/用量页单管理员语义仍未做（A6） |
| 6. 删除 SaaS 外围 | 🔵 进行中（批次 2） | 批次 1 已删支付/卡密/第三方 OAuth 等（约六成）；批次 2 做 **B3**（注册体系彻底移除）+ **B4**（人机验证全部移除）；公告/模型广场/邮销未删（A2）；**B5 邮件体系调研已完成，待站长拍板**（见第三节 B5 专节） |
| 7. 安全与上线验证 | 🟡 批次 1 已收口 | 三轮 RC 镜像内网部署验证全部通过并已合并发版（见第二节收口事实）；**批次 2 需重走一轮 RC 验证**；压测与备份恢复演练仍未做（A7） |

**当前位置**：批次 1 已合并 main 并发版 v0.1.182（收口事实见第二节）；
**批次 2 首发（B1–B4 + A1 尾款）多 Agent 并行实施中**，B5 邮件体系待站长拍板后并入后续批次。
**关口**：auto-release 经站长决策**保持现状不动**（合并后自动发版即预期行为，内部版本线就走 main）。

### ⚠️ 生产影响：`:latest` 已经是生产的活动标签

生产 `way.flyooo.uk` 的镜像坐标**已从固定版本号改为 `ghcr.io/cherrylover/sub2api:latest`**。
这条改动改变了发版的风险模型，批次 2 及以后每批都必须按此执行：

- **合并 main 会自动发版并覆盖 `:latest`，生产容器下次重启就会拉到新版本。**
  "发版"与"上生产"之间不再有人工改 tag 的闸门 —— 合并即等于把新版本预置到了生产上。
- 因此**破坏性变更必须先出 `internal-rc-<short_sha>` 标签、走 way-rc（`way-rc.flyooo.uk`）验证，
  验证通过再合并 main**。禁止"先合并 main 再验证"。
- 批次 2 中 **B3（注册体系彻底移除）触及登录与用户表语义，属破坏性变更，必须走 RC 验证**；
  B1/B2/B4/A1 尾款虽是低耦合删除，也一并纳入同一轮 RC 镜像一起验证。
- 回滚路径相应变化：出问题时**不能只靠重启**（重启只会再拉一次 `:latest`），
  需先把生产 compose 的镜像临时钉到上一个正式版本 tag（如 `ghcr.io/cherrylover/sub2api:0.1.182`）再重启。

---

## 二、批次 1（已交付并合并 main，分支 `claude/project-trim-internal-deploy-7jnynw`，CI 全绿）

- 支付/订单/充值/卡密/优惠码/推广返佣：前后端整套删除（含 5 个支付 provider、支付 SDK 依赖）
- 六种第三方 OAuth 登录（LinuxDo/微信/钉钉/OIDC/GitHub/Google）：整套删除，含 pending 绑定流程与设置面
- 多用户默认禁用：注册种子默认关闭 + 迁移 `229` 对老库升级强制关闭一次
- 保留面确认无损：邮箱密码登录、TOTP、Passkey、订阅体系、上游账号 OAuth、ops 监控、备份、审计
- 护栏：门禁契约测试（37 项断言，已删路由缺席 + 注册默认拒绝 + 保留面防误删）；对抗审查闭环（1×P1 已修，4×P2 两修两记录）
- 交付物：RC 镜像流水线（`branch-docker-image.yml`，`internal-rc-*` 标签触发）+ 部署验证清单（见第五节）

已记录的取舍：管理员调整余额不再留业务流水（HTTP 审计日志兜底）；ent 中 payment/redeem
等死实体与历史迁移暂保留，统一在数据库压平批次处理。

### 批次 1 收口事实（2026-08-27）

- **已合并 main**：merge 提交 `e11441d1`（"Merge: 内部部署裁剪批次 1 合入 main"）
- **已发版 v0.1.182**：auto-release 自动发版，`backend/cmd/server/VERSION` 同步提交 `09dad102`
- **GHCR 镜像**：`latest` / `0.1.182` / `main` 三个标签指向**同一份双架构（amd64 + arm64）镜像**
- **三轮内网部署验证全部通过**：空库全新部署（`internal-rc-058f555`）→ 存量库升级 + 修复轮复验
  （`internal-rc-6a1452d`）→ 版本号回归修复复验（`internal-rc-be45516`），过程见第六节
- **第六节验证问题清单 0 项未决**，批次 1 的部署验证到此收口

---

## 三、批次 2（实施中）与后续 backlog

### 批次 2 首发范围与进度

本批首发 = **B1 + B2 + B3 + B4 + A1 尾款**，多 Agent 并行实施中。

| 工作包 | 内容 | 状态 |
|---|---|---|
| B1 | 移除登录条款/合规确认 | ✅ 已交付，待 RC 验证 |
| B2 | 移除通用设置冗余项 | ✅ 已交付，待 RC 验证 |
| B3 | 注册体系彻底移除 | ✅ 已交付，待 RC 验证（⚠️ 破坏性） |
| B4 | 人机验证全部移除 | ✅ 已交付，待 RC 验证 |
| A1 尾款 | install.sh 的 update/rollback 子命令、`deploy/docker-compose*.yml`、`.env.example`、`deploy/README.md` 的过时"在线更新"注释 | ✅ 已交付（A1 应用内部分批次 1 已完成） |

**CI 状态**：`3b7b3c39` 的 CI 四个 job（shell / test 含 Unit+Integration / frontend / golangci-lint）
与 Security Scan 全部 success。下一步：出 `internal-rc-3b7b3c3` 镜像做 way-rc 验证。
| B5 | 邮件体系收敛 | ⏸️ **本批不实施**，调研结论见下方 B5 专节，待站长拍板 |
| D | 本文档（TRIM_PLAN.md）更新 | ✅ 已交付 |

#### 批次 2 交付明细（代码已合流，CI 待确认）

- **B3 注册体系**：注册路由/页面/开关/邮箱验证注册流/注册专用默认值链路全删；`send-verify-code` 经核实只服务注册（TOTP 与邮箱绑定各有独立通道）一并删除。**管理员后台建用户能力保留**——它与注册体系并非深耦合，只共享 `GetDefaultBalance`/`GetDefaultSubscriptions` 两个通用访问器；存量是多用户场景，管理员仍需管理这些用户。
- **`email_verify_enabled` 键保留**：它另有三处非注册消费者（找回密码的三重与门、TOTP 自助开关的验证方式判断、邮件配置面门控），删键会静默改变保留面语义。仅摘掉注册分支，三处行为逐字未变。副作用已处理：种子探针原用 `registration_enabled` 判断"是否已初始化"，已换为同批种子键。
- **B4 人机验证**：Turnstile/腾讯/阿里三家服务、设置键、前端组件全删；登录与找回密码**路径保留，只摘掉验证码校验**。
- **B1 合规确认**：`AdminComplianceGuard` 中间件、`/admin/compliance` 路由、`docs/legal/`、`/legal/:documentId` 页面、登录页勾选与弹窗全删。admin 中间件链现为 `adminAuth → 限流 → 审计`，首次部署登录后台不再有 423 拦截。
- **B2 站点设置**：实际删除 11 个设置键（`site_name`/`site_subtitle`/`api_base_url`/表格设置 2 键/`contact_info`/`site_logo`/`home_content`/`compact_home_enabled`/`hide_ccs_import_button`/`custom_menu_items`）。有真实消费者的收敛为固定常量（后端 `service.SiteName`、前端 `constants/site.ts`，均为 `Sub2API`），并已验证删除 HTML 注入后标题与图标表现逐字不变——未重蹈批次 1 版本号空值的覆辙。连带删除 CSP `frame-src` 动态注入的整条死代码链（`home_content`/`custom_menu_items` 是其唯二来源）。
- **A1 尾款**：`install.sh` 删 update/rollback/list-versions 子命令（**保留首次安装能力**——下载是脚本获取二进制的唯一来源），顺带修了覆盖运行中二进制的 ETXTBSY 缺陷；`docker-deploy.sh` 删除从公网拉编排文件的逻辑改用本地文件（避免"部的提交与拉的编排不是同一份"的静默漂移），顺带修了误覆盖 git 跟踪文件的隐患；compose/`.env.example`/README/config.example 的"在线更新"注释改写为准确描述（`update.proxy_url` 仍存活，服务于定价表拉取与 Codex 版本同步，键名沿用）。
- **镜像坐标内部化**：三个 compose 与 Apple container 路径的默认镜像从上游 `weishaw/sub2api:latest` 改为 `ghcr.io/cherrylover/sub2api:latest`（原先照仓库编排部署会静默拿到**未裁剪的上游镜像**）；两个 `Dockerfile` 的 OCI `image.source` 标签改指本仓库（goreleaser 四个镜像变体里只有一个显式注入该标签，其余继承 Dockerfile，故须在 Dockerfile 侧修正）。`DOCKER.md` 改写而非删除——它被 `release.yml` 的 DockerHub description 步骤引用，删除会留悬空引用。

合流后统一验证 → CI 全绿 → 出 `internal-rc-<short_sha>` 镜像 → way-rc 验证 → 通过后才合并 main
（原因见第一节 ⚠️ 生产影响）。

#### ⚠️ 生产真实数据规模（B3 的硬约束背景）

站长确认的生产 `way.flyooo.uk` 现存数据：

| 维度 | 数量 |
|---|---|
| 用户 | **3 个（不止管理员，另有 2 个普通用户）** |
| API Key | 9 个 |
| 上游账号 | 3 个 |
| 分组 | 4 个 |
| 调用记录 | 3988 条 |

**B3 必须保证**：

- 存量 **3 个用户全部仍能登录**（删的是"注册入口"，不是"非管理员用户"）——
  不得出现"只留管理员、其余用户被禁用/删除"的实现；
- 存量 **9 个 API Key 仍能正常转发**——Key 归属的用户若被动了，转发链路会连带失效；
- 迁移脚本不得删除 users 表数据，不得对存量用户做角色/状态强制改写。

即 B3 的边界是"注册这条**路径**整体消失"，而不是"多用户**数据**整体消失"。

### B 组：首轮部署体验反馈（2026-08-26，站长实测提出）

| 编号 | 内容 | 备注 |
|---|---|---|
| B1 | **移除登录条款/合规确认** | 首次打开的"我已同意"确认整套删除：`AdminComplianceGuard` 中间件、`/admin/compliance` 路由、`docs/legal/`（admin-compliance 等文档）、`/legal/:documentId` 前端页、登录页条款勾选 |
| B2 | **移除通用设置冗余项** | 站点名称、站点副标题、API 端点地址、通用表格设置、联系人、Logo、首页内容、简洁首页、隐藏 CSS/CCM 导入按钮、自定义映射页面等站点品牌/运营面设置（实施时以 SettingsView 实际条目为准逐项核对，程序内部对站点名等的引用改为固定值） |
| B3 | **注册体系彻底移除** | 不再是"默认关闭"而是整体删除：注册开关、注册路由/页面、邮箱验证码注册流、注册用户默认值（signup 授予/authSourceDefaults 残余）。只保留管理员账号访问；管理员后台建用户能力去留在实施时一并评估。⚠️ 存量数据约束见上方"生产真实数据规模" |
| B4 | **人机验证全部移除** | Turnstile/腾讯/阿里三家验证码整套删除（批次 1 曾保留为休眠态，实测确认无需要：自用+API 调用场景无人机验证需求） |
| B5 | **邮件体系收敛** | 注册/验证类邮件随 B3 删除。⚠️ SMTP、ops 告警邮件、余额/配额提醒邮件的去留**待站长拍板** —— 调研已完成，结论与候选方案见下方 **B5 专节** |

**B 组明确保留**：管理员 Passkey、TOTP/2FA、敏感操作二次确认（step-up）、自定义登录入口路径、默认主页设置。

### A 组：既有 backlog（顺序建议）

| 编号 | 内容 | 对应原阶段 |
|---|---|---|
| A1 | 🔵 **尾款批次 2 实施中**。核心已落地（修复轮：应用内更新检查/在线升级/回滚整套已删，版本号展示与 restart 保留；`update.proxy_url` 因定价表与 Codex 同步仍消费而保留原键名）。尾款范围：install.sh 的 update/rollback 子命令、docker-deploy.sh、`deploy/docker-compose*.yml`/`.env.example`/`deploy/README.md` 过时注释。auto-release.yml 经站长决策**保持现状不动** | 阶段 1 |
| A2 | 公告、模型广场、邮件营销面删除；渠道监控 V1/批量生图/内容安全审计**去留决策后执行**（倾向：留监控 V2 删 V1；删批量生图与内容审计） | 阶段 6 |
| A3 | 周额度改自然周（周一 00:00）锚点 | 阶段 3 |
| A4 | 订阅/余额拆除与 Key 额度直绑改造（最难一刀，单独里程碑） | 阶段 2/6 |
| A5 | 数据库压平：删除死实体（payment/redeem/promo/subscription_plan 等）、迁移基线重置、支持全新初始化 | 阶段 2 |
| A6 | 管理后台单管理员化（Keys/用量页直管语义） | 阶段 5 |
| A7 | 部署文档改写（README/DEV_GUIDE 内部化）+ 压测、备份恢复、Key 泄露演练 | 阶段 7 |

排序建议：B1–B4 与 A1 合并为批次 2 首发（均为低耦合删除，体验收益立竿见影）→ A2 → A3 → A4 → A5 → A6 → A7。B5 待决策后并入。

---

### B5 邮件体系决策点（调研完成，本批不实施，待站长拍板）

以下为实际读码后的结论，供站长在最后的"候选方案"里勾选一个。**本批不动任何邮件相关代码。**

#### B5-1 SMTP 发送器现状：全系统唯一的出站推送通道，且默认休眠

`backend/internal/service/email_service.go`（644 行）+ `email_queue_service.go`（144 行）：

- `SendEmail(ctx, to, subject, body)` 读 DB 里 `smtp_host` / `smtp_port` / `smtp_username` /
  `smtp_password` / `smtp_from` / `smtp_from_name` / `smtp_use_tls` 七个设置键直连 SMTP。
  **`smtp_host` 为空即返回 `ErrEmailNotConfigured`**，所有调用方都是 best-effort 吞错，
  不影响转发主链路。
- 队列 `EmailQueueService` 只承载两种任务：`EnqueueVerifyCode`、`EnqueuePasswordReset`。
- **全局总闸**：SMTP 配置面（系统设置 → 邮件标签页）在前端整体 `v-if="form.email_verify_enabled"`
  （`frontend/src/views/admin/SettingsView.vue:6000`），即 `email_verify_enabled` 一关，
  连配置入口都不显示。该键 seed 默认值为 `"false"`（`backend/internal/service/setting_parse.go:51`）。

> **结论一**：默认部署下整个邮件子系统是**休眠**的，一封邮件都发不出去。
> 站长需自查生产是否曾手动打开 `email_verify_enabled` 并配好 SMTP —— 这决定了"删除"到底损失什么。

> **⚠️ 结论二（决策关键）**：全仓**不存在任何其他外发推送通道** ——
> `webhook` / 企微 / 钉钉机器人 / Telegram / Bark 在 service 层零命中。
> 删 SMTP 等于系统**彻底失去"主动推送"能力**，一切告警只能靠登录面板去看。

#### B5-2 `notification_email_service.go` 的 13 个事件清单

（常量定义见该文件 22–35 行，注册顺序见 1023–1037 行）

| # | 事件键 | 生产者 | 判定 |
|---|---|---|---|
| 1 | `auth.verify_code` | `email_service.go:346`，三个上游入口：①注册验证码流 ②`auth_email_binding.go:137` 绑定登录邮箱 ③`totp_service.go:560` TOTP 邮箱验证码 | 🟡 **半死** —— ① 随 B3 死；②③ 见 B5-3 |
| 2 | `auth.password_reset` | `email_service.go:516`（忘记密码） | 🟢 存活但默认关闭 —— ⚠️ 见 **B5-4** |
| 3 | `notification_email.verify_code` | `user_service.go:1096`，`/profile` 添加"通知邮箱"时验证 | 🟡 **仅服务于 #6**，#6 删则一起死 |
| 4 | `subscription.purchase_success` | **全仓无生产者** | 🔴 **死事件**（支付/订阅购买批次 1 已删） |
| 5 | `subscription.expiry_reminder` | `subscription_expiry_service.go:199` | 🟢 存活，但随 **A4** 订阅拆除而死 |
| 6 | `balance.low` | `balance_notify_service.go:360` ← 网关计费 `gateway_usage_billing.go:473` | 🟢 存活 —— 见 **B5-5** |
| 7 | `balance.recharge_success` | **全仓无生产者** | 🔴 **死事件**（充值批次 1 已删） |
| 8 | `account.quota_alert` | `balance_notify_service.go:415` ← `gateway_usage_billing.go:514` | 🟢 存活，**清单里对内部部署最有运维价值的一条** —— 见 B5-5 |
| 9 | `content_moderation.violation_notice` | `content_moderation.go:1980` | 🟢 存活，但 **A2** 倾向删内容审计 → 随之死 |
| 10 | `content_moderation.account_disabled` | `content_moderation.go:2005` | 同上 |
| 11 | `content_moderation.cyber_policy_notice` | `content_moderation.go:3087` | 同上 |
| 12 | `ops.alert` | `ops_alert_evaluator_service.go:723` | 🟢 存活 —— 见 **B5-6** |
| 13 | `ops.scheduled_report` | `ops_scheduled_report_service.go:360` | 🟢 存活，**默认全关** —— 见 B5-6 |

**净结论**：13 个事件里 **2 个已经是死事件**（#4 #7）、**3 个随 A2 死**（#9–#11）、
**1 个随 A4 死**（#5）、**1 个随 B3 半死**（#1 的注册分支）。
真正需要站长现在拍板的只有 **4 条**：`auth.password_reset`、`balance.low`、
`account.quota_alert`、`ops.alert`（含 `ops.scheduled_report`）。

#### B5-3 ⚠️ TOTP / 2FA 对 SMTP 的依赖：**登录完全不受影响**

**结论先行：删 SMTP 不会影响 2FA 登录，一次都不会。**

证据链：

- 登录时的 2FA 校验走 `TotpService.VerifyCode`（`backend/internal/service/totp_service.go:332`），
  实现是纯算法 `totp.Validate(code, secret)` + Redis 失败计数，**完全不触碰 emailService**。
  敏感操作 step-up（`VerifyStepUp`，同文件 408 行）复用同一函数，同样不碰邮件。
- 任务书点名的 `totp_service.go:158` 那处 `emailService.VerifyCode`，位于 `verifyIdentity`，
  调用方**只有两处**：`InitiateSetup`（首次开启 2FA，187 行）与 `Disable`（关闭 2FA，319 行）——
  **只在"开关 2FA"时做身份复核，不在登录路径上**。
- 且该分支**对管理员天然不可达**：
  `usesEmailVerification`（`totp_service.go:148`）= `user.Role != RoleAdmin && IsEmailVerifyEnabled(ctx)`。
  源码注释已写明"管理员一律使用密码校验"（理由：管理员邮箱常为占位地址收不到码）。
  **管理员开/关 2FA 永远走密码，不走邮箱验证码。**

**残余影响面**：仅"非管理员用户 **且** `email_verify_enabled=true`"时，普通用户**自助开/关 2FA**
需要邮箱验证码。生产 3 个用户里有 2 个非管理员，若该开关曾被打开则落入此情形。

**处置代价近乎为零**：删 SMTP 时把 `verifyIdentity` 的邮箱分支删掉、统一走密码校验即可
（同时删 `GetVerificationMethod:532` 的分支与 `TotpService.SendVerifyCode:544`），
结果与现有管理员行为完全一致。

#### B5-4 ⚠️ 忘记密码对 SMTP 的依赖：**是唯一的自助找回通道，但默认本就关闭**

**结论先行：删 SMTP 会让管理员彻底失去"自助找回密码"的通道；
但该通道在默认配置下本来就是关闭的（403）。**

依赖链是**三重与门**（`auth_service.go:1046`、`setting_features.go:65`）：

```
IsPasswordResetEnabled = email_verify_enabled==true
                      && password_reset_enabled==true
                      && SMTP 实际可用
```

任一不满足，`POST /api/v1/auth/forgot-password` 与 `/reset-password` 直接返回
403 `PASSWORD_RESET_DISABLED`，登录页的"忘记密码"链接也不渲染
（`frontend/src/views/auth/LoginView.vue:73`，受 `passwordResetEnabled` 控制）。
**两个设置键的 seed 默认都是 `"false"`。**

> ⚠️ **站长需自查**：生产 `way.flyooo.uk` 若历史上打开过这两个开关且 SMTP 可用，
> 当前确实存在一条可用的找回通道，删 SMTP 会失去它；
> 若从未打开（默认态），删除 = **零损失**，只是删掉一条本来就返回 403 的路由。

管理员被锁在门外时的**现存兜底**（删 SMTP 后依然有效）：

- 后台"用户管理"可直接改任意用户密码（`UpdateUserInput.Password`，`admin_service.go:146`）——
  但这**要求已经登录**，救不了管理员自己；
- Passkey / TOTP 是**独立登录因子**，管理员若已绑定 Passkey，其本身就是一条不依赖邮件的兜底入口；
- `ADMIN_PASSWORD` 环境变量**只在 AUTO_SETUP 首次引导时生效**，存量库会打印
  "skipping auto admin bootstrap"，**不能用来重置**；
- **无 CLI 重置工具** —— `backend/cmd/` 下只有 `jwtgen` / `profit-preview` /
  `cleanup-ingress-reject-logs` / `server`；
- 因此真正的最后兜底是**直连 Postgres 改 `users.password_hash`**：内部部署下站长本就有库权限，
  可行，但需要自己算 bcrypt。

**建议（若选择删 SMTP）**：同批补一个 `backend/cmd/` 下的管理员密码重置小工具
（约 50 行，读 `DATABASE_*` 环境变量 + bcrypt 写库）。成本极低，
且把"找回密码"从"依赖外部 SMTP 服务"降级为"依赖宿主机 shell 权限"——
对单管理员内部部署而言反而是更合适的信任模型。

#### B5-5 余额 / 配额提醒邮件（`balance_notify_service.go`，555 行）

两个事件，触发点都在网关计费收尾（`gateway_usage_billing.go:454` / `:495`，异步 best-effort，
失败只打日志不影响转发）：

**`balance.low`（用户余额不足）**

- 用在哪：用户余额扣款后跌破阈值 → 发给用户本人 + 其在 `/profile` 绑定的"通知邮箱"。
- **已经是残缺功能**：邮件正文含 `recharge_url`（充值页）变量，
  而**充值功能批次 1 已整套删除**，这个链接现在指向不存在的页面。
- 删掉失去什么：内部部署下几乎不失去什么 —— 余额语义本身就在 **A4** 的拆除范围内，
  本事件生命周期本就到 A4 为止。
- 面板内替代：`/admin/users` 与 `/admin/usage` 可直接看余额，管理员随时可调。

**`account.quota_alert`（上游账号额度告警）**

- 用在哪：**上游账号**（Claude Console API Key / Bedrock 类）的日 / 周 / 总额度用量
  越过提醒阈值 → 发给管理员邮箱。
- 删掉失去什么：**这是清单里对内部部署唯一有实际运维价值的一条** ——
  上游 API Key 快撞额度上限时的提前知会。
- 面板内替代（**部分**）：`/admin/accounts` 有额度用量展示与"重置额度"操作，
  但**是拉取式的，需要人主动去看**；ops 告警规则的 `metric_type` 面向请求侧指标
  （错误率 / 延迟等），**不覆盖上游账号额度维度**。
  即：删掉后信息仍可查，但失去"到阈值主动提醒"，需要站长自己养成看账号页的习惯。

#### B5-6 ops 告警邮件（`OpsAlertEvaluatorService` / `OpsScheduledReportService`）

**`ops.alert`** —— `ops_alert_evaluator_service.go:678 maybeSendAlertEmail`

- 用在哪：告警规则命中 → 生成 `OpsAlertEvent` **落库** → 若规则 `NotifyEmail` 为真、
  ops 邮件配置启用、收件人非空 → 经 `MinSeverity` 过滤 + 静默窗口 + 每小时限流后发邮件，
  成功后回写 `UpdateAlertEventEmailSent`。
- **✅ 面板内有完整替代**：**告警事件独立于邮件落库，邮件只是附加推送通道**。
  后台 ops 面板有 `frontend/src/views/admin/ops/components/OpsAlertEventsCard.vue`，
  配套接口 `GET /api/v1/admin/ops/alert-events`、`GET .../alert-events/:id`、
  `PUT .../alert-events/:id/status`，可列表 / 看详情 / 改处理状态。
  **删邮件不丢任何数据，只丢"主动推送"这一形态。**
- 默认态：`Alert.Enabled=true` 但 `Recipients=[]`（`ops_settings.go:112-114`），
  收件人为空直接 return —— **默认不发**。

**`ops.scheduled_report`** —— `ops_scheduled_report_service.go`（825 行 + cron + Redis 领导者锁）

- 用在哪：日报 / 周报 / 错误摘要 / 账号健康四类定时报表，cron 触发，
  渲染 HTML 塞进邮件正文发给收件人（收件人为空时回退到第一个管理员邮箱）。
- **❌ 面板内无对等替代**：报表内容**不落库、不留存档**，是纯推送产物。
  但其**数据源全部来自 ops 面板已有的 overview / trends**，
  信息本身在面板里查得到，丢的是"定时汇总送到邮箱"这个形态。
- 默认态：`Report.Enabled=false`，四个子报表也全部 `false`（`ops_settings.go:121-131`）——
  **默认完全不跑**。
- 附带收益：删掉可一并去掉 cron 调度 + Redis 领导者锁这套机制（825 行）。

#### B5-7 候选方案（请站长勾选一个）

> 说明：以下 checkbox 是**决策项**，不是验证项 —— 站长勾选即视为拍板，
> 与第六节"修复后须在新 RC 镜像验证通过才勾"的规则无关。

- [ ] **方案 A：整删 SMTP 与全部邮件体系**
  - 删除范围：`email_service.go`(644) + `email_queue_service.go`(144) + `email_message.go`(111)
    + `notification_email_service.go`(1658) + `notify_email_entry.go`(81)
    + `balance_notify_service.go`(555) + `content_moderation_email.go`(155)
    + `auth_email_binding.go`(342) + 前端 `EmailTemplateEditor.vue`(724)
    + 7 个 `smtp_*` 设置键 + 系统设置的整个"邮件"标签页 + 13 个事件模板
    ≈ **4400+ 行**，是批次 2 之后单包体量最大的一刀
  - 代价：① **系统彻底失去主动推送能力**（无 webhook 等替代通道），ops 告警 / 上游额度告警
    只能靠登录面板主动查看；② **失去自助找回密码**（默认态下本就是 403，见 B5-4）；
    ③ 需顺带把 TOTP 的 `verifyIdentity` 邮箱分支改为统一走密码（代价近乎为零，见 B5-3）
  - 建议配套：补 `backend/cmd/` 管理员密码重置小工具（约 50 行）
  - 影响面：**不影响 2FA 登录、不影响转发链路、不影响存量用户与 Key**

- [ ] **方案 B：只保留"找回密码 + ops 告警"（最小保留）**
  - 保留：`email_service.go` 的 SMTP 发送器 + `auth.password_reset` + `ops.alert`
    + `account.quota_alert`（建议一并留，见 B5-5）
  - 删除：注册/绑定验证码流（随 B3）、`subscription.*`、`balance.*`、`content_moderation.*`、
    `ops.scheduled_report`、`notification_email.verify_code`，以及整个模板编辑器
    （只留 3–4 个内置模板，不再提供自定义模板 UI）≈ 删 **2500+ 行**
  - 代价：仍需维护一套 SMTP 配置与连接代码；站长必须**真配一个可用的 SMTP**，
    否则保留下来的两条也发不出去，等于白留
  - 收益：保住"密码找回"与"告警主动推送"两条对单管理员最实用的能力

- [ ] **方案 C：全保留，只随 B3 / A2 / A4 删除已死事件**
  - 本批只删注册相关邮件（随 B3），其余原样不动；
    `subscription.purchase_success` / `balance.recharge_success` 两个**已经无生产者的死事件**
    顺手清掉；`content_moderation.*` 留待 A2、`subscription.expiry_reminder` 留待 A4 自然消亡
  - 代价：邮件子系统（约 4400 行）继续存在于代码库中，与"内部部署裁剪"的目标背道而驰；
    系统设置里继续保留"邮件"标签页与模板编辑器等运营向 UI
  - 收益：**零风险**，不触碰任何认证与告警路径，决策可以推迟到 A2 / A4 时再做

> **调研方倾向（仅供参考，最终以站长勾选为准）**：
> 若生产从未打开 `email_verify_enabled`（默认态），**方案 A + 补密码重置小工具**性价比最高——
> 删掉的全是从未生效的代码，唯一真实损失（主动推送）在单管理员自用场景下影响有限。
> 若站长确实在用 ops 告警邮件或希望保留自助找回密码，则选**方案 B**。
> **方案 C 适合"本批不想再引入任何变量"的保守选择。**

---

## 四、遗留与技术备注

- 4 个基线遗留的未格式化 unit 测试文件（golangci-lint 未配 build-tags 检不到；补配置前先 gofmt）
- `.gitignore` 对 `docs/*` 为白名单模式，docs 新文件需 `git add -f`
- 本会话 GitHub 集成无 actions:write，workflow 触发走 `internal-rc-*` 标签推送
- 迁移 229 为幂等 UPDATE；本轮未删表未改列，镜像回滚无兼容性问题

### ⚠️ 验证盲区：Docker 门控的集成测试本地会静默跳过

全仓 3 处 testcontainers 集成测试都带同款「无 Docker 则 skip」守卫，在没有
`/var/run/docker.sock` 的开发/沙箱环境里会**静默跳过而非失败**，此时"本地集成测试通过"
对这三处不构成任何证据，必须以 CI 结论为准（或用 `miniredis` 走旁路复现）：

- `backend/internal/server/routes/auth_rate_limit_integration_test.go`（批次 2 踩中：register 路由删除后本地全绿、CI 报 404）
- `backend/internal/middleware/rate_limiter_integration_test.go`
- `backend/internal/repository/integration_harness_test.go`

后续批次凡动到**限流、认证入口、repository 层**，务必留意这条。

### ⚠️ CI 全绿覆盖不到 Docker 镜像构建

`backend-ci.yml` 只跑 `deploy/tests/` 下的 compose/runtime 脚本，**从不实际构建镜像**。
因此"CI 全绿"无法证明镜像出得来——「删文件后 Dockerfile 构建上下文引用悬空」这类问题
只有真正出 RC 镜像时才暴露。批次 2 踩中：WP-B 删了 `docs/legal/`，但
`Dockerfile` / `deploy/Dockerfile` 的 `COPY docs/legal/` 与 `.dockerignore` 的白名单例外
都没同步清理，而 Docker 的 `COPY <dir>/` 在源缺失时是硬失败，镜像构建从该提交起必然挂。

**后续批次删除任何目录/文件后，务必 grep 构建上下文**：`Dockerfile`、`deploy/Dockerfile`、
`.dockerignore`、`.goreleaser.yaml`。

### ⚠️ `//go:build embed` 的文件在 CI 里无人检查（已加防护）

`backend/internal/web/embed_on.go` 带 `//go:build embed`，而 CI 的三种编译
（`-tags=unit` / `-tags=integration` / golangci-lint 不配 build-tags）**全都不带该 tag**，
等于这个文件谁都没编译过；只有 Dockerfile 用 `-tags embed` 构建生产镜像时才会编到。
批次 2 因此连挂两轮镜像构建：WP-B 删掉站点名/图标注入代码后遗留两个未使用 import，
CI 全绿但镜像构建必然失败。

**已加防护**：`backend-ci.yml` 的 test job 新增 `Embed tag build check` 步骤
（桩 dist 目录 + `go build -tags embed ./...`），编译错误现在会在 CI 阶段就暴露。
注：未改 golangci-lint 的 build-tags，因为换 tag 会反过来漏掉 `embed_off.go`。

### ⚠️ CI 的 lint 输出不是完整清单

`golangci-lint` 默认 `max-same-issues=3`，同类问题只报前 3 个。批次 2 首轮 CI 报 3 个 gofmt，
实际有 6 个——只照 CI 输出修会再红一轮。正确做法是用 CI 同版本二进制本地跑
`--max-same-issues=0 --max-issues-per-linter=0 ./...` 拿全量清单。

另注：`unused` 检查不带 build tag 编译，因此**仅被 `//go:build unit` 测试引用的死函数会被判定为 unused**
（单测绿但 lint 红）。批次 1 与批次 2 各栽过一次，删除批量代码后需专门留意。

### 批次 2 实施中记录的后续候选项（均未处理，待排期）

- **CSP 白名单失效条目**：`security_headers.go` 仍保留腾讯天御的 script-src/frame-src/connect-src/worker-src 白名单，批次 1 同样留下了 Stripe/Airwallex 条目。对应 SDK 已删，这些条目现在只是无谓放宽策略，建议作为安全侧收敛项统一清理
- **`registration_email_domain_quota_enabled`**：注册删除后已无内部消费者，但仍被 admin 设置页读写往返；其兄弟键 `registration_email_suffix_whitelist` 仍被邮箱绑定的校验路径使用，两者在设置页属同一区块，需一并评估
- **`LOGIN_ENTRY_RESERVED_PREFIXES` 的 `/legal`、`/custom`**（前端 SettingsView 与后端 `config/web_entry.go` 镜像）：对应路由已删，但**刻意保留**——这是自定义登录入口的保留字黑名单，删条目只会放宽允许集，属安全收紧项而非裁剪项。若要收窄需前后端同步改并调整 `web_entry_test.go` 用例
- **支付遗留文档**：`docs/PAYMENT.md`、`docs/PAYMENT_CN.md`、`docs/ADMIN_PAYMENT_INTEGRATION_API.md` 描述的功能已在批次 1 删除
- **`custom_endpoints` 设置**：与已删的 `api_base_url` 在设置页相邻但语义不同，本批未动，待站长确认是否一并删除
- **ent 中的支付实体**（`payment_orders`/`payment_provider_instances` 等）与相关历史迁移：需 ent 重生成，归入 A5 数据库压平

---

## 五、内部 RC 镜像部署验证清单


本清单面向站长，用于在内网实际部署验证"单管理员内部部署"裁剪分支的镜像。
镜像由 `.github/workflows/branch-docker-image.yml`（手动触发）构建，只推 GHCR：

- 浮动标签：`ghcr.io/cherrylover/sub2api:internal-rc`（每次分支构建会顶掉，验证期 compose 固定引用它）
- 不可变标签：`ghcr.io/cherrylover/sub2api:internal-<short_sha>`（定位/回滚到具体提交用）

验证内容分三块：空库全新部署、裁剪面验证、老库升级演练。全部通过后把结论告知协调方即可，
合并 main 与 auto-release 由协调方执行。

> **⚠️ 本清单当前是批次 1 的验收基线。** 批次 2 合流后需按实际交付更新裁剪面预期
> （已知会变的：第 4.1 条注册接口的预期从 403 翻转为 404，见该条内注），
> 并新增 B1/B2/B4 的裁剪面探测与"存量 3 用户 + 9 个 Key 仍可用"的回归项。
> 另注意第一节的 ⚠️ 生产影响：**批次 2 必须先在 way-rc 验证通过才能合并 main**。

---

### 1. 拉取镜像

```bash
docker pull ghcr.io/cherrylover/sub2api:internal-rc
```

如需固定到具体提交（构建完成后 workflow 的 Summary 页会列出 short_sha）：

```bash
docker pull ghcr.io/cherrylover/sub2api:internal-<short_sha>
# 例：docker pull ghcr.io/cherrylover/sub2api:internal-5299564
```

若 GHCR 上该包是私有的，先用 GitHub PAT（需 `read:packages` 权限）登录：

```bash
echo '<你的PAT>' | docker login ghcr.io -u <你的GitHub用户名> --password-stdin
```

预期结果：拉取成功；`docker image inspect` 可看到内部版本标识（绝不是 v 开头的正式版本号）：

```bash
docker image inspect ghcr.io/cherrylover/sub2api:internal-rc \
  --format '{{ index .Config.Labels "org.opencontainers.image.version" }}'
# 预期输出形如：internal-<分支短名（tag-safe 处理后）>-<short_sha>，绝不是 vX.Y.Z
```

---

### 2. 空库全新部署

以仓库 `deploy/` 目录的 Compose 编排为基础。`deploy/docker-compose.yml` 内置三个服务：
`sub2api`（应用）、`postgres`（postgres:18-alpine）、`redis`（redis:8-alpine），
Postgres 与 Redis 不暴露宿主机端口，仅应用暴露 `${BIND_HOST:-0.0.0.0}:${SERVER_PORT:-8080}:8080`。

#### 2.1 修改镜像坐标

把 `deploy/docker-compose.yml` 中 `sub2api` 服务的镜像行：

```yaml
    image: ghcr.io/cherrylover/sub2api:latest   # 批次 2 起仓库默认值已是内部镜像
```

改为内部镜像：

```yaml
    image: ghcr.io/cherrylover/sub2api:internal-rc
```

#### 2.2 准备环境变量

```bash
cd deploy
cp .env.example .env
chmod 600 .env
```

最小必配/强烈建议项（其余保持 `.env.example` 默认即可）：

| 变量 | 必要性 | 说明 |
| --- | --- | --- |
| `POSTGRES_PASSWORD` | **必配** | compose 里写作 `${POSTGRES_PASSWORD:?}`，不配直接启动失败 |
| `JWT_SECRET` | 强烈建议 | 留空则每次启动随机生成，容器重启后所有登录会话失效。生成：`openssl rand -hex 32` |
| `TOTP_ENCRYPTION_KEY` | 强烈建议 | 留空则每次启动随机生成，已配置的 TOTP 全部失效。生成：`openssl rand -hex 32` |
| `ADMIN_EMAIL` | 建议 | 管理员邮箱，默认 `admin@sub2api.local` |
| `ADMIN_PASSWORD` | 建议 | 留空则首次启动自动生成一次性密码并打印到容器日志（只显示一次） |
| `TZ` | 建议 | 默认 `Asia/Shanghai`。影响数据库时间戳、用量统计"今天"边界、订阅到期时间与日志时间，部署后不要再改 |

说明：compose 中已硬编码 `AUTO_SETUP=true`，容器首次启动会自动完成建库迁移与管理员创建，
无需手动跑安装向导。数据库/Redis 连接指向 compose 内部网络（`DATABASE_HOST=postgres`、`REDIS_HOST=redis`），不需要改。

#### 2.3 启动并检查健康

```bash
docker compose up -d
docker compose ps
# 预期：sub2api / sub2api-postgres / sub2api-redis 均为 Up，sub2api 状态为 (healthy)
#（应用 healthcheck 即容器内 wget http://localhost:8080/health）

curl -s http://127.0.0.1:8080/health
# 预期输出：{"status":"ok"}

docker compose logs sub2api | grep -iE '"level":"error"|level=error' || echo "无 error 日志"
# 预期：无 error 日志（首次启动日志中应能看到 Auto setup completed successfully!）
```

若 `ADMIN_PASSWORD` 留空，从日志中取一次性密码（务必保存，只打印一次）：

```bash
docker compose logs sub2api | grep -i "Generated admin password"
```

---

### 3. 初始化验证

1. 浏览器打开 `http://<宿主机IP>:8080/`，预期跳到登录页。
   - 本 compose 走 `AUTO_SETUP` 自动初始化，管理员已在首次启动时创建，`/setup` 安装向导不再需要；
     初始化完成后访问 `/setup` 会因 `needs_setup=false` 自动跳回登录页（向导仅用于未开启
     `AUTO_SETUP` 的裸机安装场景，此时才会在 `/setup` 页面填库连接并创建管理员）。
2. 用 `ADMIN_EMAIL` + `ADMIN_PASSWORD`（或日志里的一次性密码）登录，预期进入管理后台。
3. TOTP（可选验证）：个人设置（`/profile`）里启用两步验证，退出后重新登录走 2FA 流程。
   启用前确认 `.env` 已固定 `TOTP_ENCRYPTION_KEY`，否则重启容器后 TOTP 将失效。
4. 进入 系统设置（`/admin/settings`），确认"允许注册"默认为**关闭**状态。
   接口层双确认：

```bash
curl -s http://127.0.0.1:8080/api/v1/settings/public | jq '.data.registration_enabled'
# 预期输出：false
```

---

### 4. 裁剪面验证

以下命令中 `BASE=http://127.0.0.1:8080`，逐条执行并核对预期。

#### 4.1 注册接口被开关拒绝（403，而非 404 —— 路由保留、默认关闭）

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X POST "$BASE/api/v1/auth/register" \
  -H 'Content-Type: application/json' \
  -d '{"email":"probe@example.com","password":"probe12345"}'
# 预期输出：403

curl -s -X POST "$BASE/api/v1/auth/register" \
  -H 'Content-Type: application/json' \
  -d '{"email":"probe@example.com","password":"probe12345"}' | grep -o REGISTRATION_DISABLED
# 预期输出：REGISTRATION_DISABLED
```

注意：该接口有每分钟 5 次的 IP 限流（Redis 故障时直接拒绝），连发过多会先撞 429，属正常。

> ⚠️ **批次 2 起本条预期翻转**：B3 把注册体系整体删除后，路由不再存在，
> **预期从 403 `REGISTRATION_DISABLED` 变为 404**。验证批次 2 的 RC 镜像时按 404 核对；
> 同时应确认存量 3 个用户仍能正常登录（B3 删的是注册路径，不是多用户数据）。

#### 4.2 已裁剪路由全部 404（支付 / 卡密 / 第三方 OAuth 登录）

```bash
for p in \
  /api/v1/payment/config \
  /api/v1/redeem/history \
  /api/v1/auth/oauth/linuxdo/start ; do
  printf '%-40s %s\n' "$p" "$(curl -s -o /dev/null -w '%{http_code}' "$BASE$p")"
done
# 预期：三行全部为 404
```

#### 4.3 公开设置不再泄露已裁剪功能键

```bash
curl -s "$BASE/api/v1/settings/public" | jq -e '.data
  | (has("payment_enabled")
     or has("linuxdo_oauth_enabled") or has("dingtalk_oauth_enabled")
     or has("wechat_oauth_enabled")  or has("oidc_oauth_enabled")
     or has("github_oauth_enabled")  or has("google_oauth_enabled"))
  | not'
# 预期输出：true（即上述键一个都不存在）
```

#### 4.4 登录页无第三方登录按钮

浏览器打开 `/login`：只有 邮箱+密码 登录（以及可选的 Passkey 登录入口），
无 LinuxDo / 微信 / 钉钉 / OIDC / GitHub / Google 任何第三方按钮。

---

### 5. 核心链路验证（上游账户 → 分组 → API Key → 转发 → 用量）

1. **添加上游账户**：后台 `/admin/accounts`，按实际平台（Claude/Codex/Gemini 等）添加一个可用账户，
   测试连通性通过。
2. **建分组**：`/admin/groups` 新建分组，选择对应平台并把上一步的账户挂进去。
3. **建 API Key**：`/keys` 页面（管理员自己的 Key 页）新建一个 Key，选择上述分组，复制生成的 `sk-...`。
4. **真发一条转发请求**（以 Claude 平台分组为例，模型名按分组实际可用模型填；
   其他平台按实际入口端点替换）：

```bash
curl -s http://127.0.0.1:8080/v1/messages \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer sk-你的Key' \
  -d '{"model":"claude-sonnet-4-5","max_tokens":64,"messages":[{"role":"user","content":"ping"}]}'
# 预期：返回上游的正常应答 JSON（含 content 数组），而非 401/403/5xx
# 认证头也可用 x-api-key: sk-你的Key
```

5. **用量可见**：后台 `/admin/usage`（或用户侧 `/usage`）能看到刚才这条请求的记录
   （模型、token 数、时间与实际相符）。

---

### 6. 老库升级演练（如有存量库）

**用备份副本演练，不要动生产卷。** 另注意：`deploy/docker-compose.yml` 固定了容器名
（`sub2api` / `sub2api-postgres` / `sub2api-redis`），同一台机器上无法与原环境同时双开，
请在另一台机器演练，或临时改掉副本编排里的 `container_name`。

```bash
# 1) 在旧环境导出备份
docker exec sub2api-postgres pg_dump -U sub2api -d sub2api > sub2api-backup.sql

# 2) 在演练环境（镜像已换成 internal-rc 的 compose）先只启动库并导入
docker compose up -d postgres redis
docker compose exec -T postgres psql -U sub2api -d sub2api < sub2api-backup.sql

# 3) 启动应用（首次启动会自动执行嵌入的 SQL 迁移，其中包含本轮新增的 229）
docker compose up -d sub2api
docker compose logs -f sub2api   # 观察启动无 error
```

逐项确认：

```bash
# 迁移 229 已执行（迁移记录在 schema_migrations 表）
docker compose exec postgres psql -U sub2api -d sub2api \
  -c "SELECT filename, applied_at FROM schema_migrations WHERE filename = '229_force_registration_disabled.sql';"
# 预期：返回 1 行

# registration_enabled 被强制置 false（即使老库曾开启注册）
docker compose exec postgres psql -U sub2api -d sub2api \
  -c "SELECT key, value FROM settings WHERE key = 'registration_enabled';"
# 预期：value = false
```

- 原有管理员账号可正常登录（存量库有用户数据时，AUTO_SETUP 会跳过管理员引导，不会覆盖原密码，
  日志中会出现 "skipping auto admin bootstrap"）；
- 原有分组 / API Key / 用量数据在后台完好可见；
- 用原有 API Key 按第 5 节方式真发一条请求，转发仍然成功，且新记录进入用量页。

---

### 7. 回滚路径

代码回滚 = 把 compose 里的镜像换回旧 tag，重启应用即可：

```bash
# 把 image 换成上一个可用的内部标签（如 ghcr.io/cherrylover/sub2api:internal-<旧 short_sha>
# 或已发版的 ghcr.io/cherrylover/sub2api:0.1.182）后：
docker compose up -d sub2api
```

- 迁移 229 是幂等 UPDATE（只把 `settings.registration_enabled` 置为 `'false'`），**不需要回滚**；
- 旧版本代码配"已跑过 229 的库"没有兼容性问题：本轮裁剪未删表、未改列，库结构对老代码完全可读写；
- 唯一可见影响：回滚后注册开关仍处于关闭状态，如需开放注册可在旧版本后台设置里重新打开。

---

### 8. 验证通过后的动作

把验证结论（通过项、失败项及现象）告知协调方即可。合并 main 与 auto-release
（正式版本号、`:latest` / `:main` 镜像）由协调方执行，无需站长操作。
`internal-rc` / `internal-<short_sha>` 只是验证期临时标签，合并发版后内网部署应切回正式 tag。

---

## 六、验证问题追踪（按镜像版本累加）


使用约定：

- 每轮部署验证对应一个镜像版本（`internal-<short_sha>`），在本节**最上方**为该版本新开一个小节，
  把验证发现的问题记成未勾选的 checkbox；
- 问题修复后，要在**修复后的新版本**上重新验证通过，才把对应项勾掉（勾选时可注明"于 internal-xxxxxxx 验证通过"）；
- 新版本验证出的新问题在新版本小节继续累加；旧版本小节中未勾掉的项即为仍未修复项，无需搬运；
- 与第三节（批次待办）的工作包有对应关系的项，标注了编号（如 B3/A1），
  该工作包交付即视为修复，按上一条流程验证后勾掉。

> 修复轮记录（2026-08-26）：本节 7 个待修项已全部交付修复（commit `b2875437`…`ea567951`）——
> 在线升级机制整删（A1 核心）、管理员初始余额过渡方案（A4 前）、注册入口随开关隐藏（B3 前过渡）、
> 登录/认证错误 i18n 补齐、profile 绑定文案、新手引导整删（含 driver.js 依赖）、上游 GitHub 链接移除
> （合规弹窗内法律文档"查看原文"链接刻意保留，随 B1 整删）。
> checkbox 待站长在新 RC 镜像上复验通过后勾选。

### internal-rc-be45516（2026-08-27，第三轮：版本号回归修复复验）

验证环境：同前（OCI 并行验证栈 `way-rc.flyooo.uk`）。
本轮通过项：上一轮版本号回归已修复，三层证据一致——接口 `settings/public.version`、
首屏注入 `window.__APP_CONFIG__.version`、后台侧边栏界面显示，均为 `internal-rc-be45516`
（修复前注入值为空串）；回归无退化——裁剪面 404/403 全部保持、4 条升级/回滚路由仍为 404
且保留项 401、存量数据（管理员/用户/分组/Key）完好、后台 7 个页面渲染正常、
浏览器控制台零错误、容器启动日志零 error；生产环境零影响。

修复要点（含一处根因订正）：版本号传递链路的后端环节（`SettingService.SetVersion` 无调用方）
**并非本轮重构引入，重构前与上游主干同样如此**；b2875437d 把侧边栏版本号从"单独调管理接口"
改为"只认首屏注入值"，才把这个长期存在的空值暴露成界面空白。因此修复同时补齐两端：
后端在装配阶段（`ProvideSettingHandler`）注入编译期版本号，前端在注入值为空时回落读接口。
已补防回归断言（后端 1 条 + 前端 3 条），并通过"注释掉修复确认断言失败"反向验证有效性。

本轮无新增待修项。**批次 1 的部署验证到此收口。**

### internal-rc-6a1452d（2026-08-26，第二轮：存量库升级部署 + 修复轮复验）

验证环境：同首轮（OCI 并行验证栈 `way-rc.flyooo.uk`）。本轮为**带数据升级部署**（沿用首轮库）。
本轮通过项：修复轮 7 项全部复验通过（见上一小节勾选）；升级平滑——无迁移需执行、存量数据
（管理员/用户/分组/Key/余额）完好、生产环境零影响；裁剪面接口回归无回退；4 条升级/回滚路由
确认从路由表删除（404）且保留项未误删（version/restart 探测 401）；空库预置余额经临时栈实测
生效（`users.balance = 1000000000.00000000`，接口与数据库双确认，临时栈已销毁无残留）；
后台全页面回归正常，控制台零 JS 报错。

本轮待修项：

- [x] **侧边栏版本号不显示（本轮新增回归）**：更新器移除重构后，`SettingService.SetVersion()`
      （backend/internal/service/setting_service.go）全仓无调用方，HTML 注入的
      `window.__APP_CONFIG__` 中 `version` 恒为空串；前端 store（frontend/src/stores/app.ts
      约 318 行）存在注入快照即短路返回，不再回落读 `/api/v1/settings/public` 的 `version`
      （接口实际有值 internal-rc-6a1452d）。结果侧边栏版本号区域为空占位。
      修复方向：启动装配处补一次 `SetVersion` 调用，或前端对注入值为空时回落接口值（于 internal-rc-be45516 验证通过）

附注（非缺陷，留观/提醒）：

- Codex 客户端版本自动同步是更新器重构后 GitHub release 客户端的唯一存活消费方，本轮仅验证到
  启动路径无报错；定时同步尚未触发，实跑效果待下轮部署时回看日志确认
- 注册入口反向验证采用非侵入方式（确认渲染条件绑定 `registration_enabled` 开关 + 开关实测为
  false），未实际开启公网环境的注册开关
- 部署编排注释滞后：`deploy/docker-compose*.yml`、`.env.example`、`deploy/README.md` 中仍有
  "在线更新"相关注释描述，`deploy/install.sh` 的 update/rollback 子命令未动（A1 尾款已在
  第三节记录，此处仅提醒）

### internal-rc-058f555（2026-08-26，首轮：空库全新部署 + 后台全页面深度验证）

验证环境：OCI 并行验证栈（`way-rc.flyooo.uk`，与生产隔离）。
本轮通过项：第 2–4 节全部通过；核心链路（Key 认证 → 余额 → 分组路由 → 模型白名单 →
调度/粘性会话 → 真实上游转发 → 401 故障转移 → 账号自动熔断 → 干净报错）用假 Key 实测打通；
后台 12 个页面 + 系统设置 8 个标签全部走查，浏览器控制台零 JS 报错；操作日志完整记录全程管理动作。

本轮待修项：

- [x] **全新部署开箱即卡余额**：管理员初始余额 $0，首条转发请求直接被 `INSUFFICIENT_BALANCE`（403）
      拒绝，必须先在用户管理里给自己充值才能使用。内部单管理员版本不应要求管理员自我充值。
      （对应 TRIM_PLAN A4 订阅/余额拆除；A4 落地前可考虑先做轻量过渡：AUTO_SETUP 时给管理员一个极大初始余额）（于 internal-rc-6a1452d 验证通过）
- [x] **登录页仍显示"注册"入口**：注册接口已 403、注册页已显示"暂时关闭"，但登录页底部
      "还没有账户？注册"链接未随注册关闭而隐藏。（B3 注册体系彻底移除后随之消失，验证时确认）（于 internal-rc-6a1452d 验证通过）
- [x] **登录失败提示未走 i18n**：中文界面下密码错误弹出英文原文 "invalid email or password"（于 internal-rc-6a1452d 验证通过）
- [x] **"有新版本可用！"在线升级提示仍在**：侧边栏与官方 Release 比对并提示升级，
      对内部版本有误导。（对应 TRIM_PLAN A1）（于 internal-rc-6a1452d 验证通过）
- [x] **个人设置残留第三方登录文案**：`/profile`"登录方式绑定"描述仍写
      "将更多第三方登录方式关联到这个账号"，实际仅剩邮箱一项（于 internal-rc-6a1452d 验证通过）
- [x] **新手引导仍是 SaaS 话术**：首次登录的 21 步引导含"VIP、免费试用套餐"等商业化表述，
      与内部单管理员定位不符（裁剪或直接移除引导，实施时一并决策）（于 internal-rc-6a1452d 验证通过）
- [x] **用户菜单 GitHub 链接指向上游仓库**：链接到 Wei-Shaw/sub2api，内部版建议移除（于 internal-rc-6a1452d 验证通过）

附注（非缺陷，使用提醒）：UI 添加 Claude Console（API Key）类型账号时会默认写入一份
带日期后缀的模型白名单，使用 `claude-sonnet-4-5` 这类别名会返回 404 `model_not_found`，
需用全名（如 `claude-sonnet-4-5-20250929`）或清空白名单。属上游原版行为；
是否在默认白名单补充常用别名，待定。
