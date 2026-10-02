/*
 * Adapted from shadcn/ui (New York v4 sheet), MIT License.
 * Copyright (c) 2023 shadcn
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
import type { ComponentProps } from 'react';
import { X } from 'lucide-react';
import { Dialog as SheetPrimitive } from 'radix-ui';
import { cva } from 'class-variance-authority';
import { cn } from '@design-system/utils/cn';
import '../overlay-motion.css';

const sheetVariants = cva(
  'overlay-motion fixed z-50 flex flex-col gap-4 overflow-y-auto border-border bg-popover p-6 text-popover-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring',
  {
    variants: {
      side: {
        left: 'inset-y-0 left-0 h-dvh w-[calc(100%-2rem)] max-w-sm border-r',
        right: 'inset-y-0 right-0 h-dvh w-[calc(100%-2rem)] max-w-sm border-l',
        top: 'inset-x-0 top-0 max-h-[calc(100dvh-2rem)] border-b',
        bottom: 'inset-x-0 bottom-0 max-h-[calc(100dvh-2rem)] border-t',
      },
    },
    defaultVariants: { side: 'right' },
  },
);

export function Sheet(props: ComponentProps<typeof SheetPrimitive.Root>) {
  return <SheetPrimitive.Root {...props} />;
}

export function SheetTrigger({ className, ...props }: ComponentProps<typeof SheetPrimitive.Trigger>) {
  return <SheetPrimitive.Trigger data-slot="sheet-trigger" className={cn('focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background', className)} {...props} />;
}

export function SheetPortal(props: ComponentProps<typeof SheetPrimitive.Portal>) {
  return <SheetPrimitive.Portal {...props} />;
}

export function SheetClose({ className, ...props }: ComponentProps<typeof SheetPrimitive.Close>) {
  return <SheetPrimitive.Close data-slot="sheet-close" className={cn('focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-popover', className)} {...props} />;
}

export function SheetOverlay({ className, ...props }: ComponentProps<typeof SheetPrimitive.Overlay>) {
  return <SheetPrimitive.Overlay data-slot="sheet-overlay" className={cn('overlay-motion fixed inset-0 z-50 bg-background/80', className)} {...props} />;
}

export type SheetContentProps = ComponentProps<typeof SheetPrimitive.Content> & {
  closeLabel: string;
  side?: 'left' | 'right' | 'top' | 'bottom';
};

export function SheetContent({ className, children, side = 'right', closeLabel, forceMount, ...props }: SheetContentProps) {
  return (
    <SheetPortal forceMount={forceMount}>
      <SheetOverlay forceMount={forceMount} />
      <SheetPrimitive.Content data-slot="sheet-content" data-side={side} forceMount={forceMount} className={cn(sheetVariants({ side }), className)} {...props}>
        {children}
        <SheetClose aria-label={closeLabel} className="absolute right-3 top-3 inline-flex size-8 items-center justify-center rounded-md text-muted-foreground transition-colors duration-(--motion-feedback) ease-(--motion-ease) hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none motion-reduce:transition-none">
          <X aria-hidden="true" className="size-4" />
        </SheetClose>
      </SheetPrimitive.Content>
    </SheetPortal>
  );
}

export function SheetHeader({ className, ...props }: ComponentProps<'div'>) {
  return <div data-slot="sheet-header" className={cn('flex flex-col gap-2 pr-8', className)} {...props} />;
}

export function SheetFooter({ className, ...props }: ComponentProps<'div'>) {
  return <div data-slot="sheet-footer" className={cn('mt-auto flex flex-col gap-2', className)} {...props} />;
}

export function SheetTitle({ className, ...props }: ComponentProps<typeof SheetPrimitive.Title>) {
  return <SheetPrimitive.Title data-slot="sheet-title" className={cn('font-display text-lg font-semibold leading-snug', className)} {...props} />;
}

export function SheetDescription({ className, ...props }: ComponentProps<typeof SheetPrimitive.Description>) {
  return <SheetPrimitive.Description data-slot="sheet-description" className={cn('text-sm text-muted-foreground', className)} {...props} />;
}
