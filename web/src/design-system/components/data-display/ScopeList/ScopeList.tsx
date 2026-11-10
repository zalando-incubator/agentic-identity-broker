/**
 * ScopeList Component
 *
 * Domain-specific component for displaying OAuth scopes/permissions in consent UI.
 * Follows the "Refined Trust Architecture" design system.
 *
 * Features:
 * - List of scopes with descriptions and category grouping
 * - Scope icons/visual indicators (user, email, calendar, etc.)
 * - Grouping scopes by category with expand/collapse
 * - Optional checkbox selection for each scope
 * - Optional action buttons per scope
 * - Search/filter functionality
 * - Visual hierarchy (title, description, secondary text)
 * - Compact and default size variants
 * - Empty state handling
 * - Loading state with skeleton
 * - Risk level indicators (low, medium, high)
 * - WCAG 2.1 AA compliant
 */

import React, { useState, useMemo } from 'react';
import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from '@design-system/utils';
import { Stack } from '../../layout/Stack';
import { Checkbox } from '../../inputs/Checkbox';
import { Badge } from '../../primitives/Badge';
import { Skeleton } from '../../feedback/Skeleton';
import {
  Calendar,
  ChevronDown,
  FileText,
  Folder,
  Mail,
  Search,
  ShieldCheck,
  SlidersHorizontal,
  User,
  Users,
} from 'lucide-react';

const scopeListVariants = cva('w-full', {
  variants: {
    size: {
      compact: 'text-sm',
      default: 'text-base',
    },
  },
  defaultVariants: {
    size: 'default',
  },
});

const scopeItemVariants = cva(
  'flex items-start gap-3 p-3 rounded-lg transition-colors border',
  {
    variants: {
      size: {
        compact: 'p-2 gap-2',
        default: 'p-3 gap-3',
      },
      selectable: {
        true: 'cursor-pointer hover:bg-neutral-50',
        false: 'bg-white',
      },
    },
    defaultVariants: {
      size: 'default',
      selectable: false,
    },
  },
);

const categoryHeaderVariants = cva(
  'flex items-center justify-between p-3 rounded-lg cursor-pointer hover:bg-neutral-50 border border-neutral-200',
  {
    variants: {
      size: {
        compact: 'p-2',
        default: 'p-3',
      },
      expanded: {
        true: 'bg-neutral-50',
        false: 'bg-white',
      },
    },
    defaultVariants: {
      size: 'default',
      expanded: false,
    },
  },
);

export interface Scope {
  /** Unique identifier for the scope */
  id: string;
  /** Scope name (e.g., "user:read", "email:send") */
  name: string;
  /** Human-readable description */
  description: string;
  /** Category for grouping (e.g., "Profile", "Email") */
  category: string;
  /** Optional icon element */
  icon?: React.ReactNode;
  /** Risk level for visual coding */
  riskLevel?: 'low' | 'medium' | 'high';
  /** Selection state */
  selected?: boolean;
  /** Selection callback */
  onSelect?: (id: string, selected: boolean) => void;
}

export interface ScopeListProps
  extends
    Omit<React.ComponentPropsWithoutRef<'div'>, 'children'>,
    VariantProps<typeof scopeListVariants> {
  /** Array of scopes to display */
  scopes: Scope[];
  /** Enable checkbox selection */
  selectable?: boolean;
  /** Enable search/filter functionality */
  searchable?: boolean;
  /** Enable expand/collapse for categories */
  expandable?: boolean;
  /** Size variant */
  size?: 'compact' | 'default';
  /** Loading state */
  loading?: boolean;
  /** Empty state content */
  empty?: React.ReactNode;
  /** Selection change callback */
  onSelectionChange?: (selectedIds: string[]) => void;
}

/**
 * Default icons for common scope categories
 */
