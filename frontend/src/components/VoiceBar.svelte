<script lang="ts">
  import { Button, Chip, IconButton, TextInput } from "@kenn-io/kit-ui";
  import { Keyboard, Mic, Send, X } from "@lucide/svelte";
  import { onMount } from "svelte";
  import {
    TAP_MS,
    cancelRecording,
    heldFor,
    loadVoiceStatus,
    micProblem,
    releaseMicrophone,
    sendText,
    startRecording,
    stopRecording,
    voice,
  } from "../lib/voice.svelte";

  interface Props {
    /** kitchen-sized controls */
    big?: boolean;
    /** a kitchen screen: every control a full-size tap target */
    touch?: boolean;
    /** show the typed command field open */
    typing?: boolean;
    /** hold Space anywhere on the page to talk (one bar per page) */
    keys?: boolean;
  }
  let { big = false, touch = false, typing = false, keys = false }: Props = $props();

  let typed = $state("");
  let showTyping = $state(false);
  // stoppedOnDown: a tap stopped a tap-started recording, so the matching
  // release does nothing.
  let stoppedOnDown = false;

  const problem = micProblem();
  const canTalk = $derived(!problem && voice.status?.listen !== false);
  const typeOpen = $derived(typing || showTyping || !canTalk);

  onMount(() => {
    void loadVoiceStatus();
    const hidden = () => {
      if (document.hidden) releaseMicrophone();
    };
    document.addEventListener("visibilitychange", hidden);
    const editable = (t: EventTarget | null) =>
      t instanceof HTMLElement && (t.isContentEditable || ["INPUT", "TEXTAREA", "SELECT"].includes(t.tagName));
    const down = (e: KeyboardEvent) => {
      if (e.key === "Escape" && voice.phase === "recording") cancelRecording();
      if (!keys || e.code !== "Space" || e.repeat || editable(e.target) || !canTalk) return;
      e.preventDefault();
      void startRecording();
    };
    const up = (e: KeyboardEvent) => {
      if (!keys || e.code !== "Space" || editable(e.target)) return;
      e.preventDefault();
      void stopRecording();
    };
    window.addEventListener("keydown", down);
    window.addEventListener("keyup", up);
    return () => {
      document.removeEventListener("visibilitychange", hidden);
      window.removeEventListener("keydown", down);
      window.removeEventListener("keyup", up);
      releaseMicrophone();
    };
  });

  function press(e: PointerEvent) {
    if (e.button !== 0) return;
    (e.currentTarget as HTMLElement).setPointerCapture?.(e.pointerId);
    if (voice.phase === "recording") {
      stoppedOnDown = true;
      void stopRecording();
      return;
    }
    stoppedOnDown = false;
    void startRecording();
  }

  function release() {
    if (stoppedOnDown) {
      stoppedOnDown = false;
      return;
    }
    // A short tap keeps listening until the next tap; a hold sends on release.
    if (voice.phase === "recording" && heldFor() >= TAP_MS) void stopRecording();
  }

  async function submit() {
    const t = typed;
    typed = "";
    await sendText(t);
  }

  const OUTCOMES: Record<string, { label: string; tone: "success" | "info" | "warning" | "danger" | "neutral" }> = {
    done: { label: "Done", tone: "success" },
    answered: { label: "Answer", tone: "info" },
    asked: { label: "Question", tone: "warning" },
    refused: { label: "Not done", tone: "danger" },
    not_understood: { label: "Not understood", tone: "warning" },
    cancelled: { label: "Cancelled", tone: "neutral" },
  };

  const label = $derived(
    voice.phase === "recording" ? "Listening… release to send" : voice.phase === "sending" ? "Working…" : "Hold to talk",
  );
</script>

