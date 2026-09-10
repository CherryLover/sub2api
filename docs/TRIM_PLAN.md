# 内部部署裁剪：计划·进度·验证

> 目标：将 sub2api 裁剪为**内部部署、自用多用户**的 API 转发网关。
> **2026-09-08 调整**：原表述为「单管理员、内部部署」。站长要给其他人创建多个用户使用，**包括多个管理员**，
> 因此目标改为「内部部署、自用多用户」；正文与第三～五节已同步改写，第六节各轮历史记录保留当时的「单管理员」表述。
>
> **保留面（不再裁）**：用户管理（创建 / 编辑 / 停用 / 删除用户）、用户角色与类型（`admin` / `user` 及现有的用户属性、
> 分组授权 `user_allowed_groups`、每用户平台额度 `user_platform_quotas`）、每用户各自的 API Key、
> 用户侧 `/keys` `/usage` `/dashboard` `/profile` 四个页面、审计日志、Passkey / TOTP。
> 管理端「用户管理」的新建 / 编辑弹窗都能把角色设为 `admin`，后端 `POST/PUT /api/v1/admin/users` 接受 `role=admin`
> （2026-09-08 读码核实，依据见第三节 A 组表下方「多管理员核实」）——**创建第二个管理员已支持，无需新增待办**。
>
> **仍然裁掉 / 保持关闭**（已删的不恢复）：注册（管理员建号，`registration_enabled=false` 不变）、支付 / 订阅 / 余额 / 兑换码 / 推广、
> 第三方登录、邮件体系、内容审计、批量生图、公告、模型广场。
> 已交付的 A6 各切片（A6-1 / A6-2 / A6-3 / 甲 / 乙 / 丙）在多用户下语义成立，**不回退**。
>
> 约定：转发链路（全部上游平台、全部入站协议、协议转换层、Codex 版本同步）**全量保留**；
> SaaS 商业化外围逐批删除，多用户基础面保留。每批：拆包 → 多 Agent 并行实施 → 逐包验证 →
> 合并后统一验证 → CI 全绿 → RC 镜像 → 内网部署验证 → 合并 main。
>
> 维护说明：本文档由裁剪工程随批次更新，是唯一权威的计划、进度与部署验证记录（验证清单见第五节，问题追踪见第六节）。

---

## 一、总体进度（对照最初 7 阶段计划）

| 原计划阶段 | 状态 | 说明 |
|---|---|---|
| 1. Fork 与范围冻结 | ✅ 完成（批次 1 + 2） | 范围与协议决策已定（转发面全保留）；应用内更新检查/在线升级/回滚整删；A1 尾款（install.sh 的 update/rollback 子命令、compose/`.env.example`/README 过时注释）批次 2 交付完毕 |
| 2. 数据模型收敛 | ✅ 完成（批次 4 + 5） | 订阅/余额语义改造 = 批次 4；ent 17 个死实体删除 + 21 张死表 + 3 个死列 + 50 多个孤儿设置键 = 批次 5。**迁移基线重置未做**，实测数据不支持该改造，见第四节 |
| 3. Key 认证与额度引擎 | 🟢 大部分完成 | 防爆破/IP ACL/幂等结算/惰性窗口/时区均已存在；额度已在批次 4 统一到 `api_keys.quota`；**A3 周额度自然周锚点已交付（批次 6 第二轮，迁移 239），已在 way-rc `internal-ae47e55` 验证、待站长验收；生产与 way-rc 均无设过周额度的账号，迁移在两边都是空操作** |
| 4. 网关与上游账户池 | ✅ 完成（按决策零改动） | 全量保留 |
| 5. 最小管理员后台 | 🔵 进行中 | 商业化入口、登录条款、通用设置冗余项、订阅/余额面均已拆；**A6-1（Keys 列表「查用量」入口，v0.1.184）与 A6-2 两步（Bark 推送通道 v0.1.185、账号用量阈值提醒规则 v0.1.186）已上线生产（2026-09-07）**，密钥列表今日用量排序 + 「最后使用时间」列（A6-3）随 v0.1.186 一并上线；**A6 已于 2026-09-08 改名「管理后台多用户直管」并标记 ✅ 完成**：A6-甲（管理端跨用户密钥总表）/ 乙（管理员访问 `/dashboard` `/usage` 转到管理端，`/keys` 保留）/ 丙（用户密钥弹窗补启停 / 删除 / IP 名单）随批次 6 第三轮交付，已 RC 验证、待站长验收 |
| 6. 删除 SaaS 外围 | ✅ 完成（批次 1–5） | 批次 1 支付/卡密/第三方 OAuth；批次 2 注册体系/人机验证；批次 3 邮件体系/内容审计/批量生图/渠道监控 V1/公告/模型广场；批次 4 订阅与余额。A2 全部落地 |
| 7. 安全与上线验证 | 🟢 批次 1–5 与批次 6（A6-1 / A6-2 / A6-3）已上线生产 | 批次 1 三轮、批次 2 一轮、批次 3+4+5 一轮（internal-498f780，2026-08-29）RC 镜像内网验证全部通过；**v0.1.183 已于 2026-09-04 切换到生产 `way.flyooo.uk`**（切换步骤、数据核对与上线后实证见第六节「v0.1.183 正式上线」）；**批次 6 三次发版、三次生产升级（v0.1.184 / v0.1.185 / v0.1.186；其中 0.1.185 未单独切换生产、随 0.1.186 一起生效）全部通过**（2026-09-04 → 0.1.184，2026-09-07 → 0.1.186，见第六节「批次 6 发版记录」）；**批次 6 第二～四轮（way-rc `internal-ae47e55`，2026-09-07 部署核对通过）待站长登录验收**（见第六节最上方小节）；压测与备份恢复演练仍未做（A7，批次 6）；⚠️ 2026-09-05 验证期间发生服务器事故，见第四节 |

**当前位置**（2026-09-10 更新）：**批次 1–5 与批次 6 全部内容（含第二～四轮）已合并 `fork/main` 并上线生产；外部 PR #10（上游 v0.2.3 核心修复回移）也已审查后合入。**
生产 `way.flyooo.uk` 与 way-rc **均为正式 tag `0.1.188`**；`fork/main` 现为 `8314ac7a5`（PR #10 合并提交，VERSION 已同步 0.1.188）。
本轮先以 **v0.1.187** 发出批次 6 第二～四轮，再以 **v0.1.188** 发出 PR #10 回移，生产由 0.1.186 一次跨到 0.1.188（迁移头 238 → 240）。
详见第六节最上方「v0.1.187 / v0.1.188 发版与上线记录」；**09-07 那份站长登录验收清单仍未逐项手点**，可直接在 0.1.188 上补做。
批次 1–5 是 2026-09-01 合并、2026-09-04 以 v0.1.183 上线生产的（见第六节「v0.1.183 正式上线」）；批次 6 至今三次发版、三次生产升级全部通过（细节见第六节「批次 6 发版记录」）：

- **v0.1.184**（2026-09-04 09:16Z 发版）：A6-1 密钥列表「查用量」入口 + 用量页订阅 / 余额残留清理 + TRIM_PLAN 补记 v0.1.183 上线；同日 09:20Z 生产 0.1.183 → 0.1.184。
- **v0.1.185**（2026-09-05 02:22Z 发版）：A6-2 第一步 Bark 推送通道 + 管理端配置 / 测试接口 + 接入运维告警引擎 + 设置页「通知」页签；
  **未单独切换生产**（当天服务器事故，见下方引用块），随 0.1.186 一起生效。
- **v0.1.186**（2026-09-07 04:2xZ 发版）：A6-2 第二步账号用量阈值提醒规则 + 密钥列表今日用量排序与「最后使用时间」列（A6-3）+ 一键三档弹窗修复；
  2026-09-07 04:26Z 生产 0.1.184 → 0.1.186（跨过 0.1.185）。

0.1.184 → 0.1.186 之间**没有任何数据库迁移**（迁移头始终 `238_drop_registration_email_suffix_whitelist_setting.sql`，77 张表）；
生产 compose 自 09-04 起钉固定版本 tag，不再跟随 `:latest`。

**批次 6 第二～四轮**（2026-09-07 交付，三轮各自 CI 全绿、各出验证镜像并依次部署到 way-rc；范围说明见第三节，RC 验证记录见第六节最上方）：

- **第二轮**（`batch6/round2`，HEAD `e394a90a1`，镜像 `internal-e394a90`）：WP-C2 后端残留清理 + WP-C1 前端残留清理 + **A3 上游账号周额度改自然周（迁移 239）**。
- **第三轮**（`batch6/round3`，HEAD `b68971a24`，镜像 `internal-b68971a`）：A6-甲 管理端跨用户密钥总表 + A6-乙 管理员 `/dashboard` `/usage` 转到管理端 + A6-丙 用户密钥弹窗扩展 + **`custom_endpoints` 整键删除（迁移 240）**。
- **第四轮**（`batch6/round4`，HEAD `ae47e5564`，镜像 `internal-ae47e55`）：使用记录账号可点 / 按账号筛选 + 账号「容量」格负载抽屉（甲方案，基于使用记录的「最近请求」）。

way-rc 现在的迁移头是 `240_drop_custom_endpoints_setting.sql`（记录 280、表 77）；**迁移 239 / 240 在生产同样是空操作**（生产没有设过周额度的账号，`custom_endpoints` 值为 `[]`），但升级仍照旧备份 + 导回验证。
**下一步**：站长登录 way-rc 按第六节该小节末尾的清单验收 → 通过后 `batch6/round4` ff 合并 `fork/main` → auto-release（预期 **v0.1.187**）→ 生产升级 → way-rc 切回正式 tag。
**批次 6 剩余**：A7 文档改写（README/DEV_GUIDE 内部化）+ 压测 + 备份恢复演练 + Key 泄露演练（未开始；方案要点见第三节 A7 行，演练涉及服务器、须遵守下方新纪律）；
乙方案「真正的在途请求登记」（候选，排下一批）；第四节「后续候选项台账」的未勾项与 FEATURE_CHECKLIST 的 P1 / P2 残留；两条 CI 维护项（docker actions Node 20 弃用告警、`internal-rc-*` 标签重复触发 CI）。
**站长待办**：在生产「系统设置 → 通知」填一次 Bark 配置（配置按环境分别存储，way-rc 上填的不会带到生产）。
**决策状态**：批次 2 验收起累计的三项未决决策已于 2026-09-07 **全部拍板**——`custom_endpoints` 整键删除（迁移 240）、`security.url_allowlist.enabled` **保持关闭**、`server.trusted_proxies` **暂不配置**（见第四节「三个决策结论」）；
迁移基线重置**维持不做**（A5 内，方案见第四节「⛔ 未完成项 1」）。
**关口**：auto-release 保持现状不动（批次 6 的三个版本都是 ff 合并 `fork/main` 后自动发出的，内部版本线继续走 main）。

> **⚠️ 2026-09-05 服务器事故与新纪律**（详细条目见第四节「⛔ 2026-09-05 事故：并行重任务压垮生产机」）：
> 约 02:02Z ～ 02:20Z（北京时间 10:02–10:20），协调者同时派三个实施者在 `oci-free`（2 OCPU / 12 GB，生产与 way-rc 同机、另有十余个常驻容器）上
> 并行跑后端编译 + 全量单测 + lint、前端装依赖 + vitest + 临时 vite 开发服务器 + 新起 browserless Chromium 容器做视觉走查，发版线同时在拉镜像，
> 15 分钟平均负载冲到 47，`way.flyooo.uk` 与 way-rc 的 HTTPS、SSH 全部超时。**原因是协调者的调度失误**，不是代码或镜像的问题。
> 停掉全部实施者后机器约 15 分钟自行恢复（无需重启）；生产容器全程未被改动（仍 0.1.184、healthy），数据库表数 / 行数一致；
> 期间应用记录 29 条「查询超时」错误，恢复后无新错误。v0.1.185 的生产升级因此只做到备份就停下，没有单独切换。
> **新纪律**（已写入协调者长期记忆）：编译、测试、lint、装依赖、无头浏览器一律走 GitHub CI；`oci-free` 上只允许拉镜像、重建容器、curl / psql 级核对；
> 任何时刻最多一个实施者操作服务器，操作前先看负载。此后的 A6-2 第二步与密钥页改动全部只用 CI 验证，视觉走查取消。

### ⚠️ 生产影响：`:latest` 已经是生产的活动标签

> **2026-09-04 更新：本节描述的"合并即上生产"风险模型已解除。** v0.1.183 上线时，生产 compose 的镜像行
> 已从 `:latest` 改回固定 tag `ghcr.io/cherrylover/sub2api:0.1.183`
> （原文件备份 `docker-compose.yml.bak-pre-0.1.183-20260904-021710`），生产从此不再跟随 `:latest`，
> 合并 main / 自动发版与"上生产"之间重新有了人工改 tag 的闸门。下文原样保留，供追溯当时的决策背景。
> **但有一条不变**：批次 5 之后回滚仍须连数据库一起恢复（第四节「⛔ 回滚不再是无损操作」），
> 改回旧 tag 只解决镜像那一半。

生产 `way.flyooo.uk` 的镜像坐标**已从固定版本号改为 `ghcr.io/cherrylover/sub2api:latest`**。
这条改动改变了发版的风险模型，批次 2 及以后每批都必须按此执行：

- **合并 main 会自动发版并覆盖 `:latest`，生产容器下次重启就会拉到新版本。**
  "发版"与"上生产"之间不再有人工改 tag 的闸门 —— 合并即等于把新版本预置到了生产上。
- 因此**破坏性变更必须先出 `internal-rc-<short_sha>` 标签、走 way-rc（`way-rc.flyooo.uk`）验证，
  验证通过再合并 main**。禁止"先合并 main 再验证"。
- 批次 2 的 **B3（注册体系彻底移除）** 触及登录与用户表语义，已按此流程走完 RC 验证；
  **批次 3 + 4 + 5 同理，且破坏性更强**（批次 4 改计费核心、批次 5 做不可逆的删表删列），
  必须先出 RC 镜像走完第五节清单，通过才能合并。
- 回滚路径相应变化：出问题时**不能只靠重启**（重启只会再拉一次 `:latest`），
  需先把生产 compose 的镜像临时钉到上一个正式版本 tag（如 `ghcr.io/cherrylover/sub2api:0.1.182`）再重启。
  ⚠️ **批次 5 合并之后这还不够** —— 删表删列后回滚必须连数据库一起恢复，见第四节「⛔ 回滚不再是无损操作」。

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

## 三、批次 2 – 5 交付记录与后续 backlog

### 批次 2 首发范围与进度（已交付，way-rc 验收通过）

本批首发 = **B1 + B2 + B3 + B4 + A1 尾款**。

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

#### 批次 2 交付明细（已 CI 全绿并于 internal-1e49744 验收通过）

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

| 编号 | 内容 | 落点 | 状态 |
|---|---|---|---|
| A1 | 应用内更新检查/在线升级/回滚整删（版本号展示与 restart 保留；`update.proxy_url` 因定价表与 Codex 同步仍消费而保留原键名）；尾款 = install.sh 的 update/rollback 子命令、docker-deploy.sh、compose/`.env.example`/README 过时注释。auto-release.yml 经站长决策**保持现状不动** | 批次 1 + 2 | ✅ 已交付 |
| A2 | 公告、模型广场、邮件营销面删除；内容安全审计删除、批量生图删除、渠道监控删 V1 留 V2（并把 `channel_monitor_mode` 切到 `v2`） | 批次 3 | ✅ 已交付 |
| A3 | 周额度改自然周（周一 00:00）锚点 | 批次 6 第二轮 | ✅ **已交付（round2，`ce429bd1e` / `208aa0d96` / `dd0844ae1` / `d6525764e`，原分支 `batch6/a3-account-weekly-anchor`），迁移 239；生产与 way-rc 均无设过周额度的账号，迁移在两边都是空操作（已 psql 核实）；已在 way-rc `internal-ae47e55` 验证，待站长验收**。实际范围只动**上游账号**的周限额：迁移 239 把「设了周限额、没选过重置方式」的账号一次性改成固定 / 周一 / 00:00 / 项目时区并写好下次重置点，前后端缺省时区从 UTC 改为项目时区；用户平台额度早已按自然周（`timezone.StartOfWeek`）无需改；Key 的 `rate_limit_7d` 是 168h 限速而非额度，刻意不动 |
| A4 | 订阅/余额拆除与 Key 额度直绑改造（最难一刀，单独里程碑） | 批次 4 | ✅ 已交付 |
| A5 | 数据库压平：删除死实体（payment/redeem/promo/subscription_plan 等）、**迁移基线重置**、支持全新初始化 | 批次 5 | 🟡 **部分交付** —— 死实体/死表/死列/孤儿键全部清完，**迁移基线重置未做**（见第四节「⛔ 未完成项 1」） |
| A6 | **管理后台多用户直管**（2026-09-08 改名；原标题「管理后台单管理员化（Keys/用量页直管语义）」作废） | 批次 6 | ✅ **已完成**：A6-1（v0.1.184）、A6-2 第一步（v0.1.185）、A6-2 第二步 + A6-3 密钥页排序（v0.1.186）已上线生产（2026-09-07）；A6-甲 / 乙 / 丙 随批次 6 第三轮交付（下方三行），已 RC 验证、待站长验收。所有切片在多用户下语义成立，**不回退** |
| A6-甲 | 管理端跨用户密钥总表：`/admin/api-keys` 页 + `GET/PUT/DELETE /api/v1/admin/api-keys*`（列表掩码、排序 / 筛选、启停、分组与 IP 名单、删除、查用量跳 `/admin/usage`） | 批次 6 第三轮 | ✅ 已交付（后端 `ab12794cc` / `be97dc3af` / `88a34fb2f` / `0ba5641a0`，前端 `c076942f2`），已 RC 验证、待站长验收 |
| A6-乙 | 管理员访问 `/dashboard` → `/admin/dashboard`、`/usage` → `/admin/usage`（`router/adminRedirect.ts`），「我的账户」侧栏去掉 `/usage`；**管理员自己的 `/keys` 保留** | 批次 6 第三轮 | ✅ 已交付（`da4be4549`），已 RC 验证、待站长验收 |
| A6-丙 | `UserApiKeysModal`（用户管理 → 「API 密钥」弹窗）补启停 / 删除 / IP 名单编辑 | 批次 6 第三轮 | ✅ 已交付（`f255ac71f`），已 RC 验证、待站长验收 |
| （第四轮） | 使用记录账号可点 + 按账号筛选 + 账号页「查看用量」；账号「容量」格负载抽屉（**甲方案**：基于使用记录的「最近请求」，`GET /admin/accounts/:id/recent-requests`）。**乙方案「真正的在途请求登记」排下一批** | 批次 6 第四轮 | ✅ 已交付（后端 `f3bcbd9a1` / `396ca2137` / `03b5d7906`，前端 `3403abe39` / `ec05c82c3`），已 RC 验证、待站长验收 |
| A7 | 部署文档改写（README/DEV_GUIDE 内部化）+ 压测、备份恢复、Key 泄露演练 | 批次 6 | ⚪ 未开始。方案要点（2026-09-08 记录）：压测用 `workflow_dispatch` 触发的 k6 打 way-rc；备份恢复在隔离栈演练；Key 泄露演练在 way-rc；全部受第四节「⛔ 2026-09-05 事故」新纪律约束（服务器上只做部署与轻量核对） |

**多管理员核实（2026-09-08 读码，结论：已支持创建第二个管理员，不需要 A8）**：
前端新建用户弹窗 `frontend/src/components/admin/user/UserCreateModal.vue:30-32` 的角色下拉含 `user` / `admin` 两项；
编辑弹窗 `UserEditModal.vue:95-97` 的 `roleOptions` 同样含 `admin`。
后端 `backend/internal/handler/admin/user_handler.go:75` / `:88` 的 `CreateUserRequest` / `UpdateUserRequest` 对 `role` 的校验是 `oneof=admin user`；
`Create`（`:212`）在 `role=admin` 时先走 step-up 2FA（`:220-224`），`Update`（`:247`）把普通用户提升为管理员时同样要 step-up（`:269-279`），目标已是管理员时不打断；
服务层 `backend/internal/service/admin_user.go:104-111` `normalizeUserRole` 只接受 `admin` / `user`，`CreateUser`（`:114-141`）按传入角色建号并对建管理员落审计日志。
配套的防锁死保护：管理员不能把自己降为普通用户（handler `:262-265`），不能降级系统里最后一个管理员（service `ensureNotLastAdmin`，`:146-160` / `:225-229`），
不能禁用 / 删除管理员（service `:178` / `:303`；前端 `UsersView.vue:565` / `:658` 对 admin 行直接隐藏停用与删除按钮）。**多个管理员并存是现有代码本来就支持的路径**，FEATURE_CHECKLIST 第 1 节已加「创建第二个管理员」验收行。

排序建议（**已被 2026-08-28 决策取代**，保留原文供追溯）：B1–B4 与 A1 合并为批次 2 首发 → A2 → A3 → A4 → A5 → A6 → A7。B5 待决策后并入。

---

### 站长决策与重排（2026-08-28）

本节是**当前生效的排期**，取代上方「排序建议」。

#### 已拍板的决策

| 决策项 | 结论 | 落点 |
|---|---|---|
| **B5 邮件体系** | **方案 A：整删 SMTP 与全部邮件体系**（约 4400 行）。配套补 `backend/cmd/` 管理员密码重置小工具 | 批次 3 |
| **注册用户默认值** | 一并去掉（不再有"新用户默认余额/默认订阅"的概念）。⚠️ 与订阅/余额强耦合，**技术上必须随 A4 落地**，不能提前 | 批次 4 |
| **充值兑换** | 一并去掉。批次 1 已删业务逻辑，**剩余是 ent 死实体 + 历史迁移 + 文档**，属 A5 范围 | 批次 5 |
| **内容安全审计** | 删除 | 批次 3 |
| **批量生图** | 删除。核查依据：生产 7 个分组 `allow_batch_image_generation` **全为 false**，`batch_image_jobs` / `batch_image_items` **均为 0 行**，从未使用 | 批次 3 |
| **渠道监控** | **删 V1 主动探测，保留 V2 被动监控**，并把系统模式切到 `v2`。核查依据：生产 `channel_monitor_mode=v1` 且 `channel_monitors` **0 行**、`channel_monitor_histories` **0 行** —— 开着一个会真实消耗上游额度的探测器却零监控项空转，同时零成本的 V2 因模式未切而没有运行 | 批次 3 |

