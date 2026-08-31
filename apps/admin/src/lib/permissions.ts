export type Permission =
  | "organization.view" | "organization.update"
  | "staff.view" | "staff.invite" | "staff.update" | "staff.remove"
  | "stores.view" | "stores.create" | "stores.update" | "stores.delete"
  | "catalogue.view" | "catalogue.manage"
  | "inventory.view" | "inventory.adjust"
  | "customers.view" | "customers.manage"
  | "orders.view" | "orders.manage" | "orders.cancel"
  | "payments.view" | "payments.manage" | "payments.reconcile"
  | "conversations.view" | "conversations.manage"
  | "complaints.view" | "complaints.manage"
  | "bot.view" | "bot.manage" | "bot.publish"
  | "ai.use" | "ai.configure" | "ai.execute"
  | "knowledge.view" | "knowledge.manage"
  | "audit.view"
  | "settings.view" | "settings.manage"
  | "channels.view" | "channels.manage"
  | "billing.view" | "billing.manage";

const allPermissions: Permission[] = [
  "organization.view", "organization.update",
  "staff.view", "staff.invite", "staff.update", "staff.remove",
  "stores.view", "stores.create", "stores.update", "stores.delete",
  "catalogue.view", "catalogue.manage",
  "inventory.view", "inventory.adjust",
  "customers.view", "customers.manage",
  "orders.view", "orders.manage", "orders.cancel",
  "payments.view", "payments.manage", "payments.reconcile",
  "conversations.view", "conversations.manage",
  "complaints.view", "complaints.manage",
  "bot.view", "bot.manage", "bot.publish",
  "ai.use", "ai.configure", "ai.execute",
  "knowledge.view", "knowledge.manage",
  "audit.view",
  "settings.view", "settings.manage",
  "channels.view", "channels.manage",
  "billing.view", "billing.manage",
];

const rolePermissions: Record<string, Permission[]> = {
  platform_admin: allPermissions,
  merchant_admin: allPermissions,
  store_manager: [
    "organization.view", "staff.view",
    "stores.view", "stores.update",
    "catalogue.view", "catalogue.manage",
    "inventory.view", "inventory.adjust",
    "customers.view",
    "orders.view", "orders.manage", "orders.cancel",
    "conversations.view", "conversations.manage",
    "complaints.view", "complaints.manage",
    "bot.view", "ai.use", "ai.execute", "knowledge.view", "settings.view",
  ],
  store_staff: [
    "organization.view", "stores.view", "catalogue.view", "inventory.view", "customers.view",
    "orders.view", "orders.manage", "orders.cancel", "conversations.view", "complaints.view", "ai.use",
  ],
  support_agent: [
    "organization.view", "customers.view", "customers.manage", "orders.view",
    "conversations.view", "conversations.manage", "complaints.view", "complaints.manage", "ai.use", "knowledge.view",
  ],
  viewer: [
    "organization.view", "staff.view", "stores.view", "catalogue.view", "inventory.view", "customers.view",
    "orders.view", "payments.view", "conversations.view", "complaints.view", "bot.view", "knowledge.view",
    "audit.view", "settings.view", "channels.view", "billing.view",
  ],
};

export function hasPermission(role: string, permission: Permission) {
  return rolePermissions[role]?.includes(permission) ?? false;
}

export function isOrganizationAdmin(role: string) {
  return role === "merchant_admin" || role === "platform_admin";
}
