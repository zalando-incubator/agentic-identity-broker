import React from 'react';
import clsx from 'clsx';
import CodeBlock from '@theme/CodeBlock';

import styles from './styles.module.css';

function isJson(body) {
  try {
    JSON.parse(body);
    return true;
  } catch {
    return false;
  }
}

function Message({label, headers, body, children}) {
  return (
    <section className={styles.message} aria-label={label}>
      <div className={styles.startLine}>{children}</div>
      {headers.length > 0 && (
        <dl className={styles.headers}>
          {headers.map(({name, value}) => (
            <div key={name}>
              <dt>{name}:</dt> <dd>{value}</dd>
            </div>
          ))}
        </dl>
      )}
      {body && (
        <CodeBlock className={styles.body} language={isJson(body) ? 'json' : 'text'}>
          {body}
        </CodeBlock>
      )}
    </section>
  );
}

function statusTone(status) {
  if (status >= 500) return styles.failed;
  if (status >= 400) return styles.refused;
  return styles.passed;
}

export default function HttpExchange({exchange}) {
  const {request, response} = JSON.parse(exchange);
  return (
    <div className={styles.exchange}>
      <Message label="Request" headers={request.headers} body={request.body}>
        <span className={clsx(styles.pill, styles.method)}>{request.method}</span>
        <code className={styles.target}>{request.target}</code>
      </Message>
      <Message label="Response" headers={response.headers} body={response.body}>
        <span className={clsx(styles.pill, statusTone(response.status))}>{response.status}</span>
        <span className={styles.reason}>{response.reason}</span>
      </Message>
    </div>
  );
}
