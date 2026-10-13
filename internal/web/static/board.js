// Drag a card to a column to move it there. The page only lights up the
// columns the server says the card may go to (its data-moves) and takes a
// drop there; the board still checks the move, and a refusal is shown on the
// reloaded board. Backlog cards also drag within the backlog to re-rank, and
// a card dropped into the backlog lands where it was dropped: the top half of
// a card puts it before that card, the bottom half after it.
//
// Moves that need more than a drop (a reason, a branch, a pull request link,
// a merge commit) are not taken yet: needsPrompt says which they are.
//
// Listeners sit on the document, so they survive live.js swapping in a
// fresh board.
(function () {
  let dragged = null;

  // needsPrompt says whether moving a card from one status to another asks
  // for something first. A held story resumes without being asked.
  function needsPrompt(from, to) {
    if (from === "on_hold") return to === "closed";
    if (to === "in_progress") return from === "backlog";
    return ["on_hold", "closed", "in_review", "merged"].includes(to);
  }

  // canDrop says whether the dragged card may be dropped on the column.
  function canDrop(column) {
    if (!dragged || !column) return false;
    const from = dragged.dataset.status;
    const to = column.dataset.status;
    if (from === "backlog" && to === "backlog") return dragged.dataset.move !== undefined;
    return (dragged.dataset.moves || "").split(" ").includes(to) && !needsPrompt(from, to);
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
    if (dragged) dragged.classList.remove("dragging");
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
    submit(card.dataset.statusUrl, { ...fields, to, back: "board" });
  });
})();
