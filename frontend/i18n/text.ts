/**
 * Text mirrors i18n.Text (i18n/i18n.go): a stable catalog key paired with
 * the built-in (English/source-language) fallback text used when no
 * catalog entry resolves the key. Declare it inline at the call site, the
 * same way Go code does:
 *
 *   const greeting = T("myapp.example.greeting", "Hello, %s!");
 */
export interface Text {
  key: string;
  other: string;
}

/** Sugar for constructing a Text literal — mirrors i18n.T in Go. */
export function T(key: string, other: string): Text {
  return { key, other };
}

/**
 * format is a minimal, dependency-free stand-in for Go's fmt.Sprintf,
 * covering the verbs the kit's own catalog strings actually use: %s
 * (string), %d (integer), %f (float), %v (default — String(arg)), and %%
 * (literal percent). Unknown verbs are left as-is rather than throwing —
 * catalog text is data, not code, and a typo in a translation should
 * degrade visibly, not crash the UI.
 */
export function format(template: string, args: readonly unknown[]): string {
  if (args.length === 0) {
    return template;
  }
  let i = 0;
  return template.replace(/%[sdfv%]/g, (verb) => {
    if (verb === "%%") {
      return "%";
    }
    const arg = args[i++];
    switch (verb) {
      case "%d":
        return String(Math.trunc(Number(arg)));
      case "%f":
        return String(Number(arg));
      case "%s":
      case "%v":
      default:
        return String(arg);
    }
  });
}
