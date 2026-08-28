<template>
  <div class="relative flex min-h-screen flex-col bg-gray-50 dark:bg-dark-950">
    <!-- Header (same pattern as HomeView) -->
    <header class="relative z-20 px-6 py-4">
      <nav class="mx-auto flex max-w-6xl items-center justify-between">
        <router-link to="/home" class="flex items-center gap-3">
          <div class="h-10 w-10 overflow-hidden rounded-xl shadow-md">
            <img src="/logo.svg" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <span class="text-lg font-semibold tracking-tight text-gray-900 dark:text-white">{{ SITE_NAME }}</span>
        </router-link>
        <div class="flex items-center gap-3">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>
          <button
            @click="toggleTheme"
            class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>
        </div>
      </nav>
    </header>

    <!-- Main Content -->
    <main class="flex-1 w-full max-w-5xl mx-auto px-6 py-12">
      <!-- Hero -->
      <div class="text-center mb-12">
        <h1 class="text-3xl sm:text-4xl font-bold tracking-tight mb-3 text-gray-900 dark:text-white">
          {{ t('keyUsage.title') }}
        </h1>
        <p class="text-gray-500 dark:text-dark-400 text-base max-w-md mx-auto">
          {{ t('keyUsage.subtitle') }}
        </p>
      </div>

      <!-- Input Section -->
      <div class="max-w-xl mx-auto mb-14">
        <div v-if="!isTokenMode" class="flex gap-3">
          <div class="flex-1 relative">
            <div class="absolute left-4 top-1/2 -translate-y-1/2 text-gray-400 dark:text-dark-500">
              <svg class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <rect x="3" y="11" width="18" height="11" rx="2" ry="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>
              </svg>
            </div>
            <input
              v-model="apiKey"
              :type="keyVisible ? 'text' : 'password'"
              :placeholder="t('keyUsage.placeholder')"
              class="input-ring w-full h-12 pl-12 pr-12 rounded-xl border border-gray-200 bg-white text-sm text-gray-900 placeholder:text-gray-400 transition-all dark:border-dark-700 dark:bg-dark-900 dark:text-white dark:placeholder:text-dark-500"
              @keydown.enter="queryKey"
            />
            <button
              @click="keyVisible = !keyVisible"
              class="absolute right-4 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-700 dark:text-dark-500 dark:hover:text-white transition-colors"
            >
              <svg v-if="!keyVisible" class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M17.94 17.94A10.07 10.07 0 0 1 12 20c-7 0-11-8-11-8a18.45 18.45 0 0 1 5.06-5.94M9.9 4.24A9.12 9.12 0 0 1 12 4c7 0 11 8 11 8a18.5 18.5 0 0 1-2.16 3.19m-6.72-1.07a3 3 0 1 1-4.24-4.24"/>
                <line x1="1" y1="1" x2="23" y2="23"/>
              </svg>
              <svg v-else class="w-5 h-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/>
              </svg>
            </button>
          </div>
          <button
            @click="queryKey"
            :disabled="isQuerying"
            class="h-12 px-7 rounded-xl bg-primary-500 hover:bg-primary-600 text-white font-medium text-sm transition-all active:scale-[0.97] flex items-center gap-2 whitespace-nowrap disabled:opacity-60"
          >
            <svg v-if="isQuerying" class="w-4 h-4 animate-spin" viewBox="0 0 24 24" fill="none">
              <circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="3" opacity="0.25"/>
              <path d="M12 2a10 10 0 0 1 10 10" stroke="currentColor" stroke-width="3" stroke-linecap="round"/>
            </svg>
            <svg v-else class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/>
            </svg>
            {{ isQuerying ? t('keyUsage.querying') : t('keyUsage.query') }}
          </button>
        </div>
        <p
          v-if="!isTokenMode"
          data-testid="privacy-note"
          class="mt-3 text-center text-xs leading-relaxed text-gray-500 dark:text-dark-400"
        >
          {{ t('keyUsage.privacyNote') }}
        </p>

        <!-- Date Range Picker -->
        <div v-if="showDatePicker" class="mt-4">
          <div class="flex flex-wrap items-center gap-2 justify-center">
            <span class="text-xs text-gray-500 dark:text-dark-400">{{ t('keyUsage.dateRange') }}</span>
            <button
              v-for="range in dateRanges"
              :key="range.key"
              @click="setDateRange(range.key)"
              class="text-xs px-3 py-1.5 rounded-lg border transition-all"
              :class="currentRange === range.key
                ? 'bg-primary-500 text-white border-primary-500'
                : 'border-gray-200 bg-white text-gray-700 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-200 hover:border-primary-300 dark:hover:border-dark-600'"
            >{{ range.label }}</button>
            <div v-if="currentRange === 'custom'" class="flex items-center gap-2 ml-1">
              <input
                v-model="customStartDate"
                type="date"
                class="input-ring text-xs px-2 py-1.5 rounded-lg border border-gray-200 bg-white text-gray-900 dark:border-dark-700 dark:bg-dark-900 dark:text-white"
              />
              <span class="text-xs text-gray-400">-</span>
              <input
                v-model="customEndDate"
                type="date"
                class="input-ring text-xs px-2 py-1.5 rounded-lg border border-gray-200 bg-white text-gray-900 dark:border-dark-700 dark:bg-dark-900 dark:text-white"
              />
              <button
                @click="refreshReport"
                class="text-xs px-3 py-1.5 rounded-lg bg-primary-500 text-white hover:bg-primary-600"
              >{{ t('keyUsage.apply') }}</button>
            </div>
          </div>
        </div>
      </div>

      <!-- Lookup Session Bar (token mode) -->
      <div
        v-if="isTokenMode"
        data-testid="session-bar"
        class="mx-auto mb-10 max-w-3xl rounded-2xl border border-primary-200 bg-primary-50/70 p-4 dark:border-primary-500/30 dark:bg-primary-500/10"
      >
        <div class="flex flex-wrap items-center gap-3">
          <span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-primary-500/15 text-primary-600 dark:text-primary-300">
            <Icon name="link" size="sm" />
          </span>
          <div class="min-w-0 flex-1">
            <p class="truncate text-sm font-medium text-gray-900 dark:text-white">
              {{ keyInfo?.name ? t('keyUsage.session.activeFor', { name: keyInfo.name }) : t('keyUsage.session.active') }}
            </p>
            <p v-if="sessionExpiresLabel" class="text-xs text-gray-500 dark:text-dark-400">
              {{ t('keyUsage.session.expiresAt', { time: sessionExpiresLabel }) }}
            </p>
          </div>
          <div class="flex shrink-0 flex-wrap items-center gap-2">
            <button
              data-testid="copy-share-link"
              @click="copyShareLink"
              class="inline-flex items-center gap-1.5 rounded-lg border border-gray-200 bg-white px-3 py-2 text-xs font-medium text-gray-700 transition-colors hover:border-primary-300 hover:text-primary-600 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-200 dark:hover:text-primary-300"
            >
              <Icon name="copy" size="xs" />
              {{ t('keyUsage.session.copyLink') }}
            </button>
            <button
              data-testid="exit-session"
              @click="clearSession()"
              class="inline-flex items-center gap-1.5 rounded-lg border border-gray-200 bg-white px-3 py-2 text-xs font-medium text-gray-700 transition-colors hover:border-rose-300 hover:text-rose-600 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-200 dark:hover:text-rose-400"
            >
              <Icon name="x" size="xs" />
              {{ t('keyUsage.session.exit') }}
            </button>
          </div>
        </div>
        <p class="mt-3 flex items-start gap-2 rounded-lg bg-amber-500/10 px-3 py-2 text-xs leading-relaxed text-amber-700 dark:text-amber-300">
          <Icon name="exclamationTriangle" size="xs" class="mt-0.5 shrink-0" />
          <span>{{ t('keyUsage.session.shareWarning') }}</span>
        </p>
      </div>

      <!-- Results Container -->
      <div v-if="showResults">
        <!-- Loading Skeleton -->
        <div v-if="showLoading" class="space-y-6">
          <div class="grid grid-cols-1 md:grid-cols-2 gap-6">
            <div class="rounded-2xl border border-gray-200 bg-white p-8 dark:border-dark-700 dark:bg-dark-900">
              <div class="skeleton h-5 w-24 mb-6"></div>
              <div class="flex justify-center"><div class="skeleton w-44 h-44 rounded-full"></div></div>
            </div>
            <div class="rounded-2xl border border-gray-200 bg-white p-8 dark:border-dark-700 dark:bg-dark-900">
              <div class="skeleton h-5 w-24 mb-6"></div>
              <div class="flex justify-center"><div class="skeleton w-44 h-44 rounded-full"></div></div>
            </div>
          </div>
          <div class="rounded-2xl border border-gray-200 bg-white p-8 dark:border-dark-700 dark:bg-dark-900">
            <div class="skeleton h-5 w-32 mb-6"></div>
            <div class="space-y-4">
              <div class="skeleton h-4 w-full"></div>
              <div class="skeleton h-4 w-3/4"></div>
              <div class="skeleton h-4 w-5/6"></div>
              <div class="skeleton h-4 w-2/3"></div>
            </div>
          </div>
          <!-- Filter bar + single-window summary skeleton -->
          <div class="rounded-2xl border border-gray-200 bg-white p-6 dark:border-dark-700 dark:bg-dark-900">
            <div class="skeleton h-4 w-28 mb-4"></div>
            <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              <div v-for="n in 3" :key="`filter-sk-${n}`" class="skeleton h-9 w-full"></div>
            </div>
          </div>
          <div class="rounded-2xl border border-gray-200 bg-white p-6 dark:border-dark-700 dark:bg-dark-900">
            <div class="skeleton h-4 w-20 mb-4"></div>
            <div class="grid gap-3 sm:grid-cols-3">
              <div class="skeleton h-3 w-full"></div>
              <div class="skeleton h-3 w-5/6"></div>
              <div class="skeleton h-3 w-2/3"></div>
            </div>
          </div>
          <!-- Podium skeleton -->
          <div
            class="rounded-2xl border border-gray-200 bg-white p-8 dark:border-dark-700 dark:bg-dark-900"
          >
            <div class="skeleton h-4 w-24 mb-6"></div>
            <div class="flex items-end justify-center gap-4">
              <div class="skeleton h-20 w-24 rounded-xl"></div>
              <div class="skeleton h-28 w-24 rounded-xl"></div>
              <div class="skeleton h-16 w-24 rounded-xl"></div>
            </div>
          </div>
        </div>

        <!-- Result Content -->
        <div v-else-if="report" class="space-y-6">
          <!-- Key Identity Chip -->
          <div v-if="keyInfo?.name" data-testid="key-info" class="fade-up flex flex-wrap items-center justify-center gap-2">
            <span class="inline-flex items-center gap-1.5 rounded-full border border-gray-200 bg-white/90 px-3 py-1.5 text-xs text-gray-600 shadow-sm dark:border-dark-700 dark:bg-dark-900/90 dark:text-dark-300">
              <Icon name="key" size="xs" />
              {{ t('keyUsage.keyInfo.name') }}: <strong class="font-semibold text-gray-900 dark:text-white">{{ keyInfo.name }}</strong>
            </span>
            <span v-if="keyInfo.created_at" class="inline-flex items-center gap-1.5 rounded-full border border-gray-200 bg-white/90 px-3 py-1.5 text-xs text-gray-600 shadow-sm dark:border-dark-700 dark:bg-dark-900/90 dark:text-dark-300">
              {{ t('keyUsage.keyInfo.createdAt') }}: {{ formatDate(keyInfo.created_at) }}
            </span>
            <span v-if="keyInfo.status" class="inline-flex items-center gap-1.5 rounded-full border border-gray-200 bg-white/90 px-3 py-1.5 text-xs text-gray-600 shadow-sm dark:border-dark-700 dark:bg-dark-900/90 dark:text-dark-300">
              {{ t('keyUsage.keyInfo.status') }}: <strong class="font-semibold text-gray-900 dark:text-white">{{ keyInfo.status }}</strong>
            </span>
          </div>

          <!-- Usage payload failed to build: say so instead of silently rendering "no data" -->
          <div
            v-if="usageUnavailable"
            data-testid="usage-unavailable"
            class="fade-up mx-auto flex max-w-2xl items-center justify-center gap-2 rounded-xl border border-amber-300 bg-amber-50/80 px-4 py-3 text-center text-xs text-amber-800 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-200"
          >
            {{ t('keyUsage.usageUnavailable') }}
          </div>

          <!-- Status Badge -->
          <div v-if="statusInfo" class="fade-up flex items-center justify-center mb-2">
            <div class="inline-flex items-center gap-2 px-5 py-2.5 rounded-full border border-gray-200 bg-white/90 shadow-sm backdrop-blur-sm dark:border-dark-700 dark:bg-dark-900/90">
              <span
                class="w-2.5 h-2.5 rounded-full pulse-dot"
                :class="statusInfo.isActive ? 'bg-emerald-500' : 'bg-rose-500'"
              ></span>
              <span class="text-sm font-medium text-gray-900 dark:text-white">{{ statusInfo.label }}</span>
              <span class="text-xs text-gray-400 dark:text-dark-500">|</span>
              <span class="text-xs text-gray-500 dark:text-dark-400">{{ statusInfo.statusText }}</span>
            </div>
          </div>

          <!-- Ring Cards Grid -->
          <div v-if="ringItems.length > 0" :class="ringGridClass">
            <div
              v-for="(ring, i) in ringItems"
              :key="i"
              class="fade-up rounded-2xl border border-gray-200 bg-white/90 p-8 backdrop-blur-sm transition-all duration-300 hover:shadow-lg dark:border-dark-700 dark:bg-dark-900/90"
              :class="`fade-up-delay-${Math.min(i + 1, 4)}`"
            >
              <div class="flex items-center justify-between mb-6">
                <h3 class="text-sm font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">
                  {{ ring.title }}
                </h3>
                <!-- Clock icon -->
                <svg v-if="ring.iconType === 'clock'" class="w-5 h-5 text-gray-400 dark:text-dark-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/>
                </svg>
                <!-- Calendar icon -->
                <svg v-else-if="ring.iconType === 'calendar'" class="w-5 h-5 text-gray-400 dark:text-dark-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <rect x="3" y="4" width="18" height="18" rx="2" ry="2"/><line x1="16" y1="2" x2="16" y2="6"/><line x1="8" y1="2" x2="8" y2="6"/><line x1="3" y1="10" x2="21" y2="10"/>
                </svg>
                <!-- Dollar icon -->
                <svg v-else class="w-5 h-5 text-gray-400 dark:text-dark-500" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <line x1="12" y1="1" x2="12" y2="23"/><path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"/>
                </svg>
              </div>
              <div class="flex justify-center">
                <div class="relative">
                  <svg class="w-44 h-44" viewBox="0 0 160 160">
                    <circle cx="80" cy="80" r="68" fill="none" :stroke="ringTrackColor" stroke-width="10"/>
                    <circle
                      class="progress-ring"
                      cx="80" cy="80" r="68" fill="none"
                      :stroke="`url(#ring-grad-${i})`"
                      stroke-width="10" stroke-linecap="round"
                      :stroke-dasharray="CIRCUMFERENCE.toFixed(2)"
                      :stroke-dashoffset="getRingOffset(ring)"
                    />
                    <defs>
                      <linearGradient :id="`ring-grad-${i}`" x1="0%" y1="0%" x2="100%" y2="100%">
                        <stop offset="0%" :stop-color="RING_GRADIENTS[i % 4].from"/>
                        <stop offset="100%" :stop-color="RING_GRADIENTS[i % 4].to"/>
                      </linearGradient>
                    </defs>
                  </svg>
                  <div class="absolute inset-0 flex flex-col items-center justify-center">
                    <template v-if="ring.isBalance">
                      <span class="text-2xl font-bold tabular-nums" :style="{ color: RING_GRADIENTS[i % 4].from }">
                        {{ ring.amount }}
                      </span>
                    </template>
                    <template v-else>
                      <span class="text-3xl font-bold tabular-nums text-gray-900 dark:text-white">
                        {{ displayPcts[i] ?? 0 }}%
                      </span>
                      <span class="text-xs text-gray-500 dark:text-dark-400 mt-0.5">{{ t('keyUsage.used') }}</span>
                      <span
                        class="text-sm font-semibold mt-1 tabular-nums"
                        :style="{ color: RING_GRADIENTS[i % 4].from }"
                      >{{ ring.amount }}</span>
                      <p v-if="ring.resetAt && formatResetTime(ring.resetAt)" class="text-xs text-gray-400 dark:text-gray-500 mt-0.5 tabular-nums">
                        ⟳ {{ formatResetTime(ring.resetAt) }}
                      </p>
                    </template>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Detail Card -->
          <div
            v-if="detailRows.length > 0"
            class="fade-up fade-up-delay-3 rounded-2xl border border-gray-200 bg-white/90 backdrop-blur-sm overflow-hidden dark:border-dark-700 dark:bg-dark-900/90"
          >
            <div class="px-8 py-5 border-b border-gray-200 dark:border-dark-700">
              <h3 class="text-sm font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.detailInfo') }}</h3>
            </div>
            <div class="divide-y divide-gray-100 dark:divide-dark-800">
              <div
                v-for="(row, i) in detailRows"
                :key="i"
                class="px-8 py-4 flex items-center justify-between"
              >
                <div class="flex items-center gap-3">
                  <div class="w-8 h-8 rounded-lg flex items-center justify-center" :class="row.iconBg">
                    <svg
                      class="w-4 h-4"
                      :class="row.iconColor"
                      viewBox="0 0 24 24" fill="none" stroke="currentColor"
                      stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
                      v-html="row.iconSvg"
                    ></svg>
                  </div>
                  <span class="text-sm text-gray-700 dark:text-dark-200">{{ row.label }}</span>
                </div>
                <span class="text-sm font-semibold tabular-nums" :class="row.valueClass || 'text-gray-900 dark:text-white'">
                  {{ row.value }}
                </span>
              </div>
            </div>
          </div>

          <!-- Usage Stats Card -->
          <div
            v-if="usageStatCells.length > 0"
            class="fade-up fade-up-delay-3 rounded-2xl border border-gray-200 bg-white/90 backdrop-blur-sm overflow-hidden dark:border-dark-700 dark:bg-dark-900/90"
          >
            <div class="px-8 py-5 border-b border-gray-200 dark:border-dark-700">
              <h3 class="text-sm font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.tokenStats') }}</h3>
            </div>
            <div class="grid grid-cols-2 md:grid-cols-4 gap-px bg-gray-100 dark:bg-dark-800">
              <div
                v-for="(cell, i) in usageStatCells"
                :key="i"
                class="bg-white px-6 py-4 dark:bg-dark-900"
              >
                <div class="text-xs text-gray-500 dark:text-dark-400 mb-1">{{ cell.label }}</div>
                <div class="text-sm font-semibold tabular-nums text-gray-900 dark:text-white" :title="cell.title">{{ cell.value }}</div>
              </div>
            </div>
          </div>

          <!-- Daily Usage Table -->
          <div
            v-if="showDailyUsage"
            class="fade-up fade-up-delay-4 rounded-2xl border border-gray-200 bg-white/90 backdrop-blur-sm overflow-hidden dark:border-dark-700 dark:bg-dark-900/90"
          >
            <div class="flex flex-col gap-3 px-8 py-5 border-b border-gray-200 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
              <h3 class="text-sm font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.dailyDetail') }}</h3>
              <div class="inline-flex rounded-lg border border-gray-200 bg-white p-0.5 dark:border-dark-700 dark:bg-dark-950">
                <button
                  v-for="option in dailyUsageOptions"
                  :key="option.value"
                  @click="setDailyUsageDays(option.value)"
                  class="min-w-12 rounded-md px-3 py-1.5 text-xs font-medium transition-colors"
                  :class="dailyUsageDays === option.value
                    ? 'bg-primary-500 text-white'
                    : 'text-gray-600 hover:bg-gray-100 dark:text-dark-300 dark:hover:bg-dark-800'"
                >
                  {{ option.label }}
                </button>
              </div>
            </div>
            <div v-if="dailyUsageRows.length > 0" class="overflow-x-auto">
              <table class="w-full">
                <thead>
                  <tr class="border-b border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-950">
                    <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.date') }}</th>
                    <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.requests') }}</th>
                    <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.inputTokens') }}</th>
                    <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.outputTokens') }}</th>
                    <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.cacheReadTokens') }}</th>
                    <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.cacheWriteTokens') }}</th>
                    <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.cost') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="row in dailyUsageRows"
                    :key="row.date"
                    class="border-b border-gray-100 last:border-b-0 dark:border-dark-800"
                  >
                    <td class="px-4 py-3 text-sm font-medium whitespace-nowrap text-gray-900 dark:text-white">{{ row.date }}</td>
                    <td class="px-4 py-3 text-sm tabular-nums text-right text-gray-700 dark:text-dark-200" :title="fmtNumExact(row.requests)">{{ fmtNum(row.requests) }}</td>
                    <td class="px-4 py-3 text-sm tabular-nums text-right text-gray-700 dark:text-dark-200" :title="fmtNumExact(row.input_tokens)">{{ fmtNum(row.input_tokens) }}</td>
                    <td class="px-4 py-3 text-sm tabular-nums text-right text-gray-700 dark:text-dark-200" :title="fmtNumExact(row.output_tokens)">{{ fmtNum(row.output_tokens) }}</td>
                    <td class="px-4 py-3 text-sm tabular-nums text-right text-gray-700 dark:text-dark-200" :title="fmtNumExact(row.cache_read_tokens)">{{ fmtNum(row.cache_read_tokens) }}</td>
                    <td class="px-4 py-3 text-sm tabular-nums text-right text-gray-700 dark:text-dark-200" :title="fmtNumExact(row.cache_write_tokens)">{{ fmtNum(row.cache_write_tokens) }}</td>
                    <td class="px-4 py-3 text-sm tabular-nums text-right font-medium text-gray-900 dark:text-white">{{ usd(row.actual_cost != null ? row.actual_cost : row.cost) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div v-else class="px-8 py-8 text-center text-sm text-gray-500 dark:text-dark-400">
              {{ t('keyUsage.noDailyUsage') }}
            </div>
          </div>

          <!-- Model Stats Table -->
          <div
            v-if="modelStats.length > 0"
            class="fade-up fade-up-delay-4 rounded-2xl border border-gray-200 bg-white/90 backdrop-blur-sm overflow-hidden dark:border-dark-700 dark:bg-dark-900/90"
          >
            <div class="px-8 py-5 border-b border-gray-200 dark:border-dark-700">
              <h3 class="text-sm font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.modelStats') }}</h3>
            </div>
            <div class="overflow-x-auto">
              <table class="w-full">
                <thead>
                  <tr class="border-b border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-950">
                    <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.model') }}</th>
                    <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.requests') }}</th>
                    <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.inputTokens') }}</th>
                    <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.outputTokens') }}</th>
                    <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.cacheCreationTokens') }}</th>
                    <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.cacheReadTokens') }}</th>
                    <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.totalTokens') }}</th>
                    <th class="px-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.cost') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr
                    v-for="(m, i) in modelStats"
                    :key="i"
                    class="border-b border-gray-100 last:border-b-0 dark:border-dark-800"
                  >
                    <td class="px-4 py-3 text-sm font-medium whitespace-nowrap text-gray-900 dark:text-white">{{ m.model || '-' }}</td>
                    <td class="px-4 py-3 text-sm tabular-nums text-right text-gray-700 dark:text-dark-200" :title="fmtNumExact(m.requests)">{{ fmtNum(m.requests) }}</td>
                    <td class="px-4 py-3 text-sm tabular-nums text-right text-gray-700 dark:text-dark-200" :title="fmtNumExact(m.input_tokens)">{{ fmtNum(m.input_tokens) }}</td>
                    <td class="px-4 py-3 text-sm tabular-nums text-right text-gray-700 dark:text-dark-200" :title="fmtNumExact(m.output_tokens)">{{ fmtNum(m.output_tokens) }}</td>
                    <td class="px-4 py-3 text-sm tabular-nums text-right text-gray-700 dark:text-dark-200" :title="fmtNumExact(m.cache_creation_tokens)">{{ fmtNum(m.cache_creation_tokens) }}</td>
                    <td class="px-4 py-3 text-sm tabular-nums text-right text-gray-700 dark:text-dark-200" :title="fmtNumExact(m.cache_read_tokens)">{{ fmtNum(m.cache_read_tokens) }}</td>
                    <td class="px-4 py-3 text-sm tabular-nums text-right text-gray-700 dark:text-dark-200" :title="fmtNumExact(m.total_tokens)">{{ fmtNum(m.total_tokens) }}</td>
                    <td class="px-4 py-3 text-sm tabular-nums text-right font-medium text-gray-900 dark:text-white">{{ usd(m.actual_cost != null ? m.actual_cost : m.cost) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <!-- ==================== Usage Explorer (window x scope x metric) ==================== -->
          <div v-if="showExplorer" data-testid="usage-explorer" class="fade-up fade-up-delay-3 space-y-4">
            <!--
              One panel, three selectors. Time window / ranking scope / sort metric are three
              composable dimensions of a single view, so they are grouped and labelled together
              instead of being scattered down the page. Each group wraps on its own, so three
              rows of tabs stack cleanly on a narrow screen rather than overflowing.
            -->
            <div
              data-testid="usage-filters"
              class="rounded-2xl border border-gray-200 bg-white/90 px-5 py-5 backdrop-blur-sm dark:border-dark-700 dark:bg-dark-900/90 sm:px-8"
            >
              <div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
                <h3 class="flex items-center gap-2 text-sm font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">
                  <Icon name="trophy" size="sm" />
                  {{ t('keyUsage.explorer.title') }}
                </h3>
                <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('keyUsage.explorer.hint') }}</p>
              </div>

              <div class="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
                <!-- Time window: drives the summary, the model breakdown AND the ranking -->
                <div class="min-w-0">
                  <p class="mb-1.5 text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('keyUsage.explorer.window') }}</p>
                  <div class="flex flex-wrap gap-1 rounded-lg border border-gray-200 bg-white p-1 dark:border-dark-700 dark:bg-dark-950" role="group">
                    <button
                      v-for="option in windowOptions"
                      :key="option.value"
                      data-testid="window-tab"
                      :data-window="option.value"
                      :aria-pressed="activeWindow === option.value"
                      :disabled="isRefreshing"
                      :class="tabClass(activeWindow === option.value)"
                      @click="setActiveWindow(option.value)"
                    >{{ option.label }}</button>
                  </div>
                </div>

                <!-- Ranking scope: a pure client-side pivot, both scopes are already loaded -->
                <div class="min-w-0">
                  <p class="mb-1.5 text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('keyUsage.rankings.scope') }}</p>
                  <div class="flex flex-wrap gap-1 rounded-lg border border-gray-200 bg-white p-1 dark:border-dark-700 dark:bg-dark-950" role="group">
                    <button
                      v-for="option in rankingScopeOptions"
                      :key="option.value"
                      data-testid="scope-tab"
                      :data-scope="option.value"
                      :aria-pressed="rankingScope === option.value"
                      :class="tabClass(rankingScope === option.value)"
                      @click="setRankingScope(option.value)"
                    >{{ option.label }}</button>
                  </div>
                </div>

                <!-- Sort metric: the backend does the sorting, so this is a refetch -->
                <div class="min-w-0">
                  <p class="mb-1.5 text-xs font-medium text-gray-500 dark:text-dark-400">{{ t('keyUsage.rankings.metric') }}</p>
                  <div class="flex flex-wrap gap-1 rounded-lg border border-gray-200 bg-white p-1 dark:border-dark-700 dark:bg-dark-950" role="group">
                    <button
                      v-for="option in metricOptions"
                      :key="option.value"
                      data-testid="metric-tab"
                      :data-metric="option.value"
                      :aria-pressed="metric === option.value"
                      :disabled="isRefreshing"
                      :class="tabClass(metric === option.value)"
                      @click="setMetric(option.value)"
                    >{{ option.label }}</button>
                  </div>
                </div>
              </div>

              <p class="mt-3 text-xs text-gray-500 dark:text-dark-400">{{ rankingScopeHint }}</p>
            </div>

            <!--
              Everything below is driven by the three selectors above. It stays mounted across
              a switch and only dims: remounting a skeleton here makes every tab click flash
              the whole page, which reads as a full reload rather than a filter change.
            -->
            <div
              class="relative space-y-4 transition-opacity duration-200"
              data-testid="explorer-body"
              :class="isRefreshing ? 'opacity-50' : 'opacity-100'"
            >
              <div v-if="isRefreshing" data-testid="rankings-refreshing" class="pointer-events-none absolute inset-x-0 -top-1 z-10 flex justify-center">
                <span class="rounded-full bg-white px-3 py-1 text-xs text-gray-600 shadow-md dark:bg-dark-800 dark:text-dark-200">
                  {{ t('keyUsage.rankings.refreshing') }}
                </span>
              </div>

              <!-- Stats for the selected window -->
              <div v-if="hasWindowStats" data-testid="windows-section" class="space-y-4">
                <div
                  data-testid="window-summary"
                  :data-window="activeWindow"
                  class="rounded-2xl border border-gray-200 bg-white/90 p-5 backdrop-blur-sm dark:border-dark-700 dark:bg-dark-900/90 sm:px-8"
                >
                  <h4 class="text-xs font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ activeWindowLabel }}</h4>
                  <dl class="mt-4 grid gap-3 sm:grid-cols-3">
                    <div class="flex items-baseline justify-between gap-3 sm:flex-col sm:items-start sm:gap-1">
                      <dt class="text-xs text-gray-500 dark:text-dark-400">{{ t('keyUsage.requests') }}</dt>
                      <dd
                        class="text-base font-semibold tabular-nums text-gray-900 dark:text-white"
                        :title="fmtNumExact(activeWindowStat?.requests ?? 0)"
                      >{{ fmtNum(activeWindowStat?.requests ?? 0) }}</dd>
                    </div>
                    <div class="flex items-baseline justify-between gap-3 sm:flex-col sm:items-start sm:gap-1">
                      <dt class="text-xs text-gray-500 dark:text-dark-400">{{ t('keyUsage.totalTokens') }}</dt>
                      <dd
                        class="text-base font-semibold tabular-nums text-gray-900 dark:text-white"
                        :title="fmtNumExact(activeWindowStat?.tokens ?? 0)"
                      >{{ fmtNum(activeWindowStat?.tokens ?? 0) }}</dd>
                    </div>
                    <div class="flex items-baseline justify-between gap-3 sm:flex-col sm:items-start sm:gap-1">
                      <dt class="text-xs text-gray-500 dark:text-dark-400">{{ t('keyUsage.cost') }}</dt>
                      <dd class="text-base font-semibold tabular-nums text-gray-900 dark:text-white">{{ usd(activeWindowStat?.cost_usd ?? 0) }}</dd>
                    </div>
                  </dl>
                  <p
                    v-if="activeWindowEmpty"
                    data-testid="window-summary-empty"
                    class="mt-3 rounded-lg bg-gray-50 px-3 py-2 text-xs text-gray-500 dark:bg-dark-800/60 dark:text-dark-400"
                  >{{ t('keyUsage.windows.empty') }}</p>
                </div>

                <div class="overflow-hidden rounded-2xl border border-gray-200 bg-white/90 backdrop-blur-sm dark:border-dark-700 dark:bg-dark-900/90">
                  <div class="border-b border-gray-200 px-5 py-5 dark:border-dark-700 sm:px-8">
                    <h3 class="text-sm font-semibold uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('keyUsage.windows.modelsTitle') }}</h3>
                  </div>
                  <WindowModelTable :models="activeWindowModels" />
                </div>
              </div>

              <!-- Ranking for the selected window + scope -->
              <div v-if="hasRankings" data-testid="rankings-section">
                <RankingWindowCard
                  :key="`${rankingScope}-${activeWindow}`"
                  :title="rankingCardTitle"
                  :window-key="activeWindow"
                  :data="activeRanking"
                  :metric="metric"
                />
              </div>
            </div>
          </div>
        </div>
      </div>
    </main>

    <!-- Footer (same pattern as HomeView) -->
    <footer class="relative z-10 border-t border-gray-200/50 px-6 py-8 dark:border-dark-800/50">
      <div class="mx-auto flex max-w-6xl flex-col items-center justify-center gap-4 text-center sm:flex-row sm:text-left">
        <p class="text-sm text-gray-500 dark:text-dark-400">
          &copy; {{ currentYear }} {{ SITE_NAME }}. {{ t('home.footer.allRightsReserved') }}
        </p>
        <div class="flex items-center gap-4">
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-dark-400 dark:hover:text-white"
          >{{ t('home.docs') }}</a>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter, type LocationQueryRaw } from 'vue-router'
import { useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import RankingWindowCard from '@/components/keyUsage/RankingWindowCard.vue'
import WindowModelTable from '@/components/keyUsage/WindowModelTable.vue'
import {
  createKeyUsageSession,
  DEFAULT_KEY_USAGE_METRIC,
  DEFAULT_KEY_USAGE_SCOPE,
  DEFAULT_KEY_USAGE_WINDOW,
  fetchKeyUsageReport,
  isEndpointMissing,
  isUnauthorized,
  isValidMetric,
  isValidScope,
  isValidWindow,
  KEY_USAGE_WINDOWS,
  type KeyUsageMetric,
  type KeyUsageRankingScope,
  type KeyUsageReport,
  type KeyUsageWindowKey,
  type KeyUsageWindowStat,
} from '@/api/keyUsage'
import { formatDateLocalInput } from '@/utils/format'
import { formatCount, formatCountExact, formatUsd } from '@/utils/keyUsageFormat'
import { sanitizeUrl } from '@/utils/url'
import { SITE_NAME } from '@/constants/site'

const { t, locale } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()

// ==================== Site Settings (same as HomeView) ====================

const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))

