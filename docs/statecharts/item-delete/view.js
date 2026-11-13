// Reader-facing labels for the delete chart. Model: verification/web/ItemDelete.tla
// (the async-feedback skill).
export const title = "Deleting an item, shown at once";

export const state = (v) => {
  if (v.dialog) return "Confirm dialog open";
  if (v.row === "gone") return "Item gone, toast shown";
  if (v.row === "hidden") return "Row hidden while deleting";
  if (v.msg === "staleAlert") return "Row back, changed-item alert";
  if (v.msg === "failedToast") return "Row back, error toast";
  return "Item listed";
};

export const group = (v) => (v.row === "gone" ? "done" : v.msg === "none" ? "active" : "error");
export const colors = { done: "teal", error: "orange", active: "blue" };

export const step = (action, from, to) => {
  switch (action) {
    case "YouPressDelete": return "you press Delete";
    case "YouCancel": return "you cancel";
    case "YouConfirm": return "you confirm";
    case "ServerAnswers": return null;
    case "OtherUserDeletes": return null;
    case "AnswerArrives":
      return to.row === "gone" ? "the delete succeeds" : to.msg === "staleAlert" ? "the item changed meanwhile (412)" : "the request fails";
    case "Done": return null;
    default: return action;
  }
};
