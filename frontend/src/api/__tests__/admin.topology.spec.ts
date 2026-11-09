import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({ get: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: { get } }))

import {
  SSEParser,
  backoffDelay,
  getTopologySnapshot,
  openTopologyEventStream,
  parseRoutingMessage,
  type RoutingEvent,
  type TopologyStreamStatus
} from '@/api/admin/topology'

describe('SSEParser', () => {
  it('parses a complete event', () => {
    const p = new SSEParser()
    expect(p.push('event: routing\ndata: {"a":1}\n\n')).toEqual([{ event: 'routing', data: '{"a":1}' }])
  })

  it('reassembles chunks split mid-line and mid-field', () => {
    const p = new SSEParser()
    expect(p.push('eve')).toEqual([])
    expect(p.push('nt: rout')).toEqual([])
    expect(p.push('ing\ndata: {"type":"req')).toEqual([])
    expect(p.push('uest_start"}\n')).toEqual([])
    expect(p.push('\n')).toEqual([{ event: 'routing', data: '{"type":"request_start"}' }])
  })

  it('ignores heartbeat comments and blank lines without data', () => {
    const p = new SSEParser()
    expect(p.push(': ping\n\n:keepalive\n\n')).toEqual([])
    expect(p.push('event: routing\n: mid-event comment\ndata: x\n\n')).toEqual([{ event: 'routing', data: 'x' }])
  })

  it('joins multi-line data with newlines', () => {
    const p = new SSEParser()
    expect(p.push('data: line1\ndata: line2\ndata:line3\n\n')).toEqual([
      { event: 'message', data: 'line1\nline2\nline3' }
    ])
  })

  it('handles CRLF, including a CR/LF pair split across chunks', () => {
    const p = new SSEParser()
    expect(p.push('event: routing\r')).toEqual([])
    expect(p.push('\ndata: 1\r\n\r')).toEqual([{ event: 'routing', data: '1' }])
    // the dangling \n must not produce an extra empty-line dispatch or swallow data
    expect(p.push('\ndata: 2\r\n\r\n')).toEqual([{ event: 'message', data: '2' }])
  })

  it('handles bare CR line endings', () => {
    const p = new SSEParser()
    expect(p.push('data: a\r\rdata: b\r\r')).toEqual([
      { event: 'message', data: 'a' },
      { event: 'message', data: 'b' }
    ])
  })

  it('emits several events from one chunk and keeps the event name per event', () => {
    const p = new SSEParser()
    const out = p.push('event: routing\ndata: 1\n\ndata: 2\n\nevent: routing\nid: 7\ndata: 3\n\n')
    expect(out).toEqual([
      { event: 'routing', data: '1' },
      { event: 'message', data: '2' },
      { event: 'routing', data: '3', id: '7' }
    ])
  })

  it('flushes a trailing unterminated event at stream end', () => {
    const p = new SSEParser()
    expect(p.push('event: routing\ndata: tail')).toEqual([])
    expect(p.flush()).toEqual([{ event: 'routing', data: 'tail' }])
  })
})

describe('parseRoutingMessage', () => {
  it('returns the event for routing messages', () => {
    const ev = parseRoutingMessage({ event: 'routing', data: '{"type":"attempt","request_id":"r1","account_id":3,"at":"x"}' })
    expect(ev).toMatchObject({ type: 'attempt', request_id: 'r1', account_id: 3 })
  })

  it('ignores other event names and malformed JSON', () => {
    expect(parseRoutingMessage({ event: 'message', data: '{"type":"attempt"}' })).toBeNull()
    expect(parseRoutingMessage({ event: 'routing', data: '{oops' })).toBeNull()
    expect(parseRoutingMessage({ event: 'routing', data: '42' })).toBeNull()
  })
})

describe('backoffDelay', () => {
  it('doubles from the minimum and caps at the maximum', () => {
    expect(backoffDelay(0, 1000, 30000)).toBe(1000)
    expect(backoffDelay(1, 1000, 30000)).toBe(2000)
    expect(backoffDelay(3, 1000, 30000)).toBe(8000)
    expect(backoffDelay(10, 1000, 30000)).toBe(30000)
  })
})

describe('getTopologySnapshot', () => {
  it('reads the snapshot endpoint through the shared api client', async () => {
    get.mockResolvedValueOnce({ data: { generated_at: 'now' } })
    await expect(getTopologySnapshot()).resolves.toEqual({ generated_at: 'now' })
    expect(get).toHaveBeenCalledWith('/admin/topology/snapshot', { signal: undefined })
  })
})

function streamResponse(chunks: string[]): Response {
  const enc = new TextEncoder()
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const c of chunks) controller.enqueue(enc.encode(c))
      controller.close()
    }
  })
  return new Response(body, { status: 200, headers: { 'Content-Type': 'text/event-stream' } })
}

describe('openTopologyEventStream', () => {
  beforeEach(() => {
    localStorage.setItem('auth_token', 'tok-123')
  })
  afterEach(() => {
    localStorage.removeItem('auth_token')
    vi.useRealTimers()
  })

  it('sends the bearer token, delivers routing events and reconnects after the stream ends', async () => {
    const events: RoutingEvent[] = []
    const statuses: TopologyStreamStatus[] = []
    const fetchImpl = vi
      .fn()
      .mockResolvedValueOnce(
        streamResponse([': hello\n\n', 'event: routing\ndata: {"type":"request_start","request_id":"a","api_key_id":1,"at":"t"}\n', '\n'])
      )
      .mockResolvedValueOnce(streamResponse(['event: routing\ndata: {"type":"request_done","request_id":"a","status_code":200,"at":"t"}\n\n']))
      .mockImplementation(() => new Promise(() => {}))

    const handle = openTopologyEventStream({
      onEvent: ev => events.push(ev),
      onStatus: s => statuses.push(s),
      fetchImpl: fetchImpl as unknown as typeof fetch,
      minBackoffMs: 1,
      maxBackoffMs: 2
    })

    await vi.waitFor(() => expect(events).toHaveLength(2))
    expect(events.map(e => e.type)).toEqual(['request_start', 'request_done'])
    const [url, init] = fetchImpl.mock.calls[0]
    expect(String(url)).toContain('/admin/topology/events')
    expect((init as RequestInit).headers).toMatchObject({ Authorization: 'Bearer tok-123' })
    expect(statuses).toContain('open')
    expect(statuses).toContain('reconnecting')

    handle.close()
    expect(statuses.at(-1)).toBe('closed')
  })

  it('backs off after HTTP errors and stops retrying once closed', async () => {
    vi.useFakeTimers()
    const fetchImpl = vi.fn().mockResolvedValue(new Response('nope', { status: 500 }))
    const handle = openTopologyEventStream({
      onEvent: () => {},
      fetchImpl: fetchImpl as unknown as typeof fetch,
      minBackoffMs: 100,
      maxBackoffMs: 1000
    })
    await vi.advanceTimersByTimeAsync(0)
    expect(fetchImpl).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(99)
    expect(fetchImpl).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(1)
    expect(fetchImpl).toHaveBeenCalledTimes(2)
    // second failure waits 200ms
    await vi.advanceTimersByTimeAsync(199)
    expect(fetchImpl).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(1)
    expect(fetchImpl).toHaveBeenCalledTimes(3)

    handle.close()
    await vi.advanceTimersByTimeAsync(5000)
    expect(fetchImpl).toHaveBeenCalledTimes(3)
  })
})