// ==================== Theme (same as HomeView) ====================

const isDark = ref(document.documentElement.classList.contains('dark'))

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

const currentYear = computed(() => new Date().getFullYear())

// ==================== Key Query State ====================

const apiKey = ref('')
const keyVisible = ref(false)
const isQuerying = ref(false)
const showResults = ref(false)
const showLoading = ref(false)
const showDatePicker = ref(false)
const report = ref<KeyUsageReport | null>(null)
/**
 * The `/v1/usage` payload the backend embeds under `report.usage`. It is built by the same
 * backend code path as the gateway endpoint，字段口径完全一致。`null` when the backend failed to
 * assemble it — see `usageUnavailable`.
 * Every legacy panel below (rings, detail rows, daily table, model table) keeps reading
 * from here, so their behaviour is unchanged.
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const resultData = computed<any>(() => report.value?.usage ?? null)

/**
 * The backend sets `usage_available: false` (and `usage: null`) when it could not assemble
 * the `/v1/usage` payload. Without this the page would silently render "no data", which is
 * indistinguishable from a Key that genuinely has never been used.
 */
const usageUnavailable = computed(() => Boolean(report.value) && report.value?.usage_available === false)
const now = ref(new Date())
let resetTimer: ReturnType<typeof setInterval> | null = null

