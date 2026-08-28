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

**当前位置**（2026-08-28 更新）：批次 1 已合并 main 并发版 v0.1.182（收口事实见第二节）；
**批次 2 代码全部交付、CI 全绿，RC 镜像 `internal-1e49744` 已部署到 way-rc，验证清单待走**（见第六节）。
**B5 邮件体系与 A2 去留已于 2026-08-28 由站长拍板**，重排后的批次计划见第三节「站长决策与重排」。
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
| A2 | 公告、模型广场、邮件营销面删除；渠道监控 V1/批量生图/内容安全审计**已于 2026-08-28 拍板**：删内容安全审计、删批量生图、监控删 V1 留 V2（并把线上 `channel_monitor_mode` 切到 `v2`）。详见「站长决策与重排」 | 阶段 6 |
| A3 | 周额度改自然周（周一 00:00）锚点 | 阶段 3 |
| A4 | 订阅/余额拆除与 Key 额度直绑改造（最难一刀，单独里程碑） | 阶段 2/6 |
| A5 | 数据库压平：删除死实体（payment/redeem/promo/subscription_plan 等）、迁移基线重置、支持全新初始化 | 阶段 2 |
| A6 | 管理后台单管理员化（Keys/用量页直管语义） | 阶段 5 |
| A7 | 部署文档改写（README/DEV_GUIDE 内部化）+ 压测、备份恢复、Key 泄露演练 | 阶段 7 |

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

| 批次 | 范围 | 性质 |
|---|---|---|
| **批次 2**（代码已完成） | B1 登录条款 / B2 通用设置冗余项 / B3 注册体系 / B4 人机验证 / A1 尾款 | **待 way-rc 验证** |
| **批次 3** | B5 邮件体系整删（+ 密码重置小工具）、内容安全审计删除、批量生图删除、渠道监控 V1 删除（模式切 V2）、A2 剩余（公告 / 模型广场 / 邮件营销）、第四节「后续候选项」里的小尾巴（CSP 失效白名单、支付遗留文档、`registration_email_domain_quota_enabled` 等） | 低耦合删除 |
| **批次 4**（代码已完成） | A4 订阅/余额拆除 + Key 额度直绑；**注册用户默认值随本批消失** | 最难一刀，**待 CI 与 way-rc 验证** |
| **批次 5**（代码已完成） | A5 数据库压平：payment / redeem / promo / subscription 死实体与死表清理；**充值兑换残留已清干净**。迁移基线重置**未做**，见下方说明 | 结构性 |
| **批次 6** | A3 周额度自然周锚点、A6 后台单管理员化、A7 文档改写 + 压测 / 备份恢复 / Key 泄露演练 | 收尾 |

> **批次 4 落地说明（2026-08-29）**：额度语义已统一到 `api_keys.quota / quota_used`，
> 认证中间件只剩「Key 状态 / 过期 / Key 额度」三道闸门，结算主干不再写
> `users.balance` 与 `user_subscriptions`。配套迁移 `234_drop_subscription_and_balance_semantics.sql`
> 做了三件事保证存量可用：① 按「已有 Key 绑在该专属分组上」的既成事实回填
> `user_allowed_groups`（补上旧代码里"订阅型分组直接放行"那条被取消的旁路）；
> ② 把非订阅分组上**原本就不生效**的 `peak_rate_enabled` 显式关掉，使升级前后计费口径一致；
> ③ 分组 `subscription_type` 归一为 `standard` 并清掉 8 个已无读写方的设置键。
> 迁移不删表不删列，`user_subscriptions` / `users.balance` / `groups.subscription_type`
> 与三个限额列全部保留给批次 5（A5）处理，因此本批可整体回滚。
>
> ⚠️ **两项行为变更需要在 way-rc 上重点复验**：
> 1. `quota_used` 现在**无条件累加**（旧实现只在 `quota > 0` 时累加），
>    不限额 Key 的 `quota_used` 会从 0 开始增长——这是"所有 Key 都记账"的前提，
>    但对存量 Key 是可见的行为变化。
> 2. Key 额度成为唯一闸门后，超支窗口从「Redis 实时余额」退化为
>    「auth cache TTL + 并发 in-flight」——`CheckBillingEligibility` 不再做额度预检，
>    耗尽判定依赖认证缓存快照与结算后的 `InvalidateAuthCacheByKey`。
>    内部单管理员部署可接受；若将来要收紧，需把 Key 额度接进 billing-cache 预检。

