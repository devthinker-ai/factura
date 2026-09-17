import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import {
  hintUStIdNr,
  isAllowedAttachmentMIME,
  previewTotals,
  toGobl,
  type InvoiceFormState,
} from '@/lib/gobl'

function loadGolden(name: string) {
  return JSON.parse(
    readFileSync(join(__dirname, '../../testdata', name), 'utf8'),
  ) as object
}

const baseParties = {
  supplier: {
    name: 'Provide One GmbH',
    street: 'Dietmar-Hopp-Allee',
    locality: 'Walldorf',
    code: '69190',
    taxId: '111111125',
    email: 'billing@example.com',
  },
  customer: {
    name: 'Sample Consumer',
    street: 'Werner-Heisenberg-Allee',
    locality: 'München',
    code: '80939',
    taxId: '282741168',
  },
}

describe('toGobl', () => {
  it('matches mixed 19+7 golden', () => {
    const state: InvoiceFormState = {
      code: 'R-2026-MIX',
      issueDate: '2026-03-01',
      currency: 'EUR',
      reverseCharge: false,
      buyerRef: 'PO-42',
      docType: 'standard',
      attachments: [],
      paymentMethod: 'credit-transfer',
      iban: 'DE89370400440532013000',
      discounts: [],
      ...baseParties,
      lines: [
        { id: '1', name: 'Consulting', quantity: '1', price: '100.00', vatRate: '19' },
        { id: '2', name: 'Books', quantity: '2', price: '20.00', vatRate: '7' },
      ],
    }
    expect(toGobl(state)).toEqual(loadGolden('gobl-mixed-19-7.json'))
  })

  it('matches reverse-charge golden', () => {
    const state: InvoiceFormState = {
      code: 'R-2026-RC',
      issueDate: '2026-03-01',
      currency: 'EUR',
      reverseCharge: true,
      buyerRef: 'NA',
      docType: 'standard',
      attachments: [],
      paymentMethod: 'credit-transfer',
      iban: 'DE89370400440532013000',
      discounts: [],
      supplier: { ...baseParties.supplier, email: undefined },
      customer: baseParties.customer,
      lines: [
        { id: '1', name: 'Consulting', quantity: '1', price: '100.00', vatRate: '19' },
      ],
    }
    expect(toGobl(state)).toEqual(loadGolden('gobl-reverse-charge.json'))
  })

  it('matches 0% line golden', () => {
    const state: InvoiceFormState = {
      code: 'R-2026-ZERO',
      issueDate: '2026-03-01',
      currency: 'EUR',
      reverseCharge: false,
      buyerRef: 'NA',
      docType: 'standard',
      attachments: [],
      paymentMethod: 'credit-transfer',
      iban: 'DE89370400440532013000',
      discounts: [],
      supplier: { ...baseParties.supplier, email: undefined },
      customer: baseParties.customer,
      lines: [
        { id: '1', name: 'Exempt service', quantity: '1', price: '50.00', vatRate: '0' },
      ],
    }
    expect(toGobl(state)).toEqual(loadGolden('gobl-zero.json'))
  })
})

describe('previewTotals', () => {
  it('sums mixed rates', () => {
    const state: InvoiceFormState = {
      code: 'x',
      issueDate: '2026-01-01',
      currency: 'EUR',
      reverseCharge: false,
      docType: 'standard',
      attachments: [],
      paymentMethod: 'credit-transfer',
      discounts: [],
      supplier: baseParties.supplier,
      customer: baseParties.customer,
      lines: [
        { id: '1', name: 'A', quantity: '1', price: '100', vatRate: '19' },
        { id: '2', name: 'B', quantity: '2', price: '20', vatRate: '7' },
      ],
    }
    const t = previewTotals(state)
    expect(t.net).toBeCloseTo(140)
    expect(t.vat).toBeCloseTo(19 + 2.8)
    expect(t.gross).toBeCloseTo(140 + 19 + 2.8)
  })
})

describe('hintUStIdNr', () => {
  it('accepts known-good sample', () => {
    expect(hintUStIdNr('111111125')).toBeNull()
    expect(hintUStIdNr('DE111111125')).toBeNull()
  })
  it('flags bad check digit', () => {
    expect(hintUStIdNr('111111126')).toEqual({ key: 'check' })
  })
  it('flags length', () => {
    expect(hintUStIdNr('123')).toEqual({ key: 'length', length: 3 })
  })
})

