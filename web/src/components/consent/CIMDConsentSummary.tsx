import { CircleCheck } from 'lucide-react';
interface CIMDConsentSummaryProps {
  domain: string;
  agentName: string;
  accessTarget: string;
  agentLogoUrl?: string | null;
}

export function CIMDConsentSummary({ domain, agentName, accessTarget, agentLogoUrl }: CIMDConsentSummaryProps) {
  return (
    <div className="flex items-start gap-4 p-4 bg-neutral-50 rounded-lg border border-neutral-200">
      {agentLogoUrl && (
        <img
          src={agentLogoUrl}
          alt={`${agentName} logo`}
          className="w-12 h-12 rounded-lg object-contain flex-shrink-0"
          referrerPolicy="no-referrer"
        />
      )}
      <div className="flex flex-col gap-1">
        <div className="flex items-center gap-1.5">
          <CircleCheck className="w-4 h-4 text-success-primary flex-shrink-0" aria-hidden="true" strokeWidth={1.5} />
          <span className="text-xs text-neutral-500">Verified domain:</span>
          <span className="font-mono font-medium text-trust-deep">{domain}</span>
        </div>
        <p className="text-neutral-600 text-sm">
          <span>{agentName}</span>{' '}
          wants to access{' '}
          <span className="font-medium text-neutral-900">{accessTarget}</span>.
        </p>
      </div>
    </div>
  );
}
