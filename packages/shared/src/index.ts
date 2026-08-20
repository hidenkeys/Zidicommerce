export const commerceRoles = [
  "platform_admin",
  "merchant_admin",
  "store_manager",
  "store_staff",
  "support_agent",
  "viewer",
] as const;

export type CommerceRole = (typeof commerceRoles)[number];

export type ApiErrorResponse = {
  error: {
    code: string;
    message: string;
  };
};

export type ApiDataResponse<T> = {
  data: T;
};

