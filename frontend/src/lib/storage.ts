// Per-device storage. It can be unavailable (private browsing, blocked
// site data), so every access is guarded and the app works without it.

export function load(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

export function save(key: string, value: string | null): void {
  try {
    if (value === null) localStorage.removeItem(key);
    else localStorage.setItem(key, value);
  } catch {
    // The value holds for this page load only.
  }
}
