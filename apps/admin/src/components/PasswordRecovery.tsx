import { useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { AuthLayout } from "../auth";
import { apiPost } from "../api/client";
import { PasswordField } from "./PasswordField";

export function PasswordRecovery({ reset = false }: { reset?: boolean }) {
  const [token] = useState(() => window.location.hash.slice(1));
  const [value, setValue] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      if (reset) {
        await apiPost("/auth/reset-password", { token, password: value });
        window.history.replaceState(null, "", window.location.pathname);
      } else {
        await apiPost("/auth/forgot-password", { email: value });
      }
      setValue("");
      setDone(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Please try again.");
    } finally {
      setBusy(false);
    }
  }
  return (
    <AuthLayout>
      <form className="z-access__card" onSubmit={submit}>
        <h1>{reset ? "Choose a new password" : "Recover your account"}</h1>
        {done ? (
          <p role="status">
            {reset
              ? "Your password has been updated. Sign in to your workspace."
              : "If your email matches an active account, look for a reset link in your inbox."}
          </p>
        ) : reset && !token ? (
          <p role="alert">
            This link is incomplete.{" "}
            <Link to="/forgot-password">Request a new link</Link>.
          </p>
        ) : (
          <>
            <p>
              {reset
                ? "Use at least eight characters."
                : "Enter your workspace email to request a password reset link."}
            </p>
            {reset ? (
              <PasswordField
                label="New password"
                autoComplete="new-password"
                required
                minLength={8}
                value={value}
                onChange={(e) => setValue(e.target.value)}
              />
            ) : (
              <label>
                Email address
                <input
                  type="email"
                  autoComplete="email"
                  required
                  value={value}
                  onChange={(e) => setValue(e.target.value)}
                />
              </label>
            )}
            {error && (
              <p role="alert" className="error-text">
                {error}
              </p>
            )}
            <button type="submit" disabled={busy}>
              {busy
                ? "Please wait…"
                : reset
                  ? "Save password"
                  : "Send reset link"}
            </button>
          </>
        )}
        <Link to="/login">Return to sign in</Link>
      </form>
    </AuthLayout>
  );
}
