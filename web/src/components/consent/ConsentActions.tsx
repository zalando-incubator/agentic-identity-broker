import { Button } from '@design-system/components/primitives/Button';
import { accessCopy } from '@copy';
import { consentCopy } from '@copy/consent';

export function ConsentActions({ pending, blocked, onDeny }: { pending: boolean; blocked: boolean; onDeny: () => void }) {
  return <div className="space-y-4 border-t border-border-soft pt-4">
    {blocked && <p className="text-sm text-muted-foreground">{consentCopy.connectFirst}</p>}
    <p data-testid="consent-next-steps" className="text-sm text-muted-foreground">{consentCopy.nextSteps}</p>
    <div className="flex flex-wrap justify-end gap-3">
      <Button variant="secondary" disabled={pending} onClick={onDeny}>{accessCopy.deny}</Button>
      <Button type="submit" variant="primary" disabled={pending || blocked} isLoading={pending}>{accessCopy.allow}</Button>
    </div>
  </div>;
}
