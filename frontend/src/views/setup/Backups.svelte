<script lang="ts">
  import { Button, Card, Chip, CopyButton, MetricCard, Notice, Spinner, Table, TableHeaderCell, showFlash } from "@kenn-io/kit-ui";
  import { Archive } from "@lucide/svelte";
  import ManagerGate from "../../components/ManagerGate.svelte";
  import { api, message, type Schemas } from "../../lib/api";
  import { dublinDate, dublinTime, shortDate } from "../../lib/format";
  import { can } from "../../lib/can.svelte";

  let backups = $state<Schemas["Backup"][] | null>(null);
  let system = $state<Schemas["System"] | null>(null);
  let error = $state("");
  let busy = $state(false);

  async function load() {
    try {
      const [b, s] = await Promise.all([api.backups(), api.system()]);
      backups = [...b.backups].sort((x, y) => y.taken_at.localeCompare(x.taken_at));
      system = s.value;
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  $effect(() => {
    if (can("backup")) void load();
  });

  async function backupNow() {
    busy = true;
    try {
      const b = await api.backupNow();
      showFlash(b.verified ? "Backup made and verified" : "Backup made, but it did not verify", { tone: b.verified ? "success" : "danger", durationMs: 6000 });
      await load();
    } catch (err) {
      showFlash(message(err), { tone: "danger", durationMs: 8000 });
    } finally {
      busy = false;
    }
  }

  const when = (t: string) => `${shortDate(dublinDate(new Date(t)))} ${new Date(t).getFullYear()}, ${dublinTime(t)}`;
  const size = (n: number) => (n >= 1 << 20 ? `${(n / (1 << 20)).toFixed(1)} MB` : `${Math.max(1, Math.round(n / 1024))} KB`);
  const latest = $derived(backups?.[0] ?? null);
  const schedule = $derived(
    !system ? "" : system.backup_interval_minutes === 0 ? "Off" : system.backup_interval_minutes % 60 === 0 ? `Every ${system.backup_interval_minutes / 60} h` : `Every ${system.backup_interval_minutes} min`,
  );
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>Backups</h1>
      <p>
        Each backup is a consistent copy of the database, verified after it is written, with a <code>.sha256</code> file beside it. Backups stay
        on this server. Copy them, and the <code>files/</code> folder of dockets, somewhere else yourself.
      </p>
    </div>
    {#if can("backup")}
      <Button tone="info" surface="solid" label={busy ? "Backing up…" : "Back up now"} disabled={busy} onclick={backupNow}><Archive size={16} /></Button>
    {/if}
  </header>

  <ManagerGate what="Backups" action="backup">
    {#if error}<Notice tone="error" message={error} />{/if}
    {#if backups === null}
      {#if !error}<Spinner />{/if}
    {:else}
      <div class="metrics">
        <MetricCard label="Latest backup" value={latest ? when(latest.taken_at) : "None yet"} meta={latest ? `${latest.records} records` : "Back up now to make the first"} tone={latest ? (latest.verified ? "success" : "danger") : "warning"} />
        <MetricCard label="Schedule" value={schedule} meta="Change it in Settings" />
        <MetricCard label="Kept" value={system?.backup_keep === 0 ? "Every backup" : `Newest ${system?.backup_keep ?? ""}`} meta={`${backups.length} on the server now`} />
      </div>

      <Card padding="none">
        {#if backups.length === 0}
          <p class="empty muted">No backups yet.</p>
        {:else}
          <Table unsorted ariaLabel="Backups">
            {#snippet header()}
              <TableHeaderCell label="Taken" />
              <TableHeaderCell label="File" />
              <TableHeaderCell label="Records" />
              <TableHeaderCell label="Size" />
              <TableHeaderCell label="Verified" />
              <TableHeaderCell label="Chain head" />
            {/snippet}
            {#each backups as b (b.name)}
              <tr>
                <td class="nowrap">{when(b.taken_at)}</td>
                <td><code title={b.path}>{b.name}</code></td>
                <td class="num">{b.records}</td>
                <td class="num nowrap">{size(b.size)}</td>
                <td><Chip tone={b.verified ? "success" : "danger"} uppercase={false}>{b.verified ? "Pass" : "Fail"}</Chip></td>
                <td>
                  <span class="hash"><code>{b.head_hash.slice(0, 16)}…</code><CopyButton text={b.head_hash} ariaLabel="Copy chain head" /></span>
                </td>
              </tr>
            {/each}
          </Table>
        {/if}
      </Card>
    {/if}
  </ManagerGate>
</div>

<style>
  .metrics :global(.kit-metric-card__layout) {
    min-height: 0;
  }
  .metrics {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    gap: var(--space-5);
  }
  code {
    font-family: var(--font-mono);
    font-size: var(--font-size-sm);
  }
  .hash {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
  }
  .nowrap {
    white-space: nowrap;
  }
  .empty {
    margin: 0;
    padding: var(--space-5) var(--space-6);
  }
</style>
