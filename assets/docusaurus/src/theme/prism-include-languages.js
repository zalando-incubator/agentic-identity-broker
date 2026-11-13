// Replaces the theme-classic version, which loads prism.additionalLanguages
// onto prism-react-renderer's Prism. redocusaurus provides prismjs as the
// global Prism, so those languages register on prismjs instead.
import siteConfig from '@generated/docusaurus.config';
import PrismJS from 'prismjs';

// Stop prismjs from highlighting the page itself. That rewrites code blocks
// before hydration and moves deep-linked headings.
PrismJS.manual = true;

export default function prismIncludeLanguages(PrismObject) {
  siteConfig.themeConfig.prism.additionalLanguages.forEach((lang) => {
    // eslint-disable-next-line global-require, import/no-dynamic-require
    require(`prismjs/components/prism-${lang}`);
  });
  // Copy the grammars prism-react-renderer lacks, such as bash and its aliases.
  Object.entries(PrismJS.languages).forEach(([name, grammar]) => {
    if (typeof grammar === 'object' && !(name in PrismObject.languages)) {
      PrismObject.languages[name] = grammar;
    }
  });
}
