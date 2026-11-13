// Reader-facing labels for the draft chart. Model: verification/web/FormDrafts.tla
// (the form-drafts skill).
export const title = "A form that keeps your unsent text";

export const state = (v) => {
  if (v.page === "away") return v.stored === "none" ? "Left the page, nothing unsent" : "Left the page, draft kept in this tab";
  if (v.submitting) return "Sending";
  if (v.msg === "saved") return "Saved, form empty";
  if (v.msg === "error") return "Error shown, your text kept";
  if (v.box === "") return "Empty form";
  return "Text typed, draft kept";
};

export const group = (v) => (v.msg === "saved" ? "done" : v.msg === "error" ? "error" : v.page === "away" ? "away" : "active");
export const colors = { done: "teal", error: "orange", away: "pink", active: "blue" };

export const step = (action) => ({
  YouType: "you type",
  YouClear: "you clear the field",
  YouSubmit: "you submit",
  EventWhileSubmitting: null,
  ServerSaves: "the server saves it",
  ServerRejects: "the server rejects it",
  YouLeave: "you leave (warned if unsaved)",
  YouComeBack: "you come back",
  YouDiscard: "you press Discard changes",
}[action] ?? action);