<div class="voice" class:big>
  <div class="controls">
    {#if canTalk}
      <button
        type="button"
        class="talk kit-control-states"
        class:recording={voice.phase === "recording"}
        disabled={voice.phase === "sending"}
        aria-label={label}
        aria-pressed={voice.phase === "recording"}
        onpointerdown={press}
        onpointerup={release}
        onpointercancel={() => cancelRecording()}
        oncontextmenu={(e) => e.preventDefault()}
        onkeydown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            if (voice.phase === "recording") void stopRecording();
            else void startRecording();
          }
        }}
      >
        <Mic size={big ? 28 : 20} />
        <span>{label}</span>
      </button>
      {#if !typing && (big || touch)}
        <Button label={showTyping ? "Hide typing" : "Type"} ariaLabel={showTyping ? "Hide typed command" : "Type a command"} onclick={() => (showTyping = !showTyping)}>
          <Keyboard size={20} />
        </Button>
      {:else if !typing}
        <IconButton ariaLabel={showTyping ? "Hide typed command" : "Type a command"} title="Type a command" onclick={() => (showTyping = !showTyping)}>
          <Keyboard size={18} />
        </IconButton>
      {/if}
    {/if}
    {#if typeOpen}
      <form
        class="typed"
        onsubmit={(e) => {
          e.preventDefault();
          void submit();
        }}
      >
        <TextInput bind:value={typed} placeholder="Type a command, such as: fire table 4 mains" ariaLabel="Voice command" block />
        <Button tone="info" surface="solid" label="Send" type="submit" disabled={!typed.trim() || voice.phase === "sending"}><Send size={16} /></Button>
      </form>
    {/if}
  </div>

  {#if voice.error}
    <p class="problem" role="alert">{voice.error}</p>
  {:else if problem && !typing}
    <p class="muted small">{problem}</p>
  {:else if voice.status && !voice.status.listen && !typing}
    <p class="muted small">Spoken commands are not set up on this server yet. Typed commands work the same way.</p>
  {/if}

  {#if voice.reply}
    {@const o = OUTCOMES[voice.reply.outcome] ?? { label: "Answer", tone: "info" as const }}
    <div class="reply" aria-live="polite">
      <div class="said">
        <Chip uppercase={false} tone={o.tone} dot>{o.label}</Chip>
        <span class="muted heard">Heard: “{voice.reply.heard}”</span>
        <span class="grow"></span>
        <IconButton ariaLabel="Clear reply" size={big ? undefined : "sm"} onclick={() => (voice.reply = null)}><X size={16} /></IconButton>
      </div>
      <p class="say">{voice.reply.say}</p>
      {#if voice.reply.waiting}
        <div class="answers">
          {#each voice.reply.options ?? [] as opt (opt)}
            <Button tone="info" surface="solid" label={opt} disabled={voice.phase === "sending"} onclick={() => sendText(opt)} />
          {/each}
          {#if voice.reply.confirm}
            <Button
              tone="danger"
              surface="solid"
              label={voice.reply.intent === "void" ? "Confirm void" : "Confirm"}
              disabled={voice.phase === "sending"}
              onclick={() => sendText(voice.reply?.intent === "void" ? "confirm void" : "confirm")}
            />
          {/if}
          <Button label="Cancel" disabled={voice.phase === "sending"} onclick={() => sendText("cancel")} />
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .voice {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
  .controls {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    flex-wrap: wrap;
  }
  .talk {
    display: inline-flex;
    align-items: center;
    gap: var(--space-3);
    min-height: max(44px, var(--kit-control-height, 0px));
    padding: 0 var(--space-5);
    border: var(--border-width) solid var(--accent-blue);
    border-radius: var(--radius-md);
    background: var(--accent-blue);
    color: var(--text-on-accent, var(--bg-surface));
    font: inherit;
    font-weight: var(--font-weight-semibold);
    cursor: pointer;
    touch-action: none;
    user-select: none;
    -webkit-user-select: none;
  }
  .big .talk {
    min-height: 64px;
    padding: 0 var(--space-6);
    font-size: var(--font-size-lg);
  }
  .talk.recording {
    background: var(--accent-red);
    border-color: var(--accent-red);
  }
  .talk:disabled {
    cursor: progress;
  }
  .typed {
    flex: 1;
    min-width: min(100%, 280px);
    display: flex;
    gap: var(--space-2);
  }
  .problem {
    color: var(--accent-red);
    font-weight: var(--font-weight-medium);
  }
  .reply {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-4);
    border: var(--border-width) solid var(--border-default);
    border-radius: var(--radius-md);
    background: var(--bg-surface);
  }
  .said {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-width: 0;
  }
  .heard {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .grow {
    flex: 1;
  }
  .say {
    font-size: var(--font-size-lg);
    font-weight: var(--font-weight-semibold);
  }
  .big .say {
    font-size: var(--font-size-xl);
  }
  .answers {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }
  .small {
    font-size: var(--font-size-sm);
  }
  p {
    margin: 0;
  }
</style>
