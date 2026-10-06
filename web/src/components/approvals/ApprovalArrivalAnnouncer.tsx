import { useEffect, useRef, useState } from 'react';
import { usePendingApprovals } from '@hooks/usePendingApprovals';
import { usePrincipal } from '@services/query/QueryProvider';
import { approvalQueueCopy as copy } from '@copy/approvalQueue';

/** Supplies content to ConsoleShell's single polite region; never creates a region or moves focus. */
export function ApprovalArrivalAnnouncer() {
  const { principal } = usePrincipal();
  const pending = usePendingApprovals();
  const previous = useRef<{ principal: string; statuses: Map<string, string> } | null>(null);
  const [announcement, setAnnouncement] = useState({ message: '', revision: 0 });

  useEffect(() => {
    if (!pending.data || pending.stale) return;
    const statuses = new Map(pending.data.map(approval => [approval.id, approval.status]));
    const before = previous.current;
    previous.current = { principal, statuses };
    if (!before || before.principal !== principal) {
      setAnnouncement({ message: '', revision: 0 });
      return;
    }
    let arrivals = 0;
    let changes = 0;
    for (const [id, status] of statuses) {
      if (!before.statuses.has(id)) arrivals++;
      else if (before.statuses.get(id) !== status) changes++;
    }
    for (const id of before.statuses.keys()) {
      if (!statuses.has(id)) changes++;
    }
    if (arrivals || changes) {
      const message = [arrivals ? copy.arrivals(arrivals) : '', changes ? copy.changed(changes) : ''].filter(Boolean).join(' ');
      setAnnouncement(current => ({ message, revision: current.revision + 1 }));
    }
  }, [pending.data, pending.dataUpdatedAt, pending.stale, principal]);

  return announcement.message ? <span key={announcement.revision}>{announcement.message}</span> : null;
}