const getCategoryIcon = (category: string): React.ReactNode => {
  const iconClass = 'w-5 h-5';

  switch (category.toLowerCase()) {
    case 'profile':
    case 'user':
      return (
        <User className={iconClass} aria-hidden="true" strokeWidth={1.5} />
      );
    case 'email':
      return (
        <Mail className={iconClass} aria-hidden="true" strokeWidth={1.5} />
      );
    case 'calendar':
      return (
        <Calendar className={iconClass} aria-hidden="true" strokeWidth={1.5} />
      );
    case 'contacts':
      return (
        <Users className={iconClass} aria-hidden="true" strokeWidth={1.5} />
      );
    case 'documents':
    case 'files':
      return (
        <FileText className={iconClass} aria-hidden="true" strokeWidth={1.5} />
      );
    case 'storage':
    case 'drive':
      return (
        <Folder className={iconClass} aria-hidden="true" strokeWidth={1.5} />
      );
    case 'api':
    case 'admin':
      return (
        <SlidersHorizontal
          className={iconClass}
          aria-hidden="true"
          strokeWidth={1.5}
        />
      );
    default:
      return (
        <ShieldCheck
          className={iconClass}
          aria-hidden="true"
          strokeWidth={1.5}
        />
      );
  }
};

/**
 * Get badge variant for risk level
 */
const getRiskBadgeVariant = (riskLevel?: 'low' | 'medium' | 'high') => {
  switch (riskLevel) {
    case 'high':
      return 'error';
    case 'medium':
      return 'warning';
    case 'low':
    default:
      return 'success';
  }
};

/**
 * ChevronDown icon for expandable sections
 */
const ChevronDownIcon: React.FC<{ className?: string }> = ({ className }) => (
  <ChevronDown
    className={cn('w-5 h-5 transition-transform', className)}
    aria-hidden="true"
    strokeWidth={1.5}
  />
);

/**
 * SearchIcon for search input
 */
const SearchIcon: React.FC<{ className?: string }> = ({ className }) => (
  <Search
    className={cn('w-5 h-5', className)}
    aria-hidden="true"
    strokeWidth={1.5}
  />
);

/**
 * ScopeList component for displaying OAuth scopes/permissions.
 * Supports grouping, selection, search, and risk indicators.
 *
 * @example
 * ```tsx
 * <ScopeList
 *   scopes={[
 *     { id: '1', name: 'user:read', description: 'Read user profile', category: 'Profile' },
 *     { id: '2', name: 'email:send', description: 'Send emails', category: 'Email' },
 *   ]}
 * />
 *
 * <ScopeList
 *   scopes={scopes}
 *   selectable
 *   searchable
 *   onSelectionChange={(ids) => console.log('Selected:', ids)}
 * />
 * ```
 */