// ==================== Lookup Session (URL token) ====================

/** Query-string parameter that carries the non-reversible lookup token. */
const TOKEN_QUERY_KEY = 't'
/** The three view selectors, mirrored into the URL alongside `?t=` so a view is shareable. */
const WINDOW_QUERY_KEY = 'w'
const SCOPE_QUERY_KEY = 's'
const METRIC_QUERY_KEY = 'm'

const sessionToken = ref('')
const sessionExpiresAt = ref<string | null>(null)
/** In token mode the key input is hidden: the URL itself is the credential. */
const isTokenMode = computed(() => Boolean(sessionToken.value))

// ==================== Rankings / Windows State ====================

const metric = ref<KeyUsageMetric>(DEFAULT_KEY_USAGE_METRIC)
const rankingScope = ref<KeyUsageRankingScope>(DEFAULT_KEY_USAGE_SCOPE)
/**
 * The one time window the page is showing. It drives BOTH the request (the backend computes
 * only this window) and everything rendered from it — summary, model breakdown, ranking.
 */
const activeWindow = ref<KeyUsageWindowKey>(DEFAULT_KEY_USAGE_WINDOW)
/** Soft refresh (window / metric / date change): keeps the current data on screen, no skeleton. */
const isRefreshing = ref(false)

// ==================== Date Range State ====================

