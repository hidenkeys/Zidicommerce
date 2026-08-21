import { FormEvent, useEffect, useState } from "react";
import { apiGet, apiPost } from "../api/client";
import { Card, EmptyState, Flash, FormGrid, Page, StatusDot } from "../components/ui";
import { parseJSON, type Row } from "../lib/format";

type PaymentConfiguration = Row & { id: string; provider: string; display_name: string; status: string; enabled: boolean; public_config?: string; secret_source?: string; has_secret?: boolean };

export function PaymentsPage() {
  const [rows, setRows] = useState<PaymentConfiguration[]>([]);
  const [org, setOrg] = useState<Row | null>(null);
  const [advanced, setAdvanced] = useState(false);
  const [form, setForm] = useState({ public_key: "", secret_key: "", mode: "test" });
  const [message, setMessage] = useState("");
  const [flash, setFlash] = useState("");

  async function load() {
    try {
      const [paymentResponse, orgResponse] = await Promise.all([
        apiGet<PaymentConfiguration[]>("/payment-configurations"),
        apiGet<Row>("/organizations/current"),
      ]);
      setRows(paymentResponse.data);
      setOrg(orgResponse.data);
      setMessage("");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not load payments");
    }
  }

  useEffect(() => {
    void load();
  }, []);

  const paystack = rows.find((row) => row.provider === "paystack");
  const connected = Boolean(paystack?.enabled && paystack.status === "active");
  const publicConfig = parseJSON<{ mode?: string; public_key?: string }>(paystack?.public_config, {});

  async function connect(event: FormEvent) {
    event.preventDefault();
    await apiPost<PaymentConfiguration>("/payment-configurations", {
      provider: "paystack",
      display_name: "Paystack",
      enabled: true,
      status: "active",
      secret_source: form.secret_key ? "merchant_secret" : (paystack?.secret_source || "environment"),
      public_config: JSON.stringify({ mode: form.mode, public_key: form.public_key || publicConfig.public_key || "" }),
      secret_config: form.secret_key ? JSON.stringify({ secret_key: form.secret_key }) : "{}",
    });
    setForm({ public_key: "", secret_key: "", mode: form.mode });
    setFlash("Paystack saved. Secret keys are never shown again.");
    await load();
  }

  async function test() {
    const response = await apiPost<PaymentConfiguration>("/payment-configurations/paystack/test", {});
    setFlash(response.data.status === "active" || response.data.enabled ? "Paystack can be used." : "Paystack still needs a usable key.");
    await load();
  }

  return (
    <Page
      title="Payments"
      description="How customers pay for orders."
      help="ZidiCommerce never displays secret keys. If a key was set in the server environment, you do not need to paste it here."
    >
      <Flash message={message} />
      <Flash message={flash} tone="success" />
      <Card>
        {paystack ? (
          <>
            <p><StatusDot live={connected} label={connected ? "Connected" : "Not ready"} /></p>
            <p><strong>Paystack</strong></p>
            <p>Currency {String(org?.currency ?? "NGN")}</p>
            <p className="muted">{publicConfig.mode === "live" ? "Live mode" : "Test mode"}{paystack.has_secret ? " · Merchant key stored" : paystack.secret_source === "environment" ? " · Using platform key" : ""}</p>
            <div className="page-actions" style={{ marginTop: 16 }}>
              <button type="button" onClick={() => void test()}>Run test</button>
            </div>
          </>
        ) : (
          <EmptyState title="Payments are not connected yet" body="Connect Paystack so customers can pay after they place an order." />
        )}
      </Card>
      <FormGrid onSubmit={connect} title={paystack ? "Update Paystack" : "Connect Paystack"}>
        <label>Mode
          <select value={form.mode} onChange={(event) => setForm({ ...form, mode: event.target.value })}>
            <option value="test">Test</option>
            <option value="live">Live</option>
          </select>
        </label>
        <div className="full"><button type="button" className="ghost" onClick={() => setAdvanced(!advanced)}>{advanced ? "Hide keys" : "Enter keys"}</button></div>
        {advanced ? (
          <>
            <label>Public key<input value={form.public_key} onChange={(event) => setForm({ ...form, public_key: event.target.value })} placeholder="Optional" /></label>
            <label>Secret key<input type="password" value={form.secret_key} onChange={(event) => setForm({ ...form, secret_key: event.target.value })} placeholder="Leave blank to keep current" /></label>
          </>
        ) : <p className="help-text full">Most merchants can leave keys blank if the platform already has a Paystack key.</p>}
        <div className="full"><button type="submit">{paystack ? "Save" : "Connect Paystack"}</button></div>
      </FormGrid>
    </Page>
  );
}
