const DB_NAME = 'hopstat'
const DB_VERSION = 1
const STORE_NAME = 'query-history'
export const QUERY_HISTORY_LIMIT = 100

export interface QueryHistoryRecord {
  key: string
  target: string
  command: string
  nodeId: number
  nodeName: string
  usedAt: number
}

export interface QueryHistorySaveInput {
  target: string
  command: string
  nodeId: number
  nodeName: string
}

function idbRequest<T>(request: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error ?? new Error('indexedDB request failed'))
  })
}

function historyKey(input: QueryHistorySaveInput): string {
  return `${input.command}|${input.nodeId}|${input.target.trim().toLowerCase()}`
}

function openDatabase(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    if (typeof indexedDB === 'undefined') {
      reject(new Error('indexedDB unavailable'))
      return
    }

    const request = indexedDB.open(DB_NAME, DB_VERSION)
    request.onupgradeneeded = () => {
      const db = request.result
      if (!db.objectStoreNames.contains(STORE_NAME)) {
        db.createObjectStore(STORE_NAME, { keyPath: 'key' })
      }
    }
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error ?? new Error('indexedDB open failed'))
  })
}

function runTransaction<T>(
  mode: IDBTransactionMode,
  fn: (store: IDBObjectStore) => Promise<T>,
): Promise<T> {
  return openDatabase().then(db => new Promise<T>((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, mode)
    const store = tx.objectStore(STORE_NAME)

    fn(store)
      .then(result => {
        tx.oncomplete = () => {
          db.close()
          resolve(result)
        }
        tx.onerror = () => {
          db.close()
          reject(tx.error ?? new Error('indexedDB transaction failed'))
        }
      })
      .catch(err => {
        db.close()
        reject(err)
      })
  }))
}

async function trimHistory(store: IDBObjectStore): Promise<void> {
  const entries = await new Promise<QueryHistoryRecord[]>((resolve, reject) => {
    const request = store.getAll()
    request.onsuccess = () => resolve(request.result as QueryHistoryRecord[])
    request.onerror = () => reject(request.error ?? new Error('indexedDB read failed'))
  })

  if (entries.length <= QUERY_HISTORY_LIMIT) return

  const keep = new Set(
    entries
      .sort((a, b) => b.usedAt - a.usedAt)
      .slice(0, QUERY_HISTORY_LIMIT)
      .map(entry => entry.key),
  )

  for (const entry of entries) {
    if (!keep.has(entry.key)) {
      store.delete(entry.key)
    }
  }
}

export async function saveSuccessfulQuery(input: QueryHistorySaveInput): Promise<void> {
  const target = input.target.trim()
  if (!target) return

  const record: QueryHistoryRecord = {
    key: historyKey(input),
    target,
    command: input.command,
    nodeId: input.nodeId,
    nodeName: input.nodeName.trim(),
    usedAt: Date.now(),
  }

  await runTransaction('readwrite', async store => {
    await idbRequest(store.put(record))
    await trimHistory(store)
  })
}

export async function listQueryHistory(): Promise<QueryHistoryRecord[]> {
  return runTransaction('readonly', store => new Promise<QueryHistoryRecord[]>((resolve, reject) => {
    const request = store.getAll()
    request.onsuccess = () => {
      const entries = (request.result as QueryHistoryRecord[])
        .sort((a, b) => b.usedAt - a.usedAt)
      resolve(entries)
    }
    request.onerror = () => reject(request.error ?? new Error('indexedDB read failed'))
  }))
}

export async function deleteQueryHistory(key: string): Promise<void> {
  if (!key) return
  await runTransaction('readwrite', async store => {
    await idbRequest(store.delete(key))
  })
}