describe('Phase 8 toGobl', () => {
  it('emits credit-note type + preceding', () => {
    const state: InvoiceFormState = {
      ...defaultLike(),
      docType: 'credit-note',
      preceding: { code: 'R-0', issueDate: '2026-08-01' },
      lines: [{ id: '1', name: 'Credit', quantity: '1', price: '100.00', vatRate: '19' }],
    }
    const doc = toGobl(state) as Record<string, unknown>
    expect(doc.type).toBe('credit-note')
    expect(doc.preceding).toEqual([{ code: 'R-0', issue_date: '2026-08-01' }])
    const lines = doc.lines as Array<{ item: { price: string } }>
    expect(lines[0].item.price).toBe('-100.00')
  })

  it('emits self-billed as $tags', () => {
    const state: InvoiceFormState = {
      ...defaultLike(),
      docType: 'self-billed',
    }
    const doc = toGobl(state) as Record<string, unknown>
    expect(doc.type).toBe('standard')
    expect(doc.$tags).toEqual(['self-billed'])
  })

  it('puts Leitweg-ID in ordering.code', () => {
    const state: InvoiceFormState = {
      ...defaultLike(),
      buyerRef: 'PO-42',
      leitwegId: '04011000-12345-34',
    }
    const doc = toGobl(state) as { ordering: { code: string } }
    expect(doc.ordering.code).toBe('04011000-12345-34')
  })

  it('embeds attachment data-URI', () => {
    const state: InvoiceFormState = {
      ...defaultLike(),
      attachments: [
        {
          id: '1',
          name: 'scan.pdf',
          description: 'scan',
          mime: 'application/pdf',
          dataUri: 'data:application/pdf;base64,JVBERi0=',
        },
      ],
    }
    const doc = toGobl(state) as { attachments: Array<{ url: string; name: string }> }
    expect(doc.attachments[0].url).toContain('data:application/pdf;base64,')
    expect(doc.attachments[0].name).toBe('scan.pdf')
  })
})

describe('Phase 9 toGobl contact + payment', () => {
  it('emits addr/num/inboxes/people (not address/number)', () => {
    const state: InvoiceFormState = {
      ...defaultLike(),
      supplier: {
        ...baseParties.supplier,
        contactName: 'Ada Lovelace',
        phone: '+49100200300',
        email: 'billing@example.com',
      },
      customer: {
        ...baseParties.customer,
        email: 'customer@example.com',
        phone: '+498912345',
        contactName: 'Bob Buyer',
      },
    }
    const doc = toGobl(state) as {
      supplier: {
        emails: Array<Record<string, string>>
        inboxes: Array<{ email: string }>
        people: Array<{
          name: { given: string }
          telephones: Array<Record<string, string>>
          emails: Array<Record<string, string>>
        }>
      }
      customer: {
        emails: Array<Record<string, string>>
        inboxes: Array<{ email: string }>
      }
    }
    expect(doc.supplier.emails[0]).toEqual({ addr: 'billing@example.com' })
    expect(doc.supplier.emails[0]).not.toHaveProperty('address')
    expect(doc.supplier.inboxes[0].email).toBe('billing@example.com')
    expect(doc.supplier.people[0].name.given).toBe('Ada Lovelace')
    expect(doc.supplier.people[0].telephones[0]).toEqual({ num: '+49100200300' })
    expect(doc.supplier.people[0].telephones[0]).not.toHaveProperty('number')
    expect(doc.customer.inboxes[0].email).toBe('customer@example.com')
    expect(doc.customer.emails[0]).toEqual({ addr: 'customer@example.com' })
  })

  it('swaps payment instructions between credit-transfer and direct-debit', () => {
    const ct = toGobl({
      ...defaultLike(),
      paymentMethod: 'credit-transfer',
      bic: 'COBADEFFXXX',
      accountHolder: 'Provide One GmbH',
    }) as {
      payment: {
        instructions: {
          key: string
          credit_transfer: Array<{ iban: string; bic?: string; name?: string }>
        }
      }
    }
    expect(ct.payment.instructions.key).toBe('credit-transfer+sepa')
    expect(ct.payment.instructions.credit_transfer[0]).toEqual({
      iban: 'DE89370400440532013000',
      bic: 'COBADEFFXXX',
      name: 'Provide One GmbH',
    })

    const dd = toGobl({
      ...defaultLike(),
      paymentMethod: 'direct-debit',
      ddMandateRef: 'MANDATE-1',
      ddCreditorId: 'DE98ZZZ09999999999',
      iban: 'DE89370400440532013000',
    }) as {
      payment: {
        instructions: {
          key: string
          direct_debit: { ref: string; creditor: string; account: string }
        }
      }
    }
    expect(dd.payment.instructions.key).toBe('direct-debit')
    expect(dd.payment.instructions.direct_debit).toEqual({
      ref: 'MANDATE-1',
      creditor: 'DE98ZZZ09999999999',
      account: 'DE89370400440532013000',
    })
  })

  it('emits advances when already paid is set', () => {
    const doc = toGobl({
      ...defaultLike(),
      alreadyPaid: '50.00',
    }) as { payment: { advances: Array<{ amount: string }> } }
    expect(doc.payment.advances).toEqual([{ amount: '50.00' }])
  })

  it('Kleinunternehmer emits tax identity and omits tax_id', () => {
    const doc = toGobl({
      ...defaultLike(),
      supplier: {
        ...baseParties.supplier,
        kleinunternehmer: true,
        steuernummer: '12/345/67890',
        taxId: '',
        email: 'billing@example.com',
        phone: '+49100200300',
        contactName: 'Ada',
      },
    }) as {
      supplier: {
        tax_id?: unknown
        identities: Array<{ scope: string; country: string; code: string }>
      }
    }
    expect(doc.supplier.tax_id).toBeUndefined()
    expect(doc.supplier.identities).toEqual([
      { scope: 'tax', country: 'DE', code: '12/345/67890' },
    ])
  })

  it('rejects disallowed attachment MIME types', () => {
    expect(isAllowedAttachmentMIME('application/pdf')).toBe(true)
    expect(isAllowedAttachmentMIME('image/jpg')).toBe(true)
    expect(isAllowedAttachmentMIME('text/html')).toBe(false)
    expect(isAllowedAttachmentMIME('application/zip')).toBe(false)
    expect(isAllowedAttachmentMIME('')).toBe(false)
  })
})

