import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { apiDelete, apiGet } from "../api/client";
import { useAuth } from "../auth";
import { Button, Card, Flash, Modal, SectionHeader } from "./ui";

type DeletionPreview = { counts: Record<string, number>; total_rows: number; dry_run: boolean };

export function OrganizationDangerZone({ organizationID, slug }: { organizationID: string; slug: string }) {
  const { logout } = useAuth();
  const navigate = useNavigate();
  const [enabled, setEnabled] = useState(false);
  const [preview, setPreview] = useState<DeletionPreview | null>(null);
  const [open, setOpen] = useState(false);
  const [confirmation, setConfirmation] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    let cancelled = false;
    setEnabled(false);
    setPreview(null);
    void apiGet<{ enabled: boolean }>(`/organizations/${organizationID}/hard-delete-capability`)
      .then(response => { if (!cancelled) setEnabled(response.data.enabled); })
      .catch(() => { /* Keep destructive controls hidden if capability cannot be verified. */ });
    return () => { cancelled = true; };
  }, [organizationID]);

  async function reviewDeletion() {
    setBusy(true);
    setError("");
    setPreview(null);
    try {
      const response = await apiDelete<DeletionPreview>(`/organizations/${organizationID}?dry_run=true`, { confirm_slug: slug });
      setPreview(response.data);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Could not preview organization deletion.");
    } finally {
      setBusy(false);
    }
  }

  async function deleteOrganization() {
    if (busy || confirmation !== slug || !preview) return;
    setBusy(true);
    setError("");
    try {
      await apiDelete<DeletionPreview>(`/organizations/${organizationID}`, { confirm_slug: confirmation });
      logout();
      navigate("/login", { replace: true });
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "Could not delete the organization.");
      setBusy(false);
    }
  }

  if (!enabled || !slug) return null;

  return (
    <Card>
      <SectionHeader title="Danger zone" description="Permanently remove this organization and all its data. This cannot be undone." />
      <p>Back up the database before continuing. Accounts shared with another organization and platform administrators are kept.</p>
      <Flash message={!open ? error : ""} />
      <Button type="button" loading={busy && !open} disabled={busy} onClick={() => void reviewDeletion()}>Preview deletion</Button>
      {preview ? <>
        <p role="status">{preview.total_rows.toLocaleString()} rows would be removed. Counts may change before deletion.</p>
        <details>
          <summary>Rows by table</summary>
          <div className="table-wrap"><table>
            <thead><tr><th scope="col">Table</th><th scope="col">Rows to remove</th></tr></thead>
            <tbody>{Object.entries(preview.counts).sort(([a], [b]) => a.localeCompare(b)).map(([table, count]) => <tr key={table}><th scope="row">{table}</th><td>{count.toLocaleString()}</td></tr>)}</tbody>
          </table></div>
        </details>
        <Button type="button" className="danger" disabled={busy} onClick={() => { setConfirmation(""); setError(""); setOpen(true); }}>Delete organization…</Button>
      </> : null}
      <Modal open={open} danger title="Delete organization permanently?" description="All organization data will be removed and you will be signed out. This action cannot be undone." onClose={() => { if (!busy) setOpen(false); }} footer={<>
        <Button type="button" className="ghost" disabled={busy} onClick={() => setOpen(false)}>Cancel</Button>
        <Button type="button" className="danger" loading={busy} disabled={confirmation !== slug || busy} onClick={() => void deleteOrganization()}>Permanently delete organization</Button>
      </>}>
        <Flash message={error} />
        <p>Type <strong>{slug}</strong> to confirm.</p>
        <label>Organization slug<input autoComplete="off" spellCheck={false} value={confirmation} disabled={busy} onChange={event => setConfirmation(event.target.value)} /></label>
      </Modal>
    </Card>
  );
}