> **可立即执行、不依赖代码改动**：把生产与 way-rc 的 `channel_monitor_mode` 从 `v1` 改为 `v2`，
> V2 被动监控即刻开始聚合（不发任何额外上游请求）。无需等批次 3 上线。

#### 重排后的批次

| 批次 | 范围 | 状态 |
|---|---|---|
| **批次 2** | B1 登录条款 / B2 通用设置冗余项 / B3 注册体系 / B4 人机验证 / A1 尾款 | ✅ **已交付，way-rc 验收通过**（第六节 internal-1e49744） |
| **批次 3** | B5 邮件体系整删（+ `adminpass` 密码重置工具）、内容安全审计删除、批量生图删除、渠道监控 V1 删除（模式切 V2）、A2 剩余（公告 / 模型广场 / 邮件营销文案）、第四节「后续候选项」里的小尾巴（CSP 失效白名单、支付遗留文档、`registration_email_domain_quota_enabled`）、批次 2 验收发现的两条部署侧隐患 | ✅ **已交付，RC 验收通过（internal-498f780），已随 v0.1.183 上线生产（2026-09-04）** |
| **批次 4** | A4 订阅/余额拆除 + Key 额度直绑；**注册用户默认值随本批消失** | ✅ **已交付，RC 验收通过（internal-498f780），已随 v0.1.183 上线生产（2026-09-04）**（最难一刀；两项行为变更已在隔离栈真实计费补验与生产实证，见第六节） |
| **批次 5** | A5 数据库压平：ent 17 个死实体 + 21 张死表 + 3 个死列 + 50 多个孤儿设置键；**充值兑换残留已清干净**；顺带修好批次 3/4 遗留的编译与格式问题 | ✅ **已交付，RC 验收通过（internal-498f780），已随 v0.1.183 上线生产（2026-09-04）**；⛔ **迁移基线重置未做**（维持不做），见第四节 |
| **批次 6** | A3 周额度自然周锚点、A6 管理后台多用户直管（2026-09-08 改名，原「后台单管理员化」）、A7 文档改写 + 压测 / 备份恢复 / Key 泄露演练 | 🔵 **进行中：A6-1（v0.1.184）、A6-2 第一步（v0.1.185）、A6-2 第二步 + 密钥页排序（v0.1.186）已上线**（生产仍在 0.1.186，发版与升级记录见第六节「批次 6 发版记录」）；**第二～四轮（下方三行）已 RC 验证、待站长验收；A7 未开始** |
| **批次 6 第二轮** | WP-C2 后端残留清理（审计路由白收参数、`server.frontend_url`、`GET /user/totp/verification-method` 死端点）/ WP-C1 前端残留清理 / **A3 上游账号周额度改自然周（迁移 239）** | ✅ **已交付**：集成分支 `batch6/round2`（HEAD `e394a90a1`，基于 `306bcf291`），集成 CI 34090219404 ✓、Security Scan 34090219309 ✓，镜像 `internal-e394a90`；**已 RC 验证（2026-09-07），待站长验收** |
| **批次 6 第三轮** | A6-甲 管理端跨用户密钥总表 / A6-乙 管理员 `/dashboard` `/usage` 转到管理端 / A6-丙 用户密钥弹窗扩展 / **`custom_endpoints` 整键删除（迁移 240）** | ✅ **已交付**：`batch6/round3`（HEAD `b68971a24`，基于 round2），集成 CI 34098887841 ✓、Security Scan 34098887797 ✓，镜像 `internal-b68971a`；**已 RC 验证（2026-09-07），待站长验收** |
| **批次 6 第四轮** | 使用记录账号可点 + 按账号筛选 + 账号页「查看用量」/ 账号「容量」格负载抽屉（甲方案） | ✅ **已交付**：`batch6/round4`（HEAD `ae47e5564`，基于 round3），集成 CI 34103833981 ✓、Security Scan 34103834034 ✓，镜像 `internal-ae47e55`；**已 RC 验证（2026-09-07），待站长验收** |

**A6-1 范围说明（Keys 列表「查用量」入口，✅ 已上线 v0.1.184，2026-09-04）**：`/keys` 每行新增「查用量」按钮——点击时先同步开一个空白标签页，
用原始 Key 向 `POST /api/v1/key-usage/session` 换只读令牌后再把该标签页导航到 `/key-usage?t=<令牌>`（原始 Key 不进网址）；弹窗被浏览器拦截时退化为当前页打开；
换令牌失败则关掉标签页并提示；`status=inactive` 的 Key 按钮置灰（后端对 inactive 返回 401）。用量页删掉「订阅类型 / 钱包余额」两行残留，
徽标改为「不限额模式」（新键 `keyUsage.unlimitedMode`）。顺带把 `KeysView.spec.ts`、`KeyUsageView.spec.ts` 加进 Makefile 的 `FRONTEND_CRITICAL_VITEST`（此前 CI 从不跑它们）。
集成分支 `batch6/a6-1`（HEAD `a6a917078`），验证镜像 `internal-a6a9170`；验收与升级记录见第六节。

**A6-2 范围说明（Bark 推送 + 告警出口，2026-09-04 启动，✅ 两步均已上线，2026-09-07 更新）**：邮件体系在批次 3 整删后系统没有任何主动推送通道，
运维告警引擎（`OpsAlertEvaluatorService`）触发 / 解除时只落 `ops_alert_events`、不外发。A6-2 分两步补回，最终范围如下（建档时的"进行中"描述已被本段取代）：

- **第一步（✅ 已上线：代码在 v0.1.185，生产随 v0.1.186 生效）：Bark 通道 + 告警出口。**
  后端：Bark 推送客户端（`bark_notifier.go`：`POST {server}/push`、`GET /ping` 探活，10 秒超时，直连不走上游代理池，响应片段截断 200 字并抹除 device_key）；
  设置服务（`notify_bark_settings.go`：配置整段 JSON 存 `settings.notify_bark_config`，`device_key` 用 `SecretEncryptor` AES 加密落库，GET 永不回显、以 `has_device_key` 表示已配置，
  PUT 传空 key 保留旧值，30 秒配置缓存）；管理端三条接口 `GET/PUT /api/v1/admin/notifications/bark`、`POST .../bark/test`
  （返回 `ok / ping_ok / status_code / message / latency_ms`；没填 key 时只探活，返回 200 且 `ok=false`）；
  告警引擎在事件落库后推「触发」，解除时按 `notify_on_resolve` 推「已恢复」，推送失败只记 Warn、不影响评估。
  裁剪门禁新增 `TestNotificationBarkSurfaceRegistered`，并断言 `notify_bark_config` 不进 `/api/v1/settings/public`。
  前端：设置页新增「通知」页签（key `notifications`，排在网关服务与数据备份之间）与 `BarkNotifySettingsCard.vue`
  （启用 / 服务器地址 / 设备 Key（密码型，已配置时显示占位）/ 分组（默认 sub2api）/ 打断级别四选一 / 提示音 / 点击跳转地址 / 恢复时也通知）；
  「测试连接」按 `ping_ok × ok` 四种组合给出四种色调，「发送测试通知」只看 `ok`；spec 14 例纳入 CI 名单。
  端到端实证（2026-09-05 02:01Z，在远程容器里用真实代码路径 + 正式同款 AES 加密器，对站长的 Bark 服务器 `https://notify.flyooo.uk`）：
  PUT 落库密文不含明文 key（272 字节）→ GET → 测试推送 ok / ping_ok 200、698 ms → 无 key 只探活 40 ms → 告警触发推送 → 恢复推送，站长手机三条全部收到（截图核实）。
  站长的 notify.flyooo.uk 是标准 bark-server（JS 版 v2.2.6，7 台设备），无需额外鉴权头。
  生产与 way-rc 均已确认 `TOTP_ENCRYPTION_KEY` 已配置（64 hex），保存设备 Key 不会被 `SECRET_ENCRYPTION_KEY_NOT_CONFIGURED` 拦。
  集成分支 `batch6/a6-2`（HEAD `34be1e12b`：后端 `0ae423571`、前端 `9aed20219`、对接修正 `34be1e12b`），CI run 33862992329 一次全绿，验证镜像 `internal-34be1e1`（run 33863752189）。
  - [ ] 未做：告警真实触发后的推送在生产上尚未观察到（上线以来没有告警发生）。
  - [ ] 站长待办：生产「系统设置 → 通知」的 Bark 配置尚未填写（配置分环境存储，way-rc 的不会带过来）。
- **第二步（✅ 已上线 v0.1.186，2026-09-07）：账号用量阈值提醒规则。** 方案由只读规划者产出：复用告警引擎，不需要迁移；两个关键坑——取账号不能用 `GetAccountAvailability`（不带 extra），
  事件必须按账号拆（否则一条规则只有一个活动事件）。
  后端：四个新指标 `account_window_used_percent`（filters.window 5h | 7d；openai 读 codex_5h / 7d_used_percent，anthropic 的 0–1 小数经 `utilizationAsPercent` 换算，grok 挂 5h，
  kimi / zhipu 读 `<provider>_5h / _weekly_used_percent`；无快照或已过期跳过，重置时间已过按 0%）、`account_quota_used_percent`（dimension daily | weekly | total，limit≤0 跳过）、
  `account_balance`（provider kimi | deepseek，读 `<provider>_balance`，事件带币种）、`account_today_cost`（`GetTodayStatsBatch` 口径，评估器新注入 `UsageLogRepository`）。
  评估按「规则 × 账号」拆目标，状态键 `{RuleID, AccountID}`，仓储新增 `GetActiveAlertEventForAccount / GetLatestAlertEventForAccount`（按 `dimensions->>'account_id'`）；
  健康度指标走 accountID=0 单目标，既有行为不变。新接口 `POST /api/v1/admin/ops/alert-rules/:id/evaluate`（body `{send}`；返回规则信息、`evaluated_at / has_data / value / breached`、
  逐账号 `accounts[]`、`sent / send_error`；不落事件、不动状态；`send=true` 且 Bark 未启用时 400 `BARK_NOT_ENABLED`）。handler 指标白名单 +4、新增 `validateAccountAlertFilters`；
  `OpsAlertNotification` 加 `MetricLabel / Unit / Details`，Bark 加 `NotifyOpsAlertManual`（标题「[Sub2API] 手动试发 <规则名>」）；门禁正向路由清单加 evaluate。
  **已知限制**：账号被删除后其名下 firing 事件不会自动解除。
  前端：「通知」页签新增 `AccountUsageAlertRulesCard.vue`：只列四类指标的规则；新建 / 编辑（指标、窗口 / 维度 / 供应商联动、平台、分组、账号多选、运算符、阈值、严重度、冷却；
  `window_minutes` 固定 1）；启停 / 删除；「立即试发」先干跑显示逐账号结果，勾选后再推 Bark；「一键三档」（仅百分比指标；名称后缀 40% / 60% / 80%，运算符 >=，
  本地查重 + 后端重名识别）；运维监控总开关关闭时显示「需开启运维监控」。`OpsAlertRulesCard.vue` 的指标定义 +4 并提示到设置页配置账号范围。spec 11 例纳入 CI。
  三档弹窗修复（`81836e9c8`，含在 v0.1.186 内）：三条全部创建成功后自动关闭弹窗；有失败或重名跳过时保留逐条结果——站长在 way-rc 实测三档时发现并要求修改。
  集成分支 `batch6/a6-2s2`（HEAD `e9936c5b5`：后端 `8a39c0ba3` / `b3ec34d4e` / `dc31704a5`，前端 `9552c97b6`）；
  分支 CI 后端第一轮 golangci-lint 红（errcheck 未检查类型断言）修后全绿（run 33942246430），前端一次全绿（run 33941774097），集成 CI run 33942864698 全绿，
  验证镜像 `internal-e9936c5`（run 33943258818）。验收与升级记录见第六节。

**A6-3 范围说明（密钥列表今日用量排序 + 「最后使用时间」列，✅ 已上线 v0.1.186，2026-09-07）**：分支 `batch6/keys-sort-lastused`（提交 `022f98c97`，基于 `e9936c5b5`），
CI run 34077780372 一次全绿。发现：「最后使用时间」列定义早已存在且可排序，但在 `DEFAULT_HIDDEN_COLUMNS` 里且隐藏列表存 localStorage，老用户改默认值也看不到——
列设置版本 3 → 4，`VERSION_REVEALED_COLUMNS` 在升版时一次性放出该列（之后仍可再隐藏）。
后端 `api_key_repo.go` 的 `apiKeyListOrder` 新增 `today_cost`：ORDER BY 关联子查询汇总 `usage_logs.actual_cost`（`created_at >= timezone.Today()`，与列表「今日用量」显示同口径），id 作次序键；
用 `entsql.ExprFunc` 保住绑定参数（`OrderExprFunc` 会丢参数）；走 `idx_usage_logs_api_key_created_at`，无迁移。SQLite 单测 + Postgres 集成测试。
前端「用量」列 `sortable: true`，映射 `sort_by=today_cost`。way-rc 验收随 v0.1.186 合并镜像 `internal-022f98c` 一起做，站长登录实测密钥页排序通过（见第六节）。

**批次 6 第二～四轮范围说明（2026-09-07 交付，2026-09-08 记录；RC 验证事实见第六节最上方小节）**：三轮各自由三条并行分支合成一个集成分支，
每条分支 CI 全绿后合并，第二、四轮无冲突，第三轮只在 Makefile 关键 vitest 名单一处冲突（三条 spec 全保留）。

- **第二轮 `batch6/round2`（`e394a90a1`，基于 `306bcf291`）**
  - **WP-C2 后端残留清理（`55734db0e`）**：删审计日志路由白收的 step-up 参数；删 `server.frontend_url` 配置项（含 viper 默认值、URL 校验、`warnIfInsecureURL` 死 helper、
    config_test、`deploy/config.example.yaml`）；删 `GET /api/v1/user/totp/verification-method` 死端点（handler + service + 门禁缺席断言）。CI 34086811737 ✓。
  - **WP-C1 前端残留清理（`20e6d5193` / `7e63d296c` / `1ad0bf6f6`）**：余额记录 tooltip 改为「点击下钻筛选」新键、删充值记录一组 12 个文案；`SettingsView.spec` 删 59 个已删字段与 payment 桩；
    删 `claudeMaxSimulation` 中英整块；`subscription_type` 整字段删除（后端 DTO 已不返回）含 `GroupBadge` / `GroupOptionItem` 订阅分支与三个 prop、三个订阅死类型；
    删 Key 脚本 `response?.balance` 兜底；删 9 个失效 `admin.users.*` 键；删 `views/auth/{README,VISUAL_GUIDE,USAGE_EXAMPLES}.md`；删 25 个零调用 API 封装及其类型。
    FEATURE_CHECKLIST 勾 6 行。CI 34087617890 ✓。**顺手发现未动**：`types/index.ts` 里 `ProxyNode / ConversionRequest / ConversionResult / SubscriptionStats / UserStats` 零引用；
    `admin.users` 下 `depositSuccess / withdrawSuccess / failedToDeposit / failedToWithdraw / useDepositWithdrawButtons / amountRequired` 零引用（已记入第四节台账）。
  - **WP-A3 账号周额度改自然周（`ce429bd1e` / `208aa0d96` / `dd0844ae1` / `d6525764e`）**：规划核实——用户平台额度早已是自然周（`timezone.StartOfWeek`）；Key 的 `rate_limit_7d` 是滚动 168h 限速，刻意不动；
    唯一改的是上游账号 `quota_weekly_*`。**迁移 239** `account_weekly_quota_natural_week.sql`：对 `quota_weekly_limit>0` 且无 `reset_mode` 的存活账号写 `fixed` / 周一 / 00:00、
    `quota_reset_timezone=current_setting('TimeZone')`、必写 `quota_weekly_reset_at`（否则计费把缺失当 1970 首笔清零）、`quota_weekly_start` 早于本周一则清零 used；
    已 fixed / rolling / limit=0 / 软删不动；幂等。后端 `account.go` 三处 `UTC` 默认改项目时区；`ops_alert_account_metrics.go` 周额度过期按 0（顺带修）。
    前端弹窗周限额默认 fixed / 周一 / 00:00、时区默认 `server_timezone`；`QuotaLimitCard.weeklyNaturalWeek.spec.ts` 9 例进 CI 名单。
    第一轮 CI 一条 spec 红（清空唯一限额时卡片自动关闭）修后全绿（CI 34088345142）。**生产与 way-rc 均无设过周额度的账号，迁移在两边都是空操作**（已 psql 核实）。
    跳变说明：命中账号旧窗口跨周一会清零本周花费（少扣不多扣）。
- **第三轮 `batch6/round3`（`b68971a24`，基于 round2）**
  - **A6-甲 + 丙 后端（`ab12794cc` / `be97dc3af` / `88a34fb2f` / `0ba5641a0`）**：`GET /api/v1/admin/api-keys`（分页、`sort_by` 复用 `apiKeyListOrder` 含 today_cost / last_used_at、
    筛选 user_id / group_id(0=无分组) / status / search，`key` 掩码前 6 + 后 4，登记审计敏感 GET）；`PUT /admin/api-keys/:id` 扩为 `{group_id?, reset_rate_limit_usage?, status?, ip_whitelist?, ip_blacklist?}`
    （IP 走 `ip.ValidateIPPatterns`）；`DELETE /admin/api-keys/:id`（`GetKeyAndOwnerID → DeleteWithAudit → InvalidateAuthCacheByKey`）；
    门禁 `TestAdminAPIKeySurfaceRegistered`（含「不许出现管理员换用量令牌接口」）。第一轮 golangci-lint gofmt 对齐红，修后全绿（CI 34097154880）。
  - **A6-乙 + 甲 + 丙 前端（`da4be4549` / `c076942f2` / `f255ac71f`）**：乙——管理员「我的账户」侧栏去掉 `/usage`，`router/adminRedirect.ts` 把管理员访问 `/dashboard` → `/admin/dashboard`、
    `/usage` → `/admin/usage`（放在 admin 校验之后、backend_mode 之前，`/keys` 不动）；甲——新页 `/admin/api-keys`「密钥总表」（列：名称 / 所属用户 / 密钥掩码 / 分组 / 用量 / 状态 / 最后使用 / 创建 / 操作；
    排序透传；筛选；操作：查用量跳 `/admin/usage?api_key_id=&user_id=`、启停、编辑分组与 IP 名单、删除确认）；丙——`UserApiKeysModal` 加启停 / 删除 / IP 名单。
    `ApiKeysView.spec`（12 例）、`UserApiKeysModal.spec`（5 例）进 CI 名单。CI 34096248631 ✓。
  - **`custom_endpoints` 删除（`bc4b4dc86` / `8e209c54b` / `3bc38c90e`）**：站长决策（生产值为 `[]`）。**迁移 240** `drop_custom_endpoints_setting.sql`（幂等 DELETE）；后端 15 处 + 前端 12 处整键删除
    （含 `safeRawJSONArray` 死 helper、`CustomEndpoint` 类型、设置页卡片 −147 行）；`EndpointPopover` 退化为只显示内置端点；门禁：公开设置反向断言 + 管理端 PUT 带该键**忽略**
    （非 400，依据 gin 不启用 DisallowUnknownFields 且升级窗口旧前端仍会带该键）；`EndpointPopover.spec.ts` 补进 CI 名单。CI 34095380932 ✓。
- **第四轮 `batch6/round4`（`ae47e5564`，基于 round3）**——站长新需求（2026-09-07）：① 使用记录显示每条请求实际走的上游账号；② 账号「容量」格的负载明细（选了甲方案：基于使用记录的「最近请求」抽屉；
  乙方案「真正的在途请求登记」排下一批）。规划核实：管理端使用记录**早已**带账号名列、导出与筛选，只缺「账号格可点、网址参数生效、账号页入口」；
  并发槽位 Redis ZSET 只有开始时间无元数据，系统无逐条在途登记。
  - **后端（`f3bcbd9a1` / `396ca2137` / `03b5d7906`）**：`GET /api/v1/admin/accounts/:id/recent-requests?minutes=15(1..1440)&limit=50(≤200)`（非法 400，账号不存在 404）→
    `{account_id, window_minutes, current_concurrency, max_concurrency, waiting_count(真实的账号等待队列计数), total_requests, items[...], by_api_key[], by_model[]}`；
    聚合走 SQL GROUP BY 整窗、走 `idx_usage_logs_account_created_at`；`by_*.cost` 为账号口径（含账号倍率）；空切片序列化为 `[]`；不含凭据；
    `request_type` 值域 `sync / stream / ws_v2 / cyber / live / unknown`；`upstream_model` omitempty。`AccountSummary` 加 `platform`。门禁 `TestAccountRecentRequestsSurfaceRegistered`。
    FEATURE_CHECKLIST 第 3 节「账号容量负载明细」、第 4 节「使用记录账号可点 / 按账号筛选」两行。CI 34102510497 ✓ 一次全绿。
  - **前端（`3403abe39` / `ec05c82c3`）**：需求 1——`UsageTable` 账号格改按钮 emit `accountClick`，`UsageView` 按账号筛选并回显名称、`applyRouteQueryFilters` 读 `account_id`
    （mount 后 `adminAPI.accounts.getById` 回填名，兜底 `#id`）、`UsageFilters` expose `setAccountKeyword`、账号操作菜单「查看用量」；
    需求 2——通用 `SideDrawer.vue`、`AccountLoadDrawer.vue`（并发 / 等待、5 / 15 / 60 分钟、按密钥 / 按模型 chips、请求表、同期错误段用 `listErrorLogs` 传精确 `start_time / end_time`——
    因为 `parseOpsDuration` 不认 `15m` 会静默退回 1h、延迟提示、刷新）、`AccountCapacityCell` 徽标改按钮（hover 300ms 防抖拉 `minutes=15&limit=1`、30 秒缓存、
    简版「并发 x/y · 最近 15 分钟 N 次 · 最活跃密钥」，点击 emit `openLoad`）、`AccountsView` 接线并把抽屉加入「弹窗打开时暂停自动刷新」清单。
    `AccountCapacityCell.spec`（5）、`AccountLoadDrawer.spec`（8）、`UsageTable.spec`（+2）、`UsageView.spec`（+4）进 CI 名单。CI 34101114427 / 34101935036 ✓。

#### 本轮执行方式（站长指定）

批次 3 + 4 + 5 **一次性做完**，多 Agent **按顺序**执行（非并行，避免同一工作区冲突），
全部完成后推 GitHub CI，CI 全绿再出 RC 镜像，最后在 way-rc 做一次合并验证。

