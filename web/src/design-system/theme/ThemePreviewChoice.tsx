import { RadioGroup as RadioPrimitive } from 'radix-ui';
import { RadioGroup } from '@design-system/components/inputs/RadioGroup/RadioGroup';
import { parseThemePreference, type ThemePreference } from './themePreference';

export interface ThemePreviewChoiceProps {
  value: ThemePreference;
  onValueChange: (value: ThemePreference) => void;
  labels: Record<ThemePreference, string>;
  'aria-label': string;
}

const preferences = ['light', 'dark', 'system'] as const;

const previews: Record<ThemePreference, readonly ['light' | 'dark', 'light' | 'dark']> = {
  light: ['light', 'light'],
  dark: ['dark', 'dark'],
  system: ['light', 'dark'],
};

export function ThemePreviewChoice({ value, onValueChange, labels, 'aria-label': ariaLabel }: ThemePreviewChoiceProps) {
  return (
    <RadioGroup value={value} onValueChange={next => onValueChange(parseThemePreference(next))}
      aria-label={ariaLabel} orientation="horizontal" className="grid w-full grid-cols-3 gap-2">
      {preferences.map(preference => (
        <RadioPrimitive.Item key={preference} value={preference} aria-label={labels[preference]}
          className="flex min-h-24 min-w-0 flex-col justify-center gap-2 rounded-lg border border-border-control bg-background p-2 text-xs font-medium text-foreground outline-none transition-colors duration-(--motion-control) ease-(--motion-ease) hover:border-primary focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background data-[state=checked]:border-primary data-[state=checked]:bg-primary-soft data-[state=checked]:text-primary-soft-foreground">
          <span aria-hidden="true" className="grid h-12 w-full grid-cols-2 overflow-hidden rounded-md border border-border-subtle">
            {previews[preference].map((preview, index) => (
              <span key={index} data-theme={preview} className="flex min-w-0 flex-col gap-1 bg-background p-1">
                <span className="h-1.5 w-3/4 rounded-sm bg-primary" />
                <span className="min-h-0 flex-1 rounded-sm border border-border-subtle bg-card" />
              </span>
            ))}
          </span>
          {labels[preference]}
        </RadioPrimitive.Item>
      ))}
    </RadioGroup>
  );
}
