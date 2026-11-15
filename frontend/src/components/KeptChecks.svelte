<script lang="ts">
  import { t } from "../lib/i18n.svelte";
  import { Button, Notice, TextInput, showFlash } from "@kenn-io/kit-ui";
  import { onMount } from "svelte";
  import { mine, summary } from "../lib/kept";
  import { kept, rememberLists, remove, sendKept, setAction } from "../lib/kept.svelte";
  import { dublinTime } from "../lib/format";
  import { live, onChange } from "../lib/live.svelte";
  import { session } from "../lib/session.svelte";

  // Checks kept on this device while Kishtin could not be reached. They
  // show until Kishtin has saved them, so nobody takes one for a record.

  const own = $derived(mine(kept.list, session.user?.id));
  const line = $derived.by(() => {
    const s = summary(kept.list, session.user?.id);
    let out = t("Checks kept on this device and not in Kishtin yet: {n}.", { n: s.count });
    if (s.others.length > 0) out += " " + t("{names} must log in here to send theirs.", { names: s.others.join(", ") });
    return out;
  });
  let open = $state(false);
  let confirming = $state<string | null>(null);

  async function send() {
    const n = await sendKept();
    if (n > 0) {
      live.saveFailed = false;
      showFlash(t("Kept checks saved in Kishtin: {n}", { n }), { tone: "success" });
    }
    if (own.some((k) => k.problem)) open = true;
  }

  // Send as soon as the connection is back, and when someone logs in.
  onMount(() => {
    void rememberLists();
    return onChange((kind) => {
      if (kind === "ready" && own.length > 0) void send();
      if (kind === "ready" || kind === "settings") void rememberLists();
    });
  });
  $effect(() => {
    if (session.user && live.status === "synced" && own.length > 0 && !own.some((k) => k.problem)) void send();
  });
</script>

{#if kept.list.length > 0}
  <div class="kept">
    <Notice tone="warning" title={t("Kept on this device")} message={line} />
    <div class="row">
      {#if own.length > 0}
        <Button tone="info" surface="solid" label={kept.sending ? t("Sending…") : t("Send to Kishtin now")} disabled={kept.sending || live.status !== "synced"} onclick={send} />
      {/if}
      <Button label={open ? t("Hide them") : t("Show them")} onclick={() => (open = !open)} />
    </div>
    {#if open}
      <ul>
        {#each kept.list as k (k.id)}
          <li>
            <div class="what">
              <strong>{k.label}</strong>
              <span class="muted">{k.userName}, {dublinTime(k.keptAt)}</span>
            </div>
            {#if k.problem}
              <Notice tone="error" title={t("Kishtin did not save this")} message={k.problem} />
              {#if k.userId === session.user?.id}
                <label class="field">
                  <span>{t("Corrective action")}</span>
                  <TextInput value={k.body.corrective_action ?? ""} oninput={(v) => setAction(k.id, v)} block placeholder={t("What did you do about it?")} />
                </label>
              {/if}
            {/if}
            {#if k.userId === session.user?.id}
              {#if confirming === k.id}
                <div class="row">
                  <span>{t("Remove it? It was never saved in Kishtin.")}</span>
                  <Button label={t("Keep it")} onclick={() => (confirming = null)} />
                  <Button tone="danger" label={t("Remove")} onclick={() => (remove(k.id), (confirming = null))} />
                </div>
              {:else}
                <div><Button size="sm" label={t("Remove")} ariaLabel={`${t("Remove")}: ${k.label}`} onclick={() => (confirming = k.id)} /></div>
              {/if}
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </div>
{/if}

<style>
  .kept {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-4) var(--space-5) 0;
  }
  ul {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
  li {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3);
    border: 1px solid var(--border-default);
    border-radius: var(--radius-md, 6px);
  }
  .what {
    display: flex;
    gap: var(--space-3);
    flex-wrap: wrap;
    align-items: baseline;
  }
</style>
