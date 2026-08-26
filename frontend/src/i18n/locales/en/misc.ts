export default {

  // Subscription Progress (Header component)
  subscriptionProgress: {
    title: 'My Subscriptions',
    viewDetails: 'View subscription details',
    activeCount: '{count} active subscription(s)',
    daily: 'Daily',
    weekly: 'Weekly',
    monthly: 'Monthly',
    daysRemaining: '{days} days left',
    expired: 'Expired',
    expiresToday: 'Expires today',
    expiresTomorrow: 'Expires tomorrow',
    viewAll: 'View all subscriptions',
    noSubscriptions: 'No active subscriptions',
    unlimited: 'Unlimited'
  },

  // Custom Page (iframe embed)
  customPage: {
    title: 'Custom Page',
    openInNewTab: 'Open in new tab',
    notFoundTitle: 'Page not found',
    notFoundDesc: 'This custom page does not exist or has been removed.',
    notConfiguredTitle: 'Page URL not configured',
    notConfiguredDesc: 'The URL for this custom page has not been properly configured.',
    tableOfContents: 'Contents',
    copyCode: 'Copy',
    copiedCode: 'Copied',
    copyCodeFailed: 'Failed'
  },

  // Announcements Page
  announcements: {
    title: 'Announcements',
    description: 'View system announcements',
    unreadOnly: 'Show unread only',
    markRead: 'Mark as read',
    markAllRead: 'Mark all as read',
    viewAll: 'View all announcements',
    markedAsRead: 'Marked as read',
    allMarkedAsRead: 'All announcements marked as read',
    newCount: '{count} new announcement | {count} new announcements',
    readAt: 'Read at',
    read: 'Read',
    unread: 'Unread',
    startsAt: 'Starts at',
    endsAt: 'Ends at',
    empty: 'No announcements',
    emptyUnread: 'No unread announcements',
    total: 'announcements',
    emptyDescription: 'There are no system announcements at this time',
    readStatus: 'You have read this announcement',
    markReadHint: 'Click "Mark as read" to mark this announcement'
  },

  // User Subscriptions Page
  userSubscriptions: {
    rate: 'Rate',
    peakRate: 'Peak Rate',
    title: 'My Subscriptions',
    description: 'View your subscription plans and usage',
    noActiveSubscriptions: 'No Active Subscriptions',
    noActiveSubscriptionsDesc:
      "You don't have any active subscriptions. Contact administrator to get one.",
    failedToLoad: 'Failed to load subscriptions',
    status: {
      active: 'Active',
      expired: 'Expired',
      revoked: 'Revoked'
    },
    usage: 'Usage',
    expires: 'Expires',
    noExpiration: 'No expiration',
    unlimited: 'Unlimited',
    unlimitedDesc: 'No usage limits on this subscription',
    daily: 'Daily',
    weekly: 'Weekly',
    monthly: 'Monthly',
    daysRemaining: '{days} days remaining',
    expiresOn: 'Expires on {date}',
    resetIn: 'Resets in {time}',
    quotaEndsIn: 'Quota ends in {time}',
    windowNotActive: 'Awaiting first use',
    usageOf: '{used} of {limit}'
  }
}
