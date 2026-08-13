/**
 * localDayStartISOString 封装该名称对应的业务处理逻辑。
 * @param now 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export function localDayStartISOString(now = new Date()) {
  const start = new Date(now);
  start.setHours(0, 0, 0, 0);
  return start.toISOString();
}

/**
 * todayUsageURL 封装该名称对应的业务处理逻辑。
 * @param now 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export function todayUsageURL(now = new Date()) {
  const query = new URLSearchParams({
    from: localDayStartISOString(now),
    limit: "1",
  });
  return `/api/account/usage?${query.toString()}`;
}
