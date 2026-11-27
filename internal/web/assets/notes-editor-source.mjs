import { createEditor } from "markora/minimal";
import "markora/minimal.css";
import { wireNotesEditor } from "./notes-editor-core.mjs";

const wired = new WeakSet();
const wireEditors = () => {
	for (const root of document.querySelectorAll("[data-notes-editor]")) {
		if (wired.has(root)) continue;
		wired.add(root);
		wireNotesEditor(root, createEditor);
	}
};

document.addEventListener("DOMContentLoaded", wireEditors);
document.addEventListener("htmx:after:swap", wireEditors);