> **批次 5 落地说明（2026-08-29）**：本批第一次在本地跑通了真正的 Go 工具链
> （go1.26.6 临时下载到 scratchpad，用完即删）与一套独立的 PostgreSQL 18.1，
> 因此结论都是实测而非静态推断。
>
> **重要发现**：批次 5 开工时，分支上 `go build ./...` **本身就编译不过**
> —— 批次 3 删公告时删掉了 `domain.AnnouncementTargeting`，但生成的
> `ent/announcement*.go` 还在引用它。也就是说批次 3 和 4 之后，CI 的
> 单测 / 集成 / lint 三条流水线全部会红。本批已一并修好。
>
> 实际交付：
> - **ent 死实体**：删除 17 个 schema 与全部生成物（支付 3 + 兑换促销 3 +
>   订阅 2 + 批量生图 3 + 渠道监控 V1 4 + 公告 2）；User 去 6 条边、
>   Group 去 2 条边 + 3 个批量生图字段、UsageLog 去指向 user_subscriptions 的边。
>   **ent 是用官方代码生成器重新生成的，不是手工改生成物。**
> - **死表**：迁移 235 DROP 21 张表（含推广返佣 2 张、内容审计日志 1 张、
>   渠道监控 V1 的聚合水位表）；迁移 236 DROP `groups` 的 3 个批量生图列；
>   迁移 237 清掉 50 多个孤儿设置键。
> - **刻意保留**：`usage_logs.subscription_id`（repository 裸 SQL 与 DTO 仍在用）、
>   `groups.subscription_type`（可用渠道 DTO 之外，migration 193 的 auth cache
>   触发器函数引用了这一列）、`users.balance` 家族（列还在，service 层已不读）。
> - **实测验证**：全新库跑完 277 条迁移无报错、最终只剩 4 个活设置键；
>   用 `fork/main` 的 269 条迁移建出「升级前老库」并塞入代表性数据后再升级，
>   28 项断言全过；停在 123 号迁移的老库也能直接升到最新；重复执行为 no-op。
>
> ⚠️ **迁移基线重置：本批未做，留给站长决策**。理由是实测数据不支持这项改造：
> 全新库跑完 277 条迁移只要 **0.3–0.6 秒**，"顺序跑历史迁移"并不是真实痛点。
> 而任何形式的基线重置都要么改迁移执行器（新增"全新库走基线、存量库标记已应用"
> 的分叉逻辑），要么删历史迁移文件——后者会让**停在中间版本的存量库无法升级**
> （例：删掉建 `batch_image_jobs` 的 159，而 187 里还有引用它的语句）。
> 若将来仍要做，建议的最小安全方案是：只加"全新库快速路径"，不删任何历史文件——
> ① 用一次性脚本把 277 条迁移跑进空库后 `pg_dump --schema-only` 生成
> `000_baseline.sql`，并在其中附带把全部历史迁移写入 `schema_migrations` 的 INSERT；
> ② 在 `applyMigrationsFS` 开头判断 `schema_migrations` 是否为空：非空（存量库）
> 就把 `000_baseline.sql` 直接标记为已应用而不执行；③ 加一个集成测试对拍
> 「按历史链建库」与「按基线建库」两份 `pg_dump` 必须一致。

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

**结论**：后续批次不必再靠「脚本模拟 gofmt 对齐」这类替代手段。批次 3 和 4 遗留的
23 个 gofmt 偏差、5 处测试编译失败、4 个失效断言，正是这套模拟手段的代价。

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

### 批次 2 实施中记录的后续候选项

