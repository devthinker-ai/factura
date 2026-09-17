/** Token + unauthorized notifier for the TokenBar / login redirect. */
const TOKEN_KEY = 'factura_token'
const SESSION_KEY = 'factura_session'

type UnauthorizedListener = () => void
const unauthorizedListeners = new Set<UnauthorizedListener>()

export function onUnauthorized(fn: UnauthorizedListener): () => void {
  unauthorizedListeners.add(fn)
  return () => unauthorizedListeners.delete(fn)
}

function notifyUnauthorized() {
  unauthorizedListeners.forEach((fn) => fn())
}

/** Operator token (FACTURA_TOKEN paste). */
export function getToken(): string | null {
  try {
    return localStorage.getItem(TOKEN_KEY)
  } catch {
    return null
  }
}

export function setToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token)
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY)
}

/** Session JWT from login (preferred over operator token). */
export function getSession(): string | null {
  try {
    return localStorage.getItem(SESSION_KEY)
  } catch {
    return null
  }
}

export function setSession(token: string) {
  localStorage.setItem(SESSION_KEY, token)
}

export function clearSession() {
  localStorage.removeItem(SESSION_KEY)
}

/** Prefer session JWT; fall back to operator token. */
export function getAuthBearer(): string | null {
  return getSession() || getToken()
}

export class ApiError extends Error {
  status: number
  body: unknown
  headers: Headers
  constructor(status: number, body: unknown, headers?: Headers) {
    const msg =
      typeof body === 'object' && body && 'error' in body
        ? String((body as { error: unknown }).error)
        : `HTTP ${status}`
    super(msg)
    this.name = 'ApiError'
    this.status = status
    this.body = body
    this.headers = headers ?? new Headers()
  }
}

export function isForbidden(err: unknown): err is ApiError {
  return (
    err instanceof ApiError &&
    err.status === 401 &&
    typeof err.body === 'object' &&
    err.body != null &&
    (err.body as { error?: string }).error === 'forbidden'
  )
}

export function isUnauthorizedBody(err: unknown): boolean {
  return (
    err instanceof ApiError &&
    err.status === 401 &&
    typeof err.body === 'object' &&
    err.body != null &&
    (err.body as { error?: string }).error === 'unauthorized'
  )
}

export interface Violation {
  rule_id: string
  human: string
  value?: string
}

export interface Report {
  valid: boolean
  format?: string
  level?: 'schema' | 'business' | string
  violations?: Violation[]
  summary?: string
}

export interface Row {
  id: number
  external_id: string
  format: string
  status: 'valid' | 'invalid' | 'parse_error' | string
  vendor_name: string
  invoice_number: string
  buyer_name: string
  invoice_date: string
  total: number
  vat_amount: number
  currency: string
  report_json?: Report | string | null
  doc_gobl_json?: unknown
  created_at: string
  source?: string
  invoice_type?: string // BT-3: 380/381/384/389
}

export interface ListResponse {
  rows: Row[]
  total: number
}

export interface GenerateResponse {
  xrechnung: string
  zugferd_pdf_base64: string
  self_check: boolean
  filename_xrechnung: string
  filename_zugferd: string
}

export interface IngestResponse {
  id: number
  external_id: string
  status: string
  format: string
  report?: Report
  vendor_name?: string
  invoice_number?: string
}

export interface HealthResponse {
  status: string
  version?: string
  plan?: string
  auth?: 'open' | 'token' | 'users'
}

export type AuthMode = 'open' | 'token' | 'users'
export type UserRole = 'admin' | 'editor' | 'viewer'

export interface AuthUser {
  id: string
  email: string
  name: string
  role: UserRole
  totp_enabled: boolean
  totp_confirmed_at?: string
  created_at?: string
}

export interface AuthStatus {
  mode: AuthMode
}

export interface LoginResult {
  token?: string
  user?: AuthUser
  mfa_required?: boolean
  mfa_token?: string
}

export interface LicenseCaps {
  max_companies: number
  max_api_keys: number
  max_clients: number
  peppol: boolean
}

export interface LicenseInfo {
  plan: string
  licensed: boolean
  subject: string
  expires: string
  caps: LicenseCaps
  in_use: {
    companies: number
    api_keys: number
    invoices_this_month: number
  }
}

export interface Company {
  id: number
  name: string
  seller_vat_id?: string
  peppol_id?: string
  is_default: boolean
  created_at: string
  has_logo?: boolean
  branding?: CompanyBranding
}

export interface CompanyBranding {
  logo_b64?: string
  logo_mime?: string
  accent?: string
  header_text?: string
  footer_text?: string
}

