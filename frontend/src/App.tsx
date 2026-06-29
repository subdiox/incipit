import { useEffect } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { useSiteTitle } from '@/lib/hooks'
import { Layout } from '@/components/Layout'
import { RequireAdmin, RequireAuth } from '@/components/RequireAuth'
import { LoginPage } from '@/pages/LoginPage'
import { LibraryPage } from '@/pages/LibraryPage'
import { BookDetailPage } from '@/pages/BookDetailPage'
import { ReaderPage } from '@/pages/ReaderPage'
import { ShelvesPage } from '@/pages/ShelvesPage'
import { AdminPage } from '@/pages/AdminPage'
import { AccountPage } from '@/pages/AccountPage'

export function App() {
  const siteTitle = useSiteTitle()
  useEffect(() => {
    document.title = siteTitle
  }, [siteTitle])

  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />

      {/* Reader is full-screen and lives outside the app shell. */}
      <Route
        path="/books/:id/read"
        element={
          <RequireAuth>
            <ReaderPage />
          </RequireAuth>
        }
      />

      <Route
        element={
          <RequireAuth>
            <Layout />
          </RequireAuth>
        }
      >
        <Route path="/" element={<LibraryPage />} />
        <Route path="/books/:id" element={<BookDetailPage />} />
        <Route path="/shelves" element={<ShelvesPage />} />
        <Route path="/account" element={<AccountPage />} />
        <Route
          path="/admin"
          element={
            <RequireAdmin>
              <AdminPage />
            </RequireAdmin>
          }
        />
      </Route>

      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
