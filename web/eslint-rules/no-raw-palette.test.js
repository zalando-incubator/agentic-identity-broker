// @vitest-environment node
import { RuleTester } from 'eslint';
import tseslint from 'typescript-eslint';
import { describe, it } from 'vitest';
import noRawPalette from './no-raw-palette.js';

RuleTester.describe = describe;
RuleTester.it = it;

const tester = new RuleTester({
  languageOptions: {
    parser: tseslint.parser,
    parserOptions: { ecmaFeatures: { jsx: true } },
  },
});

const classContexts = [
  (value) => `<div className="${value}" />`,
  (value) => `<div className={\`${value}\`} />`,
  (value) => `cn('${value}', active && 'p-2')`,
  (value) => `clsx(['${value}'])`,
  (value) => `cva('${value}', { variants: { size: { small: 'p-2' } } })`,
  (value) => `cva('p-2', { variants: { intent: { active: '${value}' } } })`,
];
const rawClasses = [
  'bg-blue-600', 'text-neutral-500', 'border-white', 'from-amber-50',
  'dark:hover:!bg-slate-950/80', 'md:focus-visible:ring-gray-500!',
  '[&>svg]:stroke-black/50', 'ring-offset-zinc-100', 'border-t-stone-300',
  'divide-y-red-500', 'outline-emerald-500', 'fill-rose-400',
  'shadow-purple-900/20', 'decoration-cyan-600', 'accent-lime-500', 'caret-pink-500',
  'bg-[#fff]', 'hover:text-[oklch(0.5_0.1_250)]/80',
  'border-[rgb(0_0_0)]', 'ring-[hsl(0_0%_0%)]', 'bg-[rebeccapurple]',
  'text-[color:#123456]', '[background-color:rgba(0,0,0,0.5)]',
  '[rgb(0_0_0)]', 'bg-[linear-gradient(#fff,#000)]',
];
const semanticClasses = [
  'bg-background text-foreground bg-primary border-border text-muted-foreground',
  'dark:hover:!bg-primary/80 md:focus-visible:ring-ring!',
  '[&>svg]:fill-current bg-transparent border-current text-current',
  'bg-card text-card-foreground border-sidebar-border ring-offset-background',
  'text-[14px] w-[calc(100%-2rem)] border-[1px] rounded-[3px]',
  'text-[length:var(--font-size)] bg-[position:50%_50%] shadow-[0_1px_2px_var(--border)]',
  'text-[var(--foreground)] bg-[color:var(--background)] [color:currentColor]',
];
const invalid = (code, messageId = 'rawPalette') => ({
  code,
  filename: 'src/Example.tsx',
  errors: [{ messageId }],
});

tester.run('no-raw-palette', noRawPalette, {
  valid: [
    ...classContexts.map((context) => ({
      code: context(semanticClasses[0]), filename: 'src/Example.tsx',
    })),
    ...semanticClasses.slice(1).map((value) => ({
      code: classContexts[0](value), filename: 'src/Example.tsx',
    })),
    { code: '<div className={`bg-primary ${className} ${active ? "text-foreground" : "text-muted-foreground"}`} />' },
    { code: '<div className={"bg-primary " + className} />' },
    { code: '<div className={`w-${width} gap-${gap}`} />' },
    { code: "cn({ 'bg-primary': color === 'bg-red-500' }, enabled && 'text-foreground')" },
    { code: "cva('bg-primary', { variants: { label: { 'bg-red-500': 'text-foreground' } }, defaultVariants: { label: 'bg-red-500' }, compoundVariants: [{ label: 'bg-red-500', class: 'bg-secondary' }] })" },
    { code: "const copy = 'Use bg-red-500'; const url = 'https://example.com/bg-red-500'; const data = { color: 'bg-red-500' }; <div title={copy} data-color={data.color}>{url}</div>" },
    { code: "const copy = { red: 'bg-red-500' }; <div className={cn('bg-primary', copy.red === 'bg-red-500' && 'p-2')} />" },
    { code: "const classes = 'bg-primary'; <div className={classes as string} />" },
    { code: "function render(className: string) { return <div className={className} /> }", filename: 'src/Example.tsx' },
  ],
  invalid: [
    ...classContexts.flatMap((context) => [
      invalid(context(rawClasses[0])),
      invalid(context(rawClasses[16])),
    ]),
    ...rawClasses.filter((_, index) => index !== 0 && index !== 16).map((value) => invalid(classContexts[0](value))),
    invalid("cn({ 'hover:bg-red-500': active })"),
    invalid("clsx({ ['text-black']: active })"),
    invalid("cva('bg-primary', { compoundVariants: [{ intent: 'warning', className: 'text-yellow-500' }] })"),
    invalid("cva('bg-primary', { compoundVariants: [{ intent: 'warning', class: ['text-yellow-500'] }] })"),
    invalid('<div className={active ? "bg-red-500" : "bg-primary"} />'),
    invalid("const classes = 'bg-red-500'; <div className={classes} />"),
    invalid("const classes = 'text-black' as const; cn(classes)"),
    invalid("import { clsx as cx } from 'clsx'; cx('bg-red-500')"),
    invalid("import { cva as variants } from 'class-variance-authority'; variants('bg-blue-500')"),
    invalid('<div className={"bg-" + "blue-500"} />'),
    invalid('<div className={`bg-${color}-500`} />', 'dynamicColor'),
    invalid('<div className={"hover:text-" + color} />', 'dynamicColor'),
    invalid('cn(`border-${color}`)', 'dynamicColor'),
    invalid('clsx({ [`dark:bg-${color}`]: active })', 'dynamicColor'),
    invalid('cva(`bg-${color}`, { variants: {} })', 'dynamicColor'),
    invalid('cva("p-2", { variants: { intent: { active: `text-${color}` } } })', 'dynamicColor'),
    invalid('<div className={`bg-[${color}]`} />', 'dynamicColor'),
    invalid('<div className={`${utility}-blue-500`} />', 'dynamicColor'),
    invalid('<div className={`hover:${utility}-${color}`} />', 'dynamicColor'),
    invalid('<div className={`bg-${active ? "primary" : "secondary"}`} />', 'dynamicColor'),
    { ...invalid('<div className="text-red-500 bg-blue-500" />'), errors: [{ messageId: 'rawPalette' }, { messageId: 'rawPalette' }] },
  ],
});
