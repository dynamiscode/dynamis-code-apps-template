// Reader-facing labels for the item create chart.
// Model: verification/web/ItemCreate.tla (the item-create-submission skill).
export const title = "Adding an item";

const list = (set) => (Array.isArray(set) ? set : Object.values(set || {}));

export const state = (v) => {
  if (v.lost) return "A press was lost with no message";
  if (list(v.reqs).length) return "Saving (button disabled)";
  switch (v.msg) {
    case "rejected": return v.box === v.rejected ? "Error shown, your title kept" : "Error shown, box emptied";
    case "saved": return "New item listed, toast shown";
    case "failed": return "Request failed, toast shown, title kept";
    default: return v.box === "" ? "Empty form" : "Title typed";
  }
};

export const group = (v) => {
  if (v.lost) return "bug";
  if (v.msg === "rejected" || v.msg === "failed") return "error";
  if (v.msg === "saved") return "done";
  return "active";
};

export const colors = { bug: "pink", error: "orange", done: "teal", active: "blue" };

export const step = (action, from, to) => {
  if (action === "YouType" || action === "YouTypeAny") return to.box === "blank" ? "you type only spaces" : "you type a title";
  if (action === "YouPressAdd") return from.busy ? null : "you press Add";
  if (action === "ServerStep" || action === "ServerAnswers") return null;
  if (action === "FailStep" || action === "RequestFails") return null;
  if (action === "BrowserStep" || action === "ResponseArrives") {
    const gone = list(from.reqs).find((r) => !list(to.reqs).some((q) => q.el === r.el && q.title === r.title));
    switch (gone?.out) {
      case "ok": return "the item is saved";
      case "replay": return "the server repeats the saved answer";
      case "invalid": return "the server rejects the blank title";
      case "limit": return "the item limit is reached";
      case "conflict": return "the form key was already used";
      case "failed": return "the request fails";
      default: return "an answer arrives";
    }
  }
  if (action === "Idle") return null;
  return action;
};
