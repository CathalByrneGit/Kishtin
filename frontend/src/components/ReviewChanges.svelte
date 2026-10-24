<script module lang="ts">
  export interface Preview {
    changes: string[];
    less_strict?: string[] | null;
    notes: string[];
    problem?: string;
  }
</script>

<script lang="ts">
  import { Button, Modal, Notice, TextInput } from "@kenn-io/kit-ui";

  interface Props {
    title: string;
    preview: Preview;
    busy: boolean;
    onsave: (reason: string, planRef: string) => void;
    onclose: () => void;
  }
  let { title, preview, busy, onsave, onclose }: Props = $props();

  let reason = $state("");
  let planRef = $state("");
  const lessStrict = $derived(preview.less_strict ?? []);
  // The preview's problem is about the reason it was asked with (empty);
  // only problems other than a missing reason or plan reference block saving.
  const blocking = $derived(preview.problem && !/needs a reason|plan section/.test(preview.problem) ? preview.problem : "");
</script>

<Modal {title} {onclose} maxWidth="560px">
  <div class="stack kitchen">
    {#if preview.changes.length > 0}
      <div>
        <strong>What changes</strong>
        <ul>
          {#each preview.changes as c (c)}<li class="num">{c}</li>{/each}
        </ul>
      </div>
    {/if}
    {#each lessStrict as l (l)}
      <Notice tone="warning" title="Less strict" message={l} />
    {/each}
    {#each preview.notes as n (n)}
      <p class="muted">{n}</p>
    {/each}
    {#if blocking}
      <Notice tone="error" title="Cannot save" message={blocking} />
    {:else}
      <label class="field">
        <span>Reason for the change (required)</span>
        <TextInput bind:value={reason} size="lg" block />
      </label>
      {#if lessStrict.length > 0}
        <label class="field">
          <span>HACCP plan section or validation (required)</span>
          <TextInput bind:value={planRef} size="lg" block />
        </label>
      {/if}
    {/if}
  </div>
  {#snippet footer()}
    <div class="row kitchen">
      <Button label="Cancel" onclick={onclose} />
      <Button
        tone="success"
        surface="solid"
        label={busy ? "Saving…" : "Save changes"}
        disabled={busy || Boolean(blocking) || !reason.trim() || (lessStrict.length > 0 && !planRef.trim())}
        onclick={() => onsave(reason.trim(), planRef.trim())}
      />
    </div>
  {/snippet}
</Modal>

<style>
  ul {
    margin: var(--space-2) 0 0;
    padding-left: var(--space-7);
  }
  li {
    font-size: var(--font-size-md);
  }
  p {
    margin: 0;
  }
</style>
