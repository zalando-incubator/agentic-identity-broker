import { useEffect, useState, type CSSProperties } from 'react';
import {
  ArrowUpRight,
  BadgeCheck,
  Ban,
  Bot,
  Cable,
  CircleCheck,
  ClockAlert,
  KeyRound,
  Laptop,
  Lock,
  ShieldAlert,
  ShieldCheck,
  Unplug,
  type LucideIcon,
} from 'lucide-react';
import { wcagContrast } from 'culori';
import './Foundations.css';

type ColorPair = { label: string; foreground: string; background: string };

const surfaces = ['background', 'card', 'muted', 'popover'] as const;

const surfacePairs: ColorPair[] = [
  { label: 'Page text', foreground: 'foreground', background: 'background' },
  { label: 'Card text', foreground: 'card-foreground', background: 'card' },
  { label: 'Popover text', foreground: 'popover-foreground', background: 'popover' },
  { label: 'Muted text on page', foreground: 'muted-foreground', background: 'background' },
  { label: 'Muted text on card', foreground: 'muted-foreground', background: 'card' },
  { label: 'Muted text on inset surface', foreground: 'muted-foreground', background: 'muted' },
  { label: 'Muted text on popover', foreground: 'muted-foreground', background: 'popover' },
  { label: 'Muted text on active sidebar item', foreground: 'muted-foreground', background: 'sidebar-accent' },
  ...surfaces.map((surface): ColorPair => ({ label: `Primary link on ${surface}`, foreground: 'primary', background: surface })),
  { label: 'Primary action', foreground: 'primary-foreground', background: 'primary' },
  { label: 'Primary action hover', foreground: 'primary-foreground', background: 'primary-hover' },
  { label: 'Selected item', foreground: 'primary-soft-foreground', background: 'primary-soft' },
  { label: 'Secondary action', foreground: 'secondary-foreground', background: 'secondary' },
  { label: 'Neutral hover', foreground: 'accent-foreground', background: 'accent' },
  { label: 'Destructive confirmation', foreground: 'destructive-foreground', background: 'destructive' },
  { label: 'Destructive confirmation hover', foreground: 'destructive-foreground', background: 'destructive-hover' },
  { label: 'Sidebar text', foreground: 'sidebar-foreground', background: 'sidebar' },
  { label: 'Sidebar link', foreground: 'sidebar-primary', background: 'sidebar' },
  { label: 'Sidebar action', foreground: 'sidebar-primary-foreground', background: 'sidebar-primary' },
  { label: 'Active sidebar item', foreground: 'sidebar-accent-foreground', background: 'sidebar-accent' },
];

const statusNames = ['success', 'warning', 'status-danger', 'info', 'risk-low', 'risk-medium', 'risk-high'] as const;
const statusPairs: ColorPair[] = statusNames.map((name) => ({
  label: name.replace(/-/g, ' '), foreground: `${name}-foreground`, background: name,
}));
const avatarPairs: ColorPair[] = Array.from({ length: 8 }, (_, index) => ({
  label: `Avatar tint ${index + 1}`,
  foreground: `avatar-${index + 1}-foreground`,
  background: `avatar-${index + 1}`,
}));

type EdgePair = { label: string; edge: string; surface: string; required: boolean };
const edgePairs: EdgePair[] = [
  { label: 'Card edge / divider', edge: 'border-subtle', surface: 'card', required: false },
  { label: 'Page divider', edge: 'border-subtle', surface: 'background', required: false },
  { label: 'Button / popover edge', edge: 'border', surface: 'popover', required: false },
  { label: 'Button edge on page', edge: 'border', surface: 'background', required: false },
  ...[...surfaces, 'secondary', 'accent'].map((surface) => ({
    label: 'Form control boundary', edge: 'border-control', surface, required: true,
  })),
  ...[...surfaces, 'secondary', 'accent', 'primary-soft'].map((surface) => ({
    label: 'Focus ring', edge: 'ring', surface, required: true,
  })),
  { label: 'Sidebar edge', edge: 'sidebar-border', surface: 'sidebar', required: false },
  { label: 'Sidebar focus ring', edge: 'sidebar-ring', surface: 'sidebar', required: true },
  { label: 'Sidebar focus on selection', edge: 'sidebar-ring', surface: 'sidebar-accent', required: true },
];
const colorNames = Object.keys(Object.fromEntries([
  ...[...surfacePairs, ...statusPairs, ...avatarPairs].flatMap(({ foreground, background }) => [foreground, background]),
  ...edgePairs.flatMap(({ edge, surface }) => [edge, surface]),
].map((name) => [name, true] as const)));

