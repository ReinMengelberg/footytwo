import type { ClientInput, Snapshot } from './protocol'

export type ConnStatus = 'connecting' | 'open' | 'closed'

export interface ConnectOptions {
  onSnapshot: (snap: Snapshot) => void
  onStatus?: (status: ConnStatus) => void
}

export interface Connection {
  send: (input: ClientInput) => void
  close: () => void
}

// connect opens a WebSocket to the server, parsing inbound snapshots and
// exposing an input sender. Naive Phase-2 transport: snapshots are applied
// directly with no interpolation.
export function connect(url: string, opts: ConnectOptions): Connection {
  const ws = new WebSocket(url)
  opts.onStatus?.('connecting')

  ws.onopen = () => opts.onStatus?.('open')
  ws.onclose = () => opts.onStatus?.('closed')
  // Swallow the error event; onclose reports the status. (A failed initial
  // connect still logs a browser network warning, which is unavoidable.)
  ws.onerror = () => {}
  ws.onmessage = (ev) => {
    try {
      opts.onSnapshot(JSON.parse(ev.data as string) as Snapshot)
    } catch {
      // ignore malformed frames
    }
  }

  return {
    send(input) {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify(input))
      }
    },
    close() {
      ws.close()
    },
  }
}
