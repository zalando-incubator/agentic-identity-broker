/*
 * Adapted from shadcn/ui (https://ui.shadcn.com).
 * MIT License — Copyright (c) 2023 shadcn
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */
import type { ComponentProps, ReactNode } from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@design-system/utils';

const badgeVariants = cva(
  'inline-flex h-5 w-fit shrink-0 items-center justify-center gap-1 rounded-md px-1.5 text-xs font-medium leading-none [&_svg]:size-3 [&_svg]:shrink-0 [&_svg]:stroke-[1.75]',
  {
    variants: {
      variant: {
        neutral: 'bg-muted text-foreground',
        success: 'bg-success text-success-foreground',
        warning: 'bg-warning text-warning-foreground',
        danger: 'bg-status-danger text-status-danger-foreground',
        info: 'bg-info text-info-foreground',
        'risk-low': 'bg-risk-low text-risk-low-foreground',
        'risk-medium': 'bg-risk-medium text-risk-medium-foreground',
        'risk-high': 'bg-risk-high text-risk-high-foreground',
      },
    },
    defaultVariants: { variant: 'neutral' },
  },
);

export interface BadgeProps extends ComponentProps<'span'>, VariantProps<typeof badgeVariants> {
  icon?: ReactNode;
}

export function Badge({ className, variant = 'neutral', icon, children, ...props }: BadgeProps) {
  return <span {...props} data-slot="badge" data-variant={variant} className={cn(badgeVariants({ variant }), className)}>
    {icon ? <span aria-hidden="true" className="shrink-0">{icon}</span> : <span aria-hidden="true" className="size-1.5 shrink-0 rounded-full bg-current" />}
    {children}
  </span>;
}

export { badgeVariants };
