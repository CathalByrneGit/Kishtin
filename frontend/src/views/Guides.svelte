<script lang="ts">
  import LanguagePicker from "../components/LanguagePicker.svelte";
  import { GUIDES as GUIDE_LIST } from "../lib/i18n/guide-messages";
  import { t } from "../lib/i18n.svelte";
  import type { Msg } from "../lib/i18n/messages";
  import { Button, Card } from "@kenn-io/kit-ui";
  import { Printer } from "@lucide/svelte";

  // One-page guides to pin up in the kitchen, in the words on the buttons.

  let { restaurant = "" }: { restaurant?: string } = $props();

  interface Guide {
    id: string;
    title: Msg;
    who: Msg;
    steps: readonly Msg[];
    remember: readonly Msg[];
  }

  const GUIDES: readonly Guide[] = GUIDE_LIST;


  let printing = $state<string | null>(null);

  function print(id: string) {
    printing = id;
    setTimeout(() => {
      window.print();
      printing = null;
    }, 50);
  }
</script>

<div class="page guides" class:printing-one={printing !== null && printing !== "all"}>
  <header class="head">
    <div>
      <h1>{t("Staff guides")}</h1>
      <p>{t("One-page guides for the kitchen, in the words on the buttons. Print them and pin them up where the work happens, or show them to new staff.")}</p>
    </div>
    <div class="no-print side">
      <LanguagePicker />
      <Button tone="info" surface="solid" label={t("Print all")} onclick={() => print("all")}><Printer size={16} /></Button>
    </div>
  </header>

  <div class="cards">
    {#each GUIDES as g (g.id)}
      <section class="guide" class:hidden={printing !== null && printing !== "all" && printing !== g.id} aria-labelledby={`guide-${g.id}`}>
        <Card padding="md">
          <div class="guide-head">
            <div>
              <h2 id={`guide-${g.id}`}>{t(g.title)}</h2>
              <span class="who">{t(g.who)}</span>
            </div>
            <div class="no-print">
              <Button size="sm" label={t("Print")} ariaLabel={`${t("Print")}: ${t(g.title)}`} onclick={() => print(g.id)}><Printer size={14} /></Button>
            </div>
          </div>
          <ol>
            {#each g.steps as s, i (i)}
              <li>{t(s)}</li>
            {/each}
          </ol>
          <div class="remember">
            <strong>{t("Remember")}</strong>
            <ul>
              {#each g.remember as r, i (i)}
                <li>{t(r)}</li>
              {/each}
            </ul>
          </div>
          <p class="foot">{restaurant || "Kishtin"} · {t("Kishtin staff guide")}</p>
        </Card>
      </section>
    {/each}
  </div>
</div>

<style>
  .head {
    display: flex;
    justify-content: space-between;
    gap: var(--space-5);
    align-items: flex-start;
    margin-bottom: var(--space-5);
  }
  .side {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    align-items: flex-end;
  }
  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
    gap: var(--space-5);
  }
  .guide-head {
    display: flex;
    justify-content: space-between;
    gap: var(--space-3);
    align-items: flex-start;
  }
  h2 {
    font-size: var(--font-size-xl);
  }
  .who {
    color: var(--text-muted, inherit);
  }
  ol {
    margin: var(--space-4) 0;
    padding-left: var(--space-6);
    font-size: var(--font-size-lg);
    line-height: 1.5;
  }
  ol li + li {
    margin-top: var(--space-2);
  }
  .remember {
    border-top: 2px solid var(--border-default, currentColor);
    padding-top: var(--space-3);
  }
  .remember ul {
    margin: var(--space-2) 0 0;
    padding-left: var(--space-5);
  }
  .foot {
    display: none;
    margin-top: var(--space-5);
    font-size: var(--font-size-sm);
  }
  @media print {
    :global(.kit-app-sidebar),
    :global(.top-bar),
    .no-print,
    .head p,
    .hidden {
      display: none !important;
    }
    .printing-one .head {
      display: none;
    }
    .cards {
      display: block;
    }
    .guide {
      break-after: page;
    }
    .guide:last-child {
      break-after: auto;
    }
    h2 {
      font-size: 2rem;
    }
    ol {
      font-size: 1.5rem;
      padding-left: 2.5rem;
    }
    .remember {
      font-size: 1.15rem;
    }
    .foot {
      display: block;
    }
  }
</style>
