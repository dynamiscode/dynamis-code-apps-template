// Reader-facing labels for the invitation chart. The chart has two regions.
export const title = "Opening and accepting a workspace invitation";

export const state = (v) => ({
  Invitation: {
    pending: "Pending",
    revoked: "Revoked by the owner",
    expired: "Expired",
    accepted: "Accepted",
  }[v.inv],
  Person: {
    notStarted: "Has not opened the link",
    linkOpened: "Opened the link",
    signIn: "Asked to sign in",
    matchingEmail: "Sees invitation, email matches",
    wrongEmail: "Signed in with another email",
    joined: "Joined the workspace",
    unusable: "Sees: invitation not usable",
  }[v.page],
});

export const colors = {
  done: "teal",
  blocked: "pink",
  active: "blue",
};

export const step = (action, from, to) => {
  switch (action) {
    case "OpenLink":
      return "you open the link";
    case "Route":
      return to.page === "unusable" ? "the link is no longer usable" : "the link asks you to sign in";
    case "SignIn":
      return to.page === "matchingEmail" ? "you sign in with the invited email" : "you sign in with another email";
    case "SwitchAccount":
      return "you switch accounts";
    case "Accept":
      return "you accept";
    case "Refused":
      return "you try to accept after it changed";
    case "Revoke":
      return "the owner revokes it";
    case "Expire":
      return "its time runs out";
    case "Done":
      return null;
    default:
      return action;
  }
};
