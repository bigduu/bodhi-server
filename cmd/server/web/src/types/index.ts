export interface AuthResponse {
  access_token: string;
  refresh_token: string;
  expires_in: number;
  user: User;
}

export interface User {
  id: string;
  username: string;
  email?: string;
  is_admin: boolean;
  created_at: string;
}

export interface UserListItem {
  id: string;
  username: string;
  email: string;
  is_active: boolean;
  is_admin: boolean;
  api_key_count: number;
  cred_count: number;
  last_login_at: string | null;
  created_at: string;
}

export interface APIKey {
  id: string;
  name: string;
  key_prefix: string;
  key_suffix: string;
  is_active: boolean;
  expires_at: string | null;
  last_used_at: string | null;
  created_at: string;
}

export interface Credential {
  provider: string;
  base_url: string;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface UsageSummary {
  total_requests: number;
  total_tokens: number;
  avg_duration_ms: number;
  active_users: number;
  error_count: number;
}

export interface DailyUsage {
  date: string;
  requests: number;
  tokens: number;
}

export interface ProviderUsage {
  provider: string;
  requests: number;
  tokens: number;
}

export interface UserUsage {
  username: string;
  requests: number;
  tokens: number;
}

export interface UsageDetail {
  id: string;
  provider: string;
  model: string;
  endpoint: string;
  tokens: number;
  duration_ms: number;
  status_code: number;
  error_message: string;
  created_at: string;
}

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
}

export interface DashboardData {
  summary: UsageSummary;
  daily: DailyUsage[];
  by_provider: ProviderUsage[];
  by_user: UserUsage[];
}

export interface Quota {
  RPM: number;
  RPD: number;
  TokenDaily: number;
  TokenMonthly: number;
  SpendDaily: number;
  SpendMonthly: number;
  AllowedModels: string[];
}

export interface Counter {
  MinuteRequests: number;
  DayRequests: number;
  DayTokens: number;
  MonthTokens: number;
  DaySpend: number;
  MonthSpend: number;
}

export interface QuotaData {
  quota: Quota;
  counter: Counter;
}

export interface LatencyStats {
  p50: number;
  p95: number;
  p99: number;
}

export interface ModelTokenUsage {
  provider: string;
  model: string;
  input_tokens: number;
  output_tokens: number;
  cost_cents: number;
  requests: number;
}

export interface ErrorEntry {
  id: string;
  username: string;
  provider: string;
  model: string;
  status_code: number;
  error_message: string;
  created_at: string;
}

export interface PricingEntry {
  provider: string;
  model_pattern: string;
  input_per_1m_cents: number;
  output_per_1m_cents: number;
  updated_at: string;
}

export interface ModelEntry {
  id: string;
  name: string;
  display_name: string;
  provider: string;
  is_active: boolean;
  is_featured: boolean;
  context_window: number;
  max_output: number;
  capabilities: string[];
  sort_order: number;
  created_at: string;
  updated_at: string;
}

export interface InviteCode {
  code: string;
  created_by: string;
  max_uses: number;
  used_count: number;
  expires_at: string | null;
  created_at: string;
}

export interface BillingReport {
  user_id: string;
  period_start: string;
  period_end: string;
  total_requests: number;
  total_input_tokens: number;
  total_output_tokens: number;
  total_cost_cents: number;
}

export interface BillingByModel {
  provider: string;
  model: string;
  requests: number;
  input_tokens: number;
  output_tokens: number;
  cost_cents: number;
}

export interface BillingCurrent {
  current: BillingReport;
  by_model: BillingByModel[];
  balance_cents: number;
}

export interface Group {
  id: string;
  name: string;
  description: string;
  created_by: string;
  created_at: string;
}

export interface GroupMember {
  group_id: string;
  user_id: string;
  username: string;
  role: string;
  joined_at: string;
}

export interface GroupCredential {
  id: string;
  group_id: string;
  provider: string;
  base_url: string;
  is_active: boolean;
  created_at: string;
}

export interface GroupQuota {
  group_id: string;
  rpm_limit: number;
  rpd_limit: number;
  token_daily: number;
  token_monthly: number;
  spend_daily: number;
  spend_monthly: number;
  allowed_models: string[];
}

export interface ProviderInstance {
  id: string;
  model_id: string;
  provider_type: string;
  instance_name: string;
  priority: number;
  base_url: string;
  is_active: boolean;
  health_status: string;
  last_check_at: string;
  created_at: string;
}
