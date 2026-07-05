// mapPool runs fn over items with a bounded number of concurrent calls,
// preserving result order. onSettled fires after each item finishes (in
// completion order) so callers can render progress. Individual rejections are
// captured as { ok:false } rather than aborting the whole batch.
export type Settled<R> = { ok: true; value: R } | { ok: false; error: unknown }

export async function mapPool<T, R>(
  items: T[],
  concurrency: number,
  fn: (item: T, index: number) => Promise<R>,
  onSettled?: (done: number, total: number) => void,
): Promise<Settled<R>[]> {
  const results = new Array<Settled<R>>(items.length)
  let next = 0
  let done = 0
  const total = items.length
  const worker = async () => {
    for (;;) {
      const i = next++
      if (i >= total) return
      try {
        results[i] = { ok: true, value: await fn(items[i], i) }
      } catch (error) {
        results[i] = { ok: false, error }
      }
      done++
      onSettled?.(done, total)
    }
  }
  const workers = Array.from({ length: Math.min(Math.max(1, concurrency), total || 1) }, worker)
  await Promise.all(workers)
  return results
}
