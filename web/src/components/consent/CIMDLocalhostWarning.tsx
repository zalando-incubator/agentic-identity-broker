import { TriangleAlert } from 'lucide-react';

interface CIMDLocalhostWarningProps {
  agentDisplayName: string;
}

export function CIMDLocalhostWarning({ agentDisplayName }: CIMDLocalhostWarningProps) {
  return (
    <div role="alert" className="flex gap-3 p-4 bg-warning-light border border-warning-primary rounded-lg">
      <TriangleAlert className="w-5 h-5 text-warning-primary flex-shrink-0 mt-0.5" aria-hidden="true" strokeWidth={1.5} />
      <p className="text-sm text-neutral-800">
        This app is requesting a redirect to your local machine. Ensure you started this request from{' '}
        <span className="font-medium">{agentDisplayName}</span>.
      </p>
    </div>
  );
}
