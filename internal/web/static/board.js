// Drag a card to a column to move it there. The page only lights up the
// columns the server says the card may go to (its data-moves) and takes a
// drop there; the board still checks the move, and a refusal is shown on the
// reloaded board. Backlog cards also drag within the backlog to re-rank, and
// a card dropped into the backlog lands where it was dropped: the top half of
// a card puts it before that card, the bottom half after it.
//
// Moves that need more than a drop (a reason, a branch, a pull request link,
// a merge commit) ask for it in a small prompt first: promptFor says what each
// asks. Cancelling sends nothing and the card stays where it was.
//
// When live.js swaps in a fresh board, cards that moved slide to their new
// place, new cards fade in and cards that left fade out, unless the person
// asks for reduced motion.
//
// Listeners sit on the document, so they survive live.js swapping in a
// fresh board.
(function () {
  const MOTION_MS = 500;
  const CARDS = "main .board .card[data-id]";
  let dragged = null;
  let prompting = false;

  // promptFor says what moving a card from one status to another asks for
  // first: the fields, each optional unless marked required. Empty means the
  // drop goes straight through. A held story resumes without being asked.
  function promptFor(from, to, blocked) {
    if (to === "on_hold" || to === "closed") return [{ name: "reason", required: true }];
    if (to === "in_progress" && from === "backlog") {
      const fields = [{ name: "branch" }];
      if (blocked) fields.unshift({ name: "workaround", required: true });
      return fields;
    }
    if (to === "in_review") return [{ name: "pr_url" }];
    if (to === "merged") return [{ name: "commit" }];
    return [];
  }

  // canDrop says whether the dragged card may be dropped on the column.
  function canDrop(column) {
    if (!dragged || !column) return false;
    const from = dragged.dataset.status;
    const to = column.dataset.status;
    if (from === "backlog" && to === "backlog") return dragged.dataset.move !== undefined;
    return (dragged.dataset.moves || "").split(" ").includes(to);
  }

  function clear() {
    for (const el of document.querySelectorAll(".drop-ok, .drop-over, .drop-above, .drop-below")) {
      el.classList.remove("drop-ok", "drop-over", "drop-above", "drop-below");
    }
  }

  // aim says where in the backlog a drop at the pointer lands: a card and
  // whether above it, or no card when it lands after the last or in an empty
  // backlog.
  function aim(column, e) {
    const card = e.target.closest(".card[data-id]");
    if (card && column.contains(card) && card !== dragged) {
      const box = card.getBoundingClientRect();
      return { card, above: e.clientY < box.top + box.height / 2 };
    }
    const others = [...column.querySelectorAll(".card[data-id]")].filter((c) => c !== dragged);
    return { card: others[others.length - 1] || null, above: false, end: true };
  }

  function mark(column, e) {
    for (const el of column.querySelectorAll(".drop-above, .drop-below")) el.classList.remove("drop-above", "drop-below");
    if (column.dataset.status !== "backlog") return;
    const at = aim(column, e);
    if (at.card) at.card.classList.add(at.above ? "drop-above" : "drop-below");
  }

  function submit(action, fields) {
    const form = document.createElement("form");
    form.method = "post";
    form.action = action;
    for (const [name, value] of Object.entries(fields)) {
      const input = document.createElement("input");
      input.type = "hidden";
      input.name = name;
      input.value = value;
      form.append(input);
    }
    document.body.append(form);
    form.submit();
  }

  document.addEventListener("dragstart", (e) => {
    const card = e.target.closest && e.target.closest(".card[draggable='true']");
    if (!card) return;
    dragged = card;
    e.dataTransfer.effectAllowed = "move";
    e.dataTransfer.setData("text/plain", card.dataset.id);
    card.classList.add("dragging");
    for (const column of document.querySelectorAll(".column[data-status]")) {
      if (canDrop(column)) column.classList.add("drop-ok");
    }
  });

  document.addEventListener("dragend", () => {
    // While the prompt is open the card keeps .dragging, which holds back
    // live refreshes; closing the prompt lets go of it.
    if (dragged && !prompting) dragged.classList.remove("dragging");
    dragged = null;
    clear();
  });

  document.addEventListener("dragover", (e) => {
    const column = e.target.closest && e.target.closest(".column[data-status]");
    if (!canDrop(column)) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    for (const c of document.querySelectorAll(".drop-over")) c.classList.remove("drop-over");
    column.classList.add("drop-over");
    mark(column, e);
  });

  document.addEventListener("drop", (e) => {
    const column = e.target.closest && e.target.closest(".column[data-status]");
    if (!canDrop(column)) return;
    e.preventDefault();
    const card = dragged;
    const to = column.dataset.status;
    const fields = {};
    if (to === "backlog") {
      const at = aim(column, e);
      if (at.card) fields[at.above ? "before" : "after"] = at.card.dataset.id;
    }
    clear();
    if (card.dataset.status === "backlog" && to === "backlog") {
      // Reordering: nothing to do when dropped beside itself or with nowhere to go.
      if (!fields.before && !fields.after) return;
      submit(card.dataset.move, fields);
      return;
    }
    const asks = promptFor(card.dataset.status, to, card.dataset.blocked !== undefined);
    if (asks.length === 0) {
      submit(card.dataset.statusUrl, { ...fields, to, back: "board" });
      return;
    }
    ask(card, column, to, asks);
  });

  // ask opens the prompt for a move and, on Move, sends the same status form
  // a plain drop does, with what was typed. The card may be gone after a live
  // refresh, so only its address and id are kept.
  function ask(card, column, to, asks) {
    const dialog = document.getElementById("move-prompt");
    const form = document.getElementById("move-form");
    if (!dialog || !dialog.showModal) return;
    const url = card.dataset.statusUrl;
    prompting = true;
    document.getElementById("move-title").textContent = card.dataset.id + " to " + column.getAttribute("aria-label");
    for (const field of form.querySelectorAll("[data-field]")) {
      const ask = asks.find((a) => a.name === field.dataset.field);
      const input = field.querySelector("input");
      field.hidden = !ask;
      input.disabled = !ask;
      input.required = !!(ask && ask.required);
      input.value = "";
    }
    form.onsubmit = (e) => {
      e.preventDefault();
      const fields = { to, back: "board" };
      for (const input of form.querySelectorAll("input:not(:disabled)")) {
        if (input.value.trim() !== "") fields[input.name] = input.value.trim();
      }
      submit(url, fields);
    };
    dialog.onclick = (e) => { if (e.target === dialog) dialog.close(); };
    form.querySelector("[data-cancel]").onclick = () => dialog.close();
    dialog.onclose = () => {
      prompting = false;
      for (const el of document.querySelectorAll(".dragging")) el.classList.remove("dragging");
      // Let live.js run a refresh it held back while the prompt was open.
      document.dispatchEvent(new Event("dragend"));
    };
    dialog.showModal();
    const first = form.querySelector("input:not(:disabled)");
    if (first) first.focus();
  }

  // Cards are measured against the board's own content, not the window, so
  // a card that only shifted because something above the board changed size
  // does not slide. getBoundingClientRect includes a running animation's
  // transform, so a redraw mid-slide continues from where the card is seen.
  let before = null;

  function snapshot() {
    const board = document.querySelector("main .board");
    if (!board) return null;
    const origin = board.getBoundingClientRect();
    const cards = new Map();
    for (const el of board.querySelectorAll(CARDS)) {
      const r = el.getBoundingClientRect();
      cards.set(el.dataset.id, {
        el,
        view: r,
        x: r.left - origin.left + board.scrollLeft,
        y: r.top - origin.top + board.scrollTop,
      });
    }
    return cards;
  }

  const motion = { duration: MOTION_MS, easing: "ease-out" };

  document.addEventListener("backpack:refreshing", () => {
    before = snapshot();
  });

  document.addEventListener("backpack:refreshed", () => {
    const old = before;
    before = null;
    if (!old || window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    const now = snapshot();
    if (!now) return;
    for (const [id, card] of now) {
      const was = old.get(id);
      if (!was) {
        card.el.animate([{ opacity: 0 }, { opacity: 1 }], motion);
        continue;
      }
      const dx = was.x - card.x;
      const dy = was.y - card.y;
      if (Math.abs(dx) < 1 && Math.abs(dy) < 1) continue;
      card.el.animate([{ transform: "translate(" + dx + "px, " + dy + "px)" }, { transform: "none" }], motion);
    }
    // A card that left the board fades out where it was, as a copy.
    for (const [id, card] of old) {
      if (now.has(id)) continue;
      const ghost = card.el.cloneNode(true);
      ghost.removeAttribute("draggable");
      ghost.setAttribute("aria-hidden", "true");
      Object.assign(ghost.style, {
        position: "fixed",
        left: card.view.left + "px",
        top: card.view.top + "px",
        width: card.view.width + "px",
        height: card.view.height + "px",
        margin: "0",
        pointerEvents: "none",
      });
      document.body.append(ghost);
      ghost.animate([{ opacity: 1 }, { opacity: 0 }], motion).finished.then(
        () => ghost.remove(),
        () => ghost.remove(),
      );
    }
  });
})();
