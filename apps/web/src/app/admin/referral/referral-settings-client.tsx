"use client";

import { type FormEvent, useCallback, useEffect, useState } from "react";
import { CheckCircle2, Percent, RefreshCw, Save } from "lucide-react";
import { useRouter } from "next/navigation";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

type ReferralConfig = {
  reward_bps: number;
  updated_at?: string;
};

const defaultConfig: ReferralConfig = { reward_bps: 1_000 };

/**
 * readError 封装该名称对应的业务处理逻辑。
 * @param response 当前响应数据。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
async function readError(response: Response) {
  const body = await response.json().catch(() => ({})) as { error?: { message?: string } };
  return body.error?.message ?? "操作失败，请稍后重试";
}

/**
 * formatPercent 封装该名称对应的业务处理逻辑。
 * @param rewardBPS 本次操作需要使用的输入参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function formatPercent(rewardBPS: number) {
  return (rewardBPS / 100).toFixed(2);
}

/**
 * formatDate 封装该名称对应的业务处理逻辑。
 * @param value 需要处理的输入值。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
function formatDate(value?: string) {
  return value ? new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)) : "使用环境默认值";
}

/**
 * ReferralSettingsClient 渲染对应的 React 界面组件。
 * @param none 无参数。
 * @author Gao Hongshun
 * @date 2026-08-13
 */
export default function ReferralSettingsClient() {
  const router = useRouter();
  const [config, setConfig] = useState<ReferralConfig>(defaultConfig);
  const [percentage, setPercentage] = useState(formatPercent(defaultConfig.reward_bps));
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    const response = await fetch("/api/admin/referral", { cache: "no-store", credentials: "same-origin" });
    if (response.status === 401) { window.location.replace("/login"); return; }
    if (response.status === 403) { router.replace("/console"); return; }
    if (!response.ok) { setError(await readError(response)); setLoading(false); return; }
    /**
     * next 封装该名称对应的业务处理逻辑。
     * @param none 无参数。
     * @author Gao Hongshun
     * @date 2026-08-13
     */
    const next = ((await response.json()) as { referral_config?: ReferralConfig }).referral_config ?? defaultConfig;
    setConfig(next);
    setPercentage(formatPercent(next.reward_bps));
    setLoading(false);
  }, [router]);

  useEffect(() => {
    const timer = window.setTimeout(() => void load(), 0);
    return () => window.clearTimeout(timer);
  }, [load]);

  /**
   * save 封装该名称对应的业务处理逻辑。
   * @param event 触发当前处理流程的事件。
   * @author Gao Hongshun
   * @date 2026-08-13
   */
  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setMessage("");
    setError("");
    if (percentage.trim() === "") {
      setError("请输入返现比例");
      return;
    }
    const parsed = Number(percentage);
    const rewardBPS = Math.round(parsed * 100);
    if (!Number.isFinite(parsed) || parsed < 0 || parsed > 100 || Math.abs(rewardBPS / 100 - parsed) > 0.000001) {
      setError("返现比例必须是 0% 到 100% 之间的数字，最多保留两位小数");
      return;
    }

    setSaving(true);
    try {
      const response = await fetch("/api/admin/referral", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        credentials: "same-origin",
        body: JSON.stringify({ reward_bps: rewardBPS }),
      });
      if (!response.ok) {
        setError(await readError(response));
        return;
      }
      /**
       * next 封装该名称对应的业务处理逻辑。
       * @param none 无参数。
       * @author Gao Hongshun
       * @date 2026-08-13
       */
      const next = ((await response.json()) as { referral_config?: ReferralConfig }).referral_config ?? { reward_bps: rewardBPS };
      setConfig(next);
      setPercentage(formatPercent(next.reward_bps));
      setMessage("返现比例已保存，新的充值到账将立即使用该比例。");
    } catch {
      setError("返现设置保存失败，请稍后重试");
    } finally {
      setSaving(false);
    }
  }

  return (
    <form className="mx-auto max-w-2xl space-y-5" onSubmit={save}>
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <span className="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-md border bg-background text-muted-foreground"><Percent aria-hidden="true" className="size-4" /></span>
          <div>
            <div className="flex flex-wrap items-center gap-2"><p className="font-medium">推荐返现</p><Badge variant="secondary">全局</Badge></div>
            <p className="mt-1 text-sm text-muted-foreground">统一控制邀请好友充值后的返现比例。</p>
          </div>
        </div>
        <div className="flex shrink-0 gap-2">
          <Button aria-label="刷新返现设置" disabled={loading || saving} onClick={() => void load()} size="icon" title="刷新返现设置" type="button" variant="outline"><RefreshCw className={loading ? "animate-spin" : ""} /></Button>
          <Button disabled={loading || saving} type="submit"><Save />{saving ? "保存中..." : "保存设置"}</Button>
        </div>
      </div>

      {message ? <div className="flex items-start gap-2 rounded-md border border-emerald-600/25 bg-emerald-500/5 px-4 py-3 text-sm text-emerald-700 dark:text-emerald-400" role="status"><CheckCircle2 className="mt-0.5 size-4 shrink-0" /><span>{message}</span></div> : null}
      {error ? <p className="rounded-md border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive" role="alert">{error}</p> : null}

      <Card>
        <CardHeader>
          <CardTitle>返现比例</CardTitle>
          <CardDescription>按好友实际支付的充值金额计算，不包含充值赠送金额。</CardDescription>
        </CardHeader>
        <CardContent className="space-y-5">
          <div className="space-y-2">
            <Label htmlFor="referral-reward-percentage">返现比例</Label>
            <div className="relative max-w-xs">
              <Input className="pr-10 text-right tabular-nums" id="referral-reward-percentage" inputMode="decimal" max={100} min={0} onChange={(event) => setPercentage(event.target.value)} step={0.01} type="number" value={percentage} />
              <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">%</span>
            </div>
            <p className="text-xs text-muted-foreground">设为 0% 会暂停新的返现，已经到账的奖励不会改变。</p>
          </div>

          <dl className="grid gap-4 border-t pt-5 text-sm sm:grid-cols-2">
            <div><dt className="text-muted-foreground">当前比例</dt><dd className="mt-1 text-lg font-semibold tabular-nums">{formatPercent(config.reward_bps)}%</dd></div>
            <div><dt className="text-muted-foreground">每充值 ¥100</dt><dd className="mt-1 text-lg font-semibold tabular-nums">返现 ¥{(config.reward_bps / 100).toFixed(2)}</dd></div>
            <div className="sm:col-span-2"><dt className="text-muted-foreground">最近保存</dt><dd className="mt-1 font-medium">{formatDate(config.updated_at)}</dd></div>
          </dl>
        </CardContent>
      </Card>
    </form>
  );
}
