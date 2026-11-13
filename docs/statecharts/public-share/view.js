// Reader-facing labels for the public share chart. The chart has two regions.
export const title = "Sharing one item with a public link";

export const state = (v) => ({
  Owner: {
    none: "No link",
    active: "Link active",
    revoked: "Link revoked",
    expired: "Link expired after 7 days",
  }[v.link],
  Visitor: {
    nothing: "Has not opened the link",
    item: "Sees the item title and status",
    notFound: "Sees: not found",
  }[v.seen],
});

export const colors = {
  done: "teal",
  blocked: "pink",
  active: "blue",
};

export const step = (action, from, to) => {
  switch (action) {
    case "Create":
      return "the owner creates a link";
    case "Revoke":
      return "the owner revokes the link";
    case "Expire":
      return "7 days pass";
    case "Visit":
      return to.seen === "item" ? "the visitor opens the link" : "the visitor opens a dead link";
    case "Done":
      return null;
    default:
      return action;
  }
};
