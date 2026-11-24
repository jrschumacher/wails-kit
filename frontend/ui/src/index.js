// @wails-kit/ui: the package's one ES module entry. Importing it defines the
// custom elements (wk-dialog, wk-menu, wk-context-menu, wk-tabs, wk-tooltip,
// wk-settings), puts the window's announcer in the page and makes wk-copy
// buttons copy.
// dist/wails-kit-ui.js is these modules in one file, with the same exports.
export { WkMenu, WkContextMenu, WkTabs, WkTooltip, WkDialog, syncSteppers, setProgress } from './kit.js';
export { timings, motion, reducedMotion } from './motion.js';
export { announce, placeAnnouncer } from './announce.js';
export { wkTask } from './task.js';
export { wkChart, niceTicks, labelStep, runs, roundedTop, roundedEnd, tableModel, readout } from './chart.js';
export { wkDiff, parseUnifiedDiff, diffSplitRows, diffWordRanges, diffCommentPlaces } from './diff.js';
export { SECRET_MASK, conditionMet, resolveOptions, fieldValue, readValue, displayValue, dependentResets, errorsByField, settingsModel } from './settings-model.js';
export { WkSettings } from './settings.js';
export { wkToast } from './toast.js';
export { wkUndo } from './undo.js';
export { wkCopy } from './copy.js';
export { wkSkeleton } from './skeleton.js';
