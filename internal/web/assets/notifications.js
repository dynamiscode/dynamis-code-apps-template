const unreadCount = document.querySelector("[data-unread-count]");
const liveStatus = document.querySelector("[data-notification-live]");

if (unreadCount && window.EventSource) {
	const cursor = document.querySelector("[data-notification-cursor]")?.dataset.notificationCursor ?? "";
	const stream = new EventSource(`/notifications/events?after=${encodeURIComponent(cursor)}`);
	stream.addEventListener("open", () => {
		if (liveStatus) liveStatus.textContent = "";
	});
	stream.addEventListener("notification.created", () => {
		const count = Number(unreadCount.textContent);
		if (Number.isSafeInteger(count) && count >= 0) unreadCount.textContent = String(count + 1);
		if (liveStatus) liveStatus.textContent = liveStatus.dataset.updated;
	});
	stream.addEventListener("error", () => {
		if (liveStatus) liveStatus.textContent = liveStatus.dataset.disconnected;
	});
	stream.addEventListener("resync", () => location.reload());
}
