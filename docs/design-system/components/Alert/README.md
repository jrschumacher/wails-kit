# Alert

A small centred question before something that cannot be undone: the app icon, a title that asks, one or two lines of consequence, and stacked buttons with the safe choice as the default.

## When to use

Before stopping a job ("Stop this job?"), taking a machine off duty, revoking, leaving HQ ("Leave HQ?"), quitting while jobs run ("Quit while jobs are running?"). Where the platform draws a native alert, use it; this is the same layout in the webview.

## What you supply

A `<dialog class="wk-dialog wk-dialog--alert" role="alertdialog" aria-labelledby aria-describedby>`:
- the app icon `<img src="assets/Logos/appicon.png" alt="">` (54px; fixed colours, the same in both themes);
- a `wk-dialog-title` that is the question, ending in "?";
- one `wk-dialog-body` saying what happens, naming the job (`<b>prune</b> · test`) and what the person can do after ("You can re-run it there.");
- `wk-dialog-actions` with the destructive or alternative choices first and the safe default last as the primary ("Keep running", "Stay"), `autofocus` on it. Return and Esc both choose the safe default.

## Classes and attributes

| Class / attribute | Effect |
|---|---|
| `wk-dialog--alert` | 280px, centred text, 18px × `wk-space-4` padding, `wk-radius-window`. |
| `> img`, `> svg` | 54px icon, centred. |
| `> .wk-dialog-title` | `wk-font-size-base` semibold. |
| `> .wk-dialog-body` | `wk-font-size-sm`. |
| `> .wk-dialog-actions` | Full-width buttons stacked, 6px apart. |

## Do and don't

- Do make the safe choice the default, even when the person probably wants the other.
- Do say what fails and where ("will fail on GitHub"); never "killed" or "terminated".
- Don't add a "Don't ask again" box.
- Don't use an Alert for information with one OK button; show it in the window.

## Replaces

Design page: `.nalert`, `.nalert svg.ai`, `.nalert h4`, `.nalert p`, `.nalert .acts`, `.nb.ring`. App: `.confirm-dialog` (`#leave-dialog`, `#quit-dialog` and its column rule).
