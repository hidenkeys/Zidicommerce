export type ReadinessItem = {
  key: string;
  label: string;
  complete: boolean;
  description: string;
  required?: boolean;
  group?: string;
};

export type SetupStatus = {
  complete_count: number;
  total_count: number;
  required_complete_count?: number;
  required_count?: number;
  ready: boolean;
  items: ReadinessItem[];
};

export const readinessOrder = ["organization", "stores", "catalogue", "inventory", "payments", "faqs", "bot", "team", "whatsapp"];

export const readinessSections = [
  { key: "business", title: "Business foundation", description: "The identity and locations used throughout Zidi." },
  { key: "selling", title: "Selling", description: "Products, stock, and payment collection." },
  { key: "customer_service", title: "Customer service", description: "What the assistant knows and is allowed to do." },
  { key: "operations", title: "Team", description: "Who handles store and customer work." },
  { key: "channels", title: "Customer channels", description: "Reserved for external customer channel connections." },
];

export function merchantReadinessItems(status: SetupStatus | null) {
  if (!status) return [];
  return readinessOrder
    .map((key) => status.items.find((item) => item.key === key))
    .filter((item): item is ReadinessItem => Boolean(item));
}