type DateRangeKey = 'today' | '7d' | '30d' | 'custom'
const currentRange = ref<DateRangeKey>('today')
const customStartDate = ref('')
const customEndDate = ref('')
const dailyUsageDays = ref<7 | 30 | 90>(30)

const dateRanges = computed(() => [
  { key: 'today' as const, label: t('keyUsage.dateRangeToday') },
  { key: '7d' as const, label: t('keyUsage.dateRange7d') },
  { key: '30d' as const, label: t('keyUsage.dateRange30d') },
  { key: 'custom' as const, label: t('keyUsage.dateRangeCustom') },
])

const dailyUsageOptions = computed(() => [
  { value: 7 as const, label: t('keyUsage.dateRange7d') },
  { value: 30 as const, label: t('keyUsage.dateRange30d') },
  { value: 90 as const, label: t('keyUsage.dateRange90d') },
])

function setDateRange(key: DateRangeKey) {
  currentRange.value = key
  if (key !== 'custom') {
    refreshReport()
  }
}

function getDateParams(): string {
  const now = new Date()
  const params = new URLSearchParams()

  if (currentRange.value === 'custom') {
    if (customStartDate.value && customEndDate.value) {
      params.set('start_date', customStartDate.value)
      params.set('end_date', customEndDate.value)
    }
  } else {
    const end = formatDateLocalInput(now)
    let start: string
    switch (currentRange.value) {
      case 'today': start = end; break
      case '7d': start = formatDateLocalInput(new Date(now.getTime() - 7 * 86400000)); break
      case '30d': start = formatDateLocalInput(new Date(now.getTime() - 30 * 86400000)); break
      default: start = formatDateLocalInput(new Date(now.getTime() - 30 * 86400000))
    }
    params.set('start_date', start)
    params.set('end_date', end)
  }
  params.set('days', String(dailyUsageDays.value))
  params.set('timezone', getBrowserTimezone())
  return params.toString()
}

