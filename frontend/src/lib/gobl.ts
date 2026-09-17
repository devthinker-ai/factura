/**
 * GOBL bill.Invoice builder for the New Invoice form.
 *
 * THIS IS THE ONLY PLACE GOBL shape is built in JS.
 * Must stay in sync with pkg/model (Go). Server 422 responses are the
 * source of truth — if generate rejects, show the error as-is.
 *
 * GOBL JSON gotcha (org.Email / org.Telephone): serialize as `addr` and
 * `num` — NOT `address` / `number`. Wrong keys are silently dropped.
 */

export type VatRateChoice = '19' | '7' | '0' | 'reverse-charge'

/** BT-3 document kinds surfaced in the New invoice form. */
export type DocTypeChoice = 'standard' | 'credit-note' | 'corrective' | 'self-billed'

/** Payment means: Überweisung (default) or Lastschrift (BG-19). */
export type PaymentMethodChoice = 'credit-transfer' | 'direct-debit'

/** ISO 3166-1 alpha-2 countries offered in the party country picker (label-only; regime stays DE). */
export const COUNTRY_OPTIONS = [
  'DE',
  'AT',
  'BE',
  'BG',
  'HR',
  'CY',
  'CZ',
  'DK',
  'EE',
  'FI',
  'FR',
  'GR',
  'HU',
  'IE',
  'IT',
  'LV',
  'LT',
  'LU',
  'MT',
  'NL',
  'PL',
  'PT',
  'RO',
  'SK',
  'SI',
  'ES',
  'SE',
] as const

export interface PartyForm {
  name: string
  street: string
  locality: string
  code: string // PLZ
  taxId: string // DE USt-IdNr digits or with DE prefix
  /** BT-40 / BT-55 address + tax_id country (default DE; regime stays DE). */
  country?: string
  email?: string
  /** BT-42 seller/buyer phone → people[].telephones[].num */
  phone?: string
  /**
   * BT-41 contact person. Stored entirely in GOBL `name.given`
   * (surname left empty) — one form field, one given name.
   */
  contactName?: string
  /** Kleinunternehmer § 19: omit tax_id, emit Steuernummer as BT-32. */
  kleinunternehmer?: boolean
  /** BT-32 Steuernummer (scope:tax identity) when kleinunternehmer. */
  steuernummer?: string
  /** BT-29 seller / BT-46 buyer identifier (identities[].code, no scope). */
  partyId?: string
  /** BT-30 Handelsregisternummer (identities[].scope = legal). */
  legalReg?: string
}

/** Invoice-level discount (BT-107) — percent XOR amount. */
export interface DiscountForm {
  id: string
  reason: string
  /** 'percent' → GOBL percent; 'amount' → fixed amount. */
  mode: 'percent' | 'amount'
  value: string
}

export interface LineForm {
  id: string
  name: string
  quantity: string
  price: string
  vatRate: VatRateChoice
}

export interface PrecedingForm {
  code: string
  issueDate: string // YYYY-MM-DD
}

export interface AttachmentForm {
  id: string
  name: string
  description: string
  mime: string
  dataUri: string // data:<mime>;base64,…
}

export interface InvoiceFormState {
  code: string
  issueDate: string // YYYY-MM-DD
  dueDate?: string
  currency: string
  notes?: string
  buyerRef?: string
  /** Optional B2G Leitweg-ID — overrides buyerRef in ordering.code when set. */
  leitwegId?: string
  docType: DocTypeChoice
  preceding?: PrecedingForm
  attachments: AttachmentForm[]
  reverseCharge: boolean
  supplier: PartyForm
  customer: PartyForm
  lines: LineForm[]
  paymentMethod: PaymentMethodChoice
  iban?: string
  /** BT-86 BIC (credit transfer, optional). */
  bic?: string
  /** BT-85 account holder / payee name (credit transfer, optional). */
  accountHolder?: string
  /** BG-19 BT-89 mandate reference (direct debit). */
  ddMandateRef?: string
  /** BG-19 BT-90 creditor ID (direct debit). */
  ddCreditorId?: string
  /** BT-113 already-paid amount. */
  alreadyPaid?: string
  /** BT-11 project reference. */
  projectRef?: string
  /** BT-12 contract reference. */
  contractRef?: string
  /** BT-13 purchase order (Bestellnummer). */
  purchaseOrder?: string
  /** BT-14 sales order (Auftragsnummer). */
  salesOrder?: string
  /** BT-17 tender (Vergabenummer). */
  tenderRef?: string
  /** BT-19 buyer accounting (Kontierung). */
  accountingCost?: string
  /**
   * BT-72 delivery / supply date.
   * gobl.cii maps `bill.Delivery.Date` (`json:"date"`) → ActualDeliverySupplyChainEvent
   * — NOT ReceiveDate / receive_date (verified against gobl.cii/delivery.go).
   */
  deliveryDate?: string
  /** BT-73 invoice period start. */
  periodStart?: string
  /** BT-74 invoice period end. */
  periodEnd?: string
  /** BT-83 remittance information (Verwendungszweck). */
  remittanceRef?: string
  /** BT-20 free-text payment terms (Zahlungsbedingungen). */
  paymentTermsNotes?: string
  /** BT-107 invoice-level discounts (Nachlässe). */
  discounts: DiscountForm[]
}

