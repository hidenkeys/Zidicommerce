import { FormEvent, useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { apiGet, apiPost, setStoredToken } from "../api/client";
import { AuthLayout } from "../auth";
import { roleLabel } from "../lib/format";

type InvitationPreview = {
  email?: string;
  business_name?: string;
  role?: string;
  role_label?: string;
  expires_at?: string;
  expired?: boolean;
  accepted?: boolean;
  first_name?: string;
  existing_user?: boolean;
};

type AcceptResponse = {
  access_token?: string;
};

export function InviteAcceptPage() {
  const [params] = useSearchParams();
  const token = params.get("token") ?? "";
  const navigate = useNavigate();
  const [preview, setPreview] = useState<InvitationPreview | null>(null);
  const [error, setError] = useState("");
  const [password, setPassword] = useState("");
  const [firstName, setFirstName] = useState("");
  const [lastName, setLastName] = useState("");
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    async function load() {
      if (!token) {
        setError("This invitation link is missing its token.");
        return;
      }
      try {
        const response = await apiGet<InvitationPreview>(`/invitations/${token}`);
        setPreview(response.data);
        setFirstName(String(response.data.first_name ?? ""));
        setError("");
      } catch (loadError) {
        setError(loadError instanceof Error ? loadError.message : "This invitation is not valid.");
      }
    }
    void load();
  }, [token]);

  const blocked = useMemo(() => preview?.accepted || preview?.expired, [preview]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setSubmitting(true);
    setError("");
    try {
      const response = await apiPost<AcceptResponse>(`/invitations/${token}/accept`, {
        password,
        first_name: firstName,
        last_name: lastName,
      });
      if (response.data.access_token) {
        setStoredToken(response.data.access_token);
      }
      navigate("/", { replace: true });
    } catch (submitError) {
      setError(submitError instanceof Error ? submitError.message : "Could not accept this invitation.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <AuthLayout>
      <div className="login-card">
        <p className="eyebrow">Invitation</p>
        <h1>{preview?.business_name ? `Join ${preview.business_name}` : "Join ZidiCommerce"}</h1>
        <p className="muted">
          {preview?.existing_user
            ? "This email already has an account. Use that password to join this workspace."
            : "Create your account to start working in this workspace."}
        </p>
        {preview ? (
          <p className="help-text">
            {preview.email} · {preview.role_label || roleLabel(String(preview.role ?? ""))}
          </p>
        ) : null}
        {error ? <p className="flash error">{error}</p> : null}
        {blocked ? (
          <p className="muted">{preview?.accepted ? "This invitation has already been accepted." : "This invitation has expired. Ask your admin to send a new one."}</p>
        ) : (
          <form onSubmit={submit}>
            {!preview?.existing_user ? (
              <>
                <label>First name<input value={firstName} onChange={(event) => setFirstName(event.target.value)} required /></label>
                <label>Last name<input value={lastName} onChange={(event) => setLastName(event.target.value)} /></label>
              </>
            ) : null}
            <label>{preview?.existing_user ? "Account password" : "Create a password"}
              <input type="password" value={password} onChange={(event) => setPassword(event.target.value)} minLength={8} required />
            </label>
            <button type="submit" className="primary" disabled={submitting || !token}>{submitting ? "Joining..." : "Accept invitation"}</button>
          </form>
        )}
        <p className="help-text"><Link to="/login">Already have access? Sign in</Link></p>
      </div>
    </AuthLayout>
  );
}
