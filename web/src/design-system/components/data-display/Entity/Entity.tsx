import { useId, useState, type ComponentProps, type ReactNode } from 'react';
import { Loader2 } from 'lucide-react';
import { Link, useNavigate } from 'react-router-dom';
import { flushSync } from 'react-dom';
import { Avatar } from '@design-system/components/primitives/Avatar';
import { TruncatedText } from '@design-system/components/data-display/TruncatedText';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@design-system/components/overlays/Tooltip';
import { cn } from '@design-system/utils/cn';

export interface EntityProps extends Omit<ComponentProps<'div'>, 'onSelect' | 'children'> {
  id: string;
  name: string;
  rowLayout?: 'stacked';
  /** Grid-template utility classes for the wide EntityRow presentation. */
  columnTemplate?: string;
  cardSize?: 'compact';
  avatar?: ReactNode;
  nameClassName?: string;
  selectionIndicator?: ReactNode;
  status?: ReactNode;
  supporting?: ReactNode;
  meta?: ReactNode;
  actions?: ReactNode;
  href?: string;
  selected?: boolean;
  busy?: boolean;
  onSelect?: () => void;
}

type Density = 'card' | 'row';

const surfaceStyles = {
  card: 'flex-col gap-2 px-4 pt-4 pb-2',
  compact: 'items-start gap-3 p-3 max-[240px]:gap-0',
  stacked: 'items-center gap-3 px-3 py-2 max-md:gap-2 max-md:p-2',
  row: 'w-full grid-cols-[minmax(0,1fr)_auto] items-center gap-x-2 gap-y-2 p-3 md:grid-cols-[minmax(0,1fr)_minmax(10rem,auto)] md:gap-x-3 md:gap-y-1 xl:gap-y-0',
} as const;

