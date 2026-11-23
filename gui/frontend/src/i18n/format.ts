// Numbers in the language's own style. Chapter numbers stay as they are.

/** A count with the language's digit grouping, e.g. 1,234 in English, 1.234 in Spanish. */
export function count(n: number, lang: string): string {
  return new Intl.NumberFormat(lang).format(n);
}

/** A 0-1 fraction as a whole percentage. */
export function percent(fraction: number, lang: string): string {
  return new Intl.NumberFormat(lang, { style: 'percent' }).format(fraction);
}

/** Size units in the languages that write them their own way; the rest keep sizeText's. */
const SIZE_UNITS: Partial<Record<string, Record<string, string>>> = {
  uk: { '': 'Б', k: 'КБ', M: 'МБ', G: 'ГБ', T: 'ТБ', P: 'ПБ', E: 'ЕБ' },
};

/**
 * A size as sizeText writes it (912 B, 6.4 GB) the way `lang` reads it:
 * «6,4 ГБ» in Ukrainian. Other languages, and text in any other shape, stay as they are.
 */
export function size(text: string, lang: string): string {
  const units = SIZE_UNITS[lang];
  const m = /^(\d+)(?:\.(\d))? ([kMGTPE]?)B$/.exec(text);
  if (!units || !m) return text;
  const [, whole, tenth, prefix] = m;
  const number = tenth === undefined ? whole : `${whole},${tenth}`;
  return `${number} ${units[prefix]}`;
}
