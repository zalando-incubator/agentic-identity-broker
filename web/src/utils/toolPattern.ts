export function formatToolPattern(
  toolPattern: string,
  paramsPattern: Record<string, string>,
): string {
  const entries = Object.keys(paramsPattern ?? {})
    .sort()
    .map((key) => `${key}=${paramsPattern[key]}`);
  return entries.length === 0 ? toolPattern : `${toolPattern}(${entries.join(',')})`;
}

// Advisory mirror of internal/toolpattern for inline feedback only; the server re-validates
// every pattern and stays authoritative (ADR 035).

export function escapeGlobLiteral(value: string): string {
  return value.replace(/[\\*]/g, (character) => `\\${character}`);
}

export function unescapeGlobLiteral(pattern: string): string {
  let result = '';
  for (let index = 0; index < pattern.length; index++) {
    if (pattern[index] === '\\' && index + 1 < pattern.length) {
      result += pattern[index + 1];
      index++;
      continue;
    }
    result += pattern[index];
  }
  return result;
}

export function matchesGlob(pattern: string, value: string): boolean {
  let patternIndex = 0;
  let valueIndex = 0;
  let star = -1;
  let retryIndex = 0;

  while (valueIndex < value.length) {
    if (patternIndex < pattern.length) {
      const character = pattern[patternIndex];
      if (character === '*') {
        star = patternIndex;
        patternIndex++;
        retryIndex = valueIndex;
        continue;
      }
      if (character === '\\') {
        if (patternIndex + 1 < pattern.length && pattern[patternIndex + 1] === value[valueIndex]) {
          patternIndex += 2;
          valueIndex++;
          continue;
        }
      } else if (character === value[valueIndex]) {
        patternIndex++;
        valueIndex++;
        continue;
      }
    }
    if (star < 0) return false;
    patternIndex = star + 1;
    retryIndex++;
    valueIndex = retryIndex;
  }

  while (patternIndex < pattern.length && pattern[patternIndex] === '*') patternIndex++;
  return patternIndex === pattern.length;
}

function canonicalJson(value: unknown): string {
  if (value === null || value === undefined) return 'null';
  if (Array.isArray(value)) return `[${value.map(canonicalJson).join(',')}]`;
  if (typeof value === 'object') {
    const record = value as Record<string, unknown>;
    const entries = Object.keys(record)
      .sort()
      .map((key) => `${JSON.stringify(key)}:${canonicalJson(record[key])}`);
    return `{${entries.join(',')}}`;
  }
  if (typeof value === 'number') return Number.isFinite(value) ? String(value) : 'null';
  return JSON.stringify(value) ?? 'null';
}

export function canonicalArgumentValue(value: unknown): string {
  if (typeof value === 'string') return value;
  if (typeof value === 'number') return Number.isFinite(value) ? String(value) : 'null';
  if (typeof value === 'boolean') return value ? 'true' : 'false';
  if (value === null || value === undefined) return 'null';
  return canonicalJson(value);
}

export function humanizeParameterKey(key: string): string {
  const spaced = key
    .replace(/[_\-.]+/g, ' ')
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/\s+/g, ' ')
    .trim();
  if (spaced === '') return key;

  const words = spaced
    .split(' ')
    .map((word) => (word.length > 1 && word === word.toUpperCase() ? word : word.toLowerCase()));
  words[0] = words[0].charAt(0).toUpperCase() + words[0].slice(1);
  return words.join(' ');
}

export function validateToolGlob(pattern: string): string | null {
  if (pattern === '' || pattern.length > 255) return 'Use at most 255 characters.';
  for (let index = 0; index < pattern.length; index++) {
    const character = pattern[index];
    if (character === '\\') {
      if (index + 1 === pattern.length) return 'Remove the trailing \\ character.';
      index++;
      continue;
    }
    if (character === '*' || /[A-Za-z0-9_.:/-]/.test(character)) continue;
    return 'Use only letters, numbers, and _ . : / - characters.';
  }
  return null;
}

export function validateParamGlob(pattern: string): string | null {
  if (pattern.length > 1024) return 'Use at most 1024 characters.';
  let trailingBackslashes = 0;
  for (let index = pattern.length - 1; index >= 0 && pattern[index] === '\\'; index--) {
    trailingBackslashes++;
  }
  if (trailingBackslashes % 2 === 1) return 'Remove the trailing \\ character.';
  return null;
}