function readTokens(names: readonly string[]): Record<string, string> {
  const style = getComputedStyle(document.documentElement);
  return {
    ...Object.fromEntries(names.map((name) => [name, style.getPropertyValue(`--${name}`).trim()])),
    'color-scheme': style.colorScheme,
  };
}

function useRootTokens(names: readonly string[]): Record<string, string> | null {
  const [values, setValues] = useState<Record<string, string> | null>(() =>
    typeof document === 'undefined' ? null : readTokens(names));

  useEffect(() => {
    const update = () => setValues(readTokens(names));
    const observer = new MutationObserver(update);
    observer.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme', 'style', 'class'] });
    const systemTheme = window.matchMedia('(prefers-color-scheme: dark)');
    systemTheme.addEventListener('change', update);
    update();
    return () => {
      observer.disconnect();
      systemTheme.removeEventListener('change', update);
    };
  }, [names]);

  return values;
}

function contrast(values: Record<string, string>, foreground: string, background: string): string {
  if (!values[foreground] || !values[background]) return 'Token unavailable';
  const ratio = wcagContrast(values[foreground], values[background]);
  return Number.isFinite(ratio) ? `${ratio.toFixed(2)}:1` : 'Color unavailable';
}

function ColorTable({ caption, pairs, values, avatar = false }: {
  caption: string; pairs: readonly ColorPair[]; values: Record<string, string>; avatar?: boolean;
}) {
  return <div className="foundation-table-scroll"><table className="foundation-table">
    <caption>{caption}</caption>
    <thead><tr><th scope="col">Role</th><th scope="col">Sample</th><th scope="col">Computed tokens</th><th scope="col">Text contrast</th></tr></thead>
    <tbody>{pairs.map(({ label, foreground, background }) => <tr key={`${foreground}-${background}`}>
      <th scope="row">{label}</th>
      <td><span className={avatar ? 'foundation-swatch foundation-avatar-swatch' : 'foundation-swatch'} style={{ color: `var(--${foreground})`, backgroundColor: `var(--${background})` }}>
        {avatar ? 'A' : label}
      </span></td>
      <td className="foundation-token-names"><code>--{foreground}</code> / <code>--{background}</code>
        <small>{values[foreground]} / {values[background]}</small>
      </td>
      <td>{contrast(values, foreground, background)}</td>
    </tr>)}</tbody>
  </table></div>;
}

function EdgeTable({ values }: { values: Record<string, string> }) {
  return <div className="foundation-table-scroll"><table className="foundation-table">
    <caption>Borders and focus indicators against adjacent surfaces</caption>
    <thead><tr><th scope="col">Role</th><th scope="col">Sample</th><th scope="col">Token / surface</th><th scope="col">Contrast</th><th scope="col">Requirement</th></tr></thead>
    <tbody>{edgePairs.map(({ label, edge, surface, required }) => {
      const ring = edge.endsWith('ring');
      const style: CSSProperties = {
        backgroundColor: `var(--${surface})`,
        color: surface.startsWith('sidebar') ? 'var(--sidebar-foreground)' : 'var(--foreground)',
        ...(ring ? { outline: `2px solid var(--${edge})`, outlineOffset: 2 } : { border: `1px solid var(--${edge})` }),
      };
      return <tr key={`${edge}-${surface}`}>
        <th scope="row">{label}</th>
        <td><span className="foundation-edge-swatch" style={style}>Edge</span></td>
        <td className="foundation-token-names"><code>--{edge}</code> / <code>--{surface}</code>
          <small>{values[edge]} / {values[surface]}</small>
        </td>
        <td>{contrast(values, edge, surface)}</td>
        <td>{required ? '3:1 minimum' : 'Decorative; no 3:1 minimum'}</td>
      </tr>;
    })}</tbody>
  </table></div>;
}

export function FoundationColors() {
  const values = useRootTokens(colorNames);
  if (!values) return <p>Reading the current theme tokens…</p>;
  return <div className="foundation-reference">
    <p>Computed values on this page use the <strong>{values['color-scheme']}</strong> theme. Change the Storybook theme toolbar to compare both themes.</p>
    <p>Ratios use the CSS token colors and WCAG relative luminance. They do not measure opacity, overlays, hover composition, or disabled content.</p>
    <ColorTable caption="Surfaces, actions, and selection" pairs={surfacePairs} values={values} />
    <ColorTable caption="Soft status and server-provided risk" pairs={statusPairs} values={values} />
    <ColorTable caption="Local avatar tint and initial text" pairs={avatarPairs} values={values} avatar />
    <EdgeTable values={values} />
  </div>;
}

