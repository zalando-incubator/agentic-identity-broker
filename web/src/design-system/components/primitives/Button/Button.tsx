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
import type { ComponentProps, SyntheticEvent } from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { Loader2 } from 'lucide-react';
import { Slot } from 'radix-ui';
import { cn } from '@design-system/utils';

const buttonVariants = cva(
  'inline-flex shrink-0 items-center justify-center gap-2 whitespace-nowrap rounded-md border border-transparent text-sm font-medium outline-none transition-colors duration-[var(--motion-feedback)] ease-[var(--motion-ease)] motion-reduce:transition-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:border-border disabled:bg-muted disabled:text-muted-foreground disabled:no-underline aria-disabled:cursor-not-allowed aria-disabled:border-border aria-disabled:bg-muted aria-disabled:text-muted-foreground aria-disabled:no-underline [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0',
  {
    variants: {
      variant: {
        primary: 'bg-primary text-primary-foreground hover:underline underline-offset-4',
        secondary: 'border-border bg-secondary text-secondary-foreground hover:bg-accent hover:underline underline-offset-4',
        outline: 'border-border bg-background text-foreground hover:bg-accent hover:text-accent-foreground',
        ghost: 'bg-transparent text-foreground hover:bg-accent hover:text-accent-foreground',
        destructive: 'bg-destructive text-destructive-foreground hover:underline underline-offset-4',
      },
      size: {
        default: 'h-10 px-4 py-2',
        sm: 'h-9 px-3',
        lg: 'h-11 px-6',
        icon: 'size-10',
      },
    },
    defaultVariants: { variant: 'primary', size: 'default' },
  },
);

export interface ButtonProps extends ComponentProps<'button'>, VariantProps<typeof buttonVariants> {
  asChild?: boolean;
  isLoading?: boolean;
}

function preventActivation(event: SyntheticEvent) {
  event.preventDefault();
  event.stopPropagation();
}

export function Button({
  className, variant = 'primary', size = 'default', asChild = false,
  isLoading = false, disabled = false, children, type = 'button',
  onClickCapture, onKeyDownCapture, tabIndex, ...props
}: ButtonProps) {
  const Comp = asChild ? Slot.Root : 'button';
  const isDisabled = disabled || isLoading;

  return (
    <Comp
      {...props}
      data-slot="button"
      data-variant={variant}
      data-size={size}
      type={asChild ? undefined : type}
      disabled={asChild ? undefined : isDisabled}
      aria-disabled={asChild && isDisabled ? true : props['aria-disabled']}
      aria-busy={isLoading ? true : props['aria-busy']}
      tabIndex={asChild && isDisabled ? -1 : tabIndex}
      className={cn(buttonVariants({ variant, size }), className)}
      onClickCapture={(event) => {
        // Capture prevents a slotted child's click handler as well as navigation.
        if (isDisabled) preventActivation(event);
        else onClickCapture?.(event);
      }}
      onKeyDownCapture={(event) => {
        if (isDisabled && (event.key === 'Enter' || event.key === ' ')) preventActivation(event);
        else onKeyDownCapture?.(event);
      }}
    >
      {isLoading && <Loader2 aria-hidden="true" className="motion-safe:animate-spin" />}
      <Slot.Slottable>{children}</Slot.Slottable>
    </Comp>
  );
}

export { buttonVariants };