function defaultLike(): InvoiceFormState {
  return {
    code: 'R-1',
    issueDate: '2026-09-01',
    currency: 'EUR',
    reverseCharge: false,
    buyerRef: 'NA',
    docType: 'standard',
    attachments: [],
    paymentMethod: 'credit-transfer',
    iban: 'DE89370400440532013000',
    discounts: [],
    ...baseParties,
    lines: [{ id: '1', name: 'Consulting', quantity: '1', price: '100.00', vatRate: '19' }],
  }
}

describe('Phase 10 optional EN 16931 fields', () => {
  it('emits ordering refs, period, and delivery.date when set; omits when empty', () => {
    const filled = toGobl({
      ...defaultLike(),
      projectRef: 'PRJ-1',
      contractRef: 'CTR-9',
      purchaseOrder: 'PO-42',
      salesOrder: 'SO-7',
      tenderRef: 'TND-3',
      accountingCost: '1287:65464',
      periodStart: '2026-08-01',
      periodEnd: '2026-08-31',
      deliveryDate: '2026-08-15',
    }) as {
      ordering: Record<string, unknown>
      delivery?: { date: string; receive_date?: string }
    }
    expect(filled.ordering).toMatchObject({
      code: 'NA',
      projects: [{ code: 'PRJ-1' }],
      contracts: [{ code: 'CTR-9' }],
      purchases: [{ code: 'PO-42' }],
      sales: [{ code: 'SO-7' }],
      tender: [{ code: 'TND-3' }],
      cost: '1287:65464',
      period: { start: '2026-08-01', end: '2026-08-31' },
    })
    // gobl.cii maps Delivery.Date (json "date"), not receive_date
    expect(filled.delivery).toEqual({ date: '2026-08-15' })
    expect(filled.delivery).not.toHaveProperty('receive_date')

    const bare = toGobl(defaultLike()) as {
      ordering: Record<string, unknown>
      delivery?: unknown
    }
    expect(bare.ordering).toEqual({ code: 'NA' })
    expect(bare.delivery).toBeUndefined()
    expect(bare).not.toHaveProperty('discounts')
  })

  it('emits party identities (BT-29/46/30) coexisting with Kleinunternehmer tax identity', () => {
    const doc = toGobl({
      ...defaultLike(),
      supplier: {
        ...baseParties.supplier,
        country: 'DE',
        partyId: 'SUP-99',
        legalReg: 'HRB 12345',
        kleinunternehmer: true,
        steuernummer: '12/345/67890',
        taxId: '',
      },
      customer: {
        ...baseParties.customer,
        country: 'DE',
        partyId: 'CUST-7',
      },
    }) as {
      supplier: { identities: Array<Record<string, string>>; tax_id?: unknown }
      customer: { identities: Array<Record<string, string>> }
    }
    expect(doc.supplier.tax_id).toBeUndefined()
    expect(doc.supplier.identities).toEqual([
      { code: 'SUP-99' },
      { scope: 'legal', code: 'HRB 12345' },
      { scope: 'tax', country: 'DE', code: '12/345/67890' },
    ])
    expect(doc.customer.identities).toEqual([{ code: 'CUST-7' }])
  })

  it('emits remittance ref (BT-83) and terms notes (BT-20) alongside due_dates', () => {
    const doc = toGobl({
      ...defaultLike(),
      dueDate: '2026-09-30',
      remittanceRef: 'R-1 / PO-42',
      paymentTermsNotes: 'Skonto 2% binnen 10 Tagen',
    }) as {
      payment: {
        instructions: { ref?: string }
        terms: { due_dates: Array<{ date: string }>; notes?: string }
      }
    }
    expect(doc.payment.instructions.ref).toBe('R-1 / PO-42')
    expect(doc.payment.terms.due_dates).toEqual([{ date: '2026-09-30' }])
    expect(doc.payment.terms.notes).toBe('Skonto 2% binnen 10 Tagen')
  })

  it('emits discounts and previewTotals applies them to Brutto', () => {
    const state: InvoiceFormState = {
      ...defaultLike(),
      discounts: [{ id: '1', reason: 'Promo', mode: 'percent', value: '10' }],
    }
    const doc = toGobl(state) as {
      discounts: Array<{ reason: string; percent?: string; amount?: string }>
    }
    expect(doc.discounts).toEqual([
      {
        reason: 'Promo',
        percent: '10%',
        base: '100.00',
        taxes: [{ cat: 'VAT', rate: 'general' }],
      },
    ])
    expect(doc.discounts[0]).not.toHaveProperty('amount')

    const before = previewTotals({ ...state, discounts: [] })
    const after = previewTotals(state)
    expect(after.net).toBeCloseTo(before.net * 0.9)
    expect(after.gross).toBeLessThan(before.gross)
    expect(after.gross).toBeCloseTo(before.gross * 0.9)
  })

  it('country select drives tax_id.country and addresses[0].country; default DE', () => {
    const de = toGobl(defaultLike()) as {
      supplier: { tax_id: { country: string }; addresses: Array<{ country: string }> }
    }
    expect(de.supplier.tax_id.country).toBe('DE')
    expect(de.supplier.addresses[0].country).toBe('DE')

    const at = toGobl({
      ...defaultLike(),
      supplier: { ...baseParties.supplier, country: 'AT' },
      customer: { ...baseParties.customer, country: 'AT' },
    }) as {
      supplier: { tax_id: { country: string }; addresses: Array<{ country: string }> }
      customer: { tax_id: { country: string }; addresses: Array<{ country: string }> }
    }
    expect(at.supplier.tax_id.country).toBe('AT')
    expect(at.supplier.addresses[0].country).toBe('AT')
    expect(at.customer.tax_id.country).toBe('AT')
    expect(at.customer.addresses[0].country).toBe('AT')
  })

  it('bare optional fields stay byte-identical to Phase 9 goldens', () => {
    const state: InvoiceFormState = {
      code: 'R-2026-MIX',
      issueDate: '2026-03-01',
      currency: 'EUR',
      reverseCharge: false,
      buyerRef: 'PO-42',
      docType: 'standard',
      attachments: [],
      paymentMethod: 'credit-transfer',
      iban: 'DE89370400440532013000',
      discounts: [],
      ...baseParties,
      lines: [
        { id: '1', name: 'Consulting', quantity: '1', price: '100.00', vatRate: '19' },
        { id: '2', name: 'Books', quantity: '2', price: '20.00', vatRate: '7' },
      ],
    }
    expect(toGobl(state)).toEqual(loadGolden('gobl-mixed-19-7.json'))
  })
})
