import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { ChevronDown, ChevronUp, Download, Plus, Trash2 } from 'lucide-react'
import * as api from '@/lib/api'
import { ApiError, type GenerateResponse } from '@/lib/api'
import {
  COUNTRY_OPTIONS,
  defaultFormState,
  hintLeitwegID,
  hintUStIdNr,
  isAllowedAttachmentMIME,
  MAX_ATTACHMENTS,
  mimeForFile,
  previewTotals,
  toGobl,
  type DiscountForm,
  type DocTypeChoice,
  type InvoiceFormState,
  type LineForm,
  type PartyForm,
  type PaymentMethodChoice,
  type VatRateChoice,
} from '@/lib/gobl'
import { useT } from '@/components/I18nProvider'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { Badge } from '@/components/ui/badge'
import { toast } from '@/components/ui/sonner'
import { downloadBlob, formatMoney } from '@/lib/utils'

function PartyFields({
  title,
  party,
  onChange,
  role,
}: {
  title: string
  party: PartyForm
  onChange: (p: PartyForm) => void
  /** Seller: phone required (BT-42). Buyer: email required (BT-49). */
  role: 'seller' | 'buyer'
}) {
  const t = useT()
  const hintInfo = !party.kleinunternehmer ? hintUStIdNr(party.taxId) : null
  const hint = hintInfo
    ? t(`new.hint.${hintInfo.key}`, hintInfo.length != null ? { length: hintInfo.length } : undefined)
    : null
  const set = (k: keyof PartyForm, v: string | boolean) => onChange({ ...party, [k]: v })
  const phoneRequired = role === 'seller'
  const emailRequired = role === 'buyer'
  return (
    <fieldset className="space-y-2">
      <legend className="text-sm font-semibold">{title}</legend>
      <div className="grid gap-2 sm:grid-cols-2">
        <div className="sm:col-span-2">
          <Label>{t('new.party.name')}</Label>
          <Input value={party.name} onChange={(e) => set('name', e.target.value)} />
        </div>
        <div className="sm:col-span-2">
          <Label>{t('new.party.street')}</Label>
          <Input value={party.street} onChange={(e) => set('street', e.target.value)} />
        </div>
        <div>
          <Label>{t('new.party.plz')}</Label>
          <Input value={party.code} onChange={(e) => set('code', e.target.value)} />
        </div>
        <div>
          <Label>{t('new.party.city')}</Label>
          <Input value={party.locality} onChange={(e) => set('locality', e.target.value)} />
        </div>
        <div>
          <Label htmlFor={`party-country-${role}`}>{t('new.party.country')}</Label>
          <Select
            id={`party-country-${role}`}
            value={party.country || 'DE'}
            onChange={(e) => set('country', e.target.value)}
          >
            {COUNTRY_OPTIONS.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </Select>
        </div>
        <div>
          <Label htmlFor={`party-id-${role}`}>
            {role === 'seller' ? t('new.party.party_id_seller') : t('new.party.party_id_buyer')}
          </Label>
          <Input
            id={`party-id-${role}`}
            value={party.partyId || ''}
            onChange={(e) => set('partyId', e.target.value)}
          />
        </div>
        {role === 'seller' && (
          <div className="sm:col-span-2 flex items-center gap-2 pb-1">
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                checked={!!party.kleinunternehmer}
                onChange={(e) => set('kleinunternehmer', e.target.checked)}
              />
              {t('new.party.kleinunternehmer')}
            </label>
          </div>
        )}
        {party.kleinunternehmer && role === 'seller' ? (
          <div className="sm:col-span-2">
            <Label htmlFor={`party-steuernummer-${role}`}>{t('new.party.steuernummer')}</Label>
            <Input
              id={`party-steuernummer-${role}`}
              value={party.steuernummer || ''}
              onChange={(e) => set('steuernummer', e.target.value)}
              placeholder="12/345/67890"
            />
            <p className="mt-0.5 text-[11px] text-muted-foreground">{t('new.party.steuernummer.hint')}</p>
          </div>
        ) : (
          <div>
            <Label>{t('new.party.tax_id')}</Label>
            <Input
              value={party.taxId}
              onChange={(e) => set('taxId', e.target.value)}
              placeholder="DE…"
              aria-invalid={!!hint}
            />
            {hint && (
              <p className="mt-0.5 text-[11px] text-amber-700 dark:text-amber-400">{hint}</p>
            )}
          </div>
        )}
        {role === 'seller' && (
          <div>
            <Label htmlFor="party-legal-reg">{t('new.party.legal_reg')}</Label>
            <Input
              id="party-legal-reg"
              value={party.legalReg || ''}
              onChange={(e) => set('legalReg', e.target.value)}
              placeholder="HRB 12345"
            />
          </div>
        )}
        <div>
          <Label htmlFor={`party-contact-${role}`}>{t('new.party.contact')}</Label>
          <Input
            id={`party-contact-${role}`}
            value={party.contactName || ''}
            onChange={(e) => set('contactName', e.target.value)}
          />
        </div>
        <div>
          <Label htmlFor={`party-phone-${role}`}>
            {phoneRequired ? t('new.party.phone_required') : t('new.party.phone')}
          </Label>
          <Input
            id={`party-phone-${role}`}
            type="tel"
            value={party.phone || ''}
            onChange={(e) => set('phone', e.target.value)}
            required={phoneRequired}
            aria-required={phoneRequired}
          />
        </div>
        <div>
          <Label htmlFor={`party-email-${role}`}>
            {emailRequired ? t('new.party.email_required') : t('new.party.email')}
          </Label>
          <Input
            id={`party-email-${role}`}
            type="email"
            value={party.email || ''}
            onChange={(e) => set('email', e.target.value)}
            required={emailRequired}
            aria-required={emailRequired}
          />
        </div>
      </div>
    </fieldset>
  )
}

function CollapsibleSection({
  title,
  testId,
  children,
  defaultOpen = false,
}: {
  title: string
  testId: string
  children: ReactNode
  defaultOpen?: boolean
}) {
  const [open, setOpen] = useState(defaultOpen)
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          <button
            type="button"
            className="flex items-center gap-1"
            data-testid={testId}
            aria-expanded={open}
            onClick={() => setOpen((o) => !o)}
          >
            {open ? <ChevronUp className="h-3.5 w-3.5" /> : <ChevronDown className="h-3.5 w-3.5" />}
            {title}
          </button>
        </CardTitle>
      </CardHeader>
      {open && <CardContent className="grid gap-2 sm:grid-cols-2">{children}</CardContent>}
    </Card>
  )
}

