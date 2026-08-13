export type Announcement = {
  available: boolean;
  title: string;
  body: string;
};

export const emptyAnnouncement: Announcement = {
  available: false,
  title: "",
  body: "",
};

type AnnouncementDismissalStorage = Pick<Storage, "getItem" | "setItem">;

function localDateKey(now: Date) {
  const year = now.getFullYear();
  const month = String(now.getMonth() + 1).padStart(2, "0");
  const day = String(now.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function dismissalStorageKey(userID: string) {
  return `novro:announcement:dismissed:${userID}`;
}

export function isAnnouncementDismissedToday(storage: AnnouncementDismissalStorage, userID: string, now = new Date()) {
  try {
    return storage.getItem(dismissalStorageKey(userID)) === localDateKey(now);
  } catch {
    return false;
  }
}

export function dismissAnnouncementForToday(storage: AnnouncementDismissalStorage, userID: string, now = new Date()) {
  try {
    storage.setItem(dismissalStorageKey(userID), localDateKey(now));
  } catch {
    // Closing the dialog should still work when browser storage is unavailable.
  }
}

export function normalizeAnnouncement(value?: Partial<Announcement>): Announcement {
  const title = typeof value?.title === "string" ? value.title.trim() : "";
  const body = typeof value?.body === "string" ? value.body.trim() : "";
  const available = value?.available === true && title !== "" && body !== "";
  return available ? { available, title, body } : emptyAnnouncement;
}

export async function readAnnouncementError(response: Response) {
  const body = await response.json().catch(() => ({})) as { error?: { message?: string } };
  return body.error?.message ?? "系统公告加载失败，请稍后重试";
}
