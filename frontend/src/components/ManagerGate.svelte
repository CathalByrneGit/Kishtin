<script lang="ts">
  import { Modal, Notice } from "@kenn-io/kit-ui";
  import type { Snippet } from "svelte";
  import Login from "../screens/Login.svelte";
  import { isManagerPassword } from "../lib/session.svelte";

  interface Props {
    /** what the page is, for the notice: "Staff", "Device keys" */
    what: string;
    /** everyone can see the page; only editing needs the manager password */
    viewable?: boolean;
    children: Snippet;
  }
  let { what, viewable = false, children }: Props = $props();

  let open = $state(false);
  const allowed = $derived(isManagerPassword());
</script>

{#if !allowed}
  <Notice
    tone="info"
    title={viewable ? `${what} is shared by every device` : `${what} needs the manager password`}
    message={viewable
      ? "Everyone can see it. Only a manager logged in with their password can change it."
      : "Log in as a manager with a password to see and change this. A PIN login is not enough."}
    actionLabel="Manager login"
    onaction={() => (open = true)}
  />
{/if}

{#if allowed || viewable}
  {@render children()}
{/if}

{#if open}
  <Modal title="Manager login" onclose={() => (open = false)} maxWidth="520px">
    <p class="muted">This device will be logged in as the manager for 30 minutes.</p>
    <Login managersOnly password onloggedin={() => (open = false)} />
  </Modal>
{/if}

<style>
  p {
    margin: 0;
  }
</style>
