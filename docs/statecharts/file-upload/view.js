// Reader-facing labels for the file upload chart.
export const title = "Uploading a private file";

export const state = (v) => {
  switch (v.phase) {
    case "idle":
      return "Choose a file";
    case "initiating":
      return "Asking the server for an upload slot";
    case "uploading":
      return "Sending the file";
    case "completing":
      return "Asking the server to finish";
    case "success":
      return "Upload complete, page reloads";
    case "error":
      if (v.failedAt === "put") return "Failed while sending: server copy stays pending";
      if (v.failedAt === "complete") return "Failed after the bytes arrived: server copy stays pending";
      if (v.held > 0) return "Failed before sending, and an earlier server copy stays pending";
      return "Failed before anything was sent";
    default:
      return v.phase;
  }
};

export const group = (v) => (v.phase === "success" ? "done" : v.phase === "error" ? "blocked" : "active");

export const colors = {
  done: "teal",
  blocked: "pink",
  active: "blue",
};

export const step = (action, from, to) => {
  switch (action) {
    case "Submit":
      return from.phase === "error" ? "you try again" : "you choose the file and submit";
    case "InitiateFails":
      return "the server refuses the upload";
    case "InitiateOk":
      return "the server reserves space";
    case "PutFails":
      return "sending the file fails";
    case "PutOk":
      return "the file is sent";
    case "CompleteFails":
      return "finishing the upload fails";
    case "CompleteOk":
      return "the server marks the file ready";
    case "Done":
      return null;
    default:
      return action;
  }
};
