import '@fontsource-variable/inter'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import './index.css'

/* Theme class is applied by the inline <head> script (no flash) and
 * kept in sync by ThemeProvider. Language is owned by I18nProvider. */

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