/** Max BT-125 attachments (EN 16931 / XRechnung practical cap). */
export const MAX_ATTACHMENTS = 200

/** BT-125 MIME allow-list (e-rechnung.bund.de / EN 16931 / XRechnung). */
export const ALLOWED_ATTACHMENT_MIMES = new Set([
  'application/pdf',
  'image/png',
  'image/jpeg',
  'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  'application/vnd.oasis.opendocument.spreadsheet',
  'text/csv',
])

const EXT_TO_MIME: Record<string, string> = {
  pdf: 'application/pdf',
  png: 'image/png',
  jpg: 'image/jpeg',
  jpeg: 'image/jpeg',
  csv: 'text/csv',
  xlsx: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  ods: 'application/vnd.oasis.opendocument.spreadsheet',
}

export function normalizeAttachmentMIME(mime: string): string {
  const m = mime.trim().toLowerCase()
  if (m === 'image/jpg') return 'image/jpeg'
  return m
}

/** Resolve MIME from File.type or filename extension. */
export function mimeForFile(file: File): string {
  if (file.type) return normalizeAttachmentMIME(file.type)
  const ext = file.name.split('.').pop()?.toLowerCase() || ''
  return EXT_TO_MIME[ext] || ''
}

export function isAllowedAttachmentMIME(mime: string): boolean {
  return ALLOWED_ATTACHMENT_MIMES.has(normalizeAttachmentMIME(mime))
}

export function hintLeitwegID(id: string): boolean {
  const s = id.trim().toUpperCase()
  if (!s) return true
  return /^[0-9A-Z]{2,12}(-[0-9A-Z]{0,30})?-[0-9]{2}$/.test(s)
}

function normalizeTaxCode(raw: string): string {
  const s = raw.trim().toUpperCase().replace(/^DE/, '').replace(/\s+/g, '')
  return s
}

function partyCountry(p: PartyForm): string {
  const c = (p.country || 'DE').trim().toUpperCase()
  return /^[A-Z]{2}$/.test(c) ? c : 'DE'
}

function partyToGobl(p: PartyForm) {
  const country = partyCountry(p)
  const out: Record<string, unknown> = {
    name: p.name.trim(),
    addresses: [
      {
        street: p.street.trim(),
        locality: p.locality.trim(),
        code: p.code.trim(),
        country,
      },
    ],
  }

  const identities: Array<Record<string, string>> = []
  const partyId = p.partyId?.trim()
  if (partyId) {
    // BT-29 / BT-46: no scope, no scheme → ram:ID
    identities.push({ code: partyId })
  }
  const legalReg = p.legalReg?.trim()
  if (legalReg) {
    // BT-30: scope legal → SpecifiedLegalOrganization/ID
    identities.push({ scope: 'legal', code: legalReg })
  }

  if (p.kleinunternehmer) {
    const sn = p.steuernummer?.trim()
    if (sn) {
      identities.push({ scope: 'tax', country, code: sn })
    }
  } else {
    out.tax_id = {
      country,
      code: normalizeTaxCode(p.taxId),
    }
  }

  if (identities.length) {
    out.identities = identities
  }

  const email = p.email?.trim()
  if (email) {
    // GOBL org.Email uses json:"addr" — NOT "address".
    out.emails = [{ addr: email }]
    // Buyer BT-49 / seller PEPPOL-EN16931-R020 come from inboxes, not emails.
    out.inboxes = [{ email }]
  }

  const phone = p.phone?.trim()
  const contactName = p.contactName?.trim()
  if (contactName || phone || email) {
    // Person.Name is required by GOBL; contactName lives entirely in `given`
    // (empty surname). Fallback to party name so phone/email-only contacts
    // still envelop.
    const person: Record<string, unknown> = {
      name: { given: contactName || p.name.trim() || 'Contact' },
    }
    if (phone) {
      // GOBL org.Telephone uses json:"num" — NOT "number".
      person.telephones = [{ num: phone }]
    }
    if (email) {
      person.emails = [{ addr: email }]
    }
    out.people = [person]
  }

  return out
}

