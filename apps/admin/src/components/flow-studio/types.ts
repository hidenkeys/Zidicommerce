import type { Point } from "../../pages/JourneyCanvas";
export type Option = { id: string; label: string; value: string; next?: string };
export type Step = { position?: Point; key: string; kind: string; title: string; text: string; help: string; required: boolean; locked: boolean; answer_type: string; presentation: string; destination?: string; source: string; options: Option[]; min?: number; max?: number; max_length: number; retry_message: string; max_retries: number; next: string; failure: string; condition_field?: string; condition_operator?: string; condition_value?: string; visible_when?: string; analytics_label: string };
export type Definition = { entry_cta?: string; name: string; purpose: string; category: string; mode: string; channel: string; connection_id?: string; trigger: string; start: string; ttl_minutes: number; steps: Step[] };
export type Version = { id: string; version_number: number; status: string; published_at?: string };
export type Journey = { id: string; name: string; description: string; updated_at?: string; published_version_id?: string; status: string };
export type Deployment = { status: string; revision: number; errors: string; warnings: string; preview_url?: string; provider_flow_id: string };
export type Detail = { channel_default?: {journey_id:string; version_id:string; mode:string}; journey: Journey; version: Version; definition: Definition; versions: Version[]; binding?: { paused: boolean; mode: string; version_id: string }; deployment?: Deployment };
export type Issue = { path: string; message: string };
export type Validation = { valid: boolean; issues: Issue[] };
export type Compilation = { json: Record<string, unknown>; errors: Issue[]; warnings: Issue[] };
export type Message = { type: string; text: string; options?: { id: string; label: string }[]; media_url?: string; location_request?: boolean; metadata?: { answer_type?: string; help?: string; [key: string]: unknown } };
export type Preview = { variables?: Record<string, unknown>; session_id: string; current_step: string; turn: string; flow_screen: string; controlled_test?: boolean; result: { status: string; messages: Message[]; handoff?: boolean }; cart: { name: string; quantity: number; unit_price_minor: number; currency: string }[] };
export type Library = { kind: string; name: string; answer_type?: string };
export type Connection = { id: string; provider: string; display_name: string; status: string };
export type Template = { id: string; name: string; description: string; mode?: string };
export type PreviewLine = { from: "customer" | "bot"; message: Message };

