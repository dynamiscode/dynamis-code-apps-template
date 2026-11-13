// Reader-facing labels for the setup chart. Model: verification/web/SetupWizard.tla
// (the form-wizard skill).
export const title = "First-run setup in two steps";

export const state = (v) => {
  if (v.result === "created") return "Owner and workspace created";
  if (v.result === "serverError") return "Server rejected the setup";
  if (v.step === 1) return v.account ? "Step 1: account filled" : "Step 1: account incomplete";
  return v.workspace ? "Step 2: workspace filled" : "Step 2: workspace incomplete";
};

export const group = (v) => (v.result === "created" ? "done" : v.result === "serverError" ? "error" : "active");
export const colors = { done: "teal", error: "orange", active: "blue" };

export const step = (action, from, to) => {
  switch (action) {
    case "YouFillAccount": return to.account ? "you fill the account step" : "you break an account field";
    case "YouFillWorkspace": return to.workspace ? "you fill the workspace step" : "you clear the workspace name";
    case "YouPressNext": return to.step === 2 ? "you press Next" : "you press Next [step invalid: errors shown]";
    case "YouPressBack": return "you press Back";
    case "YouPressCreate": return to.result === "created" ? "you press Create owner" : to.result === "serverError" ? "you press Create owner, the server refuses" : "you press Create owner [errors shown]";
    case "YouSeeServerError": return "the page shows the error on step 1";
    case "Done": return null;
    default: return action;
  }
};
