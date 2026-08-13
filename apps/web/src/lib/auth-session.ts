export type CurrentUser = {
  id: string;
  username: string;
  email: string;
  display_name: string;
  role: "admin" | "member";
};

export type SessionCheck =
  | { status: "authenticated"; user: CurrentUser }
  | { status: "unauthenticated" }
  | { status: "unavailable" };

/**
 * checkCurrentSession 封装该名称对应的业务处理逻辑。
 * @param fetcher 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export async function checkCurrentSession(fetcher: typeof fetch = fetch): Promise<SessionCheck> {
  try {
    const response = await fetcher("/api/auth/me", {
      cache: "no-store",
      credentials: "same-origin",
    });
    if (response.status === 401) return { status: "unauthenticated" };
    if (!response.ok) return { status: "unavailable" };
    const body = await response.json() as { user?: CurrentUser };
    if (!body.user?.id || !body.user.username) return { status: "unavailable" };
    return { status: "authenticated", user: body.user };
  } catch {
    return { status: "unavailable" };
  }
}
