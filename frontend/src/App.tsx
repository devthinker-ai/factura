import { BrowserRouter, Route, Routes } from 'react-router-dom'
import { AppShell } from '@/components/AppShell'
import { AuthProvider } from '@/components/AuthProvider'
import { I18nProvider } from '@/components/I18nProvider'
import { ThemeProvider } from '@/components/ThemeProvider'
import { InboxPage } from '@/pages/InboxPage'
import { InvoiceDetailPage } from '@/pages/InvoiceDetailPage'
import { LicensePage } from '@/pages/LicensePage'
import { LoginPage } from '@/pages/LoginPage'
import { NewInvoicePage } from '@/pages/NewInvoicePage'
import { BrandingPage } from '@/pages/BrandingPage'
import { PeppolLedgerPage } from '@/pages/PeppolLedgerPage'
import { PeppolSettingsPage } from '@/pages/PeppolSettingsPage'
import { SecurityPage } from '@/pages/SecurityPage'
import { UsersPage } from '@/pages/UsersPage'
import { ToastProvider } from '@/components/ui/sonner'

export default function App() {
  return (
    <ThemeProvider>
      <I18nProvider>
        <BrowserRouter basename="/app">
          <AuthProvider>
            <ToastProvider>
              <Routes>
                <Route path="login" element={<LoginPage />} />
                <Route element={<AppShell />}>
                  <Route index element={<InboxPage />} />
                  <Route path="inbox" element={<InboxPage />} />
                  <Route path="invoices/:id" element={<InvoiceDetailPage />} />
                  <Route path="new" element={<NewInvoicePage />} />
                  <Route path="peppol" element={<PeppolLedgerPage />} />
                  <Route path="settings/license" element={<LicensePage />} />
                  <Route path="settings/peppol" element={<PeppolSettingsPage />} />
                  <Route path="settings/branding" element={<BrandingPage />} />
                  <Route path="settings/security" element={<SecurityPage />} />
                  <Route path="settings/users" element={<UsersPage />} />
                </Route>
              </Routes>
            </ToastProvider>
          </AuthProvider>
        </BrowserRouter>
      </I18nProvider>
    </ThemeProvider>
  )
}