function setDailyUsageDays(days: 7 | 30 | 90) {
  if (dailyUsageDays.value === days) return
  dailyUsageDays.value = days
  if (report.value) {
    refreshReport()
  }
}

// ==================== Ring Animation ====================

const CIRCUMFERENCE = 2 * Math.PI * 68
const RING_GRADIENTS = [
  { from: '#14b8a6', to: '#5eead4' },
  { from: '#6366F1', to: '#A5B4FC' },
  { from: '#10B981', to: '#6EE7B7' },
  { from: '#F59E0B', to: '#FCD34D' },
]

const ringAnimated = ref(false)
const displayPcts = ref<number[]>([])

const ringTrackColor = computed(() => isDark.value ? '#222222' : '#F0F0EE')

interface RingItem {
  title: string
  pct: number
  amount: string
  isBalance?: boolean
  iconType: 'clock' | 'calendar' | 'dollar'
  resetAt?: string | null
}

function getRingOffset(ring: RingItem): number {
  if (!ringAnimated.value) return CIRCUMFERENCE
  if (ring.isBalance) return 0
  return CIRCUMFERENCE - (Math.min(ring.pct, 100) / 100) * CIRCUMFERENCE
}

function triggerRingAnimation(items: RingItem[]) {
  ringAnimated.value = false
  displayPcts.value = items.map(() => 0)

  nextTick(() => {
    requestAnimationFrame(() => {
      setTimeout(() => {
        ringAnimated.value = true

        // Animate percentage numbers
        const duration = 1000
        const startTime = performance.now()
        const targets = items.map(item => item.isBalance ? 0 : item.pct)

        function tick() {
          const elapsed = performance.now() - startTime
          const p = Math.min(elapsed / duration, 1)
          const ease = 1 - Math.pow(1 - p, 3)
          displayPcts.value = targets.map(target => Math.round(ease * target))
          if (p < 1) requestAnimationFrame(tick)
        }
        requestAnimationFrame(tick)
      }, 50)
    })
  })
}

// ==================== Computed Data ====================

