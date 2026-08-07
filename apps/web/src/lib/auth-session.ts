export type CurrentUser = {
  id: string;
  username: string;
  email: string;
  display_name: string;
  role: "admin" | "member";
  billing_group?: { display_name: string; multiplier_bps: number };
};

export type SessionCheck =
  | { status: "authenticated"; user: CurrentUser }
  | { status: "unauthenticated" }
  | { status: "unavailable" };

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
