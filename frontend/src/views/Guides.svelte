<script lang="ts">
  import { Button, Card } from "@kenn-io/kit-ui";
  import { Printer } from "@lucide/svelte";

  // One-page guides to pin up in the kitchen, in the words on the buttons.

  let { restaurant = "" }: { restaurant?: string } = $props();

  interface Guide {
    id: string;
    title: string;
    who: string;
    steps: string[];
    remember: string[];
  }

  const GUIDES: Guide[] = [
    {
      id: "login",
      title: "Logging in",
      who: "Everyone",
      steps: ["Tap your name.", "Type your PIN.", "Tap Log in."],
      remember: [
        "Always log in as yourself: every record shows who did it.",
        "Finished? Open the menu (☰) and switch staff, so the next person logs in as themselves.",
        "Forgot your PIN? Ask a manager to set a new one.",
      ],
    },
    {
      id: "fridge",
      title: "Fridge and freezer checks",
      who: "Everyone, at every check time",
      steps: ["Tap Log check.", "Tap Fridge / freezer.", "Choose the fridge or freezer.", "Type the temperature, such as 3.4.", "Tap Save record."],
      remember: [
        "Fridges 5°C or below. Freezers −18°C or colder.",
        "Read the thermometer. Never guess or round down.",
        "Today shows each check as Due, done, or Missed.",
      ],
    },
    {
      id: "fail",
      title: "If a reading fails",
      who: "Everyone",
      steps: [
        "Kishtin shows Fail in red (or Borderline in amber).",
        "Deal with it first: move food to a working fridge, bin high-risk food, or call the manager.",
        "Write what you did under Corrective action, or tap one of the suggestions.",
        "Tap Save record.",
        "Tell the head chef or manager.",
      ],
      remember: ["A failed reading cannot be saved without an action.", "Never change a temperature to make it pass. Records cannot be changed later."],
    },
    {
      id: "delivery",
      title: "Deliveries",
      who: "Whoever takes the delivery",
      steps: [
        "Probe chilled and frozen food before the driver leaves.",
        "Tap Log check, then Delivery.",
        "Enter the supplier and product. Choose Chilled, Frozen or Ambient.",
        "Type the probe temperature. Tick Packaging intact and In date if they are.",
        "Tap Add docket photo and photograph the docket.",
        "Tap Save record.",
      ],
      remember: [
        "Chilled 5°C or below. Frozen −18°C or colder.",
        "If Kishtin says reject, do not accept it: send it back with the driver.",
      ],
    },
    {
      id: "cooking",
      title: "Cooking and cooling",
      who: "Cooks",
      steps: [
        "Probe the thickest part of the food.",
        "Tap Log check, then Cook / reheat. Enter the food and the Core temperature. Tap Save record.",
        "When cooked food goes in the fridge to cool: Log check, Cooling. Enter when it Finished cooking and when it went Into fridge or blast chiller.",
      ],
      remember: ["Cooking 75°C or above at the core. Reheating 70°C or above.", "Cooked food into the fridge within 2 hours."],
    },
    {
      id: "cleaning",
      title: "Cleaning",
      who: "Everyone",
      steps: ["Open Today (on a phone, the Today tab).", "When a task is done, tick it.", "Weekly and longer tasks show when they are due. Tick them the same way."],
      remember: ["Only tick what is really done.", "Overdue tasks show in red: do them first."],
    },
    {
      id: "station",
      title: "The station screen",
      who: "Cooks",
      steps: [
        "Start now lists what to put on, and why.",
        "Tap Start when it goes on.",
        "Tap Done cooking, then Done resting, as it moves on.",
        "Tap Ready when it is plated at the pass.",
      ],
      remember: ["Tapped the wrong one? Recently done, at the bottom, puts it back.", "Allergy tickets are marked red and list what each dish contains."],
    },
    {
      id: "voice",
      title: "Voice commands",
      who: "Anyone with busy hands",
      steps: [
        "Hold the Hold to talk button (or the space bar) and speak, then let go.",
        "Say, for example: “Fire table 4 mains”, “Burgers on, tables 4 and 7”, “Walk-in fridge 3.4 degrees”.",
        "Kishtin says back what it did.",
      ],
      remember: ["Voids and failed readings need you to say “confirm”.", "If it asks “table 4 or table 14?”, answer it. It never guesses."],
    },
    {
      id: "allergens",
      title: "When a customer asks about allergens",
      who: "Front of house and kitchen",
      steps: [
        "Open the allergen chart: Setup, Kitchen, Allergens, or the printed copy.",
        "Find the dish. Yes means it contains that allergen.",
        "If it says Not checked, do not say it is safe: ask the head chef or manager.",
        "Mark the order Allergy, with the allergy in the notes, so the kitchen sees it.",
      ],
      remember: ["Never guess. If in doubt, say you are not sure.", "Specials and changes: check with the kitchen every time."],
    },
  ];

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
      <h1>Staff guides</h1>
      <p>One-page guides for the kitchen, in the words on the buttons. Print them and pin them up where the work happens, or show them to new staff.</p>
    </div>
    <div class="no-print">
      <Button tone="info" surface="solid" label="Print all" onclick={() => print("all")}><Printer size={16} /></Button>
    </div>
  </header>

  <div class="cards">
    {#each GUIDES as g (g.id)}
      <section class="guide" class:hidden={printing !== null && printing !== "all" && printing !== g.id} aria-labelledby={`guide-${g.id}`}>
        <Card padding="md">
          <div class="guide-head">
            <div>
              <h2 id={`guide-${g.id}`}>{g.title}</h2>
              <span class="who">{g.who}</span>
            </div>
            <div class="no-print">
              <Button size="sm" label="Print" ariaLabel={`Print ${g.title}`} onclick={() => print(g.id)}><Printer size={14} /></Button>
            </div>
          </div>
          <ol>
            {#each g.steps as s, i (i)}
              <li>{s}</li>
            {/each}
          </ol>
          <div class="remember">
            <strong>Remember</strong>
            <ul>
              {#each g.remember as r, i (i)}
                <li>{r}</li>
              {/each}
            </ul>
          </div>
          <p class="foot">{restaurant || "Kishtin"} · Kishtin staff guide</p>
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
