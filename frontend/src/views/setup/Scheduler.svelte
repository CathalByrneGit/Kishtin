<script lang="ts">
  import { Button, Card, Notice, SegmentedControl, Spinner, TextInput, Toggle, showFlash } from "@kenn-io/kit-ui";
  import { onMount } from "svelte";
  import ManagerGate from "../../components/ManagerGate.svelte";
  import ReviewChanges, { type Preview } from "../../components/ReviewChanges.svelte";
  import { api, message, type Schemas } from "../../lib/api";
  import { onChange } from "../../lib/live.svelte";
  import { isManagerPassword } from "../../lib/session.svelte";

  type S = Schemas["Settings"];

  let current = $state<Schemas["VersionedSettings"] | null>(null);
  let stations = $state<Schemas["Station"][]>([]);
  let error = $state("");

  let strategy = $state("backward");
  let batching = $state(true);
  let windowText = $state("");
  let passWaitText = $state("");
  let leadText = $state("");
  let allergy = $state(true);
  let takeaway = $state(false);
  let largeText = $state("");
  let replanText = $state("");
  let buffers = $state<Record<string, string>>({});

  let review = $state<{ preview: Preview; value: S } | null>(null);
  let saving = $state(false);
  const canEdit = $derived(isManagerPassword());

  function reset() {
    if (!current) return;
    const v = current.value;
    strategy = v.strategy;
    batching = v.batching;
    windowText = String(v.batch_window_seconds);
    passWaitText = String(v.max_pass_wait_seconds);
    leadText = String(v.priority_lead_seconds);
    allergy = v.priority_allergy;
    takeaway = v.priority_takeaway;
    largeText = String(v.large_table_covers);
    replanText = String(v.replan_seconds);
    buffers = Object.fromEntries(v.station_buffers.map((b) => [b.station_id, String(b.seconds)]));
  }

  async function load() {
    try {
      const [s, m] = await Promise.all([api.scheduler(), api.menu()]);
      current = s;
      stations = m.stations.filter((x) => x.active);
      reset();
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  onMount(() => {
    void load();
    return onChange((k) => {
      if (k === "settings" || k === "menu") void load();
    });
  });

  const whole = (t: string) => (/^\d+$/.test(t.trim()) ? Number(t) : null);
  const draft = $derived.by((): S | string => {
    const nums = { window: whole(windowText), wait: whole(passWaitText), lead: whole(leadText), large: whole(largeText), replan: whole(replanText) };
    if (Object.values(nums).some((n) => n === null)) return "Every number must be a whole number.";
    const station_buffers: Schemas["StationBuffer"][] = [];
    for (const [id, t] of Object.entries(buffers)) {
      if (t.trim() === "" || t.trim() === "0") continue;
      const n = whole(t);
      if (n === null) return "Station buffers must be whole seconds.";
      station_buffers.push({ station_id: id, seconds: n });
    }
    return {
      strategy, batching, batch_window_seconds: nums.window!, max_pass_wait_seconds: nums.wait!, priority_lead_seconds: nums.lead!,
      priority_allergy: allergy, priority_takeaway: takeaway, large_table_covers: nums.large!, replan_seconds: nums.replan!, station_buffers,
    };
  });
  /** same compares settings whatever order their fields and buffers are in. */
  function same(a: S, b: S): boolean {
    const norm = (v: S) =>
      JSON.stringify(
        Object.entries({ ...v, station_buffers: v.station_buffers.filter((x) => x.seconds > 0).map((x) => `${x.station_id}=${x.seconds}`).sort() }).sort(([x], [y]) => x.localeCompare(y)),
      );
    return norm(a) === norm(b);
  }
  const dirty = $derived(current !== null && typeof draft !== "string" && !same(draft, current.value));

  async function reviewChanges() {
    if (typeof draft === "string") return;
    try {
      review = { preview: await api.previewScheduler({ value: draft, reason: "" }), value: draft };
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 6000 });
    }
  }

  async function save(reason: string) {
    if (!review) return;
    saving = true;
    try {
      await api.saveScheduler({ value: review.value, reason });
      review = null;
      showFlash("Saved: the next plan uses the new settings", { tone: "success" });
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
      <h1>Scheduler</h1>
      <p>How the scheduler decides what each station starts and when. Changes are versioned: every plan names the settings version that made it.</p>
    </div>
    {#if canEdit && dirty}
      <div class="row">
        <Button label="Undo changes" onclick={reset} />
        <Button tone="info" surface="solid" label="Review changes" onclick={reviewChanges} />
      </div>
    {/if}
  </header>

  <ManagerGate what="The scheduler" viewable>
    {#if error}<Notice tone="error" message={error} />{/if}
    {#if !current}
      {#if !error}<Spinner />{/if}
    {:else}
      <Notice
        tone="info"
        title={current.builtin ? "Kishtin's default settings" : `Version saved: ${current.version.reason}`}
        message="The expo's own changes on the pass (a new target time, an urgent flag) always come before these settings."
      />
      {#if typeof draft === "string"}<Notice tone="error" message={draft} />{/if}

      <div class="grid">
        <Card padding="md" title="Strategy">
          <div class="stack">
            <SegmentedControl
              ariaLabel="Strategy"
              value={strategy}
              onchange={(v) => (strategy = v)}
              block
              options={[
                { value: "backward", label: "Backward scheduling" },
                { value: "fifo", label: "First in, first out" },
              ]}
            />
            {#if strategy === "backward"}
              <p class="muted">Plans back from each course's target so every item reaches the pass together, within each piece of equipment's capacity. When equipment is full, the course moves later and shows at risk.</p>
            {:else}
              <p class="muted">Starts items in the order their courses were fired, as soon as equipment allows. Simple for a quiet service; items of a course may not finish together.</p>
            {/if}
          </div>
        </Card>

        <Card padding="md" title="Batching">
          <div class="stack">
            <Toggle checked={batching} label="Cook the same dish for several tables at once" onchange={(on) => (batching = on)} />
            <label class="field"><span>Start times this close are cooked together (seconds)</span><TextInput bind:value={windowText} disabled={!batching} /></label>
            <p class="muted small">Only dishes marked "can be cooked for several tables" are batched, and only as early as the dish can wait at the pass.</p>
          </div>
        </Card>

        <Card padding="md" title="Priorities">
          <div class="stack">
            <Toggle checked={allergy} label="Allergy orders go first" onchange={(on) => (allergy = on)} />
            <Toggle checked={takeaway} label="Takeaways go first" onchange={(on) => (takeaway = on)} />
            <label class="field"><span>Tables with at least this many covers go first (0: off)</span><TextInput bind:value={largeText} /></label>
            <label class="field"><span>Each priority moves a course this much earlier in the queue (seconds)</span><TextInput bind:value={leadText} /></label>
            <p class="muted small">A course the expo flags urgent counts as two priorities.</p>
          </div>
        </Card>

        <Card padding="md" title="At the pass">
          <div class="stack">
            <label class="field"><span>A ready course counts as late after waiting this long (seconds)</span><TextInput bind:value={passWaitText} /></label>
            <label class="field"><span>Screens ask for a fresh plan this often (seconds)</span><TextInput bind:value={replanText} /></label>
            <p class="muted small">Every screen also re-plans straight away after any order, fire, start, finish or void.</p>
          </div>
        </Card>

        <Card padding="md" title="Station buffers">
          <div class="stack">
            <p class="muted small">Extra time before a station's items are due, such as 30 seconds for a slow plating station.</p>
            {#each stations as s (s.id)}
              <label class="buffer">
                <span>{s.name}</span>
                <TextInput value={buffers[s.id] ?? ""} oninput={(v: string) => (buffers[s.id] = v)} placeholder="0" ariaLabel={`${s.name} buffer in seconds`} />
                <span class="muted small">s</span>
              </label>
            {:else}
              <p class="muted">No stations yet.</p>
            {/each}
          </div>
        </Card>

        <Card padding="md" title="Not built yet">
          <ul class="muted">
            <li>Named service profiles ("Weekday lunch", "Saturday dinner").</li>
            <li>Replaying a past service under new settings before switching.</li>
            <li>A fixed pace between courses, and course gaps per service.</li>
            <li>Cook times learned from real cooks.</li>
          </ul>
        </Card>
      </div>
    {/if}
  </ManagerGate>
</div>

{#if review}
  <ReviewChanges title="Review scheduler change" preview={review.preview} busy={saving} onsave={(r) => save(r)} onclose={() => (review = null)} />
{/if}

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
    gap: var(--space-5);
    align-items: start;
  }
  .buffer {
    display: grid;
    grid-template-columns: 1fr 100px auto;
    gap: var(--space-3);
    align-items: center;
  }
  ul {
    margin: 0;
    padding-left: var(--space-6);
  }
  .small {
    font-size: var(--font-size-sm);
  }
  p {
    margin: 0;
  }
</style>
