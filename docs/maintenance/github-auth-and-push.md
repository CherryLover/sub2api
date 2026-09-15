# GitHub 授权、推送与 PR 操作记录

用于本项目后续对话复用。不要在文档、命令参数或日志中保存 GitHub Token、密码或验证码。

## 1. 授权检查

在 WebCodex 的 Mac mini 项目环境中执行：

```bash
.git/codex-tools/bin/gh auth status
```

确认：

- 登录账号是 `xiaoguai1818`
- Git 操作协议为 HTTPS
- Token 具备 `repo` 和 `workflow` 权限

如果授权失效，执行：

```bash
.git/codex-tools/bin/gh auth login --hostname github.com --git-protocol https --web
```

命令会显示一次性验证码和网页登录地址。用户在 Mac mini 浏览器打开地址、输入验证码并完成授权；不要让 AI 读取或保存 Token。

## 2. 本项目远程关系

- `origin`：`https://github.com/xiaoguai1818/sub2api.git`，个人临时工作 fork
- `lite`：`https://github.com/CherryLover/sub2api.git`，精简版最终 PR 目标
- `upstream`：`https://github.com/Wei-Shaw/sub2api.git`，完整上游项目

每轮从精简版最新 `main` 建立独立工作分支，不直接向精简版 `main` 推送。

## 3. 推送个人工作分支

先确认工作区、分支和提交：

```bash
git status --short --branch
git log -1 --oneline
```

使用已授权的 gh 凭据推送，不在命令行展开 Token：

```bash
git -c credential.helper= \
  -c credential.helper='!/Users/leobot/Projects/sub2api/.git/codex-tools/bin/gh auth git-credential' \
  push --set-upstream origin backport/<工作分支>
```

若分支已建立跟踪关系，后续使用同样的凭据 helper 执行 `git push`。

## 4. 推送后检查 GitHub 工作流

先列出该分支的 CI 和 Security Scan：

```bash
.git/codex-tools/bin/gh run list \
  --repo xiaoguai1818/sub2api \
  --branch backport/<工作分支> \
  --limit 10 \
  --json databaseId,workflowName,status,conclusion,headSha,url
```

使用最终提交对应的 run ID 等待结果：

```bash
.git/codex-tools/bin/gh run watch <run-id> \
  --repo xiaoguai1818/sub2api \
  --exit-status
```

CI 必须确认前端、部署脚本、后端 embed、unit、integration 和 golangci-lint 全部通过；Security Scan 必须确认 govulncheck 与前端审计例外校验通过。未运行或被跳过的检查不能记录为通过。

## 5. 创建面向精简版的 PR

CI 和 Security Scan 都通过后，检查是否已有同源 PR，再创建：

```bash
.git/codex-tools/bin/gh pr list \
  --repo CherryLover/sub2api \
  --head xiaoguai1818:backport/<工作分支> \
  --state open

.git/codex-tools/bin/gh pr create \
  --repo CherryLover/sub2api \
  --base main \
  --head xiaoguai1818:backport/<工作分支> \
  --title "<简洁标题>" \
  --body "<范围、来源、测试结果和未解决事项>"
```

创建后只交付 PR 链接和验证结果，不自动合并、不发版、不推镜像。PR 合入精简版后，本轮工作结束，下一轮重新从精简版最新 `main` 开始。

## 6. 本次实测结果

- 授权账号：`xiaoguai1818`
- 工作分支：`backport/2026-09-15-core-fixes`
- 精简版 PR：[CherryLover/sub2api#12](https://github.com/CherryLover/sub2api/pull/12)
- CI 与 Security Scan 已在该分支最终提交上通过，包括 PostgreSQL/Redis 集成测试。
