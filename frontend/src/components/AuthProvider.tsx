import {
  ReactNode,
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from 'react'
import { useNavigate } from 'react-router-dom'
import * as api from '@/lib/api'
import type { AuthMode, AuthUser, UserRole } from '@/lib/api'
import {
  clearSession,
  getSession,
  onUnauthorized,
  setSession,
} from '@/lib/api'

interface AuthContextValue {
  mode: AuthMode | null
  user: AuthUser | null
  loading: boolean
  refresh: () => Promise<void>
  logout: () => void
  setUserFromLogin: (user: AuthUser, token: string) => void
  /** Role rank helpers — open/token mode → full access (no session role). */
  role: UserRole | null
  canWrite: boolean
  canAdmin: boolean
  isViewer: boolean
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [mode, setMode] = useState<AuthMode | null>(null)
  const [user, setUser] = useState<AuthUser | null>(null)
  const [loading, setLoading] = useState(true)
  const navigate = useNavigate()

  const refresh = useCallback(async () => {
    setLoading(true)
    try {
      const st = await api.authStatus()
      setMode(st.mode)
      if (st.mode === 'users' && getSession()) {
        try {
          const me = await api.getMe()
          setUser(me)
        } catch {
          clearSession()
          setUser(null)
        }
      } else {
        setUser(null)
      }
    } catch {
      setMode('open')
      setUser(null)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  useEffect(() => {
    return onUnauthorized(() => {
      if (mode === 'users') {
        const next = window.location.pathname.replace(/^\/app/, '') || '/'
        navigate(`/login?next=${encodeURIComponent(next)}`)
      }
      // token mode → TokenBar (unchanged) via its own listener
    })
  }, [mode, navigate])

  const logout = useCallback(() => {
    clearSession()
    setUser(null)
    navigate('/login')
  }, [navigate])

  const setUserFromLogin = useCallback((u: AuthUser, token: string) => {
    setSession(token)
    setUser(u)
    setMode('users')
  }, [])

  const role = user?.role ?? null
  const canAdmin = mode !== 'users' || role === 'admin'
  const canWrite = mode !== 'users' || role === 'admin' || role === 'editor'
  const isViewer = mode === 'users' && role === 'viewer'

  const value = useMemo(
    () => ({
      mode,
      user,
      loading,
      refresh,
      logout,
      setUserFromLogin,
      role,
      canWrite,
      canAdmin,
      isViewer,
    }),
    [mode, user, loading, refresh, logout, setUserFromLogin, role, canWrite, canAdmin, isViewer],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth requires AuthProvider')
  return ctx
}
