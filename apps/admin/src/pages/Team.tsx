import { FormEvent, useEffect, useState } from "react";
import { apiGet, apiPatch, apiPost, apiPut } from "../api/client";
import { Badge, Card, EmptyState, Flash, FormGrid, Page } from "../components/ui";
import { humanStatus, roleLabel, type Row } from "../lib/format";

type Member = Row & { id: string; role?: string; status?: string; user?: Row };
type Store = Row & { id: string; name?: string };
type Invitation = Row & {
  id: string;
  email?: string;
  role?: string;
  status?: string;
  expires_at?: string;
  email_status?: string;
  email_error?: string;
};

const roles = ["merchant_admin", "store_manager", "store_staff", "support_agent", "viewer"];

function memberName(member: Member) {
  const user = member.user ?? {};
  return [user.first_name, user.last_name].filter(Boolean).join(" ") || String(user.email ?? "Team member");
}

function expiryCopy(value?: string) {
  if (!value) return "Expires in 7 days";
  const expires = new Date(value);
  if (Number.isNaN(expires.getTime())) return "Expires in 7 days";
  const days = Math.max(0, Math.ceil((expires.getTime() - Date.now()) / 86400000));
  if (days <= 0) return "Expired";
  if (days === 1) return "Expires in 1 day";
  return `Expires in ${days} days`;
}

function emailStatusLabel(invitation: Invitation) {
  if (invitation.email_status === "sent") return "Sent";
  if (invitation.email_status === "failed") return "Failed";
  if (invitation.email_status === "pending") return "Pending";
  return humanStatus(invitation.email_status || invitation.status);
}