export interface APIKeyRow {
  id: string
  name: string
  rpm: number
  monthly_invoices: number
  invoices_this_month: number
  enabled: boolean
  created_at: string
}

export async function getLicense(): Promise<LicenseInfo> {
  return fetchJson<LicenseInfo>('/license')
}

export async function postLicense(key: string): Promise<LicenseInfo> {
  return fetchJson<LicenseInfo>('/license', {
    method: 'POST',
    body: JSON.stringify({ key }),
  })
}

export async function deleteLicense(): Promise<void> {
  await fetchJson('/license', { method: 'DELETE' })
}

export async function listCompanies(): Promise<Company[]> {
  const res = await fetchJson<{ companies: Company[] }>('/companies')
  return res.companies ?? []
}

export async function addCompany(body: {
  name: string
  seller_vat_id?: string
  peppol_id?: string
}): Promise<Company> {
  return fetchJson<Company>('/companies', { method: 'POST', body: JSON.stringify(body) })
}

export async function setDefaultCompany(id: number): Promise<Company> {
  return fetchJson<Company>(`/companies/${id}`, { method: 'PATCH', body: '{}' })
}

export async function getCompany(id: number): Promise<Company> {
  return fetchJson<Company>(`/companies/${id}`)
}

export async function patchCompanyBranding(
  id: number,
  branding: CompanyBranding,
): Promise<Company> {
  return fetchJson<Company>(`/companies/${id}`, {
    method: 'PATCH',
    body: JSON.stringify({ branding }),
  })
}

export async function listAPIKeys(): Promise<APIKeyRow[]> {
  const res = await fetchJson<{ keys: APIKeyRow[] }>('/api-keys')
  return res.keys ?? []
}

export async function createAPIKey(body: {
  name: string
  rpm?: number
  monthly_invoices?: number
}): Promise<{ key: string; name: string; rpm: number; monthly_invoices: number }> {
  return fetchJson('/api-keys', { method: 'POST', body: JSON.stringify(body) })
}

export async function patchAPIKey(id: string, enabled: boolean): Promise<void> {
  await fetchJson(`/api-keys/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: JSON.stringify({ enabled }),
  })
}

export async function deleteAPIKey(id: string): Promise<void> {
  await fetchJson(`/api-keys/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export function isPlanLimit(err: unknown): err is ApiError {
  return (
    err instanceof ApiError &&
    err.status === 403 &&
    typeof err.body === 'object' &&
    err.body != null &&
    (err.body as { error?: string }).error === 'plan_limit'
  )
}

export function planLimitDetail(err: unknown): string {
  if (!isPlanLimit(err)) return ''
  const body = err.body as { detail?: string }
  return body.detail || err.message
}

export interface ListParams {
  q?: string
  status?: string
  vendor?: string
  since?: string
  until?: string
  limit?: number
  offset?: number
}

function authHeaders(): HeadersInit {
  const t = getAuthBearer()
  return t ? { Authorization: `Bearer ${t}` } : {}
}

function buildQuery(params: Record<string, string | number | undefined | null>): string {
  const sp = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === '') continue
    sp.set(k, String(v))
  }
  const s = sp.toString()
  return s ? `?${s}` : ''
}

export async function fetchJson<T = unknown>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  const hasBody = init?.body != null
  const isFormData = typeof FormData !== 'undefined' && init?.body instanceof FormData
  const isBlob =
    typeof Blob !== 'undefined' && init?.body instanceof Blob
  const isArrayBuffer = init?.body instanceof ArrayBuffer
  const isTyped =
    typeof ArrayBuffer !== 'undefined' &&
    ArrayBuffer.isView(init?.body as ArrayBufferView | undefined)
  const isRawBytes = isBlob || isArrayBuffer || isTyped || typeof init?.body === 'string'

  if (hasBody && !isFormData && !isRawBytes && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  // Force JSON content-type only for JSON string bodies that look like objects
  if (hasBody && typeof init?.body === 'string' && !headers.has('Content-Type')) {
    const trimmed = init.body.trimStart()
    if (trimmed.startsWith('{') || trimmed.startsWith('[')) {
      headers.set('Content-Type', 'application/json')
    }
  }

  const token = getAuthBearer()
  if (token && !headers.has('Authorization')) {
    headers.set('Authorization', `Bearer ${token}`)
  }

  const res = await fetch(path, { ...init, headers })
  if (!res.ok) {
    let body: unknown = null
    const ct = res.headers.get('content-type') || ''
    try {
      body = ct.includes('application/json') ? await res.json() : await res.text()
    } catch {
      body = null
    }
    const err = new ApiError(res.status, body, res.headers)
    // Only the standard unauthorized body triggers TokenBar / login redirect.
    // Role denial is {"error":"forbidden"} — surface as a normal error, never bounce.
    if (res.status === 401 && isUnauthorizedBody(err) && !getAuthBearer()) {
      notifyUnauthorized()
    } else if (res.status === 401 && isUnauthorizedBody(err) && getSession()) {
      // Session expired / revoked — clear and notify so AuthGate can redirect.
      clearSession()
      notifyUnauthorized()
    }
    throw err
  }

  if (res.status === 204) return undefined as T
  const ct = res.headers.get('content-type') || ''
  if (ct.includes('application/json')) {
    return (await res.json()) as T
  }
  return (await res.text()) as T
}

