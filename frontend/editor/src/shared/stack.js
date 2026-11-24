// Stack overflows: the editors' safety nets under the Markdown limits scan
// catch these, and only these.

/** Whether an error is a stack overflow (V8 and WebKit: "Maximum call stack size exceeded"). */
export function isStackOverflow(e) {
  return e instanceof RangeError && /call stack/i.test(String(e.message));
}
