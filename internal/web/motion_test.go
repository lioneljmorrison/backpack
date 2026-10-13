package web

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// motionHarness runs board.js against a stand-in page (invented cards, no
// browser) and prints one line per check that failed.
const motionHarness = `
const fs = require("fs");
const vm = require("vm");
const listeners = {};
const fails = [];
const check = (ok, what) => { if (!ok) fails.push(what); };

function makeCard(id, status, x, y, extra) {
  const rect = { left: x, top: y, width: 100, height: 40 };
  const el = {
    dataset: { id, status, statusUrl: "/w/d/stories/" + id + "/status", moves: "in_progress backlog", ...extra },
    classList: { add() {}, remove() {} },
    animations: [],
    rect,
    getBoundingClientRect() { return { ...this.rect }; },
    animate(frames) { this.animations.push(frames); return { finished: Promise.resolve() }; },
    closest(sel) { return sel.startsWith(".card") ? this : null; },
  };
  return el;
}
const a = makeCard("S-1", "backlog", 0, 0, { move: "/m/S-1" });
const b = makeCard("S-2", "backlog", 0, 50, { move: "/m/S-2" });
const column = {
  dataset: { status: "backlog" },
  getAttribute() { return "In progress"; },
  contains() { return true; },
  querySelectorAll() { return []; },
};
const board = {
  scrollLeft: 0, scrollTop: 0,
  getBoundingClientRect() { return { left: 0, top: 0 }; },
  querySelectorAll() { return [a, b]; },
};
const document = {
  addEventListener(name, fn) { (listeners[name] ||= []).push(fn); },
  dispatchEvent(ev) {
    for (const fn of listeners[ev.type] || []) fn(ev);
    return !ev.defaultPrevented;
  },
  querySelector(sel) { return sel === "main .board" ? board : null; },
  querySelectorAll() { return []; },
  getElementById() { return null; },
  createElement() { return { append() {}, submit() {} }; },
  body: { append() {} },
};
class Ev { constructor(type, init) { this.type = type; Object.assign(this, init || {}); this.defaultPrevented = false; }
  preventDefault() { this.defaultPrevented = true; } }
class CEv extends Ev { constructor(type, init) { super(type, init); this.detail = init && init.detail; this.cancelable = true; } }
const ctx = { document, Event: Ev, CustomEvent: CEv, URLSearchParams, window: { matchMedia: () => ({ matches: false }) } };
vm.runInNewContext(fs.readFileSync(process.argv[2], "utf8"), ctx);

const fire = (ev) => document.dispatchEvent(ev);

// A drop that reorders the backlog sends a move that names the dropped story.
let sent = null;
document.addEventListener("backpack:send", (e) => { sent = e.detail; e.preventDefault(); });
fire(new Ev("dragstart", { target: a, dataTransfer: { setData() {} } }));
const over = { closest: (sel) => (sel.startsWith(".column") ? column : b) };
fire(new Ev("drop", { target: over, clientY: 0 }));
check(sent && sent.dropped === "S-1", "a drop names the dropped story, got " + JSON.stringify(sent && sent.dropped));

// A merged card may be dropped only on the merged column of the branch its
// data-moves names.
const merged = makeCard("S-3", "merged", 0, 100, { moves: "merged:main" });
const mergedCol = (into) => ({ dataset: { status: "merged", into }, getAttribute() { return "Merged (" + into + ")"; },
  querySelectorAll() { return []; }, classList: { add() {}, remove() {} } });
const hover = (col) => {
  const ev = new Ev("dragover", { target: { closest: () => col }, dataTransfer: {}, clientY: 0 });
  fire(new Ev("dragstart", { target: merged, dataTransfer: { setData() {} } }));
  fire(ev);
  fire(new Ev("dragend"));
  return ev.defaultPrevented;
};
check(hover(mergedCol("main")), "a merged card may be dropped on the next branch's column");
check(!hover(mergedCol("dev")), "a merged card must not be dropped on another branch's column");

// An arrow button's move names no dropped story.
sent = null;
const form = { matches: () => true, getAttribute: () => "/w/d/stories/S-1/status" };
ctx.FormData = function () { return []; };
fire(new Ev("submit", { target: form }));
check(sent && !sent.dropped, "an arrow move must not name a dropped story");

// Redraw after a drop: the dropped card stays put, its neighbour slides.
function redraw(detail) {
  a.animations.length = 0; b.animations.length = 0;
  fire(new CEv("backpack:refreshing", { detail }));
  a.rect = { left: 200, top: 100, width: 100, height: 40 };
  b.rect = { left: 0, top: 0, width: 100, height: 40 };
  fire(new Ev("backpack:refreshed"));
  a.rect = { left: 0, top: 0, width: 100, height: 40 };
  b.rect = { left: 0, top: 50, width: 100, height: 40 };
}
redraw({ dropped: "S-1" });
check(a.animations.length === 0, "dropped card must not slide");
check(b.animations.length === 1, "a neighbour still slides after a drop");
redraw({});
check(a.animations.length === 1, "a card moved by an arrow or live refresh still slides");
redraw(undefined);
check(a.animations.length === 1, "a refresh with no detail still slides");

console.log(fails.join("\n"));
`

func TestADroppedCardLandsInPlaceWhileOtherMovesStillSlide(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	dir := t.TempDir()
	harness := filepath.Join(dir, "harness.js")
	if err := os.WriteFile(harness, []byte(motionHarness), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, harness, filepath.Join("static", "board.js")).CombinedOutput()
	if err != nil {
		t.Fatalf("harness failed: %v\n%s", err, out)
	}
	if len(out) > 1 {
		t.Fatalf("board.js:\n%s", out)
	}
}
