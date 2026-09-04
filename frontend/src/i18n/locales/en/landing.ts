export default {
  // Home Page
  home: {
    viewDocs: 'View Documentation',
    docs: 'Docs',
    switchToLight: 'Switch to Light Mode',
    switchToDark: 'Switch to Dark Mode',
    dashboard: 'Dashboard',
    login: 'Login',
    getStarted: 'Get Started',
    goToDashboard: 'Go to Dashboard',
    // User-focused value proposition
    heroSubtitle: 'One Key, All AI Models',
    tags: {
      subscriptionToApi: 'Subscription to API',
      stickySession: 'Session Persistence',
      realtimeBilling: 'Pay As You Go'
    },
    features: {
      unifiedGateway: 'One-Click Access',
      unifiedGatewayDesc: 'Get a single API key to call all connected AI models. No separate applications needed.',
      multiAccount: 'Always Reliable',
      multiAccountDesc: 'Smart routing across multiple upstream accounts with automatic failover. Say goodbye to errors.',
      balanceQuota: 'Pay What You Use',
      balanceQuotaDesc: 'Usage-based billing with quota limits. Full visibility into team consumption.'
    },
    providers: {
      title: 'Supported AI Models',
      description: 'One API, Multiple Choices',
      supported: 'Supported',
      soon: 'Soon',
      claude: 'Claude',
      gemini: 'Gemini',
      antigravity: 'Antigravity',
      more: 'More'
    },
    footer: {
      allRightsReserved: 'All rights reserved.'
    }
  },

  // Key Usage Query Page
  keyUsage: {
    title: 'API Key Usage',
    subtitle: 'Enter your API Key to view real-time spending and usage status',
    placeholder: 'sk-ant-mirror-xxxxxxxxxxxx',
    query: 'Query',
    querying: 'Querying...',
    privacyNote: 'Your Key is sent to this site’s server only to verify it and exchange it for a read-only lookup token; the exchange does not save your Key. The token carries an irreversible fingerprint rather than the Key itself, and it can only read usage — it cannot call the API.',
    dateRange: 'Date Range:',
    dateRangeToday: 'Today',
    dateRange7d: '7 Days',
    dateRange30d: '30 Days',
    dateRange90d: '90 Days',
    dateRangeCustom: 'Custom',
    apply: 'Apply',
    used: 'Used',
    usageUnavailable: 'Usage details could not be loaded right now. Windows and rankings below are still accurate.',
    detailInfo: 'Detail Information',
    tokenStats: 'Token Statistics',
    dailyDetail: 'Daily Detail',
    modelStats: 'Model Usage Statistics',
    // Table headers
    date: 'Date',
    model: 'Model',
    requests: 'Requests',
    inputTokens: 'Input Tokens',
    outputTokens: 'Output Tokens',
    cacheCreationTokens: 'Cache Creation',
    cacheReadTokens: 'Cache Read',
    cacheWriteTokens: 'Cache Write',
    totalTokens: 'Total Tokens',
    cost: 'Cost',
    // Status
    quotaMode: 'Key Quota Mode',
    unlimitedMode: 'Unlimited Mode',
    // Ring card titles
    limit5h: '5-Hour Limit',
    limitDaily: 'Daily Limit',
    limit7d: '7-Day Limit',
    limitWeekly: 'Weekly Limit',
    limitMonthly: 'Monthly Limit',
    // Detail rows
    expiresAt: 'Expires At',
    todayExpires: '(expires today)',
    daysLeft: '({days} days)',
    usedQuota: 'Used Quota',
    resetNow: 'Resetting soon',
    subscriptionExpires: 'Subscription Expires',
    // Usage stat cells
    todayRequests: 'Today Requests',
    todayInputTokens: 'Today Input',
    todayOutputTokens: 'Today Output',
    todayTokens: 'Today Tokens',
    todayCacheCreation: 'Today Cache Creation',
    todayCacheRead: 'Today Cache Read',
    todayCost: 'Today Cost',
    rpmTpm: 'RPM / TPM',
    totalRequests: 'Total Requests',
    totalInputTokens: 'Total Input',
    totalOutputTokens: 'Total Output',
    totalTokensLabel: 'Total Tokens',
    totalCacheCreation: 'Total Cache Creation',
    totalCacheRead: 'Total Cache Read',
    totalCost: 'Total Cost',
    avgDuration: 'Avg Duration',
    // Messages
    enterApiKey: 'Please enter an API Key',
    querySuccess: 'Query successful',
    queryFailed: 'Query failed',
    queryFailedRetry: 'Query failed, please try again later',
    noDailyUsage: 'No daily usage data',
    // Key identity
    keyInfo: {
      name: 'Key Name',
      createdAt: 'Created',
      status: 'Status',
    },
    // Shareable lookup link (URL token)
    session: {
      active: 'Signed in with a lookup link',
      activeFor: 'Lookup link for {name}',
      expiresAt: 'Link valid until {time}',
      copyLink: 'Copy lookup link',
      copied: 'Lookup link copied',
      copyFailed: 'Copy failed, please copy the address bar manually',
      shareWarning: 'Anyone holding this link can see this Key’s usage — it works like a read-only login. Do not share it publicly.',
      exit: 'Sign out',
      cleared: 'Lookup link cleared',
      expired: 'This lookup link is no longer valid, please enter your Key again',
    },
    // Per-window statistics
    windows: {
      title: 'Usage Windows',
      today: 'Today',
      last7d: 'Last 7 Days',
      last30d: 'Last 30 Days',
      all: 'All Time',
      modelsTitle: 'Model Breakdown',
      empty: 'No usage in this window',
      noModels: 'No model usage in this window',
    },
    // The three composable view selectors (time window x ranking scope x sort metric)
    explorer: {
      title: 'Usage & Rankings',
      window: 'Time range',
      hint: 'Combine a time range, a leaderboard and a sort metric',
    },
    // Rankings & podium
    rankings: {
      scope: 'Ranking scope',
      scopeAccount: 'Account',
      scopeSite: 'Site-wide',
      scopeAccountHint: 'Compared with every Key under the same account',
      scopeSiteHint: 'Compared with every Key on this site',
      metric: 'Sort by',
      metricCost: 'Cost (USD)',
      metricTokens: 'Total Tokens',
      metricRequests: 'Requests',
      rank: 'Rank',
      keyName: 'Key Name',
      you: 'You',
      selfRank: 'You rank #{rank} of {total} Keys',
      selfUnranked: 'You are not ranked in this window',
      empty: 'No ranking data in this window',
      unavailable: 'Rankings are temporarily unavailable, please try again later',
      podiumTied: '{medal} (tied)',
      refreshing: 'Updating rankings...',
      podiumGold: 'Champion',
      podiumSilver: 'Runner-up',
      podiumBronze: 'Third place',
    },
  },

  // Setup Wizard
  setup: {
    title: 'Sub2API Setup',
    description: 'Configure your Sub2API instance',
    database: {
      title: 'Database Configuration',
      description: 'Connect to your PostgreSQL database',
      host: 'Host',
      port: 'Port',
      username: 'Username',
      password: 'Password',
      databaseName: 'Database Name',
      sslMode: 'SSL Mode',
      passwordPlaceholder: 'Password',
      ssl: {
        disable: 'Disable',
        require: 'Require',
        verifyCa: 'Verify CA',
        verifyFull: 'Verify Full'
      }
    },
    redis: {
      title: 'Redis Configuration',
      description: 'Connect to your Redis server',
      host: 'Host',
      port: 'Port',
      username: 'Username (optional)',
      password: 'Password (optional)',
      database: 'Database',
      usernamePlaceholder: 'Leave empty for default user',
      passwordPlaceholder: 'Password',
      enableTls: 'Enable TLS',
      enableTlsHint: 'Use TLS when connecting to Redis (public CA certs)'
    },
    admin: {
      title: 'Admin Account',
      description: 'Create your administrator account',
      email: 'Email',
      password: 'Password',
      confirmPassword: 'Confirm Password',
      passwordPlaceholder: 'Min 8 characters',
      confirmPasswordPlaceholder: 'Confirm password',
      passwordMismatch: 'Passwords do not match'
    },
    ready: {
      title: 'Ready to Install',
      description: 'Review your configuration and complete setup',
      database: 'Database',
      redis: 'Redis',
      adminEmail: 'Admin Email'
    },
    status: {
      testing: 'Testing...',
      success: 'Connection Successful',
      testConnection: 'Test Connection',
      installing: 'Installing...',
      completeInstallation: 'Complete Installation',
      completed: 'Installation completed!',
      redirecting: 'Redirecting to login page...',
      restarting: 'Service is restarting, please wait...',
      timeout: 'Service restart is taking longer than expected. Please refresh the page manually.'
    }
  },

  // Common
}
