// Keep a page current. The page listens to its workspace's changes, and
// after one of the kinds it shows it fetches itself again and swaps in the
// new content. Events name only what changed; the page is the truth, so a
// missed event costs nothing.
//
// It never replaces what a person is doing: anything typed, chosen or
// unticked keeps the page as it is and shows a notice instead, and a
// refresh waits for a drag to end or a move of the person's own to be sent.
document.addEventListener("DOMContentLoaded", () => {
  const main = document.querySelector("main[data-live]");
  if (!main || !window.EventSource) return;
  const kinds = new Set(main.dataset.liveKinds.split(" "));
  let dirty = false;
  let waitingForDrop = false;
  let timer = null;
  let sending = false;
  let sent = 0;

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
    if (sending) return true;
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

  // swap puts the fresh page's content in place of the current one, keeping
  // what the person has open and where they have scrolled. It says whether
  // the page had a main to swap in. keepError holds on to a refusal the
  // person hasn't read, which a background refresh's page won't have.
  function swap(html, { keepError }) {
    const fresh = new DOMParser().parseFromString(html, "text/html").querySelector("main");
    if (!fresh) return false;
    const error = keepError ? main.querySelector(":scope > .error") : null;
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
    return true;
  }

  async function refresh() {
    if (held()) return;
    const started = sent;
    let html;
    try {
      const resp = await fetch(location.href, { headers: { Accept: "text/html" } });
      // A redirect is to sign in: the token is gone, so leave the page be.
      if (!resp.ok || resp.redirected) return;
      html = await resp.text();
    } catch {
      return;
    }
    // A move sent meanwhile makes this board older than the move's reply.
    if (started !== sent || held()) return;
    swap(html, { keepError: true });
  }

  // A person's own move (see board.js) is sent in the background and the
  // board it comes back with is swapped in like a live refresh, so cards
  // slide instead of the page reloading. Left alone, the sender falls back
  // to a plain form post: that is what a dirty page does, so nothing typed
  // is lost to a swap.
  async function send({ action, body }) {
    sending = true;
    sent++;
    try {
      // fetch follows the redirect back to the board, so the reply is the
      // fresh board, with a refusal in it if the board refused.
      const resp = await fetch(action, { method: "POST", body, headers: { Accept: "text/html" } });
      // Anything other than the same page, such as sign-in, is the browser's to show.
      if (!resp.ok || new URL(resp.url).pathname !== location.pathname) {
        location.assign(resp.url);
        return;
      }
      if (!swap(await resp.text(), { keepError: false })) location.assign(resp.url);
    } catch {
      // Not known whether the move happened, so it is never sent again.
      const p = document.createElement("p");
      p.className = "error";
      p.setAttribute("role", "alert");
      p.textContent = "The board is unavailable. Try again shortly.";
      main.querySelector(":scope > .error")?.remove();
      main.prepend(p);
    } finally {
      sending = false;
      soon();
    }
  }

  document.addEventListener("backpack:send", (e) => {
    if (dirty) return;
    e.preventDefault();
    send(e.detail);
  });

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
