import '@testing-library/jest-dom/vitest'

/**
 * Some jsdom versions (v26 here) expose a bare `localStorage` object with no
 * Storage methods (getItem/setItem/removeItem/clear all undefined). Install a
 * minimal in-memory Storage when the real one is missing so tests that read or
 * clear tokens don't crash on `localStorage.removeItem is not a function`.
 */
if (typeof localStorage !== 'undefined' && typeof localStorage.getItem !== 'function') {
  const store = new Map<string, string>()
  const memoryStorage: Storage = {
    get length() {
      return store.size
    },
    clear() {
      store.clear()
    },
    getItem(key: string) {
      return store.has(key) ? store.get(key)! : null
    },
    key(index: number) {
      return Array.from(store.keys())[index] ?? null
    },
    removeItem(key: string) {
      store.delete(key)
    },
    setItem(key: string, value: string) {
      store.set(key, String(value))
    },
  }
  ;(globalThis as any).localStorage = memoryStorage
  if (typeof (globalThis as any).window !== 'undefined') {
    ;(globalThis as any).window.localStorage = memoryStorage
  }
}

/** jsdom File/Blob often lack arrayBuffer(); polyfill for upload tests. */
if (typeof Blob !== 'undefined' && typeof Blob.prototype.arrayBuffer !== 'function') {
  Blob.prototype.arrayBuffer = function arrayBuffer(this: Blob) {
    return new Promise((resolve, reject) => {
      const reader = new FileReader()
      reader.onload = () => resolve(reader.result as ArrayBuffer)
      reader.onerror = () => reject(reader.error ?? new Error('FileReader failed'))
      reader.readAsArrayBuffer(this)
    })
  }
}
