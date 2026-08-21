import { NavLink, Outlet } from "react-router-dom";
import { getStoredToken, setStoredToken } from "../api/client";

const groups = [
  {
    label: "Overview",
    items: [["Dashboard", "/"]],
  },
  {
    label: "Commerce",
    items: [
      ["Stores", "/commerce/stores"],
      ["Catalogue", "/commerce/catalogue"],
      ["Inventory", "/commerce/inventory"],
      ["Orders", "/commerce/orders"],
      ["Customers", "/commerce/customers"],
    ],
  },
  {
    label: "Organization",
    items: [
      ["Business", "/organization/business"],
      ["Team", "/organization/team"],
      ["Stores & Access", "/organization/access"],
      ["Audit logs", "/organization/audit-logs"],
    ],
  },
  {
    label: "Configuration",
    items: [
      ["Payments", "/configuration/payments"],
      ["Fulfilment", "/configuration/fulfilment"],
      ["Channels", "/configuration/channels"],
    ],
  },
  {
    label: "Automation",
    items: [
      ["Bots", "/automation/bots"],
      ["Bot Versions", "/automation/versions"],
      ["Conversations", "/automation/conversations"],
      ["Support Handoffs", "/automation/support-handoffs"],
    ],
  },
  {
    label: "Platform",
    items: [["Organizations", "/platform/organizations"]],
  },
  {
    label: "System",
    items: [
      ["Readiness", "/settings/readiness"],
      ["Merchant Import", "/settings/import"],
      ["Settings", "/settings"],
    ],
  },
];

export function Shell() {
  function saveToken() {
    const token = window.prompt("Paste a ZidiCommerce API access token", getStoredToken());
    if (token !== null) {
      setStoredToken(token);
    }
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark">ZC</div>
          <div>
            <strong>ZidiCommerce</strong>
            <span>Merchant operations</span>
          </div>
        </div>

        <nav className="nav-groups">
          {groups.map((group) => (
            <section key={group.label}>
              <p>{group.label}</p>
              {group.items.map(([label, path]) => (
                <NavLink key={path} to={path} end={path === "/"}>
                  {label}
                </NavLink>
              ))}
            </section>
          ))}
        </nav>
      </aside>

      <main className="main-panel">
        <header className="topbar">
          <div>
            <span className="eyebrow">Phase 9 Merchant Pilot</span>
            <h1>ZidiCommerce Admin</h1>
          </div>
          <button type="button" onClick={saveToken}>Connect API</button>
        </header>
        <Outlet />
      </main>
    </div>
  );
}
