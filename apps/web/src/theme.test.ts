import { describe, expect, it } from 'vitest';
import { customThemeColorKeys, parseCustomTheme } from './theme';

const validTheme = {
  version: 1,
  id: 'dracula',
  name: 'Dracula',
  appearance: 'dark',
  colors: Object.fromEntries(customThemeColorKeys.map((key) => [key, '#282a36'])),
};

describe('custom themes', () => {
  it('accepts the custom theme format', () => {
    expect(parseCustomTheme(validTheme)).toMatchObject({
      id: 'dracula',
      name: 'Dracula',
      appearance: 'dark',
    });
  });

  it('rejects themes with missing or unsafe colors', () => {
    expect(parseCustomTheme({ ...validTheme, colors: { ...validTheme.colors, canvas: 'red' } })).toBeNull();
    expect(parseCustomTheme({ ...validTheme, version: 2 })).toBeNull();
  });
});
