import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { api, setToken } from "./api";

type User = Awaited<ReturnType<typeof api.me>>;

export function Login({
  onAuthenticated,
}: {
  onAuthenticated: (user: User) => void;
}) {
  const navigate = useNavigate();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    setError("");
    setPending(true);
    try {
      const result = (await api.login(email, password)) as {
        access_token?: string;
      };
      const token = result.access_token;
      if (!token) throw new Error("No session token returned");
      setToken(token);
      const me = await api.me();
      onAuthenticated(me);
      navigate(me.role === "service_provider" ? "/provider" : "/owner");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not sign in");
    } finally {
      setPending(false);
    }
  }
  return (
    <div className="z-access">
      <div className="z-access__body">
        <div className="z-access__brand">
          Zidi <small>Field Service</small>
        </div>
        <main className="z-access__stage">
          <form className="z-access__card" onSubmit={onSubmit}>
            <header>
              <h1>Welcome back</h1>
              <p>Sign in to your field service workspace.</p>
            </header>
            <label>
              Email address
              <input
                type="email"
                autoComplete="username"
                required
                value={email}
                onChange={(event) => setEmail(event.target.value)}
              />
            </label>
            <label>
              Password
              <input
                type="password"
                autoComplete="current-password"
                required
                value={password}
                onChange={(event) => setPassword(event.target.value)}
              />
            </label>
            {error && <p role="alert">{error}</p>}
            <button type="submit" disabled={pending}>
              {pending ? "Signing in…" : "Sign in"}
            </button>
          </form>
        </main>
      </div>
      <div className="z-access__art" aria-hidden="true" />
    </div>
  );
}