export async function health(): Promise<HealthResponse> {
  return fetchJson<HealthResponse>('/health')
}

export async function list(params: ListParams = {}): Promise<ListResponse> {
  const q = buildQuery({
    q: params.q,
    status: params.status,
    vendor: params.vendor,
    since: params.since,
    until: params.until,
    limit: params.limit,
    offset: params.offset,
  })
  return fetchJson<ListResponse>(`/invoices${q}`)
}

export async function get(id: number): Promise<Row> {
  return fetchJson<Row>(`/invoices/${id}`)
}

export interface EvidenceResponse {
  invoice_id: number
  hash: string
  prev_hash: string
  created_at: string
  verified: boolean
}

export interface AuditResponse {
  ok: boolean
  link_count: number
  head: string
  first_broken_id: number
}

export async function evidence(id: number): Promise<EvidenceResponse> {
  return fetchJson<EvidenceResponse>(`/invoices/${id}/evidence`)
}

export async function audit(): Promise<AuditResponse> {
  return fetchJson<AuditResponse>('/audit')
}

export async function vendors(limit = 100): Promise<string[]> {
  const res = await fetchJson<{ vendors: string[] }>(`/invoices/vendors${buildQuery({ limit })}`)
  return res.vendors ?? []
}

export async function exportCsv(params: ListParams = {}): Promise<Blob> {
  const q = buildQuery({
    q: params.q,
    status: params.status,
    vendor: params.vendor,
    since: params.since,
    until: params.until,
  })
  const headers = new Headers(authHeaders())
  const res = await fetch(`/invoices/export${q}`, { headers })
  if (!res.ok) {
    let body: unknown = null
    try {
      body = await res.json()
    } catch {
      body = await res.text()
    }
    if (res.status === 401 && !getAuthBearer()) notifyUnauthorized()
    throw new ApiError(res.status, body, res.headers)
  }
  return res.blob()
}

export async function ingest(
  bytes: ArrayBuffer | Blob | Uint8Array,
  opts: { externalId?: string; source?: string } = {},
): Promise<IngestResponse> {
  const headers = new Headers(authHeaders())
  if (opts.externalId) headers.set('X-Factura-External-ID', opts.externalId)
  if (opts.source) headers.set('X-Factura-Source', opts.source)
  // Do not force JSON content-type for raw invoice bytes.
  const body =
    bytes instanceof Uint8Array
      ? bytes
      : bytes instanceof ArrayBuffer
        ? bytes
        : bytes
  return fetchJson<IngestResponse>('/invoices', {
    method: 'POST',
    headers,
    body: body as BodyInit,
  })
}

export async function validate(bytes: ArrayBuffer | Blob): Promise<{ report: Report }> {
  const headers = new Headers(authHeaders())
  return fetchJson<{ report: Report }>('/invoices/validate', {
    method: 'POST',
    headers,
    body: bytes,
  })
}

export async function generate(
  goblJson: object,
  opts: { format?: 'xrechnung' | 'zugferd' | 'both'; companyId?: number } = {},
): Promise<GenerateResponse> {
  const format = opts.format ?? 'both'
  let url = `/invoices/generate?format=${format}`
  if (opts.companyId != null && opts.companyId > 0) {
    url += `&company_id=${opts.companyId}`
  }
  return fetchJson<GenerateResponse>(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...authHeaders() },
    body: JSON.stringify(goblJson),
  })
}

export async function fetchOriginalBlob(id: number): Promise<{ blob: Blob; filename: string }> {
  const headers = new Headers(authHeaders())
  const res = await fetch(`/invoices/${id}/original`, { headers })
  if (!res.ok) {
    let body: unknown = null
    try {
      body = await res.json()
    } catch {
      body = null
    }
    if (res.status === 401 && !getAuthBearer()) notifyUnauthorized()
    throw new ApiError(res.status, body, res.headers)
  }
  const cd = res.headers.get('Content-Disposition') || ''
  const m = /filename="([^"]+)"/.exec(cd)
  const filename = m?.[1] ?? `invoice-${id}-original`
  return { blob: await res.blob(), filename }
}

