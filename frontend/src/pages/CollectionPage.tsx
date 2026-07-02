import { Navigate, useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { FullPageSpinner } from '@/components/Spinner'
import { LibraryPage } from './LibraryPage'

// CollectionPage renders the library scoped to an admin-defined collection (a
// saved tag filter). The URL uses the collection's 1-based position in display
// order (so /collections/1 is always the first, and the number tracks reorders),
// reusing LibraryPage for full library functionality.
export function CollectionPage() {
  const { id } = useParams()
  const pos = Number(id)
  const { data: collections, isLoading } = useQuery({ queryKey: ['collections'], queryFn: api.collections })
  if (isLoading) return <FullPageSpinner />
  const collection = collections && pos >= 1 ? collections[pos - 1] : undefined
  if (!collection) return <Navigate to="/" replace />
  // key remounts LibraryPage when switching collections so its local state resets.
  return <LibraryPage key={collection.id} collection={collection} />
}
