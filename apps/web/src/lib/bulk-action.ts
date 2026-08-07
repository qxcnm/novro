export type BulkActionFailure = {
  id: string;
  message: string;
};

export type BulkActionResult = {
  succeeded: string[];
  failed: BulkActionFailure[];
};

async function responseError(response: Response) {
  const body = await response.json().catch(() => ({})) as { error?: { message?: string } };
  return body.error?.message ?? "操作失败，请稍后重试";
}

export async function runBulkAction(
  ids: readonly string[],
  action: (id: string) => Promise<Response>,
  concurrency = 4,
): Promise<BulkActionResult> {
  const succeeded: string[] = [];
  const failed: BulkActionFailure[] = [];
  let cursor = 0;

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
