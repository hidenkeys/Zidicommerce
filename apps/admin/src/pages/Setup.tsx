import { useEffect, useState } from "react";
import { apiGet } from "../api/client";
import { Card, Flash, Page, SetupItem } from "../components/ui";
import { setupHref } from "../lib/format";

type SetupStatus = { complete_count: number; total_count: number; ready: boolean; items: { key: string; label: string; complete: boolean; description: string }[] };

const merchantOrder = ["stores", "catalogue", "inventory", "payments", "whatsapp", "bot", "faqs"];
const labels: Record<string, { title: string; copy: string }> = {
  stores: { title: "Store", copy: "Add the location customers order from." },
  catalogue: { title: "Catalogue", copy: "Add products customers can buy." },
  inventory: { title: "Inventory", copy: "Set how many of each product you have." },
  payments: { title: "Payments", copy: "Connect Paystack so customers can pay." },
  whatsapp: { title: "WhatsApp", copy: "Connect the number customers will message." },
  bot: { title: "Assistant", copy: "Publish the assistant that answers on WhatsApp." },
  faqs: { title: "Knowledge", copy: "Add a few answers to common questions." },
};

export function SetupPage() {
  const [setupStatus, setSetupStatus] = useState<SetupStatus | null>(null);
  const [message, setMessage] = useState("");

  useEffect(() => {
    async function load() {
      try {
        const response = await apiGet<SetupStatus>("/bot-setup/status");
        setSetupStatus(response.data);
      } catch (error) {
        setMessage(error instanceof Error ? error.message : "Could not load setup");
      }
    }
    void load();
  }, []);

  const items = merchantOrder
    .map((key) => setupStatus?.items.find((item) => item.key === key))
    .filter((item): item is NonNullable<typeof item> => Boolean(item));
  const done = items.filter((item) => item.complete).length;
  const ready = items.length > 0 && done === items.length;

  return (
    <Page
      title={ready ? "Your store is ready" : "Set up your store"}
      description={ready ? "Business, catalogue, WhatsApp, payments, and assistant are in place." : "Complete these steps in order. You can skip around, but this is the usual path."}
      help="This uses the same checks as going live. Worker and outbound items stay in Advanced if you need them."
    >
      <Flash message={message} />
      <Card>
        <p className="muted">{done} of {items.length || 7} complete</p>
        <div className="progress-bar"><span style={{ width: `${items.length ? (done / items.length) * 100 : 0}%` }} /></div>
        <div className="setup-list">
          {items.map((item) => {
            const copy = labels[item.key];
            return (
              <SetupItem
                key={item.key}
                complete={item.complete}
                label={copy?.title ?? item.label}
                to={setupHref(item.key)}
                description={item.complete ? "Done" : copy?.copy ?? item.description}
              />
            );
          })}
        </div>
      </Card>
    </Page>
  );
}
