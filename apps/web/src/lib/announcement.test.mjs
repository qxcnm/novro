import assert from "node:assert/strict";
import test from "node:test";

import {
  dismissAnnouncementForToday,
  emptyAnnouncement,
  isAnnouncementDismissedToday,
  normalizeAnnouncement,
} from "./announcement.ts";

test("announcement is available only with a title and body", () => {
  assert.deepEqual(normalizeAnnouncement({ available: true, title: " 通知 ", body: " 内容 " }), {
    available: true,
    title: "通知",
    body: "内容",
  });
  assert.deepEqual(normalizeAnnouncement({ available: true, title: "通知", body: "" }), emptyAnnouncement);
});

test("disabled announcement does not expose draft content", () => {
  assert.deepEqual(normalizeAnnouncement({ available: false, title: "草稿", body: "内部内容" }), emptyAnnouncement);
});

test("announcement can be dismissed for the current local calendar day per user", () => {
  const values = new Map();
  const storage = {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
  };
  const today = new Date(2026, 7, 13, 23, 59);

  assert.equal(isAnnouncementDismissedToday(storage, "user-1", today), false);
  dismissAnnouncementForToday(storage, "user-1", today);
  assert.equal(isAnnouncementDismissedToday(storage, "user-1", today), true);
  assert.equal(isAnnouncementDismissedToday(storage, "user-2", today), false);
  assert.equal(isAnnouncementDismissedToday(storage, "user-1", new Date(2026, 7, 14, 0, 1)), false);
});

test("unavailable browser storage does not block announcement display or closing", () => {
  const storage = {
    getItem: () => { throw new Error("storage unavailable"); },
    setItem: () => { throw new Error("storage unavailable"); },
  };

  assert.equal(isAnnouncementDismissedToday(storage, "user-1"), false);
  assert.doesNotThrow(() => dismissAnnouncementForToday(storage, "user-1"));
});