> ⚠️ **已向站长提示并由站长确认承担的风险**：三批一次做完的 diff 极大，且批次 4 改动计费核心、
> 批次 5 改动数据库结构，单轮验证一旦出问题定位成本高于逐批验证。站长已确认按此执行。

**已提示风险与站长选择**（2026-08-28）：

1. ~~不先合并 main~~ —— **该风险不成立，已作废**。当时的判断基于「分支落后 main 257 个提交」，
   而那是与**上游 Wei-Shaw 仓库**的差距，不是与本工程主线 `fork/main` 的差距。
   实测 `fork/main` 领先分支 0 个提交（详见第四节）。**没有需要合并的东西，站长的"不合"选择自然成立。**
2. **核心链路验证接受降级结论**。way-rc 唯一上游账号是假账号（`rc-fake-claude`，status=error），
   站长选择不配真实上游账号。**后果**：验证只能覆盖到"网关侧鉴权 / 分组解析 / 账号选择"这一段，
   已实测确认请求确实发到了 `api.anthropic.com` 并被上游以 401 拒绝（即链路通到上游门口），
   但**"拿到上游正常应答"这一跳始终未经验证**。考虑到批次 4 将改动计费与额度核心、
   而转发链路是本项目命根子，此缺口应在批次 4 合并前由站长以其他方式补验。

---

### 批次 3 / 4 / 5 交付明细（2026-08-29，代码已全部合流到分支）

> 体例同第二节批次 1 与上方批次 2：**写清删了什么、保留了什么、为什么这么切**。
> 三批合计 **757 个文件、20699 行新增、222225 行删除**
> （批次 3：415 文件 / −72944；批次 4：250 文件 / −20091；批次 5：256 文件 / −129353，
> 其中批次 5 的 18423 行新增几乎全是 ent 重新生成的产物）。
> 状态一律是「代码已交付、CI 与 way-rc 验证未做」，**唯一例外见每包末尾的 ⛔ 标注**。

#### 批次 3（低耦合整删，7 个工作包，11 个代码提交 + 1 个文档提交）

**WP3-a｜邮件体系整删（B5 方案 A）** — `6173895b2` `50fddf33e` `371c8ca34`

- 后端整删 **27 个文件**：`email_service` / `email_queue_service` / `email_message` /
  `notification_email_service` / `balance_notify_service` / `content_moderation_email` /
  `auth_email_binding` / `ops_scheduled_report_service` / `subscription_expiry_service` /
  `notify_email_entry` / `repository.email_cache` / `admin.setting_handler_email` /
  `dto.notify_email_entry`，外加 14 个纯邮件测试。
- **认证面**：`AuthService` 去掉两个构造形参并删掉找回密码五件套；
  `forgot-password` / `reset-password` / `settings/email-unsubscribe` 三条路由下线。
- **TOTP 面按 B5-3 的调研结论执行**：`verifyIdentity` 的邮箱验证码分支整删、统一走密码，
  `InitiateSetup` / `Disable` 去掉 `emailCode` 形参。
  **登录用的 `VerifyCode` 与敏感操作 step-up 一行未动** —— 这是当初决定"删邮件不影响 2FA 登录"的前提，
  实施时逐字守住了。
- **告警与计费收尾**：ops 告警评估器只删邮件推送，**告警事件照旧落库**（B5-6 的"面板内有完整替代"）；
  网关计费只拆掉两个异步通知调用点与 `BalanceNotifyService` 依赖，**计费口径零改动**。
- **设置层删 17 个键**（7 个 `smtp_*`、`email_verify_enabled`、`password_reset_enabled`、
  `frontend_url`、余额提醒 3 个、订阅到期提醒、账号额度提醒 2 个、`ops_email_notification_config`），
  新增迁移 **230** 幂等清库（18 个显式键 —— 上面 17 个再加 `notification_email_unsubscribe_secret` ——
  以及 `notification_email_template/preference/delivery/locale` 四个前缀的历史模板行）。
- ⚠️ **一处必须知情的副作用**：`InitializeDefaultSettings` 判断"是否已种过默认设置"的探测键
  从 `email_verify_enabled` 换成了 `registration_email_suffix_whitelist`。
  老库两个键都在，不会重跑种子；但若某个环境的 `settings` 表被手工清过又只留部分键，行为会与以前不同。
- **配套交付 `backend/cmd/adminpass`**（B5-4 建议的兜底工具）：
  `-password` 参数 > `ADMINPASS_NEW_PASSWORD` 环境变量 > `-stdin` 三级取密码，
  bcrypt 走与登录校验同一个 `service.User.SetPassword`，改 `password_hash` 顺带让已签发的
  access token 失效。已加进 `Dockerfile` 与 `deploy/Dockerfile`。
  ⛔ **未加进 `Dockerfile.goreleaser` / `.goreleaser.yaml`，即发版镜像里没有这个工具**，
  详见第四节同名条目 —— **这是本轮唯一一个会影响生产的未完成项**。
  → **已由 `498f7805b` 补上**，v0.1.183 发版镜像实测含 `/app/adminpass`（见第四节「✅ 已解决（原未完成项 2）」）。
- **前端**：删 6 个组件/页面（模板编辑器、ops 邮件卡片、忘记密码页、重置密码页、
  余额提醒卡、账号绑定卡），系统设置整个「邮件」标签页移除（标签从 7 项收敛为 6 项），
  2FA 弹窗只剩密码校验，个人资料的「登录方式绑定」**降级为只读展示**。
- ⚠️ **降级说明**：普通用户从此无法自助更换登录邮箱，管理员改自己的邮箱只能走后台用户管理
  （要求已登录）。真被锁在门外时的唯一出路就是 `adminpass`。

**WP3-b｜批量生图整删 + 内容安全审计整删** — `4c3bc2a68` `109a063e6`

- **批量生图**：handler 1 + service 14 + repository 3 及其测试整删，网关十条 `/v1/images/batches*` 下线；
  `UsageBillingRepository` 去掉三个冻结/结算/解冻方法；分组三个批量字段从六处结构体与两处读写映射摘掉；
  `BatchImageConfig` 48 个字段与 46 条 viper 默认值删除；前端删掉 2691 行的向导页与整套 i18n。
- **保留面刻意守死**：`allow_image_generation`、`image_rate_*`、`image_price_1k/2k/4k` 与
  `/v1/images/generations`、`/v1/images/edits`、异步生图三组路由**一个没动**，
  并新增 `TestRetainedImageSurfaceStillRegistered` 正面锁住。
- **内容安全审计**：service 4 + repository 2 + `securityaudit/coordinator_legacy.go` + handler 整删，
  管理端 `/admin/risk-control` 全组八条路由下线；`Coordinator` 改写为单引擎
  （去掉双引擎并发与 legacy 优先级仲裁）。
- **保留面**：**提示词审计一行未动**；`risk_control_enabled` 键保留 ——
  核查发现它同时是提示词审计的总开关（`securityaudit/prompt_types.go` 与前端 `/admin/prompt-audit`
  路由守卫都认它），删键会连带打死保留面。仅把前后台文案从「风控中心」改写为「安全审计」，
  **键名不动**（改键名要动迁移与存量数据，不值当）。cyber 会话屏蔽开关与 TTL 原样保留。
- ⚠️ **一处行为变化**：`cyber_policy` 命中后不再往 `content_moderation_logs` 写风控记录
  （该表已无写入方，并在批次 5 被 DROP）。用量行、会话屏蔽、ops 错误日志三条记录路径都保留。

**WP3-c｜渠道监控 V1 整删，模式切 V2** — `2d735ca79`

- 后端删 19 个 service 文件（checker / challenge / 调度器 / 聚合视图 / 配额抓取 / 请求模板 + 测试）、
  2 个 repository、2 个 handler，17 条路由下线；V2 的 8 条管理端 + 6 条用户端路由**原样保留**。
- 设置层删 `channel_monitor_mode` / `channel_monitor_default_interval_seconds` /
  `channel_monitor_show_quota` 三键，V2 从此只受 `channel_monitor_enabled` 一个开关控制（fail-closed）。
- **迁移 231 是本轮唯一一条"改值而非删键"的迁移**：用 `INSERT ... ON CONFLICT DO UPDATE`
  把 `channel_monitor_mode` 幂等写成 `'v2'`。新代码根本不读这个键，改值纯粹是为了
  **回滚到旧镜像时旧代码读到 `'v2'` 会走被动聚合，而不是把会烧上游额度的主动探测重新打开**。
  写法上刻意避开了 229 的坑（只写 `UPDATE` 对新库是空操作）。
- 前端删 V1 的两个组件目录、3 个 api 模块与 7 个 spec，`/monitor` 直接进 V2 页。

**WP3-d｜公告整删 + 模型广场整删** — `7ebcb8824` `478b8e767`

- **公告**：后端 12 个文件、8 条路由；前端 8 个文件、顶栏铃铛、用户侧弹窗与已读回执。
  表结构当时刻意留给批次 5（已在 235 里 DROP）。
- **模型广场**：后端 5 个文件、1 条公开路由、3 个设置键（迁移 232 清库）；
  **默认落地页白名单 `allowedDefaultHomePaths` 同步去掉 `/model-plaza`** ——
  那是白名单，留着等于允许把首页指向一个已经不存在的页面。
  而 `reservedEntryPaths`（自定义登录路径黑名单）里的 `/model-plaza` **刻意保留**：
  删黑名单条目只会放宽允许集，属安全收紧项而非裁剪项。

**WP3-e｜第四节小尾巴清账** — `4c9a91120`

- CSP 强制注入表 **27 条收敛为 1 条**（只留 Cloudflare Web Analytics），
  `frame-src` 收紧为 `'none'`，并加了防回归用例锁死这批主机不许回流。
- 删除三份支付遗留文档，并把三个 README 里指向它们的条目一并删掉（避免悬空链接）。
- 删除 `registration_email_domain_quota_enabled`（迁移 233）。
  **核实结论修正**：兄弟键 `registration_email_suffix_whitelist` **必须保留** ——
  它是 `InitializeDefaultSettings` 的种子探测键（见 WP3-a 的副作用说明）。

**WP3-f｜批次 2 验收发现的两条部署侧隐患** — `96f704c11`

- 三个 compose 的镜像行改为 `${SUB2API_IMAGE:-...}`，`.env.example` 补 `SUB2API_IMAGE` 项。
  这正是「way-rc 白验了两天」的根因（第六节 internal-1e49744）。
- 存量库"有用户但一个管理员都没有"时的日志由 INFO 提到 **WARN**，并补上 `adminpass` 自救指引。
  **引导逻辑一行未改**：守卫方向不变，宁可没管理员也不覆盖既有密码。

**WP3-g｜i18n 死文案清理 + Passkey 分隔线修正** — `65bcd073d`

- zh/en 各删 61 条键（OAuth 回调、邀请码、优惠码、兑换、支付、订阅、首页四个营销区块），
  删前逐键 grep 确认零引用，删后两边键集合仍完全一致。
- 顺带修掉批次 2 验收发现的低危项：Passkey 按钮上方的分隔线从 `auth.oauthOrContinue`
  （「或使用其他继续」）改为新增的 `auth.passkeyOrContinue`。

#### 批次 4（A4 订阅/余额拆除 + Key 额度直绑，最难一刀）

提交：`c49cf8e1c`（后端）`97c445ded`（前端）`e86906521` `808982db1`（两个复查修复）

- **额度语义统一到 `api_keys.quota / quota_used` 一处，Key 就是额度本身。**
  认证中间件（Anthropic 与 Gemini 两条入站同步改）只剩「Key 状态 / 过期 / Key 额度」三道闸门。
- 下线用户侧「我的订阅」、管理端「订阅管理」与管理员充值/扣款接口
  （门禁里锁死 16 条精确路径 + `/api/v1/subscriptions`、`/api/v1/admin/subscriptions` 两个前缀）；
  删除订阅域整块与余额写接口（`UpdateBalance` / `DeductBalance` / `AdjustBalance` / `SetBalance`）。
- **注册用户默认值随本批消失**（当初拍板时就判断它与订阅/余额强耦合、技术上必须随 A4 落地，
  实施结果印证了这一点）。
- 高峰时段倍率与订阅类型解绑；专属分组授权取消「订阅型分组直接放行」的旁路，统一按
  `user_allowed_groups` 判定。
- **迁移 234 做三件事保证存量可用**：① 按「已有 Key 绑在该专属分组上」的既成事实回填
  `user_allowed_groups`（补上被取消的那条旁路）；② 把非订阅分组上**原本就不生效**的
  `peak_rate_enabled` 显式关掉，使升级前后计费口径一致；③ 分组 `subscription_type` 归一为
  `standard` 并清掉 8 个已无读写方的设置键。**不删表不删列**，因此批次 4 单独可整体回滚。
- ⚠️ **两项行为变更，必须在 way-rc 上重点复验**（验证项见第五节 4.6）：
  1. `quota_used` 现在**无条件累加**（旧实现只在 `quota > 0` 时累加）。不限额 Key 的
     `quota_used` 会从 0 开始增长 —— 这是"所有 Key 都记账"的前提，但对存量 Key 是可见的行为变化。
  2. Key 额度成为唯一闸门后，超支窗口从「Redis 实时余额」退化为「auth cache TTL + 并发 in-flight」：
     `CheckBillingEligibility` 不再做额度预检，耗尽判定依赖认证缓存快照与结算后的
     `InvalidateAuthCacheByKey`。内部单管理员部署可接受；若将来要收紧，需把 Key 额度接进 billing-cache 预检。
- 两个复查提交修的是**批次 4 自身在无编译器环境下造成的断裂**：误删了仍被 5 处生产代码调用的
  `getAdminIDFromContext`、误删了仍被 9 个存活测试引用的两个 stub、
  以及把上游供应商余额当成用户钱包余额盲删了测试数据。

#### 批次 5（A5 数据库压平 + 前四批的收尾修复）

提交：`292f3fa65`（ent）`a5d5bb6cb`（迁移）`269098f60`（gofmt）`81f7361d0` `5612f64d0` `8d3fe8105`（测试与前端修复）

- **本批第一次在本地跑通了真正的工具链**（go1.26.6 + golangci-lint v2.9 + PostgreSQL 18.1 + pnpm 9，
  全部装在 scratchpad 里、用完即删），所以下面的结论是实测而非静态推断。搭法见第四节。
- 🔴 **开工时的重要发现**：分支上 `go build ./...` **本身就编译不过** ——
  批次 3 删公告时删掉了 `domain.AnnouncementTargeting`，但生成的 `ent/announcement*.go` 还在引用它。
  也就是说批次 3 和 4 之后，**CI 的单测 / 集成 / lint 三条流水线全部会红**。本批一并修好。
- **ent 死实体**：删除 17 个 schema 与全部生成物（支付 3 + 兑换促销 3 + 订阅 2 + 批量生图 3 +
  渠道监控 V1 4 + 公告 2）；User 去 6 条边、Group 去 2 条边 + 3 个批量生图字段、
  UsageLog 去指向 `user_subscriptions` 的边。**ent 是用官方代码生成器重新生成的，不是手工改生成物。**
- **死表与死键**：迁移 **235** DROP 21 张表（含推广返佣 2 张、内容审计日志 1 张、渠道监控 V1 的聚合水位表）；
  **236** DROP `groups` 的 3 个批量生图列；**237** 清掉 50 多个孤儿设置键。
- **刻意保留的三处**：`usage_logs.subscription_id`（repository 裸 SQL 与 DTO 仍在用）、
  `groups.subscription_type`（可用渠道 DTO 之外，migration 193 的 auth cache 触发器函数引用了这一列）、
  `users.balance` 家族（列还在，service 层已不读）。
  另有三个键刻意不删：`registration_enabled`（229 的回滚兜底）、`channel_monitor_mode`（231 的回滚兜底）、
  `registration_email_suffix_whitelist`（种子探测键）。
- **实测验证**：全新库跑完 277 条迁移无报错；用 `fork/main` 的 269 条迁移建出「升级前老库」
  并塞入代表性数据后再升级，28 项断言全过；停在 123 号迁移的老库也能直接升到最新；重复执行为 no-op。
- **收尾修复**（都不是批次 5 引入的，是批次 3/4 在无编译器环境下留下的）：
  23 个文件的真实 gofmt 偏差、5 处测试编译失败、若干失效断言、
  集成测试里对已删表的断言（这批带 `//go:build integration`，本机无 Docker 时静默跳过、
  但 CI 的 runner 有 Docker 会真跑并必然失败 —— 本次专门起了 colima 实跑验证）、
  前端 3 处 typecheck 错误与 1 个全红的 spec。
- **顺带修好一个一直在骗人的护栏**：`Makefile` 的 `FRONTEND_CRITICAL_VITEST` 有 5 条指向已删测试文件。
  vitest 把这些条目当过滤词，指向已删文件不报错、只是**静默少跑** —— 实测 13 条里只有 8 条真正生效。
  已清掉死条目、补进 4 条，并加了存在性校验让下次再删测试时直接报错。
- ⛔ **迁移基线重置：本批未做，留给站长决策。** 这是本轮唯一一个原计划范围内、
  但**明确判定为不做**的事项，理由与最小安全方案见第四节同名条目。**不要把它当成已交付。**

#### 本轮的门禁护栏现状

`backend/internal/server/trimmed_surface_gate_test.go` 已累积到 **637 行**，
覆盖 4 类断言，是"删过的东西不许回流"的唯一自动化保障：

| 断言类型 | 规模 | 说明 |
|---|---|---|
| 精确路径缺席 | **100 条** | 批次 1–4 删掉的每条代表性路由 |
| 前缀兜底 | **20 个前缀** | 换个参数名或子路径加回来同样拦截 |
| 请求级 404 探测 | **46 条** | 防止将来通过 NoRoute/通配符路由复活 |
| **保留面正面锁定** | 6 组 | 认证面 / 系统面 / **普通生图** / **渠道监控 V2** / **提示词审计** / `risk_control_enabled` 公开键 |

保留面的正面断言是本轮特意加的：前缀兜底写宽一格就会误伤保留面
（例如 `/api/v1/channel-monitors` 的末尾 `s` 与 `-templates` 后缀就是刻意的，
少写一个字符就会把保留的 `/channel-monitor-v2` 一起打死）。


---

### B5 邮件体系决策点（调研完成，**已于 2026-08-28 拍板：方案 A**）

以下为实际读码后的结论。**站长已选定方案 A（整删），落在批次 3。**

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

- [x] **方案 A：整删 SMTP 与全部邮件体系** ← **站长 2026-08-28 拍板选定，落在批次 3**
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

- `.gitignore` 对 `docs/*` 为白名单模式，docs 新文件需 `git add -f`
- 本会话 GitHub 集成无 actions:write，workflow 触发走 `internal-rc-*` 标签推送
- ~~迁移 229 为幂等 UPDATE；本轮未删表未改列，镜像回滚无兼容性问题~~ ——
  **该结论仅对批次 1 成立，批次 5 之后已失效**，回滚语义见下方「⛔ 回滚不再是无损操作」
- 4 个基线遗留的未格式化 unit 测试文件：**已随批次 5 的 `269098f60` 用真 gofmt 修掉**，
  当前 `gofmt -l ./...` 全仓无输出
- ⚠️ **本文档中所有「`registration_email_suffix_whitelist` 必须保留」的结论均已作废**
  （涉及第 4.x 节的两段验证命令与批次 3 的副作用说明）。P0 收尾把邮箱域名白名单整条链路
  （前端卡片 + 后端读写 + 设置键）删了，`InitializeDefaultSettings` 的种子探测键改挂
  **`allow_ungrouped_key_scheduling`**，迁移 **238** 清库。
  选它的三个理由：属于分组隔离/调度的核心保留面、没有任何迁移会 `INSERT` 它
  （全新库不会被迁移预写的行误判成"已种过"）、默认值 `false` 是 fail-closed。
  发版核对时**不要**再去 `/api/v1/settings/public` 或 `settings` 表里找这个键。
- 本地仓库状态（2026-09-04）：本地 `main` 原停在上游 `Wei-Shaw/sub2api` 的老提交 `e866ff6ec`（比 `fork/main` 多 257 个上游提交，且全部都在 `origin/main` 里、无本地私有提交），已 `reset --hard fork/main`（`7341cce56`）并把上游跟踪改为 `fork/main`；`merge-to-main` 与 `claude/project-trim-internal-deploy-7jnynw` 均已包含在 `main` 内
- 本地仓库状态（2026-09-07）：`fork/main` = `6fd50990f`（v0.1.186 的 `VERSION` 同步提交，父 `81836e9c8`），本地 `main` 已同步；生产与 way-rc 均在正式 tag `0.1.186`
- 本地仓库状态（2026-09-08）：`fork/main` = `306bcf291`（docs 提交，标 `[skip release]`）；`batch6/round4` HEAD `ae47e5564` 含第二～四轮全部代码，待站长验收后 ff 合并；生产 `0.1.186`，way-rc `internal-ae47e55`
- **裁剪目标已于 2026-09-08 调整为「内部部署、自用多用户」**（多个用户、多个管理员），保留面与仍裁掉的清单见文首目标声明。本节及以下各处早期备注里的「单管理员」是当时表述，不再逐条改写
- ⚠️ **验证方式变更（自 2026-09-05 起）**：编译 / 测试 / lint / 装依赖 / 无头浏览器视觉走查一律走 GitHub CI（backend-ci "CI" + Security Scan 全绿后推 `internal-rc-<sha>` 标签出验证镜像）；服务器 `oci-free` 上只做拉镜像、重建容器、curl / psql 级核对；任何时刻最多一个实施者操作服务器，操作前先看负载。起因见下方「⛔ 2026-09-05 事故」。本节「✅ 本机其实拿得到 Go / golangci-lint / PostgreSQL」讲的是本机 scratchpad 工具链，与这条针对服务器的纪律不冲突

---

### ⛔ 2026-09-05 事故：并行重任务压垮生产机（原因是协调者的调度失误）

**时间**：2026-09-05 约 02:02Z ～ 02:20Z（北京时间 10:02–10:20），`way.flyooo.uk` 与 way-rc 的 HTTPS、以及 SSH 全部超时。

**原因**：协调者同时派了三个实施者在 `oci-free`（2 OCPU / 12 GB，同时托管生产与验证环境及十余个常驻容器）上并行跑重任务：
后端在容器里 `go build` + 全量单测 + golangci-lint；前端 pnpm 装依赖 + vitest + 临时 vite 开发服务器 + **新起一个 browserless Chromium 容器**做视觉走查；
发版线同时在拉镜像。15 分钟平均负载冲到 47。**这是协调者的调度失误**，不是代码或镜像的问题。

