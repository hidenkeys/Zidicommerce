import { NavLink, Outlet } from "react-router-dom";

const groups = [
  {
    label: "Commerce",
    items: [
      ["Dashboard", "/"],
      ["Organizations", "/organizations"],
      ["Stores", "/commerce/stores"],
      ["Catalogue", "/commerce/catalogue"],
      ["Inventory", "/commerce/inventory"],
      ["Orders", "/commerce/orders"],
      ["Customers", "/commerce/customers"],
    ],
  },
  {
    label: "Automation",
    items: [
      ["Bots", "/automation/bots"],
      ["Conversations", "/automation/conversations"],
    ],
  },
  {
    label: "Bot Builder",
    items: [
      ["Overview", "/bot-builder"],
      ["Flow", "/bot-builder/flow"],
      ["Questions", "/bot-builder/questions"],
      ["Responses", "/bot-builder/responses"],
      ["Variables", "/bot-builder/variables"],
      ["Actions", "/bot-builder/actions"],
      ["Conditions", "/bot-builder/conditions"],
      ["Integrations", "/bot-builder/integrations"],
      ["Publish", "/bot-builder/publish"],
    ],
  },
  {
    label: "Operations",
    items: [
      ["Channels", "/channels"],
      ["Payments", "/payments"],
      ["Fulfilment", "/fulfilment"],
      ["Team", "/team"],
      ["Settings", "/settings"],
    ],
  },
];

export function Shell() {
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
            <span className="eyebrow">Phase 1 Foundation</span>
            <h1>ZidiCommerce Admin</h1>
          </div>
          <button type="button">Connect API</button>
        </header>
        <Outlet />
      </main>
    </div>
  );
}

