import { PasswordField } from "./PasswordField";
import { FormEvent, useEffect, useState } from "react";
import { Link, Navigate, useLocation, useNavigate } from "react-router-dom";
import { apiGet, apiPost, getStoredToken, setStoredToken } from "../api/client";
import { AuthLayout, type AuthUser } from "../auth";

type LocationState = {
  from?: {
    pathname: string;
    search: string;
    hash: string;
  };
};

type LoginResponse = {
  access_token: string;
  user?: AuthUser;
};

export function LoginPage({ mode = "login" }: { mode?: "login" | "register" }) {
  const navigate = useNavigate();
  const location = useLocation();
  const from = (location.state as LocationState | null)?.from;
  const nextPath =
    from && from.pathname !== "/login" && from.pathname !== "/register"
      ? `${from.pathname}${from.search}${from.hash}`
      : "/";

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [redirectTo, setRedirectTo] = useState<string | null>(null);
  const [checkingSession, setCheckingSession] = useState(() =>
    Boolean(getStoredToken()),
  );

  useEffect(() => {
    let cancelled = false;

    async function resumeSession() {
      if (!getStoredToken()) {
        if (!cancelled) setCheckingSession(false);
        return;
      }
      try {
        await apiGet<AuthUser>("/auth/me");
        if (!cancelled) {
          setRedirectTo(mode === "register" ? "/setup" : nextPath);
        }
      } catch {
        if (!cancelled) setCheckingSession(false);
      }
    }

    void resumeSession();
    return () => {
      cancelled = true;
    };
  }, [mode, nextPath]);

  if (redirectTo) {
    return <Navigate to={redirectTo} replace />;
  }

  if (checkingSession) {
    return (
      <AuthLayout>
        <div className="z-access__card">
          <p className="muted">Checking session...</p>
        </div>
      </AuthLayout>
    );
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      const response =
        mode === "register"
          ? await apiPost<LoginResponse>("/auth/register", {
              email,
              password,
              first_name: firstName,
              last_name: lastName,
            })
          : await apiPost<LoginResponse>("/auth/login", { email, password });
      setStoredToken(response.data.access_token);
      navigate(mode === "register" ? "/setup" : nextPath, { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Request failed");
    } finally {
      setSubmitting(false);
    }
  }

  const isRegister = mode === "register";

  return (
    <AuthLayout>
      <form className="z-access__card" onSubmit={submit}>
        <div>
          <h1>{isRegister ? "Create your account" : "Welcome back"}</h1>
          <p>
            {isRegister
              ? "Create your account to set up your first commerce workspace."
              : "Sign in to your commerce workspace."}
          </p>
        </div>
        {error ? (
          <p role="alert" className="error-text">
            {error}
          </p>
        ) : null}
        {isRegister ? (
          <div className="z-access__names">
            <label>
              First name
              <input
                autoComplete="given-name"
                value={firstName}
                onChange={(event) => setFirstName(event.target.value)}
              />
            </label>
            <label>
              Last name
              <input
                autoComplete="family-name"
                value={lastName}
                onChange={(event) => setLastName(event.target.value)}
              />
            </label>
          </div>
        ) : null}
        <label>
          Email
          <input
            type="email"
            autoComplete="username"
            autoFocus
            required
            value={email}
            onChange={(event) => setEmail(event.target.value)}
          />
        </label>
        <PasswordField
          label="Password"
          autoComplete={isRegister ? "new-password" : "current-password"}
          required
          minLength={isRegister ? 8 : undefined}
          value={password}
          onChange={(event) => setPassword(event.target.value)}
        />
        {!isRegister && <Link to="/forgot-password">Forgot password?</Link>}
        <button type="submit" disabled={submitting}>
          {submitting
            ? isRegister
              ? "Creating account..."
              : "Signing in..."
            : isRegister
              ? "Create account"
              : "Sign in"}
        </button>
        <p className="z-access__note">
          {isRegister ? (
            <>
              Already have an account? <Link to="/login">Sign in</Link>
            </>
          ) : (
            <>
              New merchant? <Link to="/register">Create an account</Link>
            </>
          )}
        </p>
      </form>
    </AuthLayout>
  );
}