const statusInfo = computed(() => {
  const data = resultData.value
  if (!data) return null

  if (data.mode === 'quota_limited') {
    const isValid = data.isValid !== false
    const statusMap: Record<string, string> = {
      active: 'Active',
      quota_exhausted: 'Quota Exhausted',
      expired: 'Expired',
    }
    return {
      label: t('keyUsage.quotaMode'),
      statusText: statusMap[data.status] || data.status || 'Unknown',
      isActive: isValid && data.status === 'active',
    }
  }

  return {
    label: data.planName || t('keyUsage.walletBalance'),
    statusText: 'Active',
    isActive: true,
  }
})

const ringItems = computed<RingItem[]>(() => {
  const data = resultData.value
  if (!data) return []

  const items: RingItem[] = []

  if (data.mode === 'quota_limited') {
    if (data.quota) {
      const pct = data.quota.limit > 0 ? Math.min(Math.round((data.quota.used / data.quota.limit) * 100), 100) : 0
      items.push({ title: t('keyUsage.totalQuota'), pct, amount: `${usd(data.quota.used)} / ${usd(data.quota.limit)}`, iconType: 'dollar' })
    }
    if (data.rate_limits) {
      const windowLabels: Record<string, string> = { '5h': t('keyUsage.limit5h'), '1d': t('keyUsage.limitDaily'), '7d': t('keyUsage.limit7d') }
      const windowIcons: Record<string, 'clock' | 'calendar'> = { '5h': 'clock', '1d': 'calendar', '7d': 'calendar' }
      for (const rl of data.rate_limits) {
        const pct = rl.limit > 0 ? Math.min(Math.round((rl.used / rl.limit) * 100), 100) : 0
        items.push({
          title: windowLabels[rl.window] || rl.window,
          pct,
          amount: `${usd(rl.used)} / ${usd(rl.limit)}`,
          iconType: windowIcons[rl.window] || 'clock',
          resetAt: rl.reset_at,
        })
      }
    }
  } else {
  }

  return items
})

const ringGridClass = computed(() => {
  const len = ringItems.value.length
  if (len === 1) return 'grid grid-cols-1 max-w-md mx-auto gap-6'
  if (len === 2) return 'grid grid-cols-1 md:grid-cols-2 gap-6'
  return 'grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6'
})

interface DetailRow {
  iconBg: string
  iconColor: string
  iconSvg: string
  label: string
  value: string
  valueClass: string
}

function getUsageColor(pct: number): string {
  if (pct > 90) return 'text-rose-500'
  if (pct > 70) return 'text-amber-500'
  return 'text-emerald-500'
}

const detailRows = computed<DetailRow[]>(() => {
  const data = resultData.value
  if (!data) return []

  const rows: DetailRow[] = []
  const ICON_SHIELD = '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/>'
  const ICON_CALENDAR = '<rect x="3" y="4" width="18" height="18" rx="2" ry="2"/><line x1="16" y1="2" x2="16" y2="6"/><line x1="8" y1="2" x2="8" y2="6"/><line x1="3" y1="10" x2="21" y2="10"/>'
  const ICON_DOLLAR = '<line x1="12" y1="1" x2="12" y2="23"/><path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"/>'
  const ICON_CHECK = '<polyline points="20 6 9 17 4 12"/>'

  if (data.mode === 'quota_limited') {
    if (data.quota) {
      const remainColor = data.quota.remaining <= 0 ? 'text-rose-500'
        : data.quota.remaining < data.quota.limit * 0.1 ? 'text-amber-500'
        : 'text-emerald-500'
      rows.push({
        iconBg: 'bg-emerald-500/10', iconColor: 'text-emerald-500', iconSvg: ICON_SHIELD,
        label: t('keyUsage.remainingQuota'), value: usd(data.quota.remaining), valueClass: remainColor,
      })
    }
    if (data.expires_at) {
      const daysLeft = data.days_until_expiry
      let expiryStr = formatDate(data.expires_at)
      if (daysLeft != null) {
        expiryStr += daysLeft > 0 ? ` ${t('keyUsage.daysLeft', { days: daysLeft })}` : daysLeft === 0 ? ` ${t('keyUsage.todayExpires')}` : ''
      }
      rows.push({
        iconBg: 'bg-amber-500/10', iconColor: 'text-amber-500', iconSvg: ICON_CALENDAR,
        label: t('keyUsage.expiresAt'), value: expiryStr, valueClass: '',
      })
    }
    if (data.rate_limits) {
      const windowMap: Record<string, string> = { '5h': '5H', '1d': locale.value === 'zh' ? '日' : 'D', '7d': '7D' }
      for (const rl of data.rate_limits) {
        const pct = rl.limit > 0 ? (rl.used / rl.limit) * 100 : 0
        let valueStr = `${usd(rl.used)} / ${usd(rl.limit)}`
        const resetStr = formatResetTime(rl.reset_at)
        if (resetStr) {
          valueStr += ` (⟳ ${resetStr})`
        }
        rows.push({
          iconBg: 'bg-primary-500/10', iconColor: 'text-primary-500', iconSvg: ICON_DOLLAR,
          label: `${t('keyUsage.usedQuota')} (${windowMap[rl.window] || rl.window})`,
          value: valueStr,
          valueClass: getUsageColor(pct),
        })
      }
    }
  } else {
    rows.push({
      iconBg: 'bg-emerald-500/10', iconColor: 'text-emerald-500', iconSvg: ICON_CHECK,
      label: t('keyUsage.subscriptionType'), value: data.planName || t('keyUsage.walletBalance'), valueClass: '',
    })

    const remainColor = data.remaining != null
      ? (data.remaining <= 0 ? 'text-rose-500' : data.remaining < 10 ? 'text-amber-500' : 'text-emerald-500')
      : ''
    rows.push({
      iconBg: 'bg-emerald-500/10', iconColor: 'text-emerald-500', iconSvg: ICON_SHIELD,
      label: t('keyUsage.remainingQuota'), value: data.remaining != null ? usd(data.remaining) : '-', valueClass: remainColor,
    })
  }

  return rows
})

interface StatCell {
  label: string
  value: string
  /** Exact grouped number behind an abbreviated count; absent for costs and ratios. */
  title?: string
}

/** Count cell: abbreviated on screen (`12.3M`), exact value on hover. */
function countCell(label: string, value: number | null | undefined): StatCell {
  return { label, value: fmtNum(value), title: fmtNumExact(value) }
}

const usageStatCells = computed<StatCell[]>(() => {
  const usage = resultData.value?.usage
  if (!usage) return []

  const today = usage.today || {}
  const total = usage.total || {}

  return [
    countCell(t('keyUsage.todayRequests'), today.requests),
    countCell(t('keyUsage.todayInputTokens'), today.input_tokens),
    countCell(t('keyUsage.todayOutputTokens'), today.output_tokens),
    countCell(t('keyUsage.todayTokens'), today.total_tokens),
    countCell(t('keyUsage.todayCacheCreation'), today.cache_creation_tokens),
    countCell(t('keyUsage.todayCacheRead'), today.cache_read_tokens),
    { label: t('keyUsage.todayCost'), value: usd(today.actual_cost) },
    { label: t('keyUsage.rpmTpm'), value: `${usage.rpm || 0} / ${usage.tpm || 0}` },
    countCell(t('keyUsage.totalRequests'), total.requests),
    countCell(t('keyUsage.totalInputTokens'), total.input_tokens),
    countCell(t('keyUsage.totalOutputTokens'), total.output_tokens),
    countCell(t('keyUsage.totalTokensLabel'), total.total_tokens),
    countCell(t('keyUsage.totalCacheCreation'), total.cache_creation_tokens),
    countCell(t('keyUsage.totalCacheRead'), total.cache_read_tokens),
    { label: t('keyUsage.totalCost'), value: usd(total.actual_cost) },
    { label: t('keyUsage.avgDuration'), value: usage.average_duration_ms ? `${Math.round(usage.average_duration_ms)} ms` : '-' },
  ]
})

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const modelStats = computed<any[]>(() => resultData.value?.model_stats || [])

interface DailyUsageRow {
  date: string
  requests: number
  input_tokens: number
  output_tokens: number
  cache_read_tokens: number
  cache_write_tokens: number
  cost: number
  actual_cost?: number
}

const dailyUsageRows = computed<DailyUsageRow[]>(() => {
  const rows = resultData.value?.daily_usage
  return Array.isArray(rows) ? rows : []
})

const showDailyUsage = computed(() => Boolean(resultData.value && Array.isArray(resultData.value.daily_usage)))

