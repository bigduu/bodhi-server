import type { AuthResponse, DashboardData, PaginatedResponse, UserListItem, APIKey, Credential, UsageDetail, QuotaData, LatencyStats, ModelTokenUsage, ErrorEntry, PricingEntry, ModelEntry, InviteCode, ProviderInstance, BillingCurrent, BillingReport, Group, GroupMember, GroupCredential, GroupQuota } from '../types';

const API_BASE = '/api/v1';

class ApiClient {
  private token: string | null;

  constructor() {
    this.token = localStorage.getItem('access_token');
  }

  setToken(token: string | null) {
    this.token = token;
    if (token) {
      localStorage.setItem('access_token', token);
    } else {
      localStorage.removeItem('access_token');
      localStorage.removeItem('refresh_token');
    }
  }

  private async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...(options.headers as Record<string, string> || {}),
    };
    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`;
    }

    const res = await fetch(`${API_BASE}${path}`, { ...options, headers });
    if (!res.ok) {
      if (res.status === 401) {
        this.setToken(null);
        window.location.href = '/login';
        throw new Error('unauthorized');
      }
      const body = await res.json().catch(() => ({ error: res.statusText }));
      throw new Error(body.error || `HTTP ${res.status}`);
    }
    if (res.status === 204) return undefined as T;
    return res.json();
  }

  // Auth
  login(username: string, password: string): Promise<AuthResponse> {
    return this.request('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password }),
    });
  }

  register(username: string, password: string, email?: string, inviteCode?: string): Promise<AuthResponse> {
    return this.request('/auth/register', {
      method: 'POST',
      body: JSON.stringify({ username, password, email, invite_code: inviteCode }),
    });
  }

  me(): Promise<{ id: string; username: string; is_admin: boolean }> {
    return this.request('/auth/me');
  }

  // Admin - Users
  listUsers(page = 1, pageSize = 20): Promise<PaginatedResponse<UserListItem>> {
    return this.request(`/admin/users?page=${page}&page_size=${pageSize}`);
  }

  updateUser(id: string, data: { is_active?: boolean; is_admin?: boolean }): Promise<void> {
    return this.request(`/admin/users/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  deleteUser(id: string): Promise<void> {
    return this.request(`/admin/users/${id}`, { method: 'DELETE' });
  }

  // Admin - User Keys
  listUserKeys(userId: string): Promise<APIKey[]> {
    return this.request(`/admin/users/${userId}/keys`);
  }

  deleteUserKey(userId: string, keyId: string): Promise<void> {
    return this.request(`/admin/users/${userId}/keys/${keyId}`, { method: 'DELETE' });
  }

  // Admin - User Credentials
  listUserCredentials(userId: string): Promise<Credential[]> {
    return this.request(`/admin/users/${userId}/credentials`);
  }

  deleteUserCredential(userId: string, provider: string): Promise<void> {
    return this.request(`/admin/users/${userId}/credentials/${provider}`, { method: 'DELETE' });
  }

  // Admin - Usage
  usageSummary(days = 7): Promise<DashboardData> {
    return this.request(`/admin/usage/summary?days=${days}`);
  }

  userUsage(userId: string, page = 1, pageSize = 20): Promise<PaginatedResponse<UsageDetail>> {
    return this.request(`/admin/usage/users/${userId}?page=${page}&page_size=${pageSize}`);
  }

  // Admin - Quota
  getQuota(userId: string): Promise<QuotaData> {
    return this.request(`/admin/users/${userId}/quota`);
  }

  setQuota(userId: string, quota: QuotaData['quota']): Promise<{ status: string }> {
    return this.request(`/admin/users/${userId}/quota`, {
      method: 'PUT',
      body: JSON.stringify(quota),
    });
  }

  // Admin - Metrics
  latencyStats(days = 7, provider = ''): Promise<LatencyStats> {
    const params = new URLSearchParams({ days: String(days) });
    if (provider) params.set('provider', provider);
    return this.request(`/admin/metrics/latency?${params}`);
  }

  tokenUsage(days = 7): Promise<ModelTokenUsage[]> {
    return this.request(`/admin/metrics/tokens?days=${days}`);
  }

  errorLogs(days = 7, page = 1, pageSize = 20): Promise<PaginatedResponse<ErrorEntry>> {
    return this.request(`/admin/metrics/errors?days=${days}&page=${page}&page_size=${pageSize}`);
  }

  // Admin - Pricing
  listPricing(): Promise<PricingEntry[]> {
    return this.request('/admin/pricing');
  }

  updatePricing(data: Omit<PricingEntry, 'updated_at'>): Promise<{ status: string }> {
    return this.request('/admin/pricing', {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  // Admin - Settings
  listSettings(): Promise<Record<string, string>> {
    return this.request('/admin/settings');
  }

  updateSetting(key: string, value: string): Promise<{ status: string }> {
    return this.request(`/admin/settings/${key}`, {
      method: 'PUT',
      body: JSON.stringify({ value }),
    });
  }

  // Admin - Invites
  createInvite(maxUses = 1, expiresAt?: string): Promise<{ code: string }> {
    return this.request('/admin/invites', {
      method: 'POST',
      body: JSON.stringify({ max_uses: maxUses, expires_at: expiresAt }),
    });
  }

  listInvites(): Promise<InviteCode[]> {
    return this.request('/admin/invites');
  }

  deleteInvite(code: string): Promise<void> {
    return this.request(`/admin/invites/${code}`, { method: 'DELETE' });
  }

  // Admin - Models
  listModels(): Promise<ModelEntry[]> {
    return this.request('/admin/models');
  }

  createModel(data: Omit<ModelEntry, 'id' | 'created_at' | 'updated_at'>): Promise<ModelEntry> {
    return this.request('/admin/models', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  updateModel(id: string, data: Partial<ModelEntry>): Promise<{ status: string }> {
    return this.request(`/admin/models/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  deleteModel(id: string): Promise<void> {
    return this.request(`/admin/models/${id}`, { method: 'DELETE' });
  }

  // Admin - Provider Instances
  listInstances(): Promise<ProviderInstance[]> {
    return this.request('/admin/instances');
  }

  createInstance(data: Partial<ProviderInstance>): Promise<ProviderInstance> {
    return this.request('/admin/instances', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  updateInstance(id: string, data: Partial<ProviderInstance>): Promise<{ status: string }> {
    return this.request(`/admin/instances/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  deleteInstance(id: string): Promise<void> {
    return this.request(`/admin/instances/${id}`, { method: 'DELETE' });
  }

  // Billing
  billingCurrent(): Promise<BillingCurrent> {
    return this.request('/billing/current');
  }

  billingReports(): Promise<BillingReport[]> {
    return this.request('/billing/reports');
  }

  billingMonthly(year: number, month: number): Promise<BillingReport> {
    return this.request(`/billing/reports/${year}/${month}`);
  }

  billingCSV(year: number, month: number): Promise<void> {
    return this.requestRaw(`/billing/reports/${year}/${month}/csv`).then(() => {});
  }

  requestRaw(path: string): Promise<Response> {
    const headers: Record<string, string> = {};
    if (this.token) headers['Authorization'] = `Bearer ${this.token}`;
    return fetch(`${API_BASE}${path}`, { headers });
  }

  addBalance(userId: string, amountCents: number): Promise<{ status: string }> {
    return this.request('/admin/billing/balance', {
      method: 'POST',
      body: JSON.stringify({ user_id: userId, amount_cents: amountCents }),
    });
  }

  // Groups
  createGroup(name: string, description: string): Promise<Group> {
    return this.request('/admin/groups', {
      method: 'POST',
      body: JSON.stringify({ name, description }),
    });
  }

  listGroups(): Promise<Group[]> {
    return this.request('/admin/groups');
  }

  getGroup(id: string): Promise<Group> {
    return this.request(`/admin/groups/${id}`);
  }

  updateGroup(id: string, data: { name?: string; description?: string }): Promise<{ status: string }> {
    return this.request(`/admin/groups/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  }

  deleteGroup(id: string): Promise<void> {
    return this.request(`/admin/groups/${id}`, { method: 'DELETE' });
  }

  listGroupMembers(groupId: string): Promise<GroupMember[]> {
    return this.request(`/admin/groups/${groupId}/members`);
  }

  addGroupMember(groupId: string, userId: string, role = 'member'): Promise<{ status: string }> {
    return this.request(`/admin/groups/${groupId}/members`, {
      method: 'POST',
      body: JSON.stringify({ user_id: userId, role }),
    });
  }

  removeGroupMember(groupId: string, userId: string): Promise<void> {
    return this.request(`/admin/groups/${groupId}/members/${userId}`, { method: 'DELETE' });
  }

  listGroupCredentials(groupId: string): Promise<GroupCredential[]> {
    return this.request(`/admin/groups/${groupId}/credentials`);
  }

  setGroupCredential(groupId: string, provider: string, apiKey: string, baseURL?: string): Promise<{ status: string }> {
    return this.request(`/admin/groups/${groupId}/credentials/${provider}`, {
      method: 'PUT',
      body: JSON.stringify({ api_key: apiKey, base_url: baseURL }),
    });
  }

  deleteGroupCredential(groupId: string, provider: string): Promise<void> {
    return this.request(`/admin/groups/${groupId}/credentials/${provider}`, { method: 'DELETE' });
  }

  getGroupQuota(groupId: string): Promise<GroupQuota> {
    return this.request(`/admin/groups/${groupId}/quota`);
  }

  setGroupQuota(groupId: string, quota: Omit<GroupQuota, 'group_id'>): Promise<{ status: string }> {
    return this.request(`/admin/groups/${groupId}/quota`, {
      method: 'PUT',
      body: JSON.stringify(quota),
    });
  }
}

export const api = new ApiClient();
