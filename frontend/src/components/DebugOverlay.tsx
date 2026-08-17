import { useEffect, useRef, useState } from 'react';

interface LogLine {
  ts: string;
  level: 'INFO' | 'WARN' | 'ERROR';
  msg: string;
}

const LEVEL_COLORS: Record<string, string> = {
  ERROR: '#ef4444',
  WARN:  '#f59e0b',
  INFO:  '#6b7280',
};

/**
 * DebugOverlay streams server log lines from /api/debug/logs (SSE).
 * Only rendered in dev mode (import.meta.env.DEV) when the endpoint is active.
 * Mount in App.tsx: {import.meta.env.DEV && <DebugOverlay />}
 */
export function DebugOverlay() {
  const [lines, setLines] = useState<LogLine[]>([]);
  const [open, setOpen] = useState(false);
  const [filter, setFilter] = useState('');
  const [autoScroll, setAutoScroll] = useState(true);
  const [connected, setConnected] = useState(false);
  const bottomRef = useRef<HTMLDivElement>(null);
  const esRef = useRef<EventSource | null>(null);

  useEffect(() => {
    const es = new EventSource('/api/debug/logs');
    esRef.current = es;

    es.onopen = () => setConnected(true);
    es.onerror = () => setConnected(false);

    es.onmessage = (e) => {
      try {
        const line = JSON.parse(e.data) as LogLine;
        setLines(prev => {
          const next = [...prev, line];
          return next.length > 1000 ? next.slice(-1000) : next;
        });
      } catch {
        // ignore parse errors
      }
    };

    return () => {
      es.close();
      esRef.current = null;
      setConnected(false);
    };
  }, []);

  // Auto-scroll to bottom when new lines arrive.
  useEffect(() => {
    if (autoScroll && open) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [lines, autoScroll, open]);

  const filtered = filter
    ? lines.filter(l => l.msg.toLowerCase().includes(filter.toLowerCase()))
    : lines;

  return (
    <div style={{
      position: 'fixed',
      bottom: '16px',
      right: '16px',
      zIndex: 9999,
      fontFamily: 'monospace',
      fontSize: '0.75rem',
    }}>
      {/* Toggle button */}
      <button
        onClick={() => setOpen(o => !o)}
        style={{
          backgroundColor: connected ? '#1e293b' : '#6b7280',
          color: '#fff',
          border: 'none',
          borderRadius: '8px',
          padding: '6px 12px',
          cursor: 'pointer',
          marginBottom: open ? '4px' : 0,
          display: 'flex',
          alignItems: 'center',
          gap: '6px',
          fontSize: '0.75rem',
          boxShadow: '0 2px 8px rgba(0,0,0,0.3)',
        }}
      >
        <span style={{ fontSize: '0.6rem', color: connected ? '#22c55e' : '#ef4444' }}>●</span>
        Dev Logs {lines.length > 0 && `(${lines.length})`}
        <span style={{ fontSize: '0.65rem', opacity: 0.7 }}>{open ? '▾' : '▴'}</span>
      </button>

      {open && (
        <div style={{
          backgroundColor: '#0f172a',
          border: '1px solid #334155',
          borderRadius: '8px',
          width: '600px',
          maxWidth: 'calc(100vw - 32px)',
          boxShadow: '0 8px 32px rgba(0,0,0,0.5)',
          display: 'flex',
          flexDirection: 'column',
          height: '340px',
        }}>
          {/* Header bar */}
          <div style={{
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
            padding: '6px 10px',
            borderBottom: '1px solid #1e293b',
            backgroundColor: '#1e293b',
            borderRadius: '8px 8px 0 0',
            flexShrink: 0,
          }}>
            <input
              placeholder="Filter logs…"
              value={filter}
              onChange={e => setFilter(e.target.value)}
              style={{
                flex: 1,
                backgroundColor: '#0f172a',
                border: '1px solid #334155',
                color: '#e2e8f0',
                borderRadius: '4px',
                padding: '3px 7px',
                fontSize: '0.72rem',
                outline: 'none',
              }}
            />
            <label style={{ color: '#94a3b8', cursor: 'pointer', userSelect: 'none', display: 'flex', alignItems: 'center', gap: '3px' }}>
              <input
                type="checkbox"
                checked={autoScroll}
                onChange={e => setAutoScroll(e.target.checked)}
                style={{ margin: 0 }}
              />
              <span>Auto-scroll</span>
            </label>
            <button
              onClick={() => setLines([])}
              style={{
                backgroundColor: 'transparent',
                color: '#64748b',
                border: '1px solid #334155',
                borderRadius: '4px',
                padding: '2px 8px',
                cursor: 'pointer',
                fontSize: '0.7rem',
              }}
            >
              Clear
            </button>
          </div>

          {/* Log lines */}
          <div style={{
            flex: 1,
            overflowY: 'auto',
            padding: '6px 10px',
          }}>
            {filtered.length === 0 && (
              <div style={{ color: '#475569', padding: '8px 0' }}>
                {connected ? 'Waiting for log output…' : 'Disconnected — is DEBUG_LOGS=1 set?'}
              </div>
            )}
            {filtered.map((line, i) => (
              <div key={i} style={{ marginBottom: '2px', display: 'flex', gap: '8px', lineHeight: 1.4 }}>
                <span style={{ color: '#475569', flexShrink: 0 }}>
                  {new Date(line.ts).toLocaleTimeString()}
                </span>
                <span style={{
                  color: LEVEL_COLORS[line.level] ?? '#6b7280',
                  flexShrink: 0,
                  width: '38px',
                  fontSize: '0.68rem',
                  fontWeight: 600,
                }}>
                  {line.level}
                </span>
                <span style={{ color: '#e2e8f0', wordBreak: 'break-all' }}>
                  {line.msg}
                </span>
              </div>
            ))}
            <div ref={bottomRef} />
          </div>
        </div>
      )}
    </div>
  );
}
