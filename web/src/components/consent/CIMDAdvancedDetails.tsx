import { Accordion } from '@design-system/components/advanced/Accordion';

interface CIMDAdvancedDetailsProps {
  clientIdUrl: string;
  redirectUri: string;
  requestedScopes: string[];
}

export function CIMDAdvancedDetails({ clientIdUrl, redirectUri, requestedScopes }: CIMDAdvancedDetailsProps) {
  const content = (
    <dl className="space-y-3">
      <DetailRow label="Client ID" value={clientIdUrl} mono />
      <DetailRow label="Redirect URI" value={redirectUri} mono />
      <div>
        <dt className="text-xs font-medium text-neutral-500 mb-1">Requested Scopes</dt>
        <dd className="flex flex-wrap gap-1.5">
          {requestedScopes.map((scope) => (
            <span key={scope} className="inline-flex items-center px-2 py-0.5 rounded text-xs font-mono bg-neutral-100 text-neutral-700">
              {scope}
            </span>
          ))}
          {requestedScopes.length === 0 && (
            <span className="text-sm text-neutral-500">None requested</span>
          )}
        </dd>
      </div>
    </dl>
  );

  return (
    <Accordion
      size="sm"
      items={[{ id: 'cimd-advanced', title: 'Advanced Details', content }]}
    />
  );
}

function DetailRow({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div>
      <dt className="text-xs font-medium text-neutral-500 mb-0.5">{label}</dt>
      <dd className={`text-sm text-neutral-800 break-all ${mono ? 'font-mono' : ''}`}>{value}</dd>
    </div>
  );
}
