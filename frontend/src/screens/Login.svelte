<script lang="ts">
  import { Button, Notice, Spinner, TextInput } from "@kenn-io/kit-ui";
  import { Delete } from "@lucide/svelte";
  import { onMount } from "svelte";
  import { api, message, type Schemas } from "../lib/api";
  import { onChange } from "../lib/live.svelte";
  import { startSession, unpairDevice } from "../lib/session.svelte";

  interface Props {
    /** only list staff with a password, for a password login */
    passwordOnly?: boolean;
    /** start with the password form */
    password?: boolean;
    onloggedin?: () => void;
  }
  let { passwordOnly = false, password: usePassword = false, onloggedin }: Props = $props();

  let staff = $state<Schemas["StaffEntry"][] | null>(null);
  let picked = $state<Schemas["StaffEntry"] | null>(null);
  let pin = $state("");
  let pw = $state("");
  let passwordMode = $state(false);
  let error = $state("");
  let busy = $state(false);
  let forgot = $state(false);

  $effect(() => {
    passwordMode = usePassword;
  });

  async function load() {
    try {
      const all = (await api.staff()).staff;
      staff = passwordOnly ? all.filter((s) => s.has_password) : all;
    } catch (err) {
      error = message(err);
      staff = [];
    }
  }

  onMount(() => {
    void load();
    return onChange((kind) => {
      if (kind === "staff") void load();
    });
  });

  function pick(s: Schemas["StaffEntry"]) {
    picked = s;
    pin = "";
    pw = "";
    error = "";
    passwordMode = usePassword || !s.has_pin;
  }

  function press(d: string) {
    if (pin.length < 8) pin += d;
  }

  async function login() {
    if (!picked) return;
    busy = true;
    error = "";
    try {
      const res = await api.login(passwordMode ? { user_id: picked.id, password: pw } : { user_id: picked.id, pin });
      startSession(res);
      onloggedin?.();
    } catch (err) {
      error = message(err);
      pin = "";
    } finally {
      busy = false;
    }
  }

  const digits = ["1", "2", "3", "4", "5", "6", "7", "8", "9"];
</script>

<div class="page kitchen">
  {#if !picked}
    <h1>{passwordOnly ? "Password login" : "Who is logging in?"}</h1>
    {#if staff === null}
      <Spinner />
    {:else if staff.length === 0 && !error}
      <Notice tone="info" title="No staff" message="No staff can log in here yet. A manager adds staff with kishtin staff add." />
    {:else}
      <div class="people">
        {#each staff as s (s.id)}
          <Button label={s.name} onclick={() => pick(s)} />
        {/each}
      </div>
    {/if}
    {#if error}<Notice tone="error" message={error} />{/if}
    {#if !passwordOnly}
      <div class="footer">
        <Button label="Unpair this device" onclick={unpairDevice} />
      </div>
    {/if}
  {:else}
    <h1>{picked.name}</h1>
    {#if passwordMode}
      <form
        class="stack"
        onsubmit={(e) => {
          e.preventDefault();
          void login();
        }}
      >
        <label class="field">
          <span>Password</span>
          <TextInput type="password" bind:value={pw} autocomplete="current-password" autofocus />
        </label>
        {#if error}<Notice tone="error" message={error} />{/if}
        <Button type="submit" tone="info" surface="solid" label={busy ? "Logging in…" : "Log in"} disabled={busy || !pw} />
      </form>
      {#if forgot}
        <Notice
          tone="info"
          title="Forgot the password?"
          message="Another manager can set a new one in Setup › Staff and access › Staff. If nobody can log in as a manager: on the Kishtin computer, open PowerShell (Windows) or Terminal (Mac), type kishtind reset-password and press Enter, then type the new password twice. It works only on that computer."
        />
      {:else}
        <Button label="Forgot the password?" onclick={() => (forgot = true)} />
      {/if}
    {:else}
      <div class="pin num" aria-live="polite" aria-label="PIN entered">
        {#each { length: Math.max(4, pin.length) } as _, i (i)}
          <span class="dot" class:filled={i < pin.length}></span>
        {/each}
      </div>
      {#if error}<Notice tone="error" message={error} />{/if}
      <div class="pad">
        {#each digits as d (d)}
          <Button label={d} onclick={() => press(d)} />
        {/each}
        <Button label="Clear" onclick={() => (pin = "")} />
        <Button label="0" onclick={() => press("0")} />
        <Button ariaLabel="Delete last digit" label="" onclick={() => (pin = pin.slice(0, -1))}><Delete size={22} /></Button>
      </div>
      <Button tone="info" surface="solid" label={busy ? "Logging in…" : "Log in"} disabled={busy || pin.length < 4} onclick={login} />
      {#if picked.has_password}
        <Button label="Use manager password instead" onclick={() => (passwordMode = true)} />
      {/if}
    {/if}
    <Button label="Someone else" onclick={() => (picked = null)} />
  {/if}
</div>

<style>
  .page {
    max-width: 480px;
    margin: 0 auto;
    padding: var(--space-7) var(--space-6);
    display: flex;
    flex-direction: column;
    gap: var(--space-5);
  }
  h1 {
    font-size: var(--font-size-2xl);
  }
  .people {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
    gap: var(--space-4);
  }
  .pin {
    display: flex;
    justify-content: center;
    gap: var(--space-5);
    padding: var(--space-5) 0;
  }
  .dot {
    width: 18px;
    height: 18px;
    border-radius: var(--radius-dot, 50%);
    border: 2px solid var(--text-secondary);
  }
  .dot.filled {
    background: var(--text-primary);
    border-color: var(--text-primary);
  }
  .pad {
    --kit-control-height: 72px;
    --kit-control-font-size: var(--font-size-2xl);
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: var(--space-4);
  }
  .footer {
    margin-top: var(--space-7);
  }
</style>
