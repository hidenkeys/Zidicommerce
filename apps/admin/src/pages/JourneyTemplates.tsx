import { useEffect, useState } from "react";
import { Search, ShoppingBag, MessageCircle, GitBranch } from "lucide-react";
import { apiGet } from "../api/client";
import { Flash } from "../components/ui";
import "./journey-templates.css";

type Template = { id: string; name: string; description: string };
type Preview = { steps: { key: string; title: string; kind: string }[] };
const commerce = new Set([
  "order",
  "guided_order",
  "product_enquiry",
  "track_order",
  "catalogue",
  "store_locator",
  "returns_request",
]);
const support = new Set(["complaint", "support", "returns_request"]);
const needs: Record<string, string[]> = {
  order: ["Catalogue", "Inventory", "Paystack"],
  guided_order: ["Catalogue", "Inventory", "Paystack"],
  product_enquiry: ["Catalogue", "Grounded AI"],
  catalogue: ["Catalogue", "Inventory"],
  track_order: ["Orders"],
  store_locator: ["Stores"],
  support: ["Team inbox"],
  complaint: ["Team inbox"],
  returns_request: ["Team inbox"],
};

export function JourneyTemplates({
  templates,
  value,
  onChange,
}: {
  templates: Template[];
  value: string;
  onChange: (id: string) => void;
}) {
  const [search, setSearch] = useState(""),
    [industry, setIndustry] = useState(""),
    [useCase, setUseCase] = useState(""),
    [channel, setChannel] = useState("");
  const [preview, setPreview] = useState<Preview>(),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(false);
  const filtered = templates.filter(
    (t) =>
      (!search ||
        (t.name + " " + t.description)
          .toLowerCase()
          .includes(search.trim().toLowerCase())) &&
      (!industry ||
        (industry === "retail" ? commerce.has(t.id) : !commerce.has(t.id))) &&
      (!useCase ||
        (useCase === "support"
          ? support.has(t.id)
          : useCase === "shopping"
            ? commerce.has(t.id)
            : !support.has(t.id) && !commerce.has(t.id))) &&
      (!channel || channel === "whatsapp"),
  );
  useEffect(() => {
    let current = true;
    if (!value) return;
    setLoading(true);
    setPreview(undefined);
    setError("");
    apiGet<Preview>("/journeys/templates/" + encodeURIComponent(value))
      .then((r) => {
        if (current) setPreview(r.data);
      })
      .catch(() => {
        if (current) setError("Template preview could not be loaded.");
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    return () => {
      current = false;
    };
  }, [value]);
  return (
    <div className="template-browser">
      <label className="template-search">
        <Search size={16} />
        <input
          aria-label="Search templates"
          type="search"
          placeholder="Search templates"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
      </label>
      <div className="template-filters">
        <label>
          Industry
          <select
            value={industry}
            onChange={(e) => setIndustry(e.target.value)}
          >
            <option value="">All industries</option>
            <option value="retail">Retail &amp; food</option>
            <option value="general">General business</option>
          </select>
        </label>
        <label>
          Use case
          <select value={useCase} onChange={(e) => setUseCase(e.target.value)}>
            <option value="">All use cases</option>
            <option value="shopping">Shopping</option>
            <option value="support">Support</option>
            <option value="routing">Conversation design</option>
          </select>
        </label>
        <label>
          Channel
          <select value={channel} onChange={(e) => setChannel(e.target.value)}>
            <option value="">All channels</option>
            <option value="whatsapp">WhatsApp</option>
          </select>
        </label>
      </div>
      <div className="template-cards" aria-label="Templates">
        {filtered.map((t) => {
          const Icon = commerce.has(t.id)
            ? ShoppingBag
            : support.has(t.id)
              ? MessageCircle
              : GitBranch;
          return (
            <button
              type="button"
              key={t.id}
              aria-pressed={value === t.id}
              onClick={() => onChange(t.id)}
            >
              <Icon size={18} />
              <strong>{t.name}</strong>
              <span>{t.description}</span>
              <small>{(needs[t.id] ?? ["WhatsApp"]).join(" · ")}</small>
            </button>
          );
        })}
      </div>
      {!filtered.length && (
        <p role="status">No templates match these filters.</p>
      )}
      <section className="template-preview" aria-label="Template preview">
        <h3>{templates.find((t) => t.id === value)?.name}</h3>
        {loading && <p role="status">Loading preview…</p>}
        <Flash message={error} />
        {preview && (
          <ol>
            {preview.steps.map((step) => (
              <li key={step.key}>{step.title}</li>
            ))}
          </ol>
        )}
      </section>
    </div>
  );
}