/* —— Auth (Phase 7) —— */

export async function authStatus(): Promise<AuthStatus> {
  return fetchJson<AuthStatus>('/auth-status')
}

export async function login(email: string, password: string): Promise<LoginResult> {
  return fetchJson<LoginResult>('/login', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  })
}

export async function loginMFA(mfaToken: string, code: string): Promise<LoginResult> {
  return fetchJson<LoginResult>('/login/mfa', {
    method: 'POST',
    body: JSON.stringify({ mfa_token: mfaToken, code }),
  })
}

export async function getMe(): Promise<AuthUser> {
  const res = await fetchJson<{ user: AuthUser }>('/me')
  return res.user
}

export async function listUsers(): Promise<AuthUser[]> {
  const res = await fetchJson<{ users: AuthUser[] }>('/users')
  return res.users ?? []
}

export async function createUser(body: {
  email: string
  name?: string
  role: UserRole
  password: string
}): Promise<AuthUser> {
  return fetchJson<AuthUser>('/users', { method: 'POST', body: JSON.stringify(body) })
}

export async function patchUser(
  id: string,
  body: { role?: UserRole; password?: string },
): Promise<AuthUser> {
  return fetchJson<AuthUser>(`/users/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body: JSON.stringify(body),
  })
}

export async function deleteUser(id: string): Promise<void> {
  await fetchJson(`/users/${encodeURIComponent(id)}`, { method: 'DELETE' })
}

export async function setup2FA(): Promise<{
  secret: string
  provisioning_uri: string
  qr: string
}> {
  return fetchJson('/2fa/setup', { method: 'POST', body: '{}' })
}

export async function confirm2FA(code: string): Promise<{ recovery_codes: string[] }> {
  return fetchJson('/2fa/confirm', { method: 'POST', body: JSON.stringify({ code }) })
}

export async function delete2FA(): Promise<void> {
  await fetchJson('/2fa', { method: 'DELETE' })
}

/* —— Peppol (Phase 4) —— */

export interface PeppolParticipant {
  peppol_id: string
  service_type: string
  name: string
  is_self: boolean
  registered_endpoint?: string
  created_at?: string
}

export interface PeppolMessage {
  id: number
  direction: 'in' | 'out' | string
  from_peppol_id: string
  to_peppol_id: string
  ap_message_id: string
  status: string
  invoice_id?: number | null
  external_id?: string
  created_at: string
  updated_at?: string
  bis_xml?: string
  receipt?: string
}

export interface PeppolStatus {
  self_id: string
  ap_name: string
  ap_configured: boolean
  callback_url?: string
  poll_enabled?: boolean
  poll_interval?: string
  counts?: Record<string, number>
}

export interface PeppolSendResult {
  message_id: number
  status: string
  invoice_id: number
  receipt?: {
    ap_message_id?: string
    status?: string
    at?: string
  }
}

export async function peppolStatus(): Promise<PeppolStatus> {
  return fetchJson<PeppolStatus>('/peppol/status')
}

export async function peppolParticipants(): Promise<PeppolParticipant[]> {
  const res = await fetchJson<{ participants: PeppolParticipant[] }>('/peppol/participants')
  return res.participants ?? []
}

export async function peppolAddParticipant(
  p: Partial<PeppolParticipant> & { peppol_id: string },
): Promise<PeppolParticipant> {
  return fetchJson<PeppolParticipant>('/peppol/participants', {
    method: 'POST',
    body: JSON.stringify(p),
  })
}

export async function peppolMessages(params: {
  direction?: string
  status?: string
  limit?: number
} = {}): Promise<PeppolMessage[]> {
  const q = buildQuery({
    direction: params.direction,
    status: params.status,
    limit: params.limit,
  })
  const res = await fetchJson<{ messages: PeppolMessage[] }>(`/peppol/messages${q}`)
  return res.messages ?? []
}

export async function peppolMessage(id: number): Promise<PeppolMessage> {
  return fetchJson<PeppolMessage>(`/peppol/messages/${id}`)
}

export async function peppolSend(body: {
  invoice_id?: number
  to: string
  gobl?: object
}): Promise<PeppolSendResult> {
  return fetchJson<PeppolSendResult>('/peppol/send', {
    method: 'POST',
    body: JSON.stringify(body),
  })
}

export async function peppolPing(): Promise<{ ok: boolean; ap_name?: string; error?: string }> {
  return fetchJson('/peppol/ping', { method: 'POST', body: '{}' })
}

