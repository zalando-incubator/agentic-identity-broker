import { useEffect, useRef, useState } from 'react';
import { motion } from 'motion/react';
import { consoleMotion, useConsoleReducedMotion } from '@design-system/utils/consoleMotion';
import type { ToolApprovalDetail } from '../../types/approval';

/** Only additions after a successful initial read bump the visible pending count. */
export function usePendingCountBump(approvals: readonly Pick<ToolApprovalDetail, 'id'>[] | undefined, stale = false): number {
  const previousIds = useRef<Set<string> | null>(null);
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    if (!approvals || stale) return;
    const previous = previousIds.current;
    const ids = new Set<string>();
    let added = false;
    for (const approval of approvals) {
      ids.add(approval.id);
      if (previous && !previous.has(approval.id)) added = true;
    }
    previousIds.current = ids;
    if (added) setRevision(value => value + 1);
  }, [approvals, stale]);
  return revision;
}

const countBump = { scale: [1, 1.2, 1] };
const countFade = { opacity: [0.6, 1] };

export function PendingApprovalCount({ count, bumpRevision }: { count: number; bumpRevision: number }) {
  const reduced = useConsoleReducedMotion();
  return <motion.span key={bumpRevision} className="inline-block tabular-nums" initial={bumpRevision > 0 ? { opacity: 1, scale: 1 } : false}
    animate={bumpRevision > 0 ? reduced ? countFade : countBump : undefined}
    transition={{ duration: consoleMotion.feedback, ease: consoleMotion.ease }}>{count}</motion.span>;
}
