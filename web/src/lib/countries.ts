// Country codes (ISO 3166-1 alpha-2) to flags and names in the browser's language.

/** "PT" → 🇵🇹 */
export function flag(code: string): string {
  if (!/^[A-Za-z]{2}$/.test(code)) return '🏳️'
  return String.fromCodePoint(...code.toUpperCase().split('').map(c => 0x1f1e6 + c.charCodeAt(0) - 65))
}

let names: Intl.DisplayNames | null = null
/** "PT" → "Portugal" */
export function countryName(code: string): string {
  try { names ??= new Intl.DisplayNames(undefined, { type: 'region' }); return names.of(code.toUpperCase()) || code } catch { return code }
}

/** Every country code the browser can name, sorted by name — for pickers. */
export function allCountries(): { code: string; name: string }[] {
  const out: { code: string; name: string }[] = []
  for (let a = 65; a <= 90; a++) for (let b = 65; b <= 90; b++) {
    const code = String.fromCharCode(a, b)
    const n = countryName(code)
    if (n && n !== code && !/^[A-Z]{2}$/.test(n)) out.push({ code, name: n })
  }
  return out.sort((x, y) => x.name.localeCompare(y.name))
}
