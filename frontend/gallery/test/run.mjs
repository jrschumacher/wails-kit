// Runs the gallery's browser checks, test/browser.mjs then
// test/components.mjs, each in its own process, and fails if either failed:
// one failing file doesn't stop the other from running.
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

let failed = false;
for (const file of ['browser.mjs', 'components.mjs']) {
  const r = spawnSync(process.execPath, [fileURLToPath(new URL(file, import.meta.url)), ...process.argv.slice(2)], { stdio: 'inherit' });
  if (r.status !== 0) failed = true;
}
process.exit(failed ? 1 : 0);
