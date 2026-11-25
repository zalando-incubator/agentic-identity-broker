import React from 'react';
import clsx from 'clsx';

import styles from './styles.module.css';

const VERDICTS = {
  passed: {label: '✓ tested', tone: 'passed', hint: 'The acceptance scenario for this example passed.'},
  'not run': {label: '○ not run', tone: 'neutral', hint: 'The acceptance scenario for this example did not run in the last check.'},
  stale: {label: '✗ out of date', tone: 'failed', hint: 'This page shows an exchange that the acceptance scenario did not perform.'},
  failed: {label: '✗ failing', tone: 'failed', hint: 'The acceptance scenario for this example failed.'},
  unbound: {label: '? no test', tone: 'failed', hint: 'No acceptance scenario is bound to this example.'},
};
const UNCHECKED = {label: 'test', tone: 'neutral', hint: 'Acceptance scenario for this example.'};

export default function ExampleBadge({example, verdict, href}) {
  const {label, tone, hint} = VERDICTS[verdict] ?? UNCHECKED;
  const title = `${hint} (${example})`;
  const className = clsx(styles.badge, styles[tone]);
  if (!href) {
    return (
      <span className={className} title={title}>
        {label}
      </span>
    );
  }
  return (
    <a className={className} href={href} title={title} target="_blank" rel="noopener noreferrer">
      {label}
    </a>
  );
}
