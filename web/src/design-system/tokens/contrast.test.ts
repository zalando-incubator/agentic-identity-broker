import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { wcagContrast } from 'culori';
import { parse } from 'postcss';
import { describe, expect, it } from 'vitest';

const stylesheet = parse(readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'theme.css'), 'utf8'));
const surfaces = ['background', 'card', 'muted', 'popover'];
const statuses = ['success', 'warning', 'status-danger', 'info', 'risk-low', 'risk-medium', 'risk-high'];
const avatars = Array.from({ length: 8 }, (_, index) => `avatar-${index + 1}`);

type Pair = [foreground: string, background: string, minimum: number];
const textPairs: Pair[] = [
  ['foreground', 'background', 4.5],
  ['card-foreground', 'card', 4.5],
  ['popover-foreground', 'popover', 4.5],
  ['muted-foreground', 'sidebar-accent', 4.5],
  ['secondary-foreground', 'secondary', 4.5],
  ['accent-foreground', 'accent', 4.5],
  ...surfaces.map((surface): Pair => ['muted-foreground', surface, 4.5]),
  ...surfaces.map((surface): Pair => ['primary', surface, 4.5]),
  ['primary-foreground', 'primary', 4.5],
  ['primary-foreground', 'primary-hover', 4.5],
  ['primary-soft-foreground', 'primary-soft', 4.5],
  ['destructive-foreground', 'destructive', 4.5],
  ['destructive-foreground', 'destructive-hover', 4.5],
  ...statuses.map((status): Pair => [`${status}-foreground`, status, 4.5]),
  ...avatars.map((avatar): Pair => [`${avatar}-foreground`, avatar, 4.5]),
  ['sidebar-foreground', 'sidebar', 4.5],
  ['sidebar-primary', 'sidebar', 4.5],
  ['sidebar-primary-foreground', 'sidebar-primary', 4.5],
  ['sidebar-accent-foreground', 'sidebar-accent', 4.5],
];
const controlBoundaries: Pair[] = [
  ...surfaces.map((surface): Pair => ['border-control', surface, 3]),
  ['border-control', 'secondary', 3],
  ['border-control', 'accent', 3],
];
const focusRings: Pair[] = [
  ...surfaces.map((surface): Pair => ['ring', surface, 3]),
  ['ring', 'secondary', 3],
  ['ring', 'accent', 3],
  ['ring', 'primary-soft', 3],
  ['sidebar-ring', 'sidebar', 3],
  ['sidebar-ring', 'sidebar-accent', 3],
];

function themeRoles(theme: 'light' | 'dark') {
  const values = new Map<string, string>();
  stylesheet.walkRules((rule) => {
    if (rule.parent?.type !== 'root' || !rule.selectors.includes(`[data-theme="${theme}"]`)) return;
    rule.walkDecls(/^--/, (declaration) => {
      values.set(declaration.prop.slice(2), declaration.value);
    });
  });
  return values;
}

describe.each(['light', 'dark'] as const)('%s semantic contrast', (theme) => {
  const values = themeRoles(theme);

  it.each([...textPairs, ...controlBoundaries, ...focusRings])(
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
