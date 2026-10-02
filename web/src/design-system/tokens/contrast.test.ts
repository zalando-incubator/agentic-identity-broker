import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { wcagContrast } from 'culori';
import { parse } from 'postcss';
import { describe, expect, it } from 'vitest';

const stylesheet = parse(readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'theme.css'), 'utf8'));
const surfaces = ['background', 'card', 'muted'];
const statuses = ['destructive', 'success', 'warning', 'info', 'risk-low', 'risk-medium', 'risk-high'];
const roles = [
  'background', 'foreground', 'card', 'card-foreground', 'popover', 'popover-foreground',
  'muted', 'muted-foreground', 'primary', 'primary-foreground', 'secondary',
  'secondary-foreground', 'accent', 'accent-foreground', 'border', 'border-soft',
  'input', 'ring', ...statuses.flatMap((role) => [role, `${role}-foreground`]),
  'sidebar', 'sidebar-foreground', 'sidebar-primary', 'sidebar-primary-foreground',
  'sidebar-accent', 'sidebar-accent-foreground', 'sidebar-border', 'sidebar-ring',
];

type Pair = [foreground: string, background: string, minimum: number];
// 37 pairs per theme reproduce the research's 74 text/control measurements.
const researchPairs: Pair[] = [
  ...surfaces.flatMap((surface): Pair[] => [
    ['foreground', surface, 4.5],
    ['muted-foreground', surface, 4.5],
    ...statuses.map((status): Pair => [status, surface, 4.5]),
    ...['border', 'input', 'ring'].map((control): Pair => [control, surface, 3]),
  ]),
  ['primary-foreground', 'primary', 4.5],
];
const filledStatusPairs: Pair[] = statuses.map((status) => [`${status}-foreground`, status, 4.5]);
const mappedPairs: Pair[] = [
  ['card-foreground', 'card', 4.5],
  ['popover-foreground', 'popover', 4.5],
  ['muted-foreground', 'popover', 4.5],
  ['secondary-foreground', 'secondary', 4.5],
  ['accent-foreground', 'accent', 4.5],
  ...surfaces.map((surface): Pair => ['primary', surface, 4.5]),
  ['sidebar-foreground', 'sidebar', 4.5],
  ['sidebar-primary', 'sidebar', 4.5],
  ['sidebar-primary-foreground', 'sidebar-primary', 4.5],
  ['sidebar-accent-foreground', 'sidebar-accent', 4.5],
  ['muted-foreground', 'sidebar-accent', 4.5],
  ['sidebar-border', 'sidebar', 3],
  ['sidebar-ring', 'sidebar', 3],
  ['sidebar-ring', 'sidebar-accent', 3],
  ['ring', 'popover', 3],
  ['ring', 'secondary', 3],
  ['ring', 'accent', 3],
];

function themeRoles(theme: 'light' | 'dark') {
  const values = new Map<string, string>();
  stylesheet.walkRules((rule) => {
    if (rule.parent?.type !== 'root' || !rule.selectors.includes(`[data-theme="${theme}"]`)) return;
    rule.walkDecls((declaration) => {
      values.set(declaration.prop.slice(2), declaration.value);
    });
  });
  return values;
}

describe.each(['light', 'dark'] as const)('%s semantic contrast', (theme) => {
  const values = themeRoles(theme);

  it.each(roles)('defines an opaque %s color for this theme', (role) => {
    expect(values.get(role), `Missing ${theme} role --${role}`).toMatch(/^oklch\([\d.]+ [\d.]+ [\d.]+\)$/);
  });

  it.each([...researchPairs, ...filledStatusPairs, ...mappedPairs])(
    '%s on %s reaches %s:1',
    (foreground, background, minimum) => {
      const foregroundColor = values.get(foreground);
      const backgroundColor = values.get(background);
      expect(foregroundColor, `Missing ${theme} role --${foreground}`).toBeDefined();
      expect(backgroundColor, `Missing ${theme} role --${background}`).toBeDefined();
      expect(wcagContrast(foregroundColor!, backgroundColor!)).toBeGreaterThanOrEqual(minimum);
    },
  );
});