function lineTaxes(
  vatRate: VatRateChoice,
  reverseCharge: boolean,
): Array<Record<string, string>> {
  if (reverseCharge || vatRate === 'reverse-charge') {
    return [{ cat: 'VAT', key: 'reverse-charge' }]
  }
  if (vatRate === '19') return [{ cat: 'VAT', rate: 'general' }]
  if (vatRate === '7') return [{ cat: 'VAT', rate: 'reduced' }]
  return [{ cat: 'VAT', rate: 'zero' }]
}

function paymentInstructions(state: InvoiceFormState): Record<string, unknown> {
  let instr: Record<string, unknown>
  if (state.paymentMethod === 'direct-debit') {
    instr = {
      key: 'direct-debit',
      direct_debit: {
        ref: state.ddMandateRef?.trim() || '',
        creditor: state.ddCreditorId?.trim() || '',
        account: state.iban?.trim() || '',
      },
    }
  } else {
    const ct: Record<string, string> = {
      iban: state.iban?.trim() || 'DE89370400440532013000',
    }
    if (state.bic?.trim()) ct.bic = state.bic.trim()
    if (state.accountHolder?.trim()) ct.name = state.accountHolder.trim()
    instr = {
      key: 'credit-transfer+sepa',
      credit_transfer: [ct],
    }
  }
  // BT-83 remittance information
  const remittance = state.remittanceRef?.trim()
  if (remittance) instr.ref = remittance
  return instr
}

function refDocs(code?: string): Array<{ code: string }> | undefined {
  const c = code?.trim()
  return c ? [{ code: c }] : undefined
}

function normalizePercent(s: string): string {
  const t = s.trim().replace(',', '.').replace(/%$/, '')
  const n = Number(t)
  if (Number.isNaN(n)) return '0%'
  return `${n}%`
}

function linesNetSum(state: InvoiceFormState): string {
  let net = 0
  for (const l of state.lines) {
    const qty = Number(String(l.quantity).replace(',', '.')) || 0
    const price = Number(String(l.price).replace(',', '.')) || 0
    net += qty * price
  }
  // Credit notes use negative line prices; base for % discount is absolute sum.
  return Math.abs(net).toFixed(2)
}

/** Tax combo for invoice-level discounts (BG-20 requires a VAT category). */
function discountTaxes(state: InvoiceFormState): Array<Record<string, string>> {
  if (state.reverseCharge) return [{ cat: 'VAT', key: 'reverse-charge' }]
  const rates = state.lines.map((l) => l.vatRate)
  if (rates.includes('19')) return [{ cat: 'VAT', rate: 'general' }]
  if (rates.includes('7')) return [{ cat: 'VAT', rate: 'reduced' }]
  if (rates.includes('reverse-charge')) return [{ cat: 'VAT', key: 'reverse-charge' }]
  if (rates.includes('0')) return [{ cat: 'VAT', rate: 'zero' }]
  return [{ cat: 'VAT', rate: 'general' }]
}