**处置**：停掉全部实施者；机器约 15 分钟后自行恢复（无需重启）；杀掉残留容器 `a6s2-vite` / `a6s2-browser` / `a6s2-mock`。

**影响**：生产容器全程未被改动（仍 0.1.184，healthy），数据库表数 / 行数一致；期间应用记录 29 条「查询超时」错误，恢复后无新错误。
v0.1.185 的发版本身已完成（02:22Z），但生产升级步骤在事故前只做到备份——**0.1.185 因此没有单独切换生产**，随 0.1.186 于 2026-09-07 一起生效。

**新纪律**（已写入协调者长期记忆）：

- 编译、测试、lint、装依赖、无头浏览器一律走 GitHub CI；
- `oci-free` 上只允许拉镜像、重建容器、curl / psql 级核对；
- 任何时刻最多一个实施者操作服务器，操作前先看负载。

此后的 A6-2 第二步与密钥页改动全部只用 CI 验证，视觉走查取消。第五节的验证清单与 A7 的压测 / 备份恢复 / Key 泄露演练都涉及这台机器，同样受此约束。

### 2026-09-07 三次实施者中断与接力

批次 6 第二～四轮实施期间（2026-09-07 下午），三次实施者因 API 会话限额（15:10 重置）或流式停顿被中断。
工作通过 worktree 与分块提交保留，由后续实施者接力完成，没有丢失已写好的代码；三轮集成分支的 CI 与 RC 记录见第六节最上方小节。
全程遵守 2026-09-05 事故后的新纪律，**未再发生服务器负载事故**（全程负载 < 1.1）。

### 三个决策结论（2026-09-07 站长拍板）

批次 2 验收起累计的三项未决决策至此全部拍板，第一节「当前位置」与下方台账同步改为已决：

| 决策项 | 结论 | 落点 |
|---|---|---|
| **`custom_endpoints` 设置** | **整键删除**（生产值自上线起一直是 `[]`） | 批次 6 第三轮，迁移 240 清库；后端 15 处 + 前端 12 处一并删 |
| **`security.url_allowlist.enabled`** | **保持关闭**（不开启，启动 WARN 照旧） | 不改代码、不改配置 |
| **`server.trusted_proxies`** | **暂不配置** | 不改配置；真实客户端 IP 的准确性问题接受现状 |

### ⛔ 未完成项 1：迁移基线重置（A5 内，留给站长决策）

**这一项计划里有、本轮明确没做，不要当成已交付。**

不做的理由是实测数据不支持这项改造：全新库跑完 277 条迁移只要 **0.3–0.6 秒**，
"顺序跑历史迁移"并不是真实痛点。而任何形式的基线重置都要么改迁移执行器
（新增"全新库走基线、存量库标记已应用"的分叉逻辑），要么删历史迁移文件 ——
后者会让**停在中间版本的存量库无法升级**（例：删掉建 `batch_image_jobs` 的 159，
而 187 里还有引用它的语句）。

**需要什么条件才能做**：站长明确"愿意为此承担迁移执行器分叉逻辑的复杂度"。
若决定做，建议的最小安全方案是**只加"全新库快速路径"、不删任何历史文件**：

1. 用一次性脚本把 277 条迁移跑进空库后 `pg_dump --schema-only` 生成 `000_baseline.sql`，
   并在其中附带把全部历史迁移写入 `schema_migrations` 的 INSERT；
2. 在 `applyMigrationsFS` 开头判断 `schema_migrations` 是否为空：非空（存量库）
   就把 `000_baseline.sql` 直接标记为已应用而不执行；
3. 加一个集成测试对拍「按历史链建库」与「按基线建库」两份 `pg_dump` 必须一致。

### ✅ 已解决（原未完成项 2）：goreleaser 发版镜像里没有 `adminpass`

**已于合并前解决，并在 2026-09-04 用正式发版镜像实测确认。**

- 修复提交 `498f7805b`（"fix(release): 把 adminpass 一并打进发版镜像"，即第五轮验收镜像 internal-498f780 的来源提交）：
  `.goreleaser.yaml` 的 `builds` 增加 `adminpass` 构建目标（`./cmd/adminpass`），**只随 Docker 镜像分发、不进 archives**
  ——下文顾虑的"改变上游发布产物形状"因此没有发生；`Dockerfile.goreleaser` 增加 `COPY adminpass /app/adminpass`。
- 2026-09-04 实测：正式版 `0.1.183` 镜像内 `/app/adminpass` 存在（36,372,642 字节，在 sub2api-rc 容器里 `ls -la` 核实）；
  旧版 `0.1.182` 镜像内不存在（符合预期）。
- 连带失效的旧结论：第五节 4.8 提醒 2「这个工具目前只在 RC 镜像里」以及本文档各处的「⛔ 不在发版镜像里」自 v0.1.183 起不再成立。

以下为 2026-08-29 建档时的原始分析，保留作背景：

~~**这是本轮唯一一个会影响生产的未完成项。**~~

批次 3 删掉邮件体系后，`adminpass` 是管理员被锁在门外时的唯一兜底工具。它已经加进
`Dockerfile` 与 `deploy/Dockerfile`，**因此 RC 镜像里有**（`branch-docker-image.yml`
用的就是根目录 `./Dockerfile`）。

但**发版链路用的是另一个 Dockerfile**：`release.yml` → goreleaser → `Dockerfile.goreleaser`，
后者只 `COPY sub2api /app/sub2api` 一个预构建二进制，`.goreleaser.yaml` 的 `builds`
也只声明了 `./cmd/server` 一个构建目标。**结果是合并 main 后自动发版出的
`ghcr.io/cherrylover/sub2api:latest`（= 生产实际在跑的镜像）里没有 `/app/adminpass`。**

后果：在 way-rc 上验过的"锁在门外时 `docker exec ... /app/adminpass` 自救"这条路，
**到了生产上并不存在**。而生产恰恰是那个"删掉了自助找回密码"的环境。

**需要什么条件才能做**：给 `.goreleaser.yaml` 的 `builds` 加第二个构建目标（`./cmd/adminpass`），
并在 `Dockerfile.goreleaser` 里加一行 `COPY adminpass /app/adminpass`。
本轮没做的原因是**这会改变上游发布产物的形状**（archives 里会多一个二进制），
属于需要站长点头的范围，不是可以顺手带过的改动。

### ⛔ 回滚不再是无损操作（批次 5 之后，**语义已变，必读**）

批次 1–4 的迁移都只动 `settings` 表的行，所以"换回旧镜像即可回滚"一直成立。
**批次 5 的迁移 235 / 236 打破了这个前提**：

- **235 DROP 了 21 张表**（支付 3 + 兑换码 1 + 优惠码 2 + 推广返佣 2 + 订阅 2 +
  批量生图 3 + 渠道监控 V1 5 + 公告 2 + 内容审计日志 1），一律 `IF EXISTS ... CASCADE`；
- **236 DROP 了 `groups` 的 3 个批量生图列**。

**换回 `0.1.182` 这类旧镜像后，旧代码会去查这些已经不存在的表和列。**
也就是说，跑过 235/236 之后，回滚**必须连数据库一起回滚**（从升级前的 `pg_dump` 恢复），
不能只换 tag 重启。这一条已写进第五节第 7 节的回滚路径，**部署前请务必先确认备份可用**。

反过来，**批次 3 与批次 4 单独回滚仍然安全**（230–234 都不删表不删列），
所以出问题时的定位顺序应该是：先怀疑 235/236，再往前推。

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

### ✅ 本机其实拿得到 Go / golangci-lint / PostgreSQL（批次 5 验证，2026-08-29）

站长的约束是「不要往系统里装东西」，而不是「不能有工具链」。批次 5 用下面这套办法
在 **scratchpad 里** 起了完整的验证环境，全程不碰 `/usr/local`、不改 PATH、不用 brew，
结束后整个目录删掉即可，系统零残留：

1. **Go**：从 `https://go.dev/dl/go1.26.6.darwin-arm64.tar.gz` 下载解压到 scratchpad，
   用一个小包装脚本导出 `GOROOT` / `GOPATH` / `GOMODCACHE` / `GOCACHE`（全部指向 scratchpad）
   再调 `go`。版本与 CI 的 `go.mod` 完全一致。
2. **ent 代码生成**：`go run -mod=mod entgo.io/ent/cmd/ent generate --feature
   sql/upsert,intercept,sql/execquery,sql/lock --idtype int64 ./schema`（在 `backend/ent` 下跑）。
   注意生成器要**先能加载 `ent/schema` 包**，所以顺序必须是「改 schema → 生成」，
   不能先把生成物删掉（`schema/mixins/soft_delete.go` 依赖生成出来的 `ent/intercept`）。
   若 schema 引用了已被删除的 domain 类型，先补一个临时 stub、生成完再删。
   ent v0.14 会自己清理不再需要的生成文件，不用手工删。
3. **golangci-lint**：`GOBIN=<scratchpad>/bin go install
   github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.9.0`，
   与 CI 同版本、直接读仓库里的 `.golangci.yml`。
4. **PostgreSQL**：`io.zonky.test.postgres:embedded-postgres-binaries-darwin-arm64v8:18.1.0`
   这个 Maven 制品解压出来就是可重定位的 Postgres，`initdb` + `pg_ctl` 起在
   `127.0.0.1:55432` 即可（注意 scratchpad 路径太长，unix socket 要用 `-k /tmp/<短目录>`）。
   有了它就能真跑迁移、也能造「升级前的老库」做升级演练。

5. **pnpm / 前端**：`corepack` 起 pnpm 9，`node_modules` 装在 scratchpad 之外的仓库目录
   （`frontend/node_modules`，验证完删掉），即可跑 `lint:check` / `typecheck` / `vitest` / `build`。

**结论**：后续批次不必再靠「脚本模拟 gofmt 对齐」这类替代手段。

### 🔴 教训：批次 3 与 4 是在"分支整体编译不过"的状态下交付的

这是本轮最该记住的一条。批次 3 和 4 全程没有编译器，只能靠 grep、符号扫描、
括号配平和**自写的 gofmt 对齐模拟器**来代替编译。代价在批次 5 拿到真工具链后一次性暴露：

| 类别 | 数量 | 说明 |
|---|---|---|
| **`go build ./...` 直接失败** | 1 处 | 批次 3 删 `domain.AnnouncementTargeting`，但 `ent/announcement*.go` 还在引用 → **CI 三条流水线全红** |
| 测试编译失败 | 5 处 | 构造器实参个数对不上、stub 缺方法、helper 被连带删除 |
| 失效断言 / 被盲删的测试数据 | 7 个文件 | 含一处把上游供应商余额误当用户钱包余额而删掉的断言，会让双币种用例必然失败 |
| 真实 gofmt 偏差 | 23 个文件 | 模拟器识别不出的文件尾空行、连续空行、字面量注释列对齐 |
| 前端 typecheck 错误 | 3 处 | 死变量与死导入 |
| 全红的前端 spec | 1 个文件 5 个用例 | 断言批次 4 已删除的列 |
| 集成测试对已删表的断言 | 3 个文件 | 本机无 Docker 静默跳过，CI runner 有 Docker 会真跑并必挂 |

**这些全部不是"小瑕疵"，其中第一条会让 CI 从批次 3 起就一路全红。**
静态检查能证明"没有明显的悬空引用"，**不能证明"编译得过"**。
后续批次务必先按上一节的办法起工具链，再动手删代码。

### ⚠️ 本轮"本地已验证"的确切含义

批次 5 收尾时（`8d3fe8105` / `5612f64d0`）本地跑通了与 CI 等价的检查，具体是：

- 后端：`go build ./...`、`go build -tags embed ./...`（桩 dist 与真实 dist 各一次）、
  `go vet ./...`、`go test -tags=unit ./...`（88 包全绿）、`go test -tags=integration ./...`
  （**起了 colima 真 Docker，45 个包含 testcontainers 套件全绿**）、
  `golangci-lint v2.9 --max-same-issues=0 --max-issues-per-linter=0` 零 issue、`gofmt -l` 无输出；
- 前端：`lint:check` 0 error、`typecheck` 0 error、`vitest` 179 文件 1304 用例全绿、`pnpm build` 成功；
- shell：`deploy/tests` 四个脚本本机（macOS）全部 passed。

**仍然覆盖不到的**：Docker 镜像实际构建（见下方同名条目）、
以及 goreleaser 那条发版链路 —— 后者恰好就是 `adminpass` 缺失的地方（已由 `498f7805b` 补上，v0.1.183 镜像实测确认）。

### ✅ 分支与 main 无分叉（2026-08-28 核实，更正同日早些时候的错误结论）

**同日早些时候本文档曾记载「分支落后 main 257 个提交、Go 版本分歧、迁移号撞车」，该结论错误，现更正。**

错误原因：当时比对的是 `origin/main` = **上游 `Wei-Shaw/sub2api`**（本仓库 fork 的来源），
而裁剪工程的主线是 `fork/main` = **`CherryLover/sub2api` 的 main**。两者是不同的东西：
与上游的分叉是这个 fork 刻意为之的结果，与本工程的「验证 → 合并」流程无关。

实测（对 `fork/main`）：

| 项 | 实测结果 |
|---|---|
| 共同祖先 | `09dad1023`（2026-08-27，`chore: sync VERSION to 0.1.182`）= `fork/main` 的 HEAD |
| `fork/main` 领先分支 | **0 个提交** |
| 分支领先 `fork/main` | 17 个提交（批次 2 全部工作） |
| Go 版本 | 两边**都是 1.26.6**，无分歧 |
| 迁移号 | **不存在撞车**。`229_force_registration_disabled.sql` 是**批次 1** 的迁移，
`fork/main` 已包含，生产已于 2026-08-27 12:56 应用。上游 main 的 `229_plugins` / `230` / `231` 与本工程无关 |

**结论**：批次 3 开工前**没有需要合并的主线**，`fork/main` 就是分支的直接祖先。
「验的镜像 = 合并后的产物」这个前提**成立**。

### 后续候选项台账（批次 2 起累加，2026-08-29 按批次 3–5 实际交付更新；2026-09-07 追加两条 CI 维护项；2026-09-08 按批次 6 第二～四轮实际交付勾选并补新发现项）

- [x] **CSP 白名单失效条目**（批次 3 已处理）：强制注入表从 27 条收敛为 1 条，只留 Cloudflare Web Analytics；天御国内/国际站、Turnstile、阿里云验证码、Stripe、Airwallex 的条目与默认策略里的对应域名全部删除，`frame-src` 收紧为 `'none'`。新增防回归用例锁死这批主机不许回流
- [x] **`registration_email_domain_quota_enabled`**（批次 3 已处理）：整键删除 + 迁移 233 清库。**核实结论修正**：兄弟键 `registration_email_suffix_whitelist` 的邮箱绑定校验路径已随批次 3 邮件体系整删消失（`AuthService.validateRegistrationEmailPolicy` 零调用点，已一并删除），但该键**必须保留**——它是 `InitializeDefaultSettings` 判断"是否已种过默认设置"的探测键，删掉会导致每次启动重跑种子
- [ ] **`LOGIN_ENTRY_RESERVED_PREFIXES` 的 `/legal`、`/custom`**（前端 SettingsView 与后端 `config/web_entry.go` 镜像）：对应路由已删，但**刻意保留**——这是自定义登录入口的保留字黑名单，删条目只会放宽允许集，属安全收紧项而非裁剪项。若要收窄需前后端同步改并调整 `web_entry_test.go` 用例。批次 3 删模型广场时同理保留了 `/model-plaza`（但把它从**默认落地页白名单** `allowedDefaultHomePaths` 里删掉了，那是白名单，留着等于允许把首页指到已删页面）
- [x] **支付遗留文档**（批次 3 已处理）：三份文档删除，三个 README 里指向它们的「内置支付系统」条目与生态表格行一并删掉
- [x] **`custom_endpoints` 设置**（批次 6 / 包 D 已处理，2026-09-07）：站长确认生产值自上线起一直是空数组 `[]`，决定整键删除——站点设置卡片、Keys 页 `EndpointPopover` 的自定义端点渲染、后端读写校验 / 审计比对 / 公开设置输出 / 默认种子一并移除，迁移 240 清库（幂等 DELETE）。Keys 页只剩当前站点 origin 这一条内置端点（复制 / 测速保留）。门禁加反向断言（公开设置与默认种子都不含该键）；管理端保存带该键按未知字段**忽略**而非 400（旧前端全量保存不致失败），有用例锁定
- [x] **ent 中的支付实体**（`payment_orders`/`payment_provider_instances` 等）（批次 5 已处理）：连同另外 14 个死实体一并删除，**用官方生成器重跑 ent generate**，配套迁移 235 DROP 了对应的 21 张表。历史迁移文件本身**未删**（删了会让停在中间版本的老库升不上来），见「⛔ 未完成项 1」
- [x] **余额变动记录入口仍指向已删接口**（批次 3 新发现；**批次 6 第二轮 WP-C1 已处理**，`20e6d5193`）：复核结论是点击行为本来就是按用户下钻筛选，与充值记录无关——tooltip 改为「点击下钻筛选」新键 `admin.usage.clickToFilterByUser`，`admin.users.balanceHistory*` 充值记录一组 12 个文案整组删除
- [x] **`SettingsView.spec.ts` 的 vue-i18n mock 字典与 `baseSettingsResponse` 仍带已删字段**（批次 3 新发现；**批次 6 第二轮 WP-C1 已处理**）：删掉 59 个已删字段与 payment 桩，测试桩不再携带已删功能的键
- [x] **`admin.groups.claudeMaxSimulation` 中英键路径不一致**（批次 3 新发现；**批次 6 第二轮 WP-C1 已处理**）：不是修路径，而是核实零引用后把中英整块删除
- [x] **`config` 层的 `server.frontend_url` 是纯死配置项**（批次 3 新发现；**批次 6 第二轮 WP-C2 已处理**，`55734db0e`）：字段、viper 默认值、URL 校验、`warnIfInsecureURL` 死 helper、`config_test` 用例与 `deploy/config.example.yaml` 一并删除
- [x] **审计日志路由白收一个 step-up 参数 + `GET /user/totp/verification-method` 死端点**（FEATURE_CHECKLIST 残留表原有项；**批次 6 第二轮 WP-C2 已处理**，`55734db0e`）：`registerAuditLogRoutes` 去掉被 `_` 忽略的参数；死端点删 handler + service，并加门禁缺席断言（way-rc 实测 404，对照 `/user/totp/status` 401）
- [ ] **ops 告警规则的 `notify_email` 与告警事件的 `email_sent` 字段仍在**（批次 3 新发现）：后端结构体（`ops_alert_models.go`）与 DB 列都保留，只是前端不再提供勾选、新建默认 `false`，且永远不会再被写成 `true`。存量为 `true` 的行不会被改写，但已无任何消费方
- [ ] **`usage_logs.subscription_id` / `groups.subscription_type` / `users.balance` 家族三处刻意保留**（批次 5 决策）：分别因为 repository 裸 SQL 与 DTO 仍在读、migration 193 的 auth cache 触发器函数引用了该列、以及"列还在但 service 层已不读"。都不是漏删，要动需连带改触发器与 DTO 契约
- [ ] **`registration_enabled` 与 `channel_monitor_mode` 两个键刻意不删**（批次 5 决策）：它们分别是迁移 229 与 231 留下的**回滚兜底**（回滚到旧镜像后旧代码读到这两个值才不会退化成"开放注册"和"重开主动探测"）。等回滚窗口过去后可以单独清理
- [x] **`Makefile` 的 `FRONTEND_CRITICAL_VITEST` 名单失效**（批次 5 已处理）：5 条指向已删测试文件。vitest 把条目当过滤词，指向已删文件**不报错、只是静默少跑**——实测 13 条里只有 8 条真正生效。已清死条目、补进 4 条，并加了存在性校验，下次再删测试会直接 exit 1
- [x] **`security.url_allowlist.enabled=false`**（批次 2 验收发现；**2026-09-07 站长拍板：保持关闭**，决策项关闭）：SSRF / URL 白名单校验整体关闭，只剩最小格式校验，启动日志有 WARN。不是裁剪引入的；决策是不开启，不改代码不改配置
- [x] **`server.trusted_proxies` 未配置**（批次 2 验收发现；**2026-09-07 站长拍板：暂不配置**，决策项关闭）：way-rc 前面是 Cloudflare + Traefik 两层代理，不配这个拿不到可靠的真实客户端 IP，影响 IP 管理 / 风控 / 限流的准确性；接受现状
- [ ] **CI：`docker/*` actions 目标 Node 20 弃用告警**（批次 6 发现，2026-09-07 记录，未处理）
- [ ] **CI：推 `internal-rc-*` 标签会重复触发 CI 与 Security Scan**（批次 6 发现，2026-09-07 记录，未处理）
- [ ] **前端零引用的死类型与文案**（批次 6 第二轮 WP-C1 顺手发现，2026-09-08 记录，未处理）：`types/index.ts` 里 `ProxyNode / ConversionRequest / ConversionResult / SubscriptionStats / UserStats` 零引用；`admin.users` 下 `depositSuccess / withdrawSuccess / failedToDeposit / failedToWithdraw / useDepositWithdrawButtons / amountRequired` 零引用
- [ ] **`KeysView.vue` 里只写不读的 `publicSettings` / `loadPublicSettings()`**（批次 6 第三轮删自定义端点后暴露，2026-09-08 记录，未处理）：`custom_endpoints` 删除后 `EndpointPopover` 不再需要公开设置，但 `KeysView.vue` 仍在 mount 时拉一次 `getPublicSettings()` 存进 `publicSettings`，全文件无读取方
- [ ] **FEATURE_CHECKLIST 的 P1 / P2 未勾项仍未处理**（状态未变，此处只做索引）：ops 三开关无 UI、`OpsRuntimeSettingsCard` 孤儿组件、`revoke-all-sessions` / `system/restart` / `ws/qps` 无 UI、data-management 14 条接口无前端、`ProfileIdentityBindingsSection` 只读
- [ ] **乙方案：真正的在途请求登记**（批次 6 第四轮候选，2026-09-08 记录）：现在并发槽位的 Redis ZSET 只有开始时间、无元数据，系统没有逐条在途登记；甲方案（基于使用记录的「最近请求」抽屉）已交付，乙方案排下一批

