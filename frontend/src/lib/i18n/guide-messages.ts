// The staff guides: one page each, in the words on the buttons. They are
// here, not in the page, so every line is in the list of text to translate.

export const GUIDES = [
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
        "No connection? Tap Keep on this device. It is sent when the connection is back: check the banner has gone.",
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
] as const;

/** GUIDE_MESSAGES is every line of the guides, for translation. */
export const GUIDE_MESSAGES = GUIDES.flatMap((g) => [g.title, g.who, ...g.steps, ...g.remember]);
