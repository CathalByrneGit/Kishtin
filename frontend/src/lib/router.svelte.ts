// A small History API router. Links are ordinary <a href> elements (the
// sidebar renders its own), so same-origin clicks are intercepted and the
// address bar updated without a page load. The server answers every app
// path with index.html.

export const route = $state({ path: location.pathname, search: location.search });

/** navigate moves to an app path such as "/records?range=7". */
export function navigate(href: string, replace = false): void {
  const u = new URL(href, location.origin);
  if (u.pathname + u.search === route.path + route.search) return;
  if (replace) history.replaceState(null, "", u.pathname + u.search);
  else history.pushState(null, "", u.pathname + u.search);
  route.path = u.pathname;
  route.search = u.search;
}

/** param reads a query parameter from the current address. */
export function param(name: string): string | null {
  return new URLSearchParams(route.search).get(name);
}

/** setParam changes one query parameter, replacing the history entry. */
export function setParam(name: string, value: string | null): void {
  const q = new URLSearchParams(route.search);
  if (value === null || value === "") q.delete(name);
  else q.set(name, value);
  const s = q.toString();
  navigate(route.path + (s ? `?${s}` : ""), true);
}

/** initRouter starts listening to link clicks and the back button. */
export function initRouter(): () => void {
  const onClick = (e: MouseEvent) => {
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    const a = (e.target as Element | null)?.closest("a");
    if (!a || a.target || a.hasAttribute("download")) return;
    const u = new URL(a.href, location.href);
    if (u.origin !== location.origin || u.pathname.startsWith("/api/")) return;
    e.preventDefault();
    navigate(u.pathname + u.search);
  };
  const onPop = () => {
    route.path = location.pathname;
    route.search = location.search;
  };
  document.addEventListener("click", onClick);
  window.addEventListener("popstate", onPop);
  return () => {
    document.removeEventListener("click", onClick);
    window.removeEventListener("popstate", onPop);
  };
}