const fonts = [
  { role: 'Display · titles and decision names', token: 'font-display', sample: 'A clear decision' },
  { role: 'Body · controls, dates, and counts', token: 'font-sans', sample: 'A clear decision · 2026-10-04' },
  { role: 'Technical · IDs and raw arguments', token: 'font-mono', sample: 'agent_id: alpha-014' },
] as const;
const typeSizes = [
  { pixels: 12, token: 'text-xs', role: 'Badge and fine metadata', font: 'sans' },
  { pixels: 13, token: 'text-meta', role: 'Supporting metadata', font: 'sans' },
  { pixels: 14, token: 'text-sm', role: 'Console base, controls, entity names', font: 'sans' },
  { pixels: 16, token: 'text-base', role: 'Consent headings and long-form text', font: 'sans' },
  { pixels: 20, token: 'text-xl', role: 'Semibold page title', font: 'display' },
  { pixels: 24, token: 'text-2xl', role: 'Large decision heading when it fits', font: 'display' },
] as const;
const fontNames = [...fonts.map(({ token }) => token), ...typeSizes.map(({ token }) => token)];

export function FoundationTypography() {
  const values = useRootTokens(fontNames);
  return <div className="foundation-reference">
    <div className="foundation-table-scroll"><table className="foundation-table">
      <caption>Self-hosted font families</caption>
      <thead><tr><th scope="col">Use</th><th scope="col">Font sample</th><th scope="col">Computed family</th></tr></thead>
      <tbody>{fonts.map(({ role, token, sample }) => <tr key={token}>
        <th scope="row">{role}</th>
        <td><span style={{ fontFamily: `var(--${token})` }}>{sample}</span></td>
        <td className="foundation-token-names"><code>--{token}</code><small>{values?.[token]}</small></td>
      </tr>)}</tbody>
    </table></div>
    <div className="foundation-table-scroll"><table className="foundation-table">
      <caption>Type scale</caption>
      <thead><tr><th scope="col">Size</th><th scope="col">Use</th><th scope="col">Rendered sample</th><th scope="col">Token value</th></tr></thead>
      <tbody>{typeSizes.map(({ pixels, token, role, font }) => <tr key={token}>
        <th scope="row">{pixels} px</th><td>{role}</td>
        <td><span style={{ fontFamily: `var(--font-${font})`, fontSize: `var(--${token})`, fontWeight: pixels === 20 ? 600 : undefined }}>A clear decision</span></td>
        <td className="foundation-token-names"><code>--{token}</code><small>{values?.[token]}</small></td>
      </tr>)}</tbody>
    </table></div>
    <p>Consent card body: 15 px (<code>--text-consent</code>). Dates and counts use tabular Inter figures, not the mono face.</p>
    <p className="foundation-figures">Tabular example: 014 · 2026-10-04</p>
  </div>;
}

const spacingSteps = [4, 8, 12, 16, 24, 32, 48] as const;

export function FoundationSpacing() {
  const values = useRootTokens(['spacing']);
  return <div className="foundation-reference">
    <p>Base unit: <code>--spacing</code> = <code>{values?.spacing}</code>. Use 4–12 px within components, 16–24 px between components, and 32–48 px between sections.</p>
    <div className="foundation-table-scroll"><table className="foundation-table">
      <caption>Allowed spacing steps, sized from the current CSS base unit</caption>
      <thead><tr><th scope="col">Step</th><th scope="col">Token multiple</th><th scope="col">Rendered width</th></tr></thead>
      <tbody>{spacingSteps.map((pixels) => <tr key={pixels}>
        <th scope="row">{pixels} px</th><td><code>{pixels / 4} × --spacing</code></td>
        <td><span className="foundation-spacing-swatch" style={{ width: `calc(var(--spacing) * ${pixels / 4})` }} /></td>
      </tr>)}</tbody>
    </table></div>
    <p>Console content has a maximum width of 1120 px. Running text stops at 70ch.</p>
  </div>;
}

const radii = [
  { role: 'Badge', token: 'radius-md', pixels: 6 },
  { role: 'Control and rounded-square avatar', token: 'radius-lg', pixels: 8 },
  { role: 'Card, alert, and inset panel', token: 'radius-xl', pixels: 12 },
  { role: 'Dialog and consent card', token: 'radius-2xl', pixels: 16 },
] as const;
const radiusNames = radii.map(({ token }) => token);

