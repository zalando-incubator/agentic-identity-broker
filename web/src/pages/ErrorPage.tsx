import { Link } from 'react-router-dom';
import { PageHeader } from '@design-system/components/layout/PageHeader/PageHeader';
import { Button } from '@design-system/components/primitives/Button/Button';
import { commonCopy } from '@copy';

export function ErrorPage() {
  return (
    <section className="mx-auto flex max-w-2xl flex-col items-start gap-6 py-12">
      <p aria-hidden="true" className="font-mono text-4xl text-muted-foreground">{commonCopy.notFoundCode}</p>
      <PageHeader title={commonCopy.pageNotFound} purpose={commonCopy.pageNotFoundDescription} />
      <Button asChild variant="secondary"><Link to="/delegations">{commonCopy.returnToAgents}</Link></Button>
    </section>
  );
}

export default ErrorPage;
