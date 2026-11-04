export default {
  upstreamErrors: {
    title: 'Upstream Errors',
    description: 'Upstream 502 / 503 errors, including recovered and ultimately failed requests, newest first.',
    time: 'Time', key: 'Triggering API Key', keyId: 'API Key ID', accountId: 'Failed Account ID',
    failedAccount: 'Failed Account', finalAccount: 'Final Successful Account', events: 'Upstream Events', outcome: 'Final Outcome',
    details: 'View Trace', range: 'Time Range', status: 'Upstream Status', hours: 'Last {n} hours',
    allOutcomes: 'All outcomes', recovered: 'Recovered', failed: 'Failed', notRecorded: 'Not recorded', noSuccess: 'No success',
    retentionHint: 'Uses Ops collection and retention settings. Only collected data appears; disabled monitoring, skipped logging, dropped queues or cleanup affect availability.',
    eventsHint: 'Failures, retries and account switches appear in collection order. One attempt can produce several events; event count is not a retry count.',
    eventCount: '{n} events', loadFailed: 'Loading failed. Check that Ops monitoring is enabled and retry.',
    empty: 'No collected 502 / 503 errors match these filters', requestId: 'Client Request ID',
    upstreamRequestId: 'Upstream Request ID', responseBody: 'Sanitized upstream response body'
  }
}
