// Reader-facing labels for the MFA sign-in chart.
export const title = "Sign-in with a second factor";

export const state = (v) => {
  switch (v.phase) {
    case "signedOut":
      return "Sign-in form";
    case "challenge":
      return `Asks for a code (${v.tries} of 5 wrong so far)`;
    case "signedIn":
      return "Signed in";
    case "expired":
      return "Challenge expired after 5 minutes";
    case "locked":
      return "Locked after 5 wrong codes";
    default:
      return v.phase;
  }
};

export const group = (v) => (v.phase === "signedIn" ? "done" : v.phase === "locked" || v.phase === "expired" ? "blocked" : "active");

export const colors = {
  done: "teal",
  blocked: "pink",
  active: "blue",
};

export const step = (action, from, to) => {
  switch (action) {
    case "SignIn":
      return "you submit your password";
    case "WrongCode":
      return "you enter a wrong code";
    case "FifthWrongCode":
      return "you enter a fifth wrong code";
    case "RightCode":
      return "you enter the right code";
    case "TimeOut":
      return "5 minutes pass";
    case "StartOver":
      return "you start sign-in again";
    case "Done":
      return null;
    default:
      return action;
  }
};
