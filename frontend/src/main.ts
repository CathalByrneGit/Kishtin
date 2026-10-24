import "@kenn-io/kit-ui/theme.css";
import "./app.css";
import { initTheme, setHighContrast, setThemeMode } from "@kenn-io/kit-ui";
import { mount } from "svelte";
import App from "./App.svelte";
import { load } from "./lib/storage";

// Kitchen screens start light and high contrast. A device can change both
// in Setup; the theme store remembers the choice.
const firstRun = load("kit-ui-theme") === null;
initTheme();
if (firstRun) {
  setThemeMode("light");
  setHighContrast(true);
}

const target = document.getElementById("app");
if (!target) throw new Error("missing #app");
mount(App, { target });