---

## 五、内部 RC 镜像部署验证清单

> ⚠️ **2026-09-07 提醒**：way-rc 验证环境与生产 `way.flyooo.uk` 在同一台机器（`oci-free`，2 OCPU / 12 GB）。
> 按本清单部署 / 核对时遵守第四节「⛔ 2026-09-05 事故」定下的新纪律：服务器上只做拉镜像、重建容器、curl / psql 级核对；
> 编译、测试、lint、装依赖、无头浏览器一律走 GitHub CI；任何时刻最多一个人操作服务器，操作前先看负载。

本清单面向站长，用于在内网实际部署验证"内部部署（自用多用户）"裁剪分支的镜像（2026-09-08 起的表述；此前写作"单管理员内部部署"）。
镜像由 `.github/workflows/branch-docker-image.yml`（手动触发）构建，只推 GHCR：

- 浮动标签：`ghcr.io/cherrylover/sub2api:internal-rc`（每次分支构建会顶掉，验证期 compose 固定引用它）
- 不可变标签：`ghcr.io/cherrylover/sub2api:internal-<short_sha>`（定位/回滚到具体提交用）

验证内容分三块：空库全新部署、裁剪面验证、老库升级演练。全部通过后把结论告知协调方即可，
合并 main 与 auto-release 由协调方执行。

> **⚠️ 本清单已按批次 3 + 4 + 5 的实际交付更新（2026-08-29）。** 与批次 1/2 相比有四处实质变化：
>
> 1. **第 4 节裁剪面探测大幅扩充**：新增邮件 / 批量生图 / 内容审计 / 公告 / 模型广场 /
>    渠道监控 V1 / 订阅与余额七组 404 探测，以及被删设置键的库内核对（4.5）。
> 2. **新增 4.6「批次 4 两项行为变更复验」** —— 这是本轮**最需要人盯的一节**，
>    因为它改的是计费核心，且两项都是"能跑通但语义变了"的那种变化，探测不出来，只能观察。
> 3. **新增 4.7「`adminpass` 兜底工具实测」** —— 删掉自助找回密码后，这是唯一的自救通道，
>    **必须在 RC 上真跑一次**。⚠️ 同时注意第四节的「⛔ 未完成项 2」：这个工具**不在发版镜像里**
>    （→ 已解决：v0.1.183 起发版镜像含 `/app/adminpass`，见第四节「✅ 已解决（原未完成项 2）」）。
> 4. **第 7 节回滚路径重写** —— 批次 5 删了 21 张表和 3 个列，**回滚不再是换 tag 重启那么简单**。
>
> 另注意第一节的 ⚠️ 生产影响：**批次 3+4+5 必须先在 way-rc 验证通过才能合并 main**。

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

#### 2.1 指定镜像坐标

**批次 3 起不要再改 `image:` 行。** 三个 compose 的镜像行统一写成
`image: ${SUB2API_IMAGE:-ghcr.io/cherrylover/sub2api:latest}`，验证期只需在 `.env`
里设一行：

```bash
# deploy/.env
SUB2API_IMAGE=ghcr.io/cherrylover/sub2api:internal-rc
```

不设这一行就会拿到 `:latest`（最近一次**正式发版**的镜像，不是 RC 分支构建）——
way-rc 就是这么白验了两天。要钉死到某个提交用
`ghcr.io/cherrylover/sub2api:internal-<short_sha>`。

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
4. **注册面已整体消失**（批次 2 起注册体系被删除，不再是"开关默认关闭"）：
   系统设置（`/admin/settings`）里**不再有"允许注册"这一项**，接口层按下面的
   「新库 / 老库」两分支核对（下面的命令用到 `BASE`，先 `export BASE=http://127.0.0.1:8080`）。

> **⚠️ 这一条在批次 2 验收时被发现"验收标准本身写错了"，此处按新库 / 老库拆开重写。**
>
> 根因：迁移 `229_force_registration_disabled.sql` 全文只有
> `UPDATE settings SET value='false' WHERE key='registration_enabled'`，**没有 INSERT/UPSERT**。
> 而批次 2 之后种子里也不再写这个键。所以：
>
> | 环境 | `settings` 表里有没有 `registration_enabled` 行 | 229 的 UPDATE | 正确的验收预期 |
> |---|---|---|---|
> | **新库**（空库全新部署） | **没有**（种子不再写、229 也不 INSERT） | 命中 **0 行**，属正常 | **不要去查这一行**，查了必然查不到，别当成失败 |
> | **老库**（存量库升级） | **有**（旧版本种下的） | 命中 1 行，改为 `false` | `SELECT value` 应为 `false` |
>
> 无论新库老库，**真正的安全闭环都不靠这个开关**，而靠下面三条运行时事实
> （批次 2 已在 way-rc 实测确认：即使把开关手动置成 `true`，注册接口仍是 404）：

```bash
# ① 路由不存在（新库老库都一样，且与开关取值无关）
curl -s -o /dev/null -w '%{http_code}\n' -X POST "$BASE/api/v1/auth/register" \
  -H 'Content-Type: application/json' -d '{"email":"probe@example.com","password":"probe12345"}'
# 预期输出：404

# ② 公开设置里不再下发这个键
curl -s "$BASE/api/v1/settings/public" | jq -e '.data | has("registration_enabled") | not'
# 预期输出：true

# ③ 只在老库上做：229 确实把存量值改掉了
docker compose exec postgres psql -U sub2api -d sub2api \
  -c "SELECT key, value FROM settings WHERE key = 'registration_enabled';"
# 老库预期：value = false
# 新库预期：0 行 —— 这是正确结果，不是失败
```

（`registration_enabled` 这一行在批次 5 的迁移 237 里**刻意没有删除**，
因为它是"回滚到旧镜像后不会退化成完全开放注册"的兜底，见第四节。）

---

### 4. 裁剪面验证

以下命令中 `BASE=http://127.0.0.1:8080`，逐条执行并核对预期。

> **本节按批次 3 + 4 + 5 的实际交付重写。** 探测清单与代码里的门禁测试
> （`backend/internal/server/trimmed_surface_gate_test.go`，637 行）保持一致 ——
> 门禁测试在 CI 里跑的是"路由表里没有"，这里跑的是"真实镜像上确实 404"，两者互补。

#### 4.1 注册接口已整体消失（404，不再是 403）

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X POST "$BASE/api/v1/auth/register" \
  -H 'Content-Type: application/json' \
  -d '{"email":"probe@example.com","password":"probe12345"}'
# 预期输出：404
```

> **批次 1 的旧预期是 403 `REGISTRATION_DISABLED`（路由保留、开关拦截），已作废。**
> 批次 2 起注册体系整体删除，返回 404 且**与 `registration_enabled` 取值无关**
> （way-rc 已实测：手动把开关置 `true` 仍是 404）。
> 该键在新库上根本不存在，详见上方第 3 节的「新库 / 老库」表。

同时确认：**存量用户仍能正常登录**（删的是注册路径，不是多用户数据）。

#### 4.2 已裁剪路由全部 404

分七组探测，覆盖批次 1 到批次 4 的全部裁剪面。**建议整段贴进终端一次跑完。**

```bash
probe() { printf '%-52s %s\n' "$2 $1" "$(curl -s -o /dev/null -w '%{http_code}' -X "$2" "$BASE$1")"; }

echo "--- 批次 1：支付 / 卡密 / 第三方 OAuth ---"
probe /api/v1/payment/config              GET
probe /api/v1/redeem/history              GET
probe /api/v1/auth/oauth/linuxdo/start    GET
probe /api/v1/admin/users/1/balance-history GET

echo "--- 批次 2：注册 / 合规确认 ---"
probe /api/v1/auth/register               POST
probe /api/v1/auth/send-verify-code       POST
probe /api/v1/admin/compliance            GET

echo "--- 批次 3：邮件体系 ---"
probe /api/v1/auth/forgot-password        POST
probe /api/v1/auth/reset-password         POST
probe /api/v1/settings/email-unsubscribe  GET
probe /api/v1/admin/settings/test-smtp    POST
probe /api/v1/admin/settings/email-templates GET
probe /api/v1/user/notify-email/send-code POST
probe /api/v1/user/account-bindings/email/send-code POST
probe /api/v1/user/totp/send-code         POST

echo "--- 批次 3：批量生图（普通生图是保留面，见 4.4）---"
probe /v1/images/batches                  POST
probe /v1/images/batches/models           GET

echo "--- 批次 3：内容安全审计（提示词审计是保留面，见 4.4）---"
probe /api/v1/admin/risk-control/config   GET
probe /api/v1/admin/risk-control/logs     GET

echo "--- 批次 3：公告 / 模型广场 ---"
probe /api/v1/announcements               GET
probe /api/v1/admin/announcements         GET
probe /api/v1/model-plaza                 GET

echo "--- 批次 3：渠道监控 V1（V2 是保留面，见 4.4）---"
probe /api/v1/admin/channel-monitors      GET
probe /api/v1/admin/channel-monitor-templates GET
probe /api/v1/channel-monitors            GET

echo "--- 批次 4：订阅体系与用户余额 ---"
probe /api/v1/subscriptions               GET
probe /api/v1/subscriptions/active        GET
probe /api/v1/admin/subscriptions         GET
probe /api/v1/admin/users/1/balance       POST
probe /api/v1/admin/users/1/subscriptions GET

# 预期：以上每一行都是 404
```

> ⚠️ **带管理员 token 复测一遍**（批次 2 验收时的做法）：不带 token 的 404 有可能只是被鉴权挡住，
> 带上 token 仍是 404 才能证明路由确实被摘除。
>
> ⚠️ **两个容易误判的点**（批次 2 验收踩过，记在这里避免重复排查）：
> `GET /admin/compliance`（没有 `/api/v1` 前缀）返回 **200 是正常的**，那是 SPA 的 HTML 兜底路由；
> 后端真实路径 `/api/v1/admin/compliance` 才是 404。同理 `GET /api/v1/admin/dashboard` 返回 404
> 也不是裁剪导致，后台实际用的是 `/api/v1/admin/dashboard/snapshot-v2` 等子路径。

#### 4.3 公开设置不再泄露已裁剪功能键

```bash
curl -s "$BASE/api/v1/settings/public" | jq -e '.data as $d
  | ([ "payment_enabled",
       "linuxdo_oauth_enabled","dingtalk_oauth_enabled","wechat_oauth_enabled",
       "oidc_oauth_enabled","github_oauth_enabled","google_oauth_enabled",
       "promo_code_enabled","invitation_code_enabled","affiliate_enabled",
       "registration_enabled","registration_email_domain_quota_enabled",
       "turnstile_enabled","tencent_captcha_enabled","aliyun_captcha_enabled",
       "login_agreement_enabled","site_name","site_logo","api_base_url",
       "email_verify_enabled","password_reset_enabled",
       "balance_low_notify_enabled","account_quota_notify_enabled",
       "model_plaza_enabled","model_plaza_require_auth",
       "channel_monitor_mode","channel_monitor_show_quota",
       "default_balance","default_subscriptions",
       "purchase_subscription_enabled" ]
     | map(. as $k | $d | has($k)) | any) | not'
# 预期输出：true（上述键一个都不存在）
```

**同时确认保留面仍在下发**（防止误删）：

```bash
curl -s "$BASE/api/v1/settings/public" | jq -e '.data
  | has("registration_email_suffix_whitelist") and has("risk_control_enabled")'
# 预期输出：true
#  - registration_email_suffix_whitelist 是默认设置种子的探测键，删了会导致每次启动重跑种子
#  - risk_control_enabled 现在是「提示词审计」的总开关（内容审计删除后语义变了，键名未改）
```

#### 4.4 保留面正面核对（**比 404 探测更重要，别跳过**）

本轮删得很深，几组保留面与被删面只差一个前缀，误删的代价比漏删大得多。
逐条确认下面这些**必须仍然可用**：

```bash
# probe() 沿用 4.2 里定义的那个；换终端的话先重新定义一次
echo "--- 普通生图与异步生图（只删了批量生图）---"
# 预期：以下每一条都「不是 404」（缺鉴权/参数时返回 401/400 都算通过）
probe /v1/images/generations              POST
probe /v1/images/edits                    POST
probe /v1/images/generations/async        POST
probe /v1/images/edits/async              POST
probe /v1/images/tasks/t_1                GET

echo "--- 渠道监控 V2 被动聚合（只删了 V1 主动探测）---"
# 同上，预期「不是 404」
probe /api/v1/channel-monitor-v2/snapshot GET
probe /api/v1/channel-monitor-v2/matrix   GET
probe /api/v1/admin/channel-monitor-v2/config GET
```

> 这几条对应代码里的 `TestRetainedImageSurfaceStillRegistered` 与
> `TestRetainedChannelMonitorV2SurfaceStillRegistered` 两个正面锁定用例。
> **拿到 404 就说明保留面被误删了，比任何漏删都严重。**

浏览器侧逐项确认：

- **登录页**：只有 邮箱+密码（以及可选的 Passkey 入口），无任何第三方按钮，
  **也不再有"忘记密码"链接**（批次 3 起该通道整体消失）。
- **`/profile`**：TOTP 可正常开启与关闭，**弹窗里只有密码校验、没有邮箱验证码输入框**；
  「登录方式绑定」是**只读展示**（这是批次 3 刻意的降级，不是 bug）。
- **管理端侧边栏**：「安全审计」分组下只剩「提示词审计」一个子项（内容审核已删）；
  **没有**公告管理、订阅管理、模型广场入口。
- **`/admin/settings`**：标签页共 **6 个**，**没有「邮件」标签页**；
  「功能开关」里渠道监控只剩「启用渠道监控」与「对用户隐藏吞吐速率」两项
  （模式二选一 / 默认检测间隔 / 展示渠道用量三项已删）。
- **`/admin/users`**：**没有**「订阅」「余额」两列，也没有充值/扣款菜单项。
- **`/admin/groups`**：**没有**「订阅计费」列与订阅类型/日周月限额表单；
  高峰时段倍率现在对所有分组都显示（不再只对订阅分组）。
- **`/monitor`**：直接进 V2 被动监控页，不再有 V1/V2 切换。

#### 4.5 被删设置键已从库里清干净

批次 3–5 共新增 8 条迁移（230–237），删掉 **62 个**已无消费方的设置键
（数字来自 `8d3fe8105` 的一致性核对：这 62 个键在 Go、前端、yaml/sh/json 里已零残留）。
升级后在库里核对一遍：

```bash
docker compose exec postgres psql -U sub2api -d sub2api -c "
SELECT key FROM settings WHERE
     key LIKE 'smtp\_%'          OR key LIKE 'payment\_%'
  OR key LIKE 'promo\_%'         OR key LIKE 'auth_source_default\_%'
  OR key LIKE 'notification_email\_%'
  OR key IN ('email_verify_enabled','password_reset_enabled','frontend_url',
             'balance_low_notify_enabled','account_quota_notify_enabled',
             'subscription_expiry_notify_enabled','ops_email_notification_config',
             'model_plaza_enabled','model_plaza_require_auth','model_plaza_description',
             'channel_monitor_default_interval_seconds','channel_monitor_show_quota',
             'registration_email_domain_quota_enabled',
             'content_moderation_config','default_balance','default_subscriptions');"
# 预期：0 行
```

**同时确认三个刻意保留的键还在**（删了会出问题，见第四节）：

```bash
docker compose exec postgres psql -U sub2api -d sub2api -c "
SELECT key, value FROM settings WHERE key IN
  ('registration_enabled','channel_monitor_mode','registration_email_suffix_whitelist');"
# 老库预期：3 行，且 registration_enabled=false、channel_monitor_mode=v2
# 新库预期：channel_monitor_mode=v2（迁移 231 是 UPSERT，新库也会补上这一行）
#           registration_enabled 可能不存在——正常，见第 3 节的新库/老库表
```

#### 4.6 渠道监控已切到 V2 并真的在聚合

V1 主动探测（会真实消耗上游额度）已整体删除，V2 被动聚合成为唯一实现。
除了上面的键值核对，还要确认它**确实在工作**：

1. 按第 5 节发几条真实转发请求；
2. 打开 `/monitor`（或管理端渠道监控页），确认这几条请求被聚合进了 V2 视图；
3. 确认**没有任何指向上游的额外探测请求**（V1 的行为是定时主动打上游，V2 只汇总真实流量）。

> 若 V2 页面显示"总开关已关闭"，检查 `channel_monitor_enabled`（默认 `true`）——
> 注意现在**只受这一个开关控制**，`channel_monitor_mode` 已经没有任何代码读它。

#### 4.7 批次 4 的两项行为变更复验（**本轮最需要人盯的一节**）

这两项**探测不出来**，只能观察。它们改的是计费核心，且都是"能跑通但语义变了"的变化。

**① `quota_used` 现在无条件累加**

旧实现只在 `quota > 0` 时累加，所以不限额 Key 的 `quota_used` 永远是 0。
现在所有 Key 都记账。

```bash
# 建一个「不限额度」的 Key（quota=0），发几条请求后：
docker compose exec postgres psql -U sub2api -d sub2api \
  -c "SELECT id, name, quota, quota_used FROM api_keys ORDER BY id;"
# 预期：quota=0 的 Key，quota_used 也在增长（旧版本这里恒为 0）
# 关键确认：quota=0 仍然表示「不限额度」，不会因为 quota_used > quota 就被判定耗尽
```

**必须确认的反面**：不限额 Key **不会**被误判为额度耗尽而拒绝请求。
这是本条唯一的真实风险 —— 连发几十条请求确认一直正常转发。

**② 额度耗尽的判定窗口变宽了**

Key 额度成为唯一闸门后，`CheckBillingEligibility` 不再做额度预检，
耗尽判定依赖「认证缓存快照 + 结算后的缓存失效」。
**后果：从额度耗尽到实际拒绝之间，存在一个「auth cache TTL + 并发 in-flight」的超支窗口。**

```bash
# 建一个额度很小的 Key（比如 quota 只够 1-2 次请求），连续发 10 条：
# 预期：前几条成功，之后开始返回额度耗尽错误
# 可接受：超出配额一点点（这就是上面说的超支窗口，内部自用部署下可接受）
# 不可接受：一直不拒绝，或者额度还有很多就开始拒绝
```

#### 4.8 `adminpass` 兜底工具实测（**删掉自助找回密码后的唯一自救通道**）

```bash
# 在 RC 容器里给管理员改密码（用 stdin，避免密码进入命令行历史与进程列表）
docker compose exec -T sub2api /app/adminpass -email admin@sub2api.local -stdin <<<'新密码'
# 预期：打印成功信息；随后用新密码能登录后台

# 不传 -email 时改第一个管理员账号：
docker compose exec -T sub2api /app/adminpass -stdin <<<'新密码'
```

> ⚠️ **两条必须知道的事**：
>
> 1. **改密码只让 access token 失效，不清 Redis 里的 refresh token。**
>    旧的 refresh token 在 TTL 内理论上还能换新 access token。
>    要彻底下线所有设备，改完密码登录后还得在后台点一次「撤销全部会话」。
> 2. ⛔ **这个工具目前只在 RC 镜像里，不在发版镜像（`:latest`）里** ——
>    见第四节「⛔ 未完成项 2」。也就是说这一条在 way-rc 上验过之后，
>    **到了生产上并不成立**。建议合并前先把 goreleaser 那条链路补上，否则这次验证的意义有限。
>    → **已解决**（`498f7805b`）：v0.1.183 发版镜像内 `/app/adminpass` 实测存在（2026-09-04），
>    见第四节「✅ 已解决（原未完成项 2）」。

#### 4.9 登录页无第三方登录按钮

浏览器打开 `/login`：只有 邮箱+密码 登录（以及可选的 Passkey 登录入口），
无 LinuxDo / 微信 / 钉钉 / OIDC / GitHub / Google 任何第三方按钮，
也无人机验证组件、无登录条款勾选、无"忘记密码"链接。

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
6. **额度记账正确**（批次 4 新增）：这条请求应让该 Key 的 `quota_used` 增长，
   **无论这个 Key 有没有配额度**。同时确认 `/v1/usage` 返回的 `remaining`
   在未配额度时是 `-1`（不限额度），`planName` 取的是分组名 —— 钱包余额语义已经没有了。

> ⚠️ **批次 2 验收遗留、本轮仍未闭环的缺口**：way-rc 唯一的上游账号是假账号
> （`rc-fake-claude`，status=error），实测能确认请求确实发到了 `api.anthropic.com`
> 并被上游以 401 拒绝 —— 即**网关侧鉴权 / 分组解析 / 账号选择这一段是通的**，
> 但**"拿到上游正常应答"这一跳始终未经验证**。
> 批次 4 恰恰改动了计费与额度核心，**建议这一轮务必配一个真实上游账号把第 5 节走完**，
> 否则本轮最该验的东西反而是空白。

---

### 6. 老库升级演练（如有存量库）

**用备份副本演练，不要动生产卷。** 另注意：`deploy/docker-compose.yml` 固定了容器名
（`sub2api` / `sub2api-postgres` / `sub2api-redis`），同一台机器上无法与原环境同时双开，
请在另一台机器演练，或临时改掉副本编排里的 `container_name`。

> 🔴 **本轮这一节的重要性远高于前几批。** 批次 5 的迁移 **235 会 DROP 21 张表、236 会 DROP 3 个列**，
> 这是整个裁剪工程里第一次做**不可逆的结构变更**。演练前请确认备份**真的能恢复**
> （不是"文件存在"，是"导入回去能起得来"），因为出问题后的回滚必须连库一起回滚，见第 7 节。

```bash
# 1) 在旧环境导出备份
docker exec sub2api-postgres pg_dump -U sub2api -d sub2api > sub2api-backup.sql

# 2) 在演练环境（.env 里已把 SUB2API_IMAGE 设成 :internal-rc）先只启动库并导入
docker compose up -d postgres redis
docker compose exec -T postgres psql -U sub2api -d sub2api < sub2api-backup.sql

# 3) 启动应用（首次启动会自动执行嵌入的 SQL 迁移，本轮新增 230–237 共 8 条）
docker compose up -d sub2api
docker compose logs -f sub2api   # 观察启动无 error
```

**6.1 八条新迁移全部执行**

```bash
docker compose exec postgres psql -U sub2api -d sub2api -c "
SELECT filename, applied_at FROM schema_migrations
 WHERE substring(filename from 1 for 3) IN
   ('230','231','232','233','234','235','236','237')
 ORDER BY filename;"
