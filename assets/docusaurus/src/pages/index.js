import clsx from 'clsx';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import useBaseUrl from '@docusaurus/useBaseUrl';
import Layout from '@theme/Layout';
import HomepageFeatures from '@site/src/components/HomepageFeatures';
import styles from './index.module.css';

function HomepageHeader() {
  const wordmarkSources = {
    light: useBaseUrl('/img/AIB_Wordmark_Black.svg'),
    dark: useBaseUrl('/img/AIB_Wordmark_White.svg'),
  };
  const previewImage = useBaseUrl('/img/teaser-browser.webp');

  return (
    <header className={clsx('hero', styles.heroBanner)}>
      <div className="container">
        <div className={styles.heroLayout}>
          <div className={styles.heroContent}>
            <span className={styles.eyebrow}>Free & Open Source</span>
            <p className={styles.heroTagline}>Governed delegated access for AI agents</p>
            <p className={styles.heroDescription}>
              The Agentic Identity Broker is a governed trust boundary between your
              users, the AI agents acting for them, and the third-party services
              those agents call. Users consent once per agent and service; the
              broker holds the third-party tokens encrypted and hands agents only
              narrowly-scoped, exchangeable, auditable access.
            </p>
            <div className={styles.buttons}>
              <Link className="button button--primary button--lg" to="/docs/get-started">
                Get started
              </Link>
              <Link className={styles.heroSecondaryAction} to="/docs/introduction">
                Read the introduction <span aria-hidden="true">→</span>
              </Link>
            </div>
          </div>
          <div className={styles.heroPreview}>
            <img
              alt="Consent screen for delegating access to an AI agent"
              className={styles.heroPreviewImage}
              src={previewImage}
            />
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
