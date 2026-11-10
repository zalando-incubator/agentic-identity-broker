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
              Your assistant reads tickets, reviews pull requests, and sends email
              for your people. OAuth lets it connect, but it cannot tell one agent
              from another, ask about one specific email, or take back one agent&apos;s
              access. The broker decides what each agent can do for each person,
              keeps the provider tokens out of the agents, and gives every user one
              place to approve and revoke access.
            </p>
            <div className={styles.buttons}>
              <Link className="button button--primary button--lg" to="/docs/get-started">
                Get started
              </Link>
              <Link className={styles.heroSecondaryAction} to="/docs/introduction/why">
                Why does this exist? <span aria-hidden="true">→</span>
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