# 预期：返回 8 行，applied_at 都落在本次启动窗口内
#   230 邮件体系设置键   231 渠道监控切 V2      232 模型广场设置键
#   233 注册域名额度键   234 订阅/余额语义拆除  235 DROP 21 张死表
#   236 DROP 分组 3 个死列                      237 清 50 多个孤儿设置键
```

**6.2 死表确实没了，保留面的表还在**

```bash
docker compose exec postgres psql -U sub2api -d sub2api -c "
SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename IN
 ('payment_orders','payment_audit_logs','payment_provider_instances',
  'redeem_codes','promo_codes','promo_code_usages',
  'user_affiliates','user_affiliate_ledger',
  'subscription_plans','user_subscriptions',
  'batch_image_jobs','batch_image_items','batch_image_events',
  'channel_monitors','channel_monitor_histories','channel_monitor_request_templates',
  'channel_monitor_daily_rollups','channel_monitor_aggregation_watermark',
  'announcements','announcement_reads','content_moderation_logs');"
# 预期：0 行（以上就是 235 DROP 掉的全部 21 张表）

docker compose exec postgres psql -U sub2api -d sub2api -c "
SELECT count(*) FROM pg_tables WHERE schemaname='public'
   AND tablename LIKE 'channel_monitor_v2%';"
# 预期：> 0 —— V2 被动监控的表是保留面，绝不能被一起删掉
```

**6.3 存量业务数据一条不少**（批次 2 验收时生产是 users=3 / api_keys=9 / accounts=7 /
groups=7 / usage_logs=4018，按你自己升级前的实际值对拍）

```bash
docker compose exec postgres psql -U sub2api -d sub2api -c "
SELECT
  (SELECT count(*) FROM users)      AS users,
  (SELECT count(*) FROM api_keys)   AS api_keys,
  (SELECT count(*) FROM accounts)   AS accounts,
  (SELECT count(*) FROM groups)     AS groups,
  (SELECT count(*) FROM usage_logs) AS usage_logs;"
# 预期：与升级前逐项一致。usage_logs 尤其要盯——235 的 CASCADE 只会摘掉
#      usage_logs.subscription_id 指向 user_subscriptions 的外键约束，
#      不动列、不动数据、不删行。
```

**6.4 批次 4 的迁移 234 按预期回填了授权**（否则存量 Key 会因为专属分组失去旁路而失效）

```bash
docker compose exec postgres psql -U sub2api -d sub2api -c "
SELECT count(*) FROM user_allowed_groups;"
# 预期：> 0，且升级后所有存量 API Key 仍能正常转发（用第 5 节的方式实发验证）
```

**6.5 注册开关**（按上方第 3 节的「新库 / 老库」两分支核对，老库这一支才适用）

```bash
docker compose exec postgres psql -U sub2api -d sub2api \
  -c "SELECT key, value FROM settings WHERE key = 'registration_enabled';"
# 老库预期：value = false
```

**6.6 其余逐项确认**

- 原有管理员账号可正常登录（存量库已有管理员时，AUTO_SETUP 会跳过引导、不覆盖原密码，
  日志出现 INFO "Admin user already exists, skipping admin bootstrap"）；
  若库里有用户但**一个管理员都没有**，批次 3 起会打 WARN
  "Warning: database already has user data but no admin account..."，
  按提示用 `docker exec <container> /app/adminpass -email you@example.com -stdin` 自救；
- 原有分组 / API Key / 用量数据在后台完好可见；
- **用原有 API Key 按第 5 节方式真发一条请求，转发仍然成功，且新记录进入用量页**
  —— 这是整个演练里最关键的一条，批次 4 动了认证与计费主干；
- 按 4.7 观察一次 `quota_used`：存量 Key 里原本 `quota=0`（不限额）的，
  升级后 `quota_used` 会开始从 0 增长，**这是预期的行为变化，不是 bug**。

---

### 7. 回滚路径

> 🔴 **本轮回滚语义已改变，与批次 1/2 不同，务必先读这一段。**
>
> 批次 1–4 的迁移只动 `settings` 表的行，所以"换回旧镜像重启"一直是完整的回滚。
> **批次 5 的 235 / 236 DROP 了 21 张表和 3 个列**，旧代码会去查这些已经不存在的对象。
> 因此跑过 235/236 之后：
>
> | 回滚到 | 是否只换 tag 就够 | 说明 |
> |---|---|---|
> | 本轮的更早提交（`internal-<旧 short_sha>`，仍在批次 5 之后） | ✅ 够 | 库结构一致 |
> | **批次 5 之前的任何版本**（含 `0.1.182`、`:latest`） | ❌ **不够** | **必须连数据库一起从升级前的 `pg_dump` 恢复** |

**情况 A：回滚到本轮内的更早提交**

```bash
# 改 .env 里的 SUB2API_IMAGE，然后：
docker compose up -d sub2api
```

**情况 B：回滚到批次 5 之前的版本（含生产 `:latest`）**

```bash
# 1) 停应用
docker compose stop sub2api
# 2) 从升级前的备份恢复数据库（这一步不能省，否则旧代码会因为表不存在而起不来）
docker compose exec -T postgres psql -U sub2api -d sub2api < sub2api-backup.sql
# 3) 把 SUB2API_IMAGE 换回旧 tag 后启动
docker compose up -d sub2api
```

其余需要知道的：

- **批次 3 与批次 4 单独回滚仍然安全**（230–234 都不删表不删列）。
  所以出问题时的定位顺序是：先怀疑 235/236，再往前推。
- **迁移 229 与 231 是刻意留的回滚兜底**：229 把 `registration_enabled` 置 `false`、
  231 把 `channel_monitor_mode` 置 `'v2'`。这两个值是写给**旧代码**看的 ——
  回滚后旧代码读到它们，才不会退化成"完全开放注册"和"重新打开会烧上游额度的主动探测"。
  这也是批次 5 的迁移 237 清孤儿键时**特意不删这两行**的原因。
- **⚠️ 注册的回滚语义有个坑**（批次 2 验收发现）：`0.1.182` 里
  `/api/v1/auth/register` **路由依然存在**（返回 403），真正关死它的是 229 写进 settings 的开关；
  **只要有人把开关改回 `true`，回滚版本的注册就立刻恢复**。RC 版本则无论开关如何都是 404。
- **⚠️ SMTP 口令不可恢复**：迁移 230 会删掉 `smtp_password` 等 7 个 `smtp_*` 键。
  回滚到旧镜像后旧代码会按默认值重新初始化，功能能用，但 **SMTP 要重新填一遍**。
  如果想留一份备查，得在升级前自己导出。

---

### 8. 验证通过后的动作

把验证结论（通过项、失败项及现象）告知协调方即可。合并 main 与 auto-release
（正式版本号、`:latest` / `:main` 镜像）由协调方执行，无需站长操作。
`internal-rc` / `internal-<short_sha>` 只是验证期临时标签，合并发版后内网部署应切回正式 tag。

---

## 六、验证问题追踪（按镜像版本累加）

> 目标已于 2026-09-08 调整为「内部部署、自用多用户」，以下历史记录中的「单管理员」为当时表述，原文保留不改。

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

### v0.1.187 / v0.1.188 发版与上线记录（2026-09-10）**通过**

一次会话内连做两件事：先把积压的 `batch6/round4` ff 合并 `fork/main` 发出 **v0.1.187**，
再合并外部贡献者 PR #10（上游 v0.2.3 核心修复回移）发出 **v0.1.188**，最后 way-rc → 生产依次升级到 0.1.188。
以下数字全部为 CI 记录与实测，没列出的验证就是没做。

#### v0.1.187（2026-09-10 01:39Z 发版）— 批次 6 第二～四轮正式合并 main

- [x] `batch6/round4`（HEAD `272ab37b8`）对 `fork/main` 是纯 ff，无冲突；推送后 CI run 34425165262 一次全绿（shell / golangci-lint / frontend / test），Security Scan 34425165163 ✓
- [x] Auto Release 34425678381 ✓ → Release 34425691879 ✓（多架构镜像 + GitHub Release + VERSION 同步提交 `938eecfdc`）
- [x] ⚠️ 站长登录验收清单（下方 09-07 小节末尾那 9 项）**当时仍未勾**，本次是按站长口头指示直接合并的；代码本身已在 way-rc `internal-ae47e55` 连跑两天零错误

#### v0.1.188（2026-09-10 02:12Z 发版）— 合入 PR #10 上游 v0.2.3 核心修复回移

**PR #10**（`xiaoguai1818`，`maintenance/upstream-review-v0.2.3`，72 commits / 390 文件 / +20721 −1331）合并前做了两路专项审查：

- [x] **供应链审查通过**：workflows / `audit-exceptions.yml` / 4 个 compose / `.env.example` / `package.json` + `pnpm-lock.yaml` / Makefile / `backend/cmd` 全查；
      无外发地址、无依赖替换或 typosquat、无 CI 提权、无凭据外泄路径。整个 PR 的依赖变更只有 **DOMPurify 3.3.1 → 3.4.14**（安全升级，
      lockfile 的 sha512 已与 npm registry 实测逐字一致，无 tarball 覆写）。新增的唯一扫描例外是 `nanoid` / `GHSA-2v37-7h3g-55p8`
      （已核实为真实公告，high，修复版 3.3.18）——**本仓库 nanoid 已经是 3.3.18，该例外实际不生效**，且带 `expires_on: 2026-10-13` 自动失效
- [x] **裁剪面审查通过**：已裁域（支付 / 订阅 / 余额 / 兑换码 / 返佣 / 第三方登录 / 注册 / 邮件 / 内容审计 / 公告 / 模型广场 / 在线更新）零命中；
      无被删文件被重新加回；前端路由零改动；`backend/ent/` 零改动；PR 内先加后撤的 9 个上游特性，逐一核对 revert 均为逐行逆操作
- [x] **迁移零新增**：PR 中途加过 `239_group_reasoning_effort_over_limit.sql`，已被自己的 revert 删除，合并后迁移头仍为 `240_drop_custom_endpoints_setting.sql`，与本仓库 239 无编号冲突

**合并时改了三处（合并提交 `8314ac7a5`）**：

- 冲突 1 `frontend/src/i18n/locales/zh/admin/overview.ts`：保留本仓库的删除，**不恢复 `claudeMaxSimulation`**
  （批次 6 第二轮已整块裁掉；全仓库零引用、en 语言包同样没有，补回反而会让语言包对不齐）
- 冲突 2 `backend/cmd/server/VERSION`：保留 `0.1.187`，**拒绝 PR 改成 `0.2.3`**
- 主动撤回 PR 对 `.github/workflows/auto-release.yml` 的版本基线改写（改回本仓库原「取最大正式 tag」逻辑）

> **为什么不接受版本对齐 0.2.3**：`v0.2.3` 在本仓库没有对应 tag（`git ls-remote --tags fork` 中 0.2.x 计数为 0）。
> 若采用 PR 的 `max(tag, VERSION)` 基线，下一次发版会跳到 **v0.2.4**，且因 `sort -V` 此后永久锁死在 0.2.x、无法回到 0.1.x；
> 同时 `PREV_TAG=0.2.3` 指向一个不存在的 tag，会让变更日志区间失效、回落成「最近 20 条提交」；
> 将来再同步到一个上游已打过的版本号还会以「tag 已存在」直接发版失败。
> PR 自述也承认「版本对齐只表示完成了审查，并不声称功能等价」——精简版不该穿上游的版本号。

- [x] 集成分支 `integrate/pr10-upstream-v0.2.3` CI run 34426659417 一次全绿，Security Scan 34426659425 ✓
- [x] ff 合并 main 后 CI run 34427282865 ✓、Security Scan 34427282793 ✓；Auto Release 34427841257 ✓ 正确算出 **v0.1.188**（非 v0.2.4，验证：仓库 0.2.x tag 数仍为 0）；Release 34427858395 ✓
- [x] GitHub 上 PR #10 已自动转为 MERGED

#### way-rc 部署核对（2026-09-10 02:15Z，`internal-ae47e55` → `0.1.188`）

- [x] 13 秒 healthy、日志零 error / panic；镜像标签 `org.opencontainers.image.revision` = `8314ac7a5…`，与 CI 验证过的合并提交一致
- [x] 迁移零变化（头仍 240、记录 280、表 77）——round4 的代码 way-rc 早已在跑，本次只换成正式 tag
- [x] 路由核对：已裁域 `/api/v1/payment/orders`、`/api/v1/user/balance`、`/api/v1/auth/register`、`/api/v1/redeem` 全部 404；
      新增 `/api/v1/admin/accounts/upstream-billing-rates` 为 401（在鉴权组内，符合预期）
- [x] 打包产物 grep（从线上抓下 88 个 JS 分片，共 4.0 MB）：
      **应无**（8/8 通过）`claudeMaxSimulation` /「Claude Max 用量模拟」/ `walletBalance` / `subscription_type` / `custom_endpoints` / `customEndpoints` /「自定义端点」/ `insufficientBalance` 全部零命中；
      **应有**（8/8 通过）`recent-requests`、`AccountLoadDrawer`、「查看用量」、「密钥总表」、「自然周」、「密码已复制」、「并发数不能为负数」、`upstream-billing-rates` 全部命中
- [x] 无头浏览器：`/`（用量查询页）与 `/login` 均正常渲染、**控制台零报错**

#### 生产升级（2026-09-10 02:28Z，`0.1.186` → `0.1.188`，跨过 0.1.187）

- [x] 升级前基线：迁移头 `238`、记录 278、表 77、库 133 MB、8 账号 / 10 密钥 / 3 用户
- [x] **迁移 239 / 240 生产空操作已独立核实**（不是照抄旧结论）：
      按 239 的 WHERE 条件实测命中 **0 行**，且生产**没有任何账号带 `quota_weekly_limit` 键**（因此那个 `::numeric` 强转也不可能报错）；
      240 目标键 `custom_endpoints` 存在且值为 `[]`
- [x] 备份 `backups/sub2api-pre-0.1.188-20260910-022744.dump`（5.5 MB）；**导回临时库 `verify_restore` 验证通过**——
      77 表 / 278 迁移 / 8 账号 / 10 密钥 / 3 用户，与线上逐项一致；验证后临时库已删除
- [x] compose 已备份为 `docker-compose.yml.bak-pre-0.1.188`，只重建主体容器（postgres / redis 全程不停）
- [x] 升级后：**39 秒内 healthy**；迁移头 238 → **240**、记录 278 → **280**（+2 即 239 / 240）、表数仍 77；
      业务数据零变化（8 账号 / 10 密钥 / 3 用户）；`custom_endpoints` 已按 240 删除（0 行）
- [x] 上线后 8 分钟：日志零 error / panic，28 次 200，无任何 5xx；服务器 load 0.71
- [x] `way.flyooo.uk` 与自定义登录路径页面均正常渲染、控制台零报错
      （生产 `web_login_entry_public=false` + 自定义登录路径 + 默认首页 `/key-usage` 均为既有配置，本次升级未改动，`/login` 跳用量页是预期行为）
- [x] 生产 `data/config.yaml` 只有 server / database / redis / jwt / default / rate_limit / timezone，**没有 `gateway:` 段**，
      因此 PR 新往 compose 里接的 22 个 `GATEWAY_*` 环境变量不会覆盖线上既有网关调参（这些变量本就都在 `.env.example` 里，只是此前被容器边界吞掉）
- [x] way-rc 也已在正式 tag `0.1.188` 上（本轮不需要另外「切回正式 tag」）

**本轮遗留 / 待站长关注**（都不是阻塞项）：

- [ ] 09-07 那份「站长登录验收清单」9 项至今未逐项手点，现在可以直接在生产或 way-rc 的 0.1.188 上补做
- [ ] **Antigravity 模型映射有行为变更**：`DefaultAntigravityModelMapping` 里 `claude-sonnet-4-5-thinking` 与
      `claude-sonnet-4-5-20250929` 现在都指向 `claude-sonnet-4-6`（原先停在 4-5）。只影响 Antigravity 平台，
      且与既有的 opus / haiku「迁移旧模型」写法一致，但客户端若钉着这两个 ID 会拿到不同（更贵）的模型
- [ ] **Grok 媒体资格放宽**：`billing_inconclusive` 的 OAuth 账号现在仍保留图片 / 视频生成资格（上游认为「账单形态未知」不等于「没有权益」）
- [ ] 生产「系统设置 → 通知」的 Bark 配置仍未填（配置分环境存储，way-rc 上填的不会带过来）
- [ ] `nanoid` 那条扫描例外形同虚设（本仓库已是修复版 3.3.18），2026-10-13 到期后会让 Security Scan 失败，届时直接删掉该条即可

### 批次 6 第二～四轮 RC 验证记录（2026-09-07，way-rc）

三轮依次部署到 way-rc（同一天 06:42Z → 08:27Z → 09:33Z），每轮都做了不登录可做的核对；**站长登录验收尚未做**（清单见本小节末尾）。
生产 `way.flyooo.uk` 全程未动，仍是 `0.1.186`。以下数字全部为 CI 记录与 way-rc 实测，没列出的验证就是没做。范围说明见第三节「批次 6 第二～四轮范围说明」。

#### 第二轮 `internal-e394a90`（`batch6/round2`，HEAD `e394a90a1`，基于 `306bcf291`）

- [x] 三条分支各自 CI 全绿后合并，无冲突：WP-C2 后端 `55734db0e`（CI 34086811737）、WP-C1 前端 `20e6d5193` / `7e63d296c` / `1ad0bf6f6`（CI 34087617890）、
      WP-A3 `ce429bd1e` / `208aa0d96` / `dd0844ae1` / `d6525764e`（第一轮一条 spec 红——清空唯一限额时卡片自动关闭——修后全绿，CI 34088345142）
- [x] 集成 CI run 34090219404 ✓、Security Scan 34090219309 ✓；验证镜像 `internal-e394a90`（Branch Docker Image run 34091279607）
- [x] 迁移 239 前置核实：生产与 way-rc **均无设过周额度的账号**，迁移在两边都是空操作（psql 核实）
- [x] way-rc 部署（2026-09-07 06:42Z）：5 秒 healthy、零错误
- [x] 迁移 239 前后对照：迁移头 238 → 239、记录 278 → 279、表 77；25 个账号的 `extra` md5 逐条相同（未命中、不变）
- [x] 未登录验证：`/user/totp/verification-method` → 404（死端点已摘），`/user/totp/status` → 401（保留面对照）
- [x] 产物 grep：87 个 JS 文件里 `subscription_type` / `clickToViewBalance` / `getVerificationMethod` / `claudeMaxSimulation` 零命中；语言包含「自然周」

#### 第三轮 `internal-b68971a`（`batch6/round3`，HEAD `b68971a24`，基于 round2）

- [x] 三条分支各自 CI 全绿：A6-甲 + 丙 后端 `ab12794cc` / `be97dc3af` / `88a34fb2f` / `0ba5641a0`（第一轮 golangci-lint gofmt 对齐红，修后全绿，CI 34097154880）、
      A6-乙 + 甲 + 丙 前端 `da4be4549` / `c076942f2` / `f255ac71f`（CI 34096248631）、`custom_endpoints` 删除 `bc4b4dc86` / `8e209c54b` / `3bc38c90e`（CI 34095380932）
- [x] 合并只在 Makefile 关键 vitest 名单一处冲突（三条 spec 全保留）；集成 CI 34098887841 ✓、Security Scan 34098887797 ✓；镜像 `internal-b68971a`（run 34099817202）
- [x] way-rc 部署（2026-09-07 08:27Z）：11 秒 healthy、零错误
- [x] 迁移 240 前后对照：迁移头 239 → 240、记录 279 → 280、表 77；`settings.custom_endpoints` 迁移前值 `[]`、迁移后不存在
- [x] 未登录验证：三条 `admin/api-keys` 路由 401（对照不存在路由 404）；`/admin/api-keys` 前端路由 200
- [x] 产物 grep：74 个 JS 分片里 `custom_endpoints` / `customEndpoints` /「自定义端点」零命中；语言包「密钥总表」×2
- [x] 站长另两个决策同日拍板：`security.url_allowlist.enabled` 保持关闭、`server.trusted_proxies` 暂不配置（见第四节「三个决策结论」）

#### 第四轮 `internal-ae47e55`（`batch6/round4`，HEAD `ae47e5564`，基于 round3）

- [x] 两条分支各自 CI 全绿：后端 `f3bcbd9a1` / `396ca2137` / `03b5d7906`（CI 34102510497 一次全绿）、前端 `3403abe39` / `ec05c82c3`（CI 34101114427 / 34101935036）
- [x] 合并无冲突；集成 CI 34103833981 ✓、Security Scan 34103834034 ✓；镜像 `internal-ae47e55`（run 34104661827）
- [x] way-rc 部署（2026-09-07 09:33Z）：6 秒 healthy、零错误、心跳正常；**无迁移**（头仍 240、记录 280、表 77）
- [x] 未登录验证：`recent-requests` → 401（对照 404）
- [x] 产物 grep：73 个 JS 分片含 `recent-requests`、`AccountLoadDrawer`、`SideDrawer`、`accountClick`、`onAccountClick`、`query.account_id`、`setAccountKeyword`；
      语言包 `viewUsageLogs:"查看用量"`、`summary:"并发 {current}/{max} · 最近 {minutes} 分钟 {count} 次 · 最活跃密钥 {key}"`

#### 站长登录验收清单（way-rc `internal-ae47e55`，未做）

- [ ] 周限额默认值：新建 / 编辑账号弹窗周限额一填数字，重置方式自动为 固定 / 周一 / 00:00 / 服务器时区（生产与 way-rc 都没有升级前设过周额度的账号，只能验新建路径）
- [ ] 账号「容量」格：悬停出简版「并发 x/y · 最近 15 分钟 N 次 · 最活跃密钥」，点击打开右侧负载抽屉（并发 / 等待、5 / 15 / 60 分钟切换、按密钥 / 按模型、请求表、同期错误段、刷新）
- [ ] 账号操作菜单「查看用量」→ 进入 `/admin/usage?account_id=` 且账号名回填
- [ ] 使用记录账号可点：点账号名只剩该账号的记录、筛选栏回显账号名
- [ ] 密钥总表 `/admin/api-keys` 全套：列表掩码、排序、筛选（用户 / 分组 / 状态 / 搜索）、查用量跳转、启停、编辑分组与 IP 名单、删除确认
- [ ] 用户管理 → 「API 密钥」弹窗：启停 / 删除 / IP 名单
- [ ] 管理员重定向：管理员访问 `/dashboard` → `/admin/dashboard`、`/usage` → `/admin/usage`；「我的账户」侧栏无「用量」；管理员自己的 `/keys` 照常
- [ ] 自定义端点消失：设置页无该卡片；Keys 页端点弹层只剩当前站点内置端点（复制 / 测速仍在）
- [ ] 回归页面：登录、用户管理、账号管理、分组、使用记录、设置各页无白屏、控制台无报错
- [ ] 验收通过后：`batch6/round4` ff 合并 `fork/main` → auto-release（预期 v0.1.187）→ 生产升级（备份 + 导回验证照旧）→ way-rc 切回正式 tag

### 批次 6 发版记录：v0.1.184 / v0.1.185 / v0.1.186（2026-09-04 ～ 09-07）**通过**

批次 6 至今三次发版、三次生产升级（其中 0.1.185 未单独切换生产、随 0.1.186 生效），全部成功。以下数字全部为 CI 记录与实测；
没列出的验证就是没做。

**共同流程**：分支 → CI（backend-ci "CI" + Security Scan）全绿 → 推 `internal-rc-<sha>` 标签出验证镜像 → way-rc 部署 + 不登录可做的核对
（健康、路由 401/404 对照、公开设置不泄露、打包产物 grep、无头浏览器加载零报错）→ 站长登录 way-rc 手点 → ff 合并 `fork/main` → auto-release →
生产升级（基线 → `pg_dump` 备份 → 临时库导回核对 → 钉 tag → 重建 → 核对）→ way-rc 切回正式 tag。
0.1.184 → 0.1.186 之间**没有任何数据库迁移**：迁移头始终 `238_drop_registration_email_suffix_whitelist_setting.sql`（278 行），表数 77。
生产 compose 自 09-04 起钉固定版本 tag，不再跟随 `:latest`。

> ⚠️ 2026-09-05 验证 v0.1.185 期间发生服务器事故（并行重任务压垮 `oci-free`，原因是协调者的调度失误），详见第四节「⛔ 2026-09-05 事故」。
> 自此编译 / 测试 / lint / 无头浏览器走查一律走 CI，服务器只做部署与轻量核对；v0.1.186 的验证全部按新纪律执行，视觉走查取消。

#### v0.1.184（2026-09-04 09:16Z 发版）— A6-1 密钥列表「查用量」入口 + 用量页订阅 / 余额残留清理 + TRIM_PLAN 补记 v0.1.183 上线

- [x] 集成分支 `batch6/a6-1`（HEAD `a6a917078`）：`75e19a4d0` 查用量入口、`df94f1f70` 用量页残留清理、`f2333e0bb` 弹窗先开再导航 / 停用置灰 / spec 纳入 CI、`6e80d32c5` 文档
- [x] CI run 33832239742 一次全绿；验证镜像 `internal-a6a9170`（Branch Docker Image run 33832797821）
- [x] way-rc 验收（2026-09-04）：8 秒 healthy、日志零错误、迁移 / 表数不变；令牌流程 200 / 401 正确；打包产物 73 个 JS 文件 grep：新文案在、`walletBalance` 零命中；
      无头浏览器打开 `/key-usage?t=` 零控制台错误；**登录后点击由站长手测通过**
- [x] 生产升级（2026-09-04 09:20Z，0.1.183 → 0.1.184）：备份 `backups/sub2api-pre-0.1.184-20260904-091855.dump`（导回验证通过）；12 秒 healthy
- [x] 升级后核对：KeysView chunk 含 `usageUnavailableInactive`

#### v0.1.185（2026-09-05 02:22Z 发版）— A6-2 第一步：Bark 推送通道 + 管理端配置 / 测试接口 + 接入运维告警引擎 + 设置页「通知」页签

- [x] 集成分支 `batch6/a6-2`（HEAD `34be1e12b`）：后端 `0ae423571`，前端 `9aed20219`，对接修正 `34be1e12b`
- [x] CI run 33862992329 一次全绿；验证镜像 `internal-34be1e1`（run 33863752189）
- [x] 端到端实证（2026-09-05 02:01Z，在远程容器里用真实代码路径 + 正式同款 AES 加密器，对站长的 Bark 服务器 `https://notify.flyooo.uk`——
      标准 bark-server，JS 版 v2.2.6，7 台设备，无需额外鉴权头）：PUT 落库密文不含明文 key（272 字节）→ GET → 测试推送 ok / ping_ok 200、698 ms →
      无 key 只探活 40 ms → 告警触发推送 → 恢复推送，站长手机三条全部收到（截图核实）
