# Behaviour in @wails-kit/ui

How the kit's components behave over time: what a person sees between pressing a
button and the work being done, how the app says it worked or didn't, and how things
move. [`THEMING.md`](THEMING.md) says what a screen looks like; this page says what
happens on it. It applies to every app on the kit. The kit's components follow it, or
are listed under [Known gaps](#known-gaps).

Pieces marked *planned* don't exist yet; [Building blocks](#building-blocks)
specifies them. The kit writes British English ("Cancelled", "colour").

## In brief

- Answer every press at once; show an indicator only after 400 ms, and keep it at
  least 500 ms.
- Show feedback where the person is looking: on the control or on the row it changes.
  Never block the whole window for part of it.
- Count real progress in the person's units ("12 of 40 files"); never fake a pace.
- Work that can take over 10 s can be stopped: Cancel puts things back, Stop keeps
  what finished.
- Three tones only. Attention means a person must act; waiting, retrying, queued and
  cancelled are neutral.
- Say what happened and what's next in the person's words; raw errors go in an
  ErrorBox.
- Prefer Undo to confirmation; ask with an Alert only for what can't be undone or
  affects other people.
- Focus, scroll and selection never move because work finished. Announce the start,
  each change of stage and the end, once each.
- Work lives on the Go side: it survives reloads and closed windows, and its outcome
  waits where the person will find it.

## Principles

1. **Answer every action within 100 ms.** A press changes something at once, even if
   the work behind it takes minutes. Silence reads as "it didn't hear me", and people
   press again.
2. **Don't show waiting that isn't there.** Work that finishes quickly shows no
   indicator at all. An indicator that flashes for a moment reads as a glitch.
3. **Show progress only when it's real.** A bar that moves at a made-up pace is worse
   than honest words that name the work ("Fetching runs…"). Count in the units the
   person cares about ("12 of 40 files").
4. **Say what happened in words.** Colour and motion support the words; they never
   carry the outcome alone. The kit has three tones (neutral, working, attention) and
   no success colour. Success is the neutral state: the result itself, or words like
   "Link copied" when it isn't visible.
5. **Keep the person where they were.** Focus, scroll position and selection survive
   the work. Nothing moves, closes up or appears under the pointer or focus because
   work finished.
6. **Let people stop long work.** Anything that can take more than about 10 seconds
   has a way to stop it: **Cancel** when stopping leaves things as they were, **Stop**
   when finished parts stay. If the work can't be stopped safely, say so beside the
   indicator rather than showing a disabled button.

## Before building an action

Answer these for every action:

1. What shows at 0 ms, at 400 ms and at 10 s, and where?
2. Can it be queued? What would it wait for?
3. Is it one task with parts?
4. Can it report progress? In what units?
5. Can it be stopped? Cancel, Stop or Pause? Does stopping ask? What's left after a
   Stop?
6. What does done look like? Is the result visible, or does it need words?
7. What does failure say, and what can the person do next? Is it safe to retry
   automatically?
8. What is announced, and when? Polite or assertive?
9. If the person has moved away, or the window is closed, reloaded or asleep, where
   does the outcome stay?

## The action lifecycle

Every action that does work follows the same states. The kit's task helper
([`wkTask`](#wktask)) runs the clocks, the busy state and the ARIA; the app
writes the words and does the work. The Owner column says which.

| State | When | What the person sees | Owner |
|---|---|---|---|
| **Queued** | it can't start yet | Neutral words, no indicator: "Queued · after 2 others", "Waiting for mini-2 to come back online". Cancel takes it off the queue. | kit shows it; app writes the words |
| **Pressed** | 0 ms | The control keeps its pressed face (`--wk-color-fill-strong`) and the `progress` cursor; a second press does nothing; it keeps its accessible name, width and focus. When the action came from a menu or shortcut, the item it changes dims to secondary text at once instead. The rest of the window stays usable. | kit |
| **Quiet wait** | up to 400 ms | Nothing more. Most work ends here, with no indicator. | kit |
| **Working** | after 400 ms | An indicator where the action happened: a spinner in place of the label of the button that started it, or a working dot and words in the state cell of the thing it changes ("Removing macbookpro…"). | kit; app writes the words |
| **Measured progress** | once the work reports a total | A determinate bar with the count in words ("12 of 40 files"); in a row, the words alone. | kit; app reports progress |
| **Long wait** | after 10 s | Cancel or Stop (see [Stopping](#stopping)); time left once the estimate is stable ("About 3 min left"). | kit |
| **Waiting** | no progress for 30 s | The words say what it's waiting for ("Waiting for GitHub…"). Neutral: it may still finish by itself. | kit starts it; app writes the words |
| **Paused** | on Pause | Neutral words with what's done so far ("Paused · 12 of 40 files"); the button reads Resume. No timeout while paused. | kit; app pauses the work |
| **Stalled** | it can't go on without a person | Attention: what's stuck and what to do ("GitHub sign-in expired · Sign in", "Needs your approval on GitHub · Open"). | app |
| **Done** | on success | The indicator goes and the result shows itself where possible (the new value, the new row). Visible words only when the result isn't visible ("Link copied"); off-screen, the words offer to go there ("Repo added · Show"). The outcome is always announced politely in words, even when it shows itself. | kit; app writes the words |
| **Failed** | on failure, or timeout | What failed and what the person can do: "Couldn't save the file · Retry", "Timed out after 2 min · Retry". The raw error goes in an ErrorBox. The control returns to rest. | kit returns the control and announces; app writes the words and draws the failure |
| **Cancelled** | on Cancel | The state from before. Words only where the person might wonder ("Cancelled"). | kit; app rolls back |
| **Stopped** | on Stop | What finished stays, and the words say what: "Stopped · 12 of 40 files copied". | app |

Queued, Waiting, Paused, Cancelled and Stopped are never errors and never use the
attention tone. Every running task has a timeout on silence, so nothing spins forever.
Queued and paused tasks have no timeout, and a task that waits on something it has
named (a job queued on GitHub) may turn it off. Long tasks also have Stop.

### Starting work versus doing it

An action that starts long work (Run reviewers, Start sync) is two things. **Starting
it** follows the lifecycle on the control, and usually ends within the quiet wait.
**The work** then shows where its results will appear (the job rows), and the control
returns to rest. While that work runs, the control has `aria-disabled="true"` and says
why in a description (`aria-describedby`) that also shows as its tooltip on hover and
focus ("Reviewers are running"). Its menu item is disabled.

### Work in parts

One action that runs several parts (three reviewers, five removals) is one task with a
part per item. Each part shows its own state where it lives. The task is Done only if
every part is. When some parts fail, the parts that finished keep their results, each
failed part shows its failure in place, and the summary counts both: "2 of 3 reviewers
finished · 1 failed". Only the failed parts use the attention tone. Announce the
summary once, when the last part ends.

### Indicators

- **Where:** always in the same place for the same kind of work; see [Where feedback
  appears](#where-feedback-appears).
- **A spinner stays a spinner.** Where measured progress may follow, start with an
  indeterminate bar, and make it determinate once the total is known. Never switch a
  spinner to a bar, and never go back from determinate to indeterminate. A button's
  spinner stays in the button; when the work reports a total, the bar appears where
  the work shows (its panel, sheet or region), not in the button.
- **Progress only moves forward.** Never restart or reverse a bar, and don't reach the
  end until the work is done. A task in stages shows the stage in words ("Step 2 of 3
  · Uploading") and one bar for the stage.
- **A spinner in the button that started the work needs no words beside it.** Its
  words go to screen readers (see [Accessibility](#accessibility)).
- **Name the work, not the mechanism:** "Fetching runs…", not "Loading…" or
  "Authenticating…".

### Timings

| Name | Value | Rule |
|---|---|---|
| Indicator delay | 400 ms | Show no indicator for work that ends sooner; show the result at once. Skeleton rows follow it too. |
| Indicator minimum | 500 ms | Once shown, an indicator stays at least this long; hold the result until then and swap them in one step. |
| Stop delay | 10 s | Work that might be long reserves the space for Cancel or Stop from the start and shows it at 10 s, so nothing moves under the pointer. Work expected to be long shows it from the start. |
| Waiting | 30 s without progress | Switch the words to what it's waiting for. |
| Timeout | 2 min without progress, per running task | The task fails ("Timed out after 2 min · Retry"). A task may set its own, or turn it off while it waits on something named. |
| Time left | estimates within 20% of each other for 5 s | Show "About 3 min left" only then; drop it if they spread further. |
| Loading line | 3 s | Add a line saying what's loading and from where. |
| Re-check | 500 ms after typing stops | A field that shows an error checks again with the back end. |
| Toast | 6 s; 10 s with an action | The timer runs only while the window is focused and pointer and focus are outside the toast. |
| Copied | 2 s | How long "Copied" shows. |
| Progress value | every 5 s at most | How often a bar's `aria-valuenow` and `aria-valuetext` change. |
| Backoff | 2, 4, 8, 16 s, then every 30 s, each ±20% | Between automatic retries; see [Connection and retries](#connection-and-retries). |
| Wake | 2 s | After wake, wait this long, then announce what changed. |

Time asleep doesn't count toward any of these: on wake, every clock starts again from
zero. A component may skip the indicator only when its result appears in place within
the quiet wait (a search field filtering as you type). It may never show an indicator
sooner, or take one away sooner.

### Stopping

- **Cancel** returns to the state from before; **Stop** keeps what finished.
- Ask first with an Alert when stopping loses more than a minute of work, or anything
  that can't be got back ("Stop copying? 12 of 40 files are copied; the rest won't
  be."), or when other people will see the stop (a CI job that fails on GitHub: "Stop
  this job?"). A Stop that asks is labelled **Stop…**. Otherwise stop at once. Where
  the work can resume, offer **Pause** instead of asking.
- If the work can't be stopped safely, the indicator says so: "Applying the update ·
  This can't be stopped".
- **Esc** cancels short work whose result the person is waiting for in that place (a
  check beside a field, a search). It never stops background jobs. In a Sheet, the
  first Esc cancels the Sheet's work and a second closes the Sheet. **⌘.** does the
  same on macOS.

### Where feedback appears

Put feedback where the person is looking: on the control they pressed, or on the thing
it changes.

| The action affects | Busy and progress | Outcome |
|---|---|---|
| One control (Copy, Check token) | In the control | Words beside it, which stay until the person changes something there again. On a control they'll press again, "Copied" or its like shows for 2 s (see [Patterns](#patterns)). A toast only when there's nowhere beside it. |
| One item in a list or table | On that row: a working dot and words in its state cell, in place of the item's own state until the task ends | The row itself. A removed item stays in place, muted ("Removed"), until the view is refreshed; rows never close up under the pointer or focus. A failed row shows the attention dot and words ("Couldn't remove"). In a Table, Retry is a row action; in a List, it's in the inspector and the row's context menu. Several failures: see [Work in parts](#work-in-parts). |
| A whole view (load, refresh) | Skeleton rows in place of the content | The content; an EmptyState or ErrorBox in its place on failure |
| The whole app (connect, sync) | The connection's state in the toolbar ("Reconnecting", with its small dot) | See [Connection and retries](#connection-and-retries) |
| Background work started earlier | A row in the relevant list and in the task list; the taskbar or Dock badge | A system notification if the window isn't focused |

Never block the whole window for work on one part of it. A modal progress sheet is
only for work that makes the rest of the window meaningless until it ends (applying an
update, migrating data); it says whether the work can be stopped.

## Writing the words

Tones follow [THEMING.md](THEMING.md#state-tones): three tones, no warning tone, and
success is neutral. The rule people get wrong: **attention means a person must act.**
A failure the app retries by itself is neutral ("Next try in 8 s"). Work waiting for
something to come back is neutral (Queued). A failure that stays failed until someone
acts is attention ("Can't save to this folder"), and so is a choice the person must
make.

- Name what happened, then what's next: "Couldn't download the update · Next try 2:18
  pm". Never "Something went wrong".
- Use the person's words, not the system's: "Couldn't reach the server", not
  "ECONNREFUSED". The system's words go in the ErrorBox. On the Go side, the person's
  words come from one place: `errors.RegisterMessages`.
- An outcome away from its context names the thing, noun then verb, in at most four
  words: "Link copied", "Comment posted". Beside the thing itself, the verb is enough
  ("Saved").
- Progress counts things: "12 of 40 files", "Step 2 of 3". Percent only when there are
  no countable units.
- A button that starts work keeps its accessible name while busy; the spinner shows in
  place of the label. Change the label only when it would become wrong ("Pause"
  becomes "Resume").
- A label that opens a confirmation ends in an ellipsis: "Delete…", "Stop…".
- Durations: "34 s", "3 min", "3 h 12 m". Elapsed time in a table is `m:ss`. A next
  try under a minute away counts down ("Next try in 8 s"); later ones give the clock
  time ("Next try 2:18 pm"). "As of" carries the day when it isn't today ("As of
  yesterday, 6:02 pm").
- Clock times follow the system's 12- or 24-hour setting; the examples here are
  12-hour.
- Status fragments and outcome words take no full stop ("Link copied"); sentences in
  Notices, EmptyStates, StatusHeadline lines and notifications do.
- No apology, no exclamation marks, no reassurance line ("Don't worry…").

## Undo, confirm or neither

| The action | Do |
|---|---|
| Reversible and cheap (archive, move, rename, toggle) | Do it at once; offer Undo. |
| Reversible but costly to redo (delete a draft, clear a set of filters) | Do it at once with Undo; keep the item recoverable for the session. |
| Irreversible, or affects other people (delete for everyone, revoke access, stop a job other people will see fail) | Ask first with an Alert: the question as the title ("Stop this job?"), the consequence in one line, the safe choice as the default. |
| Destructive and frequent | Prefer Undo over a confirmation people learn to click through. |
| Destructive and in bulk | Ask with the count and a few names: "Remove 5 machines?" ("Remove…" on the button). If some fail or wait, one Notice above the list: "Removed 3 of 5 machines." then "mini-2 and studio are offline. They'll be removed when they're back.", with "Remove now" if the app can force it. |

- **Undo is never only in a toast.** The same Undo is in Edit > Undo (⌘Z / Ctrl+Z),
  named for the action ("Undo Archive"), with Redo beside it on the platform's
  shortcut: a stack for the session (the `shortcuts` package's `UndoStack`, with
  `wkUndo` in the kit). While focus is in a text field, Edit > Undo undoes typing first. A toast
  replaced by a newer one doesn't take its Undo with it. After an undo, show what came
  back, scrolling to it if needed.
- Don't ask "Are you sure?". Ask the specific question and name the thing.
- **Alert default:** the kit's Alert departs from Apple's alert guidance on purpose.
  The safe choice is the default, and it sits last in the stack; Return and Esc both
  choose it. Where the app uses a native alert, follow the platform.

## Forms and validation

- Validate a field when the person leaves it, not on every keystroke. Once a field
  shows an error, re-check it as they type, so the error clears as soon as it's fixed.
- Don't mark an empty required field as an error just because focus left it; only on
  submit.
- A check that needs the back end (a token, a path) runs when the person leaves the
  field or presses Return. Once the field shows an error, it runs again 500 ms after
  they stop typing, not on each keystroke. Its words follow the lifecycle timings and
  sit beside the field ("Checking token…"). It never blocks typing, and the value
  applies only once the check passes.
- If the window closes before the check returns, the check carries on. If it passes,
  the value applies with no message. If it fails, the value isn't applied: the main
  window shows a Notice ("Your GitHub token wasn't saved." · Open Settings) and
  Settings opens on that field with its error. The Notice is attention if the app
  can't work without the value, neutral otherwise.
- On submit, check everything. If anything fails, keep the input, move focus to the
  first field with an error, and when more than one field fails, say so above the
  form: "Fix 2 fields to save."
- An error from the back end goes on its field. An error that isn't about a field goes
  in an inline Notice above the form, never in a toast.
- The submit button stays enabled. A disabled button can't say why; an error on submit
  can.
- Errors say how to fix it: "Enter a port between 1 and 65535", not "Invalid value".
- **Windows that apply on change** (the kit's settings window) have no submit. A
  setting applies when changed, unless it's costly or needs other fields to make
  sense; then the group has an Apply button and says what's pending. An empty required
  setting shows its error when the window opens or when the app needs the value, and
  the window's Notice reads "2 settings need attention."

## Loading content

- **Within the quiet wait:** show nothing; render when the data arrives.
- **Longer:** skeleton rows ([`wkSkeleton`](#skeleton-rows)) in the shape of the real content: the same row
  heights and columns, no text, still (no shimmer). Keep headings, toolbars and
  filters real and usable.
- **Charts** (`wkChart`'s `loading`): a loading chart keeps its frame, and its legend when the
  series are known, and shows a spinner in the plot. There's no skeleton for a chart.
- **After 3 s:** add a line saying what's loading and from where, in secondary text.
- **Empty:** an EmptyState that says what's true now and what would change it, never
  an empty table.
- **Partial:** show what arrived and mark what didn't, in place.
- **Refreshing content already shown:** keep the old content with a small working
  indicator, and replace it in place. Never blank a view to reload it.

## Connection and retries

- The toolbar shows the connection's state in words with its small dot:
  "Reconnecting". The detail and Retry now go in a popover from it, or in the footer
  bar of a window with no toolbar: "Can't reach the server · Next try in 8 s · Retry
  now". Neutral while the app retries by itself.
- Back off with jitter: 2, 4, 8 and 16 s, then every 30 s, each ±20%, so many machines
  don't reconnect at once. A successful connection resets the backoff. Retry now tries
  immediately; if that fails, the backoff carries on from where it was. On wake, and
  whenever the network changes, drop the backoff and try at once.
- Retry by yourself only what's safe to repeat: a read, a sync, an upload with an
  idempotency key. When a write may or may not have happened (a timeout while posting a
  comment or starting a job), don't retry it automatically: say "Couldn't confirm the
  comment was posted", check, then offer Retry.
- Keep the last known data visible, marked as of when ("As of 2:14 pm"). Never show
  stale numbers as live.
- Switch to the attention tone only when a person must act (a password expired, a
  setting is wrong).

## Patterns

- **Several tasks at once:** each task shows its state where it started. When two or
  more run, one summary ("3 tasks running · 1 failed") sits beside the connection state
  in the toolbar, or in the footer bar of a window with no toolbar, and opens the task
  list ([`wkTask.summary` and `wkTask.showList`](README.md#the-summary-and-the-task-list)),
  also reachable from Window > Tasks. A failed task stays in the list until the person
  retries or dismisses it, so a failure never depends on a toast or on finding the
  control that started it. The taskbar shows the combined total, and
  notifications are grouped ("3 jobs finished.").
- **Work across reload, close and reopen:** task state lives on the Go side, not in the
  web view. A reloaded or reopened window asks for the current tasks and draws them as
  they are. Closing a window doesn't cancel its work; where closing the last window
  quits the app, the quit rule applies (see [Desktop](#desktop)).
- **Work on something offline:** work that will run by itself once its target is back
  is Queued, and neutral. It fails, with attention, only when a person must act.
- **Optimistic updates:** only for cheap actions that rarely fail (toggle, rename,
  archive). On failure, roll back in place, keep what the person typed, and mark the
  item: "Couldn't rename · Retry". A toast alone isn't enough.
- **Streaming and partial output:** show output as it arrives. Follow the end only
  while the person is at the end; once they scroll up, stop, and show "New lines below
  · Jump to end". Streamed text is never in a live region.
- **Live lists:** a pending row keeps its place. Updates don't reorder rows or insert
  above the row under the pointer or with focus; hold them and show "3 new · Show".
- **Stale data:** data older than its refresh interval says "As of 2:14 pm". Past twice
  the interval, the numbers drop to secondary text and the view offers Refresh.
- **Copy to clipboard** (`wk-copy`, which the kit wires): the accessible name stays ("Copy
  join code"). "Copied" goes to the polite announcer and shows for 2 s in a tooltip
  above the button. The glyph doesn't change. On failure, "Couldn't copy".
- **Drag and drop:** show the drop target on enter, and say what a drop will do ("Drop
  to add 3 files"). A drop that would be refused says why before release. Every drag
  has a keyboard or menu alternative. Files dropped from the system go through the
  same lifecycle.
- **Keyboard during busy states:** focus stays where it was. A shortcut or menu item
  for an action already running does nothing, and its menu item is disabled. Every
  other control still works.

## Notifications inside the window

| Use | For |
|---|---|
| Words next to the thing | The result of the action the person just took, when there's room |
| Toast ([`wkToast`](#toast)) | A brief, non-blocking outcome with nowhere inline to show it ("Link copied"), and Undo. At the bottom of the window, one at a time. It may carry an action only if the same action exists elsewhere (Undo in Edit > Undo, Show in the list). |
| Notice | Something about the current view that stays true until it changes. One per view: attention beats neutral, then the newest wins, and the rest are counted ("and 2 more problems"). A form's own Notice sits inside the form and doesn't count against the view's. |
| ErrorBox | The raw details of a failure, to read or copy. |
| Alert | A question that must be answered before going on. |

Never put an error a person must act on in a toast: it disappears.

## Motion

- Motion explains a change, where something came from or went. It doesn't decorate,
  and it never blocks input: focus moves and controls work from the first frame.
- Durations (tokens): `--wk-duration-fast` 100 ms for state changes (hover,
  press, a switch's thumb), `--wk-duration-base` 200 ms for things appearing or moving
  (a menu, a toast, a row), `--wk-duration-slow` 300 ms for large surfaces (a sheet).
  Nothing longer than 300 ms on a direct response. Things leaving take about
  two-thirds of the time they took to enter.
- Easing (tokens): `--wk-ease-out` for things entering, `--wk-ease-in` for
  things leaving.
- **Reduced motion:** with `prefers-reduced-motion: reduce`, transitions are instant or
  a fade of no more than 100 ms. Nothing slides, scales or zooms. Small in-place
  indicators keep moving, because a still indicator reads as frozen: a spinner keeps
  turning, and an indeterminate bar's sweep becomes a slow fade in place. No looping
  decoration.
- Nothing pulses, blinks or loops to attract attention. The running dot's halo is the
  kit's only emphasis, and it's still.

## Accessibility

- **Busy controls** stay focusable and keep their accessible name. They get
  `aria-disabled="true"` so a second press does nothing, plus `data-busy` for styling,
  which keeps the pressed face rather than the disabled look; never `disabled`, which
  drops focus. The spinner inside is `aria-hidden="true"`; the work's words go to the
  announcer once, when the indicator appears.
- **A standalone spinner or indeterminate bar** is `role="progressbar"` with an
  accessible name ("Fetching runs") and no value. A determinate bar adds
  `aria-valuemin`, `aria-valuemax`, `aria-valuenow` and `aria-valuetext` in words ("12
  of 40 files"); `aria-valuenow` and `aria-valuetext` change together, at most every
  5 s, while the visible fill may move more often.
- **Tooltips** show on hover after a short delay and at once on keyboard focus, stay
  while the pointer is over them, and close with Esc. Text that matters is also the
  control's description; a tooltip never holds the only copy.
- **Announcements** go through the window's [announcer](#announcer), which is in the
  page from load. Persistent status lines (a StatusHeadline, the connection state) may
  be their own polite regions, and announce only when the state's name changes.
- **During long work,** announce the start once (when the indicator appears), each
  change of stage, and the end. Never put a countdown, a ticking time, a progress
  value or streamed output in a live region; keep a state's words and its countdown in
  separate elements.
- **Focus:** after a submit, focus moves to the first field with an error. An Alert
  takes focus because it's modal. When a Sheet, Alert, popover or toast closes, focus
  returns to what opened it, or to where it was. Focus never moves because work
  finished or failed; the one exception is a removed row that had focus, which hands
  focus to the next row when the view refreshes.
- **Assertive** is only for a failure of work the person is waiting on: they pressed
  it, and focus is still on the control, its row, or the Sheet it started in.
  Background failures, including work they started and left, are polite and say where
  to find them: "Export failed while uploading. Retry is in Tasks."
- **Forced colours:** every state reads without colour or opacity alone. A progress
  bar's fill is `Highlight` inside a `CanvasText` outline; skeleton rows are outlined
  in `GrayText`; a spinner is drawn with a border, not a background.

## Desktop

A desktop app has places to show work that a web page doesn't. The kit's Go
[`desktop`](../../desktop/README.md) package does these, following the Go tasks service;
increased contrast is in the tokens.

- **Window not focused:** long work (over 10 s) that finishes or fails sends one
  system notification, as a sentence ("Export finished.", "Export failed while
  uploading."). It never contains secrets, tokens or private content (a private
  repository's code, a commit message). Clicking it brings the window forward with the
  result selected. Ask for permission the first time a notification would help, not
  at launch; without it, rely on the badge and the in-window words. Respect the
  person's notification settings, and never notify about work they're watching. Using
  a notification for a failure departs from Apple's advice to use an alert: an alert
  in a window that isn't in front goes unseen.
- **Taskbar and Dock:**
  - Windows: progress on the taskbar button in the Normal and Indeterminate states.
    Wails doesn't expose it; the kit calls `ITaskbarList3` itself. Error (red)
    only for a failure that needs the person, cleared when the window is focused; set
    the value to full first, because Error over Indeterminate draws a made-up
    percentage. Never Paused, which draws yellow; show paused work in the window's
    words. The count of things that need the person goes in Wails's dock badge, which
    Windows draws as an overlay icon.
  - macOS: there's no standard Dock progress; it needs a custom Dock tile view, which
    Wails doesn't expose. The badge (Wails's dock service) counts things that need the
    person, and drops as each is dealt with.
  - Linux: no standard equivalent everywhere, and Wails's badge does nothing there.
    Rely on the in-window line and notifications.
- **Quitting while work runs:** ask first ("Quit while 2 jobs are running?") with the
  safe choice as the default, and offer to quit when the work finishes.
- **Sleep and wake:** every clock starts again from zero on wake. Work interrupted by
  sleep resumes if it's safe to repeat; otherwise it says so in place, neutral: "Sync
  interrupted · Resume". Connections try at once on wake. Announce changes once, at
  least 2 s after wake, and only for tasks whose state changed. Wails reports sleep and
  wake only on Windows; elsewhere the kit detects wake from a jump in the clock.
- **Launch:** the app sets each window's background colour (Wails's
  `BackgroundColour`) to the theme's `bg` for the current appearance before the web
  view paints, and updates it when the appearance changes, so a dark launch doesn't
  flash white. A first launch shows an EmptyState, never an Alert. A migration uses
  the modal progress sheet with counted progress, says whether it can be stopped, and
  on failure keeps the old data and offers to copy diagnostics. Windows reopen at
  their last size and position (the kit's `state` package).
- **System appearance:** follow it live, including reduced motion and increased
  contrast, without a restart. Under increased contrast (`prefers-contrast: more`)
  the tokens mix lines, outlines, secondary text and tints from the theme's text and
  bg, so they're stronger in any theme.
- **Sound:** none, except an optional completion sound for very long (over 2 min),
  infrequent, important work, and then the system's.

## Building blocks

They implement the rules above; an app uses these instead of its own. The timings and
motion tokens, the progress bar and spinner (see the
[README](README.md#busy-spinner-and-progress)), [Go task events](#go-task-events)
(the `tasks` package), [`wkTask`](#wktask), the [announcer](#announcer), the task
list (see the [README](README.md#tasks-and-the-announcer)), the [toast](#toast), app
undo and redo, copy feedback and [skeleton rows](#skeleton-rows) (see the
[README](README.md#toast-undo-copy-and-skeleton-rows)) ship; the rest are *planned*.

### wkTask

```js
const task = wkTask({
  control,             // the element that started it: gets aria-disabled and data-busy (optional)
  target,              // the element whose state cell shows the dot and words (optional)
  words: 'Exporting',  // what it's doing; announced when the indicator appears
  expect: 'short',     // 'short' | 'maybe-long' (space kept for Stop) | 'long' (Stop shown at once)
  stop: 'cancel',      // 'cancel' | 'stop' | 'stop-asks' | 'pause' | 'unsafe' | false
  onStop,              // async () => void: the app stops the work
  timeout: 120_000,    // ms without progress before Failed; false while waiting on something named
});
task.progress({ done: 12, total: 40, unit: 'files', step: 2, steps: 3, stage: 'Uploading' });
task.waiting('Waiting for GitHub…');
task.paused(); task.resumed();
task.done({ words: 'Repo added', visible: false, show }); // words are always announced; shown only when visible is false
task.fail({ words: "Couldn't save the file", retry, error });
task.stopped('12 of 40 files copied');                    // task.cancelled() takes no words
wkTask.follow(id, { control, target });                    // follow a task the Go side runs
```

Every state change fires a bubbling `wk-task` event on the target, or else the
control, with `detail: { id, state, ...payload }`.

**The kit owns** the delays, the minimum display time, the busy control state, the
indicator and its ARIA, the space for Stop and when it appears, the waiting and
timeout clocks, announcements, and returning the control to rest. **The app owns** all
words, progress reports, stopping and pausing the work, rolling back a Cancel, retry
policy, and where the result or failure is drawn.

### Go task events

A Go `tasks` service keeps the list of tasks and emits one event, `tasks:changed`, with
a snapshot of every task in the list (running ones, and finished ones not yet
dismissed): `{ id, parent, subject, kind, state, done, total, unit, step, steps,
stage, words, stop, error: { code, userMsg, message }, retry: auto|manual|unsafe,
nextTry, startedAt, updatedAt }`, where `state` is one of queued, running, waiting,
paused, stalled, done, failed, cancelled or stopped. See
[`tasks/README.md`](../../tasks/README.md) for the field meanings and the Go API.

- One event name keeps `Emitter.ReplayAll` right for a reloaded window: with
  `WithHistory`, it sends the latest event of each name still in the history's ring
  buffer, and one frequent name stays in it.
- Progress is coalesced to at most four snapshots a second; changes of state go out at
  once. Don't use `events.WithThrottle` for it: it keeps the first event in each window
  and drops the rest, including a final Done or Failed.
- `error.userMsg` comes from `errors.RegisterMessages`.

### Timings and motion tokens

The package exports `timings = Object.freeze({ indicatorDelay: 400, indicatorMin: 500,
stopDelay: 10_000, waiting: 30_000, timeout: 120_000, estimateStable: 5_000,
loadingLine: 3_000, recheckDelay: 500, toast: 6_000, toastWithAction: 10_000, copied:
2_000, progressValueEvery: 5_000, backoff: [2_000, 4_000, 8_000, 16_000, 30_000],
backoffJitter: 0.2, wakeSettle: 2_000 })` from `src/motion.js`, with `motion` (the
same values as the tokens) and `reducedMotion()`. `tokens.css` sets `--wk-duration-fast`,
`--wk-duration-base`, `--wk-duration-slow`, `--wk-ease-in` and `--wk-ease-out`.

### Progress bar and spinner

- **Bar:** `<div class="wk-progress" role="progressbar" aria-label="Copying files">`;
  determinate adds `aria-valuemin="0" aria-valuemax="40" aria-valuenow="12"
  aria-valuetext="12 of 40 files"`, and indeterminate has no value attributes. 4 px
  tall, a `--wk-color-fill-strong` track and a `--wk-color-working` fill. Bars go in
  panels, sheets and standalone regions; in a table or list row, progress is words
  beside the dot, with no bar. On failure the bar goes and the words take its place.
  `setProgress(bar, { done, total, unit })` moves the fill on every call and changes
  the value attributes together, at most every 5 s (at once on the first call, when
  done, and when the total or unit changes; new words alone wait for the next update;
  `{ aria: 'now' }` forces it). It sets no timer, so `wkTask` passes `aria: 'now'` on
  waiting, pause and each change of stage, and the attributes never stay behind the
  fill while the work stands still.
- **Spinner:** in a button, `<span class="wk-spinner" aria-hidden="true">`; on its own,
  `<span class="wk-spinner" role="progressbar" aria-label="Fetching runs">`. 12 px in a
  small button and 14 px otherwise, drawn with a border in `currentColor`. In a button
  it takes the label's place, and the label keeps its place so the width holds; the
  accessible name doesn't change. (The label's text is painted transparent: `visibility:
  hidden` would drop it from the accessible name.)

### Toast

`wkToast({ words, action: { label, run } })`: one element at the bottom centre of the
window, above any footer bar, outside the tab order until it appears, and it never
takes focus. F6 moves focus to its action and back, and Esc dismisses it while focus
is inside; focus then returns to where it was. It isn't a live region: its words go
through the polite announcer. A newer toast replaces it. `wkUndo.action(id)` gives it
the Undo of the `shortcuts` package's stack, the same one Edit > Undo uses.

### Announcer

`announce(words, { assertive = false })`: two visually hidden regions,
`aria-live="polite"` and `aria-live="assertive"`, both `aria-atomic="true"`, in the
page from load (a region added together with its text often goes unread). Messages
within 500 ms of each other are joined into one. Repeating the same words clears the
region and sets it again on the next frame. Each window has its own.

### Skeleton rows

`<tr class="wk-skeleton" aria-hidden="true">` with one cell per real column, each
holding a still bar in `--wk-color-fill`, at the real row height. As many rows as were
there last time, or enough to fill the region, up to 12. The region being replaced has
`aria-busy="true"` until the content arrives. They appear at 400 ms, like any
indicator.

### Also planned

- The Sheet card notes that while it holds work, the first Esc cancels the work (the
  `<dialog>`'s `cancel` event is prevented) and a second closes it.

## Known gaps

Where shipped code doesn't yet follow this page:

- `wkTask` doesn't yet show time left ("About 3 min left") once the estimates are
  stable; the app can put it beside the indicator itself.
- Escape in a diff review comment box discards its draft with no Undo.
- The design system's Button card documents `disabled` and the locked state, but no
  busy state; the kit's README documents it.
- On Windows, Wails's question dialog offers only Yes and No, so the `desktop`
  package's own quit question there has no Quit When Done; an app that wants it sets
  its own `Asker` (a sheet in the window).
- Without `shortcuts.WithUndo`, Edit > Undo is the native role, which undoes text
  editing only; an app that keeps its own undo uses `UndoStack`.
- With Wails v3.0.0-alpha.64, app undo has two platform limits:
  - **Linux:** menu accelerators don't reach the GTK menu, so Edit > Undo and Redo run
    only when clicked.
  - **Windows:** only the window that built the menu last shows the Edit menu's
    updated labels and enabled state.
- On Windows and Linux, clicking the menu bar may take focus from the web view first,
  so Edit > Undo clicked with the mouse may undo the app's action instead of the
  typing. The shortcut isn't affected. This hasn't been checked on real hardware.
- Skeleton rows are for tables (`<tbody>`); a list (`wk-list`) has none yet. Neither
  they nor the loading chart add the line saying what's loading after 3 s; the app
  adds it.
