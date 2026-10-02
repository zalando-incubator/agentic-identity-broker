import { RadioGroup, RadioGroupItem } from '@design-system/components/inputs/RadioGroup/RadioGroup';
import {
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
} from '@design-system/components/overlays/DropdownMenu/DropdownMenu';
import { useTheme } from './ThemeProvider';
import { parseThemePreference, type ThemePreference } from './themePreference';

export interface ThemeChoiceProps {
  labels: Record<ThemePreference, string>;
  'aria-label': string;
  presentation?: 'form' | 'menu';
  className?: string;
}

const preferences = ['light', 'dark', 'system'] as const;

export function ThemeChoice({ labels, presentation = 'form', className, 'aria-label': ariaLabel }: ThemeChoiceProps) {
  const { preference, setTheme } = useTheme();
  const onValueChange = (value: string) => setTheme(parseThemePreference(value));

  if (presentation === 'menu') {
    return (
      <DropdownMenuRadioGroup value={preference} onValueChange={onValueChange} aria-label={ariaLabel} className={className}>
        {preferences.map((value) => (
          <DropdownMenuRadioItem key={value} value={value}>{labels[value]}</DropdownMenuRadioItem>
        ))}
      </DropdownMenuRadioGroup>
    );
  }

  return (
    <RadioGroup value={preference} onValueChange={onValueChange} aria-label={ariaLabel} className={className}>
      {preferences.map((value) => (
        <label key={value} className="flex min-h-9 w-fit cursor-pointer items-center gap-3 text-sm text-foreground">
          <RadioGroupItem value={value} />
          {labels[value]}
        </label>
      ))}
    </RadioGroup>
  );
}