// ==================== Windows & Rankings ====================

const keyInfo = computed(() => report.value?.key ?? null)

const WINDOW_LABEL_KEYS: Record<KeyUsageWindowKey, string> = {
  today: 'keyUsage.windows.today',
  last_7d: 'keyUsage.windows.last7d',
  last_30d: 'keyUsage.windows.last30d',
  all: 'keyUsage.windows.all',
}

function windowLabel(key: KeyUsageWindowKey): string {
  return t(WINDOW_LABEL_KEYS[key])
}

function isEmptyWindow(stat: KeyUsageWindowStat | null): boolean {
  if (!stat) return true
  return !(stat.requests > 0) && !(stat.tokens > 0) && !(stat.cost_usd > 0)
}

/** Shared look for all three selector groups, so they read as one control surface. */
function tabClass(active: boolean): string {
  const base = 'rounded-md px-3 py-1.5 text-xs font-medium transition-colors disabled:opacity-60'
  return active
    ? `${base} bg-primary-500 text-white`
    : `${base} text-gray-600 hover:bg-gray-100 dark:text-dark-300 dark:hover:bg-dark-800`
}

const windowOptions = computed(() =>
  KEY_USAGE_WINDOWS.map(value => ({ value, label: windowLabel(value) }))
)

const activeWindowLabel = computed(() => windowLabel(activeWindow.value))
const activeWindowStat = computed<KeyUsageWindowStat | null>(() => report.value?.window_stat ?? null)
const activeWindowEmpty = computed(() => isEmptyWindow(activeWindowStat.value))
const activeWindowModels = computed(() => activeWindowStat.value?.models ?? [])

const hasWindowStats = computed(() => Boolean(report.value?.window_stat))

const rankingScopeOptions = computed(() => [
  { value: 'account' as const, label: t('keyUsage.rankings.scopeAccount') },
  { value: 'site' as const, label: t('keyUsage.rankings.scopeSite') },
])

const metricOptions = computed(() => [
  { value: 'cost' as const, label: t('keyUsage.rankings.metricCost') },
  { value: 'tokens' as const, label: t('keyUsage.rankings.metricTokens') },
  { value: 'requests' as const, label: t('keyUsage.rankings.metricRequests') },
])

const rankingScopeHint = computed(() =>
  rankingScope.value === 'account' ? t('keyUsage.rankings.scopeAccountHint') : t('keyUsage.rankings.scopeSiteHint')
)

const rankingScopeLabel = computed(() =>
  rankingScope.value === 'account' ? t('keyUsage.rankings.scopeAccount') : t('keyUsage.rankings.scopeSite')
)

/** Both halves come from i18n; the separator is punctuation, not copy. */
const rankingCardTitle = computed(() => `${rankingScopeLabel.value} · ${activeWindowLabel.value}`)

/** Ranking for the selected scope, in the one window the backend computed. */
const activeRanking = computed(() => report.value?.rankings?.[rankingScope.value] ?? null)

const hasRankings = computed(() => Boolean(report.value?.rankings))

/** The filter bar is worth showing as soon as either block below it has something to filter. */
const showExplorer = computed(() => hasWindowStats.value || hasRankings.value)

function setRankingScope(scope: KeyUsageRankingScope) {
  if (rankingScope.value === scope) return
  rankingScope.value = scope
  // Pure client-side pivot: both scopes already came down with this window.
  syncViewToUrl()
}

/**
 * Switching the window is a refetch — the backend computes one window per request, so the
 * data for the newly selected one is not on the client yet.
 */
async function setActiveWindow(next: KeyUsageWindowKey) {
  if (activeWindow.value === next || isRefreshing.value || isQuerying.value) return
  const previous = activeWindow.value
  activeWindow.value = next
  isRefreshing.value = true
  try {
    applyReport(await requestReport())
    syncViewToUrl()
  } catch (err) {
    activeWindow.value = previous
    await handleReportError(err, { keepResults: true })
  } finally {
    isRefreshing.value = false
  }
}

// ==================== Utility Functions ====================

const usd = formatUsd
const fmtNum = formatCount
/** Exact grouped value behind an abbreviated count — used for `title` tooltips. */
const fmtNumExact = formatCountExact

function formatDate(iso: string | null | undefined): string {
  if (!iso) return '-'
  const d = new Date(iso)
  const loc = locale.value === 'zh' ? 'zh-CN' : 'en-US'
  return d.toLocaleDateString(loc, { year: 'numeric', month: 'long', day: 'numeric' })
}

