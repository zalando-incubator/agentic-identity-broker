import type { Plugin } from 'vite';

export function decisionModulesPlugin(): Plugin {
  let root: string;
  return {
    name: 'decision-modules',
    apply: 'build',
    configResolved(config) {
      root = `${config.root.replaceAll('\\', '/')}/`;
    },
    generateBundle(_options, bundle) {
      const modules: Record<string, string[]> = {};
      for (const output of Object.values(bundle)) {
        if (output.type === 'chunk') {
          modules[output.fileName] = Object.entries(output.modules)
            .filter(([, module]) => module.renderedLength > 0)
            .map(([id]) => id.replaceAll('\\', '/').replace(root, ''));
        }
      }
      this.emitFile({
        type: 'asset',
        fileName: '.vite/decision-modules.json',
        source: JSON.stringify(modules),
      });
    },
  };
}
