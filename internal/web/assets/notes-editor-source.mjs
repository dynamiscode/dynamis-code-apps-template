import { createEditor } from "markora/minimal";
import "markora/minimal.css";
import { wireNotesEditor } from "./notes-editor-core.mjs";

document.addEventListener("DOMContentLoaded", () => {
	for (const root of document.querySelectorAll("[data-notes-editor]")) wireNotesEditor(root, createEditor);
});