- [x] **CSP 白名单失效条目**（批次 3 已处理）：强制注入表从 27 条收敛为 1 条，只留 Cloudflare Web Analytics；天御国内/国际站、Turnstile、阿里云验证码、Stripe、Airwallex 的条目与默认策略里的对应域名全部删除，`frame-src` 收紧为 `'none'`。新增防回归用例锁死这批主机不许回流
- [x] **`registration_email_domain_quota_enabled`**（批次 3 已处理）：整键删除 + 迁移 233 清库。**核实结论修正**：兄弟键 `registration_email_suffix_whitelist` 的邮箱绑定校验路径已随批次 3 邮件体系整删消失（`AuthService.validateRegistrationEmailPolicy` 零调用点，已一并删除），但该键**必须保留**——它是 `InitializeDefaultSettings` 判断"是否已种过默认设置"的探测键，删掉会导致每次启动重跑种子
- [ ] **`LOGIN_ENTRY_RESERVED_PREFIXES` 的 `/legal`、`/custom`**（前端 SettingsView 与后端 `config/web_entry.go` 镜像）：对应路由已删，但**刻意保留**——这是自定义登录入口的保留字黑名单，删条目只会放宽允许集，属安全收紧项而非裁剪项。若要收窄需前后端同步改并调整 `web_entry_test.go` 用例。批次 3 删模型广场时同理保留了 `/model-plaza`（但把它从**默认落地页白名单** `allowedDefaultHomePaths` 里删掉了，那是白名单，留着等于允许把首页指到已删页面）
- [x] **支付遗留文档**（批次 3 已处理）：三份文档删除，三个 README 里指向它们的「内置支付系统」条目与生态表格行一并删掉
- [ ] **`custom_endpoints` 设置**：与已删的 `api_base_url` 在设置页相邻但语义不同，本批未动，待站长确认是否一并删除
- [ ] **ent 中的支付实体**（`payment_orders`/`payment_provider_instances` 等）与相关历史迁移：需 ent 重生成，归入 A5 数据库压平
- [ ] **余额变动记录入口仍指向已删接口**（批次 3 新发现）：`UsageTable.vue` 与 `OpsErrorLogTable.vue` 的余额 tooltip 仍用 `admin.usage.clickToViewBalance`（「点击查看充值记录」），而 `/api/v1/admin/users/:id/balance-history` 已在批次 1 删除；`admin.users.balanceHistory*` 一整组文案同理。点击行为需要复核后再决定是改文案还是去掉入口
- [ ] **`SettingsView.spec.ts` 的 vue-i18n mock 字典与 `baseSettingsResponse` 仍带已删字段**（批次 3 新发现）：`admin.settings.wechatConnect.*`、`admin.settings.payment*`、`admin.settings.site.*`、`registration_enabled` / `promo_code_enabled` 等。只存在于测试桩里，不进产物，但会让基于关键字 grep 的裁剪审计继续误报
- [ ] **`admin.groups.claudeMaxSimulation` 中英键路径不一致**（批次 3 新发现，非本轮引入）：zh 挂在 `admin.groups.modelRouting.claudeMaxSimulation`，en 挂在 `admin.groups.claudeMaxSimulation`，两边有一边取不到值

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

- 原有管理员账号可正常登录（存量库已有管理员时，AUTO_SETUP 会跳过引导、不覆盖原密码，
  日志出现 INFO "Admin user already exists, skipping admin bootstrap"）；
  若库里有用户但**一个管理员都没有**，批次 3 起会打 WARN
  "Warning: database already has user data but no admin account..."，
  按提示用 `docker exec <container> /app/adminpass -email you@example.com -stdin` 自救；
- 原有分组 / API Key / 用量数据在后台完好可见；
- 用原有 API Key 按第 5 节方式真发一条请求，转发仍然成功，且新记录进入用量页。

---

### 7. 回滚路径

代码回滚 = 把 `.env` 里的 `SUB2API_IMAGE` 换回旧 tag，重启应用即可：

```bash
# 把 SUB2API_IMAGE 换成上一个可用的内部标签（如
# ghcr.io/cherrylover/sub2api:internal-<旧 short_sha>
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

### internal-1e49744（2026-08-28，第四轮：批次 2 验收——进行中）

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
- [ ] **中｜零管理员实例的静默风险**：当存量库"有用户但一个 admin 都没有"时，
      setup 打印 `Database already has user data; skipping auto admin bootstrap` 后什么都不做，
      应用照常 healthy —— 结果是一个**谁都登不进去的实例，且日志只有 INFO 级提示**。
      守卫方向正确（宁可没管理员也不覆盖密码），但**建议把该日志提到 WARN**。
- [ ] **中｜回滚语义需写清**：`0.1.182` 里 `/api/v1/auth/register` **路由依然存在**（返回 403），
      真正关死它的是 229 写进 settings 的开关；**只要有人把开关改回 true，回滚版本的注册就立刻恢复**。
      RC 版本则无论开关如何都是 404。第 7 节应补上这句区别。
- [ ] **中｜`server.trusted_proxies` 未配置**：way-rc 前面是 Cloudflare + Traefik 两层代理，
      不配这个拿不到可靠的真实客户端 IP，影响 IP 管理 / 风控 / 限流的准确性。
- [ ] **中｜i18n 仍完整携带已裁剪功能的全部文案**：zh 303KB / en 315KB 两个语言包里
      LinuxDo / 微信 / 钉钉 / OIDC / GitHub / Google / captcha / 注册 字符串一个没删。
      无渲染入口、**不构成功能泄露**，但多打包约 600KB 死文案，且会让所有基于关键字 grep 的
      裁剪审计持续误报（本轮已为此多花排查步骤）。**归入批次 3 的 i18n 清理。**
- [ ] **低｜Passkey 分隔线文案挂错 key**：`LoginView` 里 Passkey 按钮上方仍用 `auth.oauthOrContinue`
      （「或使用其他继续」）。当前 `passkey_enabled=false` 不渲染所以看不见，
      **一旦开启 Passkey，登录页会出现"或使用其他继续"而下面只有一个按钮**，误导用户。
- [ ] **低｜后台邮件模板说明文字过时**：仍在描述"注册""OAuth 补全邮箱""订阅订单支付成功"等已删场景。
      **随批次 3 邮件体系整删一并消失，无需单独处理。**

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
