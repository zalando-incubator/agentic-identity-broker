import { useState, type ComponentProps, type ReactNode } from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { UserRound } from 'lucide-react';
import { cn } from '@design-system/utils';

const avatarVariants = cva(
  'relative inline-flex shrink-0 items-center justify-center overflow-hidden rounded-full border border-border bg-muted font-medium text-foreground',
  {
    variants: {
      size: { default: 'size-10 text-sm', sm: 'size-8 text-xs', lg: 'size-12 text-base' },
    },
    defaultVariants: { size: 'default' },
  },
);

export interface AvatarProps extends Omit<ComponentProps<'span'>, 'children'>, VariantProps<typeof avatarVariants> {
  label: string;
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

function AvatarContents({ source, fallback }: { source?: string; fallback?: ReactNode }) {
  const [failed, setFailed] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const showImage = Boolean(source && !failed);

  return <>
    {(!showImage || !loaded) && <span aria-hidden="true">{fallback ?? <UserRound className="size-5" />}</span>}
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

export function Avatar({ label, src, fallback, size, className, ...props }: AvatarProps) {
  const source = sameOriginSource(src);
  return <span {...props} role="img" aria-label={label} data-slot="avatar" className={cn(avatarVariants({ size }), className)}>
    <AvatarContents key={source} source={source} fallback={fallback} />
  </span>;
}
