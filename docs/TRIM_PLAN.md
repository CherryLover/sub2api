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
| 1. Fork 与范围冻结 | 🟡 部分 | 范围与协议决策已定（转发面全保留）；**更新检查/在线升级/Release 拉取未删**（批次 2-①） |
| 2. 数据模型收敛 | ⚪ 未开始 | 修订：不删 User 实体、不建 key_period_usage、不改名 RoutePool；剩余=订阅/余额语义改造 + ent 死实体清理（并入数据库压平） |
| 3. Key 认证与额度引擎 | 🟢 大部分已满足 | 防爆破/IP ACL/幂等结算/惰性窗口/时区均已存在并经审查确认；待做=周额度改自然周锚点 |
| 4. 网关与上游账户池 | ✅ 完成（按决策零改动） | 全量保留 |
| 5. 最小管理员后台 | 🟡 部分 | 商业化入口已拆；Keys/用量页单管理员语义未做 |
| 6. 删除 SaaS 外围 | 🟡 约六成 | 详见批次 1 交付；公告/模型广场/邮销未删；监控 V1/批量生图/内容审计去留未决 |
| 7. 安全与上线验证 | 🔵 进行中 | 三轮 RC 镜像内网部署验证已完成（空库全新部署、存量库升级、修复复验），全部通过；压测与备份恢复演练仍未做 |

**当前位置**：批次 1 已完成三轮内网部署验证并全部通过，验证问题清单已清空（见第六节），
可执行合并 main。
**关口**：auto-release 经站长决策**保持现状不动**（合并后自动发版即预期行为，内部版本线就走 main）。

---

## 二、批次 1（已交付，分支 `claude/project-trim-internal-deploy-7jnynw`，CI 全绿）

- 支付/订单/充值/卡密/优惠码/推广返佣：前后端整套删除（含 5 个支付 provider、支付 SDK 依赖）
- 六种第三方 OAuth 登录（LinuxDo/微信/钉钉/OIDC/GitHub/Google）：整套删除，含 pending 绑定流程与设置面
- 多用户默认禁用：注册种子默认关闭 + 迁移 `229` 对老库升级强制关闭一次
- 保留面确认无损：邮箱密码登录、TOTP、Passkey、订阅体系、上游账号 OAuth、ops 监控、备份、审计
- 护栏：门禁契约测试（37 项断言，已删路由缺席 + 注册默认拒绝 + 保留面防误删）；对抗审查闭环（1×P1 已修，4×P2 两修两记录）
- 交付物：RC 镜像流水线（`branch-docker-image.yml`，`internal-rc-*` 标签触发）+ 部署验证清单（见第五节）

已记录的取舍：管理员调整余额不再留业务流水（HTTP 审计日志兜底）；ent 中 payment/redeem
等死实体与历史迁移暂保留，统一在数据库压平批次处理。

---

## 三、批次 2 待办

### B 组：首轮部署体验反馈（2026-08-26，站长实测提出）

| 编号 | 内容 | 备注 |
|---|---|---|
| B1 | **移除登录条款/合规确认** | 首次打开的"我已同意"确认整套删除：`AdminComplianceGuard` 中间件、`/admin/compliance` 路由、`docs/legal/`（admin-compliance 等文档）、`/legal/:documentId` 前端页、登录页条款勾选 |
| B2 | **移除通用设置冗余项** | 站点名称、站点副标题、API 端点地址、通用表格设置、联系人、Logo、首页内容、简洁首页、隐藏 CSS/CCM 导入按钮、自定义映射页面等站点品牌/运营面设置（实施时以 SettingsView 实际条目为准逐项核对，程序内部对站点名等的引用改为固定值） |
| B3 | **注册体系彻底移除** | 不再是"默认关闭"而是整体删除：注册开关、注册路由/页面、邮箱验证码注册流、注册用户默认值（signup 授予/authSourceDefaults 残余）。只保留管理员账号访问；管理员后台建用户能力去留在实施时一并评估 |
| B4 | **人机验证全部移除** | Turnstile/腾讯/阿里三家验证码整套删除（批次 1 曾保留为休眠态，实测确认无需要：自用+API 调用场景无人机验证需求） |
| B5 | **邮件体系收敛** | 注册/验证类邮件随 B3 删除。⚠️ 待站长拍板：SMTP 及 ops 告警邮件、余额/配额提醒邮件是否整体删除（删则告警只剩面板内查看） |

**B 组明确保留**：管理员 Passkey、TOTP/2FA、敏感操作二次确认（step-up）、自定义登录入口路径、默认主页设置。

### A 组：既有 backlog（顺序建议）

| 编号 | 内容 | 对应原阶段 |
|---|---|---|
| A1 | 🟡 核心已落地（修复轮：应用内更新检查/在线升级/回滚整套已删，版本号展示与 restart 保留；`update.proxy_url` 因定价表与 Codex 同步仍消费而保留原键名）。尾款：install.sh 升级逻辑、docker-deploy.sh、auto-release.yml 去留决策 | 阶段 1 |
| A2 | 公告、模型广场、邮件营销面删除；渠道监控 V1/批量生图/内容安全审计**去留决策后执行**（倾向：留监控 V2 删 V1；删批量生图与内容审计） | 阶段 6 |
| A3 | 周额度改自然周（周一 00:00）锚点 | 阶段 3 |
| A4 | 订阅/余额拆除与 Key 额度直绑改造（最难一刀，单独里程碑） | 阶段 2/6 |
| A5 | 数据库压平：删除死实体（payment/redeem/promo/subscription_plan 等）、迁移基线重置、支持全新初始化 | 阶段 2 |
| A6 | 管理后台单管理员化（Keys/用量页直管语义） | 阶段 5 |
| A7 | 部署文档改写（README/DEV_GUIDE 内部化）+ 压测、备份恢复、Key 泄露演练 | 阶段 7 |

排序建议：B1–B4 与 A1 合并为批次 2 首发（均为低耦合删除，体验收益立竿见影）→ A2 → A3 → A4 → A5 → A6 → A7。B5 待决策后并入。

---

## 四、遗留与技术备注

- 4 个基线遗留的未格式化 unit 测试文件（golangci-lint 未配 build-tags 检不到；补配置前先 gofmt）
- `.gitignore` 对 `docs/*` 为白名单模式，docs 新文件需 `git add -f`
- 本会话 GitHub 集成无 actions:write，workflow 触发走 `internal-rc-*` 标签推送
- 迁移 229 为幂等 UPDATE；本轮未删表未改列，镜像回滚无兼容性问题

---

## 五、内部 RC 镜像部署验证清单


本清单面向站长，用于在内网实际部署验证"单管理员内部部署"裁剪分支的镜像。
镜像由 `.github/workflows/branch-docker-image.yml`（手动触发）构建，只推 GHCR：

- 浮动标签：`ghcr.io/cherrylover/sub2api:internal-rc`（每次分支构建会顶掉，验证期 compose 固定引用它）
- 不可变标签：`ghcr.io/cherrylover/sub2api:internal-<short_sha>`（定位/回滚到具体提交用）

验证内容分三块：空库全新部署、裁剪面验证、老库升级演练。全部通过后把结论告知协调方即可，
合并 main 与 auto-release 由协调方执行。

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
    image: weishaw/sub2api:latest
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
# 把 image 改回原坐标（如 weishaw/sub2api:latest 或此前固定的正式版本 tag）后：
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
