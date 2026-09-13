// @vitest-environment happy-dom
import { describe, expect, it } from "vitest";
import { isCollectionEditGesture } from "../src/collection/edit-interaction";

describe("collection editing gesture", () => {
  it.each([
    "button",
    "a",
    "input",
    "select",
    "textarea",
    "summary",
    '[role="checkbox"]',
    '[contenteditable="true"]',
  ])("does not steal interaction from %s or its descendants", (selector) => {
    const parent = document.createElement("article");
    const control = document.createElement(
      selector.startsWith("[") ? "div" : selector,
    );
    if (selector.startsWith("[role")) control.setAttribute("role", "checkbox");
    if (selector.startsWith("[contenteditable"))
      control.setAttribute("contenteditable", "true");
    const icon = document.createElement("span");
    control.append(icon);
    parent.append(control);
    let edits = 0;
    parent.addEventListener("dblclick", (event) => {
      if (isCollectionEditGesture(event)) edits++;
    });
    icon.dispatchEvent(new MouseEvent("dblclick", { bubbles: true }));
    expect(edits).toBe(0);
  });
  it("allows content but respects prevented gestures", () => {
    const content = document.createElement("p");
    const results: boolean[] = [];
    content.addEventListener("dblclick", (event) =>
      results.push(isCollectionEditGesture(event)),
    );
    content.dispatchEvent(new MouseEvent("dblclick", { bubbles: true }));
    const cancelled = new MouseEvent("dblclick", { cancelable: true });
    cancelled.preventDefault();
    content.dispatchEvent(cancelled);
    expect(results).toEqual([true, false]);
  });
});
