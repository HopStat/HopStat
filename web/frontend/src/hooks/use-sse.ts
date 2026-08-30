import { useState, useEffect, useRef } from 'react'
import type { ASInfo, QueryResult } from '@/types/domain'

interface UseQueryStreamReturn {
  result: QueryResult | null
  lines: string[]
  error: string | null
  outputComplete: boolean
}

function mergeAsPathEnriched(
  data: { as_path?: number[]; as_path_enriched?: ASInfo[] },
  prev: QueryResult | null,
): ASInfo[] {
  if (data.as_path_enriched?.length) {
    return data.as_path_enriched
  }
  if (data.as_path?.length) {
    return []
  }
  return prev?.as_path_enriched ?? []
}

function mergeQueryStreamResult(
  queryId: string,
  data: QueryResult,
  prev: QueryResult | null,
): QueryResult {
  const base = prev?.id === queryId ? prev : null
  return {
    ...data,
    as_path: data.as_path?.length ? data.as_path : (base?.as_path ?? []),
    as_path_prefix: data.as_path_prefix ?? base?.as_path_prefix,
    as_path_enriched: mergeAsPathEnriched(data, base),
    as_path_nodes: data.as_path_nodes?.length ? data.as_path_nodes : base?.as_path_nodes,
  }
}

export function useQueryStream(queryId: string | null): UseQueryStreamReturn {
  const [result, setResult] = useState<QueryResult | null>(null)
  const [lines, setLines] = useState<string[]>([])
  const [error, setError] = useState<string | null>(null)
  const [outputComplete, setOutputComplete] = useState(false)
  const esRef = useRef<EventSource | null>(null)
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  // Switching query clears everything the previous one produced. Done during render rather
  // than in the effect: the effect ran after the first paint, so a new query briefly showed
  // the old query's terminal output before wiping it.
  const [streamedId, setStreamedId] = useState(queryId)
  if (queryId !== streamedId) {
    setStreamedId(queryId)
    setResult(null)
    setLines([])
    setError(null)
    setOutputComplete(false)
  }

  useEffect(() => {
    if (!queryId) return

    const es = new EventSource(`/api/v1/query/${queryId}/stream`)
    esRef.current = es
    let finished = false

    es.addEventListener('output', (e) => {
      try {
        const data = JSON.parse(e.data)
        if (data.line !== undefined) {
          setLines(prev => [...prev, data.line])
        }
      } catch { /* ignore parse errors */ }
    })

    es.addEventListener('partial', (e) => {
      try {
        const data = JSON.parse(e.data) as {
          parsed?: QueryResult['parsed']
          raw?: string
          matched_rules?: QueryResult['matched_rules']
          as_path?: QueryResult['as_path']
          as_path_prefix?: QueryResult['as_path_prefix']
          as_path_enriched?: QueryResult['as_path_enriched']
          as_path_nodes?: QueryResult['as_path_nodes']
        }
        setResult(prev => {
          const base = prev?.id === queryId ? prev : null
          return {
            id: queryId,
            status: 'running',
            raw: data.raw?.trim() ? data.raw : (base?.raw ?? ''),
            parsed: data.parsed ?? base?.parsed ?? null,
            duration_ms: base?.duration_ms ?? 0,
            error_msg: base?.error_msg ?? '',
            error_code: base?.error_code ?? '',
            matched_rules: data.matched_rules ?? base?.matched_rules ?? [],
            as_path: data.as_path?.length ? data.as_path : (base?.as_path ?? []),
            as_path_prefix: data.as_path_prefix ?? base?.as_path_prefix,
            as_path_enriched: mergeAsPathEnriched(data, base),
            as_path_nodes: data.as_path_nodes?.length ? data.as_path_nodes : base?.as_path_nodes,
          }
        })
      } catch { /* ignore parse errors */ }
    })

    es.addEventListener('output_done', () => {
      setOutputComplete(true)
    })

    es.addEventListener('result', (e) => {
      try {
        const data = JSON.parse(e.data) as QueryResult
        finished = true
        setOutputComplete(true)
        setResult(prev => mergeQueryStreamResult(queryId, data, prev))
        if (data.raw) {
          setLines(prev => prev.length > 0 ? prev : data.raw.split('\n').filter(l => l.length > 0))
        }
        if (data.status === 'done' || data.status === 'error') {
          es.close()
        }
      } catch { /* ignore parse errors */ }
    })

    es.addEventListener('progress', (e) => {
      try {
        const data = JSON.parse(e.data)
        if (data.status) {
          setResult(prev => prev ? { ...prev, status: data.status } : { id: queryId, status: data.status, raw: '', parsed: null, duration_ms: 0, error_msg: '', error_code: '', matched_rules: [], as_path: [], as_path_enriched: [] })
        }
      } catch { /* ignore */ }
    })

    es.onerror = () => {
      if (finished) {
        es.close()
        return
      }
      es.close()
      // Fallback to polling with limited retries for transient errors
      let pollCount = 0
      let consecutiveErrors = 0
      const interval = setInterval(async () => {
        pollCount++
        if (pollCount > 60) {
          clearInterval(interval)
          intervalRef.current = null
          setError('Timeout')
          return
        }
        try {
          const res = await fetch(`/api/v1/query/${queryId}`)
          if (!res.ok) throw new Error(`HTTP ${res.status}`)
          const json = await res.json()
          const data = json.data as QueryResult
          consecutiveErrors = 0
          setResult(data)
          if (data.raw) {
            setLines(data.raw.split('\n').filter(l => l.length > 0))
          }
          if (data.status === 'done' || data.status === 'error') {
            setOutputComplete(true)
            clearInterval(interval)
            intervalRef.current = null
          }
        } catch {
          consecutiveErrors++
          if (consecutiveErrors >= 5) {
            clearInterval(interval)
            intervalRef.current = null
            setError('Failed to fetch result')
          }
        }
      }, 1000)
      intervalRef.current = interval
    }

    return () => {
      es.close()
      esRef.current = null
      if (intervalRef.current) {
        clearInterval(intervalRef.current)
        intervalRef.current = null
      }
    }
  }, [queryId])

  return { result, lines, error, outputComplete }
}