- [x] 生产 / way-rc 均已确认 `TOTP_ENCRYPTION_KEY` 已配置（64 hex），保存设备 Key 不会被 `SECRET_ENCRYPTION_KEY_NOT_CONFIGURED` 拦
- [x] way-rc 验收（2026-09-04 10:41Z）：8 秒 healthy、零错误、评估器心跳 `rules=9 enabled=8`；三条接口未登录 401（对照旧 `/admin/ops/email-notification/config` 404）；
      公开设置 grep bark=0；打包产物含全部文案；登录页无头浏览器零报错；**站长在 way-rc 登录后实测填写 / 测试 / 保存通过**
- [x] 生产升级：**未单独切换**——当天服务器事故（见第四节），升级步骤在事故前只做到备份；代码随 0.1.186 于 2026-09-07 一起上线生产
- [ ] 未做：告警真实触发后的推送在生产上尚未观察到（无告警发生）
- [ ] 站长待办：生产「系统设置 → 通知」的 Bark 配置待站长填写（分环境存储，way-rc 的不带过来）

#### v0.1.186（2026-09-07 04:2xZ 发版）— A6-2 第二步：账号用量阈值提醒规则 + 密钥列表今日用量排序与「最后使用时间」列 + 一键三档弹窗修复

- [x] 集成分支 `batch6/a6-2s2`（HEAD `e9936c5b5`）：后端 `8a39c0ba3` / `b3ec34d4e` / `dc31704a5`，前端 `9552c97b6`。
      分支 CI：后端第一轮 golangci-lint 红（errcheck 未检查类型断言）修后全绿（run 33942246430）；前端一次全绿（run 33941774097）；
      集成 CI run 33942864698 全绿；验证镜像 `internal-e9936c5`（run 33943258818）
- [x] 密钥页排序分支 `batch6/keys-sort-lastused`（`022f98c97`，基于 `e9936c5b5`）：CI run 34077780372 一次全绿
- [x] 三档弹窗修复 `81836e9c8`：站长在 way-rc 实测三档时发现并要求修改；**这一笔按站长决定跳过 RC 验证直接发版**（如实记录）
- [x] way-rc 验收（合并镜像 `internal-022f98c`，2026-09-07 03:11Z）：6 秒 healthy、零错误、心跳正常；评估接口未登录 401；
      语言包含「账号用量提醒规则」×6、「一键三档」×3、「立即试发」×2；设置页 chunk 含 `AccountUsageAlertRulesCard`；
      **站长登录 way-rc 实测通过（Bark 配置、规则新建 / 试发 / 三档、密钥页排序）**
- [x] 生产升级（2026-09-07 04:26Z，0.1.184 → 0.1.186，跨过 0.1.185）：备份 `backups/sub2api-pre-0.1.186-20260907-042535.dump`（4.9 MB），
      导回验证：77 表、迁移头一致、六计数 3 | 10 | 8 | 8 | 6287 | 131 一致；compose 备份 `docker-compose.yml.bak-pre-0.1.186-20260907-042601`；
      15 秒 healthy；容器 revision `81836e9c82161dc62a7030b1cabfe39a5eed2287`
- [x] way-rc 于 2026-09-07 04:27Z 切回正式 tag `0.1.186`，6 秒 healthy；生产与 way-rc 现同为 `0.1.186`
- [x] `fork/main` = `6fd50990f`（VERSION 同步提交，父 `81836e9c8`），本地 `main` 已同步
- [ ] 未做：告警真实触发后的生产推送尚未观察到
- [ ] 未做：生产 Bark 配置待站长填写

---

### v0.1.183 正式上线（2026-09-04，生产 way.flyooo.uk）**通过**

裁剪工程第一次把批次 2–5 的产物推到生产。与前几轮不同：验的是**正式发版镜像**而非 RC 镜像，
环境是**生产本身**而非 way-rc 或隔离栈。以下数字全部为实测。

**发版与镜像**：

- [x] 批次 2–5 合并提交 `cbaf63a69`（"Merge: 内部部署裁剪批次 2-5 合入 main"）于 2026-09-01 06:18Z 合入 `fork/main`，
      CI 与 Security Scan 全绿
- [x] auto-release 于 06:27Z 自动发版 **v0.1.183**（`VERSION` 同步提交 `7341cce56`）
- [x] GHCR 镜像 `ghcr.io/cherrylover/sub2api:0.1.183`（revision `cbaf63a690c22dca64e2815f8804ddce3d7f8311`，双架构）；
      2026-09-04 核实仓库里 `:latest` 的 digest 已与 `0.1.183` 相同
- [x] 发版镜像内 `/app/adminpass` 存在（36,372,642 字节）——原第四节「⛔ 未完成项 2」已解决，证据见该节

**⚠️ 第五轮验收之后、发版之前又合入的 7 个提交（未单独验收，如实记录）**：

internal-498f780 验收（2026-08-29）之后，分支上又合入了 4 个 P0 收尾提交
（`ac76ef834` 裁掉 Key 额度上限界面、`961d4cd99` 删注册设置卡片与邮箱域名白名单 + 迁移 238、
`5776f0327` 删解绑第三方登录接口、`c0d7b372d` 删管理员手工绑定第三方身份接口）
与 3 个测试/文档修复提交（`0c55fdf5f`、`731f2e883`、`04a3f68a9`）。
它们对应的验证镜像 `internal-rc-04a3f68` 于 2026-08-31 部署到 way-rc
（way-rc 库里迁移 238 的 `applied_at` = 2026-08-31 17:21 +08）；way-rc 自 2026-09-01 06:40Z 起改跑正式 tag `0.1.183`
（compose 备份 `docker-compose.yml.bak-pre-0.1.183-20260901-064037`），运行至 09-04 无真实错误
（日志里只有假账号定时测试的 401 噪音）。
**本节没有为这 4 个收尾提交或 0.1.183 正式版单独立过验收记录**——它们只在 way-rc 上运行观察过，
没有按第五节清单逐项跑过；下方生产上线后的核对是它们唯一一次系统性验证。

**生产切换（北京时间 2026-09-04 10:17，停机不到半分钟）**：

升级前状态：镜像 `ghcr.io/cherrylover/sub2api:latest`（本机缓存实际 = 0.1.182，revision `e11441d1c`），
库迁移头 `229_force_registration_disabled.sql`，98 张表。

- [x] **备份**：`pg_dump -Fc` → `/opt/stack/sub2api/backups/sub2api-pre-0.1.183-20260904-021710.dump`（4,089,501 字节）
- [x] **备份导回验证**（不是"文件存在"，是真导回去）：在同一 postgres 容器里建临时库 `restore_test` 用 `pg_restore --no-owner` 导入，
      exit=0、零告警；导回后 98 张表，users=3 / api_keys=9 / accounts=8 / groups=8 / usage_logs=4853 / settings=275，
      与生产逐项一致；验完删除临时库
- [x] **钉版本**：`docker-compose.yml` 的镜像从 `:latest` 改为 `:0.1.183`（原文件备份 `docker-compose.yml.bak-pre-0.1.183-20260904-021710`）。
      生产从此不再跟随 `:latest`，第一节「⚠️ 生产影响」的风险模型解除，回滚只需改回固定 tag——
      **但批次 5 之后回滚仍须连库恢复，该结论不变**
- [x] **重建容器**：`docker compose up -d --no-deps --force-recreate sub2api`，约 9 秒后 healthy；容器 image=0.1.183、revision=cbaf63a69
- [x] **迁移**：230–238 共 9 条自动落表（比第五轮多出的 238 来自 P0 收尾 `961d4cd99`），`applied_at` 10:17:45.80 → 10:17:46.08（约 0.28 秒）
- [x] **结构核对**：表 98 → 77；迁移 235 声明的 21 张死表一张不剩（计数 0）；渠道监控 V2 的 10 张表健在；
      `groups` 上批量生图 3 列已删（计数 0）；`settings` 275 → 131 行（迁移 230/232/233/237/238 清孤儿键）
- [x] **数据核对**（与升级前基线逐项相同）：users=3 / api_keys=9 / accounts=8 / groups=8 / usage_logs=4853；
      `usage_logs` 全量 id 串 md5 `e56e432e23edbb5c07f6a4e386a180ec` 前后一致；
      `users` 整行 json md5 `710a3eaa015e895e4f70fdef2a736906` 前后一致；
      `api_keys` id md5 `4465827426a5fff877ca7c2b8cc1aab5` 前后一致
- [x] **关键设置**：`allow_ungrouped_key_scheduling=false`（新的种子探测键）、`channel_monitor_mode=v2`、`registration_enabled=false`
- [x] **启动日志**：零 error/panic；WARN 共 4 条且全是升级前就有的老问题
      （`security.url_allowlist.enabled=false` ×2、`server.trusted_proxies` 未配置、CORS allowed_origins 未配置）

**上线后验证**：

- [x] `/health` → `{"status":"ok"}`；`/`、`/login`、`/admin/accounts` 均 200；`/api/v1/settings/public` 报 `version: 0.1.183`，
      且不含注册/支付/公告/模型广场/人机验证/SMTP/第三方登录等已删键
- [x] **已删路由全部 404**（11 条）：`POST /api/v1/auth/register`、`GET /api/v1/admin/payment/providers`、`/api/v1/admin/subscriptions`、
      `/api/v1/admin/announcements`、`/api/v1/admin/promo-codes`、`/api/v1/admin/redeem-codes`、`/api/v1/admin/channel-monitors`、
      `/api/v1/admin/batch-images`、`/api/v1/user/subscriptions`、`/api/v1/admin/users/1/balance-history`、`/api/v1/model-plaza/models`
- [x] **保留路由全部存在**（未登录返回 401，登录接口空包返回 400）：`POST /api/v1/auth/login`、`GET /api/v1/admin/accounts`、
      `/api/v1/admin/groups`、`/api/v1/admin/settings`、`/api/v1/admin/dashboard/stats`、`/api/v1/admin/usage`、
      `/api/v1/admin/ops/dashboard/overview`、`/api/v1/keys`、`/api/v1/admin/backups`、`/api/v1/admin/users/1/api-keys`、
      `/api/v1/admin/data-management/backups`、`POST /v1/chat/completions`、`/v1/messages`、`/v1/responses`
- [x] **端到端真实转发与计费实证**（第五轮在 way-rc 上一直没能验的那一跳，本次在生产用真实上游账号补上）：
      用站长自己的 Key（id=3 "jerry"，分组 2「Grok 专用」，quota=0）：`GET /v1/models` 返回模型列表；
      `POST /v1/chat/completions`（model `grok-3-mini`，max_tokens=8）拿回上游正常应答（content "OK"，上游实际模型 grok-4.3，账号 id=3）。
      随后 `usage_logs` 新增 id=4854（api_key_id=3、account_id=3、7 in / 111 out、cost 0.000072），
      `api_keys.quota_used` 由 0 变为 0.00007200，而 `quota=0` 的 Key 仍可正常请求——
      批次 4「quota=0 表示不限额、quota_used 无条件累加」的语义在生产得到实证
- [x] way-rc 未受影响（`/health` ok）

**生产数据前置核对（升级前做的）**：

- [x] `api_keys` 中 `quota>0` 的 Key = 0 把（共 9 把）；`user_subscriptions` = 0 行；`user_allowed_groups` = 0 行；
      两个专属分组（id 4 demo3、id 5 Antigravity）下面 0 把 Key，故迁移 234 的专属分组语义变化在生产零暴露
- [x] 用户 3 个（1 个 admin）

**未做的验证（如实记录，未勾）**：

- [ ] 没有用管理员账号登录后台做页面级点击（按规则不使用存储的密码登录）——
      第五节 4.4 的"浏览器侧逐项确认"与后台页面回归本次在生产上未跑
- [ ] 删除用户、开关 2FA、个人资料页四张卡片等「高风险面必测」条目本次未测

---

### 第五轮：批次 3 + 4 + 5 合并验收（2026-08-29 建档，**已完成**——结果见下方 internal-498f780 小节）

**状态（2026-09-04 更新）：已完成。** RC 镜像 `internal-498f780` 于 2026-08-29 验收通过（记录在下方同名小节），
随后合并 `fork/main`、发版 v0.1.183，并于 2026-09-04 上线生产（见上方「v0.1.183 正式上线」小节）。
本小节保留建档时的关注点清单供追溯。

**开跑前必须先确认的三件事**：

- [x] **CI 四个 job 在 GitHub 上全绿**。本地已跑通等价检查（见第四节「本轮"本地已验证"的确切含义」），
      但本地跑通 ≠ CI 全绿：CI 的 runner 有 Docker，会真跑 testcontainers 集成套件。
      → internal-498f780 的 CI 四个 job 与 Security Scan 一次全绿（2026-08-29）；
      合并提交 `cbaf63a69` 的 CI 与 Security Scan 亦全绿（2026-09-01）。
- [x] **RC 镜像构建成功**。CI 覆盖不到镜像构建（见第四节同名条目），而本轮删了大量目录 ——
      虽然已 grep 过 `Dockerfile` / `deploy/Dockerfile` / `.dockerignore` / `.goreleaser.yaml`，
      但批次 2 就是在这里连挂两轮。
      → `internal-498f780` 双架构镜像构建成功（2026-08-29）；正式版 `0.1.183` 双架构镜像亦由 auto-release 正常出包（2026-09-01）。
- [x] **升级前的数据库备份可用**（不是"文件存在"，是"导入回去能起得来"）。
      **本轮是第一次做不可逆结构变更**（DROP 21 张表 + 3 个列），回滚必须连库一起回滚。
      → 2026-09-04 生产上线前实测：`pg_dump -Fc` 后在同一 postgres 容器里建临时库 `pg_restore` 导回，exit=0、零告警，
      导回后 98 张表与 users / api_keys / accounts / groups / usage_logs / settings 行数逐项与生产一致（见上方 v0.1.183 小节）。

**本轮相比前几轮，重点变了的地方**：

| 优先级 | 项 | 为什么这轮特别重要 |
|---|---|---|
| 🔴 最高 | 第 5 节**端到端转发**（配真实上游账号） | 批次 4 改了计费与额度核心，而这条链路前两轮都因为假账号没验通。**本轮再空过去，最该验的东西就是空白** |
| 🔴 最高 | 4.7 **批次 4 两项行为变更** | 探测不出来、只能观察；`quota_used` 无条件累加、耗尽判定窗口变宽 |
| 🔴 最高 | 第 6 节**老库升级演练** | 第一次不可逆结构变更，21 张表 + 3 个列 |
| 🟡 高 | 4.8 **`adminpass` 实测** | 删掉自助找回密码后的唯一自救通道；⛔ 注意它**不在发版镜像里**（第四节未完成项 2；→ 已由 `498f7805b` 解决） |
| 🟡 高 | 4.4 **保留面正面核对** | 本轮几组保留面与被删面只差一个前缀，误删代价大于漏删 |
| 🟢 常规 | 4.1–4.3、4.5、4.6、4.9 | 裁剪面探测，按清单跑 |

**已知会带进本轮、但不是本轮引入的两项**（批次 2 验收发现，至今未决策/未处理）：

- [ ] `security.url_allowlist.enabled=false` —— 生产上线前需要明确决策是否开启
      （2026-09-04 上线后仍未决，生产启动日志该 WARN 仍在）
- [ ] `server.trusted_proxies` 未配置 —— 影响真实客户端 IP 的准确性
      （2026-09-04 上线后仍未配置，生产前面是 Cloudflare + Traefik）

---

### internal-498f780（2026-08-29，第五轮：批次 3+4+5 验收）**通过**

镜像 `ghcr.io/cherrylover/sub2api:internal-rc` = `:internal-498f780`（双架构）。
CI 四个 job（golangci-lint / shell / frontend / test）与 Security Scan **一次全绿**；
后端单测（51 包）与集成测试（45 包，真 Docker）在实施阶段本地实跑亦全绿。

**way-rc 升级（已永久变为升级后状态）**：

- [x] 迁移 269 → 277，230~237 八条全部落表，0.24 秒内跑完，日志零 error/panic
- [x] 表 98 → 77，删除清单与迁移 235 声明**精确匹配**（差集比对，不多不少），新增表 0
- [x] 数据完整：users/api_keys/accounts/groups 行数与升级前一致
- [x] **转发链路完好**（本轮最硬证据）：临时放开假账号调度后发请求，拿回 Anthropic
      官方格式的 401 应答体（`API key is invalid.`）——该响应本地无法伪造，
      证明请求真实抵达 `api.anthropic.com`。鉴权 / 分组授权 / 额度校验 / 账号选择 /
      并发槽 / 实际转发全链路通过
- [x] **渠道监控 V2 存活且在工作**：03:14 打的 3 个请求两分钟后精确出现在 V2 分钟级
      指标表的 03:14 桶内，水位线从 03:13 推进到 03:15。V1 五张表已删，
      V2 十张表（含复数的 `channel_monitor_v2_watermarks`）全部健在
- [x] **adminpass 实测通过**：改密 → 新密码可登录 / 旧 token 失效 → 改回原值 → 原密码恢复
- [x] 45 个已删接口双通道全 404；后台 19 条路由零白屏、90+ 接口调用零 4xx/5xx

**生产备份升级演练（隔离临时栈，验完已彻底销毁）**：

- [x] 演练基线取自生产 `pg_dump`（22MB，98 个 COPY 块）；先用 `:0.1.182` 起一次确认
      演练环境忠实复现升级前状态
- [x] **数据零损失**：升级后 users=3 / api_keys=9 / accounts=7 / groups=8 / usage_logs=4031
      与生产基线逐项一致；**`usage_logs` 全量 id 集合的 md5 升级前后完全相同**
      （不只是条数对上，是每条记录都在原位）；`api_keys` 与 `users` 全字段逐行 diff 均 IDENTICAL；
      admin `password_hash` 指纹未变；`users.balance` 列与 `usage_logs.subscription_id` 列均保留
