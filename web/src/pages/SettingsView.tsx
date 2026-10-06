import { ArrowLeft, LayoutGrid, List } from 'lucide-react';
import { Link } from 'react-router-dom';
import { collectionCopy, commonCopy, navigationCopy, settingsCopy, themeCopy } from '@copy';
import { PageHeader } from '@design-system/components/layout/PageHeader/PageHeader';
import { SettingRow } from '@design-system/components/layout/SettingsLayout/SettingRow';
import { SettingsLayout, type SettingsCategory } from '@design-system/components/layout/SettingsLayout/SettingsLayout';
import { Button } from '@design-system/components/primitives/Button/Button';
import { ThemePreviewChoice } from '@design-system/theme/ThemePreviewChoice';
import type { CollectionView } from '@design-system/theme/collectionPreference';
import type { ThemePreference } from '@design-system/theme/themePreference';
import { cn } from '@design-system/utils/cn';

export interface SettingsViewProps {
  theme: ThemePreference;
  onThemeChange: (theme: ThemePreference) => void;
  defaultView: CollectionView;
  onDefaultViewChange: (view: CollectionView) => void;
  categories?: readonly SettingsCategory[];
  currentHref?: string;
}

const categories = [{ href: '/settings/appearance', label: themeCopy.label }] as const;
const views = ['grid', 'list'] as const;
const viewLabels = { grid: collectionCopy.grid, list: collectionCopy.list } as const;

export function SettingsView({ theme, onThemeChange, defaultView, onDefaultViewChange,
  categories: availableCategories = categories, currentHref = '/settings/appearance' }: SettingsViewProps) {
  return (
    <>
      <Link to="/agents" className="mb-4 inline-flex items-center gap-1 rounded-sm text-sm text-muted-foreground hover:text-foreground hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background">
        <ArrowLeft aria-hidden="true" className="size-4" />{commonCopy.returnToAgents}
      </Link>
      <PageHeader title={navigationCopy.settings} purpose={settingsCopy.purpose} />
      <SettingsLayout categories={availableCategories} currentHref={currentHref} navigationLabel={settingsCopy.categoriesLabel}>
        <section aria-labelledby="settings-appearance">
          <h2 id="settings-appearance" className="mb-6 font-display text-lg font-semibold text-foreground">{themeCopy.label}</h2>
          <div className="max-w-2xl">
            <SettingRow label={settingsCopy.themeLabel} description={settingsCopy.appearanceDescription}
              control={<div className="w-full min-[768px]:w-72">
                <ThemePreviewChoice value={theme} onValueChange={onThemeChange} labels={themeCopy} aria-label={themeCopy.label} />
              </div>} />
            <SettingRow label={settingsCopy.defaultCollectionView} description={settingsCopy.defaultViewDescription}
              control={<div role="group" aria-label={settingsCopy.defaultCollectionView} className="flex w-full rounded-lg border border-border p-px min-[768px]:w-auto">
                {views.map(view => (
                  <Button key={view} variant="ghost" size="sm" aria-pressed={defaultView === view} onClick={() => onDefaultViewChange(view)}
                    className={cn('min-w-0 flex-1 px-3 min-[768px]:flex-none', defaultView === view && 'bg-primary-soft text-primary-soft-foreground hover:bg-primary-soft hover:text-primary-soft-foreground')}>
                    {view === 'grid' ? <LayoutGrid aria-hidden="true" /> : <List aria-hidden="true" />}
                    {viewLabels[view]}
                  </Button>
                ))}
              </div>} />
          </div>
        </section>
      </SettingsLayout>
    </>
  );
}
