export function localDayStartISOString(now = new Date()) {
  const start = new Date(now);
  start.setHours(0, 0, 0, 0);
  return start.toISOString();
}

export function todayUsageURL(now = new Date()) {
  const query = new URLSearchParams({
    from: localDayStartISOString(now),
    limit: "1",
  });
  return `/api/account/usage?${query.toString()}`;
}
