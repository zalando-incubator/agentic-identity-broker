import clsx from 'clsx';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import HomepageFeatures from '@site/src/components/HomepageFeatures';
import styles from './index.module.css';

const permissionSets = [
  {name: 'Read repositories', service: 'GitHub', requirement: 'Mandatory', on: true, locked: true},
  {name: 'Send email', service: 'Google', requirement: 'Optional', on: true},
  {name: 'Query data', service: 'Databricks', requirement: 'Optional', on: false},
];

function Toggle({on, locked}) {
  return (
    <span
      aria-hidden="true"
      className={clsx(styles.toggle, on && styles.toggleOn, locked && styles.toggleLocked)}>
      <span className={styles.toggleKnob} />
    </span>
  );
}

// A consent screen drawn in code, so it stays sharp and follows the color mode.
// It mirrors the real consent SPA: one agent, its permission sets, an optional
// expiry, and the grant the broker records once the user approves.
function ConsentPreview() {
  return (
    <figure className={styles.preview} aria-label="Consent screen for delegating access to an AI agent">
      <div className={styles.window}>
        <div className={styles.windowBar}>
          <span className={styles.windowUrl}>broker.example.com/consent</span>
        </div>

        <div className={styles.consent}>
          <div className={styles.agent}>
            <span className={styles.agentMark}>RA</span>
            <div>
              <p className={styles.agentName}>Release assistant</p>
              <p className={styles.agentMeta}>asks to act on your behalf</p>
            </div>
          </div>

          <ul className={styles.sets}>
            {permissionSets.map((set) => (
              <li key={set.name} className={styles.set}>
                <div className={styles.setText}>
                  <span className={styles.setName}>{set.name}</span>
                  <span className={styles.setMeta}>
                    {set.service} · {set.requirement}
                  </span>
                </div>
                <Toggle on={set.on} locked={set.locked} />
              </li>
            ))}
          </ul>

          <div className={styles.validity}>
            <span>Valid until</span>
            <span className={styles.validityDate}>2026-12-31</span>
          </div>

          <div className={styles.consentActions}>
            <span className={styles.fakeButton}>Decline</span>
            <span className={clsx(styles.fakeButton, styles.fakeButtonPrimary)}>Approve</span>
          </div>
        </div>
      </div>

      <figcaption className={styles.receipt}>
        <span className={styles.receiptStatus}>
          <span className={styles.statusDot} aria-hidden="true" />
          grant active
        </span>
        <dl className={styles.receiptFields}>
          <div><dt>principal</dt><dd>m.keller</dd></div>
          <div><dt>agent</dt><dd>release-assistant</dd></div>
          <div><dt>permission_sets</dt><dd>read-repositories, send-email</dd></div>
          <div><dt>valid_until</dt><dd>2026-12-31</dd></div>
        </dl>
      </figcaption>
    </figure>
  );
}

function HomepageHeader() {
  return (
    <header className={styles.hero}>
      <div className={clsx('container', styles.heroLayout)}>
        <div className={styles.heroContent}>
          <p className={styles.heroMeta}>Open source · MIT licensed</p>
          <h1 className={styles.heroTitle}>Governed delegated access for AI agents</h1>
          <p className={styles.heroDescription}>
            Your assistant reads tickets, reviews pull requests and sends email for
            your people. OAuth lets it connect, but cannot tell one agent from
            another or take back one agent&apos;s access. The broker decides what
            each agent can do for each person, keeps provider tokens out of the
            agents, and gives every user one place to approve and revoke.
          </p>
          <div className={styles.actions}>
            <Link className="button button--primary button--lg" to="/docs/get-started">
              Get started
            </Link>
            <Link className={styles.secondaryAction} to="/docs/introduction/why">
              Why does this exist?<span aria-hidden="true">→</span>
            </Link>
          </div>
        </div>
        <ConsentPreview />
      </div>
    </header>
  );
}

export default function Home() {
  const {siteConfig} = useDocusaurusContext();

  return (
    <Layout
      title="OAuth2 delegation and consent for AI agents"
      description={`${siteConfig.title}, an open-source OAuth2 delegation and consent broker that lets users grant AI agents scoped, revocable access to third-party services.`}>
      <HomepageHeader />
      <main>
        <HomepageFeatures />
      </main>
    </Layout>
  );
}