function formatDateTimeShort(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const loc = locale.value === 'zh' ? 'zh-CN' : 'en-US'
  return d.toLocaleString(loc, { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

const sessionExpiresLabel = computed(() => formatDateTimeShort(sessionExpiresAt.value))

function getBrowserTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
  } catch {
    return 'UTC'
  }
}

// ==================== API Query ====================

function requestReport(credential?: { token?: string; key?: string }): Promise<KeyUsageReport> {
  const token = credential ? credential.token : sessionToken.value
  const key = credential ? credential.key : apiKey.value.trim()
  return fetchKeyUsageReport({
    token: token || undefined,
    key: token ? undefined : key || undefined,
    metric: metric.value,
    window: activeWindow.value,
    extraParams: new URLSearchParams(getDateParams()),
    fallbackMessage: t('keyUsage.queryFailed'),
  })
}

function applyReport(data: KeyUsageReport) {
  report.value = data
  // Trust the metric and window the backend actually computed: a value it did not recognise
  // silently falls back server-side, and labelling fallback data with our own selection would
  // put "All time" above a today-sized number.
  if (isValidMetric(data?.metric)) {
    metric.value = data.metric
  }
  if (isValidWindow(data?.window)) {
    activeWindow.value = data.window
  }
}

async function replaceUrlQuery(query: LocationQueryRaw) {
  try {
    await router.replace({ query })
  } catch {
    /* navigation duplicated / aborted - not actionable on a read-only page */
  }
}

/** Persist the lookup token in the URL. `replace`, never `push`: Back must not
 *  return the visitor to an empty input state. */
async function syncTokenToUrl(token: string) {
  const query = { ...(route.query || {}) }
  if (query[TOKEN_QUERY_KEY] === token) return
  query[TOKEN_QUERY_KEY] = token
  applyViewToQuery(query)
  await replaceUrlQuery(query)
}

async function removeTokenFromUrl() {
  const query = { ...(route.query || {}) }
  delete query[TOKEN_QUERY_KEY]
  delete query[WINDOW_QUERY_KEY]
  delete query[SCOPE_QUERY_KEY]
  delete query[METRIC_QUERY_KEY]
  await replaceUrlQuery(query)
}

/**
 * Write the three selectors into a query object, omitting defaults so a plain
 * `/key-usage?t=…` link stays clean.
 */
function applyViewToQuery(query: LocationQueryRaw) {
  const entries: [string, string, string][] = [
    [WINDOW_QUERY_KEY, activeWindow.value, DEFAULT_KEY_USAGE_WINDOW],
    [SCOPE_QUERY_KEY, rankingScope.value, DEFAULT_KEY_USAGE_SCOPE],
    [METRIC_QUERY_KEY, metric.value, DEFAULT_KEY_USAGE_METRIC],
  ]
  for (const [name, value, fallback] of entries) {
    if (value === fallback) {
      delete query[name]
    } else {
      query[name] = value
    }
  }
}

/**
 * Mirror the current window / scope / metric into the URL so a specific view
 * ("site-wide, all time, by tokens") can be bookmarked or shared.
 *
 * `replace`, never `push`: flipping a filter tab must not pile entries onto the
 * back stack — Back should leave the page, not walk back through tab clicks.
 */
function syncViewToUrl() {
  const query = { ...(route.query || {}) }
  applyViewToQuery(query)
  void replaceUrlQuery(query)
}

/** Restore the three selectors from the URL. Unknown values are ignored, not errors. */
function readViewFromUrl() {
  const single = (name: string): unknown => {
    const raw = route?.query?.[name]
    return Array.isArray(raw) ? raw[0] : raw
  }
  const window = single(WINDOW_QUERY_KEY)
  if (isValidWindow(window)) activeWindow.value = window
  const scope = single(SCOPE_QUERY_KEY)
  if (isValidScope(scope)) rankingScope.value = scope
  const wanted = single(METRIC_QUERY_KEY)
  if (isValidMetric(wanted)) metric.value = wanted
}

interface ReportErrorOptions {
  /** Keep the currently rendered report on screen (soft refresh failures). */
  keepResults?: boolean
}

async function handleReportError(err: unknown, options: ReportErrorOptions = {}) {
  showLoading.value = false
  if (isUnauthorized(err)) {
    // Expired / revoked lookup token: drop it and fall back to the input box.
    await clearSession({ silent: true })
    appStore.showError(t('keyUsage.session.expired'))
    return
  }
  if (!options.keepResults) {
    showResults.value = false
  }
  appStore.showError((err as Error)?.message || t('keyUsage.queryFailedRetry'))
}

/** Wipe the lookup session and go back to the bare input state. */
async function clearSession(options: { silent?: boolean } = {}) {
  sessionToken.value = ''
  sessionExpiresAt.value = null
  report.value = null
  activeWindow.value = DEFAULT_KEY_USAGE_WINDOW
  rankingScope.value = DEFAULT_KEY_USAGE_SCOPE
  metric.value = DEFAULT_KEY_USAGE_METRIC
  showResults.value = false
  showLoading.value = false
  showDatePicker.value = false
  apiKey.value = ''
  ringAnimated.value = false
  await removeTokenFromUrl()
  if (!options.silent) {
    appStore.showInfo(t('keyUsage.session.cleared'))
  }
}

async function queryKey() {
  if (isQuerying.value) return
  const key = apiKey.value.trim()
  if (!key) {
    appStore.showInfo(t('keyUsage.enterApiKey'))
    return
  }

  isQuerying.value = true
  showResults.value = true
  showLoading.value = true
  report.value = null

  try {
    let token = ''
    try {
      const session = await createKeyUsageSession(key, t('keyUsage.queryFailed'))
      token = session.token
      sessionExpiresAt.value = session.expires_at
    } catch (err) {
      // Session endpoint not deployed yet -> degrade to a direct bearer lookup
      // (works, but produces no shareable link).
      if (!isEndpointMissing(err)) throw err
      sessionExpiresAt.value = null
    }

    const data = await requestReport(token ? { token } : { key })
    applyReport(data)

    if (token) {
      sessionToken.value = token
      await syncTokenToUrl(token)
    }

    showLoading.value = false
    showDatePicker.value = true

    // Trigger ring animations after DOM update
    nextTick(() => {
      triggerRingAnimation(ringItems.value)
    })

    appStore.showSuccess(t('keyUsage.querySuccess'))
  } catch (err) {
    await handleReportError(err)
  } finally {
    isQuerying.value = false
  }
}

/** Load a report straight from a URL token (no input box involved). */
async function queryWithToken(token: string) {
  sessionToken.value = token
  isQuerying.value = true
  showResults.value = true
  showLoading.value = true
  report.value = null

  try {
    const data = await requestReport({ token })
    applyReport(data)
    showLoading.value = false
    showDatePicker.value = true
    nextTick(() => {
      triggerRingAnimation(ringItems.value)
    })
  } catch (err) {
    await handleReportError(err)
  } finally {
    isQuerying.value = false
  }
}

/** Soft reload that keeps the rendered data in place (no full-page skeleton). */
async function refreshReport() {
  if (isRefreshing.value || isQuerying.value) return
  if (!sessionToken.value && !apiKey.value.trim()) return

  isRefreshing.value = true
  try {
    applyReport(await requestReport())
  } catch (err) {
    await handleReportError(err, { keepResults: true })
  } finally {
    isRefreshing.value = false
  }
}

async function setMetric(next: KeyUsageMetric) {
  if (metric.value === next || isRefreshing.value || isQuerying.value) return
  const previous = metric.value
  metric.value = next
  isRefreshing.value = true
  try {
    applyReport(await requestReport())
    syncViewToUrl()
  } catch (err) {
    metric.value = previous
    await handleReportError(err, { keepResults: true })
  } finally {
    isRefreshing.value = false
  }
}

// ==================== Share Link ====================

const shareLink = computed(() => {
  if (!sessionToken.value || typeof window === 'undefined') return ''
  try {
    const url = new URL(window.location.href)
    url.searchParams.set(TOKEN_QUERY_KEY, sessionToken.value)
    return url.toString()
  } catch {
    return ''
  }
})

async function copyShareLink() {
  const link = shareLink.value
  if (!link) return
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(link)
    } else {
      const area = document.createElement('textarea')
      area.value = link
      area.style.position = 'fixed'
      area.style.opacity = '0'
      document.body.appendChild(area)
      area.select()
      document.execCommand('copy')
      document.body.removeChild(area)
    }
    appStore.showSuccess(t('keyUsage.session.copied'))
  } catch {
    appStore.showError(t('keyUsage.session.copyFailed'))
  }
}

// ==================== Lifecycle ====================

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (savedTheme === 'dark' || (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

function formatResetTime(resetAt: string | null | undefined): string {
  if (!resetAt) return ''
  const diff = new Date(resetAt).getTime() - now.value.getTime()
  if (diff <= 0) return t('keyUsage.resetNow')
  const days = Math.floor(diff / 86400000)
  const hours = Math.floor((diff % 86400000) / 3600000)
  const mins = Math.floor((diff % 3600000) / 60000)
  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${mins}m`
  return `${mins}m`
}

function readTokenFromUrl(): string {
  const raw = route?.query?.[TOKEN_QUERY_KEY]
  const value = Array.isArray(raw) ? raw[0] : raw
  return typeof value === 'string' ? value.trim() : ''
}

onMounted(() => {
  initTheme()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
  resetTimer = setInterval(() => { now.value = new Date() }, 60000)

  // Restore a shared view (window / scope / metric) before the first request goes out,
  // so the link opens on the view it was shared from instead of flashing the default.
  readViewFromUrl()

  // A `?t=` token means "already signed in": skip the input box entirely.
  const token = readTokenFromUrl()
  if (token) {
    queryWithToken(token)
  }
})

onUnmounted(() => {
  if (resetTimer) clearInterval(resetTimer)
})
</script>

<style scoped>
/* Input focus ring */
.input-ring {
  transition: box-shadow 0.2s ease, border-color 0.2s ease;
}
.input-ring:focus {
  box-shadow: 0 0 0 3px rgba(20, 184, 166, 0.2);
  border-color: #14b8a6;
  outline: none;
}

/* Ring animation */
.progress-ring {
  transition: stroke-dashoffset 1.2s cubic-bezier(0.4, 0, 0.2, 1);
  transform: rotate(-90deg);
  transform-origin: 50% 50%;
}

/* Skeleton loading */
@keyframes shimmer-kv {
  0%   { background-position: -200% 0; }
  100% { background-position: 200% 0; }
}
.skeleton {
  background: linear-gradient(90deg, #e5e7eb 25%, #f3f4f6 50%, #e5e7eb 75%);
  background-size: 200% 100%;
  animation: shimmer-kv 1.8s ease-in-out infinite;
  border-radius: 8px;
}
:global(.dark) .skeleton {
  background: linear-gradient(90deg, #334155 25%, #1e293b 50%, #334155 75%);
  background-size: 200% 100%;
}

/* Fade up animation */
@keyframes fade-up-kv {
  from { opacity: 0; transform: translateY(16px); }
  to { opacity: 1; transform: translateY(0); }
}
.fade-up {
  animation: fade-up-kv 0.5s cubic-bezier(0.4, 0, 0.2, 1) forwards;
}
.fade-up-delay-1 { animation-delay: 0.1s; opacity: 0; }
.fade-up-delay-2 { animation-delay: 0.2s; opacity: 0; }
.fade-up-delay-3 { animation-delay: 0.3s; opacity: 0; }
.fade-up-delay-4 { animation-delay: 0.4s; opacity: 0; }

/* Pulse dot */
@keyframes pulse-dot-kv {
  0%, 100% { opacity: 1; box-shadow: 0 0 0 0 currentColor; }
  50% { opacity: 0.6; box-shadow: 0 0 8px 2px currentColor; }
}
.pulse-dot {
  animation: pulse-dot-kv 2s ease-in-out infinite;
}

/* Tabular nums */
.tabular-nums {
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.02em;
}
</style>
