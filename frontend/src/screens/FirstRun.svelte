<script lang="ts">
  import { Button, Card, CopyButton, Notice, TextInput } from "@kenn-io/kit-ui";
  import { api, message, type Schemas } from "../lib/api";
  import { pairDevice, startSession } from "../lib/session.svelte";

  interface Props {
    status: Schemas["SetupStatusOutBody"];
  }
  let { status }: Props = $props();

  let restaurant = $state("");
  let name = $state("");
  let password = $state("");
  let again = $state("");
  let pin = $state("");
  let code = $state("");
  let busy = $state(false);
  let error = $state("");
  let done = $state<Schemas["FirstRunResult"] | null>(null);

  const problem = $derived.by(() => {
    if (!restaurant.trim()) return "Type the restaurant's name.";
    if (!name.trim()) return "Type your name.";
    if (password.length < 10) return "The password needs at least 10 characters.";
    if (password !== again) return "The two passwords are not the same.";
    if (pin && !/^\d{4,8}$/.test(pin)) return "A PIN is 4 to 8 digits.";
    if (status.code_needed && !code.trim()) return "Type the setup code.";
    return "";
  });

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    if (problem) return;
    busy = true;
    error = "";
    try {
      done = await api.setup({
        restaurant: restaurant.trim(),
        name: name.trim(),
        password,
        pin: pin || undefined,
        code: status.code_needed ? code : undefined,
        device: "Office computer",
      });
    } catch (err) {
      error = message(err);
    } finally {
      busy = false;
    }
  }

  // Pairs this screen and logs the new manager in, so they carry straight on.
  async function open() {
    if (!done) return;
    busy = true;
    error = "";
    pairDevice(done.device_key);
    try {
      startSession(await api.login({ user_id: done.user.id, password }));
    } catch (err) {
      error = message(err);
      busy = false;
    }
  }
</script>

<div class="page">
  <header>
    <h1>Kishtin</h1>
    <p class="muted">Kishtin, from <i>cistin</i>, Irish for kitchen.</p>
  </header>

  {#if done}
    <Card title="Kishtin is ready" padding="md">
      <div class="stack">
        <p>{restaurant.trim()} is set up, and {done.user.name} is its manager. Next:</p>
        <ol>
          <li><strong>Add your staff</strong> in Setup › Staff and access › Staff. Each gets a PIN for the kitchen tablets.</li>
          <li><strong>Add your menu</strong> in Setup › Kitchen › Menu: stations, equipment and dishes with their cook times.</li>
          <li><strong>Check your fridges and cleaning tasks</strong> in Setup › Food safety › Fridges and tasks.</li>
          <li><strong>Connect tablets and phones</strong> in Setup › Staff and access › Device keys: tap Connect a tablet and scan the QR code with the tablet.</li>
        </ol>
        <details>
          <summary>Key for scripts and the command line (most kitchens never need this)</summary>
          <div class="stack small">
            <p class="muted">It acts as {done.user.name}. It is shown only now: copy it somewhere safe, or make another later in Device keys.</p>
            <div class="key">
              <code>{done.api_key}</code>
              <CopyButton text={done.api_key} ariaLabel="Copy the key" />
            </div>
          </div>
        </details>
        {#if error}<Notice tone="error" title="Not logged in" message={error} />{/if}
        <Button tone="info" surface="solid" label={busy ? "Opening…" : "Open Kishtin"} disabled={busy} onclick={open} />
      </div>
    </Card>
  {:else if !status.possible}
    <Notice
      tone="info"
      title="Set Kishtin up on its own computer"
      message="Kishtin is not set up yet. Open it in a browser on the computer it runs on to set it up there."
    />
  {:else}
    <Card title="Set up Kishtin" padding="md">
      <form class="stack" onsubmit={submit}>
        <p>Name the restaurant and make the manager's account. It takes a minute, and everything else can be changed later.</p>
        <label class="field">
          <span>Restaurant name</span>
          <TextInput bind:value={restaurant} placeholder="Shown on every screen and on inspection packs" autocomplete="organization" required block />
        </label>
        <label class="field">
          <span>Your name (the manager)</span>
          <TextInput bind:value={name} autocomplete="name" required block />
        </label>
        <label class="field">
          <span>Password, at least 10 characters</span>
          <TextInput type="password" bind:value={password} autocomplete="new-password" required block />
        </label>
        <label class="field">
          <span>Password again</span>
          <TextInput type="password" bind:value={again} autocomplete="new-password" required block />
        </label>
        <p class="small muted">The password protects setup, limits, staff and exports. Keep it to yourself: Kishtin cannot show it again.</p>
        <label class="field">
          <span>PIN for kitchen tablets (optional, 4 to 8 digits)</span>
          <TextInput type="tel" bind:value={pin} autocomplete="off" block />
        </label>
        {#if status.code_needed}
          <label class="field">
            <span>Setup code</span>
            <TextInput bind:value={code} placeholder="ABCD-EFGH-JKLM" autocomplete="off" block />
          </label>
          <p class="small muted">
            This device is not the computer Kishtin runs on, so it needs the setup code. It is shown where Kishtin was started, and
            saved in <code>setup-code.txt</code> in Kishtin's data folder.
          </p>
        {/if}
        {#if error}<Notice tone="error" title="Not set up" message={error} />{/if}
        {#if problem && (restaurant || name || password)}<p class="small muted" aria-live="polite">{problem}</p>{/if}
        <Button type="submit" tone="info" surface="solid" label={busy ? "Setting up…" : "Set up Kishtin"} disabled={busy || Boolean(problem)} />
      </form>
    </Card>
  {/if}
</div>

<style>
  .page {
    max-width: 560px;
    margin: 0 auto;
    padding: var(--space-8) var(--space-6);
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }
  h1 {
    font-size: var(--font-size-2xl);
    margin: 0;
  }
  header p {
    margin: 0;
  }
  p {
    margin: 0;
    line-height: 1.5;
  }
  ol {
    margin: 0;
    padding-left: var(--space-6);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    line-height: 1.5;
  }
  .small {
    font-size: var(--font-size-sm);
  }
  summary {
    cursor: pointer;
  }
  .key {
    display: flex;
    gap: var(--space-2);
    align-items: center;
  }
  code {
    font-family: var(--font-mono);
    word-break: break-all;
  }
</style>