export function TeamPage() {
  const [members, setMembers] = useState<Member[]>([]);
  const [invitations, setInvitations] = useState<Invitation[]>([]);
  const [stores, setStores] = useState<Store[]>([]);
  const [invite, setInvite] = useState({ email: "", first_name: "", last_name: "", role: "store_staff" });
  const [inviteStores, setInviteStores] = useState<string[]>([]);
  const [selectedMember, setSelectedMember] = useState("");
  const [assigned, setAssigned] = useState<string[]>([]);
  const [message, setMessage] = useState("");
  const [flash, setFlash] = useState("");
  const [sending, setSending] = useState(false);

  async function load() {
    try {
      const [memberResponse, storeResponse, invitationResponse] = await Promise.all([
        apiGet<Member[]>("/organizations/current/members"),
        apiGet<Store[]>("/stores"),
        apiGet<Invitation[]>("/organizations/current/invitations"),
      ]);
      setMembers(memberResponse.data);
      setStores(storeResponse.data);
      setInvitations(invitationResponse.data);
      setSelectedMember((current) => current || memberResponse.data[0]?.id || "");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load team");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  useEffect(() => {
    async function loadAssignments() {
      if (!selectedMember) return;
      const response = await apiGet<Row[]>(`/organizations/current/members/${selectedMember}/stores`);
      setAssigned(response.data.map((row) => String(row.store_id ?? (row.store as Row | undefined)?.id ?? row.id)));
    }
    void loadAssignments();
  }, [selectedMember]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setSending(true);
    setMessage("");
    setFlash("");
    const payload = { ...invite, store_ids: inviteStores };
    try {
      await apiPost<Invitation>("/organizations/current/invitations", payload);
      setFlash(`Invitation sent · ${invite.email} · ${roleLabel(invite.role)} · Expires in 7 days`);
      setInvite({ email: "", first_name: "", last_name: "", role: "store_staff" });
      setInviteStores([]);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Invitation could not be sent");
    } finally {
      setSending(false);
      await load();
    }
  }

  async function resend(invitation: Invitation) {
    setMessage("");
    setFlash("");
    try {
      await apiPost<Invitation>(`/organizations/current/invitations/${invitation.id}/resend`, {});
      setFlash(`Invitation sent · ${invitation.email} · ${roleLabel(String(invitation.role ?? ""))} · Expires in 7 days`);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Invitation could not be sent");
    } finally {
      await load();
    }
  }

  async function updateMember(id: string, payload: Row) {
    await apiPatch<Row>(`/organizations/current/members/${id}`, payload);
    await load();
  }

  async function saveAccess() {
    await apiPut<Row[]>(`/organizations/current/members/${selectedMember}/stores`, { store_ids: assigned });
    setFlash("Store access updated.");
  }

  function toggle(list: string[], id: string) {
    return list.includes(id) ? list.filter((item) => item !== id) : [...list, id];
  }

  const pendingInvites = invitations.filter((invitation) => invitation.status === "pending");

  return (
    <Page title="People" description="Invite staff and decide which stores they can work in." help="Owners see everything. Managers and staff only see stores you assign.">
      <Flash message={message} />
      <Flash message={flash} tone="success" />
      <FormGrid onSubmit={submit} title="Invite someone">
        <label>First name<input value={invite.first_name} onChange={(event) => setInvite({ ...invite, first_name: event.target.value })} required /></label>
        <label>Last name<input value={invite.last_name} onChange={(event) => setInvite({ ...invite, last_name: event.target.value })} /></label>
        <label>Email<input type="email" value={invite.email} onChange={(event) => setInvite({ ...invite, email: event.target.value })} required /></label>
        <label>Role
          <select value={invite.role} onChange={(event) => setInvite({ ...invite, role: event.target.value })}>
            {roles.map((role) => <option key={role} value={role}>{roleLabel(role)}</option>)}
          </select>
        </label>
        <div className="full">
          <p className="help-text">Store access (for managers and staff)</p>
          {stores.length === 0 ? <p className="muted">Add a store first.</p> : stores.map((store) => (
            <label key={store.id}>
              <input type="checkbox" checked={inviteStores.includes(store.id)} onChange={() => setInviteStores(toggle(inviteStores, store.id))} />
              {store.name}
            </label>
          ))}
        </div>
        <div className="full"><button type="submit" disabled={sending}>{sending ? "Sending..." : "Send invitation"}</button></div>
      </FormGrid>
      <Card>
        <h3>Pending invitations</h3>
        {pendingInvites.length === 0 ? (
          <EmptyState title="No pending invitations" body="Invited people appear here until they accept." />
        ) : (
          <div className="table-wrap" style={{ border: 0, margin: 0 }}>
            <table>
              <thead>
                <tr>
                  <th>Email</th>
                  <th>Role</th>
                  <th>Email status</th>
                  <th>Expiry</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {pendingInvites.map((invitation) => (
                  <tr key={invitation.id}>
                    <td>{invitation.email}</td>
                    <td>{roleLabel(String(invitation.role ?? ""))}</td>
                    <td>
                      <Badge tone={invitation.email_status === "sent" ? "success" : invitation.email_status === "failed" ? "warning" : "neutral"}>{emailStatusLabel(invitation)}</Badge>
                      {invitation.email_status === "failed" && invitation.email_error ? <p className="muted">{invitation.email_error}</p> : null}
                    </td>
                    <td>{expiryCopy(invitation.expires_at)}</td>
                    <td><button type="button" onClick={() => void resend(invitation)}>Resend invitation</button></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
      <Card>
        {members.length === 0 ? (
          <EmptyState title="No team members" body="Invite the people who take orders or handle support." />
        ) : (
          <div className="table-wrap" style={{ border: 0, margin: 0 }}>
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Email</th>
                  <th>Role</th>
                  <th>Status</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {members.map((member) => (
                  <tr key={member.id} className={selectedMember === member.id ? "selected-row click-row" : "click-row"} onClick={() => setSelectedMember(member.id)}>
                    <td>{memberName(member)}</td>
                    <td>{String(member.user?.email ?? "")}</td>
                    <td>
                      <select value={String(member.role ?? "viewer")} onChange={(event) => void updateMember(member.id, { role: event.target.value })}>
                        {roles.map((role) => <option key={role} value={role}>{roleLabel(role)}</option>)}
                      </select>
                    </td>
                    <td><Badge tone={member.status === "active" ? "success" : "warning"}>{humanStatus(member.status)}</Badge></td>
                    <td>
                      {member.status === "disabled" ? (
                        <button type="button" onClick={() => void updateMember(member.id, { status: "active" })}>Reactivate</button>
                      ) : (
                        <button type="button" className="danger" onClick={() => window.confirm("Deactivate this person?") && void updateMember(member.id, { status: "disabled" })}>Deactivate</button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>
      <Card>
        <h3>Store access</h3>
        <p className="muted">Select a person above, then choose the stores they can work in.</p>
        {stores.map((store) => (
          <label key={store.id}>
            <input type="checkbox" checked={assigned.includes(store.id)} onChange={() => setAssigned(toggle(assigned, store.id))} />
            {store.name}
          </label>
        ))}
        <div className="page-actions" style={{ marginTop: 12 }}>
          <button type="button" className="primary" onClick={() => void saveAccess()}>Save access</button>
        </div>
      </Card>
    </Page>
  );
}
