<script lang="ts">
  import { Card, Chip, Notice, Table, TableHeaderCell, Toggle } from "@kenn-io/kit-ui";
  import { onMount } from "svelte";
  import VoiceBar from "../components/VoiceBar.svelte";
  import { api, message, type Schemas } from "../lib/api";
  import { dublinTime } from "../lib/format";
  import { micProblem, setSpeak, voice } from "../lib/voice.svelte";

  let entries = $state<Schemas["LogEntry"][]>([]);
  let error = $state("");

  async function loadLog() {
    try {
      entries = (await api.voiceLog(50)).entries;
      error = "";
    } catch (err) {
      error = message(err);
    }
  }

  onMount(() => void loadLog());

  // Refresh the log after every reply.
  $effect(() => {
    if (voice.reply) void loadLog();
  });

  const commands: [string, string][] = [
    ["Fire table 4 mains", "Fires the course and sets its target time. Leave out the course to fire the next held one."],
    ["Hold table 4 mains", "Holds a fired course again, while nothing in it has started."],
    ["What's next on grill?", "Reads the scheduler's next starts for that station."],
    ["Burgers on, tables 4 and 7", "Marks those items cooking. Without a table, starts what the scheduler says to start now."],
    ["Table 7 ready", "Marks the fired course ready at the pass, once every item in it has been started."],
    ["86 salmon / Salmon back on", "Marks a dish sold out, or back on."],
    ["Walk-in fridge 3.4 degrees", "Logs a fridge or freezer reading. A fail asks what you did about it, then needs \"confirm\"."],
    ["Core temp chicken 78", "Logs a cooking check. Also \"reheat soup 74\" and \"hot hold curry 65\"."],
    ["Confirm / Cancel", "Answers a question. \"Table 4 or table 14?\" is answered with the table, or a tap."],
  ];

  const OUTCOME_LABEL: Record<string, string> = {
    done: "Done",
    answered: "Answered",
    asked: "Asked",
    refused: "Not done",
    not_understood: "Not understood",
    cancelled: "Cancelled",
  };
  const OUTCOME_TONE: Record<string, "success" | "info" | "warning" | "danger" | "neutral"> = {
    done: "success",
    answered: "info",
    asked: "warning",
    refused: "danger",
    not_understood: "warning",
    cancelled: "neutral",
  };

  const mic = micProblem();
</script>

<div class="page">
  <header class="head">
    <div>
      <h1>Voice</h1>
      <p>
        Hold the button and speak, or hold the space bar. Speech is turned into text by the Kishtin server's own speech
        sidecar: no audio leaves the building, and audio is not stored. Every command is logged with what was heard.
      </p>
    </div>
  </header>

  <Card padding="md" title="Try a command">
    <VoiceBar typing keys />
  </Card>

  <div class="grid">
    <Card padding="md" title="Set up">
      <dl>
        <dt>Spoken commands</dt>
        <dd>
          {#if voice.status}
            <Chip uppercase={false} dot tone={voice.status.listen ? "success" : "warning"}>{voice.status.listen ? "Set up" : "Not set up"}</Chip>
          {/if}
        </dd>
        <dt>Spoken replies</dt>
        <dd>
          {#if voice.status}
            <Chip uppercase={false} dot tone={voice.status.speak ? "success" : "neutral"}>{voice.status.speak ? "Set up" : "Not set up"}</Chip>
          {/if}
        </dd>
        <dt>This device's microphone</dt>
        <dd><Chip uppercase={false} dot tone={mic ? "warning" : "success"}>{mic ? "Not available" : "Available"}</Chip></dd>
      </dl>
      {#if mic}<p class="muted small">{mic}</p>{/if}
      {#if voice.status && (!voice.status.listen || !voice.status.speak)}
        <p class="muted small">
          A manager sets <code>speech.stt_url</code> (a whisper.cpp server) and <code>speech.tts_url</code> (a Piper server) in kishtin.toml.
          Both must run on this machine or the kitchen network. Typed commands work without them.
        </p>
      {/if}
      <Toggle checked={voice.speak} label="Speak replies on this device" onchange={(on) => setSpeak(on)} />
    </Card>

    <Card padding="md" title="What you can say">
      <ul class="commands">
        {#each commands as [say, does] (say)}
          <li><strong>“{say}”</strong><span class="muted">{does}</span></li>
        {/each}
      </ul>
      <p class="muted small">Voice cannot void items or delete anything. Everything here can also be done by touch.</p>
    </Card>
  </div>

  <section class="stack">
    <h2 class="section-title">Recent commands</h2>
    {#if error}<Notice tone="error" message={error} />{/if}
    {#if entries.length === 0 && !error}
      <p class="muted">No voice commands yet.</p>
    {:else}
      <Table unsorted ariaLabel="Recent voice commands">
        {#snippet header()}
          <TableHeaderCell label="Time" />
          <TableHeaderCell label="Who" />
          <TableHeaderCell label="Heard" />
          <TableHeaderCell label="Outcome" />
          <TableHeaderCell label="Reply" />
        {/snippet}
        {#each entries as e (e.id)}
          <tr>
            <td class="num">{dublinTime(e.at)}</td>
            <td>{e.user_name}</td>
            <td>“{e.transcript}”{#if e.source === "typed"}{" "}<span class="muted small">(typed)</span>{/if}</td>
            <td><Chip size="sm" uppercase={false} tone={OUTCOME_TONE[e.outcome] ?? "neutral"}>{OUTCOME_LABEL[e.outcome] ?? e.outcome}</Chip></td>
            <td>{e.reply}</td>
          </tr>
        {/each}
      </Table>
    {/if}
  </section>
</div>

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(min(100%, 380px), 1fr));
    gap: var(--space-5);
  }
  dl {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: var(--space-3) var(--space-5);
    align-items: center;
    margin: 0 0 var(--space-4);
  }
  dt {
    color: var(--text-secondary);
  }
  dd {
    margin: 0;
  }
  .commands {
    list-style: none;
    margin: 0 0 var(--space-4);
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }
  .commands li {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }
  .small {
    font-size: var(--font-size-sm);
  }
  p {
    margin: 0 0 var(--space-3);
  }
</style>
