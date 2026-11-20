import { useMemo, useState, type ReactNode } from 'react';
import { cn } from '@design-system/utils';

interface ServiceIconProps {
  iconSvg?: string | null;
  fallback?: ReactNode;
  className?: string;
}

function ServiceIconImage({
  iconSvg,
  fallback,
  className,
}: ServiceIconProps & { iconSvg: string }) {
  const [status, setStatus] = useState<'loading' | 'loaded' | 'error'>('loading');
  const source = useMemo(
    () => `data:image/svg+xml;charset=utf-8,${encodeURIComponent(iconSvg)}`,
    [iconSvg],
  );

  if (status === 'error') return fallback;

  return (
    <span
      aria-hidden="true"
      className={cn('relative inline-flex h-8 w-8 flex-shrink-0', className)}
    >
      {status === 'loading' && fallback}
      <img
        src={source}
        alt=""
        hidden={status !== 'loaded'}
        className="absolute inset-0 h-full w-full object-contain"
        onLoad={() => setStatus('loaded')}
        onError={() => setStatus('error')}
      />
    </span>
  );
}

export function ServiceIcon({ iconSvg, fallback = null, className }: ServiceIconProps) {
  if (iconSvg == null) return fallback;

  return (
    <ServiceIconImage
      key={iconSvg}
      iconSvg={iconSvg}
      fallback={fallback}
      className={className}
    />
  );
}
