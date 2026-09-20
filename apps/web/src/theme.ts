export const customThemeStorageKey = 'taskboard-custom-theme';

export const customThemeColorKeys = [
  'canvas',
  'chrome',
  'toolbar',
  'toolbarForeground',
  'toolbarBorder',
  'toolbarControl',
  'toolbarControlForeground',
  'toolbarControlHover',
  'surface',
  'surfaceRaised',
  'surfaceOverlay',
  'text',
  'textMuted',
  'border',
  'input',
  'focus',
  'accent',
  'accentForeground',
  'secondary',
  'secondaryForeground',
  'muted',
  'mutedForeground',
  'placeholder',
  'secondaryLabel',
  'iconMuted',
  'error',
  'errorForeground',
  'errorSurface',
  'warning',
  'warningForeground',
  'warningSurface',
  'update',
  'updateForeground',
  'updateSurface',
  'accentSurface',
  'accentSurfaceForeground',
  'messageSurface',
  'messageForeground',
  'messageAction',
  'messageActionForeground',
  'messageActionHover',
  'codeBackground',
  'codeForeground',
  'sidebar',
  'sidebarForeground',
  'sidebarMutedForeground',
  'sidebarControlSurface',
  'sidebarRowHover',
  'sidebarRowActive',
  'sidebarRowSelected',
  'sidebarBorder',
  'terminalBackground',
  'terminalForeground',
  'terminalCursor',
  'terminalSelection',
  'terminalScrollbar',
  'terminalScrollbarHover',
] as const;

type ThemeColors = Record<(typeof customThemeColorKeys)[number], string>;

export type CustomTheme = {
  version: 1;
  id: string;
  name: string;
  appearance: 'light' | 'dark';
  author?: string;
  description?: string;
  colors: ThemeColors;
};

const cssColor = /^#[\da-f]{3,8}$/i;

export function parseCustomTheme(value: unknown): CustomTheme | null {
  if (!value || typeof value !== 'object') return null;
  const candidate = value as Record<string, unknown>;
  const colors = candidate.colors;
  if (!colors || typeof colors !== 'object') return null;
  const colorRecord = colors as Record<string, unknown>;
  if (
    candidate.version !== 1 ||
    typeof candidate.id !== 'string' ||
    typeof candidate.name !== 'string' ||
    !['light', 'dark'].includes(String(candidate.appearance))
  ) {
    return null;
  }
  if (
    customThemeColorKeys.some(
      (key) => typeof colorRecord[key] !== 'string' || !cssColor.test(colorRecord[key] as string),
    )
  ) {
    return null;
  }
  return {
    version: 1,
    id: candidate.id,
    name: candidate.name,
    appearance: candidate.appearance as 'light' | 'dark',
    ...(typeof candidate.author === 'string' ? { author: candidate.author } : {}),
    ...(typeof candidate.description === 'string' ? { description: candidate.description } : {}),
    colors: colorRecord as ThemeColors,
  };
}

export function readCustomTheme(): CustomTheme | null {
  try {
    const stored = localStorage.getItem(customThemeStorageKey);
    return stored ? parseCustomTheme(JSON.parse(stored)) : null;
  } catch {
    return null;
  }
}

const variableMap: Record<string, string> = {
  background: 'canvas',
  foreground: 'text',
  card: 'surface',
  muted: 'textMuted',
  border: 'border',
  soft: 'surfaceRaised',
  primary: 'accentForeground',
  primaryDark: 'accent',
  accent: 'accentSurface',
  sidebar: 'sidebar',
  input: 'input',
  ring: 'focus',
  secondary: 'secondary',
  secondaryForeground: 'secondaryForeground',
  mutedForeground: 'mutedForeground',
  accentForeground: 'accentSurfaceForeground',
  destructive: 'error',
  destructiveForeground: 'errorForeground',
  popover: 'surfaceOverlay',
  popoverForeground: 'text',
  sidebarForeground: 'sidebarForeground',
  sidebarBorder: 'sidebarBorder',
};

export function applyCustomTheme(theme: CustomTheme | null) {
  const root = document.documentElement;
  for (const variable of Object.keys(variableMap)) root.style.removeProperty(`--${variable}`);
  root.removeAttribute('data-custom-theme');
  if (!theme) return;
  for (const [variable, colorKey] of Object.entries(variableMap)) {
    root.style.setProperty(`--${variable}`, theme.colors[colorKey as keyof ThemeColors]);
  }
  root.style.setProperty('--shadow', `0 12px 40px ${theme.colors.sidebarBorder}`);
  root.setAttribute('data-custom-theme', theme.id);
}

export function saveCustomTheme(theme: CustomTheme) {
  localStorage.setItem(customThemeStorageKey, JSON.stringify(theme));
  applyCustomTheme(theme);
  window.dispatchEvent(new Event('taskboard-custom-theme-change'));
}

export function clearCustomTheme() {
  localStorage.removeItem(customThemeStorageKey);
  applyCustomTheme(null);
  window.dispatchEvent(new Event('taskboard-custom-theme-change'));
}
