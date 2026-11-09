import Heading from '@theme/Heading';
import Link from '@docusaurus/Link';
import styles from './styles.module.css';

// Each capability ends in a grant line: the fact an IAM reviewer checks first.
const capabilities = [
  {
    title: 'Delegated access, governed',
    description:
      'A user grants an agent access to specific third-party services at a chosen permission set, with optional expiry. One grant per user and agent, revocable at any time.',
    grant: ['one grant', 'per user', 'per agent'],
  },
  {
    title: 'An encrypted token vault',
    description:
      'The broker stores third-party access and refresh tokens encrypted at rest with envelope encryption bound to each service, so agents never hold long-lived provider credentials.',
    grant: ['envelope encryption', 'per service'],
  },
  {
    title: 'Token exchange at the edge',
    description:
      'A gateway swaps an agent token for the right third-party token on each request using RFC 8693. Policy decides every exchange, and an optional Envoy sidecar makes it transparent.',
    grant: ['RFC 8693', 'per request'],
  },
];

const paths = [
  {
    title: 'Introduction',
    href: '/docs/introduction',
    description: 'What the broker is, the delegation problem it solves, and when to reach for it',
  },
  {
    title: 'Concepts',
    href: '/docs/concepts',
    description: 'The delegation model, architecture, server modes, token exchange and encryption',
  },
  {
    title: 'Get started',
    href: '/docs/get-started',
    description: 'Run the full stack locally and walk a delegation from consent to token exchange',
  },
  {
    title: 'API reference',
    href: '/api/enduser',
    description: 'The end-user and admin OpenAPI contracts, rendered from the source specifications',
  },
];

export default function HomepageFeatures() {
  return (
    <>
      <section className={styles.section}>
        <div className="container">
          <div className={styles.sectionHeader}>
            <p className={styles.sectionLabel}>What this is</p>
            <Heading as="h2" className={styles.sectionTitle}>
              Identity infrastructure for agents that act on a user&apos;s behalf
            </Heading>
            <p className={styles.sectionDescription}>
              These docs focus on what an IAM team or platform operator cares about:
              the delegation model, security posture, deployment, configuration, and
              the API contracts other systems integrate against.
            </p>
          </div>

          <div className={styles.capabilities}>
            {capabilities.map((item) => (
              <article key={item.title} className={styles.capability}>
                <Heading as="h3" className={styles.capabilityTitle}>
                  {item.title}
                </Heading>
                <p className={styles.capabilityDescription}>{item.description}</p>
                <p className={styles.grantLine}>{item.grant.join(' · ')}</p>
              </article>
            ))}
          </div>
        </div>
      </section>

      <section className={styles.section}>
        <div className="container">
          <div className={styles.sectionHeader}>
            <p className={styles.sectionLabel}>Start in the right place</p>
            <Heading as="h2" className={styles.sectionTitle}>
              Read by task, not by internals
            </Heading>
          </div>
          <div className={styles.paths}>
            {paths.map((path) => (
              <Link key={path.href} className={styles.path} to={path.href}>
                <span className={styles.pathTitle}>
                  {path.title}
                  <span className={styles.pathArrow} aria-hidden="true">→</span>
                </span>
                <span className={styles.pathDescription}>{path.description}</span>
              </Link>
            ))}
          </div>
        </div>
      </section>
    </>
  );
}
