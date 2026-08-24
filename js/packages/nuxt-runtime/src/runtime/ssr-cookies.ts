export const PLATFORM_SESSION_COOKIE_PREFIX = "ys_";

export function selectSsrCookies(
  inbound: Readonly<Record<string, string>>,
  names: readonly string[],
  prefixes: readonly string[],
): string {
  const selected: string[] = [];
  const seen = new Set<string>();
  const append = (name: string) => {
    const value = inbound[name];
    if (value === undefined || seen.has(name)) {
      return;
    }
    seen.add(name);
    selected.push(`${name}=${encodeURIComponent(value)}`);
  };

  for (const name of names) {
    append(name);
  }
  for (const name of Object.keys(inbound)) {
    if (prefixes.some((prefix) => name.startsWith(prefix))) {
      append(name);
    }
  }
  return selected.join("; ");
}
