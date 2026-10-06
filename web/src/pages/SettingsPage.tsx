import { useTheme } from '@design-system/theme/ThemeProvider';
import { useDefaultCollectionView } from '@hooks/useCollectionView';
import { SettingsView } from './SettingsView';

export default function SettingsPage() {
  const { preference, setTheme } = useTheme();
  const { view, setView } = useDefaultCollectionView();
  return <SettingsView theme={preference} onThemeChange={setTheme} defaultView={view} onDefaultViewChange={setView} />;
}
