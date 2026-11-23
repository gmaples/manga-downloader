// Numbers in the language's own style. Chapter numbers stay as they are.

/** A count with the language's digit grouping, e.g. 1,234 in English, 1.234 in Spanish. */
export function count(n: number, lang: string): string {
  return new Intl.NumberFormat(lang).format(n);
}

/** A 0-1 fraction as a whole percentage. */
export function percent(fraction: number, lang: string): string {
  return new Intl.NumberFormat(lang, { style: 'percent' }).format(fraction);
}

/** Size units in the languages that write them their own way; the rest keep B, kB, MB, … */
const SIZE_UNITS: Partial<Record<string, Record<string, string>>> = {
  uk: { '': 'Б', k: 'КБ', M: 'МБ', G: 'ГБ', T: 'ТБ', P: 'ПБ', E: 'ЕБ' },
};

/**
 * A size in bytes the way `lang` reads it, in the Go side's 1000-based steps:
 * 912 B, 6.4 GB, and «6,4 ГБ» in Ukrainian.
 */
export function formatSize(bytes: number, lang: string): string {
  let number = String(bytes);
  let prefix = '';
  if (bytes >= 1000) {
    const prefixes = 'kMGTPE';
    let v = bytes / 1000;
    let i = 0;
    while (v >= 1000 && i < prefixes.length - 1) {
      v /= 1000;
      i++;
    }
    number = v.toFixed(1);
    prefix = prefixes[i];
  }
  const units = SIZE_UNITS[lang];
  if (!units) return `${number} ${prefix}B`;
  return `${number.replace('.', ',')} ${units[prefix]}`;
}
