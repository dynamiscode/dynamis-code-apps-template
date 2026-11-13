---- MODULE ItemCreate ----
(*
The item create form on /workspaces/{id}/items, submitted through htmx.

Code this model follows:
- internal/web/templates/item-list.html: the create form carries a hidden
  idempotency_key, posts with hx-post and hx-sync="this:drop", swaps
  #item-list (outerHTML), and echoes the rejected title (value="{{.NewTitle}}").
- internal/web/assets/forms.js: the submit handler marks the form submitting
  at once (a second submit is cancelled), then disables its buttons and shows
  "Saving"; htmx:finally:request clears it and shows a toast when the request
  fails without an answer below 500.
- internal/web/assets/htmx.min.js (htmx 4): hx-sync "drop" ignores a trigger
  while one request is in flight. htmx.config.noSwap in app.js drops 5xx.
- internal/web/handler.go: createItem, renderItems (new CreateKey on every
  render, NewTitle echoed on a rejected create, errors re-rendered as 200 for
  htmx), itemError (ErrIdempotencyConflict renders 409 with a message).
- internal/items/service.go: Create trims and validates the title, then
  create() looks up the idempotency record first (replay or conflict), then
  the item limit, then inserts the item and the record in one transaction.

Each server step is one serializable transaction, as in create().
*)
EXTENDS Naturals, FiniteSets

CONSTANTS
    Titles,    \* titles the user can type, e.g. {"A", "B"}
    Blank,     \* a title of only spaces: the browser accepts it, the server rejects it
    MaxItems,  \* the workspace item limit (items.ErrLimit)
    MaxPress   \* how many times the user may press Add

NoTitle == "none"
Typed == Titles \cup {Blank}

VARIABLES
    box,       \* what the title box shows
    page,      \* the current form; also its idempotency key (new on each render)
    reqs,      \* requests from Add presses: [el, key, title, stage, out]
    created,   \* items stored: set of [id, key, title]
    records,   \* idempotency records: set of [key, title]
    msg,       \* what the page shows: "none" | "saved" | "rejected" | "failed"
    rejected,  \* the title of the last rejected press, or NoTitle
    busy,      \* the form is submitting: buttons disabled, "Saving" shown
    lost,      \* an Add press was neither stored nor answered on screen
    presses

vars == <<box, page, reqs, created, records, msg, rejected, busy, lost, presses>>

Init ==
    /\ box = ""
    /\ page = 1
    /\ reqs = {}
    /\ created = {}
    /\ records = {}
    /\ msg = "none"
    /\ rejected = NoTitle
    /\ busy = FALSE
    /\ lost = FALSE
    /\ presses = 0

\* The user types into the title box. Typing clears the shown message.
YouType(t) ==
    /\ box # t
    /\ box' = t
    /\ msg' = "none"
    /\ UNCHANGED <<page, reqs, created, records, rejected, busy, lost, presses>>

\* The user presses Add (or Enter). While the form is submitting, forms.js
\* cancels the submit, and hx-sync drop would ignore it, so nothing is sent.
YouPressAdd ==
    /\ box # ""
    /\ presses < MaxPress
    /\ presses' = presses + 1
    /\ IF busy THEN
           UNCHANGED <<reqs, busy>>
       ELSE
           /\ reqs' = reqs \cup {[el |-> page, key |-> page, title |-> box, stage |-> "sent", out |-> "none"]}
           /\ busy' = TRUE
    /\ UNCHANGED <<box, page, created, records, msg, rejected, lost>>

\* The server runs create() for one request, in one transaction.
ServerAnswers(r) ==
    /\ r \in reqs
    /\ r.stage = "sent"
    /\ LET out ==
            IF r.title = Blank THEN "invalid"
            ELSE IF \E rec \in records : rec.key = r.key THEN
                (IF [key |-> r.key, title |-> r.title] \in records THEN "replay" ELSE "conflict")
            ELSE IF Cardinality(created) >= MaxItems THEN "limit"
            ELSE "ok"
       IN  /\ reqs' = (reqs \ {r}) \cup {[r EXCEPT !.stage = "answered", !.out = out]}
           /\ created' = IF out = "ok" THEN created \cup {[id |-> Cardinality(created) + 1, key |-> r.key, title |-> r.title]} ELSE created
           /\ records' = IF out = "ok" THEN records \cup {[key |-> r.key, title |-> r.title]} ELSE records
    /\ UNCHANGED <<box, page, msg, rejected, busy, lost, presses>>

\* The request fails with no usable answer (a 5xx, or the network drops).
RequestFails(r) ==
    /\ r \in reqs
    /\ r.stage = "sent"
    /\ reqs' = (reqs \ {r}) \cup {[r EXCEPT !.stage = "answered", !.out = "failed"]}
    /\ UNCHANGED <<box, page, created, records, msg, rejected, busy, lost, presses>>

Stored(r) == (\E c \in created : c.key = r.key /\ c.title = r.title) \/ r.out = "replay"

\* The answer reaches the browser. A failure keeps the form and the box as they
\* were and shows a toast. Any other answer swaps in the re-rendered list: a new
\* form and key; the box is empty after a save and keeps the title after a reject.
ResponseArrives(r) ==
    /\ r \in reqs
    /\ r.stage = "answered"
    /\ reqs' = reqs \ {r}
    /\ busy' = FALSE
    /\ LET swaps == r.el = page /\ r.out # "failed"
           isReject == r.out \in {"invalid", "limit", "conflict"}
       IN  /\ page' = IF swaps THEN page + 1 ELSE page
           /\ box' = IF swaps THEN (IF isReject THEN r.title ELSE "") ELSE box
           /\ msg' = IF r.out = "failed" THEN "failed"
                     ELSE IF swaps THEN (IF isReject THEN "rejected" ELSE "saved") ELSE msg
           /\ rejected' = IF swaps /\ isReject THEN r.title ELSE IF swaps THEN NoTitle ELSE rejected
           \* A blank title that gets no answer is not counted: it could never be stored.
           /\ lost' = (lost \/ (~swaps /\ r.out # "failed" /\ ~Stored(r) /\ r.title # Blank))
    /\ UNCHANGED <<created, records, presses>>

Idle ==
    /\ reqs = {}
    /\ presses = MaxPress
    /\ UNCHANGED vars

YouTypeAny == \E t \in Typed : YouType(t)
ServerStep == \E r \in reqs : ServerAnswers(r)
FailStep == \E r \in reqs : RequestFails(r)
BrowserStep == \E r \in reqs : ResponseArrives(r)

Next == YouTypeAny \/ YouPressAdd \/ ServerStep \/ FailStep \/ BrowserStep \/ Idle

Spec == Init /\ [][Next]_vars

\* ---- Rules ----

\* One Add press never makes two items, even when the browser resends it.
OneItemPerKey == \A k \in {c.key : c \in created} :
    Cardinality({c \in created : c.key = k}) = 1

\* Every Add press with a real title is stored, or the page tells you it was not.
NoSilentLoss == ~lost

\* After the server rejects your title, the box still shows what you typed.
InputKeptOnReject == msg = "rejected" => box = rejected

\* While your Add is on its way, the button shows it is working.
BusyWhileSending == (\E r \in reqs : r.el = page) => busy

\* At most one Add is in flight from the form.
OneRequestInFlight == Cardinality(reqs) <= 1

====
