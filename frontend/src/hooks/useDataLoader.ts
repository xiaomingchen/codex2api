import { useCallback, useEffect, useRef, useState } from 'react'
import { getErrorMessage } from '../utils/error'

interface LoadOptions {
  silent?: boolean
}

interface UseDataLoaderOptions<T> {
  initialData: T
  load: () => Promise<T>
  onError?: (message: string, error: unknown) => void
}

export function useDataLoader<T>({ initialData, load, onError }: UseDataLoaderOptions<T>) {
  const [data, setData] = useState<T>(initialData)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const requestIdRef = useRef(0)

  const run = useCallback(async (options: LoadOptions = {}) => {
    const requestId = ++requestIdRef.current
    const { silent = false } = options

    if (!silent) {
      setLoading(true)
      setError(null)
    }

    try {
      const nextData = await load()
      if (requestId !== requestIdRef.current) {
        return null
      }
      setData(nextData)
      setError(null)
      return nextData
    } catch (err) {
      if (requestId !== requestIdRef.current) {
        return null
      }
      const message = getErrorMessage(err)
      if (!silent) {
        setError(message)
      }
      onError?.(message, err)
      return null
    } finally {
      if (requestId === requestIdRef.current) {
        setLoading(false)
      }
    }
  }, [load, onError])

  useEffect(() => {
    void run()
  }, [run])

  const reload = useCallback(() => run(), [run])
  const reloadSilently = useCallback(() => run({ silent: true }), [run])

  return {
    data,
    setData,
    loading,
    error,
    reload,
    reloadSilently,
  }
}