export const ScopeList = React.forwardRef<HTMLDivElement, ScopeListProps>(
  (
    {
      scopes,
      selectable = false,
      searchable = false,
      expandable = true,
      size = 'default',
      loading = false,
      empty,
      onSelectionChange,
      className,
      ...props
    },
    ref,
  ) => {
    const [searchQuery, setSearchQuery] = useState('');
    const [expandedCategories, setExpandedCategories] = useState<Set<string>>(
      new Set(),
    );
    const [selectedIds, setSelectedIds] = useState<Set<string>>(
      new Set(scopes.filter((s) => s.selected).map((s) => s.id)),
    );

    // Group scopes by category
    const groupedScopes = useMemo(() => {
      const filtered = scopes.filter(
        (scope) =>
          !searchQuery ||
          scope.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
          scope.description.toLowerCase().includes(searchQuery.toLowerCase()) ||
          scope.category.toLowerCase().includes(searchQuery.toLowerCase()),
      );

      const groups = new Map<string, Scope[]>();
      filtered.forEach((scope) => {
        const category = scope.category || 'Other';
        if (!groups.has(category)) {
          groups.set(category, []);
        }
        groups.get(category)!.push(scope);
      });

      // Sort categories alphabetically
      return Array.from(groups.entries()).sort(([a], [b]) =>
        a.localeCompare(b),
      );
    }, [scopes, searchQuery]);

    // Toggle category expansion
    const toggleCategory = (category: string) => {
      const newExpanded = new Set(expandedCategories);
      if (newExpanded.has(category)) {
        newExpanded.delete(category);
      } else {
        newExpanded.add(category);
      }
      setExpandedCategories(newExpanded);
    };

    // Handle scope selection
    const handleScopeSelection = (scopeId: string, selected: boolean) => {
      const newSelected = new Set(selectedIds);
      if (selected) {
        newSelected.add(scopeId);
      } else {
        newSelected.delete(scopeId);
      }
      setSelectedIds(newSelected);
      onSelectionChange?.(Array.from(newSelected));
    };

    // Initialize expanded categories on mount
    React.useEffect(() => {
      if (!expandable) {
        setExpandedCategories(
          new Set(groupedScopes.map(([category]) => category)),
        );
      }
    }, [expandable, groupedScopes]);

    // Loading state
    if (loading) {
      return (
        <div
          ref={ref}
          className={cn(scopeListVariants({ size }), className)}
          {...props}
        >
          <Stack gap="md">
            {[1, 2, 3].map((i) => (
              <div key={i} className="p-4 border border-neutral-200 rounded-lg">
                <Stack gap="sm">
                  <Skeleton width="120px" height="20px" />
                  <Skeleton count={2} gap="0.5rem" />
                </Stack>
              </div>
            ))}
          </Stack>
        </div>
      );
    }

    // Empty state
    if (scopes.length === 0) {
      return (
        <div
          ref={ref}
          className={cn(
            scopeListVariants({ size }),
            'flex flex-col items-center justify-center p-12 text-center border-2 border-dashed border-neutral-300 rounded-lg',
            className,
          )}
          {...props}
        >
          {empty || (
            <>
              <ShieldCheck
                className="w-12 h-12 text-neutral-400 mb-4"
                aria-hidden="true"
                strokeWidth={1.5}
              />
              <h3 className="text-lg font-medium text-neutral-900 mb-1">
                No scopes available
              </h3>
              <p className="text-sm text-neutral-600">
                There are no permissions to display.
              </p>
            </>
          )}
        </div>
      );
    }

    return (
      <div
        ref={ref}
        className={cn(scopeListVariants({ size }), className)}
        {...props}
      >
        <Stack gap="md">
          {/* Search input */}
          {searchable && (
            <div className="relative">
              <div className="absolute inset-y-0 left-0 pl-3 flex items-center pointer-events-none">
                <SearchIcon className="text-neutral-400" />
              </div>
              <input
                type="text"
                placeholder="Search scopes..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="block w-full pl-10 pr-3 py-2 border border-neutral-300 rounded-lg focus:ring-2 focus:ring-trust focus:border-trust sm:text-sm"
                aria-label="Search scopes"
              />
            </div>
          )}

          {/* Grouped scopes */}
          {groupedScopes.length === 0 ? (
            <div className="flex flex-col items-center justify-center p-8 text-center border border-neutral-200 rounded-lg">
              <SearchIcon className="text-neutral-400 w-8 h-8 mb-2" />
              <p className="text-sm text-neutral-600">
                No scopes match your search.
              </p>
            </div>
          ) : (
            <Stack gap="sm">
              {groupedScopes.map(([category, categoryScopes]) => {
                const isExpanded =
                  expandedCategories.has(category) || !expandable;

                return (
                  <div
                    key={category}
                    className="border border-neutral-200 rounded-lg"
                  >
                    {/* Category header */}
                    {expandable ? (
                      <button
                        type="button"
                        onClick={() => toggleCategory(category)}
                        className={cn(
                          categoryHeaderVariants({
                            size,
                            expanded: isExpanded,
                          }),
                          'w-full',
                        )}
                        aria-expanded={isExpanded}
                        aria-controls={`category-${category}`}
                      >
                        <div className="flex items-center gap-2">
                          <span className="text-neutral-600">
                            {getCategoryIcon(category)}
                          </span>
                          <span className="font-medium text-neutral-900">
                            {category}
                          </span>
                          <Badge variant="neutral" size="sm">
                            {categoryScopes.length}
                          </Badge>
                        </div>
                        <ChevronDownIcon
                          className={cn(
                            'text-neutral-600',
                            isExpanded && 'rotate-180',
                          )}
                        />
                      </button>
                    ) : (
                      <div
                        className={cn(
                          categoryHeaderVariants({ size }),
                          'cursor-default',
                        )}
                      >
                        <div className="flex items-center gap-2">
                          <span className="text-neutral-600">
                            {getCategoryIcon(category)}
                          </span>
                          <span className="font-medium text-neutral-900">
                            {category}
                          </span>
                          <Badge variant="neutral" size="sm">
                            {categoryScopes.length}
                          </Badge>
                        </div>
                      </div>
                    )}

                    {/* Category scopes */}
                    {isExpanded && (
                      <div
                        id={`category-${category}`}
                        className="border-t border-neutral-200"
                      >
                        <Stack gap="xs" className="p-2">
                          {categoryScopes.map((scope) => {
                            const isSelected = selectedIds.has(scope.id);

                            return (
                              <div
                                key={scope.id}
                                className={cn(
                                  scopeItemVariants({ size, selectable }),
                                  isSelected &&
                                    'border-trust bg-trust-light/10',
                                )}
                                onClick={
                                  selectable
                                    ? () =>
                                        handleScopeSelection(
                                          scope.id,
                                          !isSelected,
                                        )
                                    : undefined
                                }
                                onKeyDown={
                                  selectable
                                    ? (event) => {
                                        if (
                                          event.key === 'Enter' ||
                                          event.key === ' '
                                        ) {
                                          event.preventDefault();
                                          handleScopeSelection(
                                            scope.id,
                                            !isSelected,
                                          );
                                        }
                                      }
                                    : undefined
                                }
                                role={selectable ? 'button' : undefined}
                                tabIndex={selectable ? 0 : undefined}
                              >
                                {/* Checkbox */}
                                {selectable && (
                                  <div className="flex-shrink-0 pt-0.5">
                                    <Checkbox
                                      id={`scope-${scope.id}`}
                                      checked={isSelected}
                                      onChange={(e) =>
                                        handleScopeSelection(
                                          scope.id,
                                          e.target.checked,
                                        )
                                      }
                                      onClick={(e) => e.stopPropagation()}
                                      size={size === 'compact' ? 'sm' : 'md'}
                                    />
                                  </div>
                                )}

                                {/* Scope icon */}
                                <div className="flex-shrink-0 pt-0.5 text-neutral-600">
                                  {scope.icon ||
                                    getCategoryIcon(scope.category)}
                                </div>

                                {/* Scope content */}
                                <div className="flex-1 min-w-0">
                                  <div className="flex items-start justify-between gap-2">
                                    <div className="flex-1 min-w-0">
                                      <p
                                        className={cn(
                                          'font-mono font-medium text-neutral-900',
                                          size === 'compact'
                                            ? 'text-xs'
                                            : 'text-sm',
                                        )}
                                      >
                                        {scope.name}
                                      </p>
                                      <p
                                        className={cn(
                                          'text-neutral-600 mt-0.5',
                                          size === 'compact'
                                            ? 'text-xs'
                                            : 'text-sm',
                                        )}
                                      >
                                        {scope.description}
                                      </p>
                                    </div>

                                    {/* Risk badge */}
                                    {scope.riskLevel && (
                                      <Badge
                                        variant={getRiskBadgeVariant(
                                          scope.riskLevel,
                                        )}
                                        size="sm"
                                        className="flex-shrink-0"
                                      >
                                        {scope.riskLevel}
                                      </Badge>
                                    )}
                                  </div>
                                </div>
                              </div>
                            );
                          })}
                        </Stack>
                      </div>
                    )}
                  </div>
                );
              })}
            </Stack>
          )}
        </Stack>
      </div>
    );
  },
);

ScopeList.displayName = 'ScopeList';

export default ScopeList;
