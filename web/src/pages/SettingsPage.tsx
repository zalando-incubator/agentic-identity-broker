import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@design-system/components/data-display/Card/Card';
import { PageHeader } from '@design-system/components/layout/PageHeader/PageHeader';
import { ThemeChoice } from '@design-system/theme/ThemeChoice';
import { navigationCopy, themeCopy } from '@copy';
import { settingsCopy } from '@copy/settings';

export default function SettingsPage() {
  return (
    <>
      <PageHeader title={navigationCopy.settings} purpose={settingsCopy.purpose} />
      <Card className="max-w-xl">
        <CardHeader>
          <CardTitle>{themeCopy.label}</CardTitle>
          <CardDescription>{settingsCopy.appearanceDescription}</CardDescription>
        </CardHeader>
        <CardContent>
          <ThemeChoice labels={themeCopy} aria-label={themeCopy.label} />
        </CardContent>
      </Card>
    </>
  );
}
