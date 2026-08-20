import { Route, Routes } from "react-router-dom";
import { API_BASE_URL } from "./api/client";
import { Shell } from "./components/Shell";

const cards = [
  ["Organizations", "Tenant records, statuses, and merchant ownership."],
  ["Commerce", "Stores, catalogue, inventory, customers, orders, payments, and fulfilment."],
  ["Automation", "Future conversations and channel operations."],
  ["Bot Builder", "Future configurable modules, questions, responses, variables, actions, conditions, and publishing."],
];

function Dashboard() {
  return (
    <section className="content">
      <div className="section-heading">
        <h2>Operating foundation</h2>
        <p>The first phase establishes the shell and boundaries. Feature screens will be implemented in later phases.</p>
      </div>
      <div className="grid">
        {cards.map(([title, body]) => (
          <article className="summary-card" key={title}>
            <span>{title}</span>
            <p>{body}</p>
          </article>
        ))}
      </div>
      <div className="api-note">
        <strong>API base</strong>
        <code>{API_BASE_URL}</code>
      </div>
    </section>
  );
}

function Placeholder({ title }: { title: string }) {
  return (
    <section className="content">
      <div className="section-heading">
        <h2>{title}</h2>
        <p>This route is wired for Phase 1. Domain implementation belongs to a later phase.</p>
      </div>
      <div className="empty-state">
        <strong>Boundary established</strong>
        <span>No production workflow has been added here yet.</span>
      </div>
    </section>
  );
}

export default function App() {
  return (
    <Routes>
      <Route element={<Shell />}>
        <Route index element={<Dashboard />} />
        <Route path="*" element={<Placeholder title="Planned module" />} />
      </Route>
    </Routes>
  );
}

