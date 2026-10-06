import { useState, type ComponentProps, type ReactNode } from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@design-system/utils';

const tints = [
  'bg-avatar-1 text-avatar-1-foreground',
  'bg-avatar-2 text-avatar-2-foreground',
  'bg-avatar-3 text-avatar-3-foreground',
  'bg-avatar-4 text-avatar-4-foreground',
  'bg-avatar-5 text-avatar-5-foreground',
  'bg-avatar-6 text-avatar-6-foreground',
  'bg-avatar-7 text-avatar-7-foreground',
  'bg-avatar-8 text-avatar-8-foreground',
] as const;

const avatarVariants = cva(
  'relative inline-flex shrink-0 items-center justify-center overflow-hidden rounded-lg border border-border-subtle font-medium',
  {
    variants: {
      size: { default: 'size-10 text-sm', sm: 'size-8 text-xs', lg: 'size-12 text-base' },
      person: { true: 'rounded-full', false: '' },
    },
    defaultVariants: { size: 'default', person: false },
  },
);

function tintFor(id: string): number {
  let hash = 2166136261;
  for (let index = 0; index < id.length; index += 1) hash = Math.imul(hash ^ id.charCodeAt(index), 16777619);
  return (hash >>> 0) % tints.length;
}

export interface AvatarProps extends Omit<ComponentProps<'span'>, 'children'>, VariantProps<typeof avatarVariants> {
  label: string;
  id?: string;
  person?: boolean;
  src?: string;
  fallback?: ReactNode;
}

function sameOriginSource(src: string | undefined): string | undefined {
  if (!src || typeof window === 'undefined') return undefined;
  try {
    const url = new URL(src, window.location.href);
    if ((url.protocol === 'http:' || url.protocol === 'https:') && url.origin === window.location.origin) {
      return url.href;
    }
  } catch {
    // Unparseable metadata uses the same local fallback as an external image.
  }
  return undefined;
}

function AvatarContents({ source, fallback, label }: { source?: string; fallback?: ReactNode; label: string }) {
  const [failed, setFailed] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const showImage = Boolean(source && !failed);

  return <>
    {(!showImage || !loaded) && <span aria-hidden="true">{fallback ?? String.fromCodePoint(label.trim().codePointAt(0) ?? 63).toLocaleUpperCase()}</span>}
    {showImage && <img
      src={source}
      alt=""
      aria-hidden="true"
      className={cn('absolute inset-0 size-full object-cover', !loaded && 'invisible')}
      onLoad={() => setLoaded(true)}
      onError={() => setFailed(true)}
    />}
  </>;
}

export function Avatar({ label, id, person = false, src, fallback, size, className, ...props }: AvatarProps) {
  const source = sameOriginSource(src);
  const tint = tintFor(id ?? label);
  return <span {...props} role="img" aria-label={label} data-slot="avatar" data-tint={tint + 1} className={cn(avatarVariants({ size, person }), tints[tint], className)}>
    <AvatarContents key={source} source={source} fallback={fallback} label={label} />
  </span>;
}
