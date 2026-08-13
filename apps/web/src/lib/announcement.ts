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

/**
 * localDateKey 封装该名称对应的业务处理逻辑。
 * @param now 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function localDateKey(now: Date) {
  const year = now.getFullYear();
  const month = String(now.getMonth() + 1).padStart(2, "0");
  const day = String(now.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

/**
 * dismissalStorageKey 封装该名称对应的业务处理逻辑。
 * @param userID 目标用户的唯一标识。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function dismissalStorageKey(userID: string) {
  return `novro:announcement:dismissed:${userID}`;
}

/**
 * isAnnouncementDismissedToday 封装该名称对应的业务处理逻辑。
 * @param storage 本次操作需要使用的输入参数。
 * @param userID 目标用户的唯一标识。
 * @param now 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export function isAnnouncementDismissedToday(storage: AnnouncementDismissalStorage, userID: string, now = new Date()) {
  try {
    return storage.getItem(dismissalStorageKey(userID)) === localDateKey(now);
  } catch {
    return false;
  }
}

/**
 * dismissAnnouncementForToday 封装该名称对应的业务处理逻辑。
 * @param storage 本次操作需要使用的输入参数。
 * @param userID 目标用户的唯一标识。
 * @param now 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export function dismissAnnouncementForToday(storage: AnnouncementDismissalStorage, userID: string, now = new Date()) {
  try {
    storage.setItem(dismissalStorageKey(userID), localDateKey(now));
  } catch {
    // Closing the dialog should still work when browser storage is unavailable.
  }
}

/**
 * normalizeAnnouncement 封装该名称对应的业务处理逻辑。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export function normalizeAnnouncement(value?: Partial<Announcement>): Announcement {
  const title = typeof value?.title === "string" ? value.title.trim() : "";
  const body = typeof value?.body === "string" ? value.body.trim() : "";
  const available = value?.available === true && title !== "" && body !== "";
  return available ? { available, title, body } : emptyAnnouncement;
}

/**
 * readAnnouncementError 封装该名称对应的业务处理逻辑。
 * @param response 当前响应数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export async function readAnnouncementError(response: Response) {
  const body = await response.json().catch(() => ({})) as { error?: { message?: string } };
  return body.error?.message ?? "系统公告加载失败，请稍后重试";
}
