<script lang="ts">
  import { Button, Card, Chip, Notice, Spinner, TextInput, ThemeToggle, Toggle, getHighContrast, setHighContrast, showFlash } from "@kenn-io/kit-ui";
  import { onMount } from "svelte";
  import ManagerGate from "../../components/ManagerGate.svelte";
  import ScreenPicker from "../../components/ScreenPicker.svelte";
  import ReviewChanges, { type Preview } from "../../components/ReviewChanges.svelte";
  import { api, message, type Schemas } from "../../lib/api";
  import { onChange } from "../../lib/live.svelte";
  import { can } from "../../lib/can.svelte";
  import { session, unpairDevice } from "../../lib/session.svelte";

  let system = $state<Schemas["VersionedSystem"] | null>(null);
  let error = $state("");

  let name = $state("");
  let interval = $state("");
  let keep = $state("");
  let upload = $state("");
  let previewN = $state("");
  let voiceDays = $state("");

  let review = $state<{ preview: Preview; value: Schemas["System"] } | null>(null);
  let saving = $state(false);
  let highContrast = $state(getHighContrast());

  const canEdit = $derived(can("settings.manage"));

  // Starting with the Kishtin computer.
  let startup = $state<Schemas["StartupStatus"] | null>(null);
  let startupBusy = $state(false);
  let startupError = $state("");

  async function loadStartup() {
    try {
      startup = await api.startup();
      startupError = "";
    } catch (err) {
      startupError = message(err);
    }
  }

  // Permissions arrive after the page opens.
  let startupAsked = false;
  $effect(() => {
    if (canEdit && !startupAsked) {
      startupAsked = true;
      void loadStartup();
    }
  });

  async function setStartup(on: boolean) {
    startupBusy = true;
    try {
      startup = await api.setStartup(on);
      showFlash(on ? "Kishtin now starts with this computer" : "Kishtin no longer starts by itself", { tone: "success" });
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 10000 });
      await loadStartup();
    } finally {
      startupBusy = false;
    }
  }

  function reset() {
    if (!system) return;
    const v = system.value;
    name = v.restaurant_name;
    interval = String(v.backup_interval_minutes);
    keep = String(v.backup_keep);
    upload = String(v.max_upload_mb);
    previewN = String(v.preview_records);
    voiceDays = String(v.voice_audio_days);
  }

  async function load() {
    try {
      system = await api.system();
      reset();
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  onMount(() => {
    void load();
    return onChange((kind) => {
      if (kind === "settings") void load();
    });
  });

  const whole = (s: string) => (/^\d+$/.test(s.trim()) ? Number(s) : null);
  const draft = $derived.by((): Schemas["System"] | string => {
    const i = whole(interval);
    const k = whole(keep);
    const u = whole(upload);
    const p = whole(previewN);
    const vd = whole(voiceDays);
    if (i === null || k === null || u === null || p === null || vd === null) return "Every number must be a whole number.";
    return { restaurant_name: name.trim(), backup_interval_minutes: i, backup_keep: k, max_upload_mb: u, preview_records: p, voice_audio_days: vd };
  });
  const dirty = $derived(system !== null && typeof draft !== "string" && JSON.stringify(draft) !== JSON.stringify(system.value));

  async function reviewChanges() {
    if (typeof draft === "string") return;
    try {
      review = { preview: await api.previewSystem({ value: draft, reason: "" }), value: draft };
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 6000 });
    }
  }

  async function save(reason: string) {
    if (!review) return;
    saving = true;
    try {
      await api.saveSystem({ value: review.value, reason });
      review = null;
      showFlash("Saved", { tone: "success" });
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      saving = false;
    }
  }
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>Settings</h1>
      <p>Settings for the whole restaurant, and for this device only.</p>
    </div>
  </header>

  <div class="grid">
    <div class="stack">
      <h2 class="section-title">Restaurant</h2>
      <ManagerGate what="Restaurant settings" action="settings.manage" viewable>
        {#if error}<Notice tone="error" message={error} />{/if}
        {#if !system}
          {#if !error}<Spinner />{/if}
        {:else}
          <Card padding="md" meta={system.builtin ? "Defaults from kishtin.toml" : `Version saved for: ${system.version.reason}`}>
            <div class="form">
              <label class="field wide">
                <span>Restaurant name: shown on every screen and on the inspection pack</span>
                <TextInput bind:value={name} disabled={!canEdit} block />
              </label>
              <label class="field">
                <span>Back up every (minutes; 0 turns it off, otherwise at least 15)</span>
                <TextInput bind:value={interval} disabled={!canEdit} />
              </label>
              <label class="field">
                <span>Backups to keep (0 keeps every one)</span>
                <TextInput bind:value={keep} disabled={!canEdit} />
              </label>
              <label class="field">
                <span>Largest docket upload (MB, 1–100)</span>
                <TextInput bind:value={upload} disabled={!canEdit} />
              </label>
              <label class="field">
                <span>Records a limit preview re-scores (1–500)</span>
                <TextInput bind:value={previewN} disabled={!canEdit} />
              </label>
              <label class="field">
                <span>Keep recordings of voice commands (days, 0–365; 0 keeps none)</span>
                <TextInput bind:value={voiceDays} disabled={!canEdit} />
              </label>
            </div>
            {#if typeof draft === "string"}<Notice tone="error" message={draft} />{/if}
            {#if canEdit && dirty}
              <div class="row actions">
                <Button label="Undo changes" onclick={reset} />
                <Button tone="info" surface="solid" label="Review changes" onclick={reviewChanges} />
              </div>
            {/if}
          </Card>
          <p class="muted small">The backup folder, listen address and TLS are set in <code>kishtin.toml</code> on the server.</p>
        {/if}
      </ManagerGate>

      <h2 class="section-title">The Kishtin computer</h2>
      <ManagerGate what="Starting with the computer" action="settings.manage">
        {#if startup}
          <Card padding="md">
            <div class="stack">
              <div class="row">
                {#if !startup.supported}
                  <Chip tone="neutral" uppercase={false}>Can't change here</Chip>
                {:else if startup.on}
                  <Chip tone="success" uppercase={false}>Starts by itself</Chip>
                {:else}
                  <Chip tone="warning" uppercase={false}>Started by hand</Chip>
                {/if}
              </div>
              {#if startup.supported}
                <Toggle checked={startup.on} label="Start Kishtin when this computer starts" disabled={startupBusy} onchange={(on) => void setStartup(on)} />
              {/if}
              <span>
                {#if startup.on}{startup.note}{:else if startup.supported}Off: after the computer restarts, someone has to start Kishtin again by typing <code>kishtind</code>. Turning this on uses the computer's own start-up settings ({startup.method}).{:else}{startup.note}{/if}
              </span>
              {#if startup.on && startup.where}<span class="muted small">{startup.method}: <code>{startup.where}</code></span>{/if}
              <span class="muted small">Turning it off leaves Kishtin running until it is stopped or the computer restarts.</span>
            </div>
          </Card>
        {:else if startupError}
          <Notice tone="error" message={startupError} />
        {:else}
          <Spinner />
        {/if}
      </ManagerGate>
    </div>

    <div class="stack">
      <h2 class="section-title">This device</h2>
      <Card padding="md">
        <div class="stack">
          <ScreenPicker />
          <div class="field">
            <span>Theme</span>
            <ThemeToggle variant="segmented" block />
          </div>
          <Toggle
            checked={highContrast}
            label="High contrast"
            onchange={(on) => {
              highContrast = on;
              setHighContrast(on);
            }}
          />
          <span class="muted">Logged in as {session.user?.name}{session.method === "password" ? " with a password" : " with a PIN"}.</span>
          <div>
            <Button tone="danger" label="Unpair this device" onclick={unpairDevice} />
          </div>
          <span class="muted small">Unpairing removes the device key from this browser. A manager needs to pair it again before anyone can log in here.</span>
        </div>
      </Card>
    </div>
  </div>
</div>

{#if review}
  <ReviewChanges title="Review settings change" preview={review.preview} busy={saving} onsave={(r) => save(r)} onclose={() => (review = null)} />
{/if}

<style>
  .grid {
    display: grid;
    grid-template-columns: minmax(0, 3fr) minmax(280px, 2fr);
    gap: var(--space-7);
    align-items: start;
  }
  .form {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: var(--space-5);
  }
  .wide {
    grid-column: 1 / -1;
  }
  .actions {
    justify-content: flex-end;
    margin-top: var(--space-5);
  }
  .section-title {
    margin: 0;
  }
  .small {
    font-size: var(--font-size-sm);
  }
  code {
    font-family: var(--font-mono);
  }
  p {
    margin: 0;
  }
  @media (max-width: 900px) {
    .grid,
    .form {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