- [x] **迁移 234 的专属分组回填在生产是空操作**——不是没执行，而是两个专属分组
      （demo3、Antigravity）上**一把 Key 都没绑**，回填条件匹配 0 行。
      另注：Antigravity 已于 2026-08-27 软删，真正活着的专属分组只剩 demo3。
      执行 Agent 另做**正向对照实验**（人为造 4 把 Key + 1 条已存在授权），
      确认回填逻辑正确：该补的补、软删的 Key 与软删的分组正确跳过、已有授权不被覆盖；
      并确认该授权行"扛事"——删掉后立即 403 `GROUP_NOT_ALLOWED`
- [x] 迁移 234 另两段（关高峰倍率、分组类型归一）在生产同为空操作，`groups` 表逐行 diff IDENTICAL

**⚠️ 本轮必须记录的三件事**

**一、额度语义的真实变化（此前判断有误，此处更正）**

> 早先曾判断"升级后 9 把 Key 会变成无限额"。**该判断错误。**
> 实测对照（每次清 Redis + 重启应用，排除鉴权缓存干扰）：

| 场景 | 旧版 0.1.182 | 新版 internal-rc |
|---|---|---|
| quota=0，余额 719（生产原样） | 放行 | 放行 |
| quota=0，**余额 0** | **403 INSUFFICIENT_BALANCE** | **放行** |
| quota=0，余额 −100 | 未测 | 放行 |
| quota=1 已用 5（超额） | 429 QUOTA_EXHAUSTED | 429 QUOTA_EXHAUSTED |

> **`quota=0` 表示不限额，在 0.1.182 上就已如此，不是本轮引入。**
> 真正被拿掉的是**用户余额这道兜底闸门**：旧版余额扣到 0 会 403 断流，
> 新版余额完全不参与判断、扣成负数照样放行；`/api/v1/user/profile` 也不再返回 balance。
> 生产 9 把 Key 的 quota 全是 0 → **升级后没有任何机制会自动叫停消费**。
> 非零 quota 在新版仍严格执行（超额 429），是升级后唯一有效的刹车。

**二、回滚已不可行，且健康检查会骗人（实测，非推断）**

只换镜像回滚到 `:0.1.182`（数据库保持已迁移状态）：

- 容器**照常 `healthy`**、`/health` 返回 **200**、`RestartCount=0` —— 监控一片绿
- 但 `/api/v1/keys`、`/api/v1/groups/available`、`/api/v1/admin/users`、
  `/api/v1/subscriptions` 等**全部 500**（根因：`relation "user_subscriptions" does not exist` 7 次、
  `announcements` 1 次）
- **最致命**：`POST /v1/chat/completions` → **500 `Failed to validate API key`**，
  旧版 Key 校验会读已被删的 `user_subscriptions`。**这是全站转发 100% 中断，
  而健康检查不会告诉你。**

正确回滚 = **恢复数据库备份 + 换回旧镜像，两件事必须一起做**（已实测验证：
恢复后表数回到 98、迁移回到 269，所有接口回到 200，网关恢复正常）。
代价是升级后新增的调用记录与用量数据一并回退丢失，**升级窗口越长回滚代价越大**。

**三、`settings` 表会掉 143 条配置**

275 → 132 行。不只迁移 234 删的 8 个键，230/232/233/237 合起来清掉了
`auth_source_*` 42 条、SMTP 6 条、affiliate 6 条、payment 5 条、model_plaza 3 条、
微信/OIDC 接入、各家验证码、订阅相关等。对应功能代码早已删除、配置无人读取，
但**同样不可逆**，升级前建议单独导出 `settings` 表存档。

**升级后的一处行为变化（会实际碰到）**：给专属分组 demo3 新建 Key 时，
**必须先在后台把用户加进该分组的允许名单**，否则新建的 Key 一用就 403。
旧版靠订阅旁路能绕过，新版这条路没了。

**生产上线清单（基于实测）**

1. 升级前 `pg_dump`（`--clean --if-exists --no-owner --no-privileges`）并确认可恢复——唯一退路
2. 升级前单独导出 `settings` 表，以及 3 张有数据的待删表
   （`channel_monitor_request_templates` 5 行、`channel_monitor_aggregation_watermark` 1 行、
   `redeem_codes` 1 行；其余 18 张为空）
3. 给对外的 Key 设非零 `quota` —— 升级后唯一还有效的刹车。尤其
   `xuzhangyao` / `shengyuliang` / `gork` / `grok` 四把 demo 用户的 Key，
   虽然用户当前 disabled，一旦启用就完全不设防
4. 钉住旧镜像 tag，别让它被覆盖或清理
5. **升级后第一件事：直接打一发真实 `/v1/chat/completions` 验证转发**，
   不要只看 `/health` —— 本轮最重要的教训就是健康检查在这个场景下完全不可信
6. 划定回滚决策窗口（建议 30 分钟），过点即视为不可回滚、只能向前修

**补验：真正"扣钱"的执行路径已验证通过（2026-08-31）**

此前因 way-rc 与演练环境的上游都是假账号 / 已禁用账号、请求成本恒为 0，
批次 4 改动最深的计费执行路径一直没被执行过。现已用**另起一套隔离环境 + 生产备份**
补验完成 —— 生产备份里带着 7 个真实可用的 OAuth 上游账号，导入隔离栈后即可发真请求。

环境：`/opt/stack/sub2api-billing-drill`，容器 `s2a-bill-*`，仅绑 `127.0.0.1:18092`，
无 traefik label、独立网络。导入生产 dump 零 ERROR（3 用户 / 9 Key / 7 账号 / 4375 条用量），
升级到 `internal-rc` 后 277 条迁移、77 张表、启动日志 0 错误。验完已彻底销毁。

| 测试 | 结果 |
|---|---|
| **真实调用** `POST /v1/chat/completions`（grok-3-mini，走真实 grok 账号） | **HTTP 200**，1.73s，返回真实应答内容 |
| **成本计算** | `input_cost` 0.0000021 + `output_cost` 0.0000575 + `cache_read_cost` 0.0000144 = `total_cost` **0.0000740**，分项齐全，`billing_mode=token` |
| **用量落库** | `usage_logs` 4375 → 4376，新行含 tokens（输入 7 / 输出 115 / 缓存读 192）、account_id=3、upstream_model |
| **额度累加** | `quota_used` 0 → **0.00007400**，与 `total_cost` **分毫不差** |
| **额度闸门（真实成本）** | 把 quota 调到低于已用量 → **429 `API_KEY_QUOTA_EXHAUSTED`**「API key 额度已用完」 |
| **额度恢复** | 调回 0.10 → 放行且再次扣费，`quota_used` 0.00007400 → 0.00016010 |
| **⚠️ 余额闸门已消失（真实调用实证）** | 把用户余额置 0 后再发真请求 → **仍然 HTTP 200，拿到真实应答**，`quota_used` 0.00016010 → 0.00024170，余额停在 0 不动 |

**结论**：批次 4 的计费改造是**正确的** —— 成本算得对、用量记得全、额度扣得准、
非零额度的闸门用真实成本能拦住。同时**用真实调用坐实了余额兜底确实已被拆除**：
余额为 0 时请求照样成功、照样消耗上游，这不再是从 503 推断出来的结论，而是实测。

因此上线清单第 3 条（给对外的 Key 设非零 quota）从"建议"升级为**必做**：
它是升级后唯一还能拦住消费的机制，且已验证有效。

### internal-1e49744（2026-08-28，第四轮：批次 2 验收——**已通过**）

镜像 `ghcr.io/cherrylover/sub2api:internal-rc` = `:internal-1e49744`，
双架构（amd64 + arm64），来源 commit `1e49744d`（CI 与 Security Scan 均 success）。

**部署事实**：2026-08-28 已部署到 way-rc（`/opt/stack/sub2api-rc`）。
部署前备份在该目录 `backups/`（compose、pg_dump、data 目录，时间戳 `20260828-084526`）。

**⚠️ 本轮开工前发现并已修复的环境问题（重要）**：

- [x] **way-rc 之前跑的根本不是 RC 镜像**：`/opt/stack/sub2api-rc/docker-compose.yml` 的镜像坐标是
  `ghcr.io/cherrylover/sub2api:latest`（= 已发版的 0.1.182 **未裁剪完整版**），
  而非 `:internal-rc`。即 2026-08-27 之后在 way-rc 上做的任何"验证"验的都是错误的镜像。
  已改为 `:internal-rc` 并重建容器（于 internal-1e49744 验证通过）。
- [ ] **仓库侧同源问题未修**：`deploy/docker-compose.yml`、`docker-compose.local.yml`、
  `docker-compose.standalone.yml` 三个编排**仍写死 `:latest`**，这正是上面那个坑的来源。
  建议改为可用环境变量覆盖（如 `${SUB2API_IMAGE:-ghcr.io/cherrylover/sub2api:latest}`），
  否则下一个照仓库编排搭验证环境的人会再踩一次。**归入批次 3。**
  → **批次 3 已交付修复**（`96f704c11`）：三个 compose 改为
  `${SUB2API_IMAGE:-ghcr.io/cherrylover/sub2api:latest}`，`.env.example` 补 `SUB2API_IMAGE` 项，
  `deploy/README.md` 同步改写。**待新 RC 镜像复验后勾选。**
- [x] **数据库状态核实**：way-rc 库停留在 RC 分支状态（最新迁移 `229_force_registration_disabled.sql`，
  共 269 条），main 独有的 `229_plugins` / `230_plugin_artifacts` / `231_*` **均未执行**，
  `plugins` / `plugin_artifacts` 表不存在。故换回 RC 镜像**不构成降级**。
  反过来说，此前跑在该库上的 0.1.182 是"代码要的表不存在"的带病状态。

**已完成的验证项**：

- [x] 镜像版本标识为 `internal-claude-project-trim-internal-deploy-7jny-1e49744`，非 `vX.Y.Z`（第 1 节）
- [x] 前端确已嵌入镜像：从 GHCR 拉取 layer 解包后，二进制内含 `dist/index.html` 与 116 个
  `dist/assets/*` 资源（**注意：CI 的 Embed tag build check 用的是桩目录，只能证明"编译得过"，
  不能证明"前端真打进去了"，故本项须对实际镜像验证**）
- [x] 裁剪面：镜像内**不含**任何 register / captcha / legal 相关前端资源
- [x] 服务健康：容器 healthy，`https://way-rc.flyooo.uk/health` → 200，启动日志无 error
- [x] 4.1 注册接口（批次 2 预期已翻转）：`POST /api/v1/auth/register` → **404**
  （对照生产 way.flyooo.uk 同接口 → 400，证明路由确实只在裁剪分支上消失）
- [x] 后台可正常登录并渲染；侧边栏版本号显示为 `vinternal-claude-project-trim-internal-deploy-7jny-1e49744`
  （批次 1 第三轮修复的版本号回归在本轮镜像上仍然有效）

**验证结论（2026-08-28 完成，多 Agent 实测）**：**通过，可合并 `fork/main`。**

空库全新部署（第 2、3 节）——在服务器上另起隔离临时栈（`s2a-vfy-*`，仅绑 127.0.0.1:18090，
无 traefik label、不入 proxy 网络），验完已彻底销毁：

- [x] 2.1 起栈：RC 镜像拉起三容器全 healthy，确认零 traefik label、独立网络，未劫持 way-rc / 生产路由
- [x] 2.2 健康：`/health` → `{"status":"ok"}`
- [x] 3.1 自动初始化：日志出现 `Admin user created` → `Auto setup completed successfully`，无 error / panic
- [x] 3.2 登录：用临时栈自建凭据真实登录成功（role=admin）
- [x] 3.3 `/setup` 跳转：`GET /setup/status` → `needs_setup:false`，浏览器实测被跳到 `/login`；
      `POST /setup/install` / `test-db` 均 404（装完不可重复触发）
- [x] 4.1 空库注册面：`POST /api/v1/auth/register` → 404
- [x] 迁移-空库：269 条，含 `229_force_registration_disabled.sql`

老库升级演练（第 6 节）——⚠️ **本轮最有价值的部分**。忠实导入生产备份后发现 6.3/6.4/6.5 全是 no-op
（原因见下方"前提更正"），执行 Agent 因此在临时栈上**人工构造了真实的老库场景**
（删掉 229 行、把 `registration_enabled` 改回 `true`、注入三条 RC 不认识的迁移记录并建出对应表），
才拿到了真正的证据：

- [x] 6.1 导入：生产 pg_dump 22.9MB / 92109 行导入零 ERROR；行数与生产基线一致
- [x] 6.2 启动：RC 在存量库上 10s 内 healthy，无 error / panic / 迁移报错
- [x] 6.3 未知迁移：RC 遇到库里自己不认识的迁移行**行为正确**——
      `migrations_runner.go` 只遍历自己 embed 的文件、按 filename 正向查表，从不反向枚举库中的行，
      故多余的行被安全忽略，不报 checksum 错
- [x] 6.4 迁移 229 真的会执行：在人工回退成 pre-229 的库上启动，`schema_migrations` 出现新行，
      `applied_at` 落在本次启动窗口内
- [x] 6.5 注册强制关闭：启动前人为把 `registration_enabled` 改成 `true`，RC 启动后被改回 `false`
- [x] 6.6 存量完好：users=3 / api_keys=9 / accounts=7 / groups=7 / usage_logs=4018，
      四次观测行数完全一致，**4018 条调用记录一条未丢**
- [x] 6.7 管理员未被覆盖：三个用户的 `password_hash` 指纹全程未变，角色/状态未被改写，
      临时栈自己的 ADMIN_EMAIL 未被注入库中（B3 硬约束满足）
- [x] 6.8 裁剪面：`register` → 404，且**即使把开关置 true 仍是 404**，
      证明 404 来自路由被摘除而非开关拦截

- [x] **第 7 节 回滚路径**：换回 `:0.1.182`，旧代码在已跑过 229 的库上正常 healthy，
      无 checksum / 迁移报错，数据行数不变
- [x] **清理**：临时栈 `down -v` + 目录删除 + 临时文件清理，`docker ps -a` 无 `s2a-vfy-*` 残留，
      way-rc 与生产两套环境均 healthy

裁剪面与回归（第 4 节 + B1/B2/B3/B4，均公网 + 容器内网双通道验证，结果逐字节一致）：

- [x] 4.2 已裁剪路由全 404（另扩测 13 条 OAuth / 支付 / 兑换路径同样 404；**带管理员 token 复测仍 404**，
      排除"只是被鉴权挡住"）
- [x] 4.3 公开设置不含 7 个已裁剪功能键
- [x] 4.4 登录页无第三方按钮（接口 / bundle 关键字 / 真实浏览器可访问性树 三层验证）
- [x] B1 合规确认已移除，后台 16 条接口全 200、无 423 拦截
- [x] B2 站点品牌 8 个冗余设置项全部不再下发
- [x] B3 `send-verify-code` → 404，前端 36 个 chunk 零命中
- [x] B4 人机验证已移除：**用不含验证码字段的请求体登录，错密码返回 `401 INVALID_CREDENTIALS`
      而非"缺少验证码"**，证明登录链路上已无验证码环节
- [x] 3.1 补充：`registration_enabled` 键已整个从公开设置中消失（落在清单预设的第二分支）
- [x] 回归-管理员登录：真实登录成功，`/admin/users` 返回 2 个用户、状态与角色未被改写

**未闭环项（2 项，均为环境限制，非代码缺陷）**：

- [ ] **回归-存量普通用户登录**：way-rc 的 `test1@sub2api.local` 密码不在 `.env` 里，无法真实登录。
      数据层已确认具备登录条件（`password_hash` 为标准 bcrypt、长度 60、`status=active`、`role=user` 未改写）。
      要闭环需站长提供密码或授权做一次可回滚的重置。
- [ ] **第 5 节 端到端转发**：已实测请求确实发到 `https://api.anthropic.com` 并被上游返回
      `401 authentication_error`，即**网关侧鉴权 / 分组解析 / 账号选择全部跑通，断点在假凭据**。
      站长已选择接受降级结论（见第三节）。**"拿到上游正常应答"始终未验证。**

**⚠️ 前提更正（本轮由执行 Agent 纠正，重要）**：

任务书曾称"生产库处于 main/0.1.182 状态，含 `229_plugins` / `230_plugin_artifacts` / `231_*`
等 RC 分支没有的迁移"。**实测为假**：生产 `schema_migrations` 共 269 行，最大就是
`229_force_registration_disabled.sql`（2026-08-27 12:56 应用），全库无任何 plugin 相关表，
仓库 `backend/migrations/*.sql` 恰好 269 个文件与生产 1:1 对应。
根因与第四节「分支与 main 无分叉」是同一个：把上游 `Wei-Shaw/sub2api` 的 main 误当成了本工程的主线。

**本轮新发现的问题（按严重程度）**：

- [ ] **高｜`security.url_allowlist.enabled=false`**：SSRF / URL 白名单校验整体关闭，只剩最小格式校验。
      way-rc 与临时栈启动日志均有此 WARN。**不是本批次引入**，但生产上线前需要明确决策是否开启。
- [ ] **高｜验收标准本身写错**：清单第 6 节的「`settings.registration_enabled = false`」对**空库不成立**——
      229 全文只有 `UPDATE ... WHERE key='registration_enabled'`，**没有 INSERT/UPSERT**，
      新库从未种过该键，这条 UPDATE 命中 0 行。安全意图由运行时闭环（路由 404 + 不下发该键 + 后端 0 处读取）。
      **建议把该验收项按"新库 / 老库"拆开写**，否则每次新环境验收都会误报。
      → **已按建议改写**（2026-08-29）：见第五节第 3 节的「新库 / 老库」对照表与三条运行时闭环命令。
      这是文档修正、不涉及代码，**不需要在 RC 镜像上复验**，但按本节约定仍留给站长确认后勾选。
- [ ] **中｜零管理员实例的静默风险**：当存量库"有用户但一个 admin 都没有"时，
      setup 打印 `Database already has user data; skipping auto admin bootstrap` 后什么都不做，
      应用照常 healthy —— 结果是一个**谁都登不进去的实例，且日志只有 INFO 级提示**。
      守卫方向正确（宁可没管理员也不覆盖密码），但**建议把该日志提到 WARN**。
      → **批次 3 已交付修复**（`96f704c11`）：日志提到 WARN 并补上自救指引
      （提升现有用户为管理员，或用容器内的 `/app/adminpass` 重置密码）；**引导逻辑一行未改**。
      **待新 RC 镜像复验后勾选。**
- [ ] **中｜回滚语义需写清**：`0.1.182` 里 `/api/v1/auth/register` **路由依然存在**（返回 403），
      真正关死它的是 229 写进 settings 的开关；**只要有人把开关改回 true，回滚版本的注册就立刻恢复**。
      RC 版本则无论开关如何都是 404。第 7 节应补上这句区别。
      → **已补**（2026-08-29）：第五节第 7 节现在写清了这条区别，
      并顺带重写了整节回滚语义 —— 批次 5 删表删列之后，回滚到批次 5 之前的版本
      **必须连数据库一起恢复**，不能只换 tag。
- [ ] **中｜`server.trusted_proxies` 未配置**：way-rc 前面是 Cloudflare + Traefik 两层代理，
      不配这个拿不到可靠的真实客户端 IP，影响 IP 管理 / 风控 / 限流的准确性。
- [ ] **中｜i18n 仍完整携带已裁剪功能的全部文案**：zh 303KB / en 315KB 两个语言包里
      LinuxDo / 微信 / 钉钉 / OIDC / GitHub / Google / captcha / 注册 字符串一个没删。
      无渲染入口、**不构成功能泄露**，但多打包约 600KB 死文案，且会让所有基于关键字 grep 的
      裁剪审计持续误报（本轮已为此多花排查步骤）。**归入批次 3 的 i18n 清理。**
      → **批次 3 已交付**（`65bcd073d`）：zh/en 各删 61 条键（OAuth 回调、邀请码、优惠码、兑换、
      支付、订阅、首页四个营销区块），删前逐键 grep 确认零引用，删后两边键集合仍完全一致。
      批次 3/4 各包在整删功能时也同步删掉了自己那部分文案。**待新 RC 镜像复验后勾选。**
- [ ] **低｜Passkey 分隔线文案挂错 key**：`LoginView` 里 Passkey 按钮上方仍用 `auth.oauthOrContinue`
      （「或使用其他继续」）。当前 `passkey_enabled=false` 不渲染所以看不见，
      **一旦开启 Passkey，登录页会出现"或使用其他继续"而下面只有一个按钮**，误导用户。
      → **批次 3 已交付修复**（`65bcd073d`）：改用新增的 `auth.passkeyOrContinue`（「或」/「or」）。
      **待新 RC 镜像开启 Passkey 复验后勾选。**
- [ ] **低｜后台邮件模板说明文字过时**：仍在描述"注册""OAuth 补全邮箱""订阅订单支付成功"等已删场景。
      **随批次 3 邮件体系整删一并消失，无需单独处理。**
      → **已随批次 3 消失**（`6173895b2` / `50fddf33e`）：模板编辑器组件与整个「邮件」标签页已删除，
      这段说明文字连同宿主界面一起不存在了。**待新 RC 镜像确认设置页只剩 6 个标签后勾选。**

**记录三个"看起来像问题但不是"的点，避免下次复测重复排查**：

1. `GET /admin/compliance` 返回 **200** 不是没删干净，是 SPA 的 HTML 兜底路由
   （与随机不存在路径响应逐字节 diff，均 1727 字节、只差一个 CSP nonce）；后端真实路径
   `/api/v1/admin/compliance` 才是 404。
2. `GET /api/v1/admin/dashboard` 返回 404 不是裁剪导致，后台实际用的是
   `/api/v1/admin/dashboard/snapshot-v2` 等子路径（浏览器实测 200），是探测时路径猜错。
3. `SettingsView` 内嵌的一整段 Claude Code 系统提示词常量是产品自带「系统提示词模板」功能的默认值，
   属正常业务代码，不是提示词泄露或误提交。做安全扫描时极易误判。

**⚠️ 汇总 Agent 的一处错误（已由主控核实更正）**：汇总环节曾断言"临时栈从未创建"，
依据是它在**演练结束、临时栈已被销毁之后**去查服务器，看到目录与容器都不存在。
实际两个演练 Agent 都完整执行了，证据见上（起栈日志、导入日志、回滚日志、清理日志俱全）。
汇总因此**漏掉了第 2、3、6、7 节的全部结论**，其"10 个可执行项全部 pass"的说法严重低估了本轮覆盖面。
后续若复用该 workflow，汇总环节应改为读取各 Agent 的原始返回，而不是二次去环境里找现场。

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
