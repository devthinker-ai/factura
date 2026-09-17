/**
 * Factura JS SDK — zero deps, ESM, copy-paste from docs/EMBED.md.
 * Auth: Authorization: Bearer factura_…
 */
export class FacturaError extends Error {
  /** @param {number} status @param {string} code @param {string} [detail] */
  constructor(status, code, detail = '') {
    super(detail ? `${code}: ${detail}` : code)
    this.name = 'FacturaError'
    this.status = status
    /** @type {'unauthorized'|'quota_exceeded'|'plan_limit'|'rate_limited'|'invalid'} */
    this.code = code
    this.detail = detail
  }
}

export class Factura {
  /** @param {string} baseURL @param {string} key */
  constructor(baseURL, key) {
    this.baseURL = String(baseURL || '').replace(/\/$/, '')
    this.key = key
  }

  /** @param {string} path @param {RequestInit} [init] */
  async #req(path, init = {}) {
    const headers = new Headers(init.headers || {})
    if (this.key) headers.set('Authorization', `Bearer ${this.key}`)
    const res = await fetch(`${this.baseURL}${path}`, { ...init, headers })
    if (!res.ok) {
      let body = null
      try {
        body = await res.json()
      } catch {
        /* ignore */
      }
      const errName = (body && body.error) || 'invalid'
      let code = 'invalid'
      if (res.status === 401) code = 'unauthorized'
      else if (res.status === 429) code = 'rate_limited'
      else if (res.status === 403 && errName === 'quota_exceeded') code = 'quota_exceeded'
      else if (res.status === 403) code = 'plan_limit'
      throw new FacturaError(res.status, code, (body && body.detail) || errName)
    }
    if (res.status === 204) return undefined
    const ct = res.headers.get('content-type') || ''
    if (ct.includes('application/json')) return res.json()
    return res.arrayBuffer()
  }

  /** @param {string} filename @param {Blob|ArrayBuffer|Uint8Array} data */
  async ingest(filename, data) {
    const headers = {}
    if (filename) headers['X-Factura-External-ID'] = filename
    return this.#req('/invoices', { method: 'POST', headers, body: data })
  }

  /** @param {object} goblJSON */
  async generate(goblJSON) {
    return this.#req('/invoices/generate?format=xrechnung&download=1', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(goblJSON),
    })
  }

  /** @param {string} [query] */
  async list(query = '') {
    const q = query ? `?${query.replace(/^\?/, '')}` : ''
    const res = await this.#req(`/invoices${q}`)
    return res.rows || []
  }

  async usage() {
    return this.#req('/usage')
  }
}

export default Factura