export function FoundationRadii() {
  const values = useRootTokens(radiusNames);
  return <div className="foundation-reference"><div className="foundation-table-scroll"><table className="foundation-table">
    <caption>Semantic shape roles</caption>
    <thead><tr><th scope="col">Use</th><th scope="col">Sample</th><th scope="col">Token</th><th scope="col">Radius</th></tr></thead>
    <tbody>{radii.map(({ role, token, pixels }) => <tr key={token}>
      <th scope="row">{role}</th>
      <td><span className="foundation-radius-swatch" style={{ borderRadius: `var(--${token})` }} /></td>
      <td><code>--{token}</code></td><td>{pixels} px <small>({values?.[token]})</small></td>
    </tr>)}</tbody>
  </table></div><p>People keep circular avatars. Agent and service avatars use the rounded-square control radius.</p></div>;
}

const motions = [
  { token: 'motion-feedback', use: 'Hover and color feedback' },
  { token: 'motion-control', use: 'Controls and disclosures' },
  { token: 'motion-overlay', use: 'Overlays' },
  { token: 'motion-emphasis', use: 'Success and illustrative changes' },
  { token: 'motion-ease', use: 'CSS and console Motion easing' },
] as const;
const motionNames = motions.map(({ token }) => token);

export function FoundationMotion() {
  const values = useRootTokens(motionNames);
  return <div className="foundation-reference">
    <div className="foundation-table-scroll"><table className="foundation-table">
      <caption>Motion tokens in the current stylesheet</caption>
      <thead><tr><th scope="col">Token</th><th scope="col">Computed value</th><th scope="col">Use</th></tr></thead>
      <tbody>{motions.map(({ token, use }) => <tr key={token}>
        <th scope="row"><code>--{token}</code></th><td>{values?.[token]}</td><td>{use}</td>
      </tr>)}</tbody>
    </table></div>
    <p>Exits use 75% of their entry duration. The focus indicator appears immediately, without a transition or delay.</p>
    <p>Under reduced motion, stop position changes, scale effects, and icon animation. Keep short opacity and color feedback.</p>
    <span className="foundation-motion-sample">Hover over this sample to see color feedback.</span>
  </div>;
}

const icons: { concept: string; name: string; Icon: LucideIcon }[] = [
  { concept: 'Agents', name: 'Bot', Icon: Bot },
  { concept: 'Connections', name: 'Cable', Icon: Cable },
  { concept: 'Approvals', name: 'ShieldCheck', Icon: ShieldCheck },
  { concept: 'Required permission', name: 'Lock', Icon: Lock },
  { concept: 'Verified origin', name: 'BadgeCheck', Icon: BadgeCheck },
  { concept: 'Unverified origin', name: 'ShieldAlert', Icon: ShieldAlert },
  { concept: 'Local origin', name: 'Laptop', Icon: Laptop },
  { concept: 'Connected', name: 'CircleCheck', Icon: CircleCheck },
  { concept: 'Needs sign-in', name: 'KeyRound', Icon: KeyRound },
  { concept: 'Expired', name: 'ClockAlert', Icon: ClockAlert },
  { concept: 'Revoke', name: 'Ban', Icon: Ban },
  { concept: 'Disconnect', name: 'Unplug', Icon: Unplug },
  { concept: 'External link', name: 'ArrowUpRight', Icon: ArrowUpRight },
];

export function FoundationIcons() {
  return <div className="foundation-reference"><div className="foundation-table-scroll"><table className="foundation-table">
    <caption>Lucide icon roles</caption>
    <thead><tr><th scope="col">Concept</th><th scope="col">Icon</th><th scope="col">16 px · 1.75 stroke</th><th scope="col">20 px · 1.5 stroke</th></tr></thead>
    <tbody>{icons.map(({ concept, name, Icon }) => <tr key={name}>
      <th scope="row">{concept}</th><td><code>{name}</code></td>
      <td><Icon aria-hidden="true" className="size-4" size={16} strokeWidth={1.75} /></td>
      <td><Icon aria-hidden="true" className="size-5" size={20} strokeWidth={1.5} /></td>
    </tr>)}</tbody>
  </table></div><p>Pair status words with a dot or icon. Only three navigation hover icons, three empty-state icons, a success check, and a new-connection plug animate in their owning components.</p></div>;
}
