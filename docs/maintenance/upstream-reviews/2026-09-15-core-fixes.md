# 2026-09-15 精简版选择性回移记录

## 基线与范围

- 精简版：CherryLover/sub2api main，v0.1.194，`f8bb54204cb60b79bb1c0eeecda05009d55e72c4`。
- 上一轮 PR [#11](https://github.com/CherryLover/sub2api/pull/11) 已合入；本轮不重复处理。
- 上游核对点：Wei-Shaw/sub2api main `32682a4f84c6a29104439050a0be14737552cec2`，稳定版仍为 v0.2.4。本轮是已确认的主线候选，不表示该核对点之前全部适用变更都已回移。
- 个人 fork main 已快进到精简版基线；同步期间关闭 Actions，之后恢复原设置。发版和镜像工作流继续保持禁用。
- 工作分支：`backport/2026-09-15-core-fixes`。当前仅完成本地提交，未推送本轮工作分支，未创建本轮 PR。

## 已实施

所有代码提交均含完整 `Upstream-reference`；以下短 SHA 可在本分支解析。

| 上游 PR / SHA | 问题与精简版证据 | 最小回移及保留项 | 本地提交 | 回归 |
|---|---|---|---|---|
| [#7082](https://github.com/Wei-Shaw/sub2api/pull/7082) / 1e1d15cca527e31759ec1f1cc5b5876cc6f8377b | Antigravity 原先优先用 project_id 作为令牌缓存键，不同账号可碰撞 | 统一按账号隔离；失效时同时清除旧 project 键 | f883350ae | token_cache_key_test |
| [#7126](https://github.com/Wei-Shaw/sub2api/pull/7126) / fb90414c0512537478e3aef4de03c0dcfd70a7aa | namespace 声明检测只查看顶层 tools，漏掉 Lite additional_tools | 检测嵌套声明；保留精简版 OAuth、setup-token、compact 和摊平策略 | 535c7abb8 | namespace 单测和转发回归 |
| [#7085](https://github.com/Wei-Shaw/sub2api/pull/7085) / 008391f699bdd3b63e93550aefb5c9e1ab631011 | OAuth 各入口未清理 input 项的内部消息元数据 | 在现有 HTTP 转换/透传/compact、WS 入站/透传/HTTP 桥接接入定向清理；不引入上游缺失的整套兼容层，不改 prompt 策略或 API Key 行为 | ca3fbdd8c | 元数据边界单测；真实 WS relay 首轮和续轮（OAuth/API Key） |
| [#7111](https://github.com/Wei-Shaw/sub2api/pull/7111) / f7e959ae5f9fccb97ee12ffebd0dc3036c32c987 | 空字符串被当作“不修改”，无法清除代理认证 | 更新输入用指针区分缺省和显式清空；保留未触碰密码 | 842514d40 | handler/service/前端凭据回归 |
| [#6810](https://github.com/Wei-Shaw/sub2api/pull/6810) / 4892b8f17edcda6a4355ded98c190ac5775982b9 | 代理日期可超出 JSON 时间可序列化范围 | 仅代理日期边界和前端 max；不回移公告功能 | 191d46406 | 非法年份拒绝、0/9999 边界 |
| [#6798](https://github.com/Wei-Shaw/sub2api/pull/6798) / 6378fb0cbaff51c968470ef2fe5271e79d59b9fc | 账号菜单用固定高度估算，长菜单可越过视口 | 实测定位、尺寸上限、内部滚动；保留查用量入口、影子账号限制和状态到期心跳 | 687afe9d5 | 定位10项、影子账号11项、状态到期5项、页面内部/外部滚动 |
| [#6874](https://github.com/Wei-Shaw/sub2api/pull/6874) / 5de5e2bed035d43591a2e10e51f420ef6a84eb98 | image_gen 工具用量遗漏图片输入 token | 仅提取 ImageInputTokens；保留有界数值解析，钳制不超过总输入，不加入模型/价格/默认模型更新 | 2b6bbbef9 | 缺省/有效/负数/字符串/小数/指数/极端指数/超总量，以及 SSE |
| [#6815](https://github.com/Wei-Shaw/sub2api/pull/6815) / c54897a59dfe3faa819f2a33117aa423353e2a1e | Ent 把备用代理当双向一对一，更新会错误影响反向引用 | 独立一组改成有向多对一；基于精简版 schema 重新生成，不复制上游整套模型 | d583ff2bc | SQL mock 已更新；共享/更新/清除/链式引用集成用例已编译，运行待 GitHub |

## 数据库与独立修复

- 既有迁移 `149_proxy_expiry_fallback.sql` 已使用普通索引和有向外键（ON DELETE SET NULL），没有 backup_proxy_id 唯一约束。本轮修正 ORM 与既有 SQL 的不一致，无新 SQL 迁移、无迁移编号变更。
- 集成测试通过现有 ApplyMigrations 建库后测试代理引用。Mac mini 无 Docker，尚未验证实际 PostgreSQL 运行及旧库数据场景；该组必须以 GitHub 集成结果为合入门槛。
- 不自动修复历史上已经被错误 ORM 写坏的代理配置；如发现此类数据，单独列出并由人类确认处理。
- 保留“数据库写入失败仍保留账号内存冷却”，没有改动相关冷却代码；不引入 #6320 的持久化快照自动清除策略。
- 保留精简版已有裁剪边界、图片 token 防滥用解析、namespace 策略及账号管理独立修复；未加入新平台或新用户流程。

## 验证证据（本地）

代码树对应上述8组提交；另将新增前端回归加入根 Makefile 的关键 CI 清单。后续文档提交不改变业务代码。

| 检查 | 结果 / 证据 |
|---|---|
| 后端定向回归 | 通过；WebCodex job a2d1ddc9-ddc4-42f6-abdd-a6a204104e50 |
| make -C backend test-unit | 全量通过；job 42f8fe54-3b4d-467f-95ac-bbeb6ed37498。随后新增的 WS 首轮/续轮测试单独通过 |
| make test-frontend | lint:check、typecheck、31个文件339项关键测试通过；job 5a80a177-42c1-4b7f-87f8-037da19d06a1 |
| pnpm --dir frontend run build | 通过；job 5fcbd2ae-7594-4915-a0ef-25228afa36ab。有非阻断的分包/Browserslist 提示 |
| 部署脚本 | CI 配置中的 bash 语法、Apple容器模拟生命周期及4组 shell 检查通过；未操作实际生产容器 |
| WS新增回归 / golangci-lint / embed构建 | 通过；job 1130e74d-e61f-446f-841f-15b6b2313c4f，lint 0 issues |
| govulncheck ./... | 通过；同上 job。当前调用路径0漏洞；依赖模块有13项未触达发现，非“所有依赖零漏洞” |
| 前端生产依赖审计 | 按既有 audit-exceptions.yml 校验通过；pnpm audit 本身退出1，不宣称零告警，未扩大例外 |
| repository integration 编译 | 通过；同上 job，仅 go test -tags=integration -c |
| PostgreSQL/Redis 集成运行 | 未运行：本地无 Docker；等待 GitHub |
| GitHub CI / Security Scan | 未运行：等待用户确认推送本轮分支；运行链接和最终 SHA 在推送后补齐 |
| diff / 裁剪范围 | git diff --check 通过；无 go.mod/go.sum/pnpm-lock 或裁剪模块变更 |

环境沿用仓库隔离的 Go1.26.6、Node20、pnpm9、golangci-lint2.9.0。前端审计校验在本机 Python3.9 仅启用 postponed annotations 执行原脚本，不修改脚本或例外。

## 暂缓与交付

- 暂缓 #7094（角色降级）、#7064（WS容量语义）、#6434（断开后排空）、#6424（日志留存/默认行为）、#6235（多实例缓存），不随本轮自动带入。旧报告其余未解决项继续保留，不标记为已覆盖。
- 获准后仅推送个人 fork 工作分支；检查同一最终 SHA 的 CI 和 Security Scan 全部任务，尤其数据库集成测试。失败先修复重验，通过后向 CherryLover/main 提 PR；不自行合并、发版或推镜像。
- 回滚：以问题组提交逆序 revert；#6810 的边界测试依赖 #7111 的测试桩，回退凭据组时连同日期组处理。数据库组无新增迁移，回退代码不会自动修复历史代理数据。

## 阶段状态（待推送）

- 本轮本地提交已完成，当前分支末端：`1665aacd45ff78854be3b099e5dc0abe77576b66`。
- 工作区已核对为干净状态；本阶段没有推送、没有创建 PR，也没有触发 GitHub Actions。
- 下一步需用户确认后推送个人 fork 的工作分支，再以该最终 SHA 检查 CI、Security Scan 和数据库集成测试；全部通过后才创建面向 CherryLover/sub2api 的 PR。
