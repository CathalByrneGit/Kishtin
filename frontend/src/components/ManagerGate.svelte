<script lang="ts">
  import { Modal, Notice } from "@kenn-io/kit-ui";
  import type { Snippet } from "svelte";
  import Login from "../screens/Login.svelte";
  import { allowed } from "../lib/can.svelte";

  interface Props {
    /** what the page is, for the notice: "Staff", "Device keys" */
    what: string;
    /** the action that changes it, as in Setup › Permissions */
    action: string;
    /** everyone can see the page; only editing needs permission */
    viewable?: boolean;
    children: Snippet;
  }
  let { what, action, viewable = false, children }: Props = $props();

  let open = $state(false);
  const access = $derived(allowed(action));
</script>

{#if access !== "yes"}
  <Notice
    tone="info"
    title={viewable ? `${what} is shared by every device` : `${what} needs permission`}
    message={access === "password"
      ? `${viewable ? "Everyone can see it. " : ""}Log in with your password to change it. A PIN login is not enough.`
      : `${viewable ? "Everyone can see it. " : ""}You cannot change it. A manager can log in here, or allow you in Setup › Permissions.`}
    actionLabel="Password login"
    onaction={() => (open = true)}
  />
{/if}

{#if access === "yes" || viewable}
  {@render children()}
{/if}

{#if open}
  <Modal title="Password login" onclose={() => (open = false)} maxWidth="520px">
    <p class="muted">This device will be logged in with a password for 30 minutes.</p>
    <Login passwordOnly password onloggedin={() => (open = false)} />
  </Modal>
{/if}

<style>
  p {
    margin: 0;
  }
</style>
