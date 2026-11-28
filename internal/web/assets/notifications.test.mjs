import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const componentsCode = fs.readFileSync(path.join(__dirname, "alpine-components.js"), "utf8");

function loadNotifications() {
	const listeners = {};
	const sources = [];
	let closed = false;
	let reloads = 0;
	class FakeEventSource {
		constructor(url) {
			this.url = url;
			sources.push(this);
		}
		addEventListener(name, listener) {
			(listeners[name] ??= []).push(listener);
		}
		close() {
			closed = true;
		}
	}
	const unreadCount = { textContent: "2" };
	const liveStatus = { textContent: "", dataset: { updated: "New notice. Refresh to see it.", disconnected: "Reconnecting." } };
	const cursorNode = { dataset: { notificationCursor: "cursor /+" } };
	const root = {
		querySelector: (selector) => selector === "[data-unread-count]" ? unreadCount :
			selector === "[data-notification-live]" ? liveStatus :
			selector === "[data-notification-cursor]" ? cursorNode : null,
	};
	const fakeWindow = { EventSource: FakeEventSource };
	const fakeDocument = { addEventListener() {} };
	const exportStyle = { module: true, exports: {} };
	new Function("module", "window", "document", "EventSource", "location", componentsCode)(
		exportStyle, fakeWindow, fakeDocument, FakeEventSource, { reload: () => reloads++ }
	);
	const component = exportStyle.exports.notificationsLive();
	component.$root = root;
	component.init();
	return { listeners, sources, unreadCount, liveStatus, get closed() { return closed; }, get reloads() { return reloads; }, fire(name) { for (const listener of listeners[name] ?? []) listener(); } };
}

test("notification events update live unread feedback and resync reloads", () => {
	const state = loadNotifications();
	assert.equal(state.sources.length, 1);
	assert.equal(state.sources[0].url, "/notifications/events?after=cursor%20%2F%2B");
	state.fire("notification.created");
	assert.equal(state.unreadCount.textContent, "3");
	assert.equal(state.liveStatus.textContent, "New notice. Refresh to see it.");
	state.fire("notification.created");
	assert.equal(state.unreadCount.textContent, "4");

	state.fire("error");
	assert.equal(state.liveStatus.textContent, "Reconnecting.");
	state.fire("open");
	assert.equal(state.liveStatus.textContent, "");
	state.fire("resync");
	assert.equal(state.reloads, 1);
	assert.equal(state.listeners.close, undefined);
	assert.equal(state.closed, false);
});
