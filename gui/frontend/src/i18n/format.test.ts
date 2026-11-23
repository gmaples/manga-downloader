import { describe, expect, it } from 'vitest';
import { count, formatSize, percent } from './format';

describe('count and percent', () => {
  it('groups digits in the language style', () => {
    expect(count(1234567, 'en')).toBe('1,234,567');
    expect(count(1234567, 'es')).toBe('1.234.567');
  });

  it('formats a fraction as a percentage', () => {
    expect(percent(0.25, 'en')).toBe('25%');
  });
});

describe('formatSize', () => {
  it('writes sizes in the same steps as the Go side', () => {
    expect(formatSize(912, 'en')).toBe('912 B');
    expect(formatSize(912_400, 'en')).toBe('912.4 kB');
    expect(formatSize(1_400_000_000, 'en')).toBe('1.4 GB');
  });

  it('writes sizes the Ukrainian way', () => {
    expect(formatSize(5, 'uk')).toBe('5 Б');
    expect(formatSize(912_400, 'uk')).toBe('912,4 КБ');
    expect(formatSize(12_300_000, 'uk')).toBe('12,3 МБ');
    expect(formatSize(6_400_000_000, 'uk')).toBe('6,4 ГБ');
    expect(formatSize(1_000_000_000_000, 'uk')).toBe('1,0 ТБ');
  });

  it('writes other languages as the Go side does', () => {
    for (const lang of ['en', 'es', 'ca', 'ja']) expect(formatSize(6_400_000_000, lang)).toBe('6.4 GB');
  });
});
