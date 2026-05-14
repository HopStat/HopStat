import { useState } from 'react'
import { QueryForm } from '@/components/query/query-form'
import { ResultContainer } from '@/components/results/result-container'

export function QueryPage() {
  const [queryId, setQueryId] = useState<string | null>(null)

  return (
    <div className="max-w-5xl mx-auto py-6 px-4 sm:px-6 space-y-4">
      <QueryForm onQuerySubmit={setQueryId} />
      <ResultContainer queryId={queryId} />
    </div>
  )
}