function b64ToBlob(b64: string, type: string): Blob {
  const bin = atob(b64)
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
  return new Blob([bytes], { type })
}

export function NewInvoicePage() {
  const t = useT()
  const [state, setState] = useState<InvoiceFormState>(defaultFormState)
  const [jsonOpen, setJsonOpen] = useState(false)
  const [archive, setArchive] = useState(true)
  const [busy, setBusy] = useState(false)
  const [result, setResult] = useState<GenerateResponse | null>(null)
  const [genError, setGenError] = useState<string | null>(null)
  const [archivedId, setArchivedId] = useState<number | null>(null)

  useEffect(() => {
    api
      .listCompanies()
      .then((list) => {
        const def = list.find((c) => c.is_default) || list[0]
        if (!def) return
        setState((s) => ({
          ...s,
          supplier: {
            ...s.supplier,
            name: def.name || s.supplier.name,
            taxId: def.seller_vat_id || s.supplier.taxId,
          },
        }))
      })
      .catch(() => {
        /* prefill is best-effort */
      })
  }, [])

  const gobl = useMemo(() => toGobl(state), [state])
  const totals = useMemo(() => previewTotals(state), [state])
  const goblJson = useMemo(() => JSON.stringify(gobl, null, 2), [gobl])

  const updateLine = (id: string, patch: Partial<LineForm>) => {
    setState((s) => ({
      ...s,
      lines: s.lines.map((l) => (l.id === id ? { ...l, ...patch } : l)),
    }))
  }

  const addLine = () => {
    setState((s) => ({
      ...s,
      lines: [
        ...s.lines,
        {
          id: String(Date.now()),
          name: '',
          quantity: '1',
          price: '0.00',
          vatRate: '19' as VatRateChoice,
        },
      ],
    }))
  }

  const removeLine = (id: string) => {
    setState((s) => ({
      ...s,
      lines: s.lines.length <= 1 ? s.lines : s.lines.filter((l) => l.id !== id),
    }))
  }

  const needsPreceding =
    state.docType === 'credit-note' || state.docType === 'corrective'

  const onAddAttachments = async (files: FileList | null) => {
    if (!files?.length) return
    const room = MAX_ATTACHMENTS - state.attachments.length
    if (room <= 0) {
      toast({ title: t('new.attachments.cap'), variant: 'warning' })
      return
    }
    const all = Array.from(files)
    const accepted: File[] = []
    for (const f of all) {
      const mime = mimeForFile(f)
      if (!isAllowedAttachmentMIME(mime)) {
        toast({
          title: t('new.attachments.mime'),
          description: f.name,
          variant: 'warning',
        })
        continue
      }
      accepted.push(f)
    }
    const picked = accepted.slice(0, room)
    if (accepted.length > room) {
      toast({ title: t('new.attachments.cap'), variant: 'warning' })
    }
    if (!picked.length) return
    const added = await Promise.all(
      picked.map(
        (f) =>
          new Promise<{
            id: string
            name: string
            description: string
            mime: string
            dataUri: string
          }>((resolve, reject) => {
            const reader = new FileReader()
            reader.onload = () => {
              const dataUri = String(reader.result || '')
              resolve({
                id: `${Date.now()}-${f.name}`,
                name: f.name,
                description: '',
                mime: mimeForFile(f),
                dataUri,
              })
            }
            reader.onerror = () => reject(reader.error)
            reader.readAsDataURL(f)
          }),
      ),
    )
    setState((s) => ({ ...s, attachments: [...s.attachments, ...added] }))
  }

  const generate = async () => {
    setBusy(true)
    setGenError(null)
    setResult(null)
    setArchivedId(null)
    try {
      const res = await api.generate(gobl, { format: 'both' })
      setResult(res)
      if (archive && res.xrechnung) {
        try {
          const enc = new TextEncoder().encode(res.xrechnung)
          const ing = await api.ingest(enc, {
            externalId: state.code.trim() || undefined,
            source: 'web-generate',
          })
          setArchivedId(ing.id)
          toast({
            title: t('new.archived'),
            description: (
              <span>
                {t('new.archive_hint')}{' '}
                <a className="underline" href={`/app/invoices/${ing.id}`}>
                  {t('new.invoice_link', { id: ing.id })}
                </a>
              </span>
            ),
            variant: 'success',
          })
        } catch (e) {
          const msg =
            e instanceof ApiError
              ? `${e.message}${typeof e.body === 'object' && e.body && 'id' in e.body ? ` (id ${(e.body as { id: number }).id})` : ''}`
              : t('new.archive_failed')
          toast({ title: t('new.archive_failed'), description: msg, variant: 'warning' })
        }
      }
    } catch (e) {
      if (e instanceof ApiError) {
        const body = e.body as { error?: string; detail?: string }
        setGenError(body?.detail || body?.error || e.message)
      } else {
        setGenError(t('new.generate_failed'))
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="grid gap-4 p-4 lg:grid-cols-[1.4fr_1fr]">
      <div className="space-y-4">
        <h1 className="text-lg font-semibold tracking-tight">{t('new.title')}</h1>

        <Card>
          <CardHeader>
            <CardTitle>{t('new.parties')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-6">
            <PartyFields
              title={t('new.seller')}
              party={state.supplier}
              role="seller"
              onChange={(supplier) => setState((s) => ({ ...s, supplier }))}
            />
            <PartyFields
              title={t('new.buyer')}
              party={state.customer}
              role="buyer"
              onChange={(customer) => setState((s) => ({ ...s, customer }))}
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t('new.meta')}</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-2 sm:grid-cols-2">
            <div>
              <Label htmlFor="inv-doc-type">{t('new.doc_type')}</Label>
              <Select
                id="inv-doc-type"
                value={state.docType}
                onChange={(e) => {
                  const docType = e.target.value as DocTypeChoice
                  setState((s) => ({
                    ...s,
                    docType,
                    preceding:
                      docType === 'credit-note' || docType === 'corrective'
                        ? s.preceding ?? { code: '', issueDate: s.issueDate }
                        : undefined,
                  }))
                }}
              >
                <option value="standard">{t('new.doc_type.standard')}</option>
                <option value="credit-note">{t('new.doc_type.credit_note')}</option>
                <option value="corrective">{t('new.doc_type.corrective')}</option>
                <option value="self-billed">{t('new.doc_type.self_billed')}</option>
              </Select>
            </div>
            <div>
              <Label htmlFor="inv-code">{t('new.invoice_number')}</Label>
              <Input
                id="inv-code"
                value={state.code}
                onChange={(e) => setState((s) => ({ ...s, code: e.target.value }))}
              />
            </div>
            {needsPreceding && (
              <>
                <div>
                  <Label htmlFor="inv-preceding-code">{t('new.preceding.code')}</Label>
                  <Input
                    id="inv-preceding-code"
                    value={state.preceding?.code || ''}
                    onChange={(e) =>
                      setState((s) => ({
                        ...s,
                        preceding: {
                          code: e.target.value,
                          issueDate: s.preceding?.issueDate || s.issueDate,
                        },
                      }))
                    }
                    required
                    aria-required
                  />
                </div>
                <div>
                  <Label htmlFor="inv-preceding-date">{t('new.preceding.date')}</Label>
                  <Input
                    id="inv-preceding-date"
                    type="date"
                    value={state.preceding?.issueDate || ''}
                    onChange={(e) =>
                      setState((s) => ({
                        ...s,
                        preceding: {
                          code: s.preceding?.code || '',
                          issueDate: e.target.value,
                        },
                      }))
                    }
                  />
                </div>
              </>
            )}
            <div>
              <Label>{t('new.buyer_ref')}</Label>
              <Input
                value={state.buyerRef || ''}
                onChange={(e) => setState((s) => ({ ...s, buyerRef: e.target.value }))}
                placeholder="NA"
              />
            </div>
            <div>
              <Label htmlFor="inv-leitweg">{t('new.leitweg')}</Label>
              <Input
                id="inv-leitweg"
                value={state.leitwegId || ''}
                onChange={(e) => setState((s) => ({ ...s, leitwegId: e.target.value }))}
                placeholder="04011000-12345-34"
                aria-invalid={!!state.leitwegId && !hintLeitwegID(state.leitwegId)}
              />
              <p className="mt-0.5 text-[11px] text-muted-foreground">{t('new.leitweg.hint')}</p>
            </div>
            <div>
              <Label>{t('new.issue_date')}</Label>
              <Input
                type="date"
                value={state.issueDate}
                onChange={(e) => setState((s) => ({ ...s, issueDate: e.target.value }))}
              />
            </div>
            <div>
              <Label>{t('new.due_date')}</Label>
              <Input
                type="date"
                value={state.dueDate || ''}
                onChange={(e) => setState((s) => ({ ...s, dueDate: e.target.value }))}
              />
            </div>
            <div>
              <Label>{t('new.currency')}</Label>
              <Input value={state.currency} disabled />
            </div>
            <div className="flex items-end gap-2 pb-1">
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={state.reverseCharge}
                  onChange={(e) =>
                    setState((s) => ({ ...s, reverseCharge: e.target.checked }))
                  }
                />
                {t('new.reverse_charge')}
              </label>
            </div>
            <div className="sm:col-span-2">
              <Label>{t('new.notes')}</Label>
              <Textarea
                value={state.notes || ''}
                onChange={(e) => setState((s) => ({ ...s, notes: e.target.value }))}
              />
            </div>
            <div className="sm:col-span-2 space-y-2">
              <Label htmlFor="inv-attachments">{t('new.attachments')}</Label>
              <Input
                id="inv-attachments"
                type="file"
                multiple
                accept=".pdf,.png,.jpg,.jpeg,.csv,.xlsx,.ods,application/pdf,image/jpeg,image/png,text/csv"
                onChange={(e) => {
                  void onAddAttachments(e.target.files)
                  e.target.value = ''
                }}
              />
              <p className="text-[11px] text-muted-foreground">
                {t('new.attachments.hint', { n: state.attachments.length, max: MAX_ATTACHMENTS })}
              </p>
              {state.attachments.length > 0 && (
                <ul className="space-y-1 text-sm">
                  {state.attachments.map((a) => (
                    <li key={a.id} className="flex items-center justify-between gap-2">
                      <span className="truncate font-mono text-xs">{a.name}</span>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        onClick={() =>
                          setState((s) => ({
                            ...s,
                            attachments: s.attachments.filter((x) => x.id !== a.id),
                          }))
                        }
                      >
                        {t('new.attachments.remove')}
                      </Button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t('new.payment')}</CardTitle>
          </CardHeader>
          <CardContent className="grid gap-2 sm:grid-cols-2">
            <div className="sm:col-span-2">
              <Label htmlFor="inv-pay-method">{t('new.payment.method')}</Label>
              <Select
                id="inv-pay-method"
                value={state.paymentMethod}
                onChange={(e) =>
                  setState((s) => ({
                    ...s,
                    paymentMethod: e.target.value as PaymentMethodChoice,
                  }))
                }
              >
                <option value="credit-transfer">{t('new.payment.credit_transfer')}</option>
                <option value="direct-debit">{t('new.payment.direct_debit')}</option>
              </Select>
            </div>
            {state.paymentMethod === 'credit-transfer' ? (
              <>
                <div className="sm:col-span-2">
                  <Label htmlFor="inv-iban">{t('new.iban')}</Label>
                  <Input
                    id="inv-iban"
                    value={state.iban || ''}
                    onChange={(e) => setState((s) => ({ ...s, iban: e.target.value }))}
                    className="font-mono text-xs"
                  />
                </div>
                <div>
                  <Label htmlFor="inv-bic">{t('new.payment.bic')}</Label>
                  <Input
                    id="inv-bic"
                    value={state.bic || ''}
                    onChange={(e) => setState((s) => ({ ...s, bic: e.target.value }))}
                    className="font-mono text-xs"
                    placeholder="COBADEFFXXX"
                  />
                </div>
                <div>
                  <Label htmlFor="inv-holder">{t('new.payment.account_holder')}</Label>
                  <Input
                    id="inv-holder"
                    value={state.accountHolder || ''}
                    onChange={(e) => setState((s) => ({ ...s, accountHolder: e.target.value }))}
                  />
                </div>
              </>
            ) : (
              <>
                <div>
                  <Label htmlFor="inv-dd-ref">{t('new.payment.dd_mandate')}</Label>
                  <Input
                    id="inv-dd-ref"
                    value={state.ddMandateRef || ''}
                    onChange={(e) => setState((s) => ({ ...s, ddMandateRef: e.target.value }))}
                  />
                </div>
                <div>
                  <Label htmlFor="inv-dd-creditor">{t('new.payment.dd_creditor')}</Label>
                  <Input
                    id="inv-dd-creditor"
                    value={state.ddCreditorId || ''}
                    onChange={(e) => setState((s) => ({ ...s, ddCreditorId: e.target.value }))}
                    placeholder="DE98ZZZ09999999999"
                  />
                </div>
                <div className="sm:col-span-2">
                  <Label htmlFor="inv-dd-iban">{t('new.iban')}</Label>
                  <Input
                    id="inv-dd-iban"
                    value={state.iban || ''}
                    onChange={(e) => setState((s) => ({ ...s, iban: e.target.value }))}
                    className="font-mono text-xs"
                  />
                </div>
              </>
            )}
            <div className="sm:col-span-2">
              <Label htmlFor="inv-already-paid">{t('new.payment.already_paid')}</Label>
              <Input
                id="inv-already-paid"
                value={state.alreadyPaid || ''}
                onChange={(e) => setState((s) => ({ ...s, alreadyPaid: e.target.value }))}
                className="tabular-nums"
                placeholder="0.00"
              />
              <p className="mt-0.5 text-[11px] text-muted-foreground">
                {t('new.payment.already_paid.hint')}
              </p>
            </div>
            <div className="sm:col-span-2">
              <Label htmlFor="inv-remittance">{t('new.payment.remittance')}</Label>
              <Input
                id="inv-remittance"
                value={state.remittanceRef || ''}
                onChange={(e) => setState((s) => ({ ...s, remittanceRef: e.target.value }))}
              />
            </div>
            <div className="sm:col-span-2">
              <Label htmlFor="inv-terms-notes">{t('new.payment.terms_notes')}</Label>
              <Textarea
                id="inv-terms-notes"
                value={state.paymentTermsNotes || ''}
                onChange={(e) => setState((s) => ({ ...s, paymentTermsNotes: e.target.value }))}
                placeholder={t('new.payment.terms_notes.placeholder')}
              />
            </div>
          </CardContent>
        </Card>

        <CollapsibleSection title={t('new.refs')} testId="section-refs">
          <div>
            <Label htmlFor="inv-project">{t('new.refs.project')}</Label>
            <Input
              id="inv-project"
              value={state.projectRef || ''}
              onChange={(e) => setState((s) => ({ ...s, projectRef: e.target.value }))}
            />
          </div>
          <div>
            <Label htmlFor="inv-contract">{t('new.refs.contract')}</Label>
            <Input
              id="inv-contract"
              value={state.contractRef || ''}
              onChange={(e) => setState((s) => ({ ...s, contractRef: e.target.value }))}
            />
          </div>
          <div>
            <Label htmlFor="inv-po">{t('new.refs.purchase')}</Label>
            <Input
              id="inv-po"
              value={state.purchaseOrder || ''}
              onChange={(e) => setState((s) => ({ ...s, purchaseOrder: e.target.value }))}
            />
          </div>
          <div>
            <Label htmlFor="inv-so">{t('new.refs.sales')}</Label>
            <Input
              id="inv-so"
              value={state.salesOrder || ''}
              onChange={(e) => setState((s) => ({ ...s, salesOrder: e.target.value }))}
            />
          </div>
          <div>
            <Label htmlFor="inv-tender">{t('new.refs.tender')}</Label>
            <Input
              id="inv-tender"
              value={state.tenderRef || ''}
              onChange={(e) => setState((s) => ({ ...s, tenderRef: e.target.value }))}
            />
          </div>
          <div>
            <Label htmlFor="inv-cost">{t('new.refs.cost')}</Label>
            <Input
              id="inv-cost"
              value={state.accountingCost || ''}
              onChange={(e) => setState((s) => ({ ...s, accountingCost: e.target.value }))}
            />
          </div>
        </CollapsibleSection>

        <CollapsibleSection title={t('new.period')} testId="section-period">
          <div>
            <Label htmlFor="inv-delivery-date">{t('new.period.delivery')}</Label>
            <Input
              id="inv-delivery-date"
              type="date"
              value={state.deliveryDate || ''}
              onChange={(e) => setState((s) => ({ ...s, deliveryDate: e.target.value }))}
            />
          </div>
          <div className="hidden sm:block" />
          <div>
            <Label htmlFor="inv-period-start">{t('new.period.start')}</Label>
            <Input
              id="inv-period-start"
              type="date"
              value={state.periodStart || ''}
              onChange={(e) => setState((s) => ({ ...s, periodStart: e.target.value }))}
            />
          </div>
          <div>
            <Label htmlFor="inv-period-end">{t('new.period.end')}</Label>
            <Input
              id="inv-period-end"
              type="date"
              value={state.periodEnd || ''}
              onChange={(e) => setState((s) => ({ ...s, periodEnd: e.target.value }))}
            />
          </div>
        </CollapsibleSection>

        <CollapsibleSection title={t('new.discounts')} testId="section-discounts">
          <div className="sm:col-span-2 space-y-2">
            {(state.discounts || []).map((d) => (
              <div
                key={d.id}
                className="grid gap-2 rounded-md border border-border p-2 sm:grid-cols-[1fr_110px_100px_auto]"
              >
                <div>
                  <Label htmlFor={`disc-reason-${d.id}`}>{t('new.discounts.reason')}</Label>
                  <Input
                    id={`disc-reason-${d.id}`}
                    value={d.reason}
                    onChange={(e) =>
                      setState((s) => ({
                        ...s,
                        discounts: s.discounts.map((x) =>
                          x.id === d.id ? { ...x, reason: e.target.value } : x,
                        ),
                      }))
                    }
                  />
                </div>
                <div>
                  <Label htmlFor={`disc-mode-${d.id}`}>{t('new.discounts.mode')}</Label>
                  <Select
                    id={`disc-mode-${d.id}`}
                    value={d.mode}
                    onChange={(e) =>
                      setState((s) => ({
                        ...s,
                        discounts: s.discounts.map((x) =>
                          x.id === d.id
                            ? { ...x, mode: e.target.value as DiscountForm['mode'] }
                            : x,
                        ),
                      }))
                    }
                  >
                    <option value="percent">{t('new.discounts.percent')}</option>
                    <option value="amount">{t('new.discounts.amount')}</option>
                  </Select>
                </div>
                <div>
                  <Label htmlFor={`disc-value-${d.id}`}>{t('new.discounts.value')}</Label>
                  <Input
                    id={`disc-value-${d.id}`}
                    value={d.value}
                    onChange={(e) =>
                      setState((s) => ({
                        ...s,
                        discounts: s.discounts.map((x) =>
                          x.id === d.id ? { ...x, value: e.target.value } : x,
                        ),
                      }))
                    }
                    className="tabular-nums"
                    placeholder={d.mode === 'percent' ? '10' : '0.00'}
                  />
                </div>
                <div className="flex items-end">
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    aria-label={t('new.discounts.remove')}
                    onClick={() =>
                      setState((s) => ({
                        ...s,
                        discounts: s.discounts.filter((x) => x.id !== d.id),
                      }))
                    }
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
            ))}
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() =>
                setState((s) => ({
                  ...s,
                  discounts: [
                    ...s.discounts,
                    {
                      id: String(Date.now()),
                      reason: '',
                      mode: 'percent',
                      value: '',
                    },
                  ],
                }))
              }
            >
              <Plus className="h-3.5 w-3.5" />
              {t('new.discounts.add')}
            </Button>
          </div>
        </CollapsibleSection>

        <Card>
          <CardHeader
            action={
              <Button type="button" variant="outline" size="sm" onClick={addLine}>
                <Plus className="h-3.5 w-3.5" />
                {t('new.add_line')}
              </Button>
            }
          >
            <CardTitle>{t('new.lines')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {state.lines.map((l) => (
              <div
                key={l.id}
                className="grid gap-2 rounded-md border border-border p-2 sm:grid-cols-[1fr_80px_100px_130px_auto]"
              >
                <div>
                  <Label>{t('new.line.description')}</Label>
                  <Input
                    value={l.name}
                    onChange={(e) => updateLine(l.id, { name: e.target.value })}
                  />
                </div>
                <div>
                  <Label>{t('new.line.qty')}</Label>
                  <Input
                    value={l.quantity}
                    onChange={(e) => updateLine(l.id, { quantity: e.target.value })}
                    className="tabular-nums"
                  />
                </div>
                <div>
                  <Label>{t('new.line.price')}</Label>
                  <Input
                    value={l.price}
                    onChange={(e) => updateLine(l.id, { price: e.target.value })}
                    className="tabular-nums"
                  />
                </div>
                <div>
                  <Label>{t('new.line.vat')}</Label>
                  <Select
                    value={state.reverseCharge ? 'reverse-charge' : l.vatRate}
                    disabled={state.reverseCharge}
                    onChange={(e) =>
                      updateLine(l.id, { vatRate: e.target.value as VatRateChoice })
                    }
                  >
                    <option value="19">{t('new.line.vat_19')}</option>
                    <option value="7">{t('new.line.vat_7')}</option>
                    <option value="0">{t('new.line.vat_0')}</option>
                    <option value="reverse-charge">{t('new.line.vat_rc')}</option>
                  </Select>
                </div>
                <div className="flex items-end">
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    aria-label={t('new.line.remove')}
                    onClick={() => removeLine(l.id)}
                    disabled={state.lines.length <= 1}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </Button>
                </div>
              </div>
            ))}
          </CardContent>
        </Card>

        <div className="flex flex-wrap items-center gap-3">
          <Button onClick={() => void generate()} disabled={busy}>
            {busy ? t('new.generating') : t('new.generate')}
          </Button>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={archive}
              onChange={(e) => setArchive(e.target.checked)}
              aria-label={t('new.archive_check')}
            />
            {t('new.archive_check')}
          </label>
          <p className="text-xs text-muted-foreground">{t('new.archive_hint')}</p>
        </div>

        {(result || genError) && (
          <Card data-testid="generate-result">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                {t('new.result')}
                {result && (
                  <Badge variant={result.self_check ? 'success' : 'destructive'}>
                    {result.self_check ? t('new.self_ok') : t('new.self_fail')}
                  </Badge>
                )}
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-2">
              {genError && (
                <p className="whitespace-pre-wrap text-sm text-amber-800 dark:text-amber-300">
                  {genError}
                </p>
              )}
              {result && (
                <div className="flex flex-wrap gap-2">
                  {result.xrechnung && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() =>
                        downloadBlob(
                          new Blob([result.xrechnung], { type: 'application/xml' }),
                          result.filename_xrechnung || 'xrechnung.xml',
                        )
                      }
                    >
                      <Download className="h-3.5 w-3.5" />
                      {t('new.download_xr')}
                    </Button>
                  )}
                  {result.zugferd_pdf_base64 && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() =>
                        downloadBlob(
                          b64ToBlob(result.zugferd_pdf_base64, 'application/pdf'),
                          result.filename_zugferd || 'zugferd.pdf',
                        )
                      }
                    >
                      <Download className="h-3.5 w-3.5" />
                      {t('new.download_zf')}
                    </Button>
                  )}
                </div>
              )}
              {archivedId != null && (
                <p className="text-xs text-muted-foreground">
                  {t('new.archived_as')}{' '}
                  <Link className="underline" to={`/invoices/${archivedId}`}>
                    {t('new.invoice_link', { id: archivedId })}
                  </Link>
                </p>
              )}
            </CardContent>
          </Card>
        )}
      </div>

      <aside className="space-y-4 lg:sticky lg:top-4 lg:self-start">
        <Card>
          <CardHeader>
            <CardTitle>{t('new.preview')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-1 text-sm">
            <p className="text-xs text-muted-foreground">{t('new.preview_hint')}</p>
            <div className="flex justify-between">
              <span>{t('new.net')}</span>
              <span className="tabular-nums">{formatMoney(totals.net)}</span>
            </div>
            {totals.byRate.map((b) => (
              <div key={b.label} className="flex justify-between text-muted-foreground">
                <span>{t('new.vat_row', { label: b.label })}</span>
                <span className="tabular-nums">{formatMoney(b.vat)}</span>
              </div>
            ))}
            <div className="flex justify-between border-t border-border pt-1 font-medium">
              <span>{t('new.gross')}</span>
              <span className="tabular-nums">{formatMoney(totals.gross)}</span>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>
              <button
                type="button"
                className="flex items-center gap-1"
                onClick={() => setJsonOpen((o) => !o)}
              >
                {jsonOpen ? (
                  <ChevronUp className="h-3.5 w-3.5" />
                ) : (
                  <ChevronDown className="h-3.5 w-3.5" />
                )}
                {t('new.gobl_json')}
              </button>
            </CardTitle>
          </CardHeader>
          {jsonOpen && (
            <CardContent>
              <pre className="max-h-[420px] overflow-auto rounded-md bg-muted/50 p-2 font-mono text-[10px] leading-relaxed">
                {goblJson}
              </pre>
            </CardContent>
          )}
        </Card>
      </aside>
    </div>
  )
}
