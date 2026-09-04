export function formatToolPattern(
  toolPattern: string,
  paramsPattern: Record<string, string>,
): string {
  const entries = Object.keys(paramsPattern ?? {})
    .sort()
    .map((key) => `${key}=${paramsPattern[key]}`);
  return entries.length === 0 ? toolPattern : `${toolPattern}(${entries.join(',')})`;
}