/** Build bare bill.Invoice JSON (not an envelope). */
export function toGobl(state: InvoiceFormState): object {
  const lines = state.lines.map((l) => {
    let price = normalizeMoney(l.price)
    // Credit-note sign convention: negative line prices (matches pkg/model).
    if (state.docType === 'credit-note' && !price.startsWith('-') && price !== '0.00') {
      price = `-${price}`
    }
    return {
      quantity: String(l.quantity || '1'),
      item: {
        name: l.name.trim(),
        price,
      },
      taxes: lineTaxes(l.vatRate, state.reverseCharge),
    }
  })

  const buyerRef = state.leitwegId?.trim() || state.buyerRef?.trim() || 'NA'

  const type =
    state.docType === 'self-billed'
      ? 'standard'
      : state.docType === 'corrective'
        ? 'corrective'
        : state.docType === 'credit-note'
          ? 'credit-note'
          : 'standard'

  const payment: Record<string, unknown> = {
    instructions: paymentInstructions(state),
  }
  const paid = state.alreadyPaid?.trim()
  if (paid) {
    payment.advances = [{ amount: normalizeMoney(paid) }]
  }

  const ordering: Record<string, unknown> = {
    code: buyerRef,
  }
  const projects = refDocs(state.projectRef)
  if (projects) ordering.projects = projects
  const contracts = refDocs(state.contractRef)
  if (contracts) ordering.contracts = contracts
  const purchases = refDocs(state.purchaseOrder)
  if (purchases) ordering.purchases = purchases
  const sales = refDocs(state.salesOrder)
  if (sales) ordering.sales = sales
  const tender = refDocs(state.tenderRef)
  if (tender) ordering.tender = tender
  const cost = state.accountingCost?.trim()
  if (cost) ordering.cost = cost
  const pStart = state.periodStart?.trim()
  const pEnd = state.periodEnd?.trim()
  if (pStart && pEnd) {
    ordering.period = { start: pStart, end: pEnd }
  }

  const inv: Record<string, unknown> = {
    $schema: 'https://gobl.org/draft-0/bill/invoice',
    $regime: 'DE',
    $addons: ['de-xrechnung-v3', 'de-zugferd-v2'],
    type,
    code: state.code.trim(),
    issue_date: state.issueDate,
    currency: state.currency || 'EUR',
    supplier: partyToGobl(state.supplier),
    customer: partyToGobl(state.customer),
    lines,
    ordering,
    payment,
  }

  // BT-72: gobl.cii reads Delivery.Date (json "date"), not ReceiveDate.
  const deliveryDate = state.deliveryDate?.trim()
  if (deliveryDate) {
    inv.delivery = { date: deliveryDate }
  }

  if (state.docType === 'self-billed') {
    // Canonical GOBL field for BT-3=389 (dry-read: $tags works; bare tags does not).
    inv.$tags = state.reverseCharge ? ['self-billed', 'reverse-charge'] : ['self-billed']
  } else if (state.reverseCharge) {
    inv.tags = ['reverse-charge']
  }

  if (
    (state.docType === 'credit-note' || state.docType === 'corrective') &&
    state.preceding?.code?.trim()
  ) {
    inv.preceding = [
      {
        code: state.preceding.code.trim(),
        issue_date: state.preceding.issueDate || state.issueDate,
      },
    ]
  }

  if (state.attachments?.length) {
    inv.attachments = state.attachments.map((a, i) => ({
      name: a.name,
      code: `ATT-${i + 1}`,
      description: a.description || undefined,
      mime: a.mime,
      url: a.dataUri,
    }))
  }

  if (state.notes?.trim()) {
    inv.notes = [{ key: 'general', text: state.notes.trim() }]
  }

  const termsNotes = state.paymentTermsNotes?.trim()
  if (state.dueDate?.trim()) {
    const terms: Record<string, unknown> = {
      due_dates: [{ date: state.dueDate.trim() }],
    }
    // BT-20 free text coexists with BT-9 due dates when set.
    if (termsNotes) terms.notes = termsNotes
    payment.terms = terms
  } else {
    // BR-CO-25: positive BT-115 requires BT-9 (due date) or BT-20 (terms).
    // Match NewDEB2B default when the form leaves due date empty.
    payment.terms = { notes: termsNotes || '14 Tage netto' }
  }

  // BT-107 invoice-level discounts
  const discounts = (state.discounts || [])
    .map((d) => {
      const reason = d.reason.trim()
      const value = d.value.trim()
      if (!reason || !value) return null
      const row: Record<string, unknown> = {
        reason,
        taxes: discountTaxes(state),
      }
      if (d.mode === 'percent') {
        // PEPPOL-EN16931-R041: percent requires base (BT-93) alongside BT-94.
        row.percent = normalizePercent(value)
        row.base = linesNetSum(state)
      } else {
        row.amount = normalizeMoney(value)
      }
      return row
    })
    .filter((d): d is Record<string, unknown> => d != null)
  if (discounts.length) {
    inv.discounts = discounts
  }

  return inv
}

function normalizeMoney(s: string): string {
  const t = s.trim().replace(',', '.')
  const n = Number(t)
  if (Number.isNaN(n)) return '0.00'
  return n.toFixed(2)
}

