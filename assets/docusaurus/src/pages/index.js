import clsx from 'clsx';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import useBaseUrl from '@docusaurus/useBaseUrl';
import Layout from '@theme/Layout';
import ThemedImage from '@theme/ThemedImage';
import Heading from '@theme/Heading';
import HomepageFeatures from '@site/src/components/HomepageFeatures';
import styles from './index.module.css';

const heroLinks = [
  {
    to: '/docs/introduction',
    label: 'Read the introduction',
    className: 'button button--primary button--lg',
  },
  {
    to: '/docs/get-started',
    label: 'Get started',
    className: 'button button--secondary button--lg',
  },
  {
    to: '/api/enduser',
    label: 'API reference',
    className: 'button button--secondary button--lg',
  },
];

function HomepageHeader() {
  const wordmarkSources = {
    light: useBaseUrl('/img/AIB_Wordmark_Black.svg'),
    dark: useBaseUrl('/img/AIB_Wordmark_White.svg'),
  };

  return (
    <header className={clsx('hero', styles.heroBanner)}>
      <div className="container">
        <div className={styles.heroContent}>
          <span className={styles.eyebrow}>Open source · OAuth2 · Delegated access</span>
          <Heading as="h1" className={styles.heroTitle}>
            <ThemedImage
              alt="Agentic Identity Broker"
              className={styles.heroWordmark}
              sources={wordmarkSources}
            />
          </Heading>
          <p className={styles.heroTagline}>
            Let users delegate scoped, revocable access to AI agents — without
            handing agents their credentials.
          </p>
          <p className={styles.heroDescription}>
            The Agentic Identity Broker is a governed trust boundary between your
            users, the AI agents acting for them, and the third-party services
            those agents call. Users consent once per agent and service; the
            broker holds the third-party tokens encrypted and hands agents only
            narrowly-scoped, exchangeable, auditable access.
          </p>
          <div className={styles.buttons}>
            {heroLinks.map((link) => (
              <Link key={link.to} className={link.className} to={link.to}>
                {link.label}
              </Link>
            ))}
          </div>
          <div className={styles.heroHighlights}>
            <div className={styles.heroHighlight}>
              <strong>Delegation with consent</strong>
              <span>Per-agent, per-service, scoped grants users can revoke.</span>
            </div>
            <div className={styles.heroHighlight}>
              <strong>Encrypted token vault</strong>
              <span>Third-party tokens sealed at rest with envelope encryption.</span>
            </div>
            <div className={styles.heroHighlight}>
              <strong>Standards-based</strong>
              <span>OAuth2, PKCE, RFC 8414 metadata, and RFC 8693 token exchange.</span>
            </div>
          </div>
        </div>
      </div>
    </header>
  );
}

export default function Home() {
  const {siteConfig} = useDocusaurusContext();

  return (
    <Layout
      title="OAuth2 delegation and consent for AI agents"
      description={`${siteConfig.title} — an open-source OAuth2 delegation and consent broker that lets users grant AI agents scoped, revocable access to third-party services.`}>
      <HomepageHeader />
      <main>
        <HomepageFeatures />
      </main>
    </Layout>
  );
}
