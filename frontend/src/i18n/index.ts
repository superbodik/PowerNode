import { en } from './locales/en';
import { ru } from './locales/ru';

export type Locale = 'en' | 'ru';
export type Dict = typeof en;
export type Key = keyof Dict;

const dictionaries: Record<Locale, Dict> = { en, ru };

function detectLocale(): Locale {
  const navLang =
    (typeof navigator !== 'undefined' && (navigator.language || navigator.languages?.[0])) || 'en';
  return navLang.toLowerCase().startsWith('ru') ? 'ru' : 'en';
}

export const locale: Locale = detectLocale();
const dict = dictionaries[locale];

if (import.meta.env.DEV) {
  const missing = Object.keys(en).filter((k) => !(k in ru));
  if (missing.length > 0) {
    // eslint-disable-next-line no-console
    console.warn('[i18n] keys missing from ru dictionary:', missing);
  }
}

/** Translate a key, optionally interpolating `{name}`-style placeholders. */
export function t(key: Key, params?: Record<string, string | number>): string {
  let str = dict[key] ?? en[key] ?? key;
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      str = str.split(`{${k}}`).join(String(v));
    }
  }
  return str;
}
