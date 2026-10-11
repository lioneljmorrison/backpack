// Drag a backlog card onto another to place it there. It submits the same
// move a card's arrow buttons do, so the board checks it the same way.
document.addEventListener("DOMContentLoaded", () => {
  let dragged = null;
  const cards = document.querySelectorAll("[data-move]");
  for (const card of cards) {
    card.addEventListener("dragstart", (e) => {
      dragged = card;
      e.dataTransfer.effectAllowed = "move";
      card.classList.add("dragging");
    });
    card.addEventListener("dragend", () => {
      card.classList.remove("dragging");
      for (const c of cards) c.classList.remove("drop-above", "drop-below");
    });
    card.addEventListener("dragover", (e) => {
      if (!dragged || dragged === card) return;
      e.preventDefault();
      const above = e.offsetY < card.offsetHeight / 2;
      card.classList.toggle("drop-above", above);
      card.classList.toggle("drop-below", !above);
    });
    card.addEventListener("dragleave", () => card.classList.remove("drop-above", "drop-below"));
    card.addEventListener("drop", (e) => {
      if (!dragged || dragged === card) return;
      e.preventDefault();
      const form = document.createElement("form");
      form.method = "post";
      form.action = dragged.dataset.move;
      const field = document.createElement("input");
      field.type = "hidden";
      field.name = card.classList.contains("drop-above") ? "before" : "after";
      field.value = card.dataset.id;
      form.append(field);
      document.body.append(form);
      form.submit();
    });
  }
});
