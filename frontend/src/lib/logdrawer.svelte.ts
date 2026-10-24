// The Log check drawer, opened from the top bar, Today and Records.

import type { LogPreset } from "../components/LogForm.svelte";

export const logDrawer = $state({ open: false, preset: null as LogPreset | null });

/** openLog opens the drawer, optionally starting from a preset. */
export function openLog(preset: LogPreset | null = null): void {
  logDrawer.preset = preset;
  logDrawer.open = true;
}

export function closeLog(): void {
  logDrawer.open = false;
  logDrawer.preset = null;
}
