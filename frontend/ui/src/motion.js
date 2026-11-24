// The action lifecycle's timings and the motion values, for scripts
// (BEHAVIOUR.md, "Timings and motion tokens"). No DOM at import, so the task
// helper, toasts and tests can import it anywhere. Every value is in
// milliseconds unless named otherwise.
//
//   timings          the lifecycle's delays, minimums and clocks
//   motion           the --wk-duration-* and --wk-ease-* tokens (tokens.css
//                    sets the same values; test/motion.test.mjs checks)
//   reducedMotion()  true while the system asks for reduced motion

const timings = Object.freeze({
  indicatorDelay: 400,      // show no indicator for work that ends sooner
  indicatorMin: 500,        // once shown, an indicator stays at least this long
  stopDelay: 10_000,        // Cancel or Stop shows for work still running
  waiting: 30_000,          // without progress: the words say what it waits for
  timeout: 120_000,         // without progress, per running task: it fails
  estimateStable: 5_000,    // time-left estimates within 20% for this long
  loadingLine: 3_000,       // add a line saying what is loading and from where
  recheckDelay: 500,        // after typing stops, a field with an error checks again
  toast: 6_000,
  toastWithAction: 10_000,
  copied: 2_000,            // how long "Copied" shows
  progressValueEvery: 5_000, // a bar's aria-valuenow and aria-valuetext change at most this often
  backoff: Object.freeze([2_000, 4_000, 8_000, 16_000, 30_000]), // between automatic retries; the last repeats
  backoffJitter: 0.2,       // each backoff step ±20%
  wakeSettle: 2_000,        // after wake, wait this long, then announce what changed
});

const motion = Object.freeze({
  durationFast: 100,        // state changes: hover, press, a switch's thumb
  durationBase: 200,        // things appearing or moving: a menu, a toast, a row
  durationSlow: 300,        // large surfaces: a sheet
  easeOut: 'cubic-bezier(.2, 0, 0, 1)', // things entering
  easeIn: 'cubic-bezier(.4, 0, 1, 1)',  // things leaving
});

// With reduced motion, transitions are instant or a fade of 100 ms at most,
// and nothing slides, scales or zooms; small in-place indicators keep moving.
const reducedMotion = () => typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;

export { timings, motion, reducedMotion };
