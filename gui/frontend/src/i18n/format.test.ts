import { describe, expect, it } from 'vitest';
import { count, percent, size } from './format';

describe('count and percent', () => {
  it('groups digits in the language style', () => {
    expect(count(1234567, 'en')).toBe('1,234,567');
    expect(count(1234567, 'es')).toBe('1.234.567');
  });

  it('formats a fraction as a percentage', () => {
    expect(percent(0.25, 'en')).toBe('25%');
  });
});

describe('size', () => {
  it('writes sizes the Ukrainian way', () => {
    expect(size('5 B', 'uk')).toBe('5 Б');
    expect(size('912.4 kB', 'uk')).toBe('912,4 КБ');
    expect(size('12.3 MB', 'uk')).toBe('12,3 МБ');
    expect(size('6.4 GB', 'uk')).toBe('6,4 ГБ');
    expect(size('1.0 TB', 'uk')).toBe('1,0 ТБ');
  });

  it('leaves other languages as sizeText writes them', () => {
    for (const lang of ['en', 'es', 'ca', 'ja']) expect(size('6.4 GB', lang)).toBe('6.4 GB');
  });

  it('leaves text that is not a size alone', () => {
    expect(size('a lot', 'uk')).toBe('a lot');
    expect(size('6.4 GiB', 'uk')).toBe('6.4 GiB');
  });
});
