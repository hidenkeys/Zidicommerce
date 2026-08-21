import { createContext, ReactNode, useContext, useEffect, useMemo, useState } from "react";
import { Navigate, Outlet, useLocation } from "react-router-dom";
import { UNAUTHORIZED_EVENT, apiGet, clearStoredToken, getStoredToken } from "./api/client";

export type AuthUser = {
  id: string;
  organization_id: string;
  email?: string;
  role: string;
};

type AuthContextValue = {
  user: AuthUser;
  logout: () => void;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthLayout({ children }: { children: ReactNode }) {
  return (
    <div className="login-layout">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark">ZC</div>
          <div>
            <strong>ZidiCommerce</strong>
            <span>Merchant operations</span>
          </div>
        </div>
        <p className="login-sidebar-copy">Sign in to manage your organization, catalogue, orders, and bots.</p>
      </aside>
      <main className="login-panel">{children}</main>
    </div>
  );
}

export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) {
    throw new Error("useAuth must be used inside a signed-in session");
  }
  return value;
}

export function RequireAuth() {
  const location = useLocation();
  const [user, setUser] = useState<AuthUser | null>(null);
  const [checking, setChecking] = useState(true);

  useEffect(() => {
    let cancelled = false;

    async function checkSession() {
      if (!getStoredToken()) {
        if (!cancelled) {
          setUser(null);
          setChecking(false);
        }
        return;
      }

      try {
        const response = await apiGet<AuthUser>("/auth/me");
        if (!cancelled) {
          setUser(response.data);
        }
      } catch {
        clearStoredToken();
        if (!cancelled) {
          setUser(null);
        }
      } finally {
        if (!cancelled) {
          setChecking(false);
        }
      }
    }

    function onUnauthorized() {
      setUser(null);
    }

    window.addEventListener(UNAUTHORIZED_EVENT, onUnauthorized);
    void checkSession();

    return () => {
      cancelled = true;
      window.removeEventListener(UNAUTHORIZED_EVENT, onUnauthorized);
    };
  }, []);

  const value = useMemo<AuthContextValue | null>(() => {
    if (!user) return null;
    return {
      user,
      logout: () => {
        clearStoredToken();
        setUser(null);
      },
    };
  }, [user]);

  if (checking) {
    return (
      <AuthLayout>
        <div className="login-card">
          <p className="muted">Checking session...</p>
        </div>
      </AuthLayout>
    );
  }

  if (!value) {
    return <Navigate to="/login" replace state={{ from: location }} />;
  }

  return (
    <AuthContext.Provider value={value}>
      <Outlet />
    </AuthContext.Provider>
  );
}
