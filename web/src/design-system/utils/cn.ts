/**
 * Utility for merging Tailwind CSS classes with proper precedence
 * Combines clsx (conditional classes) with tailwind-merge (deduplication)
 *
 * @example
 * cn('px-4 py-2', isActive && 'bg-primary', className)
 * // Correctly handles conflicting classes like 'px-2 px-4' → 'px-4'
 */

import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
