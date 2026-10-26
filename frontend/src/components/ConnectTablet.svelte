<script lang="ts">
  import { Button, CopyButton, Modal, Notice, SegmentedControl, Spinner, TextInput } from "@kenn-io/kit-ui";
  import { api, message, type Schemas } from "../lib/api";
  import { dublinTime } from "../lib/format";
  import { onChange } from "../lib/live.svelte";
  import QrCode from "./QrCode.svelte";

  interface Props {
    onclose: () => void;
    /** called when a tablet has paired, so the list can refresh */
    onpaired?: () => void;
  }
  let { onclose, onpaired }: Props = $props();

  let name = $state("");
  let busy = $state(false);
  let error = $state("");
  let code = $state<Schemas["JoinCode"] | null>(null);
  let address = $state("");
  let paired = $state(false);
  let now = $state(Date.now());

  const link = $derived(code?.links.find((l) => l.address === address) ?? code?.links[0] ?? null);
  const ended = $derived(code !== null && now >= new Date(code.expires_at).getTime());

  async function make(e?: SubmitEvent) {
    e?.preventDefault();
    busy = true;
    error = "";
    try {
      code = await api.createJoinCode(name.trim());
      address = code.links[0]?.address ?? "";
      paired = false;
      now = Date.now();
    } catch (err) {
      error = message(err);
    } finally {
      busy = false;
    }
  }

  $effect(() => {
    const t = setInterval(() => (now = Date.now()), 5000);
    return () => clearInterval(t);
  });

  // A paired tablet shows up as a new device key.
  $effect(() =>
    onChange((kind) => {
      if (kind === "device" && code && !paired) {
        paired = true;
        onpaired?.();
      }
    }),
  );

  function another() {
    code = null;
    paired = false;
    name = "";
  }
</script>

<Modal title="Connect a tablet or phone" {onclose} maxWidth="640px">
  {#if !code}
    <form class="stack" onsubmit={make}>
      <p>Kishtin makes a QR code. Scan it with the tablet's camera, and the tablet sets itself up. The code works once, for 15 minutes.</p>
      <label class="field">
        <span>Name for the tablet or phone</span>
        <TextInput bind:value={name} placeholder="Pass tablet" autofocus block />
      </label>
      {#if error}<Notice tone="error" title="No QR code" message={error} />{/if}
      <Button type="submit" tone="info" surface="solid" label={busy ? "Making…" : "Make QR code"} disabled={busy || !name.trim()} />
    </form>
  {:else if paired}
    <div class="stack">
      <Notice tone="success" title="{code.name} is connected" message="Staff can now log in on it with their PIN. It is listed under Device keys, where it can be revoked." />
      <div class="actions">
        <Button label="Connect another" onclick={another} />
        <Button tone="info" surface="solid" label="Done" onclick={onclose} />
      </div>
    </div>
  {:else if ended}
    <div class="stack">
      <Notice tone="warning" title="This QR code has ended" message="It was not used within 15 minutes. Make a new one." />
      <Button tone="info" surface="solid" label="Make a new QR code" onclick={() => make()} />
    </div>
  {:else if link}
    <div class="show">
      <div class="code">
        <QrCode rows={link.qr} label="QR code to connect {code.name}" />
        {#if code.links.length > 1}
          <SegmentedControl
            ariaLabel="Network address"
            options={code.links.map((l) => ({ value: l.address, label: l.address }))}
            value={address}
            onchange={(v) => (address = v)}
          />
          <p class="small muted">This computer is on more than one network. Use the address of the one the tablet is on.</p>
        {/if}
      </div>
      <div class="stack">
        <ol>
          <li>Open the camera on <strong>{code.name}</strong> and point it at the QR code. Tap the link that appears.</li>
          {#if code.fingerprint}
            <li>The first time, the tablet installs Kishtin's certificate. The page on the tablet shows each step.</li>
          {/if}
          <li>The tablet pairs itself, and staff log in with their PIN.</li>
        </ol>
        <p class="waiting" aria-live="polite"><Spinner size={16} label="Waiting" /> Waiting for {code.name}. The code works until {dublinTime(code.expires_at)}.</p>
        <details>
          <summary>No camera? Type the link instead</summary>
          <div class="stack small">
            <div class="copy"><code>{link.url}</code><CopyButton text={link.url} ariaLabel="Copy the link" /></div>
            <p>Code: <strong class="mono">{code.code}</strong></p>
            {#if code.fingerprint}
              <p class="muted">Kishtin's certificate fingerprint (SHA-256), as the tablet shows it: <code>{code.fingerprint}</code></p>
            {/if}
          </div>
        </details>
      </div>
    </div>
  {/if}
</Modal>

<style>
  p,
  ol {
    margin: 0;
    line-height: 1.5;
  }
  ol {
    padding-left: var(--space-6);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }
  .show {
    display: grid;
    grid-template-columns: 260px 1fr;
    gap: var(--space-6);
    align-items: start;
  }
  @media (max-width: 640px) {
    .show {
      grid-template-columns: 1fr;
    }
  }
  .code {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
  .waiting {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    font-weight: 600;
  }
  .actions {
    display: flex;
    gap: var(--space-3);
    justify-content: flex-end;
  }
  .copy {
    display: flex;
    gap: var(--space-2);
    align-items: center;
  }
  code,
  .mono {
    font-family: var(--font-mono);
    word-break: break-all;
  }
  .small {
    font-size: var(--font-size-sm);
  }
  summary {
    cursor: pointer;
  }
</style>
