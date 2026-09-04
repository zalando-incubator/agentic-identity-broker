/**
 * Tests for approval glob helpers.
 */

import { describe, it, expect } from 'vitest';
import {
  canonicalArgumentValue,
  escapeGlobLiteral,
  formatToolPattern,
  humanizeParameterKey,
  matchesGlob,
  unescapeGlobLiteral,
  validateParamGlob,
  validateToolGlob,
} from './toolPattern';

describe('escapeGlobLiteral / unescapeGlobLiteral', () => {
  it('round-trips values containing wildcards and backslashes', () => {
    for (const value of ['acme/app', 'a*b', 'C:\\temp\\*', '\\', '*', '']) {
      expect(unescapeGlobLiteral(escapeGlobLiteral(value))).toBe(value);
    }
  });

  it('escapes so the pattern matches only the original value', () => {
    expect(matchesGlob(escapeGlobLiteral('a*b'), 'a*b')).toBe(true);
    expect(matchesGlob(escapeGlobLiteral('a*b'), 'axb')).toBe(false);
  });

  it('keeps a trailing lone backslash when unescaping', () => {
    expect(unescapeGlobLiteral('acme\\')).toBe('acme\\');
  });
});

describe('matchesGlob', () => {
  it('treats * as any run of characters including empty', () => {
    expect(matchesGlob('acme/*', 'acme/app')).toBe(true);
    expect(matchesGlob('acme/*', 'acme/')).toBe(true);
    expect(matchesGlob('acme/*', 'other/app')).toBe(false);
  });

  it('treats a backslash as escaping the next character', () => {
    expect(matchesGlob('a\\*b', 'a*b')).toBe(true);
    expect(matchesGlob('a\\*b', 'axb')).toBe(false);
  });

  it('matches literal patterns exactly', () => {
    expect(matchesGlob('create_pull_request', 'create_pull_request')).toBe(true);
    expect(matchesGlob('create_pull_request', 'create_pull_requests')).toBe(false);
  });
});

describe('canonicalArgumentValue', () => {
  it('renders scalars the way the server matches them', () => {
    expect(canonicalArgumentValue('acme/app')).toBe('acme/app');
    expect(canonicalArgumentValue(3)).toBe('3');
    expect(canonicalArgumentValue(true)).toBe('true');
    expect(canonicalArgumentValue(false)).toBe('false');
    expect(canonicalArgumentValue(null)).toBe('null');
    expect(canonicalArgumentValue(undefined)).toBe('null');
    expect(canonicalArgumentValue(Number.NaN)).toBe('null');
  });

  it('renders arrays and objects as compact JSON with sorted keys', () => {
    expect(canonicalArgumentValue(['b', 'a'])).toBe('["b","a"]');
    expect(canonicalArgumentValue({ b: 2, a: 1 })).toBe('{"a":1,"b":2}');
    expect(canonicalArgumentValue({ z: { y: 1, x: [1, { b: 2, a: 1 }] } })).toBe(
      '{"z":{"x":[1,{"a":1,"b":2}],"y":1}}',
    );
  });
});

describe('humanizeParameterKey', () => {
  it('derives readable labels from raw keys', () => {
    expect(humanizeParameterKey('create_pull_request')).toBe('Create pull request');
    expect(humanizeParameterKey('pullRequestTitle')).toBe('Pull request title');
    expect(humanizeParameterKey('repo')).toBe('Repo');
  });

  it('keeps acronyms and falls back to the raw key', () => {
    expect(humanizeParameterKey('API_KEY')).toBe('API KEY');
    expect(humanizeParameterKey('___')).toBe('___');
  });
});

describe('validateToolGlob', () => {
  it('accepts legal tool globs', () => {
    expect(validateToolGlob('create_*')).toBeNull();
    expect(validateToolGlob('svc:tools/read-file.v2')).toBeNull();
    expect(validateToolGlob('a\\*b')).toBeNull();
  });

  it('rejects empty, over-long, trailing-escape, and illegal-character globs', () => {
    expect(validateToolGlob('')).toBe('Use at most 255 characters.');
    expect(validateToolGlob('a'.repeat(256))).toBe('Use at most 255 characters.');
    expect(validateToolGlob('create_\\')).toBe('Remove the trailing \\ character.');
    expect(validateToolGlob('create pull')).toBe(
      'Use only letters, numbers, and _ . : / - characters.',
    );
  });
});

describe('validateParamGlob', () => {
  it('accepts legal constraints including escaped trailing backslashes', () => {
    expect(validateParamGlob('')).toBeNull();
    expect(validateParamGlob('acme/*')).toBeNull();
    expect(validateParamGlob('acme\\\\')).toBeNull();
  });

  it('rejects over-long values and unescaped trailing backslashes', () => {
    expect(validateParamGlob('a'.repeat(1025))).toBe('Use at most 1024 characters.');
    expect(validateParamGlob('acme\\')).toBe('Remove the trailing \\ character.');
    expect(validateParamGlob('acme\\\\\\')).toBe('Remove the trailing \\ character.');
  });
});

describe('formatToolPattern', () => {
  it('renders sorted parameter constraints', () => {
    expect(formatToolPattern('create_pull_request', { title: 'Fix bug', repo: 'acme/*' })).toBe(
      'create_pull_request(repo=acme/*,title=Fix bug)',
    );
    expect(formatToolPattern('create_pull_request', {})).toBe('create_pull_request');
  });
});
