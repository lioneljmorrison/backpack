// Keep a page current. The page listens to its workspace's changes, and
// after one of the kinds it shows it fetches itself again and swaps in the
// new content. Events name only what changed; the page is the truth, so a
// missed event costs nothing.
//
// It never replaces what a person is doing: anything typed, chosen or
// unticked keeps the page as it is and shows a notice instead, and a
// refresh waits for a drag to end.
document.addEventListener("DOMContentLoaded", () => {
  const main = document.querySelector("main[data-live]");
  if (!main || !window.EventSource) return;
  const kinds = new Set(main.dataset.liveKinds.split(" "));
  let dirty = false;
  let waitingForDrop = false;
  let timer = null;

  main.addEventListener("input", () => { dirty = true; });
  main.addEventListener("change", () => { dirty = true; });

  const dragging = () => document.querySelector(".dragging") !== null;

  function notice() {
    if (document.getElementById("live-notice")) return;
    const p = document.createElement("p");
    p.id = "live-notice";
    p.className = "notice";
    p.setAttribute("role", "status");
    const reload = document.createElement("a");
    reload.href = location.href;
    reload.textContent = "reload to see it";
    p.append("This page has changed — ", reload, ".");
    main.prepend(p);
  }

  // held says whether the page must stay as it is for now.
  function held() {
    if (dirty) {
      notice();
      return true;
    }
    if (dragging()) {
      waitingForDrop = true;
      return true;
    }
    return false;
  }

  async function refresh() {
    if (held()) return;
    let html;
    try {
      const resp = await fetch(location.href, { headers: { Accept: "text/html" } });
      // A redirect is to sign in: the token is gone, so leave the page be.
      if (!resp.ok || resp.redirected) return;
      html = await resp.text();
    } catch {
      return;
    }
    if (held()) return;
    const fresh = new DOMParser().parseFromString(html, "text/html").querySelector("main");
    if (!fresh) return;
    // Keep a refusal the person hasn't read, which the fresh page won't have.
    const error = main.querySelector(":scope > .error");
    const open = new Set([...main.querySelectorAll("details[open]")].map((d) => d.className));
    const x = window.scrollX;
    const y = window.scrollY;
    const boardX = main.querySelector(".board")?.scrollLeft;
    // board.js notes where the cards are before they are swapped out.
    document.dispatchEvent(new Event("backpack:refreshing"));
    main.replaceChildren(...fresh.childNodes);
    if (error && !main.querySelector(":scope > .error")) main.prepend(error);
    for (const d of main.querySelectorAll("details")) d.open = open.has(d.className);
    window.scrollTo(x, y);
    // The board scrolls sideways on its own, apart from the window.
    const board = main.querySelector(".board");
    if (board && boardX) board.scrollLeft = boardX;
    document.dispatchEvent(new Event("backpack:refreshed"));
  }

  // Several changes close together are one refresh.
  function soon() {
    clearTimeout(timer);
    timer = setTimeout(refresh, 150);
  }

  document.addEventListener("dragend", () => {
    if (waitingForDrop) {
      waitingForDrop = false;
      soon();
    }
  });

  let lost = false;
  const source = new EventSource(main.dataset.live);
  source.addEventListener("change", (e) => {
    try {
      if (kinds.has(JSON.parse(e.data).kind)) soon();
    } catch {
      // Not an event this page understands.
    }
  });
  source.addEventListener("error", () => { lost = true; });
  // Back after a drop: changes may have been missed, so catch up.
  source.addEventListener("open", () => {
    if (lost) {
      lost = false;
      soon();
    }
  });
});
