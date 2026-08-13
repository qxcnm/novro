export type BulkActionFailure = {
  id: string;
  message: string;
};

export type BulkActionResult = {
  succeeded: string[];
  failed: BulkActionFailure[];
};

/**
 * responseError 封装该名称对应的业务处理逻辑。
 * @param response 当前响应数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
async function responseError(response: Response) {
  const body = await response.json().catch(() => ({})) as { error?: { message?: string } };
  return body.error?.message ?? "操作失败，请稍后重试";
}

/**
 * runBulkAction 封装该名称对应的业务处理逻辑。
 * @param ids 本次操作需要使用的输入参数。
 * @param action 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export async function runBulkAction(
  ids: readonly string[],
  action: (id: string) => Promise<Response>,
  concurrency = 4,
): Promise<BulkActionResult> {
  const succeeded: string[] = [];
  const failed: BulkActionFailure[] = [];
  let cursor = 0;

  /**
   * worker 封装该名称对应的业务处理逻辑。
   * @param none 无参数。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  async function worker() {
    while (cursor < ids.length) {
      const id = ids[cursor];
      cursor += 1;

      try {
        const response = await action(id);
        if (response.ok) {
          succeeded.push(id);
        } else {
          failed.push({ id, message: await responseError(response) });
        }
      } catch {
        failed.push({ id, message: "网络请求失败，请稍后重试" });
      }
    }
  }

  const workerCount = Math.min(Math.max(1, concurrency), ids.length);
  await Promise.all(Array.from({ length: workerCount }, () => worker()));
  return { succeeded, failed };
}

/**
 * bulkResultMessage 封装该名称对应的业务处理逻辑。
 * @param actionLabel 本次操作需要使用的输入参数。
 * @param result 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export function bulkResultMessage(actionLabel: string, result: BulkActionResult) {
  if (result.failed.length === 0) {
    return `已批量${actionLabel} ${result.succeeded.length} 项`;
  }

  const reasons = [...new Set(result.failed.map((failure) => failure.message))].slice(0, 2).join("；");
  if (result.succeeded.length === 0) {
    return `批量${actionLabel}失败，共 ${result.failed.length} 项：${reasons}`;
  }
  return `已批量${actionLabel} ${result.succeeded.length} 项，${result.failed.length} 项失败：${reasons}`;
}
