.PHONY: build build-backend build-frontend test test-backend test-frontend test-frontend-critical

# vitest 把这些条目当「文件名过滤词」用：指向已删文件的条目不会报错，只是
# 静默少跑一个文件。裁剪期间这里一度有 5 条指向已删除的测试，覆盖面无声缩水，
# 因此 test-frontend-critical 增加了存在性校验，见下方。
FRONTEND_CRITICAL_VITEST := \
	src/api/__tests__/client.spec.ts \
	src/api/__tests__/tokenRefresh.spec.ts \
	src/api/__tests__/channelMonitorV2.spec.ts \
	src/router/__tests__/guards.spec.ts \
	src/router/__tests__/feature-access.spec.ts \
	src/__tests__/integration/navigation.spec.ts \
	src/components/user/profile/__tests__/ProfileInfoCard.spec.ts \
	src/views/admin/__tests__/SettingsView.spec.ts \
	src/views/admin/__tests__/GroupsView.columnSettings.spec.ts \
	src/features/channel-monitor-v2/__tests__/designSystem.structure.spec.ts \
	src/features/channel-monitor-v2/__tests__/monitorFormat.spec.ts \
	src/features/channel-monitor-v2/__tests__/monitorZoom.spec.ts

# 一键编译前后端
build: build-backend build-frontend

# 编译后端（复用 backend/Makefile）
build-backend:
	@$(MAKE) -C backend build

# 编译前端（需要已安装依赖）
build-frontend:
	@pnpm --dir frontend run build

# 运行测试（后端 + 前端）
test: test-backend test-frontend

test-backend:
	@$(MAKE) -C backend test

test-frontend:
	@pnpm --dir frontend run lint:check
	@pnpm --dir frontend run typecheck
	@$(MAKE) test-frontend-critical

test-frontend-critical:
	@for f in $(FRONTEND_CRITICAL_VITEST); do \
		[ -f "frontend/$$f" ] || { echo "FRONTEND_CRITICAL_VITEST 指向不存在的文件: $$f"; exit 1; }; \
	done
	@pnpm --dir frontend exec vitest run $(FRONTEND_CRITICAL_VITEST)
