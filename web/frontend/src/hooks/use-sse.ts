import { useState, useEffect, useRef } from 'react'
import type { QueryResult } from '@/types/domain'

interface UseQueryStreamReturn {
  result: QueryResult | null
  lines: string[]
  error: string | null
}

export function useQueryStream(queryId: string | null): UseQueryStreamReturn {
  const [result, setResult] = useState<QueryResult | null>(null)
  const [lines, setLines] = useState<string[]>([])
  const [error, setError] = useState<string | null>(null)
  const esRef = useRef<EventSource | null>(null)

  useEffect(() => {
    if (!queryId) {
      setResult(null)
      setLines([])
      setError(null)
      return
    }

    setResult(null)
    setLines([])
    setError(null)

    const es = new EventSource(`/api/v1/query/${queryId}/stream`)
    esRef.current = es

    es.addEventListener('output', (e) => {
      try {
        const data = JSON.parse(e.data)
        if (data.line !== undefined) {
          setLines(prev => [...prev, data.line])
        }
      } catch { /* ignore parse errors */ }
    })

    es.addEventListener('result', (e) => {
      try {
        const data = JSON.parse(e.data) as QueryResult
        setResult(data)
        if (data.status === 'done' || data.status === 'error') {
          es.close()
        }
      } catch { /* ignore parse errors */ }
    })

    es.addEventListener('progress', (e) => {
      try {
        const data = JSON.parse(e.data)
        if (data.status) {
          setResult(prev => prev ? { ...prev, status: data.status } : { id: queryId, status: data.status, raw: '', parsed: null, duration_ms: 0, error_msg: '', error_code: '', matched_rules: [], as_path_enriched: [] })
        }
      } catch { /* ignore */ }
    })

    es.onerror = () => {
      es.close()
      // Fallback to polling
      let pollCount = 0
      const interval = setInterval(async () => {
        pollCount++
        if (pollCount > 60) { clearInterval(interval); setError('Timeout'); return }
        try {
          const res = await fetch(`/api/v1/query/${queryId}`)
          const json = await res.json()
          const data = json.data as QueryResult
          setResult(data)
          if (data.raw) {
            setLines(data.raw.split('\n').filter(l => l.length > 0))
          }
          if (data.status === 'done' || data.status === 'error') clearInterval(interval)
        } catch {
          clearInterval(interval)
          setError('Failed to fetch result')
        }
      }, 1000)
    }

    return () => {
      es.close()
      esRef.current = null
    }
  }, [queryId])

  return { result, lines, error }
}