function Entity({ density, id, name, nameClassName, selectionIndicator, avatar, status, supporting, meta, actions, href, selected = false, busy = false, onSelect, rowLayout, columnTemplate, cardSize, 'aria-controls': ariaControls, className, style, ...props }: EntityProps & { density: Density }) {
  const navigate = useNavigate();
  const descriptionId = useId();
  const [nameClipped, setNameClipped] = useState(false);
  const [tooltipOpen, setTooltipOpen] = useState(false);
  const descriptionIds = [status && `${descriptionId}-status`, supporting && `${descriptionId}-supporting`, meta && `${descriptionId}-meta`].filter(Boolean).join(' ') || undefined;
  const clickable = !busy && (Boolean(href) || Boolean(onSelect));
  const card = density === 'card';
  const compactCard = card && cardSize === 'compact';
  const standardCard = card && !compactCard;
  const stacked = density === 'row' && rowLayout === 'stacked';
  const columns = density === 'row' && !stacked;
  const nameClass = cn('min-w-0 text-sm font-semibold', stacked && clickable ? 'leading-5 [overflow-wrap:anywhere]' : 'truncate', columns && 'flex-1', card && 'text-base leading-5', standardCard && 'font-display', selected ? 'text-primary-soft-foreground' : 'text-card-foreground', nameClassName);
  const avatarContent = <span className={cn('flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-lg [&>img]:size-full [&>img]:object-contain', compactCard && 'max-[240px]:hidden')} aria-hidden="true">{avatar ?? <Avatar id={id} label={name} aria-hidden="true" />}</span>;
  const titleContent = clickable ? <TruncatedText text={name} lines={stacked ? 2 : 1} interactive={false} onClippedChange={setNameClipped} data-testid="entity-name" className={nameClass} />
    : busy ? <span data-testid="entity-name" className={nameClass}>{name}</span>
      : <TruncatedText text={name} lines={1} data-testid="entity-name" className={nameClass} />;
  const content = standardCard ? <>
    <span data-slot="entity-identity" className="flex w-full min-w-0 items-start gap-3">
      {avatarContent}
      <span className="flex min-w-0 flex-1 flex-col gap-1">
        {titleContent}
        {status && <span id={`${descriptionId}-status`} data-slot="entity-status" className="flex min-h-5 min-w-0 items-center text-xs">{status}</span>}
      </span>
    </span>
    {supporting && <span id={`${descriptionId}-supporting`} data-slot="entity-supporting" className="relative z-10 min-w-0 text-xs text-muted-foreground">{supporting}</span>}
    {meta && <span id={`${descriptionId}-meta`} data-slot="entity-meta" className="relative z-10 min-w-0 text-xs text-muted-foreground">{meta}</span>}
  </> : compactCard ? <>
    {avatarContent}
    <span className="flex h-full min-w-0 flex-1 flex-col">
      {titleContent}
      {status && <span id={`${descriptionId}-status`} data-slot="entity-status" className="flex h-5 min-w-0 items-center text-xs">{status}</span>}
      {supporting && <span id={`${descriptionId}-supporting`} data-slot="entity-supporting" className="relative z-10 min-w-0 truncate text-muted-foreground text-xs font-normal leading-[15px] max-[240px]:overflow-visible max-[240px]:whitespace-normal">{supporting}</span>}
      {meta && <span id={`${descriptionId}-meta`} data-slot="entity-meta" className="relative z-10 min-w-0 truncate text-muted-foreground text-xs leading-[15px] tabular-nums max-[240px]:overflow-visible max-[240px]:whitespace-normal">{meta}</span>}
    </span>
  </> : stacked ? <>
    {avatarContent}
    <span data-slot="entity-identity" className="flex min-w-0 flex-1 flex-col justify-center gap-1">
      {titleContent}
      <span className={cn('flex min-w-0 items-center gap-2', actions && 'max-md:flex-wrap max-md:gap-y-1')}>
        {supporting && <span id={`${descriptionId}-supporting`} data-slot="entity-supporting" className={cn('relative z-10 min-w-0 flex-1 truncate text-sm text-muted-foreground', actions && 'max-md:basis-full')}>{supporting}</span>}
        {status && <span id={`${descriptionId}-status`} data-slot="entity-status" className="shrink-0 text-xs">{status}</span>}
        {meta && <span id={`${descriptionId}-meta`} data-slot="entity-meta" className={cn('relative z-10 min-w-0 max-w-[30%] shrink-0 truncate text-meta text-muted-foreground', actions && 'max-md:max-w-none max-md:flex-1')}>{meta}</span>}
      </span>
    </span>
  </> : <>
    <span data-slot="entity-identity" className={cn('col-start-1 flex min-w-0 items-center gap-3 max-md:gap-2 xl:col-auto', !status && 'col-span-2 xl:col-span-1')}>{avatarContent}{titleContent}</span>
    {status && <span id={`${descriptionId}-status`} data-slot="entity-status" className="min-w-0 justify-self-end text-xs xl:justify-self-auto">{status}</span>}
    {supporting && <span id={`${descriptionId}-supporting`} data-slot="entity-supporting" className="relative z-10 col-span-2 min-w-0 whitespace-normal text-sm text-muted-foreground [overflow-wrap:anywhere] md:col-span-1">{supporting}</span>}
    {meta && <span id={`${descriptionId}-meta`} data-slot="entity-meta" className={cn('relative z-10 col-span-2 w-full min-w-0 text-meta text-muted-foreground md:col-span-1', !supporting && 'md:col-span-2 xl:col-span-1', status && !supporting && columnTemplate && 'xl:col-start-4')}>{meta}</span>}
  </>;

  const surfaceClass = cn(
    'min-w-0 flex-1 text-left text-card-foreground no-underline outline-none',
    columns ? 'grid' : 'flex',
    surfaceStyles[card ? cardSize ?? 'card' : rowLayout ?? 'row'],
    columns && (columnTemplate ?? 'xl:grid-flow-col xl:auto-cols-auto'),
    clickable && 'before:absolute before:inset-0 before:rounded-xl focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring',
  );
  const surface = busy ? <div className={surfaceClass}>{content}</div>
    : href ? <Link to={href} aria-label={name} aria-describedby={descriptionIds} aria-current={selected ? 'true' : undefined} aria-controls={ariaControls} onClick={event => {
      onSelect?.();
      if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || !document.startViewTransition || window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;
      event.preventDefault();
      document.startViewTransition(() => { flushSync(() => navigate(href)); });
    }} className={surfaceClass}>{content}</Link>
      : onSelect ? <button type="button" aria-label={name} aria-describedby={descriptionIds} aria-pressed={selected} aria-controls={ariaControls} onClick={onSelect} className={surfaceClass}>{content}</button>
        : <div className={surfaceClass}>{content}</div>;

  return <div
    {...props}
    id={id}
    data-slot={card ? 'entity-card' : 'entity-row'}
    data-layout={stacked ? 'stacked' : undefined}
    data-selected={selected || undefined}
    aria-busy={busy || undefined}
    style={{ ...style, viewTransitionName: href ? `entity-${id}` : undefined }}
    className={cn(
      'relative flex min-w-0 overflow-hidden rounded-xl border border-border-subtle bg-card text-card-foreground transition-colors duration-(--motion-feedback) ease-(--motion-ease)',
      compactCard ? 'h-48 flex-col' : standardCard ? 'min-h-40 flex-col' : stacked ? 'h-[88px] items-center' : 'min-h-[88px] flex-col items-stretch md:flex-row md:items-center',
      stacked && actions && 'max-md:h-40 max-md:flex-col max-md:items-stretch',
      stacked && clickable && 'border-l-4 border-l-transparent',
      clickable && 'cursor-pointer',
      clickable && (selected ? 'hover:border-border' : 'hover:border-border hover:bg-accent'),
      selected && 'border-primary-soft bg-primary-soft',
      stacked && selected && !selectionIndicator && 'border-l-primary hover:border-l-primary',
      className,
    )}
  >
    {selectionIndicator}
    {clickable ? <TooltipProvider><Tooltip open={nameClipped && tooltipOpen} onOpenChange={setTooltipOpen}><TooltipTrigger asChild>{surface}</TooltipTrigger>{nameClipped && <TooltipContent>{name}</TooltipContent>}</Tooltip></TooltipProvider> : surface}
    {(actions || busy) && <fieldset
      disabled={busy}
      className={cn('relative z-10 flex min-w-0 shrink-0 items-center gap-2 border-0 p-0', compactCard && 'mt-auto justify-end px-3 pb-3 max-[240px]:flex-wrap max-[240px]:justify-start max-[240px]:gap-y-2', standardCard && 'mt-auto mx-4 w-[calc(100%-2rem)] justify-end border-t border-border-subtle pt-2 pb-3', stacked && actions && 'max-md:w-full max-md:justify-end max-md:px-2 max-md:pb-2', columns && 'w-full flex-wrap justify-end px-3 pb-3 md:w-auto md:flex-nowrap md:px-0 md:pb-0 md:pr-3')}
      inert={busy}
    >
      {actions}
      {busy && <Loader2 aria-hidden="true" className="size-4 motion-safe:animate-spin" />}
    </fieldset>}
  </div>;
}

export function EntityCard(props: EntityProps) {
  return <Entity {...props} density="card" />;
}

export function EntityRow(props: EntityProps) {
  return <Entity {...props} density="row" />;
}
