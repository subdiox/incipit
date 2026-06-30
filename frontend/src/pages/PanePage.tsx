import { Navigate, useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { api } from '@/lib/api'
import { FullPageSpinner } from '@/components/Spinner'
import { LibraryPage } from './LibraryPage'

// PanePage renders the library scoped to an admin-defined pane (a saved tag
// filter). It reuses LibraryPage so the page keeps full library functionality.
export function PanePage() {
  const { id } = useParams()
  const paneId = Number(id)
  const { data: panes, isLoading } = useQuery({ queryKey: ['panes'], queryFn: api.panes })
  if (isLoading) return <FullPageSpinner />
  const pane = panes?.find((p) => p.id === paneId)
  if (!pane) return <Navigate to="/" replace />
  // key remounts LibraryPage when switching panes so its local state resets.
  return <LibraryPage key={pane.id} pane={pane} />
}
