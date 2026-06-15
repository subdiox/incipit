import { createContext, useCallback, useContext, useMemo, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '@/lib/api'
import type { User } from '@/types'

interface AuthValue {
  user: User | null
  loading: boolean
  setUser: (user: User) => void
  logout: () => Promise<void>
  refetch: () => void
}

const AuthContext = createContext<AuthValue | undefined>(undefined)

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()

  const { data, isLoading, refetch } = useQuery({
    queryKey: ['me'],
    queryFn: async () => {
      try {
        return await api.me()
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) return null
        throw err
      }
    },
    retry: false,
    staleTime: 60_000,
  })

  const setUser = useCallback(
    (user: User) => {
      queryClient.setQueryData(['me'], user)
    },
    [queryClient],
  )

  const logout = useCallback(async () => {
    try {
      await api.logout()
    } finally {
      queryClient.setQueryData(['me'], null)
      queryClient.clear()
    }
  }, [queryClient])

  const value = useMemo<AuthValue>(
    () => ({
      user: data ?? null,
      loading: isLoading,
      setUser,
      logout,
      refetch: () => void refetch(),
    }),
    [data, isLoading, setUser, logout, refetch],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

// eslint-disable-next-line react-refresh/only-export-components
export function useAuth(): AuthValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be used within AuthProvider')
  return ctx
}