function vatPercent(rate: VatRateChoice, reverseCharge: boolean): number {
  if (reverseCharge || rate === 'reverse-charge' || rate === '0') return 0
  if (rate === '19') return 0.19
  if (rate === '7') return 0.07
  return 0
}

export interface PreviewTotals {
  net: number
  vat: number
  gross: number
  byRate: { label: string; net: number; vat: number }[]
}

/** Client-side preview only — labeled as preview in the UI. Server is judge. */
export function previewTotals(state: InvoiceFormState): PreviewTotals {
  const buckets = new Map<string, { net: number; vat: number }>()
  let net = 0
  let vat = 0
  for (const l of state.lines) {
    const qty = Number(String(l.quantity).replace(',', '.')) || 0
    const price = Number(String(l.price).replace(',', '.')) || 0
    const lineNet = qty * price
    const pct = vatPercent(l.vatRate, state.reverseCharge)
    const lineVat = lineNet * pct
    net += lineNet
    vat += lineVat
    const label =
      state.reverseCharge || l.vatRate === 'reverse-charge'
        ? 'reverse-charge'
        : l.vatRate === '19'
          ? '19%'
          : l.vatRate === '7'
            ? '7%'
            : '0%'
    const b = buckets.get(label) ?? { net: 0, vat: 0 }
    b.net += lineNet
    b.vat += lineVat
    buckets.set(label, b)
  }

  // BT-107: subtract invoice-level discounts from net; scale VAT proportionally.
  let discountNet = 0
  for (const d of state.discounts || []) {
    const value = Number(String(d.value).replace(',', '.').replace(/%$/, '')) || 0
    if (!d.reason.trim() || !value) continue
    if (d.mode === 'percent') {
      discountNet += net * (value / 100)
    } else {
      discountNet += value
    }
  }
  if (discountNet !== 0 && net !== 0) {
    const scale = (net - discountNet) / net
    net = net - discountNet
    vat = vat * scale
    for (const b of buckets.values()) {
      b.net *= scale
      b.vat *= scale
    }
  } else if (discountNet !== 0) {
    net = net - discountNet
  }

  const paid = Number(String(state.alreadyPaid || '').replace(',', '.')) || 0
  return {
    net,
    vat,
    gross: net + vat - paid,
    byRate: [...buckets.entries()].map(([label, v]) => ({ label, ...v })),
  }
}

/**
 * Live mod-11 syntax hint for DE VAT ID (USt-IdNr).
 * Digits only (strips DE), length 9, mod-11 check digit.
 * Returns a stable key for i18n (or null if empty/OK).
 */
export type UStHintKey = 'digits' | 'length' | 'check'

export function hintUStIdNr(code: string): { key: UStHintKey; length?: number } | null {
  const raw = code.trim()
  if (!raw) return null
  const digits = raw.toUpperCase().replace(/^DE/, '').replace(/\s+/g, '')
  if (!/^\d+$/.test(digits)) {
    return { key: 'digits' }
  }
  if (digits.length !== 9) {
    return { key: 'length', length: digits.length }
  }
  // Official BZSt mod-11 algorithm for DE VAT IDs
  let product = 10
  for (let i = 0; i < 8; i++) {
    const sum = Number(digits[i]) + product
    let m = sum % 10
    if (m === 0) m = 10
    product = (2 * m) % 11
  }
  let check = 11 - product
  if (check === 10) check = 0
  if (Number(digits[8]) !== check) {
    return { key: 'check' }
  }
  return null
}

export function defaultFormState(): InvoiceFormState {
  const today = new Date().toISOString().slice(0, 10)
  return {
    code: '',
    issueDate: today,
    currency: 'EUR',
    reverseCharge: false,
    buyerRef: 'NA',
    leitwegId: '',
    docType: 'standard',
    attachments: [],
    paymentMethod: 'credit-transfer',
    iban: 'DE89370400440532013000',
    discounts: [],
    supplier: {
      name: '',
      street: '',
      locality: '',
      code: '',
      taxId: '',
      country: 'DE',
      email: '',
      phone: '',
      contactName: '',
      kleinunternehmer: false,
      steuernummer: '',
      partyId: '',
      legalReg: '',
    },
    customer: {
      name: '',
      street: '',
      locality: '',
      code: '',
      taxId: '',
      country: 'DE',
      email: '',
      phone: '',
      contactName: '',
      partyId: '',
    },
    lines: [
      {
        id: '1',
        name: '',
        quantity: '1',
        price: '0.00',
        vatRate: '19',
      },
    ],
  }
}
